package envelopes

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func TestLifecycleAndArtifact(t *testing.T) {
	s := New()
	e, dup := s.Create(Envelope{IdempotencyKey: "audit-1", SubjectType: "capital_authorization", SubjectID: "car-1", Title: "CAR CR-1",
		Signers:  []Signer{{UserID: "evan.exec", Name: "Evan", Order: 2, Role: "cfo"}, {UserID: "alice.owner", Name: "Alice", Order: 1, Role: "project_owner"}},
		Metadata: map[string]string{"amount": "42000000.00 USD"}})
	if dup || e.Status != Sent || e.Signers[0].UserID != "alice.owner" || e.ID == "" {
		t.Fatalf("create: %+v", e)
	}
	if again, dup := s.Create(Envelope{IdempotencyKey: "audit-1"}); !dup || again.ID != e.ID {
		t.Fatal("idempotency key must return the same envelope")
	}
	if _, err := s.Sign("nope", "", false); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown envelope")
	}
	if _, err := s.Sign(e.ID, "stranger", false); !errors.Is(err, ErrNoSigner) {
		t.Fatal("stranger cannot sign")
	}
	got, err := s.Sign(e.ID, "alice.owner", false)
	if err != nil || got.Status != Sent || got.Signers[0].Status != "signed" || got.Artifact != nil {
		t.Fatalf("first signature: %+v %v", got, err)
	}
	got, err = s.Sign(e.ID, "", false) // next pending signer
	if err != nil || got.Status != Completed || got.CompletedAt.IsZero() || len(got.Artifact) == 0 || got.ArtifactSHA256 == "" {
		t.Fatalf("completion: %s %v", got.Status, err)
	}
	sum := sha256.Sum256(got.Artifact)
	if hex.EncodeToString(sum[:]) != got.ArtifactSHA256 {
		t.Fatal("artifact hash mismatch")
	}
	if !bytes.HasPrefix(got.Artifact, []byte("%PDF-1.4")) || !bytes.Contains(got.Artifact, []byte("%%EOF")) || !bytes.Contains(got.Artifact, []byte("evan.exec")) {
		t.Fatalf("artifact is not the expected PDF: %.80s", got.Artifact)
	}
	if _, err := s.Sign(e.ID, "", false); !errors.Is(err, ErrClosed) {
		t.Fatal("completed envelope cannot be signed again")
	}
	if _, err := s.Cancel(e.ID, "late"); !errors.Is(err, ErrClosed) {
		t.Fatal("completed envelope cannot be voided")
	}

	d, _ := s.Create(Envelope{Signers: []Signer{{UserID: "bob", Order: 1}}})
	if got, err := s.Sign(d.ID, "bob", true); err != nil || got.Status != Declined {
		t.Fatalf("decline: %+v %v", got, err)
	}
	v, _ := s.Create(Envelope{Signers: []Signer{{UserID: "bob", Order: 1}}})
	if got, err := s.Cancel(v.ID, "changed my mind"); err != nil || got.Status != Voided || got.Detail != "changed my mind" {
		t.Fatalf("void: %+v %v", got, err)
	}
	none, _ := s.Create(Envelope{Title: "no signers"})
	if none.Status != Completed || len(none.Artifact) == 0 {
		t.Fatal("an envelope without signers completes immediately")
	}
	if s.Len() != 4 {
		t.Fatalf("len %d", s.Len())
	}
}

func TestPDFEscapes(t *testing.T) {
	pdf := minimalPDF([]string{"a (b) \\ c", "ünïcode"})
	if !bytes.Contains(pdf, []byte(`(a \(b\) \\ c) Tj`)) || !bytes.Contains(pdf, []byte("(?n?code) Tj")) {
		t.Fatalf("%s", pdf)
	}
}
