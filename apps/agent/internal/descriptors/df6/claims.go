package df6

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"ngfw/agent/binapi/memclnt"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp"
)

// Ownership of untagged objects (D-071 claim rule): SR local SIDs / policies / steering entries,
// SR-MPLS policies, LISP locator sets / mappings, GPE entries … carry no owner tag, so the only
// proof that one is ours is a claim record written after our own successful Create:
// ClaimStore.Claim(id, holder) with holder = descriptor name. Retrieve reports claimed ids only,
// Create refuses to take over an existing unclaimed object, Delete never touches an unclaimed one.
//
// The store is DF-1's per-owner ClaimStore (iface.Claims(owner)); P05/P08 install one persisted
// in the agent state dir with iface.SetClaimStore so claims survive an agent restart
// (FileClaimStore below is such a store).

// ErrNotOurs means the object exists in VPP but carries no claim of this owner: another owner's
// or an operator's object, never taken over (D-071).
var ErrNotOurs = errors.New("object exists in vpp but is not ours (no claim)")

// ErrNotGlobalsOwner means a VPP-global setting was requested by an agent that is not the
// globals owner (D-071); only the globals owner sets, resets or disables globals.
var ErrNotGlobalsOwner = errors.New("vpp-global setting is managed by the globals owner only")

// ClaimStore is DF-1's claim store interface.
type ClaimStore = iface.ClaimStore

// Options configures the DF-6 Register functions.
type Options struct {
	// GlobalsOwner registers the setters of VPP-global singletons (D-071). False: the
	// "require" variants (Create checks, never sets; Delete no-op; never deleted on absence).
	GlobalsOwner bool
	// Claims is the owner's claim store; nil = iface.Claims(owner).
	Claims ClaimStore
}

// Option configures Options.
type Option func(*Options)

// WithGlobalsOwner marks this agent as the globals owner (agent config globalsOwner: true).
func WithGlobalsOwner(on bool) Option { return func(o *Options) { o.GlobalsOwner = on } }

// WithClaims sets the claim store (default iface.Claims(owner)).
func WithClaims(s ClaimStore) Option { return func(o *Options) { o.Claims = s } }

// BuildOptions applies opts for owner.
func BuildOptions(owner string, opts []Option) Options {
	var o Options
	for _, f := range opts {
		f(&o)
	}
	if o.Claims == nil {
		o.Claims = iface.Claims(owner)
	}
	return o
}

// BootID is the identity of the running VPP instance: the PID control_ping reports. A
// write-only object whose VPP add is not idempotent is re-added only when this changes (D-076).
func BootID(ctx context.Context, c vpp.Client) (uint32, error) {
	rep := &memclnt.ControlPingReply{}
	if err := c.Invoke(ctx, &memclnt.ControlPing{}, rep); err != nil {
		return 0, fmt.Errorf("control_ping: %w", err)
	}
	return rep.VpePID, nil
}

// BootHolder is the claim holder recording that holder's object was applied to VPP boot id.
func BootHolder(holder string, boot uint32) string { return fmt.Sprintf("%s@vpp-%d", holder, boot) }

// FileClaimStore is a ClaimStore persisted as JSON in one file (agent state dir). Safe for
// concurrent use within one process.
type FileClaimStore struct {
	mu   sync.Mutex
	path string
	m    map[string]map[string]bool // holder → id → true
}

// OpenFileClaimStore loads (or starts) the store at path.
func OpenFileClaimStore(path string) (*FileClaimStore, error) {
	s := &FileClaimStore{path: path, m: map[string]map[string]bool{}}
	b, err := os.ReadFile(path) //nolint:gosec // path is the agent state-dir file chosen by the caller
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(b, &s.m); err != nil {
		return nil, fmt.Errorf("claim store %s: %w", path, err)
	}
	return s, nil
}

func (s *FileClaimStore) save() error {
	b, err := json.MarshalIndent(s.m, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Claim implements ClaimStore.
func (s *FileClaimStore) Claim(id, holder string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[holder] == nil {
		s.m[holder] = map[string]bool{}
	}
	s.m[holder][id] = true
	return s.save()
}

// Release implements ClaimStore.
func (s *FileClaimStore) Release(id, holder string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m[holder], id)
	return s.save()
}

// Claimed implements ClaimStore.
func (s *FileClaimStore) Claimed(id, holder string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[holder][id]
}
