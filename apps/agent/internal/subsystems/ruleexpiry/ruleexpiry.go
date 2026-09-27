// Package ruleexpiry is F-rule-expiry's enforcement: rules with `expiresAt` (ACL rules, host rules, NAT44-ED static
// mappings) stop being rendered at that instant without a commit.
//
// The projections decide with Expired (or their own clock for ACL schedules): an expired rule is simply not projected,
// so every transaction — a commit, a resync after an agent or VPP restart — leaves it out of the data plane while the
// configuration keeps it. A rule that IS rendered and expires later is noted (Note); the Watcher arms one timer at the
// earliest noted instant and asks the agent for a resync of its stored desired state then (Wiring.RequestResync), which
// re-projects at that time and so removes exactly the rules that just expired.
//
// Notes are process-wide (one agent per process, like the other projection environments). A note that no longer
// belongs to a rendered rule (the rule was deleted, extended, or the note came from a DryRun) costs one harmless resync.
package ruleexpiry

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// Now is the clock of the projections and the watcher; tests replace it.
var Now = time.Now

// Expired parses expiresAt (RFC 3339) and reports whether it is at or before now. An empty string never expires.
func Expired(expiresAt string, now time.Time) (bool, time.Time, error) {
	if expiresAt == "" {
		return false, time.Time{}, nil
	}
	at, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("expiresAt %q: not RFC 3339", expiresAt)
	}
	return !at.After(now), at, nil
}

// coalesce: a note closer than this to an already noted instant does not get a timer of its own (one resync for a
// batch of rules expiring together; a rule still in the future at that resync is noted again by its projection).
const coalesce = 2 * time.Second

var (
	mu      sync.Mutex
	pending []time.Time // sorted, future instants of rendered rules
	w       *Watcher
)

// Note records that a rendered rule expires at `at` (a future instant); the watcher re-arms when it is the earliest.
func Note(at time.Time) {
	mu.Lock()
	i := sort.Search(len(pending), func(i int) bool { return !pending[i].Before(at) })
	switch {
	case i > 0 && at.Sub(pending[i-1]) < coalesce:
		// an earlier instant close by fires first; this rule is re-noted by that resync's projection if it is
		// not expired yet (never fired early: fire takes only instants that are due)
	case i < len(pending) && pending[i].Sub(at) < coalesce:
		pending[i] = at // keep the earlier of two close instants
	default:
		pending = append(pending, time.Time{})
		copy(pending[i+1:], pending[i:])
		pending[i] = at
	}
	cur := w
	mu.Unlock()
	if cur != nil {
		cur.arm()
	}
}

// Pending returns the noted instants (tests, the log).
func Pending() []time.Time {
	mu.Lock()
	defer mu.Unlock()
	return append([]time.Time(nil), pending...)
}

// Reset drops every note and the watcher (tests).
func Reset() {
	mu.Lock()
	cur := w
	pending, w = nil, nil
	mu.Unlock()
	if cur != nil {
		cur.Stop()
	}
}

// Watcher asks for a resync when the earliest noted instant is reached.
type Watcher struct {
	resync func()
	log    *slog.Logger

	mu      sync.Mutex
	timer   *time.Timer
	at      time.Time
	stopped bool
}

// Start installs the process's watcher (the previous one is stopped) and arms it for the notes already taken.
func Start(resync func(), log *slog.Logger) *Watcher {
	if log == nil {
		log = slog.Default()
	}
	nw := &Watcher{resync: resync, log: log}
	mu.Lock()
	old := w
	w = nw
	mu.Unlock()
	if old != nil {
		old.Stop()
	}
	nw.arm()
	return nw
}

// Stop cancels the timer; notes are kept for the next watcher.
func (x *Watcher) Stop() {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.stopped = true
	if x.timer != nil {
		x.timer.Stop()
		x.timer = nil
	}
}

// arm (re)sets the timer to the earliest pending instant.
func (x *Watcher) arm() {
	mu.Lock()
	var next time.Time
	if len(pending) > 0 {
		next = pending[0]
	}
	mu.Unlock()
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.stopped || next.IsZero() || (x.timer != nil && !next.Before(x.at)) {
		return
	}
	if x.timer != nil {
		x.timer.Stop()
	}
	x.at = next
	x.timer = time.AfterFunc(max(time.Until(next), 0), x.fire)
}

// fire drops the instants that are due and asks for one resync.
func (x *Watcher) fire() {
	now := Now()
	mu.Lock()
	n := 0
	for n < len(pending) && !pending[n].After(now) {
		n++
	}
	due := pending[:n:n]
	pending = append([]time.Time(nil), pending[n:]...)
	mu.Unlock()
	x.mu.Lock()
	x.timer, x.at = nil, time.Time{}
	stopped := x.stopped
	x.mu.Unlock()
	if stopped {
		return
	}
	if len(due) > 0 {
		x.log.Info("rules expired: re-projecting the stored desired state (F-rule-expiry)", "expired_at", due[0].UTC().Format(time.RFC3339), "instants", len(due))
		x.resync()
	}
	x.arm()
}
