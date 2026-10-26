// Package commsi is pm-core's client for the notification connector contract
// (pm.connector.v1.CommsiService, FSD §10). pm-core decides who is told what
// and when; the module delivers.
package commsi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

// Errors the outbox branches on.
var (
	ErrUnavailable = errors.New("commsi: unavailable")
	ErrRejected    = errors.New("commsi: rejected") // INVALID_ARGUMENT: never retried
)

// Client wraps one connection.
type Client struct {
	conn      *grpc.ClientConn
	commsi    connectorv1.CommsiServiceClient
	connector connectorv1.ConnectorServiceClient
	Timeout   time.Duration
}

// Dial connects lazily.
func Dial(addr string) (*Client, error) {
	if addr == "" {
		return nil, errors.New("commsi: address required")
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("commsi: dial %s: %w", addr, err)
	}
	return &Client{conn: conn, commsi: connectorv1.NewCommsiServiceClient(conn), connector: connectorv1.NewConnectorServiceClient(conn), Timeout: 20 * time.Second}, nil
}

// Close releases the connection.
func (c *Client) Close() error { return c.conn.Close() }

// Health probes the module.
func (c *Client) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	h, err := c.connector.HealthCheck(ctx, &connectorv1.HealthCheckRequest{})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if h.GetStatus() == connectorv1.HealthCheckResponse_STATUS_NOT_SERVING {
		return fmt.Errorf("%w: not serving: %s", ErrUnavailable, h.GetDetail())
	}
	return nil
}

// Send is SendNotification.
func (c *Client) Send(ctx context.Context, req *connectorv1.SendNotificationRequest) (*connectorv1.SendNotificationResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	resp, err := c.commsi.SendNotification(ctx, req)
	return resp, wrap(err)
}

func wrap(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.InvalidArgument:
		return fmt.Errorf("%w: %s", ErrRejected, status.Convert(err).Message())
	case codes.Unavailable, codes.DeadlineExceeded:
		return fmt.Errorf("%w: %s", ErrUnavailable, status.Convert(err).Message())
	default:
		return fmt.Errorf("commsi: %w", err)
	}
}
