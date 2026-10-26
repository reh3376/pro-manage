// Package sigi is pm-core's client for the e-signature connector contract
// (pm.connector.v1.SigiService, FSD §10). Thin, like the IdPi client: the
// module talks to the provider; pm-core keeps the envelope record and the
// artifact. One configured instance today (SIGI_ADDR); the registry later.
package sigi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

// Errors the callers branch on.
var (
	ErrUnavailable = errors.New("sigi: unavailable")
	ErrRejected    = errors.New("sigi: rejected")  // INVALID_ARGUMENT: a bad request, never retried
	ErrNotFound    = errors.New("sigi: not found") // unknown envelope
	ErrNotReady    = errors.New("sigi: not ready") // FAILED_PRECONDITION: artifact asked before completion
)

// Client wraps one connection.
type Client struct {
	conn      *grpc.ClientConn
	sigi      connectorv1.SigiServiceClient
	connector connectorv1.ConnectorServiceClient
	Provider  string // what the module is (stub | docusign | ...), for the envelope record
	Timeout   time.Duration
}

// Dial connects lazily.
func Dial(addr, provider string) (*Client, error) {
	if addr == "" {
		return nil, errors.New("sigi: address required")
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("sigi: dial %s: %w", addr, err)
	}
	if provider == "" {
		provider = "unknown"
	}
	return &Client{conn: conn, sigi: connectorv1.NewSigiServiceClient(conn), connector: connectorv1.NewConnectorServiceClient(conn), Provider: provider, Timeout: 15 * time.Second}, nil
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

// Create is CreateEnvelope.
func (c *Client) Create(ctx context.Context, req *connectorv1.CreateEnvelopeRequest) (*connectorv1.CreateEnvelopeResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.sigi.CreateEnvelope(ctx, req)
	return resp, wrap(err)
}

// Status is GetEnvelopeStatus.
func (c *Client) Status(ctx context.Context, providerRef string) (*connectorv1.GetEnvelopeStatusResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.sigi.GetEnvelopeStatus(ctx, &connectorv1.GetEnvelopeStatusRequest{EnvelopeId: providerRef})
	return resp, wrap(err)
}

// Cancel is CancelEnvelope.
func (c *Client) Cancel(ctx context.Context, providerRef, reason string) (*connectorv1.CancelEnvelopeResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.sigi.CancelEnvelope(ctx, &connectorv1.CancelEnvelopeRequest{EnvelopeId: providerRef, Reason: reason})
	return resp, wrap(err)
}

// Download streams the signed artifact into memory (artifacts are PDFs of a few hundred KB at most).
func (c *Client) Download(ctx context.Context, providerRef string) (data []byte, mediaType string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 2*c.Timeout)
	defer cancel()
	stream, err := c.sigi.DownloadSignedArtifact(ctx, &connectorv1.DownloadSignedArtifactRequest{EnvelopeId: providerRef})
	if err != nil {
		return nil, "", wrap(err)
	}
	var buf bytes.Buffer
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", wrap(err)
		}
		if msg.GetMediaType() != "" {
			mediaType = msg.GetMediaType()
		}
		buf.Write(msg.GetChunk())
		if buf.Len() > 64<<20 {
			return nil, "", fmt.Errorf("%w: artifact exceeds 64 MiB", ErrRejected)
		}
	}
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return buf.Bytes(), mediaType, nil
}

// Webhook forwards a provider payload for verification.
func (c *Client) Webhook(ctx context.Context, provider string, body []byte, headers map[string]string) (*connectorv1.OnWebhookResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.sigi.OnWebhook(ctx, &connectorv1.OnWebhookRequest{Provider: provider, Body: body, Headers: headers})
	return resp, wrap(err)
}

func wrap(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.InvalidArgument:
		return fmt.Errorf("%w: %s", ErrRejected, status.Convert(err).Message())
	case codes.NotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, status.Convert(err).Message())
	case codes.FailedPrecondition:
		return fmt.Errorf("%w: %s", ErrNotReady, status.Convert(err).Message())
	case codes.Unavailable, codes.DeadlineExceeded:
		return fmt.Errorf("%w: %s", ErrUnavailable, status.Convert(err).Message())
	default:
		return fmt.Errorf("sigi: %w", err)
	}
}
