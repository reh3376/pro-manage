// Package config loads pm-core's runtime configuration.
//
// Non-secret settings come from environment variables with PM_ prefixes and
// sane defaults. Secrets come only through secrets.Provider, never read
// directly from the environment by other packages.
package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/internal/secrets"
)

// Version is set at build time with -ldflags "-X .../config.Version=...".
var Version = "dev"

// Config is the resolved configuration for one pm-core (or pm-workers) process.
type Config struct {
	ServiceName string
	ListenAddr  string
	LogLevel    slog.Level
	LogFormat   string // json | text

	// DatabaseURL is the Postgres DSN for pm-core's OLTP store. Optional in
	// slice 0 so the skeleton runs without a database; required from slice 1.
	DatabaseURL string

	// MDEMG connection. BaseURL is non-secret; APIKey is a secret.
	MDEMGBaseURL string
	MDEMGAPIKey  string
	MDEMGTimeout time.Duration

	// Workers
	WorkerTick time.Duration
}

// Load resolves configuration for the named service.
func Load(ctx context.Context, service string, sp secrets.Provider) (Config, error) {
	cfg := Config{
		ServiceName:  service,
		ListenAddr:   envOr("PM_"+upper(service)+"_LISTEN_ADDR", defaultAddr(service)),
		LogFormat:    envOr("PM_LOG_FORMAT", "json"),
		MDEMGBaseURL: strings.TrimRight(envOr("MDEMG_BASE_URL", "http://127.0.0.1:9998"), "/"),
		MDEMGTimeout: durationOr("MDEMG_TIMEOUT", 10*time.Second),
		WorkerTick:   durationOr("PM_WORKERS_TICK", 30*time.Second),
	}

	switch strings.ToLower(envOr("PM_LOG_LEVEL", "info")) {
	case "debug":
		cfg.LogLevel = slog.LevelDebug
	case "warn", "warning":
		cfg.LogLevel = slog.LevelWarn
	case "error":
		cfg.LogLevel = slog.LevelError
	default:
		cfg.LogLevel = slog.LevelInfo
	}

	var err error
	if cfg.DatabaseURL, err = optionalSecret(ctx, sp, "DATABASE_URL"); err != nil {
		return cfg, err
	}
	if cfg.MDEMGAPIKey, err = optionalSecret(ctx, sp, "MDEMG_API_KEY"); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Logger builds the process logger from the config.
func (c Config) Logger() *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.LogLevel}
	var h slog.Handler
	if c.LogFormat == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(h).With("service", c.ServiceName, "version", Version)
}

func optionalSecret(ctx context.Context, sp secrets.Provider, name string) (string, error) {
	v, err := sp.Get(ctx, name)
	if err == nil {
		return v, nil
	}
	if errors.Is(err, secrets.ErrNotFound) {
		return "", nil
	}
	return "", fmt.Errorf("config: %s: %w", name, err)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func durationOr(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func upper(s string) string { return strings.ToUpper(strings.ReplaceAll(s, "-", "_")) }

func defaultAddr(service string) string {
	switch service {
	case "pm-workers":
		return ":8081"
	case "stub-mdemg":
		return ":9999"
	default:
		return ":8080"
	}
}
