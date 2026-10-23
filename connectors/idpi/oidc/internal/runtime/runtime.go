// Package runtime is the module's process plumbing: configuration from the
// environment, secrets through a provider door (file then env, as pm-core's
// secrets package does), logging, and the HTTP health listener. It is the part
// of IdPi_OIDC the Connector SDK (FSD §15) will absorb, so it stays small and
// free of OIDC knowledge.
package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// Version and GitSHA are set with -ldflags at build time.
var (
	Version = "dev"
	GitSHA  = "unknown"
)

// Config is the resolved process configuration.
type Config struct {
	GRPCAddr   string
	HTTPAddr   string
	LogLevel   slog.Level
	LogFormat  string
	Reflection bool

	Issuer            string
	DiscoveryURL      string
	BackchannelBase   string
	ClientID          string
	ClientSecret      string // secret
	Scopes            []string
	GroupsClaim       string
	RolesClaim        string
	AcceptedAudiences []string
	MappingFile       string
	DiscoveryTimeout  time.Duration
}

// Load reads the environment. Secrets come from PM_SECRETS_FILE (KEY=value
// lines) when set, then the environment.
func Load() (Config, error) {
	cfg := Config{
		GRPCAddr:         envOr("IDPI_GRPC_ADDR", ":7080"),
		HTTPAddr:         envOr("IDPI_HTTP_ADDR", ":7081"),
		LogFormat:        envOr("PM_LOG_FORMAT", "json"),
		Reflection:       boolOr("IDPI_GRPC_REFLECTION", false),
		Issuer:           strings.TrimRight(os.Getenv("OIDC_ISSUER"), "/"),
		DiscoveryURL:     os.Getenv("OIDC_DISCOVERY_URL"),
		BackchannelBase:  os.Getenv("OIDC_BACKCHANNEL_BASE"),
		ClientID:         os.Getenv("OIDC_CLIENT_ID"),
		Scopes:           strings.Fields(envOr("OIDC_SCOPES", "openid profile email")),
		GroupsClaim:      envOr("OIDC_GROUPS_CLAIM", "groups"),
		RolesClaim:       envOr("OIDC_ROLES_CLAIM", "realm_access.roles"),
		MappingFile:      envOr("IDPI_MAPPING_FILE", "/etc/idpi/mapping.yaml"),
		DiscoveryTimeout: durationOr("OIDC_DISCOVERY_TIMEOUT", 5*time.Second),
	}
	if v := os.Getenv("OIDC_ACCEPTED_AUDIENCES"); v != "" {
		cfg.AcceptedAudiences = strings.Split(v, ",")
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
	if cfg.Issuer == "" || cfg.ClientID == "" {
		return cfg, errors.New("OIDC_ISSUER and OIDC_CLIENT_ID are required")
	}
	secret, err := Secret("OIDC_CLIENT_SECRET")
	if err != nil && !errors.Is(err, ErrSecretNotFound) {
		return cfg, err
	}
	cfg.ClientSecret = secret
	return cfg, nil
}

// ErrSecretNotFound is returned when no source holds the secret.
var ErrSecretNotFound = errors.New("secret not found")

// Secret resolves a secret from PM_SECRETS_FILE, then the environment.
func Secret(name string) (string, error) {
	if path := os.Getenv("PM_SECRETS_FILE"); path != "" {
		f, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("secrets file: %w", err)
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if ok && strings.TrimSpace(k) == name && strings.TrimSpace(v) != "" {
				return strings.Trim(strings.TrimSpace(v), `"'`), nil
			}
		}
	}
	if v := os.Getenv(name); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("%q: %w", name, ErrSecretNotFound)
}

// Logger builds the process logger.
func (c Config) Logger() *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.LogLevel}
	var h slog.Handler
	if c.LogFormat == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(h).With("service", "idpi-oidc", "version", Version)
}

// ReadyFunc reports readiness.
type ReadyFunc func(ctx context.Context) error

// HealthMux serves /healthz and /readyz.
func HealthMux(checks map[string]ReadyFunc) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "idpi-oidc", "version": Version})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		result := map[string]string{}
		status := http.StatusOK
		for name, check := range checks {
			if err := check(ctx); err != nil {
				result[name] = "fail: " + err.Error()
				status = http.StatusServiceUnavailable
			} else {
				result[name] = "ok"
			}
		}
		writeJSON(w, status, map[string]any{"status": map[int]string{200: "ready", 503: "not_ready"}[status], "service": "idpi-oidc", "checks": result})
	})
	return mux
}

// ServeHTTP runs the health listener until ctx is cancelled.
func ServeHTTP(ctx context.Context, log *slog.Logger, addr string, h http.Handler) error {
	srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		log.Info("http listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func boolOr(k string, d bool) bool {
	switch strings.ToLower(os.Getenv(k)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return d
}

func durationOr(k string, d time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if x, err := time.ParseDuration(v); err == nil {
			return x
		}
	}
	return d
}
