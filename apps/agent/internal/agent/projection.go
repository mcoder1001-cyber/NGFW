package agent

// Projection between the configuration document (vrx.v1.DesiredState) and the scheduler's
// per-object KVs of the core descriptors, in both directions:
//
//	project:  DesiredState --(authoritative domains)--> []KV + JSON pointers + validation issues
//	assemble: Retrieve() []KV --> DesiredState (canonical, owned objects only)
//
// Domains implemented by this build (HealthResponse.subsystems): interfaces, vrfs, routing.
//
//	interfaces.<loopN>                 → interface.loopback/<loopN>
//	interfaces.<if>.vrf (≠ "default")  → interface-ip.table/<if>        (table id from vrfs)
//	interfaces.<if>.ipv4[] / .ipv6[]   → interface-ip/<if>/<addr>/<len>
//	vrfs.<name> (id ≠ 0)               → vrf/<id>                       (VPP table name "<owner>:<name>")
//	routing.static[]                   → ip.route/<table>/<prefix>      (vrf → table id, distance → preference)
//
// VRF and static-route descriptions are not VPP state: the service returns them from its stored
// desired state for objects that exist (D-073b). Leaves the core descriptors do not implement
// (interface enabled/mtu/mac/rx_mode/promiscuous/unnumbered/subinterfaces/description/dhcp_client,
// routing protocols)
// are reported as ISSUE_SEVERITY_WARNING "agent.unsupported-field" by DryRun and are never part of
// a Retrieve result (contract §5: a leaf the backend cannot report is left unset). DF-*/F-* tasks
// extend the projection when they wire their descriptors.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// Root keys of the configuration document (ROOT_KEYS in packages/schema), documented order.
var rootKeys = []string{
	"system", "dataplane", "interfaces", "vrfs", "routing", "nat", "objects",
	"acl", "vpn", "tunnels", "services", "ha", "management",
}

// domainDescriptors maps each implemented domain to the descriptors that realise it (P08: the
// wiring lives in internal/subsystems).
var domainDescriptors = subsystems.Domains

// implementedDomains lists the implemented domains in ROOT_KEYS order.
func implementedDomains() []string {
	var out []string
	for _, k := range rootKeys {
		if _, ok := domainDescriptors[k]; ok {
			out = append(out, k)
		}
	}
	return out
}

func isRootKey(k string) bool {
	for _, r := range rootKeys {
		if r == k {
			return true
		}
	}
	return false
}

// domainOf returns the domain a descriptor belongs to.
func domainOf(descriptor string) string {
	for d, ds := range domainDescriptors {
		for _, n := range ds {
			if n == descriptor {
				return d
			}
		}
	}
	return ""
}

// scopeOf is the scheduler scope managing exactly the descriptors of domains.
func scopeOf(domains []string) scheduler.Scope {
	var names []string
	for _, d := range domains {
		names = append(names, domainDescriptors[d]...)
	}
	return scheduler.Only(names...)
}

// domainPresent reports whether a domain is present in ds (D-041: message set; for the two map
// domains a non-empty map).
func protoName(key string) protoreflect.Name { return protoreflect.Name(key) }

func domainPresent(ds *vrxv1.DesiredState, key string) bool {
	if ds == nil {
		return false
	}
	switch key {
	case "interfaces":
		return len(ds.GetInterfaces()) > 0
	case "vrfs":
		return len(ds.GetVrfs()) > 0
	}
	fd := ds.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(key))
	return fd != nil && ds.ProtoReflect().Has(fd)
}

// issue is a projection finding (becomes a vrx.v1.ValidationIssue).
type issue struct {
	pointer  string
	message  string
	severity vrxv1.IssueSeverity
	rule     string
}

// projected is the result of project.
type projected struct {
	kvs      []scheduler.KV
	pointers map[scheduler.Key]string
	issues   []issue
}

func (p *projected) add(k scheduler.Key, v proto.Message, pointer string) {
	for _, kv := range p.kvs {
		if kv.Key == k {
			p.errorf(pointer, "agent.duplicate-object", "duplicate object %s (also at %s)", k, p.pointers[k])
			return
		}
	}
	p.kvs = append(p.kvs, scheduler.KV{Key: k, Value: v})
	p.pointers[k] = pointer
}

// Add implements desired.Sink.
func (p *projected) Add(k scheduler.Key, v proto.Message, pointer string) { p.add(k, v, pointer) }

// Errorf implements desired.Sink.
func (p *projected) Errorf(pointer, rule, format string, a ...any) {
	p.errorf(pointer, rule, format, a...)
}

// Warnf implements desired.Sink.
func (p *projected) Warnf(pointer, rule, format string, a ...any) {
	p.warnf(pointer, rule, format, a...)
}

func (p *projected) errorf(pointer, rule, format string, a ...any) {
	p.issues = append(p.issues, issue{pointer: pointer, rule: rule, severity: vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR, message: fmt.Sprintf(format, a...)})
}

func (p *projected) warnf(pointer, rule, format string, a ...any) {
	p.issues = append(p.issues, issue{pointer: pointer, rule: rule, severity: vrxv1.IssueSeverity_ISSUE_SEVERITY_WARNING, message: fmt.Sprintf(format, a...)})
}

func (p *projected) hasErrors() bool {
	for _, is := range p.issues {
		if is.severity == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
			return true
		}
	}
	return false
}

// ptr builds an RFC 6901 pointer from segments ("/" → "~1", "~" → "~0").
func ptr(segs ...string) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteByte('/')
		b.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(s))
	}
	return b.String()
}

// vrfResolver maps a VRF name to its table id.
type vrfResolver func(name string) (uint32, bool)

// project turns the authoritative domains of ds into KVs. resolve maps VRF names that are not in
// ds.vrfs (e.g. when `vrfs` is not part of this transaction) to table ids.
// netdev (nil: no check) is the Linux netdev lookup of the af_packet veth rule (D-105).
func project(ds *vrxv1.DesiredState, domains []string, resolve vrfResolver, netdev desired.NetdevKind) *projected {
	p := &projected{pointers: map[scheduler.Key]string{}}
	in := map[string]bool{}
	for _, d := range domains {
		in[d] = true
	}
	vrfID := func(name string) (uint32, bool) {
		if name == "" || name == "default" {
			return 0, true
		}
		if v, ok := ds.GetVrfs()[name]; ok && v.Id != nil {
			return v.GetId(), true
		}
		if resolve != nil {
			return resolve(name)
		}
		return 0, false
	}

	if in["vrfs"] {
		ids := map[uint32]string{}
		for _, name := range sortedMapKeys(ds.GetVrfs()) {
			v := ds.GetVrfs()[name]
			pt := ptr("vrfs", name)
			if v.Id == nil {
				p.errorf(ptr("vrfs", name, "id"), "vrfs.id-required", "VRF %q has no table id", name)
				continue
			}
			if v.GetId() == 0 {
				if name != "default" {
					p.warnf(ptr("vrfs", name, "id"), "vrfs.default-table", "VRF %q uses table 0 (VPP's default table); it is not created or deleted", name)
				}
				continue
			}
			if other, dup := ids[v.GetId()]; dup {
				p.errorf(ptr("vrfs", name, "id"), "vrfs.id-unique", "table id %d is used by VRFs %q and %q", v.GetId(), other, name)
				continue
			}
			ids[v.GetId()] = name
			if _, err := core.TableName("x", name); err != nil {
				p.errorf(pt, "vrfs.name", "VRF name %q: %v", name, err)
				continue
			}
			p.add(core.VRFKey(v.GetId()), &core.Table{Id: v.GetId(), Vrf: name}, pt)
		}
	}

	if in["interfaces"] {
		desired.Interfaces(p, ds.GetInterfaces(), vrfID, netdev) // P08: aliases, creators, attributes, sub-interfaces
	}

	for _, k := range rootKeys {
		if _, impl := domainDescriptors[k]; !impl && domainPresent(ds, k) && !isEmptyDomain(ds, k) {
			p.warnf(ptr(k), "agent.unimplemented-domain", "%s is not implemented by this agent build (Health.subsystems) and is not applied", k)
		}
	}

	if in["routing"] {
		for i, r := range ds.GetRouting().GetStatic() {
			pt := ptr("routing", "static", strconv.Itoa(i))
			if frr.StaticOwnedByFRR(i, r, nil) {
				continue // D-072: FRR (staticd) programs this route; the agent never does both
			}
			pfx, err := core.CanonNetPrefix(r.GetPrefix())
			if err != nil {
				p.errorf(ptr("routing", "static", strconv.Itoa(i), "prefix"), "routing.static.prefix", "%v", err)
				continue
			}
			vrf := r.GetVrf()
			table, ok := vrfID(vrf)
			if !ok {
				p.errorf(ptr("routing", "static", strconv.Itoa(i), "vrf"), "routing.static.vrf-exists", "VRF %q does not exist", vrf)
				continue
			}
			v := &core.Route{TableId: table, Prefix: pfx, Preference: r.GetDistance()}
			bad := false
			for j, nh := range r.GetNextHops() {
				rp := &core.RoutePath{Interface: nh.GetInterface(), Weight: nh.GetWeight()}
				if rp.Weight == 0 {
					rp.Weight = 1
				}
				if nh.Address != nil {
					a, err := core.CanonAddr(nh.GetAddress())
					if err != nil {
						p.errorf(ptr("routing", "static", strconv.Itoa(i), "nextHops", strconv.Itoa(j), "address"), "routing.static.next-hop", "%v", err)
						bad = true
						continue
					}
					rp.Address = a
				}
				if nh.Vrf != nil { // F-vrf-static-ecmp: the next hop is resolved in another VRF
					nt, ok := vrfID(nh.GetVrf())
					if !ok || rp.Address == "" || rp.Interface != "" {
						p.errorf(ptr("routing", "static", strconv.Itoa(i), "nextHops", strconv.Itoa(j), "vrf"), "routing.static.next-hop-vrf", "next-hop VRF %q: it must exist and needs a next-hop address without an egress interface", nh.GetVrf())
						bad = true
						continue
					}
					if nt != table {
						rp.NextHopTable = proto.Uint32(nt)
					}
				}
				if rp.Address == "" && rp.Interface == "" {
					p.errorf(ptr("routing", "static", strconv.Itoa(i), "nextHops", strconv.Itoa(j)), "routing.static.next-hop", "next hop needs an address or an interface")
					bad = true
					continue
				}
				v.Paths = append(v.Paths, rp)
			}
			if bad {
				continue
			}
			if r.GetBlackhole() != (len(v.Paths) == 0) {
				p.errorf(pt, "routing.static.blackhole", "a route needs at least one next hop, unless it is a blackhole route (then none)")
				continue
			}
			core.SortPaths(v.Paths)
			p.add(core.RouteKey(table, pfx), v, pt)
		}
		rt := ds.GetRouting()
		if rt.GetPolicy() != nil || rt.GetBgp() != nil || rt.GetOspf() != nil || rt.GetIsis() != nil || rt.GetRip() != nil || rt.GetBfd() != nil {
			p.warnf(ptr("routing"), "agent.unsupported-field", "routing protocols and policy are rendered by RF-1 (FRR), not by this agent build")
		}
	}
	// Feature builders (internal/desired/<slug>.go): one call under the feature's anchor, e.g.
	// `if in["nat"] { desired.Nat(p, ds.GetNat(), vrfID) }` (wave-A-hotspots A2).
	// wave-BC: F-det44-map-dslite-cnat
	// wave-BC: F-tunnels
	// wave-BC: F-vrrp-config-sync
	// wave-BC: F-ikev2-native
	// wave-BC: F-mpls-srmpls
	// wave-BC: F-srv6
	// wave-BC: F-lisp
	if in["tunnels"] {
		desired.Lisp(p, ds.GetTunnels(), vrfID)
	}
	// wave-BC: F-bfd-redistribution
	// wave-BC: F-igmp-mfib
	// wave-BC: F-ha-state-sync
	// wave-A: F-bonding
	if in["interfaces"] {
		desired.Bonds(p, ds.GetInterfaces()) // interfaces.<BondEthernet<id>>.bond → bond.bond, bond.member, bond.member-weight
	}
	// wave-A: F-bridge-l2
	if in["interfaces"] {
		desired.BridgeL2(p, ds, vrfID) // interfaces.<if>.l2 + routing.l2 (D-109 c); descriptors in the interfaces domain
	}
	// wave-A: F-loopback-bvi-gso-lldp-span
	desired.LoopbackBviGsoLldpSpan(p, ds, in, subsystems.LoopbackBviGsoLldpSpanEnv()) // interfaces.<if>.gso/.mirror, services.lldp/.nsim
	// wave-A: F-vrf-static-ecmp
	desired.VrfStaticEcmp(p, ds, in, vrfID, subsystems.SvsRange())
	// wave-A: F-neighbors-ra
	desired.NeighborsRa(p, ds, in, vrfID)
	// wave-A: F-rpf-adl-pbr
	// wave-A: F-object-model
	// wave-A: F-acl
	// wave-A: F-host-acl-nftables
	// wave-A: F-nat44-ed-sessions
	// wave-A: F-nat44-ei-64-66-nptv6
	// wave-A: P11
	// wave-A: F-wireguard
	// wave-A: P12
	// wave-A: F-kea-dhcp-relay
	// wave-A: F-unbound-chrony-syslog
	if in["services"] { // F-snmp (unanchored)
		desired.Snmp(p, ds.GetServices())
	}
	if in["services"] {
		desired.HostStack(p, ds.GetServices().GetHostStack(), vrfID)
	} // F-host-stack (unanchored)
	// wave-BC: F-ipfix-sflow (unanchored)
	if in["services"] {
		desired.IpfixSflow(p, ds.GetServices(), vrfID)
	}
	// F-qos-flat (unanchored: no `wave-BC: F-qos-flat` anchor in this block)
	if in["services"] {
		desired.QoS(p, ds.GetServices().GetQos())
		desired.ServicesUnsupported(p, ds.GetServices()) // once per projection (merge note: F-kea-dhcp-relay calls it too)
	}
	return p
}

// isEmptyDomain reports whether a present domain message carries nothing (the API sends all 13
// domains, most of them prefaulted to {}); only non-empty unimplemented domains are worth a warning.
func isEmptyDomain(ds *vrxv1.DesiredState, key string) bool {
	fd := ds.ProtoReflect().Descriptor().Fields().ByName(protoName(key))
	if fd == nil || fd.Message() == nil {
		return true
	}
	return proto.Size(ds.ProtoReflect().Get(fd).Message().Interface()) == 0
}

// assemble builds the DesiredState of the given domains from retrieved KVs. names maps table ids
// to VRF names for tables that are not among the retrieved VRFs (e.g. `vrfs` not requested); stored
// is the agent's stored `interfaces` document and live the interface table (P08, desired.Assemble).
func assemble(kvs []scheduler.KV, domains []string, names func(id uint32) (string, bool), stored map[string]*vrxv1.Interface, live desired.Live) *vrxv1.DesiredState {
	ds := &vrxv1.DesiredState{}
	in := map[string]bool{}
	for _, d := range domains {
		in[d] = true
	}
	tableName := map[uint32]string{}
	for _, kv := range kvs {
		if t, ok := kv.Value.(*core.Table); ok {
			tableName[t.GetId()] = t.GetVrf()
		}
	}
	nameOf := func(id uint32) string {
		if n, ok := tableName[id]; ok {
			return n
		}
		if names != nil {
			if n, ok := names(id); ok {
				return n
			}
		}
		if id == 0 {
			return "default" // L-a: table 0 by id; the name of a desired VRF with id 0 wins above
		}
		return strconv.FormatUint(uint64(id), 10)
	}
	var routes []*core.Route
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *core.Table:
			if in["vrfs"] {
				if ds.Vrfs == nil {
					ds.Vrfs = map[string]*vrxv1.Vrf{}
				}
				ds.Vrfs[v.GetVrf()] = &vrxv1.Vrf{Id: proto.Uint32(v.GetId())}
			}
		case *core.Route:
			if in["routing"] {
				routes = append(routes, v)
			}
		}
	}
	if in["interfaces"] {
		// P08: every (sub-)interface the agent created or holds an object on, plus the ones the stored
		// document names that exist; M4 (P05): an interface without a table binding is in the default VRF.
		if live == nil {
			live = noLive{}
		}
		if ifs := desired.Assemble(kvs, stored, live, nameOf); len(ifs) > 0 {
			ds.Interfaces = ifs
		}
	}
	if in["routing"] {
		ds.Routing = &vrxv1.RoutingConfig{}
		sort.Slice(routes, func(a, b int) bool {
			na, nb := nameOf(routes[a].GetTableId()), nameOf(routes[b].GetTableId())
			if na != nb {
				return na < nb
			}
			return routes[a].GetPrefix() < routes[b].GetPrefix()
		})
		for _, r := range routes {
			sr := &vrxv1.StaticRoute{Prefix: proto.String(r.GetPrefix()), Vrf: proto.String(nameOf(r.GetTableId())), Blackhole: proto.Bool(len(r.GetPaths()) == 0)}
			if r.GetPreference() != 0 {
				sr.Distance = proto.Uint32(r.GetPreference())
			}
			for _, p := range r.GetPaths() {
				nh := &vrxv1.NextHop{Weight: proto.Uint32(p.GetWeight())}
				if p.GetAddress() != "" {
					nh.Address = proto.String(p.GetAddress())
				}
				if p.GetInterface() != "" {
					nh.Interface = proto.String(p.GetInterface())
				}
				if p.NextHopTable != nil { // F-vrf-static-ecmp
					nh.Vrf = proto.String(nameOf(p.GetNextHopTable()))
				}
				sr.NextHops = append(sr.NextHops, nh)
			}
			ds.Routing.Static = append(ds.Routing.Static, sr)
		}
	}
	// Feature assemblers (internal/desired/<slug>.go): one call under the feature's anchor; it runs after
	// desired.Assemble and the routes, so it adds its leaves to the assembled document (wave-A-hotspots A2).
	// wave-BC: F-det44-map-dslite-cnat
	// wave-BC: F-tunnels
	// wave-BC: F-vrrp-config-sync
	// wave-BC: F-ikev2-native
	// wave-BC: F-mpls-srmpls
	// wave-BC: F-srv6
	// wave-BC: F-lisp
	if in["tunnels"] {
		desired.AssembleLisp(ds, kvs, nameOf)
	}
	// wave-BC: F-bfd-redistribution
	// wave-BC: F-igmp-mfib
	// wave-BC: F-ha-state-sync
	// wave-A: F-bonding
	if in["interfaces"] {
		desired.AssembleBonds(ds, kvs, stored, nameOf) // the bond leaf of every retrieved bond (F-bonding)
	}
	// wave-A: F-bridge-l2
	if in["interfaces"] {
		desired.AssembleBridgeL2(ds, kvs, stored, nameOf)
	}
	// wave-A: F-loopback-bvi-gso-lldp-span
	desired.AssembleLoopbackBviGsoLldpSpan(ds, kvs, in, stored)
	// wave-A: F-vrf-static-ecmp
	if in["vrfs"] {
		desired.AssembleVrfStaticEcmp(ds, kvs, nameOf)
	}
	// wave-A: F-neighbors-ra
	desired.AssembleNeighborsRa(ds, kvs, in, stored, nameOf)
	// wave-A: F-rpf-adl-pbr
	// wave-A: F-object-model
	// wave-A: F-acl
	// wave-A: F-host-acl-nftables
	// wave-A: F-nat44-ed-sessions
	// wave-A: F-nat44-ei-64-66-nptv6
	// wave-A: P11
	// wave-A: F-wireguard
	// wave-A: P12
	// wave-A: F-kea-dhcp-relay
	// wave-A: F-unbound-chrony-syslog
	if in["services"] { // F-snmp (unanchored)
		desired.AssembleSnmp(ds, kvs)
	}
	if in["services"] {
		desired.HostStackAssemble(ds, kvs)
	} // F-host-stack (unanchored)
	// wave-BC: F-ipfix-sflow (unanchored)
	if in["services"] {
		desired.AssembleIpfixSflow(ds, kvs, nameOf)
	}
	// F-qos-flat (unanchored)
	if in["services"] {
		desired.AssembleQoS(ds, kvs)
	}
	return ds
}

// noLive is the empty desired.Live (unit tests without an interface table).
type noLive struct{}

func (noLive) State(string) (*vrxv1.InterfaceState, bool) { return nil, false }

func sortedMapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
