package objects

import (
	"fmt"
	"time"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// Active reports whether schedule s is active at instant now.
//
//   - recurring: now, read on the wall clock of loc (nil = UTC; consumers pass the box's
//     system.timezone), falls on one of s.days and its time of day is in [start, end). The
//     comparison is on the wall clock, so across a DST change a window covers what the clock
//     shows: on a spring-forward day a window inside the skipped hour is never active, on a
//     fall-back day a window inside the repeated hour is active twice, and a window spanning the
//     change is shorter or longer by the shift.
//   - once: start ≤ now < end, with the absolute RFC 3339 instants of the schedule (loc unused).
//
// What an inactive schedule means for a rule is the consumer's decision (docs/agent/objects.md:
// re-project periodically, default every 60 s).
func Active(s *vrxv1.Schedule, now time.Time, loc *time.Location) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("%w: nil schedule", ErrInvalid)
	}
	switch s.GetType() {
	case "recurring":
		start, err1 := minuteOfDay(s.GetStart())
		end, err2 := minuteOfDay(s.GetEnd())
		if err1 != nil || err2 != nil || start >= end {
			return false, fmt.Errorf("%w: recurring schedule window %q–%q", ErrInvalid, s.GetStart(), s.GetEnd())
		}
		if loc == nil {
			loc = time.UTC
		}
		local := now.In(loc)
		on := false
		for _, d := range s.GetDays() {
			wd, ok := weekdays[d]
			if !ok {
				return false, fmt.Errorf("%w: weekday %q", ErrInvalid, d)
			}
			on = on || wd == local.Weekday()
		}
		if !on {
			return false, nil
		}
		sec := local.Hour()*3600 + local.Minute()*60 + local.Second()
		return start*60 <= sec && sec < end*60, nil
	case "once":
		start, err1 := time.Parse(time.RFC3339, s.GetStart())
		end, err2 := time.Parse(time.RFC3339, s.GetEnd())
		if err1 != nil || err2 != nil || !start.Before(end) {
			return false, fmt.Errorf("%w: one-time schedule window %q–%q", ErrInvalid, s.GetStart(), s.GetEnd())
		}
		return !now.Before(start) && now.Before(end), nil
	}
	return false, fmt.Errorf("%w: schedule type %q", ErrInvalid, s.GetType())
}

// minuteOfDay parses "HH:MM" (00:00–23:59).
func minuteOfDay(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil || len(s) != 5 {
		return 0, fmt.Errorf("time of day %q", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}
