package config

import (
	"os"
	"time"
)

const defaultDatabaseTimeout = 5 * time.Second

// DatabaseConfig contains PostgreSQL connection settings for runtime processes.
type DatabaseConfig struct {
	URL            string
	ConnectTimeout time.Duration
}

// DatabaseFromEnv loads database settings without logging or exposing secrets.
func DatabaseFromEnv() DatabaseConfig {
	return DatabaseConfig{
		URL:            os.Getenv("DATABASE_URL"),
		ConnectTimeout: defaultDatabaseTimeout,
	}
}
