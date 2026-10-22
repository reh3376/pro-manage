package mdemgadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Errors the caller branches on (FSD §06 tiers: Unavailable is what fail-closed
// tiers refuse on; BadRequest and Unauthorized are pm-core bugs or
// misconfiguration and are never retried).
var (
	ErrUnavailable  = errors.New("mdemg: unavailable")
	ErrUnauthorized = errors.New("mdemg: unauthorized")
	ErrBadRequest   = errors.New("mdemg: bad request")
	ErrNotFound     = errors.New("mdemg: not found")
)

// Error carries the HTTP detail behind a sentinel.
type Error struct {
	Sentinel  error
	Status    int
	Body      string
	RequestID string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%v (http %d, request_id %s): %s", e.Sentinel, e.Status, e.RequestID, e.Body)
}

func (e *Error) Unwrap() error { return e.Sentinel }

// Options configure a Client.
type Options struct {
	BaseURL string
	APIKey  string
	// Timeouts per surface (FSD §06 latency budgets; classify is a policy variable).
	GuideTimeout    time.Duration
	ClassifyTimeout time.Duration
	WriteTimeout    time.Duration // feedback, observe, warm
	HealthTimeout   time.Duration
	HTTPClient      *http.Client
	UserAgent       string
}

// Client talks to one MDEMG instance.
type Client struct {
	base string
	key  string
	http *http.Client
	opts Options
}

// New validates options and builds a client.
func New(opts Options) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("mdemgadapter: invalid base url %q", opts.BaseURL)
	}
	if opts.GuideTimeout == 0 {
		opts.GuideTimeout = 1500 * time.Millisecond
	}
	if opts.ClassifyTimeout == 0 {
		opts.ClassifyTimeout = 10 * time.Second
	}
	if opts.WriteTimeout == 0 {
		opts.WriteTimeout = 5 * time.Second
	}
	if opts.HealthTimeout == 0 {
		opts.HealthTimeout = 3 * time.Second
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "pro-manage/pm-core"
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{} // per-call timeouts come from contexts
	}
	return &Client{base: base, key: opts.APIKey, http: hc, opts: opts}, nil
}

// ---------------------------------------------------------------- surfaces

// Health probes /healthz and /v1/jiminy/healthz.
func (c *Client) Health(ctx context.Context) Health {
	start := time.Now()
	h := Health{}
	var root map[string]any
	if err := c.do(ctx, c.opts.HealthTimeout, http.MethodGet, "/healthz", nil, nil, &root); err != nil {
		h.Error = err.Error()
		h.LatencyMs = time.Since(start).Milliseconds()
		return h
	}
	h.Reachable = true
	var j struct {
		Status  string `json:"status"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.do(ctx, c.opts.HealthTimeout, http.MethodGet, "/v1/jiminy/healthz", nil, nil, &j); err != nil {
		h.Error = err.Error()
	} else {
		h.JiminyEnabled = j.Enabled
		h.Status = j.Status
	}
	h.LatencyMs = time.Since(start).Milliseconds()
	return h
}

// Warm asks MDEMG to pre-compute guidance for a space (fire-and-forget on the caller's side).
func (c *Client) Warm(ctx context.Context, req WarmRequest) (WarmResponse, error) {
	var out WarmResponse
	err := c.do(ctx, c.opts.WriteTimeout, http.MethodPost, "/v1/jiminy/warm", nil, req, &out)
	return out, err
}

// Latest reads the warmed result for a space.
func (c *Client) Latest(ctx context.Context, spaceID, sessionID string) (LatestResponse, error) {
	q := url.Values{"space_id": {spaceID}}
	if sessionID != "" {
		q.Set("session_id", sessionID)
	}
	var out LatestResponse
	err := c.do(ctx, c.opts.GuideTimeout, http.MethodGet, "/v1/jiminy/latest", q, nil, &out)
	return out, err
}

// Guide computes guidance for a space (the full call; prefer Warm + Latest on page open).
func (c *Client) Guide(ctx context.Context, req GuideRequest) (GuideResponse, error) {
	var env struct {
		Data GuideResponse `json:"data"`
	}
	err := c.do(ctx, c.opts.GuideTimeout, http.MethodPost, "/v1/jiminy/guide", nil, req, &env)
	if env.Data.Guidance == nil {
		env.Data.Guidance = []GuidanceItem{}
	}
	return env.Data, err
}

// Classify returns MDEMG's verdict for a proposed action in one space.
func (c *Client) Classify(ctx context.Context, req ClassifyRequest) (ClassifyResponse, error) {
	var env struct {
		Data ClassifyResponse `json:"data"`
	}
	err := c.do(ctx, c.opts.ClassifyTimeout, http.MethodPost, "/v1/jiminy/classify", nil, req, &env)
	return env.Data, err
}

// Feedback closes the loop on a guidance id.
func (c *Client) Feedback(ctx context.Context, req FeedbackRequest) (FeedbackResponse, error) {
	var out FeedbackResponse
	err := c.do(ctx, c.opts.WriteTimeout, http.MethodPost, "/v1/jiminy/feedback", nil, req, &out)
	return out, err
}

// Observe writes one observation.
func (c *Client) Observe(ctx context.Context, req ObserveRequest) (ObserveResponse, error) {
	var out ObserveResponse
	err := c.do(ctx, c.opts.WriteTimeout, http.MethodPost, "/v1/conversation/observe", nil, req, &out)
	return out, err
}

// ---------------------------------------------------------------- transport

func (c *Client) do(ctx context.Context, timeout time.Duration, method, path string, query url.Values, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("mdemgadapter: encode: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return fmt.Errorf("mdemgadapter: request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.opts.UserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.key != "" {
		req.Header.Set("X-API-Key", c.key)
	}
	if rid := RequestIDFrom(ctx); rid != "" {
		req.Header.Set("X-Request-ID", rid)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return &Error{Sentinel: ErrUnavailable, Body: err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	rid := resp.Header.Get("X-Request-ID")

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if out != nil && len(raw) > 0 {
			if err := json.Unmarshal(raw, out); err != nil {
				return &Error{Sentinel: ErrUnavailable, Status: resp.StatusCode, RequestID: rid, Body: "undecodable response: " + err.Error()}
			}
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return &Error{Sentinel: ErrUnauthorized, Status: resp.StatusCode, RequestID: rid, Body: trim(raw)}
	case resp.StatusCode == http.StatusNotFound:
		return &Error{Sentinel: ErrNotFound, Status: resp.StatusCode, RequestID: rid, Body: trim(raw)}
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return &Error{Sentinel: ErrBadRequest, Status: resp.StatusCode, RequestID: rid, Body: trim(raw)}
	default:
		return &Error{Sentinel: ErrUnavailable, Status: resp.StatusCode, RequestID: rid, Body: trim(raw)}
	}
}

func trim(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 512 {
		s = s[:512] + "..."
	}
	return s
}

type ctxKey int

const requestIDKey ctxKey = 1

// WithRequestID attaches pm-core's request id so MDEMG logs correlate (§06 audit coupling).
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFrom reads it back.
func RequestIDFrom(ctx context.Context) string {
	s, _ := ctx.Value(requestIDKey).(string)
	return s
}
