package audit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
)

func testKey() []byte { return []byte("0123456789abcdef0123456789abcdef") }

func TestNewChainRejectsShortKey(t *testing.T) {
	if _, err := NewChain([]byte("short")); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestCanonicalIsStableUnderJSONKeyOrder(t *testing.T) {
	id := uuid.MustParse("01900000-0000-7000-8000-000000000001")
	at := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	a := db.AuditEvent{AuditID: id, TenantID: "default", ActorUserID: "alice", ActionType: "site.create", SubjectType: "site", SubjectID: "s1",
		AfterState: json.RawMessage(`{"name":"NF-KY","code":"NF-KY"}`), At: at, PrevHash: GenesisHash}
	b := a
	b.AfterState = json.RawMessage(`{ "code": "NF-KY", "name": "NF-KY" }`)
	ca, err := canonicalBytes(a)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := canonicalBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(ca) != string(cb) {
		t.Fatalf("canonical differs by key order:\n%s\n%s", ca, cb)
	}
	c, _ := NewChain(testKey())
	if c.sign(GenesisHash, ca) != c.sign(GenesisHash, cb) {
		t.Fatal("signature differs by key order")
	}
}

func TestSignatureChangesWithContentAndPrev(t *testing.T) {
	c, _ := NewChain(testKey())
	p1 := []byte(`{"a":1}`)
	p2 := []byte(`{"a":2}`)
	if c.sign(GenesisHash, p1) == c.sign(GenesisHash, p2) {
		t.Fatal("different payloads must sign differently")
	}
	if c.sign(GenesisHash, p1) == c.sign("ff", p1) {
		t.Fatal("different prev hashes must sign differently")
	}
	if len(c.sign(GenesisHash, p1)) != 64 {
		t.Fatal("expected hex sha256 length")
	}
}

func TestMarshalStateNormalizes(t *testing.T) {
	got, err := marshalState(map[string]any{"z": 1, "a": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a":"x","z":1}` {
		t.Fatalf("got %s", got)
	}
	nilState, err := marshalState(nil)
	if err != nil || nilState != nil {
		t.Fatalf("nil state should stay nil, got %v %v", nilState, err)
	}
}
