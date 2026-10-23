package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	st := New(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	t.Cleanup(func() { _ = st.Close() })
	return st, mr
}

func TestPendingIsRedeemedOnce(t *testing.T) {
	st, mr := newStore(t)
	ctx := context.Background()
	if err := st.PutPending(ctx, Pending{State: "s1", Nonce: "n", Verifier: "v", RedirectURI: "http://x/cb", ReturnTo: "/"}); err != nil {
		t.Fatal(err)
	}
	p, err := st.TakePending(ctx, "s1")
	if err != nil || p.Verifier != "v" || p.CreatedAt.IsZero() {
		t.Fatalf("take: %+v %v", p, err)
	}
	if _, err := st.TakePending(ctx, "s1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second take must fail: %v", err)
	}
	if _, err := st.TakePending(ctx, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("empty state")
	}
	if err := st.PutPending(ctx, Pending{}); err == nil {
		t.Fatal("pending without state must fail")
	}
	_ = st.PutPending(ctx, Pending{State: "s2"})
	mr.FastForward(DefaultPendingTTL + time.Second)
	if _, err := st.TakePending(ctx, "s2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired pending must be gone: %v", err)
	}
}

func TestSessionSlidingWindowAndCap(t *testing.T) {
	st, mr := newStore(t)
	st.TTL, st.MaxAge = time.Hour, 3*time.Hour
	ctx := context.Background()
	sess, err := st.Create(ctx, Session{Username: "alice.owner", Subject: "sub-1", PlatformRoles: []string{}, Pools: []string{"finance_approver_eligible"}, RefreshToken: "rt"})
	if err != nil || len(sess.ID) != 43 || sess.ExpiresAt.Sub(sess.CreatedAt) != 3*time.Hour {
		t.Fatalf("create: %+v %v", sess, err)
	}
	got, err := st.Get(ctx, sess.ID)
	if err != nil || got.Username != "alice.owner" || got.RefreshToken != "rt" || got.Pools[0] != "finance_approver_eligible" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if ttl := mr.TTL(st.sessionKey(sess.ID)); ttl != time.Hour {
		t.Fatalf("initial ttl %v", ttl)
	}
	// 50 minutes of silence (last_seen in the past, key TTL run down), then a request: the window slides back to a full hour.
	got.LastSeenAt = time.Now().Add(-50 * time.Minute)
	if err := st.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(50 * time.Minute)
	if _, err := st.Get(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if ttl := mr.TTL(st.sessionKey(sess.ID)); ttl < 59*time.Minute {
		t.Fatalf("window did not slide: %v", ttl)
	}
	// Idle past the window: gone.
	mr.FastForward(61 * time.Minute)
	if _, err := st.Get(ctx, sess.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("idle session must expire: %v", err)
	}
	// The absolute cap wins over the sliding window.
	old, _ := st.Create(ctx, Session{Username: "bob.finance"})
	old.ExpiresAt = time.Now().Add(-time.Second)
	_ = st.Update(ctx, old)
	if _, err := st.Get(ctx, old.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("capped session must be refused: %v", err)
	}
	if err := st.Delete(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get(ctx, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("empty id")
	}
}
