// pm-workers runs pro_manage's scheduled jobs and outbox dispatch (FSD §05).
//
// Slice 2b: the outbox drainer (feedback and observe to MDEMG through
// pkg/mdemgadapter) on every tick, with health, readiness, and a /stats view
// of what it has delivered. SLA ticks, harvest triggers, and Commsi delivery
// arrive with the features they serve.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/internal/config"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
	"github.com/reh3376/pro-manage/services/pm-core/internal/outbox"
	"github.com/reh3376/pro-manage/services/pm-core/internal/secrets"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pm-workers:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sp, err := secrets.FromEnvironment()
	if err != nil {
		return err
	}
	cfg, err := config.Load(ctx, "pm-workers", sp)
	if err != nil {
		return err
	}
	log := cfg.Logger()

	checks := map[string]httpx.ReadyFunc{}
	var drainer *outbox.Drainer

	if cfg.DatabaseURL == "" || cfg.MDEMGBaseURL == "" {
		log.Warn("outbox drain disabled: DATABASE_URL and MDEMG_BASE_URL are both required")
	} else {
		st, err := store.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer st.Close()
		checks["postgres"] = st.Pool.Ping
		sink, err := mdemgadapter.New(mdemgadapter.Options{BaseURL: cfg.MDEMGBaseURL, APIKey: cfg.MDEMGAPIKey, UserAgent: "pro-manage/pm-workers " + config.Version})
		if err != nil {
			return err
		}
		host, _ := os.Hostname()
		drainer = &outbox.Drainer{Store: st, Sink: sink, Log: log, Worker: fmt.Sprintf("%s:%d", host, os.Getpid()),
			MaxAttempts: int32(cfg.OutboxMaxAttempts), BaseBackoff: cfg.OutboxBaseBackoff}
		go drainer.Run(ctx, cfg.WorkerTick)
		log.Info("outbox drain running", "tick", cfg.WorkerTick.String(), "max_attempts", cfg.OutboxMaxAttempts, "mdemg", cfg.MDEMGBaseURL)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", httpx.Health("pm-workers", config.Version))
	mux.HandleFunc("GET /readyz", httpx.Ready("pm-workers", checks))
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, _ *http.Request) {
		if drainer == nil {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"outbox": nil, "tick": cfg.WorkerTick.String()})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"outbox": drainer.Counters(), "tick": cfg.WorkerTick.String()})
	})
	err = httpx.Run(ctx, log, cfg.ListenAddr, mux)
	stop()
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

var _ = time.Second
