package iface

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Interface references
//
// Every descriptor that decorates or consumes an interface (interface.* attributes, l2.*,
// bond.member, l3xc, …) names it by a reference: the alias "interface/<logical name>" (canonical,
// see names.go) or the creator's full key, e.g. "interface.loopback/loop201" or
// "tapv2.tap/w2-tap0". The key's ID() is the interface's stable name; the interface-creating
// descriptor stamps it into the VPP interface tag as vpp.OwnerTag(owner, ID) ("w2:loop201",
// D-030). Resolution therefore never depends on VPP's own interface names (tap0, BondEthernet3,
// memif1/0 …), which are index-based and not stable across restarts:
//
//   - at apply time Table.Index finds the sw_if_index whose tag is "<owner>:<ID>";
//   - at Retrieve time Table.KeyFor rebuilds the full key from the tag and the interface's VPP
//     device class (sw_interface_details.interface_dev_type → descriptor name, table below).
//
// Interface names are ONE namespace across all interface types, exactly as in VPP.

// Descriptor names of the interface-creating descriptors. LoopbackName is P05 core's.
const (
	LoopbackName      = "interface.loopback"
	SubinterfaceName  = "interface.subinterface"
	TapName           = "tapv2.tap"
	HostInterfaceName = "af-packet.host-interface"
	BondName          = "bond.bond"
	MemifName         = "memif.memif"
)

// kindByDevType maps sw_interface_details.interface_dev_type (vnet_device_class_t.name in VPP
// 26.06: src/vnet/ethernet/interface.c "Loopback", src/plugins/tap/tap.c "tap",
// src/plugins/af_packet/device.c "af-packet", src/vnet/bonding/device.c "bond",
// src/plugins/memif/device.c "memif"; verified on the host, see docs/agent/descriptors/interface.md)
// to the descriptor that creates such interfaces. Sub-interfaces are recognised by IF_API_TYPE_SUB.
var kindByDevType = map[string]string{
	"Loopback":  LoopbackName,
	"tap":       TapName,
	"af-packet": HostInterfaceName,
	"bond":      BondName,
	"memif":     MemifName,
}

// RegisterKind lets another plugin's descriptor (tunnels, …) declare which descriptor owns
// interfaces of a VPP device class so that interface.* attributes can decorate them. It panics on
// a conflicting registration (programming error at start-up).
func RegisterKind(devType, descriptor string) {
	if cur, ok := kindByDevType[devType]; ok && cur != descriptor {
		panic(fmt.Sprintf("iface: device class %q already mapped to %q, not %q", devType, cur, descriptor))
	}
	kindByDevType[devType] = descriptor
}

// Meta is the runtime handle interface-shaped descriptors store: the sw_if_index.
type Meta struct{ SwIfIndex uint32 }

// AllInterfaces is the sw_if_index wildcard of sw_interface_dump and friends.
const AllInterfaces = ^uint32(0)

// Errors.
var (
	ErrNotFound   = errors.New("iface: no owned interface with this id")
	ErrBadRef     = errors.New("iface: interface reference is not a full scheduler key")
	ErrWrongKind  = errors.New("iface: interface exists but is of another type")
	ErrBadMeta    = errors.New("iface: unexpected meta type")
	ErrEmptyValue = errors.New("iface: nil or wrong desired value type")
)

// ParseRef validates an interface reference: "<descriptor>/<id>" with both parts non-empty.
func ParseRef(ref string) (scheduler.Key, error) {
	k := scheduler.Key(ref)
	if !strings.Contains(ref, scheduler.KeySeparator) || k.Descriptor() == "" || k.ID() == "" {
		return "", fmt.Errorf("%w: %q", ErrBadRef, ref)
	}
	return k, nil
}

// RefID returns the id part of a reference for KeyOf (which cannot fail): the ID() of a valid
// reference, or the raw string so the key is still unique and the error surfaces in Create.
func RefID(ref string) string {
	if k, err := ParseRef(ref); err == nil {
		return k.ID()
	}
	return ref
}

// Table is one full sw_interface_dump indexed for the lookups descriptors need. It is
// immutable after Dump and safe for concurrent reads.
type Table struct {
	owner   string
	byIndex map[uint32]*ifapi.SwInterfaceDetails
	order   []uint32
}

// Dump performs sw_interface_dump over all interfaces.
func Dump(ctx context.Context, client vpp.Client, owner string) (*Table, error) {
	stream, err := ifapi.NewServiceClient(client).SwInterfaceDump(ctx, &ifapi.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(AllInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_dump: %w", err)
	}
	t := &Table{owner: owner, byIndex: make(map[uint32]*ifapi.SwInterfaceDetails)}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("sw_interface_dump: %w", err)
		}
		idx := uint32(d.SwIfIndex)
		t.byIndex[idx] = d
		t.order = append(t.order, idx)
	}
	sort.Slice(t.order, func(i, j int) bool { return t.order[i] < t.order[j] })
	return t, nil
}

// Owner returns the owner the table filters by.
func (t *Table) Owner() string { return t.owner }

// Indexes returns every dumped sw_if_index in ascending order.
func (t *Table) Indexes() []uint32 { return append([]uint32(nil), t.order...) }

// Details returns the dump row of idx.
func (t *Table) Details(idx uint32) (*ifapi.SwInterfaceDetails, bool) {
	d, ok := t.byIndex[idx]
	return d, ok
}

// Kind returns the descriptor name for the interface's type, or "" when unknown.
func Kind(d *ifapi.SwInterfaceDetails) string {
	if d.Type == interface_types.IF_API_TYPE_SUB || (d.SupSwIfIndex != uint32(d.SwIfIndex) && d.SubID != 0) {
		return SubinterfaceName
	}
	return kindByDevType[strings.TrimRight(d.InterfaceDevType, "\x00")]
}

// OwnedID returns the id from the interface's owner tag, false for other owners / untagged.
func (t *Table) OwnedID(idx uint32) (string, bool) {
	d, ok := t.byIndex[idx]
	if !ok {
		return "", false
	}
	return vpp.ParseOwnerTag(d.Tag, t.owner)
}

// KeyFor rebuilds the full scheduler key of an owned interface of a known type. It is false for
// other owners' interfaces, untagged interfaces (local0, ens*) and device classes no descriptor
// has claimed.
func (t *Table) KeyFor(idx uint32) (scheduler.Key, bool) {
	id, ok := t.OwnedID(idx)
	if !ok {
		return "", false
	}
	kind := Kind(t.byIndex[idx])
	if kind == "" {
		return "", false
	}
	return scheduler.Join(kind, id), true
}

// Index resolves an interface reference to its sw_if_index. An alias reference
// ("interface/<name>") resolves by logical name (IndexByName); a creator key by owner tag, and
// when the interface's device class is known it must match the key's descriptor (ErrWrongKind).
func (t *Table) Index(ref string) (uint32, error) {
	k, err := ParseRef(ref)
	if err != nil {
		return 0, err
	}
	if k.Descriptor() == AliasName { // "interface/<logical name>" (D-065, D-069)
		return t.IndexByName(k.ID())
	}
	for _, idx := range t.order {
		id, ok := t.OwnedID(idx)
		if !ok || id != k.ID() {
			continue
		}
		if kind := Kind(t.byIndex[idx]); kind != "" && kind != k.Descriptor() {
			return 0, fmt.Errorf("%w: %q is %s", ErrWrongKind, ref, kind)
		}
		return idx, nil
	}
	return 0, fmt.Errorf("%w: %q (owner %q)", ErrNotFound, ref, t.owner)
}

// Resolve is Dump followed by Index for a single reference.
func Resolve(ctx context.Context, client vpp.Client, owner, ref string) (uint32, error) {
	t, err := Dump(ctx, client, owner)
	if err != nil {
		return 0, err
	}
	return t.Index(ref)
}

// MetaOf asserts meta is a Meta.
func MetaOf(meta any) (Meta, error) {
	m, ok := meta.(Meta)
	if !ok {
		return Meta{}, fmt.Errorf("%w: %T", ErrBadMeta, meta)
	}
	return m, nil
}

// Tag stamps the owner tag "<owner>:<id>" on sw_if_index (sw_interface_tag_add_del).
func Tag(ctx context.Context, client vpp.Client, owner, id string, swIfIndex uint32) error {
	tag, err := vpp.OwnerTag(owner, id)
	if err != nil {
		return err
	}
	_, err = ifapi.NewServiceClient(client).SwInterfaceTagAddDel(ctx, &ifapi.SwInterfaceTagAddDel{
		IsAdd: true, SwIfIndex: interface_types.InterfaceIndex(swIfIndex), Tag: tag,
	})
	if err != nil {
		return fmt.Errorf("sw_interface_tag_add_del: %w", err)
	}
	return nil
}
