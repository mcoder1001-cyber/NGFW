package df6

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"ngfw/agent/internal/descriptors/dfkit/persist"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp/bootid"
)

// Ownership of untagged objects (D-071 claim rule): SR local SIDs / policies / steering entries,
// SR-MPLS policies, LISP locator sets / mappings, GPE entries … carry no owner tag, so the only
// proof that one is ours is a claim record written after our own successful Create:
// ClaimStore.Claim(id, holder) with holder = descriptor name. Retrieve reports claimed ids only,
// Create refuses to take over an existing unclaimed object, Delete never touches an unclaimed one.
//
// Two stores are involved. Per-interface claims (an untagged interface a bypass/toggle configures)
// always go to DF-1's per-owner ClaimStore (iface.Claims(owner)), which the product wiring persists
// with iface.SetClaimStore (subsystems.IfaceClaims, bound to the interface's sw_if_index). Keyed and
// per-boot claims go to Options.Claims: their ids are not interface names, so the product wiring
// passes a persisted store keyed by id — df6.WithClaims(Wiring.PairClaims("df6")) — and the
// product agent refuses to start otherwise (CheckPersistent, TD-11b; re-review N7 "claims split
// across two stores"). The default (iface.Claims(owner)) serves tests with an in-memory or
// FileClaimStore installed for the owner.

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

// BootHolder is the claim holder recording that holder's object belongs to VPP instance boot
// (D-080 identity, bootid.Current). Claims made on another VPP instance — or in the pre-D-080
// PID-only format — never match and so have expired.
func BootHolder(holder string, boot bootid.Identity) string { return holder + "@vpp-" + boot.String() }

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

// Claim implements ClaimStore. Memory changes only when the file was written (TD-11b; re-review
// N7): a claim kept in memory after a failed write made the retry report success, and the claim
// was then lost on the next agent restart.
func (s *FileClaimStore) Claim(id, holder string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[holder][id] {
		return nil
	}
	if s.m[holder] == nil {
		s.m[holder] = map[string]bool{}
	}
	s.m[holder][id] = true
	if err := s.save(); err != nil {
		delete(s.m[holder], id)
		if len(s.m[holder]) == 0 {
			delete(s.m, holder)
		}
		return err
	}
	return nil
}

// Release implements ClaimStore (memory changes only when the file was written, as Claim).
func (s *FileClaimStore) Release(id, holder string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.m[holder][id] {
		return nil
	}
	delete(s.m[holder], id)
	if err := s.save(); err != nil {
		s.m[holder][id] = true
		return err
	}
	return nil
}

// Persistent marks FileClaimStore as a store that survives an agent restart (dfkit/persist).
func (*FileClaimStore) Persistent() bool { return true }

// ErrClaimStoreKind means a DF-6 descriptor was given a claim store that binds every claim to an
// interface's sw_if_index (subsystems.IfaceClaims): keyed and per-boot DF-6 claims are keyed by an
// object id ("<sid>", "<iface>@<idx>/ip4"), which is not an interface name, so every Claim of such
// a store fails (ErrClaimUnbound). The product wiring passes df6.WithClaims(Wiring.PairClaims("df6")).
var ErrClaimStoreKind = errors.New("df6: claim store binds claims to interface indexes; keyed claims need a store keyed by id")

// checkIDClaims is the persistence check (dfkit/persist) of the store DF-6 keyed and per-boot
// claims go to: it must survive an agent restart and must not bind claims to interface indexes.
func checkIDClaims(name string, s ClaimStore) error {
	if err := persist.Require("df6 "+name+": keyed claims (pass df6.WithClaims(Wiring.PairClaims(\"df6\")))", s); err != nil {
		return err
	}
	if b, ok := s.(interface{ BindsInterfaceIndex() bool }); ok && b.BindsInterfaceIndex() {
		return fmt.Errorf("%w: df6 %s: %T (pass df6.WithClaims(Wiring.PairClaims(\"df6\")))", ErrClaimStoreKind, name, s)
	}
	return nil
}

// Claimed implements ClaimStore.
func (s *FileClaimStore) Claimed(id, holder string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[holder][id]
}

// checkIfaceClaims is the persistence check of the owner's DF-1 store, where per-interface DF-6
// claims go (Interfaces.ClaimIfUntagged).
func checkIfaceClaims(name, owner string) error {
	return persist.Require("df6 "+name+": claims on untagged interfaces of owner "+owner+" (install a persisted store with iface.SetClaimStore)", iface.Claims(owner))
}
