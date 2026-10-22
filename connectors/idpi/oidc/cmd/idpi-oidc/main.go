// idpi-oidc is the IdPi_OIDC connector module (FSD §04, §10, §15): the
// generic OpenID Connect backend of the identity connector, serving
// pm.connector.v1.ConnectorService and pm.connector.v1.IdpiService over gRPC
// with /healthz and /readyz over HTTP. pm-core owns users, sessions, and
// grants; this module only talks to the IdP and resolves claims to roles.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/reh3376/pro-manage/connectors/idpi/oidc/internal/mapping"
	"github.com/reh3376/pro-manage/connectors/idpi/oidc/internal/oidc"
	"github.com/reh3376/pro-manage/connectors/idpi/oidc/internal/runtime"
	"github.com/reh3376/pro-manage/connectors/idpi/oidc/internal/service"
	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "idpi-oidc:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := runtime.Load()
	if err != nil {
		return err
	}
	log := cfg.Logger()

	provider, err := oidc.New(oidc.Config{
		Issuer: cfg.Issuer, DiscoveryURL: cfg.DiscoveryURL, BackchannelBase: cfg.BackchannelBase, ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret,
		Scopes: cfg.Scopes, GroupsClaim: cfg.GroupsClaim, RolesClaim: cfg.RolesClaim, AcceptedAudiences: cfg.AcceptedAudiences, DiscoveryTimeout: cfg.DiscoveryTimeout,
	})
	if err != nil {
		return err
	}
	m, err := mapping.Load(cfg.MappingFile)
	if err != nil {
		return err
	}
	manifest := service.DefaultManifest
	manifest.Version = runtime.Version
	svc := &service.Service{Provider: provider, Mapping: m, Manifest: manifest, Log: log}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Discovery is lazy and retried, so the module comes up before its IdP does.
	if d, err := provider.Discover(ctx); err != nil {
		log.Warn("identity provider not reachable yet; will retry on demand", "issuer", cfg.Issuer, "err", err)
	} else {
		log.Info("identity provider discovered", "issuer", d.Issuer, "token_endpoint", d.TokenEndpoint, "mapping_rules", m.Len(), "client_id", cfg.ClientID, "public_client", cfg.ClientSecret == "")
	}

	srv := grpc.NewServer()
	h := health.NewServer()
	healthpb.RegisterHealthServer(srv, h)
	connectorv1.RegisterConnectorServiceServer(srv, svc)
	connectorv1.RegisterIdpiServiceServer(srv, svc)
	if cfg.Reflection {
		reflection.Register(srv)
	}
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	h.SetServingStatus("pm.connector.v1.ConnectorService", healthpb.HealthCheckResponse_SERVING)
	h.SetServingStatus("pm.connector.v1.IdpiService", healthpb.HealthCheckResponse_SERVING)

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("grpc listen %s: %w", cfg.GRPCAddr, err)
	}
	grpcErr := make(chan error, 1)
	go func() {
		log.Info("grpc listening", "addr", cfg.GRPCAddr, "reflection", cfg.Reflection)
		grpcErr <- srv.Serve(lis)
	}()

	checks := map[string]runtime.ReadyFunc{"oidc_discovery": func(ctx context.Context) error { _, err := provider.Discover(ctx); return err }}
	httpErr := runtime.ServeHTTP(ctx, log, cfg.HTTPAddr, runtime.HealthMux(checks))
	stop()

	h.Shutdown()
	done := make(chan struct{})
	go func() { srv.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		srv.Stop()
	}
	if err := <-grpcErr; err != nil && !errors.Is(err, grpc.ErrServerStopped) && httpErr == nil {
		return err
	}
	return httpErr
}
