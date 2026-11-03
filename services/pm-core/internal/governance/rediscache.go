package governance

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

// PreclassStore is what the gate needs from a pre-classification cache (§06:
// a verdict for a settled form, scoped to user and project, with a TTL). The
// in-process PreclassCache serves one replica; RedisPreclass serves many, so a
// verdict pre-classified on one pm-core instance is honoured by the one that
// takes the submit (backlog B-06).
type PreclassStore interface {
	Get(user, project, key string) (mdemgadapter.Decision, bool)
	Put(user, project, key string, d mdemgadapter.Decision)
}

// RedisPreclass keeps verdicts in the stack's Redis under pm:preclass:<user>|<project>|<key>
// with the TTL as the key's expiry. Redis trouble degrades to "not cached": a
// miss re-classifies, a failed put is logged, and the submit still proceeds.
type RedisPreclass struct {
	rdb     *redis.Client
	TTL     time.Duration
	Prefix  string
	Log     *slog.Logger
	Timeout time.Duration
}

// NewRedisPreclass wraps a client; a zero ttl is the §06 default of 10 minutes.
func NewRedisPreclass(rdb *redis.Client, ttl time.Duration, log *slog.Logger) *RedisPreclass {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if log == nil {
		log = slog.Default()
	}
	return &RedisPreclass{rdb: rdb, TTL: ttl, Prefix: "pm:preclass:", Log: log, Timeout: 500 * time.Millisecond}
}

func (c *RedisPreclass) key(user, project, key string) string {
	return c.Prefix + scoped(user, project, key)
}

func (c *RedisPreclass) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), c.Timeout)
}

// Get returns a live verdict.
func (c *RedisPreclass) Get(user, project, key string) (mdemgadapter.Decision, bool) {
	ctx, cancel := c.ctx()
	defer cancel()
	raw, err := c.rdb.Get(ctx, c.key(user, project, key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return mdemgadapter.Decision{}, false
	}
	if err != nil {
		c.Log.Warn("preclass cache get", "err", err)
		return mdemgadapter.Decision{}, false
	}
	var d mdemgadapter.Decision
	if err := json.Unmarshal(raw, &d); err != nil {
		c.Log.Warn("preclass cache decode", "err", err)
		return mdemgadapter.Decision{}, false
	}
	return d, true
}

// Put stores a verdict for the TTL.
func (c *RedisPreclass) Put(user, project, key string, d mdemgadapter.Decision) {
	b, err := json.Marshal(d)
	if err != nil {
		c.Log.Warn("preclass cache encode", "err", err)
		return
	}
	ctx, cancel := c.ctx()
	defer cancel()
	if err := c.rdb.Set(ctx, c.key(user, project, key), b, c.TTL).Err(); err != nil {
		c.Log.Warn("preclass cache put", "err", err)
	}
}

// Len counts the cached verdicts (tests and /stats); a scan, so not for hot paths.
func (c *RedisPreclass) Len() int {
	ctx, cancel := c.ctx()
	defer cancel()
	n := 0
	iter := c.rdb.Scan(ctx, 0, c.Prefix+"*", 1000).Iterator()
	for iter.Next(ctx) {
		n++
	}
	return n
}
