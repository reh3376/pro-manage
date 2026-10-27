// Package erpi is pm-core's client for the ERP connector contract
// (pm.connector.v1.ErpiService, FSD §10 "ERPi"). Thin, like the other
// connector clients: the module talks to the ERP; pm-core keeps the mirror and
// the release record. One configured instance today (ERPI_ADDR); the registry
// and per-site routing (§05) come with the second ERP.
package erpi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

// Errors the callers branch on.
var (
	ErrUnavailable  = errors.New("erpi: unavailable")
	ErrRejected     = errors.New("erpi: rejected")      // INVALID_ARGUMENT: a bad request, never retried
	ErrNotFound     = errors.New("erpi: not found")     // unknown project or record
	ErrNotSupported = errors.New("erpi: not supported") // FAILED_PRECONDITION: the instance does not declare the tier
)

// Client wraps one connection.
type Client struct {
	conn      *grpc.ClientConn
	erpi      connectorv1.ErpiServiceClient
	connector connectorv1.ConnectorServiceClient
	Provider  string // what the module is (stub | ifs | ...), recorded on every mirrored row
	Timeout   time.Duration
}

// Dial connects lazily.
func Dial(addr, provider string) (*Client, error) {
	if addr == "" {
		return nil, errors.New("erpi: address required")
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("erpi: dial %s: %w", addr, err)
	}
	if provider == "" {
		provider = "unknown"
	}
	return &Client{conn: conn, erpi: connectorv1.NewErpiServiceClient(conn), connector: connectorv1.NewConnectorServiceClient(conn), Provider: provider, Timeout: 30 * time.Second}, nil
}

// Close releases the connection.
func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) ctx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.Timeout)
}

// Health probes the module.
func (c *Client) Health(ctx context.Context) error {
	ctx, cancel := c.ctx(ctx)
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

// Capabilities returns the module's declared tiers and capabilities.
func (c *Client) Capabilities(ctx context.Context) (*connectorv1.CapabilitiesResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.connector.Capabilities(ctx, &connectorv1.CapabilitiesRequest{})
	return resp, wrap(err)
}

// BudgetVersions drains FetchBudgetVersions.
func (c *Client) BudgetVersions(ctx context.Context, projectRef string, since *time.Time) ([]*connectorv1.BudgetVersion, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	req := &connectorv1.FetchBudgetVersionsRequest{ProjectRef: projectRef}
	if since != nil {
		req.Since = ts(*since)
	}
	stream, err := c.erpi.FetchBudgetVersions(ctx, req)
	if err != nil {
		return nil, wrap(err)
	}
	var out []*connectorv1.BudgetVersion
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, wrap(err)
		}
		if v := msg.GetVersion(); v != nil {
			out = append(out, v)
		}
	}
}

// ForecastVersions drains FetchForecastVersions.
func (c *Client) ForecastVersions(ctx context.Context, projectRef string, since *time.Time) ([]*connectorv1.BudgetVersion, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	req := &connectorv1.FetchForecastVersionsRequest{ProjectRef: projectRef}
	if since != nil {
		req.Since = ts(*since)
	}
	stream, err := c.erpi.FetchForecastVersions(ctx, req)
	if err != nil {
		return nil, wrap(err)
	}
	var out []*connectorv1.BudgetVersion
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, wrap(err)
		}
		if v := msg.GetVersion(); v != nil {
			out = append(out, v)
		}
	}
}

// Eac is FetchEac.
func (c *Client) Eac(ctx context.Context, projectRef string) (*connectorv1.FetchEacResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.erpi.FetchEac(ctx, &connectorv1.FetchEacRequest{ProjectRef: projectRef})
	return resp, wrap(err)
}

// Actuals drains FetchActuals.
func (c *Client) Actuals(ctx context.Context, projectRef string, from, to *time.Time) ([]*connectorv1.Actual, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	req := &connectorv1.FetchActualsRequest{ProjectRef: projectRef}
	if from != nil {
		req.FromTs = ts(*from)
	}
	if to != nil {
		req.ToTs = ts(*to)
	}
	stream, err := c.erpi.FetchActuals(ctx, req)
	if err != nil {
		return nil, wrap(err)
	}
	var out []*connectorv1.Actual
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, wrap(err)
		}
		if a := msg.GetActual(); a != nil {
			out = append(out, a)
		}
	}
}

// Commitments drains FetchCommitments.
func (c *Client) Commitments(ctx context.Context, projectRef string, includeClosed bool) ([]*connectorv1.Commitment, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	stream, err := c.erpi.FetchCommitments(ctx, &connectorv1.FetchCommitmentsRequest{ProjectRef: projectRef, IncludeClosed: includeClosed})
	if err != nil {
		return nil, wrap(err)
	}
	var out []*connectorv1.Commitment
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, wrap(err)
		}
		if cm := msg.GetCommitment(); cm != nil {
			out = append(out, cm)
		}
	}
}

// ReleaseBudget is the one financial write (§10), idempotent on car_id at the ERP.
func (c *Client) ReleaseBudget(ctx context.Context, req *connectorv1.ReleaseBudgetRequest) (*connectorv1.ReleaseBudgetResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.erpi.ReleaseBudget(ctx, req)
	return resp, wrap(err)
}

func ts(t time.Time) *timestamppb.Timestamp { return timestamppb.New(t) }

func wrap(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.InvalidArgument:
		return fmt.Errorf("%w: %s", ErrRejected, status.Convert(err).Message())
	case codes.NotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, status.Convert(err).Message())
	case codes.FailedPrecondition, codes.Unimplemented:
		return fmt.Errorf("%w: %s", ErrNotSupported, status.Convert(err).Message())
	case codes.Unavailable, codes.DeadlineExceeded:
		return fmt.Errorf("%w: %s", ErrUnavailable, status.Convert(err).Message())
	default:
		return fmt.Errorf("erpi: %w", err)
	}
}
