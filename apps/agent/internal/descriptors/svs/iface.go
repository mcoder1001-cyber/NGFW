package svs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/svs"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

// InterfaceDescriptor enables source VRF select on an interface for IPv4 and IPv6 with the interface's svs table
// (svs_enable_disable; VPP then adds 0/0 → "the interface's own table" to the svs table and puts the svs-ip4/svs-ip6
// feature on the unicast arcs). Enablement is not idempotent in VPP (a second enable stacks the feature), so Create
// enables only the families svs_dump does not list.
type InterfaceDescriptor struct{ Env }

var _ scheduler.Descriptor = (*InterfaceDescriptor)(nil)

// Name implements scheduler.Descriptor.
func (*InterfaceDescriptor) Name() string { return InterfaceName }

func asIface(obj proto.Message) *Interface {
	if v, ok := obj.(*Interface); ok && v != nil {
		return v
	}
	return &Interface{}
}

// KeyOf implements scheduler.Descriptor.
func (*InterfaceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return InterfaceKey(asIface(obj).GetInterface())
}

// Dependencies implements scheduler.Descriptor: the interface, the svs table and — optional, for ordering and so
// that a VRF rebind re-enables svs (the reconciler re-creates dependents around it) — the interface's VRF binding.
func (d *InterfaceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	v := asIface(obj)
	deps := []scheduler.Dependency{{Key: d.ifRef(v.GetInterface())}, {Key: TableKey(v.GetTableId())}}
	if d.IfTableRef != nil {
		deps = append(deps, scheduler.Dependency{Key: d.IfTableRef(v.GetInterface()), Optional: true})
	}
	return deps
}

// enablement is one svs_dump entry.
type enablement struct {
	table uint32
	v6    bool
}

// dumpEnabled returns svs_dump grouped by sw_if_index.
func (d *InterfaceDescriptor) dumpEnabled(ctx context.Context) (map[uint32][]enablement, error) {
	stream, err := svs.NewServiceClient(d.Client).SvsDump(ctx, &svs.SvsDump{})
	if err != nil {
		return nil, fmt.Errorf("svs_dump: %w", err)
	}
	out := map[uint32][]enablement{}
	for {
		e, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("svs_dump: %w", err)
		}
		out[uint32(e.SwIfIndex)] = append(out[uint32(e.SwIfIndex)], enablement{table: e.TableID, v6: e.Af == ip_types.ADDRESS_IP6})
	}
}

func (d *InterfaceDescriptor) send(ctx context.Context, idx, table uint32, v6, enable bool) error {
	if _, err := svs.NewServiceClient(d.Client).SvsEnableDisable(ctx, &svs.SvsEnableDisable{IsEnable: enable, Af: af(v6), TableID: table, SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
		return fmt.Errorf("svs_enable_disable sw_if_index %d table %d ipv6=%v enable=%v: %w", idx, table, v6, enable, err)
	}
	return nil
}

// resolve returns the sw_if_index of the logical interface name (our tag id, else an untagged VPP name; never
// another owner's interface).
func (d *InterfaceDescriptor) resolve(ctx context.Context, name string) (uint32, error) {
	t, err := iface.Dump(ctx, d.Client, d.Owner)
	if err != nil {
		return 0, err
	}
	return t.IndexByName(name)
}

// ensure enables the families of v that are not enabled; a family enabled with another table is a conflict.
func (d *InterfaceDescriptor) ensure(ctx context.Context, v *Interface) error {
	idx, err := d.resolve(ctx, v.GetInterface())
	if err != nil {
		return err
	}
	all, err := d.dumpEnabled(ctx)
	if err != nil {
		return err
	}
	have := map[bool]uint32{}
	for _, e := range all[idx] {
		have[e.v6] = e.table
	}
	for _, v6 := range []bool{false, true} {
		if t, ok := have[v6]; ok && t != v.GetTableId() {
			return fmt.Errorf("%w: %s ipv6=%v uses table %d, want %d", ErrInterfaceConflict, v.GetInterface(), v6, t, v.GetTableId())
		}
	}
	var done []bool
	for _, v6 := range []bool{false, true} {
		if _, ok := have[v6]; ok {
			continue
		}
		if err := d.send(ctx, idx, v.GetTableId(), v6, true); err != nil {
			for _, f := range done { // never half enabled
				_ = d.send(context.WithoutCancel(ctx), idx, v.GetTableId(), f, false)
			}
			return err
		}
		done = append(done, v6)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *InterfaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	v, ok := obj.(*Interface)
	if !ok {
		return nil, fmt.Errorf("%w %T", ErrBadValue, obj)
	}
	return nil, d.ensure(ctx, v)
}

// Update implements scheduler.Descriptor: another table needs disable + enable; a missing family is repaired.
func (d *InterfaceDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, n := asIface(oldObj), asIface(newObj)
	if o.GetTableId() != n.GetTableId() || o.GetInterface() != n.GetInterface() {
		return nil, scheduler.ErrRecreate
	}
	return meta, d.ensure(ctx, n)
}

// Delete implements scheduler.Descriptor: disables the families still enabled with our table (the index is
// re-resolved right before; a vanished interface is not an error).
func (d *InterfaceDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	v := asIface(obj)
	idx, err := d.resolve(ctx, v.GetInterface())
	if err != nil {
		if errors.Is(err, iface.ErrNotFound) {
			return nil
		}
		return err
	}
	all, err := d.dumpEnabled(ctx)
	if err != nil {
		return err
	}
	for _, e := range all[idx] {
		if e.table != v.GetTableId() {
			continue
		}
		if err := d.send(ctx, idx, e.table, e.v6, false); err != nil {
			return err
		}
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: enablements whose table is one of ours, by logical interface name.
func (d *InterfaceDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	owned, err := ownedTables(ctx, d.Client, d.Owner)
	if err != nil || len(owned) == 0 {
		return nil, err
	}
	all, err := d.dumpEnabled(ctx)
	if err != nil || len(all) == 0 {
		return nil, err
	}
	t, err := iface.Dump(ctx, d.Client, d.Owner)
	if err != nil {
		return nil, err
	}
	byName := map[string]*Interface{}
	for idx, es := range all {
		name, ok := t.Logical(idx)
		if !ok {
			continue
		}
		for _, e := range es {
			if _, ours := owned[e.table]; !ours {
				continue
			}
			v := byName[name]
			if v == nil {
				v = &Interface{Interface: name, TableId: e.table, MissingIp4: true, MissingIp6: true}
				byName[name] = v
			}
			if e.table != v.TableId {
				continue // two of our tables on one interface cannot happen through Create; report the first
			}
			if e.v6 {
				v.MissingIp6 = false
			} else {
				v.MissingIp4 = false
			}
		}
	}
	out := make([]scheduler.KV, 0, len(byName))
	for name, v := range byName {
		out = append(out, scheduler.KV{Key: InterfaceKey(name), Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
