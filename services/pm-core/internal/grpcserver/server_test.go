package grpcserver

import (
	"context"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	corev1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/core/v1"
)

func TestVersionAndHealthOverTheWire(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	s := New(log, Options{Addr: addr, Version: "test", GitSHA: "abc1234", Reflection: true})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	callCtx, callCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer callCancel()

	var resp *corev1.GetVersionResponse
	for i := 0; i < 50; i++ { // wait for the listener
		resp, err = corev1.NewVersionServiceClient(conn).GetVersion(callCtx, &corev1.GetVersionRequest{})
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if resp.GetService() != "pm-core" || resp.GetVersion() != "test" || resp.GetGitSha() != "abc1234" {
		t.Fatalf("unexpected response %+v", resp)
	}
	h, err := healthpb.NewHealthClient(conn).Check(callCtx, &healthpb.HealthCheckRequest{Service: ""})
	if err != nil || h.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("health: %v %v", err, h.GetStatus())
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("server did not stop")
	}
}
