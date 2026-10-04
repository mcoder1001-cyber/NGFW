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
