//go:build integration

// Live test against the dev Keycloak realm (deploy/compose/keycloak):
//
//	IDPI_TEST_ISSUER=http://127.0.0.1:8180/realms/northfield go test -tags integration ./tests/integration/
//
// It obtains tokens with the password grant through the dev-only pm-dev-cli
// client (the browser leg of the code flow cannot run headless here), then
// drives the in-process service: ValidateToken on the access and id tokens,
// ResolveClaimsToRoles on the real claims, RefreshToken, Logout with
// revocation, and the error paths (bad code, bad refresh token).
package integration

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/reh3376/pro-manage/connectors/idpi/oidc/internal/mapping"
	"github.com/reh3376/pro-manage/connectors/idpi/oidc/internal/oidc"
	"github.com/reh3376/pro-manage/connectors/idpi/oidc/internal/service"
	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

type tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
}

func passwordGrant(t *testing.T, issuer, user, pass string) tokens {
	t.Helper()
	form := url.Values{"grant_type": {"password"}, "client_id": {"pm-dev-cli"}, "username": {user}, "password": {pass}, "scope": {"openid"}}
	resp, err := http.PostForm(issuer+"/protocol/openid-connect/token", form)
	if err != nil {
		t.Fatalf("token endpoint: %v", err)
	}
	defer resp.Body.Close()
	var tk tokens
	if err := json.NewDecoder(resp.Body).Decode(&tk); err != nil || resp.StatusCode != 200 || tk.AccessToken == "" {
		t.Fatalf("password grant for %s: http %d (%v)", user, resp.StatusCode, err)
	}
	return tk
}

func setup(t *testing.T) (*service.Service, string) {
	t.Helper()
	issuer := os.Getenv("IDPI_TEST_ISSUER")
	if issuer == "" {
		t.Skip("IDPI_TEST_ISSUER not set (e.g. http://127.0.0.1:8180/realms/northfield)")
	}
	// Keycloak advertises KC_HOSTNAME (localhost) as its issuer; the test reaches it on 127.0.0.1.
	disc := issuer + "/.well-known/openid-configuration"
	resp, err := http.Get(disc)
	if err != nil {
		t.Skipf("keycloak not reachable at %s: %v", issuer, err)
	}
	var d struct {
		Issuer string `json:"issuer"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&d)
	resp.Body.Close()
	p, err := oidc.New(oidc.Config{Issuer: d.Issuer, DiscoveryURL: disc, BackchannelBase: strings.TrimSuffix(issuer, "/realms/northfield"), ClientID: "pm-dev-cli", Scopes: []string{"openid"}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := mapping.Load(filepath.Join("..", "..", "mapping.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return &service.Service{Provider: p, Mapping: m, Manifest: service.DefaultManifest, Log: slog.New(slog.NewTextHandler(os.Stderr, nil))}, issuer
}

func TestKeycloakTokensRolesRefreshLogout(t *testing.T) {
	s, issuer := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pass := envOr("IDPI_TEST_PASSWORD", "northfield-dev")

	h, err := s.HealthCheck(ctx, &connectorv1.HealthCheckRequest{})
	if err != nil || h.GetStatus() != connectorv1.HealthCheckResponse_STATUS_SERVING {
		t.Fatalf("health: %+v %v", h, err)
	}

	// bob.finance: employee + finance approver group.
	bob := passwordGrant(t, issuer, "bob.finance", pass)
	v, err := s.ValidateToken(ctx, &connectorv1.ValidateTokenRequest{Token: bob.AccessToken})
	if err != nil || !v.GetValid() {
		t.Fatalf("validate access token: %+v %v", v, err)
	}
	if v.GetClaims().GetPreferredUsername() != "bob.finance" || v.GetClaims().GetEmail() == "" || v.GetExpiresAt() == nil {
		t.Fatalf("claims: %+v", v.GetClaims())
	}
	if !contains(v.GetClaims().GetGroups(), "pro_manage.finance_approvers") && !contains(v.GetClaims().GetGroups(), "/pro_manage.finance_approvers") {
		t.Fatalf("groups claim missing: %v", v.GetClaims().GetGroups())
	}
	idv, err := s.ValidateToken(ctx, &connectorv1.ValidateTokenRequest{Token: bob.IDToken})
	if err != nil || !idv.GetValid() || idv.GetExternalId() != v.GetExternalId() {
		t.Fatalf("validate id token: %+v %v", idv, err)
	}
	r, err := s.ResolveClaimsToRoles(ctx, &connectorv1.ResolveClaimsToRolesRequest{Claims: v.GetClaims()})
	if err != nil || !contains(r.GetPlatformRoles(), "finance_approver_eligible") || !contains(r.GetPools(), "finance_approver_eligible") || r.GetAttributes()["employee"] != "true" {
		t.Fatalf("bob roles: %+v %v", r, err)
	}

	// carol.admin resolves to platform_admin; fay.ops to the KY operations pool.
	carol := passwordGrant(t, issuer, "carol.admin", pass)
	cv, _ := s.ValidateToken(ctx, &connectorv1.ValidateTokenRequest{Token: carol.AccessToken})
	cr, _ := s.ResolveClaimsToRoles(ctx, &connectorv1.ResolveClaimsToRolesRequest{Claims: cv.GetClaims()})
	if !contains(cr.GetPlatformRoles(), "platform_admin") {
		t.Fatalf("carol roles: %+v (claims %+v)", cr, cv.GetClaims())
	}
	fay := passwordGrant(t, issuer, "fay.ops", pass)
	fv, _ := s.ValidateToken(ctx, &connectorv1.ValidateTokenRequest{Token: fay.AccessToken})
	fr, _ := s.ResolveClaimsToRoles(ctx, &connectorv1.ResolveClaimsToRolesRequest{Claims: fv.GetClaims()})
	if !contains(fr.GetPools(), "site_operations_lead") || fr.GetAttributes()["site"] != "NF-KY" {
		t.Fatalf("fay pools: %+v (claims %+v)", fr, fv.GetClaims())
	}

	// Refresh rotates the access token; the old refresh token still works until logout revokes it.
	rt, err := s.RefreshToken(ctx, &connectorv1.RefreshTokenRequest{RefreshToken: bob.RefreshToken})
	if err != nil || rt.GetAccessToken() == "" || rt.GetRefreshToken() == "" || rt.GetExpiresAt() == nil {
		t.Fatalf("refresh: %+v %v", rt, err)
	}
	if nv, err := s.ValidateToken(ctx, &connectorv1.ValidateTokenRequest{Token: rt.GetAccessToken()}); err != nil || !nv.GetValid() {
		t.Fatalf("refreshed token must validate: %+v %v", nv, err)
	}
	l, err := s.Logout(ctx, &connectorv1.LogoutRequest{RefreshToken: rt.GetRefreshToken(), IdTokenHint: bob.IDToken, PostLogoutRedirectUri: "http://localhost:3100/"})
	if err != nil || !l.GetRevoked() || !strings.Contains(l.GetEndSessionUrl(), "/protocol/openid-connect/logout?") {
		t.Fatalf("logout: %+v %v", l, err)
	}
	if _, err := s.RefreshToken(ctx, &connectorv1.RefreshTokenRequest{RefreshToken: rt.GetRefreshToken()}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("revoked refresh token must be rejected as invalid argument, got %v", err)
	}

	// Error paths.
	if _, err := s.CompleteAuthentication(ctx, &connectorv1.CompleteAuthenticationRequest{Code: "bogus", RedirectUri: "http://localhost:8080/auth/callback", CodeVerifier: strings.Repeat("v", 43)}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bogus code: %v", err)
	}
	if bad, err := s.ValidateToken(ctx, &connectorv1.ValidateTokenRequest{Token: bob.AccessToken[:len(bob.AccessToken)-4] + "AAAA"}); err != nil || bad.GetValid() {
		t.Fatalf("tampered token must be invalid: %+v %v", bad, err)
	}
	a, err := s.AuthenticateInteractive(ctx, &connectorv1.AuthenticateInteractiveRequest{RedirectUri: "http://localhost:8080/auth/callback", State: "it", Nonce: "n"})
	if err != nil || !strings.Contains(a.GetAuthorizationUrl(), "/protocol/openid-connect/auth?") || !strings.Contains(a.GetAuthorizationUrl(), "code_challenge_method=S256") {
		t.Fatalf("auth url: %+v %v", a, err)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
