package multiwan

import (
	"context"
	"fmt"
	"ngfw/agent/internal/descriptors/nat44ed"
	"testing"
)

type cleanupFake struct {
	rows    []nat44ed.Session
	deleted []nat44ed.Endpoint
}

func (f *cleanupFake) Users(context.Context) ([]nat44ed.User, error) {
	return []nat44ed.User{{IP: "10.0.0.1", VRF: 7}}, nil
}
func (f *cleanupFake) UserSessions(_ context.Context, _ nat44ed.User, offset, limit int) ([]nat44ed.Session, error) {
	if offset >= len(f.rows) {
		return nil, nil
	}
	end := offset + limit
	if end > len(f.rows) {
		end = len(f.rows)
	}
	return append([]nat44ed.Session(nil), f.rows[offset:end]...), nil
}
func (f *cleanupFake) DeleteSession(_ context.Context, e nat44ed.Endpoint, _ string, _ uint32, _ nat44ed.Endpoint) error {
	f.deleted = append(f.deleted, e)
	for i, s := range f.rows {
		if s.Inside == e {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			break
		}
	}
	return nil
}
func TestCleanupFiltersAndResumesBoundedPages(t *testing.T) {
	f := &cleanupFake{}
	for i := 0; i < 1100; i++ {
		f.rows = append(f.rows, nat44ed.Session{Inside: nat44ed.Endpoint{IP: fmt.Sprintf("10.0.%d.%d", i/256, i%256)}, Outside: nat44ed.Endpoint{IP: "192.0.2.2"}, Protocol: "tcp"})
	}
	f.rows = append(f.rows, nat44ed.Session{Inside: nat44ed.Endpoint{IP: "static"}, Outside: nat44ed.Endpoint{IP: "192.0.2.2"}, Static: true}, nat44ed.Session{Inside: nat44ed.Endpoint{IP: "foreign"}, Outside: nat44ed.Endpoint{IP: "198.51.100.2"}})
	progress := &CleanupProgress{}
	complete := false
	for i := 0; i < 5 && !complete; i++ {
		count, done, err := ClearDeadSessions(context.Background(), f, map[string]bool{"192.0.2.2": true}, map[uint32]bool{7: true}, progress)
		if err != nil || count > 512 {
			t.Fatal(count, err)
		}
		complete = done
	}
	if !complete || len(f.deleted) != 1100 || len(f.rows) != 2 {
		t.Fatal(complete, len(f.deleted), len(f.rows))
	}
}

type cleanupRetryFake struct {
	cleanupFake
	fail bool
}

func (f *cleanupRetryFake) DeleteSession(ctx context.Context, e nat44ed.Endpoint, p string, vrf uint32, ext nat44ed.Endpoint) error {
	if f.fail {
		return fmt.Errorf("transient delete failure")
	}
	return f.cleanupFake.DeleteSession(ctx, e, p, vrf, ext)
}

func TestCleanupRetryProtectsReboundLeaseSessions(t *testing.T) {
	doc := routeDoc()
	doc.Interfaces["wan1"].Ipv4 = []string{"192.0.2.2/24"}
	doc.Interfaces["wan2"].Ipv4 = []string{"198.51.100.2/24"}
	f := &cleanupRetryFake{fail: true, cleanupFake: cleanupFake{rows: []nat44ed.Session{
		{Inside: nat44ed.Endpoint{IP: "10.0.0.1"}, Outside: nat44ed.Endpoint{IP: "192.0.2.2"}, Protocol: "tcp"},
		{Inside: nat44ed.Endpoint{IP: "10.0.0.2"}, Outside: nat44ed.Endpoint{IP: "198.51.100.2"}, Protocol: "tcp"},
	}}}
	pending := map[string]bool{"192.0.2.2": true, "198.51.100.2": true}
	retired := map[string]bool{"192.0.2.2": true}
	progress := &CleanupProgress{}
	if _, done, err := ClearDeadSessions(context.Background(), f, pending, map[uint32]bool{7: true}, progress); err == nil || done {
		t.Fatal("failure not retained")
	}
	// The ISP reassigns the retired address, before probe hysteresis recovers.
	// Sessions cannot be distinguished by lease generation, so protect all of them.
	f.fail = false
	if !PruneLiveCleanup(pending, retired, doc, health(false, false)) {
		t.Fatal("rebound not protected")
	}
	*progress = CleanupProgress{}
	if pending["192.0.2.2"] || len(retired) != 0 || !pending["198.51.100.2"] {
		t.Fatal(pending, retired)
	}
	if n, done, err := ClearDeadSessions(context.Background(), f, pending, map[uint32]bool{7: true}, progress); err != nil || !done || n != 1 {
		t.Fatal(n, done, err)
	}
	if len(f.rows) != 1 || f.rows[0].Outside.IP != "192.0.2.2" {
		t.Fatal("replacement lease session deleted", f.rows)
	}
	// Ordinary dead-link cleanup is also cancelled if health recovers on retry.
	pending = map[string]bool{"198.51.100.2": true}
	if !PruneLiveCleanup(pending, nil, doc, health(false, true)) || len(pending) != 0 {
		t.Fatal("recovered member queued", pending)
	}
}
