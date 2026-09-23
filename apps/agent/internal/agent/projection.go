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
// Leaves the core descriptors do not implement (interface enabled/mtu/mac/rx_mode/promiscuous/
// unnumbered/subinterfaces/description, vrf description, route description, routing protocols)
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
	"ngfw/agent/internal/scheduler"
)

// Root keys of the configuration document (ROOT_KEYS in packages/schema), documented order.
var rootKeys = []string{
	"system", "dataplane", "interfaces", "vrfs", "routing", "nat", "objects",
	"acl", "vpn", "tunnels", "services", "ha", "management",
}

// domainDescriptors maps each implemented domain to the descriptors that realise it.
var domainDescriptors = map[string][]string{
	"interfaces": {core.LoopbackName, core.InterfaceTableName, core.InterfaceAddrName},
	"vrfs":       {core.VRFName},
	"routing":    {core.RouteName},
}

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
func project(ds *vrxv1.DesiredState, domains []string, resolve vrfResolver) *projected {
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
			if v.Description != nil {
				p.warnf(ptr("vrfs", name, "description"), "agent.unsupported-field", "vrfs.description is not stored in VPP by this agent build")
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
		for _, name := range sortedMapKeys(ds.GetInterfaces()) {
			itf := ds.GetInterfaces()[name]
			pt := ptr("interfaces", name)
			if inst, ok := core.LoopbackInstance(name); ok {
				p.add(core.LoopbackKey(name), &core.Loopback{Name: name, Instance: inst}, pt)
			}
			for _, f := range unsupportedInterfaceFields(itf) {
				p.warnf(ptr("interfaces", name, f), "agent.unsupported-field", "interfaces.%s is not implemented by this agent build (DF-1/P08)", f)
			}
			if itf.Vrf != nil {
				id, ok := vrfID(itf.GetVrf())
				switch {
				case !ok:
					p.errorf(ptr("interfaces", name, "vrf"), "interfaces.vrf-exists", "VRF %q does not exist", itf.GetVrf())
				case id != 0:
					p.add(core.InterfaceTableKey(name), &core.InterfaceTable{Interface: name, TableId: id}, ptr("interfaces", name, "vrf"))
				}
			}
			for fam, list := range map[string][]string{"ipv4": itf.GetIpv4(), "ipv6": itf.GetIpv6()} {
				for i, a := range list {
					ap := ptr("interfaces", name, fam, strconv.Itoa(i))
					c, err := core.CanonAddrPrefix(a)
					if err != nil {
						p.errorf(ap, "interfaces.address", "%v", err)
						continue
					}
					if isV6 := strings.Contains(c, ":"); isV6 != (fam == "ipv6") {
						p.errorf(ap, "interfaces.address-family", "%s is not an %s address", a, fam)
						continue
					}
					p.add(core.InterfaceAddrKey(name, c), &core.InterfaceAddress{Interface: name, Prefix: c}, ap)
				}
			}
		}
	}

	if in["routing"] {
		for i, r := range ds.GetRouting().GetStatic() {
			pt := ptr("routing", "static", strconv.Itoa(i))
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
			if r.Description != nil {
				p.warnf(ptr("routing", "static", strconv.Itoa(i), "description"), "agent.unsupported-field", "routing.static.description is not stored in VPP")
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
			core.SortPaths(v.Paths)
			p.add(core.RouteKey(table, pfx), v, pt)
		}
		rt := ds.GetRouting()
		if len(rt.GetPrefixLists()) > 0 || len(rt.GetRouteMaps()) > 0 || rt.Bgp != nil || rt.Ospf != nil || rt.Isis != nil || rt.Rip != nil || rt.Bfd != nil {
			p.warnf(ptr("routing"), "agent.unsupported-field", "routing protocols and policy are rendered by RF-1 (FRR), not by this agent build")
		}
	}
	return p
}

func unsupportedInterfaceFields(i *vrxv1.Interface) []string {
	var out []string
	if i.Enabled != nil {
		out = append(out, "enabled")
	}
	if i.Description != nil {
		out = append(out, "description")
	}
	if i.Mtu != nil {
		out = append(out, "mtu")
	}
	if i.Mac != nil {
		out = append(out, "mac")
	}
	if i.RxMode != nil {
		out = append(out, "rxMode")
	}
	if len(i.GetSubinterfaces()) > 0 {
		out = append(out, "subinterfaces")
	}
	if i.Unnumbered != nil {
		out = append(out, "unnumbered")
	}
	if i.Promiscuous != nil {
		out = append(out, "promiscuous")
	}
	return out
}

// assemble builds the DesiredState of the given domains from retrieved KVs. names maps table ids
// to VRF names for tables that are not among the retrieved VRFs (e.g. `vrfs` not requested).
func assemble(kvs []scheduler.KV, domains []string, names func(id uint32) (string, bool)) *vrxv1.DesiredState {
	ds := &vrxv1.DesiredState{}
	in := map[string]bool{}
	for _, d := range domains {
		in[d] = true
	}
	tableName := map[uint32]string{0: "default"}
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
		return strconv.FormatUint(uint64(id), 10)
	}
	iface := func(name string) *vrxv1.Interface {
		if ds.Interfaces == nil {
			ds.Interfaces = map[string]*vrxv1.Interface{}
		}
		i := ds.Interfaces[name]
		if i == nil {
			i = &vrxv1.Interface{}
			ds.Interfaces[name] = i
		}
		return i
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
		case *core.Loopback:
			if in["interfaces"] {
				iface(v.GetName())
			}
		case *core.InterfaceTable:
			if in["interfaces"] {
				iface(v.GetInterface()).Vrf = proto.String(nameOf(v.GetTableId()))
			}
		case *core.InterfaceAddress:
			if in["interfaces"] {
				i := iface(v.GetInterface())
				if strings.Contains(v.GetPrefix(), ":") {
					i.Ipv6 = append(i.Ipv6, v.GetPrefix())
				} else {
					i.Ipv4 = append(i.Ipv4, v.GetPrefix())
				}
			}
		case *core.Route:
			if in["routing"] {
				routes = append(routes, v)
			}
		}
	}
	for _, i := range ds.Interfaces {
		sortAddrs(i.Ipv4)
		sortAddrs(i.Ipv6)
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
			sr := &vrxv1.StaticRoute{Prefix: proto.String(r.GetPrefix()), Vrf: proto.String(nameOf(r.GetTableId()))}
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
				sr.NextHops = append(sr.NextHops, nh)
			}
			ds.Routing.Static = append(ds.Routing.Static, sr)
		}
	}
	return ds
}

func sortAddrs(a []string) {
	sort.Slice(a, func(i, j int) bool {
		x, e1 := core.CanonAddrPrefix(a[i])
		y, e2 := core.CanonAddrPrefix(a[j])
		if e1 != nil || e2 != nil {
			return a[i] < a[j]
		}
		return x < y
	})
}

func sortedMapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
