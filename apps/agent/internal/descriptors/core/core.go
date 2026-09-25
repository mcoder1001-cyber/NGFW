// Package core holds the P05 core descriptors — the minimum that proves the reconciler end to
// end (prompts/P05-agent-core.md step 3):
//
//	descriptor            key                                  VPP messages (binapi)
//	interface.loopback    interface.loopback/<loopN>           create_loopback_instance, delete_loopback, sw_interface_tag_add_del, sw_interface_dump
//	interface-ip.table    interface-ip.table/<if>              sw_interface_set_table, sw_interface_get_table
//	interface-ip          interface-ip/<if>/<addr>/<len>       sw_interface_add_del_address, ip_address_dump
//	vrf                   vrf/<table id>                       ip_table_add_del, ip_table_dump
//	ip.route              ip.route/<table id>/<prefix>         ip_route_add_del, ip_route_dump
//
// Cross-plugin keys (D-065): consumers reference an interface through the generic alias
// "interface/<name>" owned by DF-1 (its Dependencies point at the creator key). The product agent
// registers that alias and passes Env.IfRef = AliasInterfaceRef (internal/subsystems, P08). A nil
// IfRef (standalone core tests) falls back to DirectInterfaceRef: the creator key
// "interface.loopback/<name>" for loopbacks, "interface/<name>" otherwise. A VRF is "vrf/<id>".
//
// Ownership (docs/contracts/proto.md §6): loopbacks carry the interface tag "<owner>:<name>";
// addresses and table bindings belong to us when their interface does; VRF tables are named
// "<owner>:<vrf name>" in VPP; routes (no tag field) are recorded in the owner table in the state
// dir (internal/ownertable) and Retrieve reports only recorded routes that exist in VPP.
package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"strconv"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	LoopbackName       = "interface.loopback"
	InterfaceTableName = "interface-ip.table"
	InterfaceAddrName  = "interface-ip"
	VRFName            = "vrf"
	RouteName          = "ip.route"
	// InterfaceRef is the descriptor segment of the generic interface reference key
	// "interface/<name>" (an alias provided by interface-creating descriptors).
	InterfaceRef = "interface"
)

// Errors.
var (
	ErrNotOwned  = errors.New("core: interface is not owned by this agent")
	ErrBadValue  = errors.New("core: unexpected value type")
	ErrBadPrefix = errors.New("core: invalid prefix")
)

// Env is what every core descriptor needs: the VPP client, the owner and the owner table.
type Env struct {
	Client vpp.Client
	Owner  string
	Owned  ownertable.Set
	// IfRef maps an interface name to the key core objects depend on (nil = DirectInterfaceRef;
	// AliasInterfaceRef once DF-1's "interface" alias descriptor is registered, D-065).
	IfRef func(name string) scheduler.Key
}

// DirectInterfaceRef references loopbacks by their creator key and every other interface by the
// generic alias key.
func DirectInterfaceRef(name string) scheduler.Key {
	if _, ok := LoopbackInstance(name); ok {
		return LoopbackKey(name)
	}
	return InterfaceKey(name)
}

// AliasInterfaceRef references every interface by the generic alias "interface/<name>" (D-065).
func AliasInterfaceRef(name string) scheduler.Key { return InterfaceKey(name) }

func (e Env) ifRef(name string) scheduler.Key {
	if e.IfRef != nil {
		return e.IfRef(name)
	}
	return DirectInterfaceRef(name)
}

// Register registers the core descriptors in dependency-friendly order (the scheduler's tie
// breaker): vrf, loopback, interface table binding, interface address, route.
func Register(r scheduler.Registry, env Env) {
	r.Register(&VRFDescriptor{env})
	r.Register(&LoopbackDescriptor{env})
	r.Register(&InterfaceTableDescriptor{env})
	r.Register(&InterfaceAddrDescriptor{env})
	r.Register(&RouteDescriptor{env})
}

// Names returns the core descriptor names in registration order.
func Names() []string {
	return []string{VRFName, LoopbackName, InterfaceTableName, InterfaceAddrName, RouteName}
}

// InterfaceKey is the generic interface reference "interface/<name>".
func InterfaceKey(name string) scheduler.Key { return scheduler.Join(InterfaceRef, name) }

// VRFKey is "vrf/<id>".
func VRFKey(id uint32) scheduler.Key {
	return scheduler.Join(VRFName, strconv.FormatUint(uint64(id), 10))
}

// LoopbackKey is "interface.loopback/<name>".
func LoopbackKey(name string) scheduler.Key { return scheduler.Join(LoopbackName, name) }

// InterfaceAddrKey is "interface-ip/<if>/<prefix>".
func InterfaceAddrKey(ifName, prefix string) scheduler.Key {
	return scheduler.Join(InterfaceAddrName, ifName, prefix)
}

// InterfaceTableKey is "interface-ip.table/<if>".
func InterfaceTableKey(ifName string) scheduler.Key {
	return scheduler.Join(InterfaceTableName, ifName)
}

// RouteKey is "ip.route/<table>/<prefix>".
func RouteKey(table uint32, prefix string) scheduler.Key {
	return scheduler.Join(RouteName, strconv.FormatUint(uint64(table), 10), prefix)
}

var loopRe = regexp.MustCompile(`^loop([0-9]{1,9})$`)

// LoopbackInstance parses "loop<N>" and reports whether name is a loopback name.
func LoopbackInstance(name string) (uint32, bool) {
	m := loopRe.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseUint(m[1], 10, 32)
	if err != nil || n == uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(n), true
}

// CanonAddrPrefix canonicalises an interface address "a.b.c.d/len" (host bits kept).
func CanonAddrPrefix(s string) (string, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return "", fmt.Errorf("%w %q: %v", ErrBadPrefix, s, err)
	}
	return netip.PrefixFrom(p.Addr().Unmap().WithZone(""), p.Bits()).String(), nil
}

// CanonNetPrefix canonicalises a destination prefix (host bits cleared).
func CanonNetPrefix(s string) (string, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return "", fmt.Errorf("%w %q: %v", ErrBadPrefix, s, err)
	}
	return netip.PrefixFrom(p.Addr().Unmap().WithZone(""), p.Bits()).Masked().String(), nil
}

// CanonAddr canonicalises an IP address.
func CanonAddr(s string) (string, error) {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return "", fmt.Errorf("core: invalid address %q: %v", s, err)
	}
	return a.Unmap().WithZone("").String(), nil
}

// ifInfo is one interface of a sw_interface_dump.
type ifInfo struct {
	Index   uint32
	VPPName string
	DevType string
	ID      string // owner-tag id; "" when not owned by us
}

// ifTable indexes one dump.
type ifTable struct {
	all     []ifInfo
	byID    map[string]ifInfo // owned, by tag id
	byName  map[string]ifInfo // all, by VPP name
	byIndex map[uint32]ifInfo
}

func dumpInterfaces(ctx context.Context, c vpp.Client, owner string) (*ifTable, error) {
	stream, err := interfaces.NewServiceClient(c).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_dump: %w", err)
	}
	t := &ifTable{byID: map[string]ifInfo{}, byName: map[string]ifInfo{}, byIndex: map[uint32]ifInfo{}}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("sw_interface_dump: %w", err)
		}
		in := ifInfo{Index: uint32(d.SwIfIndex), VPPName: trimNul(d.InterfaceName), DevType: trimNul(d.InterfaceDevType)}
		if id, ok := vpp.ParseOwnerTag(d.Tag, owner); ok {
			in.ID = id
			t.byID[id] = in
		}
		t.all = append(t.all, in)
		t.byName[in.VPPName] = in
		t.byIndex[in.Index] = in
	}
	return t, nil
}

// owned resolves an owned interface by its document name (tag id).
func (t *ifTable) owned(name string) (ifInfo, error) {
	in, ok := t.byID[name]
	if !ok {
		return ifInfo{}, fmt.Errorf("%w: %q", ErrNotOwned, name)
	}
	return in, nil
}

// any resolves an interface by document name: an owned one by tag id first, else by VPP name.
func (t *ifTable) any(name string) (ifInfo, bool) {
	if in, ok := t.byID[name]; ok {
		return in, true
	}
	in, ok := t.byName[name]
	return in, ok
}

// nameOf returns the document name of sw_if_index: the tag id when owned, else the VPP name.
func (t *ifTable) nameOf(idx uint32) (string, bool) {
	in, ok := t.byIndex[idx]
	if !ok {
		return "", false
	}
	if in.ID != "" {
		return in.ID, true
	}
	return in.VPPName, true
}

func trimNul(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return s[:i]
		}
	}
	return s
}

// IfMeta is the Meta of interface-shaped core objects: the sw_if_index.
type IfMeta struct{ SwIfIndex uint32 }
