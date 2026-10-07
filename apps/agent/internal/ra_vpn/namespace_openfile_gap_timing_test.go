package ravpn

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNumericPublisherGapTimingClosedFailureAndSilence(t *testing.T) {
	trace := newNumericPublisherGapTiming(context.Background(), time.Now())
	if trace.failureLine(false) != "" {
		t.Fatal("healthy call logged")
	}
	want := "remote-access publisher-client phase=completion entry_budget_ms=40000 probe_response_ms=-1 probe_verified_ms=-1 probe_exit_ms=-1 publish_response_ms=-1 publish_verified_ms=-1"
	if trace.failureLine(true) != want {
		t.Fatal("unset completion marks fabricated")
	}
	trace.started = time.Now().Add(-time.Hour)
	for index := range trace.marks {
		trace.mark(index)
		if trace.marks[index] != 40000 {
			t.Fatal("unbounded completion time")
		}
	}
	before := trace.marks
	trace.mark(-1)
	trace.mark(5)
	if trace.marks != before {
		t.Fatal("foreign completion mark accepted")
	}
	line := trace.failureLine(true)
	if len(line) > 300 || strings.ContainsAny(line, "/@\n") {
		t.Fatal("nonclosed completion line")
	}
	trace.started = time.Now().Add(time.Hour)
	trace.mark(0)
	if trace.marks[0] != 0 {
		t.Fatal("negative completion time")
	}
}

func TestNumericPublisherBoot38BudgetConsumption(t *testing.T) {
	// Use fixed instants to reproduce Boot38's measured client interval without
	// waiting30s or inferring its unknown per-primitive latency. The26825ms
	// initial budget below is a model consistent with expiry, not a logged fact.
	start := time.Now()
	parentDeadline := start.Add(26825 * time.Millisecond)
	if got := numericPublisherRemainingBudget(start, parentDeadline, true); got != 26825 {
		t.Fatal("entry budget reset", got)
	}
	if got := numericPublisherRemainingBudget(start.Add(20357*time.Millisecond), parentDeadline, true); got != 6468 {
		t.Fatal("publish reset caller budget", got)
	}
	if got := numericPublisherRemainingBudget(parentDeadline, parentDeadline, true); got != 0 {
		t.Fatal("expired caller gained time", got)
	}
	parent, cancel := context.WithDeadline(context.Background(), parentDeadline)
	defer cancel()
	publication, cancelPublication := context.WithTimeout(parent, NumericOpenFilePublicationBudget)
	defer cancelPublication()
	validation, cancelValidation := context.WithTimeout(publication, NumericPublisherValidationBudget)
	defer cancelValidation()
	work, cancelWork := context.WithTimeout(publication, NumericPublisherWorkBudget)
	defer cancelWork()
	// Each phase derives from publication, not the previous phase. Their phase
	// limits may be earlier; none may extend publication or the caller deadline.
	for _, ctx := range []context.Context{publication, validation, work} {
		deadline, present := ctx.Deadline()
		if !present || deadline.After(parentDeadline) {
			t.Fatal("phase extended caller deadline")
		}
	}
	deadline, _ := publication.Deadline()
	if !deadline.Equal(parentDeadline) {
		t.Fatal("publication did not inherit exact caller deadline")
	}
	for _, test := range []struct {
		delta time.Duration
		want  int64
	}{{-time.Second, 0}, {time.Hour, 40000}, {20 * time.Second, 20000}} {
		if got := numericPublisherRemainingBudget(start, start.Add(test.delta), true); got != test.want {
			t.Fatal("budget clamp changed", got)
		}
	}
}
