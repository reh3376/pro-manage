// commsi-smtp is the Commsi_SMTP connector module (FSD §10 "Commsi"): pm-core's
// notifications delivered as email over SMTP (Mailpit in the dev stack).
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/reh3376/pro-manage/connectors/commsi/smtp/internal/mail"
	"github.com/reh3376/pro-manage/connectors/commsi/smtp/internal/service"
	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

// Version is set with -ldflags.
var Version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "commsi-smtp:", err)
		os.Exit(1)
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// secret reads PM_SECRETS_FILE (KEY=value) then the environment, as the other modules do.
func secret(name string) string {
	if path := os.Getenv("PM_SECRETS_FILE"); path != "" {
		if f, err := os.Open(path); err == nil {
			defer f.Close()
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
				if ok && strings.TrimSpace(k) == name {
					return strings.Trim(strings.TrimSpace(v), `"'`)
				}
			}
		}
	}
	return os.Getenv(name)
}

func run() error {
	grpcAddr, httpAddr := envOr("COMMSI_GRPC_ADDR", ":7100"), envOr("COMMSI_HTTP_ADDR", ":7101")
	var h slog.Handler = slog.NewJSONHandler(os.Stdout, nil)
	if envOr("PM_LOG_FORMAT", "json") == "text" {
		h = slog.NewTextHandler(os.Stdout, nil)
	}
	log := slog.New(h).With("service", "commsi-smtp", "version", Version)
	port, _ := strconv.Atoi(envOr("SMTP_PORT", "587"))
	window, _ := time.ParseDuration(envOr("COMMSI_DEDUPE_WINDOW", "10m"))
	sender, err := mail.New(mail.Config{
		Host: envOr("SMTP_HOST", ""), Port: port, StartTLS: envOr("SMTP_STARTTLS", "true") == "true",
		Username: secret("SMTP_USERNAME"), Password: secret("SMTP_PASSWORD"),
		From: envOr("COMMSI_FROM", "pro_manage <no-reply@pro-manage.local>"), DedupeWindow: window, AppBaseURL: envOr("PM_FRONTEND_URL", ""),
	})
	if err != nil {
		return err
	}
	manifest := service.DefaultManifest
	manifest.Version = Version
	svc := &service.Service{Sender: sender, Manifest: manifest, Log: log}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := sender.Probe(ctx); err != nil {
		log.Warn("smtp relay not reachable yet; deliveries will retry through pm-workers", "relay", sender.Addr(), "err", err)
	} else {
		log.Info("smtp relay reachable", "relay", sender.Addr(), "from", envOr("COMMSI_FROM", ""), "dedupe_window", window.String())
	}

	srv := grpc.NewServer()
	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	connectorv1.RegisterConnectorServiceServer(srv, svc)
	connectorv1.RegisterCommsiServiceServer(srv, svc)
	if envOr("COMMSI_GRPC_REFLECTION", "false") == "true" {
		reflection.Register(srv)
	}
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	hs.SetServingStatus("pm.connector.v1.CommsiService", healthpb.HealthCheckResponse_SERVING)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}
	grpcErr := make(chan error, 1)
	go func() {
		log.Info("grpc listening", "addr", grpcAddr)
		grpcErr <- srv.Serve(lis)
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","service":"commsi-smtp","version":%q}`+"\n", Version)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := sender.Probe(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"not_ready","service":"commsi-smtp","checks":{"smtp":%q}}`+"\n", err.Error())
			return
		}
		fmt.Fprint(w, `{"status":"ready","service":"commsi-smtp","checks":{"smtp":"ok"}}`+"\n")
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
