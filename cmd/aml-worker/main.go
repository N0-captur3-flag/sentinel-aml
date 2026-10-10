package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"sentinel-aml/internal/config"
	"sentinel-aml/internal/db"
	"sentinel-aml/internal/networkroom"
)

type networkRoomProcessor interface {
	ProcessDirtyBatch(context.Context, int, time.Duration) (networkroom.LifecycleStats, error)
}

type workerConfig struct {
	BatchSize    int
	LockTTL      time.Duration
	TickTimeout  time.Duration
	ScanInterval time.Duration
}

func main() {
	cfg := workerConfigFromEnv()
	once := flag.Bool("once", false, "run one deterministic worker scan and exit")
	interval := flag.Duration("interval", cfg.ScanInterval, "network-room scan interval")
	flag.Parse()
	cfg.ScanInterval = *interval

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(ctx, config.DatabaseFromEnv())
	if err != nil {
		logger.Error("aml-worker database unavailable", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(database); err != nil {
			logger.Error("aml-worker database close failed", "error", err)
		}
	}()

	repo := networkroom.NewRepository(database)
	repo.SetLifecycleLimits(
		intFromEnv("NETWORK_ROOM_CASE_LIMIT_PER_CANDIDATE", networkroom.DefaultCaseLimitPerCandidate),
		intFromEnv("NETWORK_ROOM_ARTIFACT_LIMIT_PER_CASE", networkroom.DefaultArtifactLimitPerCase),
	)
	if err := run(ctx, logger, repo, cfg, *once); err != nil {
		logger.Error("aml-worker failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger, processor networkRoomProcessor, cfg workerConfig, once bool) error {
	if processor == nil {
		return nil
	}
	cfg = normalizeWorkerConfig(cfg)
	if err := processNetworkRooms(ctx, logger, processor, cfg); err != nil {
		return err
	}
	if once {
		logger.Info("aml-worker dirty batch completed")
		return nil
	}

	ticker := time.NewTicker(cfg.ScanInterval)
	defer ticker.Stop()
	logger.Info("aml-worker dirty queue processor started", "interval", cfg.ScanInterval.String(), "batch_size", cfg.BatchSize, "timeout", cfg.TickTimeout.String())
	for {
		select {
		case <-ctx.Done():
			logger.Info("aml-worker stopped")
			return nil
		case <-ticker.C:
			if err := processNetworkRooms(ctx, logger, processor, cfg); err != nil {
				return err
			}
		}
	}
}

func processNetworkRooms(ctx context.Context, logger *slog.Logger, processor networkRoomProcessor, cfg workerConfig) error {
	if err := ctx.Err(); err != nil {
		return nil
	}
	tickCtx, cancel := context.WithTimeout(ctx, cfg.TickTimeout)
	defer cancel()

	stats, err := processor.ProcessDirtyBatch(tickCtx, cfg.BatchSize, cfg.LockTTL)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(tickCtx.Err(), context.DeadlineExceeded) {
			logger.Warn("network-room dirty batch timed out", "timeout", cfg.TickTimeout.String(), "error", err)
			return nil
		}
		logger.Warn("network-room dirty batch failed", "error", err)
		return nil
	}
	logger.Info(
		"network-room dirty batch completed",
		"groups", stats.GroupsProcessed,
		"cases_attached", stats.CasesAttached,
		"participants_joined", stats.ParticipantsJoined,
		"duplicate_case_links", stats.DuplicateCaseLinks,
		"duplicate_participants", stats.DuplicateParticipants,
		"artifacts_materialized", stats.ArtifactsMaterialized,
		"duplicate_artifacts", stats.DuplicateArtifacts,
	)
	return nil
}

func workerConfigFromEnv() workerConfig {
	return workerConfig{
		BatchSize:    intFromEnv("NETWORK_ROOM_BATCH_SIZE", networkroom.DefaultDirtyBatchSize),
		LockTTL:      durationSecondsFromEnv("NETWORK_ROOM_LOCK_TTL_SECONDS", networkroom.DefaultDirtyLockTTL),
		TickTimeout:  durationMillisFromEnv("NETWORK_ROOM_WORKER_TIMEOUT_MS", 5000*time.Millisecond),
		ScanInterval: durationSecondsFromEnv("NETWORK_ROOM_SCAN_INTERVAL_SECONDS", 5*time.Second),
	}
}

func normalizeWorkerConfig(cfg workerConfig) workerConfig {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = networkroom.DefaultDirtyBatchSize
	}
	if cfg.LockTTL <= 0 {
		cfg.LockTTL = networkroom.DefaultDirtyLockTTL
	}
	if cfg.TickTimeout <= 0 {
		cfg.TickTimeout = 5000 * time.Millisecond
	}
	if cfg.ScanInterval <= 0 {
		cfg.ScanInterval = 5 * time.Second
	}
	return cfg
}

func intFromEnv(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func durationSecondsFromEnv(name string, fallback time.Duration) time.Duration {
	return time.Duration(intFromEnv(name, int(fallback/time.Second))) * time.Second
}

func durationMillisFromEnv(name string, fallback time.Duration) time.Duration {
	return time.Duration(intFromEnv(name, int(fallback/time.Millisecond))) * time.Millisecond
}
