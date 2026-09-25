package agent

// F-unbound-chrony-syslog, review M3 / D-132: the state RPCs walk daemons (unbound-control, chronyc, the rsyslog stats
// file) or the journal (journalctl, up to 5000 entries). Each kind allows one walk in flight per agent; a second caller
// waits at most walkWait and then gets UNAVAILABLE (the API answers 503; the UI's next poll or Refresh retries), so N
// browser tabs never start N daemon walks or journal scans at once.

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// walkWait bounds the wait for a walk of the same kind (D-132: 3 s).
var walkWait = 3 * time.Second

type walkLimit struct {
	name string
	ch   chan struct{}
}

func newWalkLimit(name string) *walkLimit { return &walkLimit{name: name, ch: make(chan struct{}, 1)} }

var (
	dnsWalk     = newWalkLimit("DNS")
	ntpWalk     = newWalkLimit("NTP")
	syslogWalk  = newWalkLimit("syslog export")
	journalWalk = newWalkLimit("journal")
)

// acquire takes the walk slot; the returned function releases it.
func (w *walkLimit) acquire(ctx context.Context) (func(), error) {
	t := time.NewTimer(walkWait)
	defer t.Stop()
	select {
	case w.ch <- struct{}{}:
		return func() { <-w.ch }, nil
	case <-t.C:
		return nil, status.Errorf(codes.Unavailable, "another %s state walk is in flight; one at a time (D-132) — retry", w.name)
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	}
}
