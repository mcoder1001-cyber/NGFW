package ravpn

import (
	"context"
	"encoding/json"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
	"net/netip"
	"ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/core"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"strconv"
	"strings"
)

// TransportObjects projects the complete isolated transport and bidirectional policy dependencies.
func TransportObjects(s EngineSpec) ([]scheduler.KV, error) {
	if s.Validate() != nil {
		return nil, ErrEngine
	}
	p, err := BuildNetworkPlan(s.Owner, s.Profile, s.Configuration)
	if err != nil {
		return nil, ErrEngine
	}
	ns, err := NamespaceValue(p)
	if err != nil {
		return nil, ErrEngine
	}
	out := []scheduler.KV{{Key: scheduler.Join(NamespaceName, NamespaceKeyID(s.Instance)), Value: ns}}
	outer, inner, err := TransitTAPs(p, s.OuterID, s.InnerID)
	if err != nil {
		return nil, ErrEngine
	}
	for i, tap := range []*tapv2.Tap{outer, inner} {
		table, link, policy := s.OuterTable, p.Outer, s.Configuration.GetOuterPolicy()
		if i == 1 {
			table, link, policy = s.InnerTable, p.Inner, s.Configuration.GetAccessPolicy()
		}
		creator := scheduler.Join(tapv2.TapName, tap.Name)
		out = append(out, scheduler.KV{Key: creator, Value: tap}, scheduler.KV{Key: scheduler.Join(iface.AliasName, tap.Name), Value: &iface.InterfaceAlias{Name: tap.Name, Creator: string(creator)}}, scheduler.KV{Key: scheduler.Join(iface.AdminStateName, tap.Name), Value: &iface.AdminState{Interface: string(core.InterfaceKey(tap.Name))}})
		if table != 0 {
			out = append(out, scheduler.KV{Key: scheduler.Join(core.InterfaceTableName, tap.Name), Value: &core.InterfaceTable{Interface: tap.Name, TableId: table}})
		}
		out = append(out, scheduler.KV{Key: scheduler.Join(core.InterfaceAddrName, tap.Name, link.VPP), Value: &core.InterfaceAddress{Interface: tap.Name, Prefix: link.VPP}}, scheduler.KV{Key: acl.KeyInterfaceBinding(tap.Name), Value: (acl.InterfaceBinding{Interface: tap.Name, Input: policy.GetIngress(), Output: policy.GetEgress()}).Proto()})
	}
	if p.InnerIPv6 != nil {
		out = append(out, scheduler.KV{Key: scheduler.Join(core.InterfaceAddrName, inner.Name, p.InnerIPv6.VPP), Value: &core.InterfaceAddress{Interface: inner.Name, Prefix: p.InnerIPv6.VPP}})
	}
	addRoute := func(table uint32, prefix, address, ifname string) {
		out = append(out, scheduler.KV{Key: scheduler.Join(core.RouteName, strconv.FormatUint(uint64(table), 10), prefix), Value: &core.Route{TableId: table, Prefix: prefix, Paths: []*core.RoutePath{{Address: address, Interface: ifname, Weight: 1}}}})
	}
	endpoint, _ := netip.ParseAddr(p.LocalAddress)
	outerPeer, _ := netip.ParsePrefix(p.Outer.Namespace)
	addRoute(s.OuterTable, netip.PrefixFrom(endpoint, endpoint.BitLen()).String(), outerPeer.Addr().String(), outer.Name)
	for _, text := range p.Pools {
		prefix, _ := netip.ParsePrefix(text)
		link := p.Inner
		if prefix.Addr().Is6() {
			link = *p.InnerIPv6
		}
		peer, _ := netip.ParsePrefix(link.Namespace)
		addRoute(s.InnerTable, text, peer.Addr().String(), inner.Name)
	}
	for i := range out {
		out[i].Key = PrivateKey(out[i].Key)
	}
	return out, nil
}

// DescriptorReader looks up concrete descriptor readback implementations.
type DescriptorReader interface {
	Get(string) (scheduler.Descriptor, bool)
}

// RegistryVerifier verifies exact namespace, TAP, routing and policy readback.
type RegistryVerifier struct {
	Registry DescriptorReader
	Boot     func(context.Context) (bootid.Identity, error)
	Tables   func(context.Context, uint32) (uint32, uint32, error)
}

// Verify reads back the complete isolated transport and policy handoff.
func (v *RegistryVerifier) Verify(ctx context.Context, s EngineSpec) (*NetworkPlan, Handoff, error) {
	if v == nil || v.Registry == nil || v.Boot == nil || v.Tables == nil {
		return nil, Handoff{}, ErrEngine
	}
	p, err := ReadAgentPlan(s.Instance)
	if err != nil || p.Owner != s.Owner || p.Profile != s.Profile {
		return nil, Handoff{}, ErrEngine
	}
	expected, err := BuildNetworkPlan(s.Owner, s.Profile, s.Configuration)
	if err != nil {
		return nil, Handoff{}, ErrEngine
	}
	expected.NamespaceInode = p.NamespaceInode
	expected.HostNamespaceInode = p.HostNamespaceInode
	expected.KernelLinks = p.KernelLinks
	a, _ := json.Marshal(expected)
	b, _ := json.Marshal(p)
	if string(a) != string(b) {
		return nil, Handoff{}, ErrEngine
	}
	boot, err := v.Boot(ctx)
	if err != nil || !boot.Complete() || boot.PID <= 0 {
		return nil, Handoff{}, ErrEngine
	}
	wanted, err := TransportObjects(s)
	if err != nil {
		return nil, Handoff{}, ErrEngine
	}
	dumps := map[string]map[scheduler.Key]scheduler.KV{}
	for _, kv := range wanted {
		name := kv.Key.Descriptor()
		if name == NamespaceName {
			continue
		}
		rows, ok := dumps[name]
		if !ok {
			d, exists := v.Registry.Get(name)
			if !exists {
				return nil, Handoff{}, ErrEngine
			}
			actual, err := d.Retrieve(ctx)
			if err != nil {
				return nil, Handoff{}, ErrEngine
			}
			rows = map[scheduler.Key]scheduler.KV{}
			for _, row := range actual {
				if _, dup := rows[row.Key]; dup {
					return nil, Handoff{}, ErrEngine
				}
				rows[row.Key] = row
			}
			dumps[name] = rows
		}
		actual, ok := rows[kv.Key]
		if !ok || !proto.Equal(kv.Value, actual.Value) {
			return nil, Handoff{}, ErrEngine
		}
	}
	// Default VRF bindings are absent from desired objects: prove that no
	// non-default binding exists rather than assuming table zero.
	tableDescriptor, exists := v.Registry.Get(PrivateKey(scheduler.Join(core.InterfaceTableName, "")).Descriptor())
	if !exists {
		return nil, Handoff{}, ErrEngine
	}
	tables, err := tableDescriptor.Retrieve(ctx)
	if err != nil {
		return nil, Handoff{}, ErrEngine
	}
	for _, binding := range []struct {
		name  string
		table uint32
	}{{LinkName(s.Instance, true), s.OuterTable}, {LinkName(s.Instance, false), s.InnerTable}} {
		for _, row := range tables {
			actual, ok := row.Value.(*core.InterfaceTable)
			if !ok {
				return nil, Handoff{}, ErrEngine
			}
			if actual.Interface == binding.name && actual.TableId != binding.table {
				return nil, Handoff{}, ErrEngine
			}
		}
	}
	ids := []uint32{}
	for _, outer := range []bool{true, false} {
		row := dumps[tapv2.TapName][scheduler.Join(tapv2.TapName, LinkName(s.Instance, outer))]
		meta, ok := row.Meta.(TAPReceipt)
		if !ok || meta.Pending {
			return nil, Handoff{}, ErrEngine
		}
		ids = append(ids, meta.Index)
	}
	for index, expected := range []uint32{s.OuterTable, s.InnerTable} {
		v4, v6, err := v.Tables(ctx, ids[index])
		if err != nil || v4 != expected || v6 != expected {
			return nil, Handoff{}, ErrEngine
		}
	}
	receipt := Handoff{Format: 1, Instance: s.Instance, NamespaceInode: p.NamespaceInode, HostNamespaceInode: p.HostNamespaceInode, VPPBoot: boot, OuterIndex: ids[0], InnerIndex: ids[1], OuterName: LinkName(s.Instance, true), InnerName: LinkName(s.Instance, false)}
	if ValidateHandoff(p, receipt, boot, ids[0], ids[1]) != nil {
		return nil, Handoff{}, ErrEngine
	}
	after, err := v.Boot(ctx)
	if err != nil || !after.Equal(boot) {
		return nil, Handoff{}, ErrEngine
	}
	return p, receipt, nil
}

// PrivateKey maps shared object families into the private remote-access descriptor scope.
func PrivateKey(k scheduler.Key) scheduler.Key {
	switch k.Descriptor() {
	case core.InterfaceTableName, core.InterfaceAddrName, core.RouteName, iface.AliasName, iface.AdminStateName, acl.NameInterfaceBinding:
		return scheduler.Join("remote-access."+k.Descriptor(), k.ID())
	}
	return k
}

// HostPrerequisites is read-only: it verifies required host capabilities and
// already-present kernel facilities. It never loads modules or creates links.
func HostPrerequisites() error {
	data, e := os.ReadFile("/proc/self/status")
	if e != nil || len(data) > 16384 {
		return ErrEngine
	}
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "CapEff:") {
			bits, e := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "CapEff:")), 16, 64)
			if e != nil || bits&(uint64(1)<<21) == 0 || bits&(uint64(1)<<12) == 0 {
				return ErrEngine
			}
			found = true
		}
	}
	if !found {
		return ErrEngine
	}
	for _, path := range []string{"/proc/self/ns/net", "/proc/self/ns/mnt", "/proc/self/ns/pid", "/proc/net/xfrm_stat", "/sys/module/xfrm_interface"} {
		if _, e := os.Stat(path); e != nil {
			return ErrEngine
		}
	}
	var device unix.Stat_t
	if unix.Lstat("/dev/net/tun", &device) != nil || device.Mode&unix.S_IFMT != unix.S_IFCHR || unix.Major(device.Rdev) != 10 || unix.Minor(device.Rdev) != 200 {
		return ErrEngine
	}
	return nil
}
