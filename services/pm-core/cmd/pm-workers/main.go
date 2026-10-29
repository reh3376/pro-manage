// pm-workers runs pro_manage's scheduled jobs and outbox dispatch (FSD §05).
//
// The outbox drainer (feedback and observe to MDEMG, envelope creation at the
// Sigi module, notifications through the Commsi module), the e-signature
// reconciliation, and the SLA tick (nudge, overdue, escalate, expire) run on
// every tick, with health, readiness, and a /stats view. Harvest triggers
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

	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/blobs"
	"github.com/reh3376/pro-manage/services/pm-core/internal/budget"
	"github.com/reh3376/pro-manage/services/pm-core/internal/config"
	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/commsi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/erpi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/sigi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
	"github.com/reh3376/pro-manage/services/pm-core/internal/notify"
	"github.com/reh3376/pro-manage/services/pm-core/internal/outbox"
	"github.com/reh3376/pro-manage/services/pm-core/internal/secrets"
	"github.com/reh3376/pro-manage/services/pm-core/internal/signing"
	"github.com/reh3376/pro-manage/services/pm-core/internal/sla"
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

		// E-signature (FSD §10 Sigi): create envelopes queued by approvals, then reconcile open ones every tick.
		if cfg.SigiAddr != "" && cfg.AuditChainKey != "" {
			chain, err := audit.NewChain([]byte(cfg.AuditChainKey))
			if err != nil {
				return err
			}
			sc, err := sigi.Dial(cfg.SigiAddr, cfg.SigiProvider)
			if err != nil {
				return err
			}
			defer sc.Close()
			sign := &signing.Service{Store: st, Chain: chain, Sigi: sc, Tenant: cfg.TenantID, Log: log}
			if cfg.S3Endpoint != "" {
				bs, err := blobs.Open(ctx, blobs.Config{Endpoint: cfg.S3Endpoint, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, Bucket: cfg.S3Bucket, Region: cfg.S3Region})
				if err != nil {
					return fmt.Errorf("object store (signed artifacts): %w", err)
				}
				sign.Blobs = bs
				checks["s3"] = bs.Ping
			}
			drainer.Signing = sign
			go func() {
				t := time.NewTicker(cfg.WorkerTick)
				defer t.Stop()
				for {
					if n, err := sign.Reconcile(ctx, 200); err != nil {
						log.Error("envelope reconcile", "err", err)
					} else if n > 0 {
						log.Debug("envelopes reconciled", "open", n)
					}
					select {
					case <-ctx.Done():
						return
					case <-t.C:
					}
				}
			}()
			log.Info("signing running", "sigi", cfg.SigiAddr, "provider", cfg.SigiProvider, "object_store", sign.Blobs != nil)
		} else {
			log.Warn("signing disabled: SIGI_ADDR and AUDIT_CHAIN_KEY are both required; sigi.envelope.create rows will wait")
		}
		// ERP mirror (FSD §10 ERPi): release approved budgets from the outbox and pull every project's
		// budget versions, actuals, and commitments on the sync interval.
		if cfg.ErpiAddr != "" && cfg.AuditChainKey != "" {
			chain, err := audit.NewChain([]byte(cfg.AuditChainKey))
			if err != nil {
				return err
			}
			ec, err := erpi.Dial(cfg.ErpiAddr, cfg.ErpiProvider)
			if err != nil {
				return err
			}
			defer ec.Close()
			bud := &budget.Service{Store: st, Chain: chain, Erpi: ec, Tenant: cfg.TenantID, Log: log}
			drainer.Budget = bud
			go func() {
				t := time.NewTicker(cfg.ErpSyncInterval)
				defer t.Stop()
				for {
					if n, err := bud.SyncDue(ctx, 50); err != nil {
						log.Warn("erp sync", "synced", n, "err", err)
					} else if n > 0 {
						log.Info("erp sync", "synced", n)
					}
					select {
					case <-ctx.Done():
						return
					case <-t.C:
					}
				}
			}()
			log.Info("erp mirror running", "erpi", cfg.ErpiAddr, "provider", cfg.ErpiProvider, "sync_interval", cfg.ErpSyncInterval.String())
		} else {
			log.Warn("erp mirror disabled: ERPI_ADDR and AUDIT_CHAIN_KEY are both required; erpi.budget.release rows will wait")
		}
		// Notifications (FSD §10 Commsi) and the SLA tick (§08): the module delivers what
		// the outbox holds; the tick nudges, flags overdue, escalates, and expires stages.
		if cfg.CommsiAddr != "" {
			cc, err := commsi.Dial(cfg.CommsiAddr)
			if err != nil {
				return err
			}
			defer cc.Close()
			drainer.Notify = cc
			log.Info("notifications running", "commsi", cfg.CommsiAddr)
		} else {
			log.Warn("notifications disabled: COMMSI_ADDR not set; commsi.send rows will wait")
		}
		if cfg.AuditChainKey != "" {
			chain, err := audit.NewChain([]byte(cfg.AuditChainKey))
			if err != nil {
				return err
			}
			ticker := &sla.Ticker{Store: st, Chain: chain, Notifier: &notify.Notifier{Tenant: cfg.TenantID, FallbackDomain: cfg.NotifyFallbackDomain}, Tenant: cfg.TenantID, Log: log}
			go func() {
				t := time.NewTicker(cfg.WorkerTick)
				defer t.Stop()
				for {
					if c, err := ticker.Tick(ctx); err != nil {
						log.Error("sla tick", "err", err)
					} else if c.Nudged+c.Overdue+c.Escalated+c.Expired > 0 {
						log.Info("sla tick", "scanned", c.Scanned, "nudged", c.Nudged, "overdue", c.Overdue, "escalated", c.Escalated, "expired", c.Expired)
					}
					select {
					case <-ctx.Done():
						return
					case <-t.C:
					}
				}
			}()
			log.Info("sla tick running", "tick", cfg.WorkerTick.String())
		}
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
