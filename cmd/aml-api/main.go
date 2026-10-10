package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sentinel-aml/internal/account"
	"sentinel-aml/internal/alert"
	"sentinel-aml/internal/audit"
	"sentinel-aml/internal/auth"
	"sentinel-aml/internal/casework"
	"sentinel-aml/internal/config"
	"sentinel-aml/internal/customer"
	"sentinel-aml/internal/db"
	"sentinel-aml/internal/edd"
	"sentinel-aml/internal/exporting"
	"sentinel-aml/internal/graph"
	"sentinel-aml/internal/httpapi"
	"sentinel-aml/internal/networkroom"
	"sentinel-aml/internal/risk"
	"sentinel-aml/internal/transaction"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(ctx, config.DatabaseFromEnv())
	if err != nil {
		logger.Error("aml-api database unavailable", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(database); err != nil {
			logger.Error("aml-api database close failed", "error", err)
		}
	}()

	authService := auth.NewService(auth.NewRepository(database), auth.ServiceConfig{})
	transactionRepo := transaction.NewRepository(database)
	alertRepo := alert.NewRepository(database)
	caseRepo := casework.NewRepository(database)
	handler := httpapi.NewServer(
		authService,
		httpapi.WithDomainRepositories(
			customer.NewRepository(database),
			account.NewRepository(database),
			transactionRepo,
		),
		httpapi.WithInvestigationRepositories(alertRepo, caseRepo),
		httpapi.WithTransactionRiskService(risk.NewService(transactionRepo, alertRepo)),
		httpapi.WithEDDRepository(edd.NewRepository(database)),
		httpapi.WithAuditRepository(audit.NewRepository(database)),
		httpapi.WithGraphRepository(graph.NewRepository(database)),
		httpapi.WithExportRepository(exporting.NewRepository(database)),
		httpapi.WithNetworkRoomRepository(networkroom.NewRepository(database)),
	).Routes()

	server := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("aml-api listening", "addr", *addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("aml-api shutdown failed", "error", err)
			os.Exit(1)
		}
		logger.Info("aml-api stopped")
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("aml-api failed", "error", err)
			os.Exit(1)
		}
	}
}
