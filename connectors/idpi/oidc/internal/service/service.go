// Package service implements pm.connector.v1.ConnectorService and
// pm.connector.v1.IdpiService over the oidc provider and the claims mapping.
// Errors map to gRPC codes the UDTS specs assert: INVALID_ARGUMENT for a bad
// grant or missing input, UNAVAILABLE when the IdP cannot be reached, OK with
// valid=false for a token that does not verify.
package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/reh3376/pro-manage/connectors/idpi/oidc/internal/mapping"
	"github.com/reh3376/pro-manage/connectors/idpi/oidc/internal/oidc"
	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

// Manifest is what Capabilities reports (from connector.yaml, baked in at build).
type Manifest struct {
	Module       string
	Type         string
	Code         string
	Version      string
	Contract     string
	Capabilities []string
}

// DefaultManifest mirrors connectors/idpi/oidc/connector.yaml.
var DefaultManifest = Manifest{
	Module: "IdPi_OIDC", Type: "IdPi", Code: "OIDC", Contract: "pm.connector.v1.IdpiService",
	Capabilities: []string{"interactive_login", "pkce", "token_validation", "token_refresh", "claims_to_roles", "logout"},
}

// Service holds the dependencies.
type Service struct {
	connectorv1.UnimplementedConnectorServiceServer
	connectorv1.UnimplementedIdpiServiceServer

	Provider *oidc.Provider
	Mapping  *mapping.Mapping
	Manifest Manifest
	Log      *slog.Logger
}

// ---------------------------------------------------------------- ConnectorService

// HealthCheck probes discovery: SERVING when the IdP answers, DEGRADED when it
// does not (the module is up; logins will fail until the IdP returns).
func (s *Service) HealthCheck(ctx context.Context, _ *connectorv1.HealthCheckRequest) (*connectorv1.HealthCheckResponse, error) {
	resp := &connectorv1.HealthCheckResponse{Status: connectorv1.HealthCheckResponse_STATUS_SERVING, Checks: map[string]string{}}
	if _, err := s.Provider.Discover(ctx); err != nil {
		resp.Status = connectorv1.HealthCheckResponse_STATUS_DEGRADED
		resp.Detail = "identity provider discovery failed"
		resp.Checks["oidc_discovery"] = err.Error()
		return resp, nil
	}
	resp.Checks["oidc_discovery"] = "ok"
	resp.Detail = "issuer " + s.Provider.Issuer()
	return resp, nil
}

// Capabilities reports the manifest.
func (s *Service) Capabilities(_ context.Context, _ *connectorv1.CapabilitiesRequest) (*connectorv1.CapabilitiesResponse, error) {
	m := s.Manifest
	return &connectorv1.CapabilitiesResponse{Module: m.Module, Type: m.Type, Code: m.Code, Version: m.Version, Contract: m.Contract, Capabilities: m.Capabilities, Tiers: []string{}}, nil
}

// ---------------------------------------------------------------- IdpiService

// AuthenticateInteractive builds the PKCE redirect; the state and verifier are minted here.
func (s *Service) AuthenticateInteractive(ctx context.Context, req *connectorv1.AuthenticateInteractiveRequest) (*connectorv1.AuthenticateInteractiveResponse, error) {
	u, state, verifier, err := s.Provider.AuthURL(ctx, req.GetRedirectUri(), req.GetNonce(), req.GetScopes())
	if err != nil {
		return nil, grpcErr(err)
	}
	return &connectorv1.AuthenticateInteractiveResponse{AuthorizationUrl: u, State: state, CodeVerifier: verifier}, nil
}

// CompleteAuthentication exchanges the code and returns the verified claims.
func (s *Service) CompleteAuthentication(ctx context.Context, req *connectorv1.CompleteAuthenticationRequest) (*connectorv1.CompleteAuthenticationResponse, error) {
	tok, c, err := s.Provider.Exchange(ctx, req.GetCode(), req.GetRedirectUri(), req.GetCodeVerifier(), req.GetNonce())
	if err != nil {
		return nil, grpcErr(err)
	}
	return &connectorv1.CompleteAuthenticationResponse{
		ExternalId: c.Subject, Claims: toProto(c), AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, IdToken: tok.IDToken, ExpiresAt: ts(tok.ExpiresAt),
	}, nil
}

// ValidateToken never fails the RPC for a bad token: valid=false with the reason.
func (s *Service) ValidateToken(ctx context.Context, req *connectorv1.ValidateTokenRequest) (*connectorv1.ValidateTokenResponse, error) {
	c, err := s.Provider.Verify(ctx, req.GetToken())
	if err != nil {
		if errors.Is(err, oidc.ErrInvalidToken) {
			return &connectorv1.ValidateTokenResponse{Valid: false, Error: err.Error()}, nil
		}
		return nil, grpcErr(err)
	}
	return &connectorv1.ValidateTokenResponse{Valid: true, ExternalId: c.Subject, Claims: toProto(c), ExpiresAt: ts(c.ExpiresAt)}, nil
}

// RefreshToken redeems a refresh token.
func (s *Service) RefreshToken(ctx context.Context, req *connectorv1.RefreshTokenRequest) (*connectorv1.RefreshTokenResponse, error) {
	tok, err := s.Provider.Refresh(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, grpcErr(err)
	}
	return &connectorv1.RefreshTokenResponse{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, IdToken: tok.IDToken, ExpiresAt: ts(tok.ExpiresAt)}, nil
}

// ResolveClaimsToRoles applies the mapping file. Pure; no IdP call.
func (s *Service) ResolveClaimsToRoles(_ context.Context, req *connectorv1.ResolveClaimsToRolesRequest) (*connectorv1.ResolveClaimsToRolesResponse, error) {
	if req.GetClaims() == nil {
		return nil, status.Error(codes.InvalidArgument, "claims are required")
	}
	r := s.Mapping.Resolve(fromProto(req.GetClaims()))
	return &connectorv1.ResolveClaimsToRolesResponse{PlatformRoles: r.PlatformRoles, Pools: r.Pools, Attributes: r.Attributes}, nil
}

// Logout revokes the refresh token and returns the end-session URL.
func (s *Service) Logout(ctx context.Context, req *connectorv1.LogoutRequest) (*connectorv1.LogoutResponse, error) {
	revoked, end, err := s.Provider.Logout(ctx, req.GetRefreshToken(), req.GetIdTokenHint(), req.GetPostLogoutRedirectUri())
	if err != nil {
		return nil, grpcErr(err)
	}
	return &connectorv1.LogoutResponse{Revoked: revoked, EndSessionUrl: end}, nil
}

// ---------------------------------------------------------------- helpers

func grpcErr(err error) error {
	switch {
	case errors.Is(err, oidc.ErrInvalidGrant):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, oidc.ErrInvalidToken):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, oidc.ErrNotReady), errors.Is(err, oidc.ErrUpstream):
		return status.Error(codes.Unavailable, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func ts(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func toProto(c oidc.Claims) *connectorv1.Claims {
	return &connectorv1.Claims{Subject: c.Subject, Email: c.Email, DisplayName: c.DisplayName, PreferredUsername: c.PreferredUsername, Groups: c.Groups, Roles: c.Roles, Attributes: c.Attributes}
}

func fromProto(c *connectorv1.Claims) mapping.Claims {
	return mapping.Claims{Subject: c.GetSubject(), Email: c.GetEmail(), DisplayName: c.GetDisplayName(), PreferredUsername: c.GetPreferredUsername(), Groups: c.GetGroups(), Roles: c.GetRoles(), Attributes: c.GetAttributes()}
}
