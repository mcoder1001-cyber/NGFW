package df7

import (
	"context"
	"fmt"
	"sync"

	iface "ngfw/agent/internal/descriptors/interface"
)

// AppliedStore records, per write-only object key, the VPP boot identity (main-thread PID,
// iface.VPPIdentity) in which the object was applied (D-076). A write-only descriptor whose VPP
// "add" is not idempotent — a feature enable that stacks a second instance of the feature node,
// for example — skips the re-add while the identity is unchanged and re-adds once after a VPP
// restart. P05 installs a store persisted in the agent state dir (SetAppliedStore) so an agent
// restart does not re-add either; the default is in memory.
type AppliedStore interface {
	Applied(key string) (boot uint32, ok bool)
	SetApplied(key string, boot uint32) error
	ClearApplied(key string) error
}

type memApplied struct {
	mu sync.Mutex
	m  map[string]uint32
}

// NewMemoryAppliedStore returns an in-memory AppliedStore.
func NewMemoryAppliedStore() AppliedStore { return &memApplied{m: map[string]uint32{}} }

func (s *memApplied) Applied(key string) (uint32, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.m[key]
	return b, ok
}

func (s *memApplied) SetApplied(key string, boot uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = boot
	return nil
}

func (s *memApplied) ClearApplied(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

var (
	appliedMu sync.Mutex
	appliedBy = map[string]AppliedStore{}
)

// SetAppliedStore installs the AppliedStore of owner (P05: persisted); nil restores a fresh
// in-memory one.
func SetAppliedStore(owner string, s AppliedStore) {
	appliedMu.Lock()
	defer appliedMu.Unlock()
	if s == nil {
		s = NewMemoryAppliedStore()
	}
	appliedBy[owner] = s
}

// AppliedFor returns the AppliedStore of owner.
func AppliedFor(owner string) AppliedStore {
	appliedMu.Lock()
	defer appliedMu.Unlock()
	s, ok := appliedBy[owner]
	if !ok {
		s = NewMemoryAppliedStore()
		appliedBy[owner] = s
	}
	return s
}

// BootIdentity is the current VPP identity (iface.VPPIdentity: main-thread PID).
func (b Base) BootIdentity(ctx context.Context) (uint32, error) {
	return iface.VPPIdentity(ctx, b.Client)
}

// AppliedNow reports whether key was applied in the current VPP lifetime.
func (b Base) AppliedNow(ctx context.Context, key string) (bool, error) {
	boot, err := b.BootIdentity(ctx)
	if err != nil {
		return false, err
	}
	got, ok := AppliedFor(b.Owner).Applied(key)
	return ok && got == boot, nil
}

// ApplyOnce runs apply unless key was already applied in the current VPP lifetime, and records
// the identity afterwards. skipped reports a skip.
func (b Base) ApplyOnce(ctx context.Context, key string, apply func() error) (skipped bool, err error) {
	boot, err := b.BootIdentity(ctx)
	if err != nil {
		return false, err
	}
	store := AppliedFor(b.Owner)
	if got, ok := store.Applied(key); ok && got == boot {
		return true, nil
	}
	if err := apply(); err != nil {
		return false, err
	}
	if err := store.SetApplied(key, boot); err != nil {
		return false, fmt.Errorf("%s: record applied %s: %w", b.name, key, err)
	}
	return false, nil
}

// ForgetApplied drops the record of key (after Delete).
func (b Base) ForgetApplied(key string) error { return AppliedFor(b.Owner).ClearApplied(key) }
