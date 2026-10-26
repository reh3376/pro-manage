// Package mail renders and sends pro_manage notifications as email: a plain
// text body (with an optional HTML alternative) carrying the deep link, sent
// over SMTP with optional STARTTLS and PLAIN auth. A dedupe window drops a
// repeat of the same key (§10 "Dedupe window per (user, kind, subject)").
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"sync"
	"time"
)

// Config is the per-instance configuration (connector.yaml config_schema).
type Config struct {
	Host         string
	Port         int
	StartTLS     bool
	Username     string
	Password     string
	From         string // "pro_manage <no-reply@example>"
	DedupeWindow time.Duration
	AppBaseURL   string
	Timeout      time.Duration
}

// Message is what the service asks to send.
type Message struct {
	To        []mail.Address
	Subject   string
	Text      string
	HTML      string
	DeepLink  string
	DedupeKey string
	Headers   map[string]string // extra headers (X-PM-Kind, X-PM-Subject)
}

// Result of a send.
type Result struct {
	MessageID string
	Status    string // sent | deduped | rejected
	Detail    string
}

// Errors.
var (
	ErrRejected    = errors.New("mail: rejected") // bad input, never retried
	ErrUnavailable = errors.New("mail: relay unavailable")
)

// Sender sends messages over one relay.
type Sender struct {
	cfg  Config
	from mail.Address
	mu   sync.Mutex
	seen map[string]time.Time
}

// New validates the config.
func New(cfg Config) (*Sender, error) {
	if cfg.Host == "" {
		return nil, errors.New("mail: smtp host required")
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("mail: from address: %w", err)
	}
	if cfg.DedupeWindow == 0 {
		cfg.DedupeWindow = 10 * time.Minute
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &Sender{cfg: cfg, from: *from, seen: map[string]time.Time{}}, nil
}

// Addr is host:port.
func (s *Sender) Addr() string { return net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port)) }

// Probe dials the relay and reads its greeting (HealthCheck).
func (s *Sender) Probe(ctx context.Context) error {
	d := net.Dialer{Timeout: s.cfg.Timeout}
	conn, err := d.DialContext(ctx, "tcp", s.Addr())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer conn.Close()
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer c.Close()
	return c.Noop()
}

// Deduped reports whether key was sent inside the window and records it otherwise.
func (s *Sender) Deduped(key string) bool {
	if key == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, t := range s.seen {
		if now.Sub(t) > s.cfg.DedupeWindow {
			delete(s.seen, k)
		}
	}
	if t, ok := s.seen[key]; ok && now.Sub(t) <= s.cfg.DedupeWindow {
		return true
	}
	s.seen[key] = now
	return false
}

// Render builds the RFC 5322 message bytes and its Message-ID.
func (s *Sender) Render(m Message) (id string, data []byte, err error) {
	if len(m.To) == 0 {
		return "", nil, fmt.Errorf("%w: no recipients", ErrRejected)
	}
	if strings.TrimSpace(m.Subject) == "" {
		return "", nil, fmt.Errorf("%w: subject required", ErrRejected)
	}
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	domain := "pro-manage.local"
	if at := strings.LastIndex(s.from.Address, "@"); at > 0 {
		domain = s.from.Address[at+1:]
	}
	id = "<" + hex.EncodeToString(b) + "@" + domain + ">"
	link := m.DeepLink
	if link != "" && strings.HasPrefix(link, "/") && s.cfg.AppBaseURL != "" {
		link = strings.TrimRight(s.cfg.AppBaseURL, "/") + link
	}
	text := m.Text
	if link != "" {
		text = strings.TrimRight(text, "\n") + "\n\nOpen in pro_manage: " + link + "\n"
	}
	var buf bytes.Buffer
	h := func(k, v string) { fmt.Fprintf(&buf, "%s: %s\r\n", k, v) }
	h("Message-ID", id)
	h("Date", time.Now().UTC().Format(time.RFC1123Z))
	h("From", s.from.String())
	tos := make([]string, 0, len(m.To))
	for _, t := range m.To {
		tos = append(tos, t.String())
	}
	h("To", strings.Join(tos, ", "))
	h("Subject", mimeHeader(m.Subject))
	h("MIME-Version", "1.0")
	h("X-Mailer", "pro_manage Commsi_SMTP")
	for k, v := range m.Headers {
		if strings.HasPrefix(k, "X-") {
			h(k, mimeHeader(v))
		}
	}
	if m.HTML == "" {
		h("Content-Type", "text/plain; charset=utf-8")
		h("Content-Transfer-Encoding", "8bit")
		buf.WriteString("\r\n")
		buf.WriteString(crlf(text))
		return id, buf.Bytes(), nil
	}
	mw := multipart.NewWriter(&buf)
	h("Content-Type", "multipart/alternative; boundary="+mw.Boundary())
	buf.WriteString("\r\n")
	for _, part := range []struct{ ct, body string }{{"text/plain; charset=utf-8", text}, {"text/html; charset=utf-8", m.HTML}} {
		pw, err := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {part.ct}, "Content-Transfer-Encoding": {"8bit"}})
		if err != nil {
			return "", nil, err
		}
		pw.Write([]byte(crlf(part.body)))
	}
	_ = mw.Close()
	return id, buf.Bytes(), nil
}

// Send delivers a message. Dedupe is applied first.
func (s *Sender) Send(ctx context.Context, m Message) (Result, error) {
	if s.Deduped(m.DedupeKey) {
		return Result{Status: "deduped", Detail: "repeat of " + m.DedupeKey + " inside the dedupe window"}, nil
	}
	id, data, err := s.Render(m)
	if err != nil {
		return Result{Status: "rejected", Detail: err.Error()}, err
	}
	rcpts := make([]string, 0, len(m.To))
	for _, t := range m.To {
		rcpts = append(rcpts, t.Address)
	}
	if err := s.deliver(ctx, rcpts, data); err != nil {
		return Result{Status: "rejected", Detail: err.Error()}, err
	}
	return Result{MessageID: id, Status: "sent"}, nil
}

func (s *Sender) deliver(ctx context.Context, rcpts []string, data []byte) error {
	d := net.Dialer{Timeout: s.cfg.Timeout}
	conn, err := d.DialContext(ctx, "tcp", s.Addr())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(s.cfg.Timeout))
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer c.Close()
	if s.cfg.StartTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("%w: starttls: %v", ErrUnavailable, err)
			}
		}
	}
	if s.cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("%w: auth: %v", ErrUnavailable, err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return fmt.Errorf("%w: from: %v", ErrRejected, err)
	}
	for _, r := range rcpts {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("%w: rcpt %s: %v", ErrRejected, r, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("%w: data: %v", ErrUnavailable, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("%w: write: %v", ErrUnavailable, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("%w: close: %v", ErrUnavailable, err)
	}
	return c.Quit()
}

func crlf(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}

func mimeHeader(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")
	for _, c := range s {
		if c > 126 {
			return "=?utf-8?q?" + qEncode(s) + "?="
		}
	}
	return s
}

func qEncode(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case c == ' ':
			b.WriteByte('_')
		case c >= 33 && c <= 126 && c != '=' && c != '?' && c != '_':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "=%02X", c)
		}
	}
	return b.String()
}
