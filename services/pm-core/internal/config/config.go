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

// Version and GitSHA are set at build time with -ldflags "-X .../config.Version=... -X .../config.GitSHA=...".
var (
	Version = "dev"
	GitSHA  = "unknown"
)

// Config is the resolved configuration for one pm-core (or pm-workers) process.
type Config struct {
	ServiceName string
	ListenAddr  string
	// GRPCAddr is pm-core's internal gRPC listener (FSD §05); GRPCReflection enables server reflection (dev/CI only).
	GRPCAddr       string
	GRPCReflection bool
	LogLevel       slog.Level
	LogFormat      string // json | text

	// DatabaseURL is the Postgres DSN for pm-core's OLTP store. When unset the
	// binary serves health endpoints only (slice 0 behaviour).
	DatabaseURL string
	// DBAutoMigrate applies embedded goose migrations at startup (dev and CI;
	// production runs `pm-core migrate` as a release step later).
	DBAutoMigrate bool
	// AuditChainKey signs the AuditEvent chain (secret, >= 32 bytes).
	AuditChainKey string
	// TenantID is constant for a single-company install (FSD §03).
	TenantID string
	// DevActorHeader accepts X-Dev-Actor as the acting user. Development only;
	// off, every /api/v1 call is 401 until IdPi_OIDC lands.
	DevActorHeader bool

	// MDEMG connection. BaseURL is non-secret; APIKey is a secret.
	MDEMGBaseURL string
	MDEMGAPIKey  string
	MDEMGTimeout time.Duration
	// GovernanceOrgID names the governance space, org.<id>.governance (FSD §06; roadmap: org.pm_dev.governance in dev).
	GovernanceOrgID string
	// PreclassifyTTL is how long a settled form's verdict stays valid (§06 default 10 minutes).
	PreclassifyTTL time.Duration

	// Identity and sessions (FSD §04). Login is enabled when both IdPiAddr and RedisURL are set.
	IdPiAddr      string        // IDPI_OIDC_ADDR, the IdPi_OIDC module's gRPC address
	RedisURL      string        // REDIS_URL, the session store
	PublicURL     string        // PM_PUBLIC_URL, where the browser reaches pm-core (callback base)
	FrontendURL   string        // PM_FRONTEND_URL, default return_to and post-logout landing
	SessionTTL    time.Duration // PM_SESSION_TTL sliding window (default 8h)
	SessionMaxAge time.Duration // PM_SESSION_MAX_AGE absolute cap (default 24h)
	CookieSecure  bool          // PM_COOKIE_SECURE (true behind TLS)

	// Workers
	WorkerTick        time.Duration
	OutboxMaxAttempts int           // dead-letter after this many failed deliveries
	OutboxBaseBackoff time.Duration // first retry delay; doubles per attempt, capped at 10 min
}

// Load resolves configuration for the named service.
func Load(ctx context.Context, service string, sp secrets.Provider) (Config, error) {
	cfg := Config{
		ServiceName:       service,
		ListenAddr:        envOr("PM_"+upper(service)+"_LISTEN_ADDR", defaultAddr(service)),
		LogFormat:         envOr("PM_LOG_FORMAT", "json"),
		MDEMGBaseURL:      strings.TrimRight(envOr("MDEMG_BASE_URL", "http://127.0.0.1:9998"), "/"),
		MDEMGTimeout:      durationOr("MDEMG_TIMEOUT", 10*time.Second),
		GovernanceOrgID:   envOr("PM_GOVERNANCE_ORG_ID", "pm_dev"),
		PreclassifyTTL:    durationOr("PM_PRECLASSIFY_TTL", 10*time.Minute),
		WorkerTick:        durationOr("PM_WORKERS_TICK", 30*time.Second),
		OutboxMaxAttempts: intOr("PM_OUTBOX_MAX_ATTEMPTS", 10),
		OutboxBaseBackoff: durationOr("PM_OUTBOX_BASE_BACKOFF", 5*time.Second),
		GRPCAddr:          envOr("PM_CORE_GRPC_ADDR", ":8090"),
		GRPCReflection:    boolOr("PM_CORE_GRPC_REFLECTION", false),
		DBAutoMigrate:     boolOr("PM_DB_AUTO_MIGRATE", false),
		DevActorHeader:    boolOr("PM_DEV_ACTOR_HEADER", false),
		TenantID:          envOr("PM_TENANT_ID", "default"),
		IdPiAddr:          os.Getenv("IDPI_OIDC_ADDR"),
		RedisURL:          os.Getenv("REDIS_URL"),
		PublicURL:         strings.TrimRight(envOr("PM_PUBLIC_URL", "http://localhost:8080"), "/"),
		FrontendURL:       strings.TrimRight(envOr("PM_FRONTEND_URL", "http://localhost:3100"), "/"),
		SessionTTL:        durationOr("PM_SESSION_TTL", 8*time.Hour),
		SessionMaxAge:     durationOr("PM_SESSION_MAX_AGE", 24*time.Hour),
		CookieSecure:      boolOr("PM_COOKIE_SECURE", false),
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
	if cfg.AuditChainKey, err = optionalSecret(ctx, sp, "AUDIT_CHAIN_KEY"); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func intOr(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}

func boolOr(key string, def bool) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
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
