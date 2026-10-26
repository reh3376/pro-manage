// sigi-stub is the development and CI e-signature connector (FSD §10 "Sigi"):
// pm.connector.v1.SigiService and ConnectorService over gRPC, with a dev HTTP
// endpoint that completes a signer so the whole envelope lifecycle (create,
// sign, status, artifact capture, audit) runs without a provider account.
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
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/reh3376/pro-manage/connectors/sigi/stub/internal/envelopes"
	"github.com/reh3376/pro-manage/connectors/sigi/stub/internal/service"
	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

// Version is set with -ldflags.
var Version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sigi-stub:", err)
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
	grpcAddr, httpAddr := envOr("SIGI_GRPC_ADDR", ":7090"), envOr("SIGI_HTTP_ADDR", ":7091")
	var h slog.Handler = slog.NewJSONHandler(os.Stdout, nil)
	if envOr("PM_LOG_FORMAT", "json") == "text" {
		h = slog.NewTextHandler(os.Stdout, nil)
	}
	log := slog.New(h).With("service", "sigi-stub", "version", Version)
	manifest := service.DefaultManifest
	manifest.Version = Version
	svc := &service.Service{Store: envelopes.New(), Manifest: manifest, Log: log, WebhookSecret: envOr("SIGI_WEBHOOK_SECRET", "stub-webhook-secret")}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := grpc.NewServer()
	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	connectorv1.RegisterConnectorServiceServer(srv, svc)
	connectorv1.RegisterSigiServiceServer(srv, svc)
	if envOr("SIGI_GRPC_REFLECTION", "false") == "true" {
		reflection.Register(srv)
	}
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	hs.SetServingStatus("pm.connector.v1.SigiService", healthpb.HealthCheckResponse_SERVING)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}
	grpcErr := make(chan error, 1)
	go func() {
		log.Info("grpc listening", "addr", grpcAddr)
		grpcErr <- srv.Serve(lis)
	}()

	mux := svc.DevMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","service":"sigi-stub","version":%q}`+"\n", Version)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ready","service":"sigi-stub"}`+"\n")
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
