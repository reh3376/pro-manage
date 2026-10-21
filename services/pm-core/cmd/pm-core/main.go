// pm-core is the Go monolith at the centre of pro_manage (FSD §05).
//
// Slice 0 ships the skeleton: configuration, secrets, structured logging,
// /healthz, /readyz (Postgres ping when DATABASE_URL is set), /version, and
// graceful shutdown. Domain services arrive in later slices.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/reh3376/pro-manage/services/pm-core/internal/config"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
	"github.com/reh3376/pro-manage/services/pm-core/internal/secrets"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pm-core:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	sp, err := secrets.FromEnvironment()
	if err != nil {
		return err
	}
	cfg, err := config.Load(ctx, "pm-core", sp)
	if err != nil {
		return err
	}
	log := cfg.Logger()

	checks := map[string]httpx.ReadyFunc{}
	if cfg.DatabaseURL != "" {
		pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("postgres pool: %w", err)
		}
		defer pool.Close()
		checks["postgres"] = pool.Ping
		log.Info("postgres configured")
	} else {
		log.Warn("DATABASE_URL not set; running without a database (slice 0 only)")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", httpx.Health("pm-core", config.Version))
	mux.HandleFunc("GET /readyz", httpx.Ready("pm-core", checks))
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"service": "pm-core", "version": config.Version})
	})

	return httpx.Run(ctx, log, cfg.ListenAddr, mux)
}
