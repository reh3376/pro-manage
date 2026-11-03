// Package calendar is the business calendar of a site (FSD §03 Site
// "business_calendar_id", §08 "SLA clocks run in business days of the
// project's site calendar"): which weekdays are worked and which dates are
// holidays, in the site's time zone. Add advances a clock that runs only on
// business days, so a stage opened on Friday afternoon with a 24-hour SLA is
// due Monday afternoon, not Saturday.
package calendar

import (
	"fmt"
	"time"
)

// Calendar is one site's working-day rule.
type Calendar struct {
	ID       string
	Name     string
	Loc      *time.Location
	Weekend  map[time.Weekday]bool // days never worked
	Holidays map[string]bool       // "2006-01-02" dates never worked, in Loc
}

// New builds a calendar. weekend uses ISO numbering (1 Monday ... 7 Sunday),
// which is how the business_calendars table stores it; holidays are
// YYYY-MM-DD strings. An unknown zone falls back to UTC rather than failing a
// submission.
func New(id, name, zone string, weekend []int, holidays []string) Calendar {
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		loc = time.UTC
	}
	c := Calendar{ID: id, Name: name, Loc: loc, Weekend: map[time.Weekday]bool{}, Holidays: map[string]bool{}}
	for _, d := range weekend {
		c.Weekend[time.Weekday(d%7)] = true // ISO 7 (Sunday) maps to Go's 0
	}
	for _, h := range holidays {
		c.Holidays[h] = true
	}
	return c
}

// WallClock is the calendar every site has by default: every day is a
// business day, which is the pre-slice-12 behaviour.
var WallClock = Calendar{ID: "", Name: "wall clock", Loc: time.UTC, Weekend: map[time.Weekday]bool{}, Holidays: map[string]bool{}}

// IsBusinessDay reports whether the date of t (in the calendar's zone) is worked.
func (c Calendar) IsBusinessDay(t time.Time) bool {
	lt := t.In(c.loc())
	if c.Weekend[lt.Weekday()] {
		return false
	}
	return !c.Holidays[lt.Format("2006-01-02")]
}

// Add returns the instant at which d of business time has elapsed from start:
// the clock runs through business days end to end and is paused through
// weekend and holiday dates. A non-positive d returns start.
func (c Calendar) Add(start time.Time, d time.Duration) time.Time {
	if d <= 0 {
		return start
	}
	loc := c.loc()
	cur := start.In(loc)
	remaining := d
	for guard := 0; guard < 4000; guard++ { // more than ten years of calendar days
		next := nextMidnight(cur)
		if c.IsBusinessDay(cur) {
			span := next.Sub(cur)
			if remaining <= span {
				return cur.Add(remaining).In(start.Location())
			}
			remaining -= span
		}
		cur = next
	}
	return cur.Add(remaining).In(start.Location())
}

// Elapsed is the business time between a and b (b after a), the inverse of Add.
func (c Calendar) Elapsed(a, b time.Time) time.Duration {
	if !b.After(a) {
		return 0
	}
	loc := c.loc()
	cur := a.In(loc)
	end := b.In(loc)
	var total time.Duration
	for guard := 0; guard < 4000 && cur.Before(end); guard++ {
		next := nextMidnight(cur)
		if next.After(end) {
			next = end
		}
		if c.IsBusinessDay(cur) {
			total += next.Sub(cur)
		}
		cur = next
	}
	return total
}

func (c Calendar) loc() *time.Location {
	if c.Loc == nil {
		return time.UTC
	}
	return c.Loc
}

func nextMidnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, t.Location())
}

// String names the calendar for logs and snapshots.
func (c Calendar) String() string {
	if c.ID == "" {
		return "wall clock"
	}
	return fmt.Sprintf("%s (%s, %d holidays)", c.ID, c.loc().String(), len(c.Holidays))
}
