package outbox

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

func TestBackoffDoublesAndCaps(t *testing.T) {
	d := &Drainer{BaseBackoff: time.Second, MaxBackoff: 10 * time.Second}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second, 10 * time.Second}
	for i, w := range want {
		if got := d.Backoff(int32(i + 1)); got != w {
			t.Fatalf("attempt %d: got %v want %v", i+1, got, w)
		}
	}
	if d.Backoff(0) != time.Second {
		t.Fatal("attempt 0 should behave as 1")
	}
}

func TestPermanentErrors(t *testing.T) {
	cases := map[error]bool{
		mdemgadapter.ErrBadRequest:             true,
		mdemgadapter.ErrUnauthorized:           true,
		fmt.Errorf("wrap: %w", errUnknownKind): true,
		mdemgadapter.ErrUnavailable:            false,
		errors.New("network"):                  false,
		&mdemgadapter.Error{Sentinel: mdemgadapter.ErrUnavailable, Status: 503}: false,
		&mdemgadapter.Error{Sentinel: mdemgadapter.ErrBadRequest, Status: 400}:  true,
	}
	for err, want := range cases {
		if got := permanent(err); got != want {
			t.Fatalf("%v: permanent=%v want %v", err, got, want)
		}
	}
}
