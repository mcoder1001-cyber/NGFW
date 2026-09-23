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

// Resolve returns the sw_if_index of an owned interface by name. The error wraps
// ErrNoInterface or ErrNotOwned.
func (t *Ifaces) Resolve(name, owner string) (uint32, error) {
	i, ok := t.ByName[name]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrNoInterface, name)
	}
	if !i.Owned(owner) {
		return 0, fmt.Errorf("%w: %q (tag %q, owner %q)", ErrNotOwned, name, i.Tag, owner)
	}
	return i.Index, nil
}

// OwnedName returns the name of the interface with sw_if_index idx when it is owned.
func (t *Ifaces) OwnedName(idx uint32, owner string) (string, bool) {
	i, ok := t.ByIndex[idx]
	if !ok || !i.Owned(owner) {
		return "", false
	}
	return i.Name, true
}

// ResolveInterface is DumpInterfaces + Resolve for one name.
func ResolveInterface(ctx context.Context, c vpp.Client, name, owner string) (uint32, error) {
	t, err := DumpInterfaces(ctx, c)
	if err != nil {
		return 0, err
	}
	return t.Resolve(name, owner)
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
