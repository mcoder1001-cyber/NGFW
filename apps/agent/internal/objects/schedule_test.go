package objects

import (
	"errors"
	"testing"
	"time"
	_ "time/tzdata" // DST edges must not depend on the host's zoneinfo

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

func recurring(days []string, start, end string) *vrxv1.Schedule {
	return &vrxv1.Schedule{Type: ptr("recurring"), Days: days, Start: ptr(start), End: ptr(end)}
}

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestActiveRecurring(t *testing.T) {
	office := recurring([]string{"mon", "tue", "wed", "thu", "fri"}, "08:00", "18:00")
	tehran := mustLoc(t, "Asia/Tehran") // UTC+03:30, no DST since 2022
	cases := []struct {
		at   string // UTC
		loc  *time.Location
		want bool
	}{
		{"2026-09-21T07:59:59Z", nil, false}, // Monday 07:59:59 UTC
		{"2026-09-21T08:00:00Z", nil, true},  // start is inclusive
		{"2026-09-21T17:59:59Z", nil, true},
		{"2026-09-21T18:00:00Z", nil, false}, // end is exclusive
		{"2026-09-26T12:00:00Z", nil, false}, // Saturday
		// the wall clock of loc decides: Monday 04:30 UTC = 08:00 in Tehran
		{"2026-09-21T04:29:00Z", tehran, false},
		{"2026-09-21T04:30:00Z", tehran, true},
		// Friday 20:00 UTC = Friday 23:30 in Tehran: a weekday, outside the window
		{"2026-09-25T20:00:00Z", tehran, false},
	}
	for _, c := range cases {
		now, _ := time.Parse(time.RFC3339, c.at)
		got, err := Active(office, now, c.loc)
		if err != nil || got != c.want {
			t.Errorf("office hours at %s (%v): %v %v, want %v", c.at, c.loc, got, err, c.want)
		}
	}
}

// DST: the window is read on the wall clock. Europe/Berlin 2026: spring forward Sun 29 Mar
// 02:00 → 03:00 (01:00 UTC); fall back Sun 25 Oct 03:00 → 02:00 (01:00 UTC).
func TestActiveDSTChange(t *testing.T) {
	berlin := mustLoc(t, "Europe/Berlin")
	sunday := []string{"sun"}
	skipped := recurring(sunday, "02:15", "02:45")  // inside the skipped hour on 29 Mar
	repeated := recurring(sunday, "02:15", "02:45") // inside the repeated hour on 25 Oct
	spanning := recurring(sunday, "01:30", "03:30") // across the change
	at := func(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }
	check := func(name string, s *vrxv1.Schedule, utc string, want bool) {
		t.Helper()
		got, err := Active(s, at(utc), berlin)
		if err != nil || got != want {
			t.Errorf("%s at %s (Berlin %s): %v %v, want %v", name, utc, at(utc).In(berlin).Format("15:04 MST"), got, err, want)
		} else {
			t.Logf("%-9s %s UTC = %s Berlin → active=%v", name, utc[11:16], at(utc).In(berlin).Format("15:04 MST"), got)
		}
	}
	// spring forward: 00:59 UTC = 01:59 CET, 01:00 UTC = 03:00 CEST — 02:15–02:45 never exists
	for _, u := range []string{"2026-03-29T00:59:00Z", "2026-03-29T01:00:00Z", "2026-03-29T01:15:00Z", "2026-03-29T01:30:00Z"} {
		check("skipped", skipped, u, false)
	}
	// fall back: 00:20 UTC = 02:20 CEST and 01:20 UTC = 02:20 CET — active in both hours
	check("repeated", repeated, "2026-10-25T00:20:00Z", true)
	check("repeated", repeated, "2026-10-25T01:20:00Z", true)
	check("repeated", repeated, "2026-10-25T01:50:00Z", false) // 02:50 CET
	// spanning 01:30–03:30 on the spring-forward day: 1 h long (00:30–01:30 UTC)
	check("spanning", spanning, "2026-03-29T00:29:00Z", false)
	check("spanning", spanning, "2026-03-29T00:30:00Z", true) // 01:30 CET
	check("spanning", spanning, "2026-03-29T01:29:00Z", true) // 03:29 CEST
	check("spanning", spanning, "2026-03-29T01:30:00Z", false)
	// … and 3 h long on the fall-back day (23:30 UTC Sat – 02:30 UTC)
	check("spanning", spanning, "2026-10-24T23:30:00Z", true) // 01:30 CEST
	check("spanning", spanning, "2026-10-25T02:29:00Z", true) // 03:29 CET
	check("spanning", spanning, "2026-10-25T02:30:00Z", false)
}

func TestActiveOnce(t *testing.T) {
	// a maintenance window written with an offset; instants are absolute, loc does not matter
	w := &vrxv1.Schedule{Type: ptr("once"), Start: ptr("2026-10-01T22:00:00+03:30"), End: ptr("2026-10-02T02:00:00.5+03:30")}
	for _, c := range []struct {
		at   string
		want bool
	}{
		{"2026-10-01T18:29:59Z", false},
		{"2026-10-01T18:30:00Z", true}, // start inclusive
		{"2026-10-01T22:30:00Z", true}, // fractional end: 22:30:00.5 UTC
		{"2026-10-01T22:30:01Z", false},
	} {
		now, _ := time.Parse(time.RFC3339, c.at)
		for _, loc := range []*time.Location{nil, mustLoc(t, "America/New_York")} {
			if got, err := Active(w, now, loc); err != nil || got != c.want {
				t.Errorf("once at %s (%v): %v %v, want %v", c.at, loc, got, err, c.want)
			}
		}
	}
}

func TestActiveInvalid(t *testing.T) {
	now := time.Now()
	for name, s := range map[string]*vrxv1.Schedule{
		"nil-type":   {},
		"bad-day":    recurring([]string{"mon", "xyz"}, "08:00", "09:00"),
		"end≤start":  recurring([]string{"mon"}, "09:00", "09:00"),
		"24:00":      recurring([]string{"mon"}, "08:00", "24:00"),
		"once-order": {Type: ptr("once"), Start: ptr("2026-10-02T00:00:00Z"), End: ptr("2026-10-01T00:00:00Z")},
		"once-text":  {Type: ptr("once"), Start: ptr("tomorrow"), End: ptr("2026-10-01T00:00:00Z")},
	} {
		if _, err := Active(s, now, nil); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := Active(nil, now, nil); !errors.Is(err, ErrInvalid) {
		t.Error("nil schedule")
	}
}
