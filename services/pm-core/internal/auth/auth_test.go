package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/idpi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/domain"
	"github.com/reh3376/pro-manage/services/pm-core/internal/sessions"
)

// fakeIdPi plays the module: one known code, fixed claims and roles.
type fakeIdPi struct {
	down      bool
	logouts   int
	lastState string
}

func (f *fakeIdPi) Begin(_ context.Context, redirectURI, nonce string) (*connectorv1.AuthenticateInteractiveResponse, error) {
	if f.down {
		return nil, idpi.ErrUnavailable
	}
	f.lastState = "st-" + nonce[:6]
	return &connectorv1.AuthenticateInteractiveResponse{AuthorizationUrl: "http://idp.example/auth?state=" + f.lastState + "&redirect_uri=" + url.QueryEscape(redirectURI), State: f.lastState, CodeVerifier: "verifier-" + nonce[:4]}, nil
}

func (f *fakeIdPi) Complete(_ context.Context, code, redirectURI, verifier, nonce string) (*connectorv1.CompleteAuthenticationResponse, error) {
	if code != "good-code" || !strings.HasPrefix(verifier, "verifier-") || redirectURI == "" || nonce == "" {
		return nil, idpi.ErrRejected
	}
	return &connectorv1.CompleteAuthenticationResponse{
		ExternalId: "sub-bob", Claims: &connectorv1.Claims{Subject: "sub-bob", Email: "bob@northfield.example", DisplayName: "Bob Lindqvist", PreferredUsername: "bob.finance", Groups: []string{"pro_manage.finance_approvers"}},
		AccessToken: "at", RefreshToken: "rt", IdToken: "idt",
	}, nil
}

func (f *fakeIdPi) Resolve(_ context.Context, c *connectorv1.Claims) (*connectorv1.ResolveClaimsToRolesResponse, error) {
	return &connectorv1.ResolveClaimsToRolesResponse{PlatformRoles: []string{"finance_approver_eligible"}, Pools: []string{"finance_approver_eligible"}, Attributes: map[string]string{"employee": "true"}}, nil
}

func (f *fakeIdPi) Logout(_ context.Context, refreshToken, idToken, post string) (*connectorv1.LogoutResponse, error) {
	f.logouts++
	return &connectorv1.LogoutResponse{Revoked: refreshToken == "rt", EndSessionUrl: "http://idp.example/logout?post_logout_redirect_uri=" + url.QueryEscape(post)}, nil
}

type fakeUsers struct {
	logins  []domain.LoginInput
	logouts []string
}

func (u *fakeUsers) RecordLogin(_ context.Context, in domain.LoginInput) (db.User, error) {
	u.logins = append(u.logins, in)
	return db.User{UserID: uuid.New(), Username: in.Username, ExternalID: in.Subject, Email: in.Email, DisplayName: in.DisplayName, PlatformRoles: in.PlatformRoles}, nil
}

func (u *fakeUsers) RecordLogout(_ context.Context, actor domain.Actor, sessionID string) error {
	u.logouts = append(u.logouts, actor.UserID+":"+sessionID)
	return nil
}

func newHandler(t *testing.T) (*Handler, *fakeIdPi, *fakeUsers, *httptest.Server) {
	t.Helper()
	mr := miniredis.RunT(t)
	store := sessions.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	f, u := &fakeIdPi{}, &fakeUsers{}
	h := &Handler{Log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), Sessions: store, IdPi: f, Users: u, PublicURL: "http://pm.example", FrontendURL: "http://app.example"}
	mux := http.NewServeMux()
	h.Mount(mux, func(n http.Handler) http.Handler { return n })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return h, f, u, srv
}

func noRedirect() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func TestLoginCallbackMeLogout(t *testing.T) {
	h, f, users, srv := newHandler(t)
	c := noRedirect()

	// login: 302 to the IdP with the module's state; a pending record waits for the callback.
	resp, err := c.Get(srv.URL + "/auth/login?return_to=/projects")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode != 302 || !strings.HasPrefix(loc, "http://idp.example/auth?state="+f.lastState) || !strings.Contains(loc, url.QueryEscape("http://pm.example/auth/callback")) {
		t.Fatalf("login: %d %s", resp.StatusCode, loc)
	}

	// callback with an unknown state is refused; with a bad code, rejected; the good one signs in.
	r, _ := c.Get(srv.URL + "/auth/callback?state=nope&code=good-code")
	if r.StatusCode != 400 || !strings.Contains(body(r), "login_state_unknown") {
		t.Fatalf("unknown state: %d", r.StatusCode)
	}
	r, _ = c.Get(srv.URL + "/auth/callback?state=" + f.lastState + "&code=bad")
	if r.StatusCode != 400 || !strings.Contains(body(r), "login_rejected") {
		t.Fatalf("bad code: %d", r.StatusCode)
	}
	// The state was consumed by the failed attempt: start again.
	resp, _ = c.Get(srv.URL + "/auth/login?return_to=/projects")
	resp.Body.Close()
	r, _ = c.Get(srv.URL + "/auth/callback?state=" + f.lastState + "&code=good-code")
	if r.StatusCode != 302 || r.Header.Get("Location") != "/projects" {
		t.Fatalf("callback: %d %s %s", r.StatusCode, r.Header.Get("Location"), body(r))
	}
	var cookie *http.Cookie
	for _, ck := range r.Cookies() {
		if ck.Name == CookieName {
			cookie = ck
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || len(cookie.Value) != 43 {
		t.Fatalf("cookie: %+v", cookie)
	}
	if len(users.logins) != 1 || users.logins[0].Username != "bob.finance" || users.logins[0].Pools[0] != "finance_approver_eligible" || users.logins[0].Subject != "sub-bob" {
		t.Fatalf("record login: %+v", users.logins)
	}

	// me: 401 without the cookie, the user with it; the actor resolves for /api/v1.
	r, _ = c.Get(srv.URL + "/auth/me")
	if r.StatusCode != 401 {
		t.Fatalf("me without cookie: %d", r.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/auth/me", nil)
	req.AddCookie(cookie)
	r, _ = c.Do(req)
	var me struct {
		User          map[string]any `json:"user"`
		PlatformRoles []string       `json:"platform_roles"`
		Pools         []string       `json:"pools"`
	}
	_ = json.Unmarshal([]byte(body(r)), &me)
	if r.StatusCode != 200 || me.User["username"] != "bob.finance" || me.PlatformRoles[0] != "finance_approver_eligible" || me.Pools[0] != "finance_approver_eligible" {
		t.Fatalf("me: %d %+v", r.StatusCode, me)
	}
	req.AddCookie(cookie)
	if actor, ok := h.Actor(req); !ok || actor.UserID != "bob.finance" || actor.Subject != "sub-bob" || !actor.HasRole("finance_approver_eligible") || actor.SessionID != cookie.Value {
		t.Fatalf("actor: %+v %v", actor, ok)
	}

	// logout: cascades to the IdP, clears the cookie, ends the session.
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/auth/logout", nil)
	req.AddCookie(cookie)
	r, _ = c.Do(req)
	if r.StatusCode != 200 || !strings.Contains(body(r), "http://idp.example/logout") || f.logouts != 1 || len(users.logouts) != 1 {
		t.Fatalf("logout: %d logouts=%d", r.StatusCode, f.logouts)
	}
	cleared := false
	for _, ck := range r.Cookies() {
		if ck.Name == CookieName && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("cookie not cleared")
	}
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/auth/me", nil)
	req.AddCookie(cookie)
	if r, _ = c.Do(req); r.StatusCode != 401 {
		t.Fatalf("me after logout: %d", r.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/auth/logout", nil)
	if r, _ = c.Do(req); r.StatusCode != 401 {
		t.Fatalf("logout without session: %d", r.StatusCode)
	}
}

func TestLoginWhenIdPDownAndIdPErrorCallback(t *testing.T) {
	_, f, _, srv := newHandler(t)
	c := noRedirect()
	f.down = true
	r, _ := c.Get(srv.URL + "/auth/login")
	if r.StatusCode != 503 || !strings.Contains(body(r), "idp_unavailable") {
		t.Fatalf("login with idp down: %d", r.StatusCode)
	}
	r, _ = c.Get(srv.URL + "/auth/callback?error=access_denied&error_description=user+cancelled")
	if r.StatusCode != 400 || !strings.Contains(body(r), "idp_error") {
		t.Fatalf("idp error callback: %d", r.StatusCode)
	}
}

func TestSafeReturnTo(t *testing.T) {
	h := &Handler{PublicURL: "http://pm.example", FrontendURL: "http://app.example"}
	cases := map[string]string{
		"":                             "http://app.example/",
		"/projects/1":                  "/projects/1",
		"//evil.example/x":             "http://app.example/",
		"http://app.example/cars":      "http://app.example/cars",
		"http://pm.example/auth/me":    "http://pm.example/auth/me",
		"https://evil.example/phish":   "http://app.example/",
		"javascript:alert(1)":          "http://app.example/",
		"http://app.example.evil.io/x": "http://app.example/",
	}
	for in, want := range cases {
		if got := h.safeReturnTo(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func body(r *http.Response) string {
	defer r.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}

var _ = errors.New
