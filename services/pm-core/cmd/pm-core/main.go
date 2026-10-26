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
	"github.com/reh3376/pro-manage/services/pm-core/internal/auth"
	"github.com/reh3376/pro-manage/services/pm-core/internal/blobs"
	"github.com/reh3376/pro-manage/services/pm-core/internal/config"
	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/idpi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/sigi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/domain"
	"github.com/reh3376/pro-manage/services/pm-core/internal/governance"
	"github.com/reh3376/pro-manage/services/pm-core/internal/grpcserver"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
	"github.com/reh3376/pro-manage/services/pm-core/internal/notify"
	"github.com/reh3376/pro-manage/services/pm-core/internal/secrets"
	"github.com/reh3376/pro-manage/services/pm-core/internal/sessions"
	"github.com/reh3376/pro-manage/services/pm-core/internal/signing"
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
		sign := &signing.Service{Store: st, Chain: chain, Tenant: cfg.TenantID, Log: log}
		if cfg.SigiAddr != "" {
			sc, err := sigi.Dial(cfg.SigiAddr, cfg.SigiProvider)
			if err != nil {
				return err
			}
			defer sc.Close()
			sign.Sigi = sc
		}
		if cfg.S3Endpoint != "" {
			bs, err := blobs.Open(ctx, blobs.Config{Endpoint: cfg.S3Endpoint, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, Bucket: cfg.S3Bucket, Region: cfg.S3Region})
			if err != nil {
				log.Warn("object store not available; signed artifacts cannot be served", "endpoint", cfg.S3Endpoint, "err", err)
			} else {
				sign.Blobs = bs
				checks["s3"] = bs.Ping
			}
		}
		log.Info("signing configured", "sigi", cfg.SigiAddr, "provider", cfg.SigiProvider, "object_store", sign.Blobs != nil)
		svc := &domain.Service{Store: st, Chain: chain, Tenant: cfg.TenantID, Gate: gate, Log: log, Signing: sign, Notifier: &notify.Notifier{Tenant: cfg.TenantID, FallbackDomain: cfg.NotifyFallbackDomain}}
		a := &api.API{Log: log, Domain: svc, Chain: chain, DevActorHeader: cfg.DevActorHeader, MDEMGBaseURL: cfg.MDEMGBaseURL, Signing: sign}
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
		if cfg.IdPiAddr != "" && cfg.RedisURL != "" {
			store, err := sessions.Open(ctx, cfg.RedisURL)
			if err != nil {
				return err
			}
			defer store.Close()
			store.TTL, store.MaxAge = cfg.SessionTTL, cfg.SessionMaxAge
			ic, err := idpi.Dial(cfg.IdPiAddr)
			if err != nil {
				return err
			}
			defer ic.Close()
			checks["redis"] = store.Ping // the IdP's own health shows at /auth/login (503) and the module's /readyz, not here: pm-core serves without it
			a.Auth = &auth.Handler{Log: log, Sessions: store, IdPi: ic, Users: svc, PublicURL: cfg.PublicURL, FrontendURL: cfg.FrontendURL, CookieSecure: cfg.CookieSecure, RequestID: api.RequestIDFrom}
			a.Auth.Mount(mux, a.RequestIDMiddleware)
			log.Info("login configured", "idpi", cfg.IdPiAddr, "callback", cfg.PublicURL+"/auth/callback", "session_ttl", cfg.SessionTTL.String(), "session_max_age", cfg.SessionMaxAge.String())
		} else {
			log.Warn("login not configured (IDPI_OIDC_ADDR and REDIS_URL required); only the dev actor header can authenticate", "dev_actor_header", cfg.DevActorHeader)
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
