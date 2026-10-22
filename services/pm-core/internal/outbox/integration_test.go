//go:build integration

// Drains real outbox rows against an MDEMG-shaped endpoint (the stub in CI):
//
//	TEST_DATABASE_URL=... MDEMG_TEST_URL=http://127.0.0.1:9997 MDEMG_TEST_KEY=ci-stub-key go test -tags integration ./internal/outbox/
package outbox

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

const tenant = "it-outbox"

func setup(t *testing.T) (*store.Store, *slog.Logger) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("MDEMG_TEST_URL") == "" {
		t.Skip("TEST_DATABASE_URL and MDEMG_TEST_URL required")
	}
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := store.Migrate(ctx, log, dsn); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	return st, log
}

func enqueue(t *testing.T, st *store.Store, kind string, payload any) string {
	t.Helper()
	ref := "it-" + ids.New().String()
	if err := st.Tx(context.Background(), func(q *db.Queries) error { return Enqueue(context.Background(), q, tenant, kind, ref, payload) }); err != nil {
		t.Fatal(err)
	}
	return ref
}

func rowByRef(t *testing.T, st *store.Store, kind, ref string) db.Outbox {
	t.Helper()
	var row db.Outbox
	err := st.Pool.QueryRow(context.Background(), "SELECT outbox_id FROM outbox WHERE kind=$1 AND source_ref=$2", kind, ref).Scan(&row.OutboxID)
	if err != nil {
		t.Fatal(err)
	}
	row, err = st.Queries().GetOutbox(context.Background(), row.OutboxID)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestDeliverRetryAndDeadLetter(t *testing.T) {
	st, log := setup(t)
	ctx := context.Background()
	good, _ := mdemgadapter.New(mdemgadapter.Options{BaseURL: os.Getenv("MDEMG_TEST_URL"), APIKey: os.Getenv("MDEMG_TEST_KEY"), WriteTimeout: 30 * time.Second})
	dead, _ := mdemgadapter.New(mdemgadapter.Options{BaseURL: "http://127.0.0.1:1", WriteTimeout: 300 * time.Millisecond})

	// 1. Delivered on the first attempt.
	okRef := enqueue(t, st, KindMDEMGObserve, mdemgadapter.ObserveRequest{SpaceID: "proj_itoutbox", SessionID: "it", Content: "outbox integration observation", ObsType: "decision"})
	// Duplicate enqueue of the same source_ref is a no-op.
	if err := st.Tx(ctx, func(q *db.Queries) error {
		return Enqueue(ctx, q, tenant, KindMDEMGObserve, okRef, map[string]any{"ignored": true})
	}); err != nil {
		t.Fatal(err)
	}
	// 2. Permanent failure: MDEMG rejects an observe without content (400) -> failed immediately.
	badRef := enqueue(t, st, KindMDEMGObserve, mdemgadapter.ObserveRequest{SpaceID: "proj_itoutbox", Content: "", ObsType: "decision"})
	// 3. Unknown kind -> failed immediately.
	unkRef := enqueue(t, st, "nobody.handles.this", map[string]any{"x": 1})

	d := &Drainer{Store: st, Sink: good, Log: log, Worker: "it", BaseBackoff: time.Second, MaxAttempts: 3}
	if _, err := d.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if r := rowByRef(t, st, KindMDEMGObserve, okRef); r.Status != StatusDelivered || r.DeliveredAt == nil {
		t.Fatalf("expected delivered: %+v", r)
	}
	if r := rowByRef(t, st, KindMDEMGObserve, badRef); r.Status != StatusFailed || r.LastError == nil {
		t.Fatalf("expected failed (permanent): %+v", r)
	}
	if r := rowByRef(t, st, "nobody.handles.this", unkRef); r.Status != StatusFailed {
		t.Fatalf("expected failed (unknown kind): %+v", r)
	}

	// 4. Transient failure: unreachable sink -> retry with backoff, then dead-letter at MaxAttempts.
	retryRef := enqueue(t, st, KindMDEMGFeedback, mdemgadapter.FeedbackRequest{GuidanceID: "gd-it", ActionSummary: "x", SpaceID: "proj_itoutbox", Outcome: mdemgadapter.OutcomeFollowed})
	dd := &Drainer{Store: st, Sink: dead, Log: log, Worker: "it-dead", BaseBackoff: 10 * time.Millisecond, MaxBackoff: 20 * time.Millisecond, MaxAttempts: 3}
	if _, err := dd.Once(ctx); err != nil {
		t.Fatal(err)
	}
	r := rowByRef(t, st, KindMDEMGFeedback, retryRef)
	if r.Status != StatusPending || r.Attempts != 1 || r.LastError == nil || !r.NextAttemptAt.After(time.Now().Add(-time.Second)) {
		t.Fatalf("expected one retry scheduled: %+v", r)
	}
	for i := 0; i < 4; i++ { // the row becomes due after the tiny backoff; keep draining until dead-lettered
		time.Sleep(30 * time.Millisecond)
		if _, err := dd.Once(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if r = rowByRef(t, st, KindMDEMGFeedback, retryRef); r.Status != StatusFailed || r.Attempts != 3 {
		t.Fatalf("expected dead-letter after 3 attempts: %+v", r)
	}
	c := dd.Counters()
	if c.Retried < 2 || c.Failed != 1 {
		t.Fatalf("counters: %+v", c)
	}

	// 5. Requeue failed rows for the tenant and deliver the feedback one with the good sink.
	n, err := st.Queries().RequeueOutboxFailed(ctx, db.RequeueOutboxFailedParams{TenantID: tenant, Kind: ptr(KindMDEMGFeedback)})
	if err != nil || n < 1 {
		t.Fatalf("requeue: %v %d", err, n)
	}
	if _, err := d.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if r = rowByRef(t, st, KindMDEMGFeedback, retryRef); r.Status != StatusDelivered {
		t.Fatalf("expected delivered after requeue: %+v", r)
	}
	s, err := StatsFor(ctx, st.Queries(), tenant)
	if err != nil || s.Delivered < 2 || s.Failed < 2 {
		t.Fatalf("stats: %v %+v", err, s)
	}
}

func ptr(s string) *string { return &s }
