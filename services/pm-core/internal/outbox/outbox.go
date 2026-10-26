// Package outbox implements the Postgres outbox (FSD §05 DF-01, §06 "Delivery").
//
// A domain transaction enqueues rows alongside its mutation and AuditEvent;
// pm-workers drains them with at-least-once delivery, exponential backoff, and
// a dead-letter status after MaxAttempts. Delivery is idempotent on
// (kind, source_ref), and the source_ref (the audit_id) travels with the
// payload so MDEMG can deduplicate on its side too.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/sigi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

// Kinds the drainer knows how to deliver.
const (
	KindMDEMGObserve  = "mdemg.observe"
	KindMDEMGFeedback = "mdemg.feedback"
	KindSigiCreate    = "sigi.envelope.create" // internal/signing.OutboxKind
)

// Statuses.
const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
)

// Enqueue writes one row inside the caller's transaction. A duplicate
// (kind, source_ref) is silently ignored, which is what makes retried
// requests safe.
func Enqueue(ctx context.Context, q *db.Queries, tenantID, kind, sourceRef string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("outbox: payload: %w", err)
	}
	_, err = q.EnqueueOutbox(ctx, db.EnqueueOutboxParams{OutboxID: ids.New(), TenantID: tenantID, Kind: kind, Payload: b, SourceRef: sourceRef})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // already enqueued
	}
	if err != nil {
		return fmt.Errorf("outbox: enqueue: %w", err)
	}
	return nil
}

// Sink is what the drainer delivers to; *mdemgadapter.Client satisfies it.
type Sink interface {
	Observe(ctx context.Context, req mdemgadapter.ObserveRequest) (mdemgadapter.ObserveResponse, error)
	Feedback(ctx context.Context, req mdemgadapter.FeedbackRequest) (mdemgadapter.FeedbackResponse, error)
}

// Signing creates e-signature envelopes at the provider (internal/signing); nil disables the kind.
type Signing interface {
	CreateAtProvider(ctx context.Context, payload json.RawMessage) error
}

// Drainer delivers pending rows.
type Drainer struct {
	Store       *store.Store
	Sink        Sink
	Signing     Signing
	Log         *slog.Logger
	Worker      string        // locked_by identity (hostname:pid)
	Batch       int32         // rows per claim (default 50)
	MaxAttempts int32         // dead-letter after this many failed attempts (default 10)
	BaseBackoff time.Duration // first retry delay (default 5 s)
	MaxBackoff  time.Duration // cap (default 10 min)

	delivered, retried, failed atomic.Int64
}

// Counters is a snapshot of the drainer's work since start.
type Counters struct {
	Delivered int64 `json:"delivered"`
	Retried   int64 `json:"retried"`
	Failed    int64 `json:"failed"`
}

// Counters returns the snapshot.
func (d *Drainer) Counters() Counters {
	return Counters{Delivered: d.delivered.Load(), Retried: d.retried.Load(), Failed: d.failed.Load()}
}

func (d *Drainer) defaults() {
	if d.Batch <= 0 {
		d.Batch = 50
	}
	if d.MaxAttempts <= 0 {
		d.MaxAttempts = 10
	}
	if d.BaseBackoff <= 0 {
		d.BaseBackoff = 5 * time.Second
	}
	if d.MaxBackoff <= 0 {
		d.MaxBackoff = 10 * time.Minute
	}
	if d.Worker == "" {
		d.Worker = "pm-workers"
	}
}

// Backoff returns the delay before the given attempt number (1-based), doubling from BaseBackoff and capped.
func (d *Drainer) Backoff(attempt int32) time.Duration {
	d.defaults()
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(float64(d.BaseBackoff) * math.Pow(2, float64(attempt-1)))
	if delay > d.MaxBackoff || delay <= 0 {
		delay = d.MaxBackoff
	}
	return delay
}

// Run drains on every tick until ctx is cancelled.
func (d *Drainer) Run(ctx context.Context, tick time.Duration) {
	d.defaults()
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		if n, err := d.Once(ctx); err != nil {
			d.Log.Error("outbox drain", "err", err)
		} else if n > 0 {
			d.Log.Info("outbox drained", "rows", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Once claims one batch and delivers it. Returns the number of rows handled.
func (d *Drainer) Once(ctx context.Context) (int, error) {
	d.defaults()
	now := time.Now().UTC()
	worker := d.Worker
	rows, err := d.Store.Queries().ClaimOutboxBatch(ctx, db.ClaimOutboxBatchParams{LockedAt: &now, Limit: d.Batch, LockedBy: &worker})
	if err != nil {
		return 0, fmt.Errorf("outbox: claim: %w", err)
	}
	for _, row := range rows {
		d.deliver(ctx, row)
	}
	return len(rows), nil
}

func (d *Drainer) deliver(ctx context.Context, row db.Outbox) {
	q := d.Store.Queries()
	err := d.dispatch(ctx, row)
	switch {
	case err == nil:
		now := time.Now().UTC()
		if e := q.MarkOutboxDelivered(ctx, db.MarkOutboxDeliveredParams{OutboxID: row.OutboxID, DeliveredAt: &now}); e != nil {
			d.Log.Error("outbox mark delivered", "outbox_id", row.OutboxID, "err", e)
			return
		}
		d.delivered.Add(1)
	case permanent(err) || row.Attempts+1 >= d.MaxAttempts:
		msg := err.Error()
		if e := q.MarkOutboxFailed(ctx, db.MarkOutboxFailedParams{OutboxID: row.OutboxID, LastError: &msg}); e != nil {
			d.Log.Error("outbox mark failed", "outbox_id", row.OutboxID, "err", e)
			return
		}
		d.failed.Add(1)
		d.Log.Error("outbox dead-lettered", "outbox_id", row.OutboxID, "kind", row.Kind, "attempts", row.Attempts+1, "err", msg)
	default:
		msg := err.Error()
		next := time.Now().UTC().Add(d.Backoff(row.Attempts + 1))
		if e := q.MarkOutboxRetry(ctx, db.MarkOutboxRetryParams{OutboxID: row.OutboxID, NextAttemptAt: next, LastError: &msg}); e != nil {
			d.Log.Error("outbox mark retry", "outbox_id", row.OutboxID, "err", e)
			return
		}
		d.retried.Add(1)
		d.Log.Warn("outbox retry scheduled", "outbox_id", row.OutboxID, "kind", row.Kind, "attempt", row.Attempts+1, "next", next.Format(time.RFC3339), "err", msg)
	}
}

// permanent reports errors that retrying cannot fix: a payload the target rejects or a bad key.
func permanent(err error) bool {
	return errors.Is(err, mdemgadapter.ErrBadRequest) || errors.Is(err, mdemgadapter.ErrUnauthorized) || errors.Is(err, sigi.ErrRejected) || errors.Is(err, errUnknownKind)
}

var errUnknownKind = errors.New("outbox: unknown kind")

func (d *Drainer) dispatch(ctx context.Context, row db.Outbox) error {
	ctx = mdemgadapter.WithRequestID(ctx, "outbox:"+row.OutboxID.String())
	switch row.Kind {
	case KindMDEMGObserve:
		var req mdemgadapter.ObserveRequest
		if err := json.Unmarshal(row.Payload, &req); err != nil {
			return fmt.Errorf("%w: %v", mdemgadapter.ErrBadRequest, err)
		}
		if req.SourceRef == "" {
			req.SourceRef = row.SourceRef
		}
		_, err := d.Sink.Observe(ctx, req)
		return err
	case KindMDEMGFeedback:
		var req mdemgadapter.FeedbackRequest
		if err := json.Unmarshal(row.Payload, &req); err != nil {
			return fmt.Errorf("%w: %v", mdemgadapter.ErrBadRequest, err)
		}
		_, err := d.Sink.Feedback(ctx, req)
		return err
	case KindSigiCreate:
		if d.Signing == nil {
			return errors.New("outbox: no signing service configured (retrying)")
		}
		return d.Signing.CreateAtProvider(ctx, row.Payload)
	default:
		return fmt.Errorf("%w: %s", errUnknownKind, row.Kind)
	}
}

// Stats summarizes the table for the API.
type Stats struct {
	Rows      []db.OutboxStatsRow `json:"rows"`
	Pending   int64               `json:"pending"`
	Delivered int64               `json:"delivered"`
	Failed    int64               `json:"failed"`
}

// StatsFor reads the per-kind/status counts.
func StatsFor(ctx context.Context, q *db.Queries, tenantID string) (Stats, error) {
	rows, err := q.OutboxStats(ctx, tenantID)
	if err != nil {
		return Stats{}, fmt.Errorf("outbox: stats: %w", err)
	}
	s := Stats{Rows: rows}
	if s.Rows == nil {
		s.Rows = []db.OutboxStatsRow{}
	}
	for _, r := range rows {
		switch r.Status {
		case StatusPending:
			s.Pending += r.N
		case StatusDelivered:
			s.Delivered += r.N
		case StatusFailed:
			s.Failed += r.N
		}
	}
	return s, nil
}
