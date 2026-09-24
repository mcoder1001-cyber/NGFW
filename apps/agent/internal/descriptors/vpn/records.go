package vpn

import (
	"context"
	"errors"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// Ownership records (D-071, D-076, D-080).
//
// SPDs and SAs carry neither a tag nor a name, only a numeric id the desired state chooses, and
// VPP reuses ids across restarts; SPD bindings on untagged (physical) interfaces carry no tag
// either. Such an object is this owner's only while a Record says so:
//
//   - a record is written only after VPP accepted OUR add (never before it, never for an object
//     that already existed — existing objects are never adopted);
//   - it is bound to the D-080 VPP boot identity (kernel boot_id, VPP main PID, VPP start time):
//     after a VPP restart or a host reboot every record has expired, and the object is ours again
//     only when our own Create re-adds it;
//   - Delete re-reads the record and the object right before deleting by id.
//
// The store is the owner's persisted dfkit.BootStore (P05/P08 pass dfkit.NewFileBootStore in the
// agent state dir through each package's WithBootStore option), so an agent restart keeps its
// objects; the default is in memory (then a restarted agent does not recognise, and never deletes,
// what its predecessor created — it fails to re-create it instead of adopting it).
//
// The same store holds the D-076 applied-once records of write-only objects whose VPP setter is
// not idempotent (ikev2.responder-hostname).

// IdentitySource returns the D-080 boot identity of the connected VPP (complete: kernel boot id and
// VPP start time readable). Unit tests replace it (the fake VPP's PID is not a real process).
var IdentitySource = dfkit.BootIdentity

// Records reads and writes ownership records of one owner.
type Records struct {
	Client vpp.Client
	Store  dfkit.BootStore
}

// Identity returns the current boot identity (once per Retrieve / operation).
func (r Records) Identity(ctx context.Context) (bootid.Identity, error) {
	return IdentitySource(ctx, r.Client)
}

// Valid returns the value recorded for key on the VPP instance id (ok=false: no record, or a
// record of an earlier VPP instance).
func (r Records) Valid(id bootid.Identity, key string) (string, bool) {
	if r.Store == nil {
		return "", false
	}
	rec, ok := r.Store.Get(key)
	if !ok || !bootid.Matches(rec.Identity, id) {
		return "", false
	}
	return rec.Value, true
}

// Put records value for key on the VPP instance id — call it only after VPP accepted our add.
func (r Records) Put(id bootid.Identity, key, value string) error {
	if r.Store == nil {
		return ErrNoRecordStore
	}
	return r.Store.Put(dfkit.BootRecord{Key: key, Identity: id.String(), Value: value})
}

// Drop removes key's record (after our delete, or when the object is gone).
func (r Records) Drop(key string) error {
	if r.Store == nil {
		return nil
	}
	return r.Store.Delete(key)
}

// ErrNoRecordStore is returned when a descriptor that needs ownership records was constructed
// without a store (Config.Boot; Register always sets one).
var ErrNoRecordStore = errors.New("vpn: no ownership record store configured (Config.Boot)")
