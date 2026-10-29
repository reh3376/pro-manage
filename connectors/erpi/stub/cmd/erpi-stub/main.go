// erpi-stub is the development and CI ERP connector (FSD §10 "ERPi"):
// pm.connector.v1.ErpiService (the spine subset) and ConnectorService over
// gRPC on an in-memory ledger seeded with the Northfield projects, with dev
// HTTP endpoints that add projects, post actuals, and list the budget releases
// pm-core made, so the ERP mirror and ReleaseBudget run without an ERP.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/reh3376/pro-manage/connectors/erpi/stub/internal/ledger"
	"github.com/reh3376/pro-manage/connectors/erpi/stub/internal/service"
	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

// Version is set with -ldflags.
var Version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "erpi-stub:", err)
		os.Exit(1)
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func run() error {
	grpcAddr, httpAddr := envOr("ERPI_GRPC_ADDR", ":7110"), envOr("ERPI_HTTP_ADDR", ":7111")
	var h slog.Handler = slog.NewJSONHandler(os.Stdout, nil)
	if envOr("PM_LOG_FORMAT", "json") == "text" {
		h = slog.NewTextHandler(os.Stdout, nil)
	}
	log := slog.New(h).With("service", "erpi-stub", "version", Version)
	manifest := service.DefaultManifest
	manifest.Version = Version
	// ERPI_TIERS narrows the declared tiers (e.g. "financials" to play a financials-only ERP).
	if t := strings.TrimSpace(os.Getenv("ERPI_TIERS")); t != "" {
		manifest.Tiers = nil
		for _, x := range strings.Split(t, ",") {
			if x = strings.TrimSpace(x); x != "" {
				manifest.Tiers = append(manifest.Tiers, x)
			}
		}
	}
	l := ledger.New()
	if envOr("ERPI_SEED", "northfield") == "northfield" {
		for _, p := range ledger.Northfield() {
			if err := l.Add(p); err != nil {
				return err
			}
		}
	}
	svc := &service.Service{Ledger: l, Manifest: manifest, Log: log}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := grpc.NewServer()
	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	connectorv1.RegisterConnectorServiceServer(srv, svc)
	connectorv1.RegisterErpiServiceServer(srv, svc)
	if envOr("ERPI_GRPC_REFLECTION", "false") == "true" {
		reflection.Register(srv)
	}
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	hs.SetServingStatus("pm.connector.v1.ErpiService", healthpb.HealthCheckResponse_SERVING)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}
	grpcErr := make(chan error, 1)
	go func() {
		log.Info("grpc listening", "addr", grpcAddr, "tiers", manifest.Tiers, "projects", l.Len())
		grpcErr <- srv.Serve(lis)
	}()

	mux := svc.DevMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","service":"erpi-stub","version":%q}`+"\n", Version)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ready","service":"erpi-stub"}`+"\n")
	})
	hsrv := &http.Server{Addr: httpAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	httpErr := make(chan error, 1)
	go func() {
		log.Info("http listening", "addr", httpAddr)
		if err := hsrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			httpErr <- err
		}
		close(httpErr)
	}()

	select {
	case err := <-httpErr:
		return err
	case err := <-grpcErr:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = hsrv.Shutdown(shutdown)
	hs.Shutdown()
	srv.GracefulStop()
	return nil
}
