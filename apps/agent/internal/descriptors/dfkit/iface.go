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
}

// DumpInterfaces runs sw_interface_dump for all interfaces (iface.Dump) for owner.
func DumpInterfaces(ctx context.Context, c vpp.Client, owner string) (*Ifaces, error) {
	t, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return nil, err
	}
	out := &Ifaces{T: t, ByIndex: map[uint32]Iface{}, ByName: map[string]Iface{}}
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
// agent's: always on our tagged interfaces, on untagged ones only with holder's claim (D-071).
func (t *Ifaces) Reportable(idx uint32, holder string) (string, bool) {
	if !t.T.Owns(idx, holder) {
		return "", false
	}
	return t.T.Logical(idx)
}

// Logical returns the logical name of idx (our tag id, or VPP's name when untagged).
func (t *Ifaces) Logical(idx uint32) (string, bool) { return t.T.Logical(idx) }

// ResolveInterface resolves a logical name (iface.ResolveName) without claiming.
func ResolveInterface(ctx context.Context, c vpp.Client, name, owner string) (uint32, error) {
	return iface.ResolveName(ctx, c, owner, name)
}

// ResolveAndClaim resolves name and, when it is an untagged interface, records holder's claim in
// the owner's ClaimStore (iface.Claims, shared with DF-1) so Retrieve reports the object (D-071).
func ResolveAndClaim(ctx context.Context, c vpp.Client, name, owner, holder string) (uint32, error) {
	t, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return 0, err
	}
	idx, err := t.IndexByName(name)
	if err != nil {
		return 0, err
	}
	if err := t.ClaimIfUntagged(idx, holder); err != nil {
		return 0, fmt.Errorf("claim %s for %s: %w", name, holder, err)
	}
	return idx, nil
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

// DefaultClassifyTableKey is the classify table key scheme assumed until DF-2 is merged:
// "classify-table/<id>".
func DefaultClassifyTableKey(id string) scheduler.Key { return scheduler.Join("classify-table", id) }

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
