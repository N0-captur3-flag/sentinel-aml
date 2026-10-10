package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"sentinel-aml/internal/config"
)

const postgresDriverName = "pgx"

// Open opens a PostgreSQL database handle and verifies connectivity with a bounded context.
func Open(ctx context.Context, cfg config.DatabaseConfig) (*sql.DB, error) {
	if cfg.URL == "" {
		return nil, errors.New("database url is required")
	}
	timeout := cfg.ConnectTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	db, err := sql.Open(postgresDriverName, cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("open postgres database: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres database: %w", err)
	}

	return db, nil
}

// Close closes a database handle. It accepts nil so callers can defer it safely.
func Close(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}
