// Package httpx holds the small amount of HTTP plumbing every binary shares:
// a server with sane timeouts, graceful shutdown, JSON helpers, and the
// standard /healthz and /readyz handlers.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ReadyFunc reports readiness; a non-nil error means not ready.
type ReadyFunc func(ctx context.Context) error

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Health returns a handler for /healthz: the process is up.
func Health(service, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": service, "version": version})
	}
}

// Ready returns a handler for /readyz: dependencies answer.
func Ready(service string, checks map[string]ReadyFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		WriteJSON(w, status, map[string]any{"status": map[int]string{200: "ready", 503: "not_ready"}[status], "service": service, "checks": result})
	}
}

// Run serves handler on addr until SIGINT/SIGTERM or ctx cancellation, then
// drains in-flight requests for up to 10 seconds.
func Run(ctx context.Context, log *slog.Logger, addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", addr)
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

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
