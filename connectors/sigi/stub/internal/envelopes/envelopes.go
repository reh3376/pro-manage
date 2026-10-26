// Package envelopes is the stub's in-memory envelope store: create, sign (through
// the dev endpoint), status, cancel, and the signed artifact (a minimal PDF
// with a SHA-256). Everything a provider would do, without a provider.
package envelopes

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Statuses (the proto enum names without the prefix).
const (
	Created   = "created"
	Sent      = "sent"
	Completed = "completed"
	Declined  = "declined"
	Voided    = "voided"
)

// Signer is one party.
type Signer struct {
	UserID   string
	Email    string
	Name     string
	Order    int32
	Role     string
	Status   string // pending | signed | declined
	SignedAt time.Time
}

// Envelope is one signing request.
type Envelope struct {
	ID             string
	IdempotencyKey string
	SubjectType    string
	SubjectID      string
	Title          string
	Document       []byte
	DocumentType   string
	Signers        []Signer
	Metadata       map[string]string
	Status         string
	CreatedAt      time.Time
	CompletedAt    time.Time
	Artifact       []byte
	ArtifactSHA256 string
	Detail         string
}

// Errors.
var (
	ErrNotFound = errors.New("envelope not found")
	ErrClosed   = errors.New("envelope is not open")
	ErrNoSigner = errors.New("no such pending signer")
)

// Store is safe for concurrent use.
type Store struct {
	mu     sync.Mutex
	byID   map[string]*Envelope
	byKey  map[string]string
	ticker *time.Ticker
}

// New returns an empty store.
func New() *Store { return &Store{byID: map[string]*Envelope{}, byKey: map[string]string{}} }

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "stub-env-" + hex.EncodeToString(b)
}

// Create registers an envelope; the same idempotency key returns the same envelope.
func (s *Store) Create(e Envelope) (*Envelope, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.IdempotencyKey != "" {
		if id, ok := s.byKey[e.IdempotencyKey]; ok {
			return s.byID[id], true
		}
	}
	e.ID = newID()
	e.Status = Sent // the stub "sends" immediately
	e.CreatedAt = time.Now().UTC()
	for i := range e.Signers {
		e.Signers[i].Status = "pending"
	}
	sort.SliceStable(e.Signers, func(i, j int) bool { return e.Signers[i].Order < e.Signers[j].Order })
	if len(e.Signers) == 0 {
		e.Status = Completed
		e.CompletedAt = e.CreatedAt
		e.Artifact, e.ArtifactSHA256 = render(&e)
	}
	s.byID[e.ID] = &e
	if e.IdempotencyKey != "" {
		s.byKey[e.IdempotencyKey] = e.ID
	}
	return &e, false
}

// Get returns a copy.
func (s *Store) Get(id string) (Envelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byID[id]
	if !ok {
		return Envelope{}, ErrNotFound
	}
	return *e, nil
}

// Sign marks a signer signed (or declined); the envelope completes when every
// signer has signed, and the artifact is rendered then.
func (s *Store) Sign(id, userID string, decline bool) (Envelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byID[id]
	if !ok {
		return Envelope{}, ErrNotFound
	}
	if e.Status != Sent && e.Status != Created {
		return *e, ErrClosed
	}
	found := false
	for i := range e.Signers {
		sg := &e.Signers[i]
		if sg.Status != "pending" || (userID != "" && sg.UserID != userID) {
			continue
		}
		found = true
		sg.SignedAt = time.Now().UTC()
		if decline {
			sg.Status = "declined"
			e.Status = Declined
			e.Detail = "declined by " + sg.UserID
			return *e, nil
		}
		sg.Status = "signed"
		break
	}
	if !found {
		return *e, ErrNoSigner
	}
	done := true
	for _, sg := range e.Signers {
		if sg.Status != "signed" {
			done = false
		}
	}
	if done {
		e.Status = Completed
		e.CompletedAt = time.Now().UTC()
		e.Artifact, e.ArtifactSHA256 = render(e)
	}
	return *e, nil
}

// Cancel voids an open envelope.
func (s *Store) Cancel(id, reason string) (Envelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byID[id]
	if !ok {
		return Envelope{}, ErrNotFound
	}
	if e.Status == Completed {
		return *e, ErrClosed
	}
	e.Status = Voided
	e.Detail = reason
	return *e, nil
}

// Len is for /stats.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byID)
}

// render produces the signed artifact: a minimal, valid single-page PDF whose
// text lists the title, subject, and every signature with its time, plus the
// SHA-256 of those bytes. Good enough to open in a viewer and to hash.
func render(e *Envelope) ([]byte, string) {
	var lines []string
	lines = append(lines, "pro_manage signed record (Sigi_STUB, development only)")
	lines = append(lines, "Title: "+e.Title)
	lines = append(lines, fmt.Sprintf("Subject: %s %s", e.SubjectType, e.SubjectID))
	lines = append(lines, "Envelope: "+e.ID)
	if e.DocumentType == "text/plain" && len(e.Document) > 0 {
		lines = append(lines, "")
		for _, l := range strings.Split(string(e.Document), "\n") {
			lines = append(lines, l)
		}
	}
	lines = append(lines, "", "Signatures:")
	for _, sg := range e.Signers {
		lines = append(lines, fmt.Sprintf("  %s (%s, %s) signed %s", sg.Name, sg.UserID, sg.Role, sg.SignedAt.UTC().Format(time.RFC3339)))
	}
	keys := make([]string, 0, len(e.Metadata))
	for k := range e.Metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		lines = append(lines, "", "Metadata:")
		for _, k := range keys {
			lines = append(lines, "  "+k+": "+e.Metadata[k])
		}
	}
	pdf := minimalPDF(lines)
	sum := sha256.Sum256(pdf)
	return pdf, hex.EncodeToString(sum[:])
}

// minimalPDF writes a one-page PDF with Helvetica text, one line per entry.
func minimalPDF(lines []string) []byte {
	var content bytes.Buffer
	content.WriteString("BT /F1 10 Tf 40 780 Td 13 TL\n")
	for i, l := range lines {
		if i > 55 {
			break // one page is plenty for the stub
		}
		content.WriteString("(" + pdfEscape(l) + ") Tj T*\n")
	}
	content.WriteString("ET\n")

	var out bytes.Buffer
	offsets := make([]int, 0, 6)
	obj := func(body string) {
		offsets = append(offsets, out.Len())
		out.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", len(offsets), body))
	}
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	obj("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	obj(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", content.Len(), content.String()))
	xref := out.Len()
	out.WriteString(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1))
	for _, o := range offsets {
		out.WriteString(fmt.Sprintf("%010d 00000 n \n", o))
	}
	out.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref))
	return out.Bytes()
}

func pdfEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`)
	var b strings.Builder
	for _, c := range r.Replace(s) {
		if c < 32 || c > 126 {
			b.WriteByte('?')
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}
