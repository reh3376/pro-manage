//go:build integration

// Notifications end to end: enqueue inside a transaction, drain through the
// Commsi_SMTP module, and find the mail in Mailpit.
//
//	TEST_DATABASE_URL=... COMMSI_TEST_ADDR=127.0.0.1:7100 MAILPIT_TEST_URL=http://127.0.0.1:8026 \
//	go test -tags integration ./internal/notify/
package notify

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/commsi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/outbox"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

const tenant = "it-notify"

type mailpitList struct {
	Total    int `json:"total"`
	Messages []struct {
		ID      string `json:"ID"`
		Subject string `json:"Subject"`
		To      []struct {
			Address string `json:"Address"`
		} `json:"To"`
	} `json:"messages"`
}

func search(t *testing.T, base, q string) mailpitList {
	t.Helper()
	resp, err := http.Get(base + "/api/v1/search?query=" + url.QueryEscape(q))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out mailpitList
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestEnqueueDrainDeliver(t *testing.T) {
	dsn, addr, mp := os.Getenv("TEST_DATABASE_URL"), os.Getenv("COMMSI_TEST_ADDR"), os.Getenv("MAILPIT_TEST_URL")
	if dsn == "" || addr == "" || mp == "" {
		t.Skip("TEST_DATABASE_URL, COMMSI_TEST_ADDR, and MAILPIT_TEST_URL required")
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
	defer st.Close()
	cc, err := commsi.Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer cc.Close()
	if err := cc.Health(ctx); err != nil {
		t.Fatalf("commsi health: %v", err)
	}

	n := &Notifier{Tenant: tenant, FallbackDomain: "it.example"}
	marker := "IT-" + strings.ToUpper(ids.New().String()[24:])
	// One recipient has a users row with an email; one falls back to the domain; one is skipped (no fallback would skip, here none).
	err = st.Tx(ctx, func(q *db.Queries) error {
		if _, err := q.UpsertUserOnLogin(ctx, db.UpsertUserOnLoginParams{UserID: ids.New(), TenantID: tenant, ExternalID: "sub-" + marker, Username: "it-known-" + marker, Email: "known-" + marker + "@users.example", DisplayName: "Known User", PlatformRoles: []string{}, Attributes: []byte("{}")}); err != nil {
			return err
		}
		sent, skipped, err := n.Enqueue(ctx, q, Notification{Kind: KindApprovalRequested, SubjectType: "capital_authorization", SubjectID: marker, Recipients: []string{"it-known-" + marker, "it-fallback-" + marker, "it-known-" + marker},
			Subject: "CAR " + marker + " awaits your decision", Body: "Stage 1 of 2.", DeepLink: "/cars/" + marker})
		if err != nil || sent != 2 || len(skipped) != 0 {
			t.Fatalf("enqueue: sent=%d skipped=%v err=%v", sent, skipped, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Without a fallback domain an unknown handle is skipped and a notification with no one left is not queued.
	strict := &Notifier{Tenant: tenant}
	err = st.Tx(ctx, func(q *db.Queries) error {
		sent, skipped, err := strict.Enqueue(ctx, q, Notification{Kind: KindApprovalNudge, SubjectType: "x", SubjectID: "y", Recipients: []string{"nobody-" + marker}, Subject: "s"})
		if err != nil || sent != 0 || len(skipped) != 1 {
			t.Fatalf("strict: sent=%d skipped=%v err=%v", sent, skipped, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	d := &outbox.Drainer{Store: st, Sink: noMDEMG{}, Notify: cc, Log: log, Worker: "it-notify", BaseBackoff: time.Second, MaxAttempts: 3}
	if _, err := d.Once(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		list := search(t, mp, marker)
		if list.Total >= 1 {
			msg := list.Messages[0]
			if !strings.Contains(msg.Subject, marker) || len(msg.To) != 2 {
				t.Fatalf("mail: %+v", msg)
			}
			addrs := msg.To[0].Address + " " + msg.To[1].Address
			if !strings.Contains(addrs, "known-"+marker+"@users.example") || !strings.Contains(addrs, "it-fallback-"+marker+"@it.example") {
				t.Fatalf("recipients: %s", addrs)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("mail with %s not found in Mailpit", marker)
		}
		time.Sleep(500 * time.Millisecond)
	}
	// A second row with the same dedupe key inside the window is deduped by the module, not an error.
	err = st.Tx(ctx, func(q *db.Queries) error {
		_, _, err := n.Enqueue(ctx, q, Notification{Kind: KindApprovalRequested, SubjectType: "capital_authorization", SubjectID: marker, Recipients: []string{"it-known-" + marker, "it-fallback-" + marker}, Subject: "CAR " + marker + " awaits your decision", DedupeKey: KindApprovalRequested + ":capital_authorization:" + marker + ":it-known-" + marker + ",it-fallback-" + marker})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if c := d.Counters(); c.Failed != 0 {
		t.Fatalf("deliveries failed: %+v", c)
	}
}

// noMDEMG is a sink that must never be called in this test.
type noMDEMG struct{}

func (noMDEMG) Observe(context.Context, mdemgadapter.ObserveRequest) (mdemgadapter.ObserveResponse, error) {
	panic("unexpected observe")
}

func (noMDEMG) Feedback(context.Context, mdemgadapter.FeedbackRequest) (mdemgadapter.FeedbackResponse, error) {
	panic("unexpected feedback")
}
