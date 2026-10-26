//go:build integration

package login

import (
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
	projectLink = regexp.MustCompile(`href="/projects/([0-9a-f-]{36})"`)
	carLink     = regexp.MustCompile(`href="/cars/([0-9a-f-]{36})"`)
)

// TestPersonaScreens signs in through the real SSO flow and walks the persona
// screens of the spine (roadmap: project page, CAR form, approval screen with
// the guidance rail, inbox, audit) as the session user. The frontend renders
// them server-side from pm-core with the forwarded pm_session cookie, so a 200
// with the expected headings proves the whole path: cookie -> actor -> ACL ->
// page. PM_FRONTEND_URL names the frontend; the walk continues on the host the
// session cookie was issued for (cookies are host-scoped). With
// SCREENS_EXPECT_CHAIN=1 (CI, after the CAR check has run) at least one CAR
// page must show an approval chain and an audit trail.
//
//	PM_CORE_URL=http://127.0.0.1:8080 PM_FRONTEND_URL=http://127.0.0.1:3100 LOGIN_USER=alice.owner go test -tags integration -run PersonaScreens ./tests/login/
func TestPersonaScreens(t *testing.T) {
	core, fe := os.Getenv("PM_CORE_URL"), os.Getenv("PM_FRONTEND_URL")
	if core == "" || fe == "" {
		t.Skip("PM_CORE_URL and PM_FRONTEND_URL not set")
	}
	user, pass := envOr("LOGIN_USER", "alice.owner"), envOr("LOGIN_PASSWORD", "northfield-dev")
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	// Signed out: the home page renders with the sign-in link, and a persona screen bounces to
	// pm-core's sign-in with the page as return_to.
	anon := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Timeout: 30 * time.Second}
	resp, err := anon.Get(fe + "/")
	if err != nil {
		t.Fatal(err)
	}
	home, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(home), "Sign in") {
		t.Fatalf("anonymous /: %d (want 200 with the sign-in link)", resp.StatusCode)
	}
	resp, err = anon.Get(fe + "/projects")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); resp.StatusCode/100 != 3 || !strings.Contains(loc, "/auth/login?return_to=") || !strings.Contains(loc, url.QueryEscape("/projects")) {
		t.Fatalf("anonymous /projects: %d Location=%q (want a redirect to /auth/login carrying /projects)", resp.StatusCode, loc)
	}

	// Sign in the way the frontend's link does: pm-core /auth/login -> Keycloak form -> callback -> back to the frontend.
	resp, err = c.Get(core + "/auth/login?return_to=" + url.QueryEscape(fe+"/inbox"))
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	m := formAction.FindSubmatch(page)
	if m == nil {
		t.Fatalf("no login form in the Keycloak page (%d at %s)", resp.StatusCode, resp.Request.URL)
	}
	resp, err = c.PostForm(htmlAmp.Replace(string(m[1])), url.Values{"username": {user}, "password": {pass}, "credentialId": {""}})
	if err != nil {
		t.Fatalf("login form: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Request.URL.Path != "/inbox" {
		t.Fatalf("after login: %d at %s: %.300s", resp.StatusCode, resp.Request.URL, body)
	}
	fe = resp.Request.URL.Scheme + "://" + resp.Request.URL.Host
	get := func(path string, want ...string) string {
		t.Helper()
		resp, err := c.Get(fe + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("GET %s: %d: %.300s", path, resp.StatusCode, b)
		}
		for _, w := range want {
			if !strings.Contains(string(b), w) {
				t.Fatalf("GET %s: missing %q in %d bytes", path, w, len(b))
			}
		}
		return string(b)
	}
	ids := func(re *regexp.Regexp, html string, max int) []string {
		seen := map[string]bool{}
		var out []string
		for _, m := range re.FindAllStringSubmatch(html, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
			if len(out) == max {
				break
			}
		}
		return out
	}

	get("/", "Signed in as", user)
	get("/inbox", "<h1>Inbox</h1>")
	projects := ids(projectLink, get("/projects", "<h1>Projects</h1>"), 5)
	if len(projects) == 0 {
		t.Logf("%s sees no projects; the project and CAR screens are not walked", user)
		return
	}
	cars, chains, audits := 0, 0, 0
	for _, pid := range projects {
		project := get("/projects/"+pid, "Your access", "Overview")
		if strings.Contains(project, `href="/projects/`+pid+`/cars/new"`) {
			get("/projects/"+pid+"/cars/new", "New capital authorization request", `name="amount_requested"`)
		}
		for _, cid := range ids(carLink, project, 8) {
			car := get("/cars/"+cid, "Request", "Guidance")
			cars++
			if strings.Contains(car, "Approval chain") {
				chains++
			}
			if strings.Contains(car, "Audit trail") {
				audits++
			}
		}
	}
	t.Logf("%s: %d projects, %d CAR pages, %d with an approval chain, %d with an audit trail", user, len(projects), cars, chains, audits)
	if os.Getenv("SCREENS_EXPECT_CHAIN") == "1" && (chains == 0 || audits == 0) {
		t.Fatalf("expected at least one CAR page with an approval chain and an audit trail (chains=%d audits=%d)", chains, audits)
	}
}
