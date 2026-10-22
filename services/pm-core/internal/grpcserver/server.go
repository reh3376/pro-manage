// Package grpcserver hosts pm-core's internal gRPC surface (FSD §05): the
// pm.core.v1 services, the standard health service, and server reflection in
// development. Every RPC is covered by a UDTS spec (docs/tests/udts).
package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	corev1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/core/v1"
)

// Options configure the server.
type Options struct {
	Addr       string
	Version    string
	GitSHA     string
	Reflection bool // enable server reflection (dev and CI; off in production)
}

// Server wraps grpc.Server with health reporting.
type Server struct {
	srv    *grpc.Server
	health *health.Server
	addr   string
	log    *slog.Logger
}

// New builds the server with every pm.core.v1 service registered.
func New(log *slog.Logger, opts Options) *Server {
	srv := grpc.NewServer()
	h := health.NewServer()
	healthpb.RegisterHealthServer(srv, h)
	corev1.RegisterVersionServiceServer(srv, &versionService{service: "pm-core", version: opts.Version, gitSHA: opts.GitSHA})
	if opts.Reflection {
		reflection.Register(srv)
	}
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	h.SetServingStatus("pm.core.v1.VersionService", healthpb.HealthCheckResponse_SERVING)
	return &Server{srv: srv, health: h, addr: opts.Addr, log: log}
}

// Run serves until ctx is cancelled, then stops gracefully (10 s budget).
func (s *Server) Run(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("grpc: listen %s: %w", s.addr, err)
	}
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("grpc listening", "addr", s.addr)
		if err := s.srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	s.health.Shutdown() // flips every service to NOT_SERVING for in-flight probes
	done := make(chan struct{})
	go func() {
		s.srv.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		s.srv.Stop()
	}
	s.log.Info("grpc stopped")
	return nil
}

type versionService struct {
	corev1.UnimplementedVersionServiceServer
	service, version, gitSHA string
}

func (v *versionService) GetVersion(context.Context, *corev1.GetVersionRequest) (*corev1.GetVersionResponse, error) {
	return &corev1.GetVersionResponse{Service: v.service, Version: v.version, GitSha: v.gitSHA}, nil
}
