//go:build integration

// Against the dev stack's RustFS:
//
//	S3_TEST_ENDPOINT=http://127.0.0.1:9100 S3_TEST_ACCESS_KEY=... S3_TEST_SECRET_KEY=... go test -tags integration ./internal/blobs/
package blobs

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestPutGetRoundTrip(t *testing.T) {
	ep := os.Getenv("S3_TEST_ENDPOINT")
	if ep == "" {
		t.Skip("S3_TEST_ENDPOINT not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := Open(ctx, Config{Endpoint: ep, AccessKey: os.Getenv("S3_TEST_ACCESS_KEY"), SecretKey: os.Getenv("S3_TEST_SECRET_KEY"), Bucket: "pm-it-blobs"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	key := Key("envelopes", "it", time.Now().UTC().Format("20060102T150405.000")+".pdf")
	sha, err := st.Put(ctx, key, []byte("%PDF-1.4\nhello\n%%EOF\n"), "application/pdf")
	if err != nil || len(sha) != 64 {
		t.Fatalf("put: %s %v", sha, err)
	}
	data, mt, err := st.Get(ctx, key)
	if err != nil || string(data) != "%PDF-1.4\nhello\n%%EOF\n" || mt != "application/pdf" {
		t.Fatalf("get: %q %q %v", data, mt, err)
	}
	if _, _, err := st.Get(ctx, Key("envelopes", "it", "missing.pdf")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing key: %v", err)
	}
}
