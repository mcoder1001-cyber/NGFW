package dfkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ErrNoInterface is returned (wrapped) when a desired object names an interface VPP does not have.
var ErrNoInterface = errors.New("no such interface")

// ErrNotOwned is returned (wrapped) when a desired object names an interface that is not tagged
// with this agent's owner: the descriptors never configure another owner's interfaces.
var ErrNotOwned = errors.New("interface not owned by this agent")

// AllInterfaces is the sw_if_index wildcard (~0) of dump requests.
const AllInterfaces = ^uint32(0)

// Iface is what the DF-8 descriptors need from sw_interface_details.
type Iface struct {
	Index    uint32
	SupIndex uint32
	Name     string
	Tag      string
}

// Ifaces maps interface names to indexes and back for one call.
type Ifaces struct {
	ByIndex map[uint32]Iface
	ByName  map[string]Iface
}

// DumpInterfaces runs sw_interface_dump for all interfaces. The DF-8 objects are keyed by
// interface name (stable across restarts) while VPP speaks sw_if_index.
func DumpInterfaces(ctx context.Context, c vpp.Client) (*Ifaces, error) {
	stream, err := interfaces.NewServiceClient(c).SwInterfaceDump(ctx,
		&interfaces.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(AllInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_dump: %w", err)
	}
	t := &Ifaces{ByIndex: map[uint32]Iface{}, ByName: map[string]Iface{}}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return t, nil
		}
		if err != nil {
			_ = stream.Close()
			return nil, fmt.Errorf("sw_interface_dump: %w", err)
		}
		i := Iface{
			Index:    uint32(d.SwIfIndex),
			SupIndex: d.SupSwIfIndex,
			Name:     strings.TrimRight(d.InterfaceName, "\x00"),
			Tag:      strings.TrimRight(d.Tag, "\x00"),
		}
		t.ByIndex[i.Index] = i
		t.ByName[i.Name] = i
	}
}

// Owned reports whether the interface carries this owner's tag (vpp.OwnerTag).
func (i Iface) Owned(owner string) bool {
	_, ok := vpp.ParseOwnerTag(i.Tag, owner)
	return ok
}

// Untagged reports whether the interface carries no tag and is not local0 (a physical or
// pre-existing interface: usable through a claim, D-071).
func (i Iface) Untagged() bool { return i.Tag == "" && i.Index != 0 && i.Name != "local0" }

// Resolve is the logical-name resolver of D-069 (mirrors DF-1's iface.Table.IndexByName until
// DF-1 is on main): this owner's tag id first, then an untagged interface with that VPP name.
// A name that only matches another owner's interface fails with ErrNotOwned; anything else
// (including local0) with ErrNoInterface.
func (t *Ifaces) Resolve(name, owner string) (uint32, error) {
	if name == "" || name == "local0" {
		return 0, fmt.Errorf("%w: %q", ErrNoInterface, name)
	}
	for _, i := range t.ByIndex {
		if id, ok := vpp.ParseOwnerTag(i.Tag, owner); ok && id == name {
			return i.Index, nil
		}
	}
	i, ok := t.ByName[name]
	switch {
	case !ok:
		return 0, fmt.Errorf("%w: %q (owner %q)", ErrNoInterface, name, owner)
	case i.Untagged():
		return i.Index, nil
	case i.Owned(owner):
		id, _ := vpp.ParseOwnerTag(i.Tag, owner)
		return 0, fmt.Errorf("%w: %q is VPP's name of our interface %q; use the logical name", ErrNoInterface, name, id)
	default:
		return 0, fmt.Errorf("%w: %q (tag %q, owner %q)", ErrNotOwned, name, i.Tag, owner)
	}
}

// Logical returns the logical name of idx for this owner (D-069): the owner-tag id of our
// interfaces, VPP's name for untagged ones (untagged=true); ok=false for other owners'
// interfaces, local0 and unknown indexes.
func (t *Ifaces) Logical(idx uint32, owner string) (name string, untagged, ok bool) {
	i, found := t.ByIndex[idx]
	if !found {
		return "", false, false
	}
	if id, mine := vpp.ParseOwnerTag(i.Tag, owner); mine {
		return id, false, true
	}
	if i.Untagged() {
		return i.Name, true, true
	}
	return "", false, false
}

// Reportable returns the logical name of idx when an object of descriptor holder on it belongs to
// this agent: always on our tagged interfaces, on untagged ones only with a claim (D-071).
func (t *Ifaces) Reportable(idx uint32, owner, holder string) (string, bool) {
	name, untagged, ok := t.Logical(idx, owner)
	if !ok || (untagged && !Claims(owner).Claimed(name, holder)) {
		return "", false
	}
	return name, true
}

// OwnedName is Reportable without claims: the logical name of our tagged interface idx.
func (t *Ifaces) OwnedName(idx uint32, owner string) (string, bool) {
	name, untagged, ok := t.Logical(idx, owner)
	return name, ok && !untagged
}

// ResolveInterface is DumpInterfaces + Resolve for one name.
func ResolveInterface(ctx context.Context, c vpp.Client, name, owner string) (uint32, error) {
	t, err := DumpInterfaces(ctx, c)
	if err != nil {
		return 0, err
	}
	return t.Resolve(name, owner)
}

// ResolveAndClaim resolves name and, when it is an untagged interface, records the claim
// (name, holder) in the owner's ClaimStore so Retrieve reports the object (D-071).
func ResolveAndClaim(ctx context.Context, c vpp.Client, name, owner, holder string) (uint32, error) {
	t, err := DumpInterfaces(ctx, c)
	if err != nil {
		return 0, err
	}
	idx, err := t.Resolve(name, owner)
	if err != nil {
		return 0, err
	}
	if t.ByIndex[idx].Untagged() {
		if err := Claims(owner).Claim(name, holder); err != nil {
			return 0, fmt.Errorf("claim %s for %s: %w", name, holder, err)
		}
	}
	return idx, nil
}

// KeyFunc maps an object id (an interface name, a VRF id, a classify table id) to the key of the
// object another descriptor owns, for Dependencies.
type KeyFunc func(id string) scheduler.Key

// DefaultInterfaceKey is the interface key scheme assumed until DF-1 is merged: "interface/<name>".
func DefaultInterfaceKey(name string) scheduler.Key { return scheduler.Join("interface", name) }

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
