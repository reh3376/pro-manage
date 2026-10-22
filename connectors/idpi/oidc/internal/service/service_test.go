package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/reh3376/pro-manage/connectors/idpi/oidc/internal/mapping"
	"github.com/reh3376/pro-manage/connectors/idpi/oidc/internal/oidc"
	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

func newService(t *testing.T, issuer, discoveryURL string, hc *http.Client) *Service {
	t.Helper()
	p, err := oidc.New(oidc.Config{Issuer: issuer, DiscoveryURL: discoveryURL, ClientID: "pm-core", HTTPClient: hc})
	if err != nil {
		t.Fatal(err)
	}
	m, err := mapping.Load(filepath.Join("..", "..", "mapping.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return &Service{Provider: p, Mapping: m, Manifest: DefaultManifest, Log: slog.New(slog.NewTextHandler(os.Stderr, nil))}
}

func TestWithIdPDown(t *testing.T) {
	s := newService(t, "http://127.0.0.1:1/realms/x", "", nil)
	ctx := context.Background()
	h, err := s.HealthCheck(ctx, &connectorv1.HealthCheckRequest{})
	if err != nil || h.GetStatus() != connectorv1.HealthCheckResponse_STATUS_DEGRADED || h.GetChecks()["oidc_discovery"] == "" {
		t.Fatalf("health with idp down: %+v %v", h, err)
	}
	c, err := s.Capabilities(ctx, &connectorv1.CapabilitiesRequest{})
	if err != nil || c.GetModule() != "IdPi_OIDC" || c.GetType() != "IdPi" || len(c.GetCapabilities()) != 6 || c.GetContract() != "pm.connector.v1.IdpiService" {
		t.Fatalf("capabilities: %+v %v", c, err)
	}
	if _, err := s.AuthenticateInteractive(ctx, &connectorv1.AuthenticateInteractiveRequest{RedirectUri: "http://x/cb", State: "s"}); status.Code(err) != codes.Unavailable {
		t.Fatalf("auth with idp down: %v", err)
	}
	if _, err := s.AuthenticateInteractive(ctx, &connectorv1.AuthenticateInteractiveRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("auth without redirect: %v", err)
	}
	if _, err := s.RefreshToken(ctx, &connectorv1.RefreshTokenRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("refresh without token: %v", err)
	}
	if _, err := s.CompleteAuthentication(ctx, &connectorv1.CompleteAuthenticationRequest{Code: "c"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("complete without verifier: %v", err)
	}
	// ResolveClaimsToRoles is pure and works without the IdP.
	r, err := s.ResolveClaimsToRoles(ctx, &connectorv1.ResolveClaimsToRolesRequest{Claims: &connectorv1.Claims{Groups: []string{"/pro_manage.employees", "/pro_manage.finance_approvers", "/site.NF-KY.operations_leads"}}})
	if err != nil || strings.Join(r.GetPlatformRoles(), ",") != "finance_approver_eligible" || strings.Join(r.GetPools(), ",") != "finance_approver_eligible,site_operations_lead" || r.GetAttributes()["site"] != "NF-KY" {
		t.Fatalf("resolve: %+v %v", r, err)
	}
	if _, err := s.ResolveClaimsToRoles(ctx, &connectorv1.ResolveClaimsToRolesRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("resolve without claims: %v", err)
	}
}

func TestWithFakeIdP(t *testing.T) {
	const issuer = "http://idp.example/realms/x"
	mux := http.NewServeMux()
	mux.HandleFunc("/disc", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/auth", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/certs", "end_session_endpoint": issuer + "/logout"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	s := newService(t, issuer, srv.URL+"/disc", srv.Client())
	ctx := context.Background()
	h, _ := s.HealthCheck(ctx, &connectorv1.HealthCheckRequest{})
	if h.GetStatus() != connectorv1.HealthCheckResponse_STATUS_SERVING {
		t.Fatalf("health: %+v", h)
	}
	a, err := s.AuthenticateInteractive(ctx, &connectorv1.AuthenticateInteractiveRequest{RedirectUri: "http://localhost:8080/auth/callback", State: "login-1", Nonce: "n"})
	if err != nil || !strings.HasPrefix(a.GetAuthorizationUrl(), issuer+"/auth?") || a.GetCodeVerifier() == "" || a.GetState() != "login-1" {
		t.Fatalf("auth: %+v %v", a, err)
	}
	v, err := s.ValidateToken(ctx, &connectorv1.ValidateTokenRequest{Token: "garbage"})
	if err != nil || v.GetValid() || v.GetError() == "" {
		t.Fatalf("validate garbage must be OK with valid=false: %+v %v", v, err)
	}
	l, err := s.Logout(ctx, &connectorv1.LogoutRequest{IdTokenHint: "idt", PostLogoutRedirectUri: "http://localhost:3100/"})
	if err != nil || l.GetRevoked() || !strings.HasPrefix(l.GetEndSessionUrl(), issuer+"/logout?") {
		t.Fatalf("logout: %+v %v", l, err)
	}
}
