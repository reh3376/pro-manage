package governance

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

func TestRedisPreclassRoundTripScopeAndTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c := NewRedisPreclass(rdb, 10*time.Minute, nil)
	d := mdemgadapter.Decision{Verdict: "pass", ViolatedCodes: []string{}}
	c.Put("alice", "p1", "k1", d)
	got, ok := c.Get("alice", "p1", "k1")
	if !ok || got.Verdict != "pass" {
		t.Fatalf("round trip: ok=%v %+v", ok, got)
	}
	// Scoped to user and project: another user or project misses.
	if _, ok := c.Get("bob", "p1", "k1"); ok {
		t.Fatal("verdict leaked across users")
	}
	if _, ok := c.Get("alice", "p2", "k1"); ok {
		t.Fatal("verdict leaked across projects")
	}
	if c.Len() != 1 {
		t.Fatalf("len: %d", c.Len())
	}
	// The TTL is the key's expiry.
	mr.FastForward(11 * time.Minute)
	if _, ok := c.Get("alice", "p1", "k1"); ok {
		t.Fatal("verdict survived its TTL")
	}
}

func TestRedisPreclassDegradesToMiss(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c := NewRedisPreclass(rdb, time.Minute, nil)
	c.Put("alice", "p1", "k1", mdemgadapter.Decision{Verdict: "deny"})
	mr.Close() // Redis goes away
	if _, ok := c.Get("alice", "p1", "k1"); ok {
		t.Fatal("a dead Redis must read as a miss, not a verdict")
	}
	c.Put("alice", "p1", "k2", mdemgadapter.Decision{Verdict: "pass"}) // must not panic
}

// The gate accepts either store.
var _ PreclassStore = (*PreclassCache)(nil)
var _ PreclassStore = (*RedisPreclass)(nil)
