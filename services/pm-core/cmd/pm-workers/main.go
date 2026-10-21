// pm-workers runs pro_manage's scheduled jobs and outbox dispatch (FSD §05).
//
// Slice 0 ships the loop and the health endpoint only. The outbox drain to
// MDEMG and Commsi, the SLA tick, and the harvest triggers arrive with the
// features they serve.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/internal/config"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
	"github.com/reh3376/pro-manage/services/pm-core/internal/secrets"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pm-workers:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sp, err := secrets.FromEnvironment()
	if err != nil {
		return err
	}
	cfg, err := config.Load(ctx, "pm-workers", sp)
	if err != nil {
		return err
	}
	log := cfg.Logger()

	go func() {
		t := time.NewTicker(cfg.WorkerTick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				// Job leasing and the outbox drain land here in later slices.
				log.Debug("tick", "at", now.UTC().Format(time.RFC3339))
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", httpx.Health("pm-workers", config.Version))
	mux.HandleFunc("GET /readyz", httpx.Ready("pm-workers", nil))
	return httpx.Run(ctx, log, cfg.ListenAddr, mux)
}
