// Package idpi is pm-core's client for the identity connector contract
// (pm.connector.v1.IdpiService, FSD §04, §10). It is deliberately thin: the
// module owns the IdP conversation; pm-core owns users, sessions, and grants.
// The connector registry (§05 connector_instances) will supply the address
// later; today it is one configured instance (IDPI_OIDC_ADDR).
package idpi

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

// Errors the login handler maps to HTTP statuses.
var (
	ErrUnavailable = errors.New("idpi: unavailable")
	ErrRejected    = errors.New("idpi: rejected") // INVALID_ARGUMENT from the module: bad code, verifier, or token
)

// Client wraps one connection.
type Client struct {
	conn      *grpc.ClientConn
	idpi      connectorv1.IdpiServiceClient
	connector connectorv1.ConnectorServiceClient
	Timeout   time.Duration
}

// Dial connects (lazily; gRPC connects on first use) to a module address.
func Dial(addr string) (*Client, error) {
	if addr == "" {
		return nil, errors.New("idpi: address required")
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("idpi: dial %s: %w", addr, err)
	}
	return &Client{conn: conn, idpi: connectorv1.NewIdpiServiceClient(conn), connector: connectorv1.NewConnectorServiceClient(conn), Timeout: 10 * time.Second}, nil
}

// Close releases the connection.
func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) ctx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.Timeout)
}

// Health probes the module's common HealthCheck; an error or NOT_SERVING is unavailable.
func (c *Client) Health(ctx context.Context) error {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	h, err := c.connector.HealthCheck(ctx, &connectorv1.HealthCheckRequest{})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if h.GetStatus() == connectorv1.HealthCheckResponse_STATUS_NOT_SERVING {
		return fmt.Errorf("%w: module not serving: %s", ErrUnavailable, h.GetDetail())
	}
	if h.GetStatus() == connectorv1.HealthCheckResponse_STATUS_DEGRADED {
		return fmt.Errorf("%w: degraded: %s", ErrUnavailable, h.GetChecks()["oidc_discovery"])
	}
	return nil
}

// Begin is AuthenticateInteractive.
func (c *Client) Begin(ctx context.Context, redirectURI, nonce string) (*connectorv1.AuthenticateInteractiveResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.idpi.AuthenticateInteractive(ctx, &connectorv1.AuthenticateInteractiveRequest{RedirectUri: redirectURI, Nonce: nonce})
	return resp, wrap(err)
}

// Complete is CompleteAuthentication.
func (c *Client) Complete(ctx context.Context, code, redirectURI, verifier, nonce string) (*connectorv1.CompleteAuthenticationResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.idpi.CompleteAuthentication(ctx, &connectorv1.CompleteAuthenticationRequest{Code: code, RedirectUri: redirectURI, CodeVerifier: verifier, Nonce: nonce})
	return resp, wrap(err)
}

// Resolve is ResolveClaimsToRoles.
func (c *Client) Resolve(ctx context.Context, claims *connectorv1.Claims) (*connectorv1.ResolveClaimsToRolesResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.idpi.ResolveClaimsToRoles(ctx, &connectorv1.ResolveClaimsToRolesRequest{Claims: claims})
	return resp, wrap(err)
}

// Refresh is RefreshToken.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*connectorv1.RefreshTokenResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.idpi.RefreshToken(ctx, &connectorv1.RefreshTokenRequest{RefreshToken: refreshToken})
	return resp, wrap(err)
}

// Logout is Logout.
func (c *Client) Logout(ctx context.Context, refreshToken, idToken, postLogoutRedirect string) (*connectorv1.LogoutResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.idpi.Logout(ctx, &connectorv1.LogoutRequest{RefreshToken: refreshToken, IdTokenHint: idToken, PostLogoutRedirectUri: postLogoutRedirect})
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
		return fmt.Errorf("idpi: %w", err)
	}
}
