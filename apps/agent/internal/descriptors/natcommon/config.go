package natcommon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"syscall"

	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ---- configuration (D-071) --------------------------------------------------------------------

// Config is the per-agent configuration every NAT family is constructed with.
type Config struct {
	// GlobalsOwner marks the one agent that manages VPP-global singletons (plugin
	// enable/disable, timeouts, forwarding, MAP params, the cnat default SNAT entry and
	// policy, IPFIX). D-071: the product agent on a real box; never a test slot on the shared
	// host. Default false: globals are only *required* (verified), never set, reset or
	// disabled.
	GlobalsOwner bool
	// Claims records the untagged objects this owner created (D-071 claim rule). Default: an
	// in-memory store per plugin family; P05 passes one persisted in the agent state dir.
	Claims ClaimStore
	// LockDir holds the host-wide lock files that serialise hazardous global mutations
	// (cnat default SNAT entry). Default /run/lock.
	LockDir string
}

// Option configures a family constructor.
type Option func(*Config)

// WithGlobalsOwner sets Config.GlobalsOwner.
func WithGlobalsOwner(owner bool) Option { return func(c *Config) { c.GlobalsOwner = owner } }

// WithClaims sets Config.Claims.
func WithClaims(s ClaimStore) Option {
	return func(c *Config) {
		if s != nil {
			c.Claims = s
		}
	}
}

// WithLockDir sets Config.LockDir.
func WithLockDir(dir string) Option {
	return func(c *Config) {
		if dir != "" {
			c.LockDir = dir
		}
	}
}

// BuildConfig applies opts over the defaults.
func BuildConfig(opts []Option) Config {
	c := Config{LockDir: "/run/lock"}
	for _, o := range opts {
		o(&c)
	}
	if c.Claims == nil {
		c.Claims = NewMemoryClaimStore()
	}
	return c
}

// ---- claims (D-071: own tag → ours; foreign tag → never; untagged → only via a claim) --------

// ClaimStore records the keys of untagged objects this owner created (DF-4 pattern,
// descriptors/acl ClaimStore). Implementations must be safe for concurrent use.
type ClaimStore interface {
	Claim(key string) error
	Release(key string) error
	Claimed(key string) bool
}

// NewMemoryClaimStore returns the default in-memory ClaimStore.
func NewMemoryClaimStore() ClaimStore { return &memClaims{m: map[string]struct{}{}} }

type memClaims struct {
	mu sync.Mutex
	m  map[string]struct{}
}

func (c *memClaims) Claim(k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = struct{}{}
	return nil
}

func (c *memClaims) Release(k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, k)
	return nil
}

func (c *memClaims) Claimed(k string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.m[k]
	return ok
}

// ---- globals (D-071) ------------------------------------------------------------------------

var (
	// ErrGlobalNotSet is returned by a non-owner's Create when a required global is off.
	ErrGlobalNotSet = errors.New("natcommon: required VPP global is not set and this agent is not the globals owner (D-071)")
	// ErrGlobalMismatch is returned by a non-owner's Create when a required global has a
	// different value than desired.
	ErrGlobalMismatch = errors.New("natcommon: VPP global differs from the desired value and this agent is not the globals owner (D-071)")
	// ErrNotEmpty is returned when a change needs a plugin disable but the plugin still holds
	// objects (of any owner).
	ErrNotEmpty = errors.New("natcommon: plugin still holds objects (any owner): refusing disable")
)

// GlobalState is what a global singleton's reader reports.
type GlobalState[T any] struct {
	Value T
	// Present: VPP has the global set / non-default / enabled.
	Present bool
	// Observable: VPP lets us read it at all. When false, a non-owner cannot verify the
	// requirement; Create then succeeds without touching VPP (documented per plugin).
	Observable bool
}

// GlobalOps describe one VPP-global singleton.
type GlobalOps[T any] struct {
	Name string
	ID   string
	Deps func(spec T) []scheduler.Dependency
	// Read reports the current VPP state.
	Read func(ctx context.Context) (GlobalState[T], error)
	// Set applies spec (owner only). Update defaults to Set; SetUpdate overrides it.
	Set       func(ctx context.Context, spec T) error
	SetUpdate func(ctx context.Context, oldSpec, newSpec T) error
	// Reset removes spec (owner only): reset to defaults / disable after an emptiness check.
	Reset func(ctx context.Context, spec T) error
	// WriteOnly: the owner cannot read it back either (Retrieve → ErrRetrieveUnsupported).
	WriteOnly bool
	// Match compares the observed value with the desired one for a non-owner's requirement;
	// default: equality after Normalize. Globals whose value is not observable (only their
	// presence is) set it to accept any value.
	Match func(have, want T) bool
	// Absent reports values that are "no object" for the owner's Retrieve (VPP defaults), so
	// a desired state without the global does not plan a reset on every pass. Optional.
	Absent func(v T) bool
}

// Global builds the descriptor of a VPP-global singleton following D-071: the globals owner
// sets/resets it with full Retrieve; any other owner only requires it — Create/Update verify
// the VPP value, Delete is a no-op, Retrieve is ErrRetrieveUnsupported (the reconciler then
// re-checks on every resync and never deletes on absence).
func Global[T any](cfg Config, ops GlobalOps[T]) *Descriptor[T] {
	id := func(T) string { return ops.ID }
	if !cfg.GlobalsOwner {
		require := func(ctx context.Context, want T) error {
			st, err := ops.Read(ctx)
			if err != nil {
				return err
			}
			switch {
			case !st.Observable:
				return nil
			case !st.Present:
				return fmt.Errorf("%s: %w", ops.Name, ErrGlobalNotSet)
			case ops.Match != nil && !ops.Match(st.Value, want), ops.Match == nil && !sameSpec(st.Value, want):
				return fmt.Errorf("%s: %w: VPP has %+v, desired %+v", ops.Name, ErrGlobalMismatch, st.Value, want)
			}
			return nil
		}
		return New(Ops[T]{
			Name: ops.Name, ID: id, Deps: ops.Deps,
			Create:   func(ctx context.Context, s T) (any, error) { return nil, require(ctx, s) },
			Update:   func(ctx context.Context, _, s T, _ any) (any, error) { return nil, require(ctx, s) },
			Delete:   func(context.Context, T, any) error { return nil },
			Retrieve: func(context.Context) ([]Item[T], error) { return nil, ErrRetrieveUnsupported },
			Global:   true,
		})
	}
	update := func(ctx context.Context, o, n T, _ any) (any, error) {
		if ops.SetUpdate != nil {
			return nil, ops.SetUpdate(ctx, o, n)
		}
		return nil, ops.Set(ctx, n)
	}
	retrieve := func(ctx context.Context) ([]Item[T], error) {
		if ops.WriteOnly {
			return nil, ErrRetrieveUnsupported
		}
		st, err := ops.Read(ctx)
		if err != nil || !st.Present || (ops.Absent != nil && ops.Absent(st.Value)) {
			return nil, err
		}
		return []Item[T]{{Spec: st.Value}}, nil
	}
	return New(Ops[T]{
		Name: ops.Name, ID: id, Deps: ops.Deps,
		Create:   func(ctx context.Context, s T) (any, error) { return nil, ops.Set(ctx, s) },
		Update:   update,
		Delete:   func(ctx context.Context, s T, _ any) error { return ops.Reset(ctx, s) },
		Retrieve: retrieve,
		Global:   true,
	})
}

func sameSpec[T any](a, b T) bool {
	if n, ok := any(&a).(Normalizer); ok {
		n.Normalize()
	}
	if n, ok := any(&b).(Normalizer); ok {
		n.Normalize()
	}
	return reflect.DeepEqual(a, b)
}

// ---- host-wide locks ------------------------------------------------------------------------

// HostLock takes a host-wide flock on <dir>/vrx-nat-<name>.lock (shared or exclusive) and
// returns the release function. It serialises hazardous global mutations across agents and
// test slots on one VPP (cnat default SNAT entry, review finding 8).
func HostLock(dir, name string, exclusive bool) (func(), error) {
	path := filepath.Join(filepath.Clean(dir), "vrx-nat-"+name+".lock")
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o644) //nolint:gosec // fixed lock path
	if err != nil {
		return nil, fmt.Errorf("natcommon: lock %s: %w", path, err)
	}
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("natcommon: flock %s: %w", path, err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// AnyValue is a GlobalOps.Match for globals whose value is not observable.
func AnyValue[T any](T, T) bool { return true }

// VPPIdentity returns the PID of VPP's main thread (show_threads, thread 0). It changes with
// every VPP restart, so a claim record "<key>@<identity>" says "applied to THIS VPP process"
// (D-076; same identity as DF-4's acl.stats-enable).
func VPPIdentity(ctx context.Context, c vpp.Client) (uint32, error) {
	rep, err := vlib.NewServiceClient(c).ShowThreads(ctx, &vlib.ShowThreads{})
	if err != nil {
		return 0, fmt.Errorf("show_threads: %w", err)
	}
	for _, t := range rep.ThreadData {
		if t.ID == 0 {
			return t.PID, nil
		}
	}
	return 0, fmt.Errorf("show_threads: no main thread in %d entries", len(rep.ThreadData))
}

// AppliedRecord is the D-076 claim-record key for a non-idempotent write-only add.
func AppliedRecord(key string, vppID uint32) string { return fmt.Sprintf("%s@vpp%d", key, vppID) }
