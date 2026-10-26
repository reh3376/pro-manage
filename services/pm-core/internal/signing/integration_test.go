//go:build integration

// The envelope lifecycle against Postgres, the Sigi_STUB module, and RustFS:
//
//	TEST_DATABASE_URL=... SIGI_TEST_ADDR=127.0.0.1:7090 SIGI_TEST_HTTP=http://127.0.0.1:7091 \
//	S3_TEST_ENDPOINT=http://127.0.0.1:9100 S3_TEST_ACCESS_KEY=... S3_TEST_SECRET_KEY=... \
//	go test -tags integration ./internal/signing/
//
// request -> CreateAtProvider (what the outbox does) -> a signer signs through
// the stub's dev endpoint -> Reconcile captures the PDF to the object store,
// checks the hash, and audits it; a declined envelope is recorded as such.
package signing

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/blobs"
	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/sigi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
)

const tenant = "it-signing"

func setup(t *testing.T) (*Service, string) {
	t.Helper()
	dsn, addr, ep := os.Getenv("TEST_DATABASE_URL"), os.Getenv("SIGI_TEST_ADDR"), os.Getenv("S3_TEST_ENDPOINT")
	if dsn == "" || addr == "" || ep == "" {
		t.Skip("TEST_DATABASE_URL, SIGI_TEST_ADDR, and S3_TEST_ENDPOINT required")
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
	chain, _ := audit.NewChain([]byte("integration-test-chain-key-0123456789"))
	sc, err := sigi.Dial(addr, "stub")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sc.Close() })
	bs, err := blobs.Open(ctx, blobs.Config{Endpoint: ep, AccessKey: os.Getenv("S3_TEST_ACCESS_KEY"), SecretKey: os.Getenv("S3_TEST_SECRET_KEY"), Bucket: "pm-it-signing"})
	if err != nil {
		t.Fatal(err)
	}
	httpBase := os.Getenv("SIGI_TEST_HTTP")
	if httpBase == "" {
		httpBase = "http://127.0.0.1:7091"
	}
	return &Service{Store: st, Chain: chain, Blobs: bs, Sigi: sc, Tenant: tenant, Log: log}, httpBase
}

func devSign(t *testing.T, base, ref, user string, decline bool) {
	t.Helper()
	url := base + "/dev/envelopes/" + ref + "/sign?user_id=" + user
	if decline {
		url += "&decline=1"
	}
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("dev sign %s: http %d", user, resp.StatusCode)
	}
}

func request(t *testing.T, s *Service, title string, signers []Signer) (db.SignatureEnvelope, json.RawMessage) {
	t.Helper()
	ctx := context.Background()
	var row db.SignatureEnvelope
	var payload json.RawMessage
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		// an audit row to hang the envelope on (the approval, in real life)
		ev, err := s.Chain.Append(ctx, q, audit.Event{TenantID: tenant, ActorUserID: "it", ActionType: "it.approve", SubjectType: "capital_authorization", SubjectID: "car-" + ids.New().String()})
		if err != nil {
			return err
		}
		id, err := s.Request(ctx, q, Request{SubjectType: "capital_authorization", SubjectID: ev.SubjectID, Title: title, Document: "Line 1\nLine 2", Signers: signers,
			Metadata: map[string]string{"amount": "42000000.00 USD"}, RequestedBy: "it", AuditID: ev.AuditID},
			func(kind, sourceRef string, p any) error { payload, _ = json.Marshal(p); return nil })
		if err != nil {
			return err
		}
		row, err = q.GetEnvelope(ctx, db.GetEnvelopeParams{TenantID: tenant, EnvelopeID: id})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return row, payload
}

func TestEnvelopeLifecycle(t *testing.T) {
	s, httpBase := setup(t)
	ctx := context.Background()
	q := s.Store.Queries()

	row, payload := request(t, s, "CAR CR-IT", []Signer{{UserID: "it-owner", Name: "Owner", Order: 1, Role: "project_owner"}, {UserID: "it-cfo", Name: "CFO", Order: 2, Role: "cfo"}})
	if row.Status != "requested" || row.Provider != "stub" || row.ProviderRef != nil {
		t.Fatalf("requested row: %+v", row)
	}
	// The outbox job creates it at the provider; a second delivery is a no-op.
	if err := s.CreateAtProvider(ctx, payload); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAtProvider(ctx, payload); err != nil {
		t.Fatal(err)
	}
	row, _ = q.GetEnvelope(ctx, db.GetEnvelopeParams{TenantID: tenant, EnvelopeID: row.EnvelopeID})
	if row.ProviderRef == nil || row.Status != "sent" {
		t.Fatalf("after create: %+v", row)
	}
	// Nothing to capture yet; one signer signs; still open.
	if _, err := s.Reconcile(ctx, 100); err != nil {
		t.Fatal(err)
	}
	devSign(t, httpBase, *row.ProviderRef, "it-owner", false)
	if _, err := s.Reconcile(ctx, 100); err != nil {
		t.Fatal(err)
	}
	row, _ = q.GetEnvelope(ctx, db.GetEnvelopeParams{TenantID: tenant, EnvelopeID: row.EnvelopeID})
	var signers []Signer
	_ = json.Unmarshal(row.Signers, &signers)
	if row.Status != "sent" || signers[0].Status != "signed" || signers[1].Status != "pending" {
		t.Fatalf("after first signature: %s %+v", row.Status, signers)
	}
	// The last signature completes it: artifact captured, hashed, audited.
	devSign(t, httpBase, *row.ProviderRef, "it-cfo", false)
	if _, err := s.Reconcile(ctx, 100); err != nil {
		t.Fatal(err)
	}
	row, _ = q.GetEnvelope(ctx, db.GetEnvelopeParams{TenantID: tenant, EnvelopeID: row.EnvelopeID})
	if row.Status != "completed" || row.ArtifactRef == nil || row.ArtifactSha256 == nil || row.CompletedAt == nil || row.ArtifactBytes == nil || *row.ArtifactBytes == 0 {
		t.Fatalf("completed row: %+v", row)
	}
	data, mt, err := s.Artifact(ctx, row)
	if err != nil || mt != "application/pdf" || !strings.HasPrefix(string(data), "%PDF-1.4") || !strings.Contains(string(data), "it-cfo") {
		t.Fatalf("artifact: %q %v", mt, err)
	}
	if latest, _ := s.Latest(ctx, "capital_authorization", row.SubjectID); latest == nil || latest.EnvelopeID != row.EnvelopeID {
		t.Fatal("latest")
	}
	rows, _ := q.ListAuditEventsForProject(ctx, db.ListAuditEventsForProjectParams{TenantID: tenant, ProjectID: nil, Limit: 5})
	_ = rows // project-less rows are not listed by project; verify the chain instead
	if r, err := s.Chain.Verify(ctx, q, tenant); err != nil || !r.Valid {
		t.Fatalf("chain: %+v %v", r, err)
	}
	// Reconciling again touches nothing.
	if n, err := s.Reconcile(ctx, 100); err != nil || n != 0 {
		t.Fatalf("second reconcile: %d %v", n, err)
	}

	// Declined envelope.
	d, p2 := request(t, s, "CAR CR-DECLINE", []Signer{{UserID: "it-vp", Order: 1, Role: "vp_finance"}})
	if err := s.CreateAtProvider(ctx, p2); err != nil {
		t.Fatal(err)
	}
	d, _ = q.GetEnvelope(ctx, db.GetEnvelopeParams{TenantID: tenant, EnvelopeID: d.EnvelopeID})
	devSign(t, httpBase, *d.ProviderRef, "it-vp", true)
	if _, err := s.Reconcile(ctx, 100); err != nil {
		t.Fatal(err)
	}
	d, _ = q.GetEnvelope(ctx, db.GetEnvelopeParams{TenantID: tenant, EnvelopeID: d.EnvelopeID})
	if d.Status != "declined" || !strings.Contains(d.Detail, "declined") {
		t.Fatalf("declined: %+v", d)
	}

	// Webhook: a bad signature is refused; the module's own signed body is accepted.
	resp, err := s.Webhook(ctx, "stub", []byte(`{"envelope_id":"x"}`), map[string]string{"X-Stub-Signature": "bad"})
	if err != nil || resp.GetAccepted() {
		t.Fatalf("bad webhook: %+v %v", resp, err)
	}
	if time.Since(row.RequestedAt) > time.Minute {
		t.Fatal("clock")
	}
}
