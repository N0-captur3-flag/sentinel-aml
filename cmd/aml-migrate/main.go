package main

import (
	"context"
	"log/slog"
	"os"

	"sentinel-aml/internal/config"
	"sentinel-aml/internal/db"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx := context.Background()

	database, err := db.Open(ctx, config.DatabaseFromEnv())
	if err != nil {
		logger.Error("open database failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(database); err != nil {
			logger.Error("close database failed", "error", err)
		}
	}()

	migrations, err := db.LoadMigrations("migrations")
	if err != nil {
		logger.Error("load migrations failed", "error", err)
		os.Exit(1)
	}

	if err := db.ApplyMigrations(ctx, database, migrations); err != nil {
		logger.Error("apply migrations failed", "error", err)
		os.Exit(1)
	}

	logger.Info("migrations applied", "count", len(migrations))
}
