// pm-core is the Go monolith at the centre of pro_manage (FSD §05).
//
// Configuration, secrets, structured logging, migrations (goose, embedded),
// the audit chain, Site and Project services with audit-in-transaction,
// policy-variable resolution, the MDEMG adapter and governance gate, the toy
// CAR workflow, and the /api/v1 REST edge behind a development actor header.
// SSO arrives with IdPi_OIDC.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/reh3376/pro-manage/services/pm-core/internal/api"
	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/config"
	"github.com/reh3376/pro-manage/services/pm-core/internal/domain"
	"github.com/reh3376/pro-manage/services/pm-core/internal/governance"
	"github.com/reh3376/pro-manage/services/pm-core/internal/grpcserver"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
	"github.com/reh3376/pro-manage/services/pm-core/internal/secrets"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
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

	mux := http.NewServeMux()
	checks := map[string]httpx.ReadyFunc{}

	if cfg.DatabaseURL == "" {
		log.Warn("DATABASE_URL not set; serving health endpoints only")
	} else {
		if cfg.AuditChainKey == "" {
			return errors.New("AUDIT_CHAIN_KEY is required when DATABASE_URL is set (FSD §08 audit chain)")
		}
		chain, err := audit.NewChain([]byte(cfg.AuditChainKey))
		if err != nil {
			return err
		}
		if cfg.DBAutoMigrate {
			if err := store.Migrate(ctx, log, cfg.DatabaseURL); err != nil {
				return err
			}
		}
		st, err := store.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer st.Close()
		checks["postgres"] = st.Pool.Ping
		log.Info("postgres configured", "auto_migrate", cfg.DBAutoMigrate, "dev_actor_header", cfg.DevActorHeader)

		gate := &governance.Gate{OrgID: cfg.GovernanceOrgID, Log: log, Cache: governance.NewPreclassCache(cfg.PreclassifyTTL)}
		svc := &domain.Service{Store: st, Chain: chain, Tenant: cfg.TenantID, Gate: gate, Log: log}
		a := &api.API{Log: log, Domain: svc, Chain: chain, DevActorHeader: cfg.DevActorHeader, MDEMGBaseURL: cfg.MDEMGBaseURL}
		if cfg.MDEMGBaseURL != "" {
			mc, err := mdemgadapter.New(mdemgadapter.Options{BaseURL: cfg.MDEMGBaseURL, APIKey: cfg.MDEMGAPIKey, ClassifyTimeout: cfg.MDEMGTimeout, UserAgent: "pro-manage/pm-core " + config.Version})
			if err != nil {
				return err
			}
			a.MDEMG = mc
			gate.Client = mc
			h := mc.Health(ctx)
			log.Info("mdemg configured", "base_url", cfg.MDEMGBaseURL, "governance_space", mdemgadapter.GovernanceSpace(cfg.GovernanceOrgID), "reachable", h.Reachable, "jiminy_enabled", h.JiminyEnabled, "latency_ms", h.LatencyMs)
		}
		a.Mount(mux)
	}

	mux.HandleFunc("GET /healthz", httpx.Health("pm-core", config.Version))
	mux.HandleFunc("GET /readyz", httpx.Ready("pm-core", checks))
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"service": "pm-core", "version": config.Version, "git_sha": config.GitSHA})
	})

	// gRPC (internal surface) and HTTP (edge) share one lifetime: a signal or a
	// failure of either stops both.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	grpcSrv := grpcserver.New(log, grpcserver.Options{Addr: cfg.GRPCAddr, Version: config.Version, GitSHA: config.GitSHA, Reflection: cfg.GRPCReflection})
	grpcErr := make(chan error, 1)
	go func() { grpcErr <- grpcSrv.Run(ctx) }()

	httpErr := httpx.Run(ctx, log, cfg.ListenAddr, mux)
	stop()
	if err := <-grpcErr; err != nil && httpErr == nil {
		return err
	}
	return httpErr
}
