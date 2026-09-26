package desired

// Bonds (F-bonding): `interfaces.<BondEthernet<id>>.bond` → DF-1's bond.bond + bond.member and F-bonding's
// bond.member-weight, and back (AssembleBonds).
//
//	interfaces.BondEthernet<id>.bond                    → bond.bond/BondEthernet<id>   {id, mode, lb, numa_only}
//	                                                       (+ alias interface/BondEthernet<id> with creator bond.bond/…,
//	                                                        emitted by Interfaces through KindBond)
//	….bond.members.<m>                                  → bond.member/BondEthernet<id>/<m> {interface/…, interface/<m>,
//	                                                                                         passive, long_timeout}
//	….bond.members.<m>.weight                           → bond.member-weight/BondEthernet<id>/<m>
//
// The bond is otherwise an ordinary interface: enabled, MTU, MAC, addresses, VRF and sub-interfaces of
// interfaces.BondEthernet<id> go through Interfaces like any other interface's, all depending on its alias.
// The members' own entries (enabled, MTU) are ordinary interfaces too.

import (
	"regexp"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/bond"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

// bondNameRe is VPP's (and TNSR's) bond interface name, BondEthernet<id> (packages/schema BOND_INTERFACE_RE).
var bondNameRe = regexp.MustCompile(`^BondEthernet(0|[1-9][0-9]{0,9})$`)

// BondID returns the bond id encoded in a bond interface name ("BondEthernet12" → 12).
func BondID(name string) (uint32, bool) {
	m := bondNameRe.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	id, err := strconv.ParseUint(m[1], 10, 32)
	if err != nil || id == 1<<32-1 { // ~0 means "VPP chooses"
		return 0, false
	}
	return uint32(id), true
}

// bondModes are the configurable modes. VPP's broadcast mode is not offered (manager decision Q1); BondModeName still
// names it, for a bond VPP reports.
var bondModes = map[string]bond.Mode{
	"lacp":          bond.Mode_MODE_LACP,
	"xor":           bond.Mode_MODE_XOR,
	"round-robin":   bond.Mode_MODE_ROUND_ROBIN,
	"active-backup": bond.Mode_MODE_ACTIVE_BACKUP,
}

// nonEthernetRe mirrors packages/schema NON_ETHERNET_INTERFACE_RE: names that are never Ethernet NICs (review F4).
var nonEthernetRe = regexp.MustCompile(`^(?:loop|wg|ipip|gre|ipsec|vxlan_tunnel|vxlan_gpe_tunnel|gtpu_tunnel|geneve_tunnel|l2tpv3_tunnel|pppoe_session|mpls-tunnel|bvi|lisp_gpe|sr-tunnel)[0-9]+$`)

var bondHashes = map[string]bond.LoadBalance{
	"l2":  bond.LoadBalance_LOAD_BALANCE_L2,
	"l23": bond.LoadBalance_LOAD_BALANCE_L23,
	"l34": bond.LoadBalance_LOAD_BALANCE_L34,
}

// forcedLB is the algorithm VPP forces (and reports) for the modes without a hash.
var forcedLB = map[bond.Mode]bond.LoadBalance{
	bond.Mode_MODE_ROUND_ROBIN:   bond.LoadBalance_LOAD_BALANCE_ROUND_ROBIN,
	bond.Mode_MODE_ACTIVE_BACKUP: bond.LoadBalance_LOAD_BALANCE_ACTIVE_BACKUP,
	bond.Mode_MODE_BROADCAST:     bond.LoadBalance_LOAD_BALANCE_BROADCAST,
}

// BondModeName is the configuration spelling of a bond mode ("" for an unknown value); "broadcast" names VPP's mode that
// the configuration does not offer.
func BondModeName(m bond.Mode) string {
	if m == bond.Mode_MODE_BROADCAST {
		return "broadcast"
	}
	for n, v := range bondModes {
		if v == m {
			return n
		}
	}
	return ""
}

// BondLBName is the configuration spelling of a load-balance algorithm; the forced values of round-robin,
// active-backup and broadcast bonds are named after their mode.
func BondLBName(lb bond.LoadBalance) string {
	for n, v := range bondHashes {
		if v == lb {
			return n
		}
	}
	switch lb {
	case bond.LoadBalance_LOAD_BALANCE_ROUND_ROBIN:
		return "round-robin"
	case bond.LoadBalance_LOAD_BALANCE_ACTIVE_BACKUP:
		return "active-backup"
	case bond.LoadBalance_LOAD_BALANCE_BROADCAST:
		return "broadcast"
	}
	return ""
}

// Bonds emits the bond objects of every interface in ifs that carries a `bond` leaf. The semantic rules of the
// API (interfaces.bonding-*) are the first line; the builder repeats the checks the agent cannot do without
// (a name that encodes the id, a known mode, members that exist in the document, mode-dependent options), so a
// document that reached the agent some other way fails validation instead of VPP.
func Bonds(s Sink, ifs map[string]*vrxv1.Interface) {
	member := map[string]string{} // member → bond, for "at most one bond"
	for _, name := range sortedKeys(ifs) {
		b := ifs[name].GetBond()
		if b == nil {
			continue
		}
		pt := Ptr("interfaces", name, "bond")
		id, ok := BondID(name)
		if !ok {
			s.Errorf(pt, "interfaces.bonding-name", "a bond interface must be named BondEthernet<id> (VPP's name), not %q", name)
			continue
		}
		if b.Id != nil && b.GetId() != id {
			s.Errorf(Ptr("interfaces", name, "bond", "id"), "interfaces.bonding-name", "bond id %d does not match the interface name %s (id %d)", b.GetId(), name, id)
			continue
		}
		mode, ok := bondModes[b.GetMode()]
		if !ok {
			s.Errorf(Ptr("interfaces", name, "bond", "mode"), "interfaces.bonding-mode", "bond mode %q is not lacp, xor, round-robin or active-backup", b.GetMode())
			continue
		}
		lb, forced := forcedLB[mode]
		switch {
		case forced && b.LoadBalance != nil:
			s.Errorf(Ptr("interfaces", name, "bond", "loadBalance"), "interfaces.bonding-load-balance", "loadBalance applies to xor and lacp bonds only; VPP forces the algorithm of a %s bond", b.GetMode())
			continue
		case !forced:
			lb = bond.LoadBalance_LOAD_BALANCE_L2
			if b.LoadBalance != nil {
				if lb, ok = bondHashes[b.GetLoadBalance()]; !ok {
					s.Errorf(Ptr("interfaces", name, "bond", "loadBalance"), "interfaces.bonding-load-balance", "load balance %q is not l2, l23 or l34", b.GetLoadBalance())
					continue
				}
			}
		}
		s.Add(scheduler.Join(bond.BondName, name), &bond.Bond{Name: name, Id: id, Mode: mode, Lb: lb, NumaOnly: b.GetNumaOnly()}, pt)
		bondRef := string(iface.AliasKey(name))
		for _, m := range sortedKeys(b.GetMembers()) {
			mc := b.GetMembers()[m]
			mp := Ptr("interfaces", name, "bond", "members", m)
			switch _, configured := ifs[m]; {
			case !configured:
				s.Errorf(mp, "interfaces.bonding-member-exists", "member interface %q is not configured", m)
				continue
			case m == name || ifs[m].GetBond() != nil:
				s.Errorf(mp, "interfaces.bonding-member-kind", "%q is a bond; bond members must be physical interfaces", m)
				continue
			}
			if kind, _ := KindOf(m); kind == KindLoopback || kind == KindBond || nonEthernetRe.MatchString(m) {
				s.Errorf(mp, "interfaces.bonding-member-kind", "%q is not a physical (Ethernet) interface; it cannot be a bond member", m)
				continue
			}
			if ifs[m].Mac != nil {
				s.Errorf(Ptr("interfaces", m, "mac"), "interfaces.bonding-member-mac", "%s is a member of %s: members take the bond's MAC address; set mac on the bond instead", m, name)
				continue
			}
			if other, dup := member[m]; dup {
				s.Errorf(mp, "interfaces.bonding-member-unique", "%q is already a member of %s", m, other)
				continue
			}
			member[m] = name
			if mode != bond.Mode_MODE_LACP && (mc.GetPassive() || mc.GetLongTimeout()) {
				s.Errorf(mp, "interfaces.bonding-lacp-options", "passive and longTimeout are LACP options; %s is a %s bond", name, b.GetMode())
				continue
			}
			memberRef := string(iface.AliasKey(m))
			s.Add(scheduler.Join(bond.MemberName, name, m), &bond.Member{Bond: bondRef, Interface: memberRef, Passive: mc.GetPassive(), LongTimeout: mc.GetLongTimeout()}, mp)
			if mc.Weight == nil {
				continue
			}
			switch {
			case mode != bond.Mode_MODE_ACTIVE_BACKUP:
				s.Errorf(mp+"/weight", "interfaces.bonding-weight", "weight applies to active-backup bonds only; %s is a %s bond", name, b.GetMode())
			case mc.GetWeight() == 0:
				s.Errorf(mp+"/weight", "interfaces.bonding-weight", "weight must be 1 or more")
			default:
				s.Add(bond.WeightKey(bondRef, memberRef), bond.Weight{Bond: bondRef, Interface: memberRef, Weight: mc.GetWeight()}.Proto(), mp+"/weight")
			}
		}
	}
}

// AssembleBonds adds the `bond` leaf of every retrieved bond to ds.interfaces (after Assemble built the interfaces,
// wave-A-hotspots A2). A bond the agent created but holds no attribute on is added with Assemble's defaults
// (disabled, not promiscuous, VRF of table 0). Canonical form (D-039, proto.md §11 F-bonding): mode, numa_only,
// every member's passive/long_timeout are always set; load_balance for xor/lacp bonds when the stored document sets
// it or VPP's value is not l2; id only when the stored document sets it; weight when VPP reports one.
func AssembleBonds(ds *vrxv1.DesiredState, kvs []scheduler.KV, stored map[string]*vrxv1.Interface, tableName func(uint32) string) {
	bonds := map[string]*vrxv1.Bond{}
	var members []*bond.Member
	weights := map[[2]string]uint32{}
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *bond.Bond:
			b := &vrxv1.Bond{Mode: proto.String(BondModeName(v.GetMode())), NumaOnly: proto.Bool(v.GetNumaOnly())}
			sb := stored[v.GetName()].GetBond() // nil when the stored document has no such bond
			if _, forced := forcedLB[v.GetMode()]; !forced && ((sb != nil && sb.LoadBalance != nil) || v.GetLb() != bond.LoadBalance_LOAD_BALANCE_L2) {
				b.LoadBalance = proto.String(BondLBName(v.GetLb()))
			}
			if sb != nil && sb.Id != nil {
				b.Id = proto.Uint32(v.GetId())
			}
			bonds[v.GetName()] = b
		case *bond.Member:
			members = append(members, v)
		}
		if kv.Key.Descriptor() == bond.WeightName {
			if w, err := bond.WeightFromProto(kv.Value); err == nil {
				weights[[2]string{iface.RefID(w.Bond), iface.RefID(w.Interface)}] = w.Weight
			}
		}
	}
	if len(bonds) == 0 {
		return
	}
	sort.Slice(members, func(i, j int) bool { return members[i].GetInterface() < members[j].GetInterface() })
	for _, m := range members {
		bn, mn := iface.RefID(m.GetBond()), iface.RefID(m.GetInterface())
		b, ok := bonds[bn]
		if !ok {
			continue
		}
		if b.Members == nil {
			b.Members = map[string]*vrxv1.BondMember{}
		}
		bm := &vrxv1.BondMember{Passive: proto.Bool(m.GetPassive()), LongTimeout: proto.Bool(m.GetLongTimeout())}
		if w, ok := weights[[2]string{bn, mn}]; ok {
			bm.Weight = proto.Uint32(w)
		}
		b.Members[mn] = bm
	}
	if ds.Interfaces == nil {
		ds.Interfaces = map[string]*vrxv1.Interface{}
	}
	for name, b := range bonds {
		itf, ok := ds.Interfaces[name]
		if !ok {
			itf = &vrxv1.Interface{Enabled: proto.Bool(false), Promiscuous: proto.Bool(false), Vrf: proto.String(tableName(0))}
			if st, ok := stored[name]; ok && st != nil {
				itf.Description = st.Description
			}
			ds.Interfaces[name] = itf
		}
		itf.Bond = b
	}
}
