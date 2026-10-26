package mail

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestRenderAndDedupe(t *testing.T) {
	s, err := New(Config{Host: "127.0.0.1", Port: 1, From: "pro_manage <no-reply@northfield.example>", AppBaseURL: "http://localhost:3100", DedupeWindow: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	id, data, err := s.Render(Message{To: []mail.Address{{Name: "Alice Okafor", Address: "alice.owner@northfield.example"}}, Subject: "CAR CR-1 awaits your decision", Text: "Stage 1 of 4.\n", DeepLink: "/inbox", Headers: map[string]string{"X-PM-Kind": "approval.requested", "Ignored": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{"Message-ID: " + id, "From: \"pro_manage\" <no-reply@northfield.example>", "To: \"Alice Okafor\" <alice.owner@northfield.example>", "Subject: CAR CR-1 awaits your decision", "X-PM-Kind: approval.requested", "Content-Type: text/plain; charset=utf-8", "Open in pro_manage: http://localhost:3100/inbox"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in\n%s", want, body)
		}
	}
	if strings.Contains(body, "Ignored:") || !strings.HasSuffix(id, "@northfield.example>") {
		t.Fatalf("headers: %s %s", id, body)
	}
	_, html, _ := s.Render(Message{To: []mail.Address{{Address: "a@b"}}, Subject: "ünïcode", Text: "t", HTML: "<p>h</p>"})
	if !strings.Contains(string(html), "multipart/alternative") || !strings.Contains(string(html), "=?utf-8?q?") || !strings.Contains(string(html), "<p>h</p>") {
		t.Fatalf("html: %s", html)
	}
	if _, _, err := s.Render(Message{Subject: "x"}); !errors.Is(err, ErrRejected) {
		t.Fatal("no recipients must be rejected")
	}
	if _, _, err := s.Render(Message{To: []mail.Address{{Address: "a@b"}}}); !errors.Is(err, ErrRejected) {
		t.Fatal("empty subject must be rejected")
	}

	if s.Deduped("k1") || !s.Deduped("k1") || s.Deduped("k2") || s.Deduped("") {
		t.Fatal("dedupe window")
	}
	time.Sleep(60 * time.Millisecond)
	if s.Deduped("k1") {
		t.Fatal("window must expire")
	}
	// A deduped send never dials.
	s.Deduped("k3")
	if r, err := s.Send(context.Background(), Message{To: []mail.Address{{Address: "a@b"}}, Subject: "s", DedupeKey: "k3"}); err != nil || r.Status != "deduped" {
		t.Fatalf("deduped send: %+v %v", r, err)
	}
	// An unreachable relay is unavailable, not rejected.
	if _, err := s.Send(context.Background(), Message{To: []mail.Address{{Address: "a@b"}}, Subject: "s"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("down relay: %v", err)
	}
	if err := s.Probe(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("probe: %v", err)
	}
	if _, err := New(Config{Host: "h", From: "not an address"}); err == nil {
		t.Fatal("bad from")
	}
	if qEncode("a b=c?") != "a_b=3Dc=3F" {
		t.Fatalf("qencode: %s", qEncode("a b=c?"))
	}
}
