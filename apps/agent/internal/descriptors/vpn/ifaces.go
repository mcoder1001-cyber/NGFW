package vpn

import (
	"context"
	"errors"
	"fmt"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp"
)

// NoInterface is the "no interface" sw_if_index (~0).
const NoInterface = ^uint32(0)

// Interface resolution errors (DF-1's resolver, D-069).
var (
	// ErrNoInterface: the logical name does not resolve (unknown, local0, or VPP's name of one of
	// our own interfaces).
	ErrNoInterface = iface.ErrNotFound
	// ErrForeignInterface: the name resolves only to an interface tagged by another owner — the
	// DF-5 descriptors never configure it (D-069, D-071).
	ErrForeignInterface = iface.ErrForeignInterface
	// ErrNotOurs: an object exists in VPP but this owner did not create it (no tag / no claim
	// record); it is never adopted (D-071).
	ErrNotOurs = errors.New("vpn: object exists in VPP but is not this owner's")
)

// Interfaces is one sw_interface_dump seen through DF-1's logical-name resolver (iface.Table,
// D-069): our interfaces are named by their owner-tag id, untagged ones (physical NICs) by VPP's
// name, other owners' interfaces have no name for us.
type Interfaces struct{ t *iface.Table }

// DumpInterfaces reads every interface (sw_interface_dump) for owner.
func DumpInterfaces(ctx context.Context, c vpp.Client, owner string) (*Interfaces, error) {
	t, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return nil, err
	}
	return &Interfaces{t: t}, nil
}

// Resolve resolves a logical interface name (iface.Table.IndexByName): our tag id first, then an
// untagged interface with that VPP name. Another owner's interface fails with
// ErrForeignInterface, anything else (local0 included) with ErrNoInterface.
func (t *Interfaces) Resolve(name string) (interface_types.InterfaceIndex, error) {
	idx, err := t.t.IndexByName(name)
	if err != nil {
		return 0, fmt.Errorf("vpn: interface %q: %w", name, err)
	}
	return interface_types.InterfaceIndex(idx), nil
}

// ResolveOwn resolves a logical name that must be one of OUR (tagged) interfaces: tunnel and
// WireGuard interfaces are always created by some owner, so an untagged one is not ours
// (ErrNotOurs).
func (t *Interfaces) ResolveOwn(name string) (interface_types.InterfaceIndex, error) {
	idx, err := t.Resolve(name)
	if err != nil {
		return 0, err
	}
	if _, ok := t.t.OwnedID(uint32(idx)); !ok {
		return 0, fmt.Errorf("%w: interface %q carries no %q tag", ErrNotOurs, name, t.t.Owner())
	}
	return idx, nil
}

// Logical returns the logical name of idx ("" for other owners' interfaces, local0, unknown).
func (t *Interfaces) Logical(idx uint32) string {
	n, _ := t.t.Logical(idx)
	return n
}

// Owned reports whether idx carries this owner's tag and returns the tag id (= logical name).
func (t *Interfaces) Owned(idx uint32) (string, bool) { return t.t.OwnedID(idx) }

// Untagged reports whether idx exists, carries no tag and is not local0.
func (t *Interfaces) Untagged(idx uint32) bool { return t.t.Untagged(idx) }

// Exists reports whether sw_if_index idx is in the dump.
func (t *Interfaces) Exists(idx uint32) bool {
	_, ok := t.t.Details(idx)
	return ok
}

// OwnedAt re-verifies, right before a delete by index, that idx still is our interface with tag
// id id (D-071: sw_if_indexes are reused after a VPP restart). ok=false with a nil error means
// the interface is gone (nothing to delete, D-074); an index now held by any other interface is
// ErrNotOurs.
func OwnedAt(ctx context.Context, c vpp.Client, owner string, idx uint32, id string) (bool, error) {
	t, err := DumpInterfaces(ctx, c, owner)
	if err != nil {
		return false, err
	}
	if !t.Exists(idx) {
		return false, nil
	}
	if got, ok := t.Owned(idx); !ok || got != id {
		return false, fmt.Errorf("%w: sw_if_index %d is no longer %q", ErrNotOurs, idx, id)
	}
	return true, nil
}

// TagInterface stamps the interface with the owner tag "<owner>:<id>" (sw_interface_tag_add_del).
func TagInterface(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, owner, id string) error {
	tag, err := vpp.OwnerTag(owner, id)
	if err != nil {
		return err
	}
	if _, err := interfaces.NewServiceClient(c).SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{
		IsAdd: true, SwIfIndex: idx, Tag: tag,
	}); err != nil {
		return fmt.Errorf("sw_interface_tag_add_del: %w", err)
	}
	return nil
}
