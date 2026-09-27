package ruleexpiry

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestExpired(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		in      string
		expired bool
		err     bool
	}{
		{"", false, false},
		{"2026-09-27T11:59:59Z", true, false},
		{"2026-09-27T12:00:00Z", true, false}, // at the instant: expired
		{"2026-09-27T15:30:01+03:30", false, false},
		{"2026-09-27T15:30:00+03:30", true, false},
		{"tomorrow", false, true},
	} {
		got, _, err := Expired(c.in, now)
		if got != c.expired || (err != nil) != c.err {
			t.Errorf("%q: expired %v err %v, want %v err %v", c.in, got, err, c.expired, c.err)
		}
	}
}

// The watcher fires one resync per due instant (close instants coalesce into one, keeping the earlier), never before
// an instant, and re-arms for the next.
func TestWatcherFiresAtTheEarliestNote(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	var n atomic.Int32
	x := Start(func() { n.Add(1) }, nil)
	t0 := time.Now()
	Note(t0.Add(300 * time.Millisecond))
	Note(t0.Add(150 * time.Millisecond))                      // earlier: re-arms
	Note(t0.Add(150*time.Millisecond + 500*time.Microsecond)) // within coalesce of an earlier one: no timer of its own
	if p := Pending(); len(p) != 1 {
		// 300 ms and 150 ms are within `coalesce`: one instant, the earlier (the resync at 150 ms re-projects and notes
		// the 300 ms rule again if it is still in the future)
		t.Fatalf("pending %v, want the 150 ms instant only", p)
	}
	time.Sleep(100 * time.Millisecond)
	if n.Load() != 0 {
		t.Fatal("fired before the instant")
	}
	deadline := time.Now().Add(3 * time.Second)
	for n.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n.Load() != 1 || len(Pending()) != 0 {
		t.Fatalf("resyncs %d, pending %v", n.Load(), Pending())
	}
	// a later note re-arms; Stop cancels
	Note(time.Now().Add(50 * time.Millisecond))
	x.Stop()
	time.Sleep(150 * time.Millisecond)
	if n.Load() != 1 {
		t.Fatalf("fired after Stop: %d", n.Load())
	}
}
