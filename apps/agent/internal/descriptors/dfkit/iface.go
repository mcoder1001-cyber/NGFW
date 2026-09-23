package dfkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ErrNoInterface is returned (wrapped) when a desired object names an interface this owner
// cannot resolve (DF-1's iface.ErrNotFound: unknown, local0, or VPP's name of our own interface).
var ErrNoInterface = iface.ErrNotFound

// ErrNotOwned is returned (wrapped) when a name resolves only to an interface tagged by another
// owner (DF-1's iface.ErrForeignInterface): the descriptors never configure it.
var ErrNotOwned = iface.ErrForeignInterface

// AllInterfaces is the sw_if_index wildcard (~0) of dump requests.
const AllInterfaces = iface.AllInterfaces

// Iface is what the DF-8 descriptors need from sw_interface_details.
type Iface struct {
	Index    uint32
	SupIndex uint32
	Name     string
	Tag      string
}

// Ifaces is one sw_interface_dump seen through DF-1's resolver (iface.Table, D-069): logical
// names, owner tags and the owner's ClaimStore for untagged interfaces (D-071).
type Ifaces struct {
	T       *iface.Table
	ByIndex map[uint32]Iface
	ByName  map[string]Iface

	ctx      context.Context //nolint:containedctx // one Retrieve call's context, for the lazy identity
	client   vpp.Client
	identity string
}

// DumpInterfaces runs sw_interface_dump for all interfaces (iface.Dump) for owner.
func DumpInterfaces(ctx context.Context, c vpp.Client, owner string) (*Ifaces, error) {
	t, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return nil, err
	}
	out := &Ifaces{T: t, ByIndex: map[uint32]Iface{}, ByName: map[string]Iface{}, ctx: ctx, client: c}
	for _, idx := range t.Indexes() {
		d, _ := t.Details(idx)
		i := Iface{
			Index: idx, SupIndex: d.SupSwIfIndex,
			Name: strings.TrimRight(d.InterfaceName, "\x00"), Tag: strings.TrimRight(d.Tag, "\x00"),
		}
		out.ByIndex[idx] = i
		out.ByName[i.Name] = i
	}
	return out, nil
}

// Resolve is DF-1's logical-name resolver (iface.Table.IndexByName): this owner's tag id first,
// then an untagged interface with that VPP name; ErrNotOwned for another owner's interface,
// ErrNoInterface for anything else (including local0).
func (t *Ifaces) Resolve(name string) (uint32, error) { return t.T.IndexByName(name) }

// Reportable returns the logical name of idx when an object of descriptor holder on it is this
// agent's: always on our tagged interfaces, on untagged ones only with holder's claim made on the
// running VPP instance for this sw_if_index (D-071, D-080). The boot identity is read once per
// Ifaces, only when an untagged interface is asked about; if it cannot be read nothing untagged is
// reported.
func (t *Ifaces) Reportable(idx uint32, holder string) (string, bool) {
	name, ok := t.T.Logical(idx)
	if !ok {
		return "", false
	}
	if !t.T.Untagged(idx) {
		return name, true
	}
	if t.identity == "" {
		id, err := IdentitySource(t.ctx, t.client)
		if err != nil {
			return "", false
		}
		t.identity = id
	}
	tg := Target{Name: name, Index: idx, Untagged: true, Owner: t.T.Owner(), Holder: holder, Identity: t.identity}
	return name, tg.Claimed()
}

// Logical returns the logical name of idx (our tag id, or VPP's name when untagged).
func (t *Ifaces) Logical(idx uint32) (string, bool) { return t.T.Logical(idx) }

// ResolveInterface resolves a logical name (iface.ResolveName) without claiming.
func ResolveInterface(ctx context.Context, c vpp.Client, name, owner string) (uint32, error) {
	return iface.ResolveName(ctx, c, owner, name)
}

// Target is a resolved interface for a per-interface object of descriptor Holder.
type Target struct {
	Name     string
	Index    uint32
	Untagged bool
	Owner    string
	Holder   string
	Identity string // D-080 boot identity at resolution time
}

// claimHolder qualifies the claim with the VPP boot identity and the sw_if_index (D-080): a claim
// made on another VPP instance, or on another interface that now has the same name, never matches
// and so has expired. (Stale entries of earlier instances stay in the store as inert garbage.)
func (t Target) claimHolder() string {
	return fmt.Sprintf("%s@%s#%d", t.Holder, t.Identity, t.Index)
}

// ResolveTarget resolves a logical name (D-069) for holder. Nothing is claimed here: the claim is
// recorded only after VPP accepted the add (Claim), so a failed add never leaves one (review H1).
func ResolveTarget(ctx context.Context, c vpp.Client, name, owner, holder string) (Target, error) {
	t, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return Target{}, err
	}
	idx, err := t.IndexByName(name)
	if err != nil {
		return Target{}, err
	}
	tg := Target{Name: name, Index: idx, Untagged: t.Untagged(idx), Owner: owner, Holder: holder}
	if tg.Untagged {
		if tg.Identity, err = IdentitySource(ctx, c); err != nil {
			return Target{}, err
		}
	}
	return tg, nil
}

// Claimed reports whether the object on this interface is this holder's: always on our tagged
// interfaces, on an untagged one only with an existing claim.
func (t Target) Claimed() bool {
	return !t.Untagged || iface.Claims(t.Owner).Claimed(t.Name, t.claimHolder())
}

// Claim records the claim after a successful add (no-op on tagged interfaces).
func (t Target) Claim() error {
	if !t.Untagged {
		return nil
	}
	if err := iface.Claims(t.Owner).Claim(t.Name, t.claimHolder()); err != nil {
		return fmt.Errorf("claim %s for %s: %w", t.Name, t.Holder, err)
	}
	return nil
}

// Release drops the claim (after a delete).
func (t Target) Release() error {
	if !t.Untagged {
		return nil
	}
	return iface.Claims(t.Owner).Release(t.Name, t.claimHolder())
}

// Adopt is the "already exists" path of a Create: an existing object may be treated as ours
// only if we created it before (our tagged interface, or our claim survived an agent restart);
// otherwise the result is ErrNotOurs and nothing is claimed.
func (t Target) Adopt() error {
	if t.Claimed() {
		return nil
	}
	return fmt.Errorf("%w: %s on untagged interface %q has no claim of %s", ErrNotOurs, t.Holder, t.Name, t.Owner)
}

// VerifyIndex re-resolves name right before a delete by index (D-071/D-074: sw_if_indexes are
// reused after a VPP restart). It returns the current index of name, or ok=false when name no
// longer resolves (the object went with its interface).
func VerifyIndex(ctx context.Context, c vpp.Client, name, owner string) (uint32, bool, error) {
	idx, err := iface.ResolveName(ctx, c, owner, name)
	switch {
	case errors.Is(err, iface.ErrNotFound):
		return 0, false, nil
	case err != nil:
		return 0, false, err
	}
	return idx, true, nil
}

// Claims returns the owner's ClaimStore (DF-1's iface.Claims; P05/P08 install a persisted one
// with iface.SetClaimStore).
func Claims(owner string) iface.ClaimStore { return iface.Claims(owner) }

// KeyFunc maps an object id (an interface name, a VRF id, a classify table id) to the key of the
// object another descriptor owns, for Dependencies.
type KeyFunc func(id string) scheduler.Key

// DefaultInterfaceKey is DF-1's alias key "interface/<logical name>" (D-065, D-069).
func DefaultInterfaceKey(name string) scheduler.Key { return iface.AliasKey(name) }

// DefaultVRFKey is the VRF key scheme of the DF-8 prompt: "vrf/<id>".
func DefaultVRFKey(id string) scheduler.Key { return scheduler.Join("vrf", id) }

// Drain reads a generated dump stream until io.EOF (the control_ping_reply). On any other error
// the stream is closed and the error returned.
func Drain[T any](stream interface{ Close() error }, recv func() (T, error)) ([]T, error) {
	var out []T
	for {
		d, err := recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			_ = stream.Close()
			return nil, err
		}
		out = append(out, d)
	}
}

// ResolveForDelete re-resolves the logical name right before a delete by sw_if_index (indexes are
// reused after a VPP restart, D-071/D-074). ok is false when there is nothing of ours to delete:
// the interface is gone, or it is untagged and this holder has no claim on it for the running VPP
// instance (a foreign object is never deleted, review H1).
func ResolveForDelete(ctx context.Context, c vpp.Client, name, owner, holder string) (Target, bool, error) {
	tg, err := ResolveTarget(ctx, c, name, owner, holder)
	switch {
	case errors.Is(err, iface.ErrNotFound):
		return Target{}, false, nil
	case err != nil:
		return Target{}, false, err
	}
	return tg, tg.Claimed(), nil
}
