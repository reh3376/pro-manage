// Package oidc wraps one OpenID Connect issuer for the IdPi_OIDC module:
// discovery, PKCE authorization URLs, code exchange, token verification against
// the issuer's JWKS, refresh, and RP-initiated logout with refresh-token
// revocation. It knows nothing about pro_manage roles (see mapping) and holds
// no session state (pm-core does). Written to be lifted into the Connector SDK.
package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Config is the per-instance configuration (connector.yaml config_schema).
type Config struct {
	Issuer string // the `iss` value tokens carry; discovery is <Issuer>/.well-known/openid-configuration unless DiscoveryURL is set
	// DiscoveryURL overrides where the discovery document is fetched (a dev
	// IdP that advertises a browser-facing issuer the module cannot reach).
	DiscoveryURL string
	// BackchannelBase, when set, replaces the issuer's origin in server-to-server
	// endpoints (token, jwks, userinfo, revocation). Browser endpoints
	// (authorization, end_session) keep the advertised origin.
	BackchannelBase   string
	ClientID          string
	ClientSecret      string // empty for a public client
	Scopes            []string
	GroupsClaim       string // default "groups"
	RolesClaim        string // default "realm_access.roles" (dotted path)
	AcceptedAudiences []string
	HTTPClient        *http.Client
	DiscoveryTimeout  time.Duration
}

// Discovery is the subset of the provider metadata the module uses.
type Discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	RevocationEndpoint    string `json:"revocation_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

// Claims is what the module extracts from a token.
type Claims struct {
	Subject           string
	Email             string
	DisplayName       string
	PreferredUsername string
	Groups            []string
	Roles             []string
	Attributes        map[string]string
	ExpiresAt         time.Time
}

// Tokens is the result of an exchange or refresh.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    time.Time
}

// Errors the service maps to gRPC codes.
var (
	ErrNotReady     = errors.New("oidc: discovery not available")
	ErrInvalidGrant = errors.New("oidc: invalid grant") // bad code, verifier, or refresh token (INVALID_ARGUMENT)
	ErrInvalidToken = errors.New("oidc: invalid token")
	ErrUpstream     = errors.New("oidc: identity provider unavailable")
)

// Provider is one configured issuer. Discovery is lazy and cached; a failed
// discovery is retried on the next call, so the module starts before its IdP.
type Provider struct {
	cfg  Config
	http *http.Client

	mu       sync.Mutex
	disc     *Discovery
	keys     *gooidc.RemoteKeySet
	verifier *gooidc.IDTokenVerifier
	loadedAt time.Time
}

// New validates the config and returns a provider (no network yet).
func New(cfg Config) (*Provider, error) {
	cfg.Issuer = strings.TrimRight(strings.TrimSpace(cfg.Issuer), "/")
	if u, err := url.Parse(cfg.Issuer); err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("oidc: invalid issuer %q", cfg.Issuer)
	}
	if cfg.ClientID == "" {
		return nil, errors.New("oidc: client_id is required")
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{gooidc.ScopeOpenID, "profile", "email"}
	}
	if cfg.GroupsClaim == "" {
		cfg.GroupsClaim = "groups"
	}
	if cfg.RolesClaim == "" {
		cfg.RolesClaim = "realm_access.roles"
	}
	if cfg.DiscoveryTimeout == 0 {
		cfg.DiscoveryTimeout = 5 * time.Second
	}
	if cfg.DiscoveryURL == "" {
		cfg.DiscoveryURL = cfg.Issuer + "/.well-known/openid-configuration"
	}
	cfg.BackchannelBase = strings.TrimRight(cfg.BackchannelBase, "/")
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Provider{cfg: cfg, http: hc}, nil
}

// Issuer is the configured issuer.
func (p *Provider) Issuer() string { return p.cfg.Issuer }

// ClientID is the configured client.
func (p *Provider) ClientID() string { return p.cfg.ClientID }

// Discover fetches (or returns the cached) provider metadata.
func (p *Provider) Discover(ctx context.Context) (*Discovery, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.disc != nil {
		return p.disc, nil
	}
	ctx, cancel := context.WithTimeout(ctx, p.cfg.DiscoveryTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.DiscoveryURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: discovery: %v", ErrNotReady, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: discovery http %d", ErrNotReady, resp.StatusCode)
	}
	var d Discovery
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("%w: discovery decode: %v", ErrNotReady, err)
	}
	if strings.TrimRight(d.Issuer, "/") != p.cfg.Issuer {
		return nil, fmt.Errorf("%w: discovery issuer %q does not match configured %q", ErrNotReady, d.Issuer, p.cfg.Issuer)
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" || d.JWKSURI == "" {
		return nil, fmt.Errorf("%w: discovery lacks authorization, token, or jwks endpoints", ErrNotReady)
	}
	d.TokenEndpoint = p.backchannel(d.TokenEndpoint)
	d.JWKSURI = p.backchannel(d.JWKSURI)
	d.UserinfoEndpoint = p.backchannel(d.UserinfoEndpoint)
	d.RevocationEndpoint = p.backchannel(d.RevocationEndpoint)
	p.disc = &d
	keyCtx := gooidc.ClientContext(context.Background(), p.http) // outlives the request: the key set caches across calls
	p.keys = gooidc.NewRemoteKeySet(keyCtx, d.JWKSURI)
	p.verifier = gooidc.NewVerifier(p.cfg.Issuer, p.keys, &gooidc.Config{SkipClientIDCheck: true})
	p.loadedAt = time.Now()
	return p.disc, nil
}

// backchannel rewrites a frontend-origin URL to the backchannel origin.
func (p *Provider) backchannel(u string) string {
	if u == "" || p.cfg.BackchannelBase == "" {
		return u
	}
	iss, err := url.Parse(p.cfg.Issuer)
	if err != nil {
		return u
	}
	origin := iss.Scheme + "://" + iss.Host
	if strings.HasPrefix(u, origin) {
		return p.cfg.BackchannelBase + strings.TrimPrefix(u, origin)
	}
	return u
}

func (p *Provider) oauth(ctx context.Context, redirectURI string) (*oauth2.Config, error) {
	d, err := p.Discover(ctx)
	if err != nil {
		return nil, err
	}
	style := oauth2.AuthStyleInParams
	if p.cfg.ClientSecret != "" {
		style = oauth2.AuthStyleInHeader
	}
	return &oauth2.Config{
		ClientID: p.cfg.ClientID, ClientSecret: p.cfg.ClientSecret, RedirectURL: redirectURI, Scopes: p.cfg.Scopes,
		Endpoint: oauth2.Endpoint{AuthURL: d.AuthorizationEndpoint, TokenURL: d.TokenEndpoint, AuthStyle: style},
	}, nil
}

// AuthURL builds the PKCE authorization URL. The state is minted here from
// crypto/rand (never caller-supplied, so it cannot be fixed or guessed); the
// caller keeps it and the verifier server-side in its pending-login record
// and checks the IdP's callback against the state.
func (p *Provider) AuthURL(ctx context.Context, redirectURI, nonce string, extraScopes []string) (authURL, state, verifier string, err error) {
	if redirectURI == "" {
		return "", "", "", fmt.Errorf("%w: redirect_uri is required", ErrInvalidGrant)
	}
	oc, err := p.oauth(ctx, redirectURI)
	if err != nil {
		return "", "", "", err
	}
	oc.Scopes = append(append([]string{}, oc.Scopes...), extraScopes...)
	state = randomToken()
	verifier = oauth2.GenerateVerifier()
	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	if nonce != "" {
		opts = append(opts, gooidc.Nonce(nonce))
	}
	return oc.AuthCodeURL(state, opts...), state, verifier, nil
}

// randomToken is 32 random bytes, base64url without padding (43 characters).
func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Exchange redeems the authorization code and verifies the id token.
func (p *Provider) Exchange(ctx context.Context, code, redirectURI, verifier, nonce string) (Tokens, Claims, error) {
	if code == "" || redirectURI == "" || verifier == "" {
		return Tokens{}, Claims{}, fmt.Errorf("%w: code, redirect_uri, and code_verifier are required", ErrInvalidGrant)
	}
	oc, err := p.oauth(ctx, redirectURI)
	if err != nil {
		return Tokens{}, Claims{}, err
	}
	tok, err := oc.Exchange(context.WithValue(ctx, oauth2.HTTPClient, p.http), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Tokens{}, Claims{}, classify(err)
	}
	t := fromOAuth(tok)
	if t.IDToken == "" {
		return Tokens{}, Claims{}, fmt.Errorf("%w: no id_token in the token response", ErrInvalidToken)
	}
	c, err := p.verify(ctx, t.IDToken, nonce, true)
	if err != nil {
		return Tokens{}, Claims{}, err
	}
	return t, c, nil
}

// Verify checks a token's signature, issuer, expiry, and audience (the client
// id, the accepted audiences, or azp) and extracts the claims.
func (p *Provider) Verify(ctx context.Context, raw string) (Claims, error) {
	return p.verify(ctx, raw, "", false)
}

func (p *Provider) verify(ctx context.Context, raw, nonce string, idToken bool) (Claims, error) {
	if strings.TrimSpace(raw) == "" {
		return Claims{}, fmt.Errorf("%w: empty token", ErrInvalidToken)
	}
	if _, err := p.Discover(ctx); err != nil {
		return Claims{}, err
	}
	p.mu.Lock()
	v := p.verifier
	p.mu.Unlock()
	tok, err := v.Verify(gooidc.ClientContext(ctx, p.http), raw)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	var all map[string]any
	if err := tok.Claims(&all); err != nil {
		return Claims{}, fmt.Errorf("%w: claims: %v", ErrInvalidToken, err)
	}
	if !p.audienceOK(tok.Audience, str(all["azp"])) {
		return Claims{}, fmt.Errorf("%w: audience %v (azp %q) is not accepted", ErrInvalidToken, tok.Audience, str(all["azp"]))
	}
	if idToken && nonce != "" && tok.Nonce != nonce {
		return Claims{}, fmt.Errorf("%w: nonce mismatch", ErrInvalidToken)
	}
	c := p.extract(all)
	c.Subject = tok.Subject
	c.ExpiresAt = tok.Expiry
	return c, nil
}

func (p *Provider) audienceOK(aud []string, azp string) bool {
	accepted := map[string]bool{p.cfg.ClientID: true}
	for _, a := range p.cfg.AcceptedAudiences {
		accepted[a] = true
	}
	if azp != "" && accepted[azp] {
		return true
	}
	for _, a := range aud {
		if accepted[a] {
			return true
		}
	}
	return false
}

// extract pulls the claims pro_manage relies on; every other scalar string
// claim lands in Attributes for the mapping to reference.
func (p *Provider) extract(all map[string]any) Claims {
	c := Claims{Attributes: map[string]string{}}
	c.Email = str(all["email"])
	c.PreferredUsername = str(all["preferred_username"])
	c.DisplayName = str(all["name"])
	if c.DisplayName == "" {
		c.DisplayName = strings.TrimSpace(str(all["given_name"]) + " " + str(all["family_name"]))
	}
	c.Groups = strs(path(all, p.cfg.GroupsClaim))
	c.Roles = strs(path(all, p.cfg.RolesClaim))
	skip := map[string]bool{"iss": true, "sub": true, "aud": true, "exp": true, "iat": true, "nbf": true, "jti": true, "nonce": true, "at_hash": true, "c_hash": true, "sid": true, "session_state": true, "typ": true, "azp": true, "acr": true, "auth_time": true, "scope": true, "email": true, "preferred_username": true, "name": true, "given_name": true, "family_name": true, "email_verified": true}
	for k, v := range all {
		if skip[k] || k == p.cfg.GroupsClaim {
			continue
		}
		if s, ok := v.(string); ok {
			c.Attributes[k] = s
		}
	}
	return c
}

// Refresh redeems a refresh token.
func (p *Provider) Refresh(ctx context.Context, refreshToken string) (Tokens, error) {
	if refreshToken == "" {
		return Tokens{}, fmt.Errorf("%w: refresh_token is required", ErrInvalidGrant)
	}
	oc, err := p.oauth(ctx, "")
	if err != nil {
		return Tokens{}, err
	}
	tok, err := oc.TokenSource(context.WithValue(ctx, oauth2.HTTPClient, p.http), &oauth2.Token{RefreshToken: refreshToken, Expiry: time.Now().Add(-time.Minute)}).Token()
	if err != nil {
		return Tokens{}, classify(err)
	}
	t := fromOAuth(tok)
	if t.RefreshToken == "" {
		t.RefreshToken = refreshToken
	}
	return t, nil
}

// Logout revokes the refresh token (when the IdP has a revocation endpoint) and
// builds the RP-initiated logout URL (when it has an end-session endpoint).
func (p *Provider) Logout(ctx context.Context, refreshToken, idTokenHint, postLogoutRedirect string) (revoked bool, endSessionURL string, err error) {
	d, err := p.Discover(ctx)
	if err != nil {
		return false, "", err
	}
	if d.EndSessionEndpoint != "" {
		q := url.Values{"client_id": {p.cfg.ClientID}}
		if idTokenHint != "" {
			q.Set("id_token_hint", idTokenHint)
		}
		if postLogoutRedirect != "" {
			q.Set("post_logout_redirect_uri", postLogoutRedirect)
		}
		endSessionURL = d.EndSessionEndpoint + "?" + q.Encode()
	}
	if refreshToken == "" || d.RevocationEndpoint == "" {
		return false, endSessionURL, nil
	}
	form := url.Values{"token": {refreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {p.cfg.ClientID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.RevocationEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return false, endSessionURL, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if p.cfg.ClientSecret != "" {
		req.SetBasicAuth(p.cfg.ClientID, p.cfg.ClientSecret)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return false, endSessionURL, fmt.Errorf("%w: revoke: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	switch {
	case resp.StatusCode == http.StatusOK:
		return true, endSessionURL, nil
	case resp.StatusCode >= 500:
		return false, endSessionURL, fmt.Errorf("%w: revoke http %d", ErrUpstream, resp.StatusCode)
	default:
		return false, endSessionURL, nil // RFC 7009: an unknown token is not an error
	}
}

// ---------------------------------------------------------------- helpers

func fromOAuth(tok *oauth2.Token) Tokens {
	t := Tokens{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, ExpiresAt: tok.Expiry}
	if id, ok := tok.Extra("id_token").(string); ok {
		t.IDToken = id
	}
	return t
}

func classify(err error) error {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		if re.Response != nil && re.Response.StatusCode >= 500 {
			return fmt.Errorf("%w: token endpoint http %d", ErrUpstream, re.Response.StatusCode)
		}
		return fmt.Errorf("%w: %s %s", ErrInvalidGrant, re.ErrorCode, re.ErrorDescription)
	}
	return fmt.Errorf("%w: %v", ErrUpstream, err)
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func strs(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(list))
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// path resolves a dotted claim path ("realm_access.roles").
func path(m map[string]any, p string) any {
	var cur any = m
	for _, part := range strings.Split(p, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[part]
	}
	return cur
}
