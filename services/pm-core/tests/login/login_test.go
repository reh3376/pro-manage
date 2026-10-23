//go:build integration

// The SSO login end to end against the running dev stack (FSD §04): pm-core
// /auth/login -> IdPi_OIDC -> Keycloak's login form (driven over HTTP with a
// cookie jar, as a browser would) -> /auth/callback -> session cookie ->
// /auth/me and an /api/v1 call as that user -> /auth/logout.
//
//	PM_CORE_URL=http://127.0.0.1:8080 go test -tags integration ./tests/login/
//
// pm-core's PM_PUBLIC_URL must be reachable from here (the dev stack publishes
// it on localhost:8080) and Keycloak on the KC_HOSTNAME it advertises
// (localhost:8180).
package login

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

var (
	formAction = regexp.MustCompile(`<form[^>]*id="kc-form-login"[^>]*action="([^"]+)"`)
	htmlAmp    = strings.NewReplacer("&amp;", "&")
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func TestSSOLoginEndToEnd(t *testing.T) {
	core := os.Getenv("PM_CORE_URL")
	if core == "" {
		t.Skip("PM_CORE_URL not set (e.g. http://127.0.0.1:8080)")
	}
	user, pass := envOr("LOGIN_USER", "bob.finance"), envOr("LOGIN_PASSWORD", "northfield-dev")
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	// 1. /auth/login redirects through to Keycloak's login page.
	resp, err := c.Get(core + "/auth/login?return_to=/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Request.URL.Host, "8180") {
		t.Fatalf("expected the Keycloak login page, got %d at %s", resp.StatusCode, resp.Request.URL)
	}
	m := formAction.FindSubmatch(page)
	if m == nil {
		t.Fatalf("no login form in the Keycloak page (%d bytes)", len(page))
	}
	action := htmlAmp.Replace(string(m[1]))

	// 2. Post the credentials; Keycloak redirects to pm-core's callback, which sets the cookie and lands on return_to.
	resp, err = c.PostForm(action, url.Values{"username": {user}, "password": {pass}, "credentialId": {""}})
	if err != nil {
		t.Fatalf("login form: %v", err)
	}
	final, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasSuffix(resp.Request.URL.Path, "/auth/me") {
		t.Fatalf("after login: %d at %s: %.300s", resp.StatusCode, resp.Request.URL, final)
	}
	var me struct {
		User          map[string]any `json:"user"`
		PlatformRoles []string       `json:"platform_roles"`
		Pools         []string       `json:"pools"`
	}
	if err := json.Unmarshal(final, &me); err != nil || me.User["username"] != user {
		t.Fatalf("/auth/me: %s (%v)", final, err)
	}
	if user == "bob.finance" && !contains(me.Pools, "finance_approver_eligible") {
		t.Fatalf("bob should be in the finance pool: %+v", me)
	}
	coreURL, _ := url.Parse(core)
	cookieSeen := false
	for _, ck := range jar.Cookies(coreURL) {
		if ck.Name == "pm_session" {
			cookieSeen = true
		}
	}
	if !cookieSeen {
		t.Fatal("no pm_session cookie in the jar")
	}

	// 3. The session authenticates /api/v1 without the dev header, and the pool rows came from the IdP.
	resp, err = c.Get(core + "/api/v1/inbox")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("/api/v1/inbox as the session user: %d %s", resp.StatusCode, b)
	}

	// 4. Logout cascades to the IdP and ends the session.
	req, _ := http.NewRequest(http.MethodPost, core+"/auth/logout", nil)
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "openid-connect/logout") {
		t.Fatalf("logout: %d %s", resp.StatusCode, b)
	}
	resp, _ = c.Get(core + "/auth/me")
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("/auth/me after logout: %d", resp.StatusCode)
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
