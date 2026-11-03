package calendar

import (
	"testing"
	"time"
)

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestAddSkipsWeekendsAndHolidays(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	c := New("us-ky", "Kentucky", "America/New_York", []int{6, 7}, []string{"2026-11-26"}) // Thanksgiving
	// Friday 2026-11-20 15:00 ET plus 24 business hours: the remaining 9 h of Friday, then
	// Saturday and Sunday are skipped, then 15 h of Monday -> Monday 2026-11-23 15:00 ET.
	start := time.Date(2026, 11, 20, 15, 0, 0, 0, ny)
	got := c.Add(start, 24*time.Hour)
	want := time.Date(2026, 11, 23, 15, 0, 0, 0, ny)
	if !got.Equal(want) {
		t.Fatalf("Friday + 24h: got %s, want %s", got, want)
	}
	// Wednesday 2026-11-25 10:00 plus 48 business hours: 14 h of Wednesday, Thursday is a
	// holiday, 24 h of Friday, the weekend is skipped, 10 h of Monday -> Monday 2026-11-30 10:00.
	start = time.Date(2026, 11, 25, 10, 0, 0, 0, ny)
	got = c.Add(start, 48*time.Hour)
	want = time.Date(2026, 11, 30, 10, 0, 0, 0, ny)
	if !got.Equal(want) {
		t.Fatalf("Wednesday + 48h over a holiday: got %s, want %s", got, want)
	}
	// The inverse agrees.
	if el := c.Elapsed(start, got); el != 48*time.Hour {
		t.Fatalf("elapsed: got %s, want 48h", el)
	}
}

func TestWallClockIsPlainAddition(t *testing.T) {
	start := time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC) // a Saturday
	if got := WallClock.Add(start, 72*time.Hour); !got.Equal(start.Add(72 * time.Hour)) {
		t.Fatalf("wall clock: got %s", got)
	}
}

func TestAddKeepsCallerZoneAndRejectsNegative(t *testing.T) {
	c := New("ca-on", "Ontario", "America/Toronto", []int{6, 7}, nil)
	start := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC) // Wednesday 07:00 Toronto
	got := c.Add(start, 2*time.Hour)
	if got.Location() != time.UTC || !got.Equal(start.Add(2*time.Hour)) {
		t.Fatalf("got %s in %s", got, got.Location())
	}
	if got := c.Add(start, -time.Hour); !got.Equal(start) {
		t.Fatalf("negative duration should return start, got %s", got)
	}
}

func TestUnknownZoneFallsBackToUTC(t *testing.T) {
	c := New("x", "x", "Mars/Olympus", []int{7}, nil)
	if c.Loc != time.UTC {
		t.Fatalf("expected UTC fallback, got %s", c.Loc)
	}
	sunday := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	if c.IsBusinessDay(sunday) {
		t.Fatal("Sunday should not be a business day")
	}
}
