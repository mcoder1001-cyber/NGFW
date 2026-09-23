package df7

import (
	"context"
	"fmt"
	"sync"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
)

// Records of what this owner applied to the running VPP instance (D-076, D-080), in a
// dfkit.BootStore bound to the D-080 boot identity (kernel boot_id, VPP main PID, VPP start
// time — dfkit.BootIdentity):
//
//   - applied-once records of write-only objects whose VPP add is not idempotent (policer.interface,
//     lb.intf-nat stack a feature node on every enable): the value is "<sw_if_index>/<logical
//     name>", so a VPP restart, a reboot with a repeated PID and an interface re-created under the
//     same name all expire the record and the object is applied once more;
//   - ownership records of untagged global objects (lb VIPs/ASes, MPLS label routes in the shared
//     table 0): written only after our own successful add; an existing object without a record
//     is never adopted.
//
// P05 installs a persisted store (dfkit.NewFileBootStore in the state dir) with SetBootStore so an
// agent restart keeps them; the default is in memory.

var (
	bootMu sync.Mutex
	bootBy = map[string]dfkit.BootStore{}
)

// SetBootStore installs the BootStore of owner; nil restores a fresh in-memory one.
func SetBootStore(owner string, s dfkit.BootStore) {
	bootMu.Lock()
	defer bootMu.Unlock()
	if s == nil {
		s = dfkit.NewMemoryBootStore()
	}
	bootBy[owner] = s
}

// BootStoreFor returns the BootStore of owner.
func BootStoreFor(owner string) dfkit.BootStore {
	bootMu.Lock()
	defer bootMu.Unlock()
	s, ok := bootBy[owner]
	if !ok {
		s = dfkit.NewMemoryBootStore()
		bootBy[owner] = s
	}
	return s
}

// IfaceValue is the record value of a per-interface object: sw_if_index and logical name (D-080).
func IfaceValue(swIfIndex uint32, name string) string { return fmt.Sprintf("%d/%s", swIfIndex, name) }

// AppliedNow reports whether key was recorded with value on the running VPP instance.
func (b Base) AppliedNow(ctx context.Context, key, value string) (bool, error) {
	ok, _, err := dfkit.AppliedThisBoot(ctx, b.Client, BootStoreFor(b.Owner), scheduler.Key(key), value)
	return ok, err
}

// ApplyOnce runs apply unless key was already recorded with value on the running VPP instance,
// and records it afterwards (only when apply succeeded). skipped reports a skip.
func (b Base) ApplyOnce(ctx context.Context, key, value string, apply func() error) (skipped bool, err error) {
	ok, id, err := dfkit.AppliedThisBoot(ctx, b.Client, BootStoreFor(b.Owner), scheduler.Key(key), value)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	if err := apply(); err != nil {
		return false, err
	}
	return false, b.record(key, id, value)
}

func (b Base) record(key, id, value string) error {
	if err := BootStoreFor(b.Owner).Put(dfkit.BootRecord{Key: key, Identity: id, Value: value}); err != nil {
		return fmt.Errorf("%s: record %s: %w", b.name, key, err)
	}
	return nil
}

// RecordNow stores key/value for the running VPP instance (after our own successful add).
func (b Base) RecordNow(ctx context.Context, key, value string) error {
	id, err := dfkit.IdentitySource(ctx, b.Client) // bootid.Current via dfkit (D-080, TD-1)
	if err != nil {
		return err
	}
	return b.record(key, id.String(), value)
}

// Recorded reports whether key has a record (any value) on the running VPP instance.
func (b Base) Recorded(ctx context.Context, key string) (bool, error) {
	return dfkit.StartedThisBoot(ctx, b.Client, BootStoreFor(b.Owner), scheduler.Key(key))
}

// ForgetApplied drops the record of key (after Delete).
func (b Base) ForgetApplied(key string) error { return BootStoreFor(b.Owner).Delete(key) }
