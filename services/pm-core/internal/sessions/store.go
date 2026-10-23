// Package sessions is the Redis-backed session model of FSD §04: opaque bearer
// session ids issued by pm-core, a sliding lifetime with an absolute cap, and
// the short-lived pending-login records that carry OAuth state, nonce, and the
// PKCE verifier between the two legs of the login. Nothing here is a credential
// the browser can read: the cookie holds only the session id.
package sessions

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Defaults from §04 "Session model".
const (
	DefaultTTL        = 8 * time.Hour
	DefaultMaxAge     = 24 * time.Hour
	DefaultPendingTTL = 10 * time.Minute
)

// ErrNotFound is returned for an unknown, expired, or already-used record.
var ErrNotFound = errors.New("sessions: not found")

// Pending is one login in flight (keyed by the OAuth state the IdP echoes back).
type Pending struct {
	State       string    `json:"state"`
	Nonce       string    `json:"nonce"`
	Verifier    string    `json:"verifier"`
	RedirectURI string    `json:"redirect_uri"`
	ReturnTo    string    `json:"return_to"`
	CreatedAt   time.Time `json:"created_at"`
}

// Session is the server-side record behind a cookie.
type Session struct {
	ID             string            `json:"id"`
	UserID         string            `json:"user_id"`
	Username       string            `json:"username"`
	Subject        string            `json:"subject"`
	Email          string            `json:"email"`
	DisplayName    string            `json:"display_name"`
	PlatformRoles  []string          `json:"platform_roles"`
	Pools          []string          `json:"pools"`
	Attributes     map[string]string `json:"attributes"`
	CreatedAt      time.Time         `json:"created_at"`
	ExpiresAt      time.Time         `json:"expires_at"` // absolute cap; the sliding window is the key's TTL
	LastSeenAt     time.Time         `json:"last_seen_at"`
	RefreshToken   string            `json:"refresh_token,omitempty"` // IdP tokens for logout cascade and refresh; never serialized to clients
	IDToken        string            `json:"id_token,omitempty"`
	TokenExpiresAt time.Time         `json:"token_expires_at"`
}

// Store holds sessions and pending logins in Redis.
type Store struct {
	rdb        *redis.Client
	Prefix     string
	TTL        time.Duration // sliding window
	MaxAge     time.Duration // absolute cap from creation
	PendingTTL time.Duration
}

// Open connects to a redis:// URL and pings it.
func Open(ctx context.Context, url string) (*Store, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("sessions: redis url: %w", err)
	}
	rdb := redis.NewClient(opts)
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("sessions: redis ping: %w", err)
	}
	return New(rdb), nil
}

// New wraps an existing client (tests use miniredis).
func New(rdb *redis.Client) *Store {
	return &Store{rdb: rdb, Prefix: "pm:", TTL: DefaultTTL, MaxAge: DefaultMaxAge, PendingTTL: DefaultPendingTTL}
}

// Ping is the readiness probe.
func (s *Store) Ping(ctx context.Context) error { return s.rdb.Ping(ctx).Err() }

// Close releases the client.
func (s *Store) Close() error { return s.rdb.Close() }

func (s *Store) pendingKey(state string) string { return s.Prefix + "login:" + state }
func (s *Store) sessionKey(id string) string    { return s.Prefix + "session:" + id }

// NewID is 32 random bytes, base64url (43 characters), for session ids and nonces.
func NewID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// PutPending stores a login in flight until the callback or the TTL.
func (s *Store) PutPending(ctx context.Context, p Pending) error {
	if p.State == "" {
		return errors.New("sessions: pending login needs a state")
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	b, _ := json.Marshal(p)
	return s.rdb.Set(ctx, s.pendingKey(p.State), b, s.PendingTTL).Err()
}

// TakePending consumes a pending login: a state can be redeemed once.
func (s *Store) TakePending(ctx context.Context, state string) (Pending, error) {
	if state == "" {
		return Pending{}, ErrNotFound
	}
	raw, err := s.rdb.GetDel(ctx, s.pendingKey(state)).Result()
	if errors.Is(err, redis.Nil) {
		return Pending{}, ErrNotFound
	}
	if err != nil {
		return Pending{}, fmt.Errorf("sessions: take pending: %w", err)
	}
	var p Pending
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Pending{}, fmt.Errorf("sessions: pending decode: %w", err)
	}
	return p, nil
}

// Create stores a new session and returns its id. ExpiresAt is set to the absolute cap.
func (s *Store) Create(ctx context.Context, sess Session) (Session, error) {
	now := time.Now().UTC()
	sess.ID = NewID()
	sess.CreatedAt, sess.LastSeenAt = now, now
	sess.ExpiresAt = now.Add(s.MaxAge)
	if err := s.write(ctx, sess, s.TTL); err != nil {
		return Session{}, err
	}
	return sess, nil
}

// Get returns a live session and slides its window (never past the absolute cap).
func (s *Store) Get(ctx context.Context, id string) (Session, error) {
	if id == "" {
		return Session{}, ErrNotFound
	}
	raw, err := s.rdb.Get(ctx, s.sessionKey(id)).Result()
	if errors.Is(err, redis.Nil) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("sessions: get: %w", err)
	}
	var sess Session
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return Session{}, fmt.Errorf("sessions: decode: %w", err)
	}
	now := time.Now().UTC()
	if !now.Before(sess.ExpiresAt) {
		_ = s.rdb.Del(ctx, s.sessionKey(id)).Err()
		return Session{}, ErrNotFound
	}
	// Slide at most once a minute to keep the hot path cheap.
	if now.Sub(sess.LastSeenAt) > time.Minute {
		sess.LastSeenAt = now
		_ = s.write(ctx, sess, s.remaining(sess, now))
	}
	return sess, nil
}

// Update rewrites a session (e.g. after a token refresh) keeping its window.
func (s *Store) Update(ctx context.Context, sess Session) error {
	return s.write(ctx, sess, s.remaining(sess, time.Now().UTC()))
}

// Delete ends a session.
func (s *Store) Delete(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	return s.rdb.Del(ctx, s.sessionKey(id)).Err()
}

func (s *Store) remaining(sess Session, now time.Time) time.Duration {
	ttl := s.TTL
	if cap := sess.ExpiresAt.Sub(now); cap < ttl {
		ttl = cap
	}
	if ttl <= 0 {
		ttl = time.Second
	}
	return ttl
}

func (s *Store) write(ctx context.Context, sess Session, ttl time.Duration) error {
	b, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	if err := s.rdb.Set(ctx, s.sessionKey(sess.ID), b, ttl).Err(); err != nil {
		return fmt.Errorf("sessions: write: %w", err)
	}
	return nil
}
