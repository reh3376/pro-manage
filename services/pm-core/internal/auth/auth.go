// Package auth is pm-core's login edge on the identity connector (FSD §04):
// /auth/login starts the code flow through IdPi, /auth/callback completes it,
// provisions the user just in time, fills the approver pools from the claims,
// opens a Redis-backed session and sets the cookie; /auth/me reports the
// signed-in user; /auth/logout ends the session and cascades to the IdP.
// Everything else in pm-core sees only the Actor the session resolves to.
package auth

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/idpi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/domain"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
	"github.com/reh3376/pro-manage/services/pm-core/internal/sessions"
)

// CookieName carries the opaque session id (§04: never a token).
const CookieName = "pm_session"

// IdPi is the slice of the connector client the handler uses (a fake in tests).
type IdPi interface {
	Begin(ctx context.Context, redirectURI, nonce string) (*connectorv1.AuthenticateInteractiveResponse, error)
	Complete(ctx context.Context, code, redirectURI, verifier, nonce string) (*connectorv1.CompleteAuthenticationResponse, error)
	Resolve(ctx context.Context, claims *connectorv1.Claims) (*connectorv1.ResolveClaimsToRolesResponse, error)
	Logout(ctx context.Context, refreshToken, idToken, postLogoutRedirect string) (*connectorv1.LogoutResponse, error)
}

// Users is the slice of the domain the handler uses.
type Users interface {
	RecordLogin(ctx context.Context, in domain.LoginInput) (db.User, error)
	RecordLogout(ctx context.Context, actor domain.Actor, sessionID string) error
}

// Handler serves /auth/*.
type Handler struct {
	Log          *slog.Logger
	Sessions     *sessions.Store
	IdPi         IdPi
	Users        Users
	PublicURL    string // where the browser reaches pm-core; the callback is <PublicURL>/auth/callback
	FrontendURL  string // default return_to and the post-logout landing
	CookieSecure bool
	RequestID    func(ctx context.Context) string
}

// Mount registers the routes. They sit outside /api/v1 and outside requireActor.
func (h *Handler) Mount(mux *http.ServeMux, wrap func(http.Handler) http.Handler) {
	mux.Handle("GET /auth/login", wrap(http.HandlerFunc(h.login)))
	mux.Handle("GET /auth/callback", wrap(http.HandlerFunc(h.callback)))
	mux.Handle("GET /auth/me", wrap(http.HandlerFunc(h.me)))
	mux.Handle("POST /auth/logout", wrap(http.HandlerFunc(h.logout)))
}

func (h *Handler) callbackURL() string { return strings.TrimRight(h.PublicURL, "/") + "/auth/callback" }

func (h *Handler) rid(ctx context.Context) string {
	if h.RequestID == nil {
		return ""
	}
	return h.RequestID(ctx)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	httpx.WriteJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

// login: mint nonce, ask the module for the redirect, remember the pending login, send the browser.
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	returnTo := h.safeReturnTo(r.URL.Query().Get("return_to"))
	nonce := sessions.NewID()
	resp, err := h.IdPi.Begin(r.Context(), h.callbackURL(), nonce)
	if err != nil {
		h.Log.Error("login begin", "err", err, "request_id", h.rid(r.Context()))
		writeError(w, http.StatusServiceUnavailable, "idp_unavailable", "sign-in is not available right now")
		return
	}
	if err := h.Sessions.PutPending(r.Context(), sessions.Pending{State: resp.GetState(), Nonce: nonce, Verifier: resp.GetCodeVerifier(), RedirectURI: h.callbackURL(), ReturnTo: returnTo}); err != nil {
		h.Log.Error("login pending", "err", err)
		writeError(w, http.StatusServiceUnavailable, "session_store_unavailable", "sign-in is not available right now")
		return
	}
	http.Redirect(w, r, resp.GetAuthorizationUrl(), http.StatusFound)
}

// callback: redeem the pending login, exchange the code, resolve roles, provision, open the session.
func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		h.Log.Warn("idp returned an error", "error", e, "description", q.Get("error_description"))
		writeError(w, http.StatusBadRequest, "idp_error", e+": "+q.Get("error_description"))
		return
	}
	pending, err := h.Sessions.TakePending(r.Context(), q.Get("state"))
	if errors.Is(err, sessions.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "login_state_unknown", "this sign-in attempt is unknown or expired; start again at /auth/login")
		return
	}
	if err != nil {
		h.Log.Error("take pending", "err", err)
		writeError(w, http.StatusServiceUnavailable, "session_store_unavailable", "sign-in is not available right now")
		return
	}
	if q.Get("code") == "" {
		writeError(w, http.StatusBadRequest, "code_missing", "the identity provider sent no authorization code")
		return
	}
	done, err := h.IdPi.Complete(r.Context(), q.Get("code"), pending.RedirectURI, pending.Verifier, pending.Nonce)
	if err != nil {
		if errors.Is(err, idpi.ErrRejected) {
			writeError(w, http.StatusBadRequest, "login_rejected", "the identity provider rejected this sign-in; start again at /auth/login")
			return
		}
		h.Log.Error("complete authentication", "err", err)
		writeError(w, http.StatusServiceUnavailable, "idp_unavailable", "sign-in is not available right now")
		return
	}
	claims := done.GetClaims()
	roles, err := h.IdPi.Resolve(r.Context(), claims)
	if err != nil {
		h.Log.Error("resolve roles", "err", err)
		writeError(w, http.StatusServiceUnavailable, "idp_unavailable", "sign-in is not available right now")
		return
	}
	username := claims.GetPreferredUsername()
	if username == "" {
		username = claims.GetEmail()
	}
	host, _, _ := strings.Cut(r.RemoteAddr, ":")
	user, err := h.Users.RecordLogin(r.Context(), domain.LoginInput{
		Subject: claims.GetSubject(), Username: username, Email: claims.GetEmail(), DisplayName: claims.GetDisplayName(),
		PlatformRoles: roles.GetPlatformRoles(), Pools: roles.GetPools(), Attributes: roles.GetAttributes(), SourceIP: host, RequestID: h.rid(r.Context()),
	})
	if err != nil {
		if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrValidation) {
			writeError(w, http.StatusConflict, "identity_conflict", err.Error())
			return
		}
		h.Log.Error("record login", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "sign-in failed; see request_id in logs")
		return
	}
	var tokenExp time.Time
	if done.GetExpiresAt() != nil {
		tokenExp = done.GetExpiresAt().AsTime()
	}
	sess, err := h.Sessions.Create(r.Context(), sessions.Session{
		UserID: user.UserID.String(), Username: user.Username, Subject: user.ExternalID, Email: user.Email, DisplayName: user.DisplayName,
		PlatformRoles: orEmpty(roles.GetPlatformRoles()), Pools: orEmpty(roles.GetPools()), Attributes: roles.GetAttributes(),
		RefreshToken: done.GetRefreshToken(), IDToken: done.GetIdToken(), TokenExpiresAt: tokenExp,
	})
	if err != nil {
		h.Log.Error("create session", "err", err)
		writeError(w, http.StatusServiceUnavailable, "session_store_unavailable", "sign-in is not available right now")
		return
	}
	http.SetCookie(w, h.cookie(sess.ID, int(h.Sessions.MaxAge/time.Second)))
	h.Log.Info("signed in", "user", user.Username, "roles", roles.GetPlatformRoles(), "pools", roles.GetPools(), "session", sess.ID[:8])
	http.Redirect(w, r, pending.ReturnTo, http.StatusFound)
}

// me reports the session's user; 401 without one.
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.Session(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "no session; sign in at /auth/login")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user":           map[string]any{"user_id": sess.UserID, "username": sess.Username, "subject": sess.Subject, "email": sess.Email, "display_name": sess.DisplayName},
		"platform_roles": sess.PlatformRoles, "pools": sess.Pools, "attributes": sess.Attributes,
		"session": map[string]any{"created_at": sess.CreatedAt, "expires_at": sess.ExpiresAt, "last_seen_at": sess.LastSeenAt},
	})
}

// logout ends the session, revokes at the IdP, and returns the end-session URL for the browser.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.Session(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "no session")
		return
	}
	endSession := ""
	if resp, err := h.IdPi.Logout(r.Context(), sess.RefreshToken, sess.IDToken, h.FrontendURL); err != nil {
		h.Log.Warn("idp logout failed; local session ended anyway", "err", err)
	} else {
		endSession = resp.GetEndSessionUrl()
	}
	_ = h.Sessions.Delete(r.Context(), sess.ID)
	host, _, _ := strings.Cut(r.RemoteAddr, ":")
	logoutActor := domain.Actor{UserID: sess.Username, Subject: sess.Subject, SourceIP: host, RequestID: h.rid(r.Context())}
	if id, err := uuid.Parse(sess.UserID); err == nil {
		logoutActor.ID = &id
	}
	if err := h.Users.RecordLogout(r.Context(), logoutActor, sess.ID); err != nil {
		h.Log.Error("record logout", "err", err)
	}
	http.SetCookie(w, h.cookie("", -1))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"signed_out": true, "end_session_url": endSession})
}

// Session reads the cookie and loads the session; false when absent or expired.
func (h *Handler) Session(r *http.Request) (sessions.Session, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return sessions.Session{}, false
	}
	sess, err := h.Sessions.Get(r.Context(), c.Value)
	if err != nil {
		return sessions.Session{}, false
	}
	return sess, true
}

// Actor is what requireActor uses: the session's user as a domain actor.
func (h *Handler) Actor(r *http.Request) (domain.Actor, bool) {
	sess, ok := h.Session(r)
	if !ok {
		return domain.Actor{}, false
	}
	host, _, _ := strings.Cut(r.RemoteAddr, ":")
	actor := domain.Actor{UserID: sess.Username, Subject: sess.Subject, PlatformRoles: sess.PlatformRoles, SessionID: sess.ID, SourceIP: host}
	if id, err := uuid.Parse(sess.UserID); err == nil {
		actor.ID = &id
	}
	return actor, true
}

func (h *Handler) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: value, Path: "/", HttpOnly: true, Secure: h.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge}
}

// safeReturnTo allows relative paths and the frontend or pm-core origins; anything else lands on the frontend.
func (h *Handler) safeReturnTo(raw string) string {
	def := strings.TrimRight(h.FrontendURL, "/") + "/"
	if raw == "" {
		return def
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return def
	}
	origin := u.Scheme + "://" + u.Host
	for _, allowed := range []string{h.FrontendURL, h.PublicURL} {
		if a, err := url.Parse(allowed); err == nil && a.Scheme+"://"+a.Host == origin {
			return raw
		}
	}
	return def
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
