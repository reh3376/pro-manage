package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeIdP serves a discovery document that advertises a browser-facing issuer
// the test rewrites to the httptest origin for backchannel calls.
func fakeIdP(t *testing.T, issuer string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/x/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": issuer, "authorization_endpoint": issuer + "/auth", "token_endpoint": issuer + "/token",
			"jwks_uri": issuer + "/certs", "revocation_endpoint": issuer + "/revoke", "end_session_endpoint": issuer + "/logout",
		})
	})
	mux.HandleFunc("/realms/x/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if r.Form.Get("grant_type") == "refresh_token" && r.Form.Get("refresh_token") == "good" {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at2", "refresh_token": "rt2", "token_type": "bearer", "expires_in": 300})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_grant", "error_description": "nope"})
	})
	mux.HandleFunc("/realms/x/revoke", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("token") == "good" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	return httptest.NewServer(mux)
}

func TestDiscoveryBackchannelRewriteAndAuthURL(t *testing.T) {
	const issuer = "http://idp.example/realms/x"
	srv := fakeIdP(t, issuer)
	defer srv.Close()
	p, err := New(Config{Issuer: issuer, DiscoveryURL: srv.URL + "/realms/x/.well-known/openid-configuration", BackchannelBase: srv.URL, ClientID: "pm-core", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.TokenEndpoint != srv.URL+"/realms/x/token" || d.JWKSURI != srv.URL+"/realms/x/certs" || d.RevocationEndpoint != srv.URL+"/realms/x/revoke" {
		t.Fatalf("backchannel endpoints not rewritten: %+v", d)
	}
	if d.AuthorizationEndpoint != issuer+"/auth" || d.EndSessionEndpoint != issuer+"/logout" {
		t.Fatalf("browser endpoints must keep the advertised origin: %+v", d)
	}

	authURL, state, verifier, err := p.AuthURL(context.Background(), "http://localhost:8080/auth/callback", "nonce-1", []string{"groups"})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authURL)
	q := u.Query()
	if !strings.HasPrefix(authURL, issuer+"/auth?") || q.Get("response_type") != "code" || q.Get("client_id") != "pm-core" || q.Get("state") != state || len(state) != 43 || q.Get("nonce") != "nonce-1" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || len(verifier) < 43 {
		t.Fatalf("auth url: %s (state %q, verifier %d chars)", authURL, state, len(verifier))
	}
	if _, again, _, _ := p.AuthURL(context.Background(), "http://localhost:8080/auth/callback", "", nil); again == state {
		t.Fatal("state must be fresh per call")
	}
	if q.Get("scope") != "openid profile email groups" || q.Get("redirect_uri") != "http://localhost:8080/auth/callback" {
		t.Fatalf("scope/redirect: %s", authURL)
	}
	if _, _, _, err := p.AuthURL(context.Background(), "", "", nil); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("missing redirect must be invalid grant, got %v", err)
	}

	// Refresh and revoke against the fake token endpoint.
	tok, err := p.Refresh(context.Background(), "good")
	if err != nil || tok.AccessToken != "at2" || tok.RefreshToken != "rt2" || tok.ExpiresAt.IsZero() {
		t.Fatalf("refresh: %+v %v", tok, err)
	}
	if _, err := p.Refresh(context.Background(), "bad"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad refresh: %v", err)
	}
	if _, err := p.Refresh(context.Background(), ""); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("empty refresh: %v", err)
	}
	revoked, end, err := p.Logout(context.Background(), "good", "idt", "http://localhost:3100/")
	if err != nil || !revoked || !strings.HasPrefix(end, issuer+"/logout?") || !strings.Contains(end, "id_token_hint=idt") || !strings.Contains(end, "post_logout_redirect_uri=http%3A%2F%2Flocalhost%3A3100%2F") {
		t.Fatalf("logout: %v %q %v", revoked, end, err)
	}
	if revoked, _, err := p.Logout(context.Background(), "unknown", "", ""); err != nil || revoked {
		t.Fatalf("unknown token revoke must be a quiet no: %v %v", revoked, err)
	}
	if _, err := p.Verify(context.Background(), "not.a.jwt"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("garbage token: %v", err)
	}
	if _, err := p.Verify(context.Background(), ""); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("empty token: %v", err)
	}
}

func TestDiscoveryIssuerMismatchAndUnreachable(t *testing.T) {
	srv := fakeIdP(t, "http://other.example/realms/x")
	defer srv.Close()
	p, _ := New(Config{Issuer: "http://idp.example/realms/x", DiscoveryURL: srv.URL + "/realms/x/.well-known/openid-configuration", ClientID: "c", HTTPClient: srv.Client()})
	if _, err := p.Discover(context.Background()); !errors.Is(err, ErrNotReady) {
		t.Fatalf("issuer mismatch: %v", err)
	}
	down, _ := New(Config{Issuer: "http://127.0.0.1:1/realms/x", ClientID: "c"})
	if _, err := down.Discover(context.Background()); !errors.Is(err, ErrNotReady) {
		t.Fatalf("unreachable: %v", err)
	}
	if _, _, _, err := down.AuthURL(context.Background(), "http://x/cb", "", nil); !errors.Is(err, ErrNotReady) {
		t.Fatalf("auth url without discovery: %v", err)
	}
	if _, err := New(Config{Issuer: "nope", ClientID: "c"}); err == nil {
		t.Fatal("bad issuer must fail")
	}
	if _, err := New(Config{Issuer: "http://x/y"}); err == nil {
		t.Fatal("missing client id must fail")
	}
}

func TestExtractClaims(t *testing.T) {
	p, _ := New(Config{Issuer: "http://idp.example/realms/x", ClientID: "c"})
	all := map[string]any{
		"sub": "u1", "email": "a@b", "preferred_username": "alice", "given_name": "Alice", "family_name": "Okafor",
		"groups": []any{"/pro_manage.employees", 7}, "realm_access": map[string]any{"roles": []any{"site_operations_lead"}},
		"department": "Finance", "exp": 1.0, "iss": "x",
	}
	c := p.extract(all)
	if c.Email != "a@b" || c.PreferredUsername != "alice" || c.DisplayName != "Alice Okafor" || len(c.Groups) != 1 || c.Roles[0] != "site_operations_lead" {
		t.Fatalf("%+v", c)
	}
	if c.Attributes["department"] != "Finance" || c.Attributes["iss"] != "" || c.Attributes["email"] != "" {
		t.Fatalf("attributes: %+v", c.Attributes)
	}
	if !p.audienceOK([]string{"account"}, "c") || p.audienceOK([]string{"account"}, "other") || !p.audienceOK([]string{"c"}, "") {
		t.Fatal("audience rules")
	}
}
