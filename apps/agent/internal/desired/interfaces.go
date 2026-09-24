// Package desired is the desired-state builder of the `interfaces` domain (P08): it turns
// `interfaces.<name>` of the configuration document (vrx.v1.Interface) into the scheduler objects of
// the core, DF-1, af_packet and DHCP descriptors, and assembles retrieved objects back into the same
// messages for Retrieve (canonical form, docs/contracts/proto.md §5).
//
// Names and references (D-065, D-069, D-073a): the configuration key is the LOGICAL name. Every
// interface the document names gets one `interface/<name>` alias object, and every object that
// refers to an interface depends on that alias — never on a creator key:
//
//	loop<N>          interface.loopback/loop<N>          + alias{creator: interface.loopback/loop<N>}
//	host-<netdev>    af-packet.host-interface/host-<if>  + alias{creator: af-packet.host-interface/…}
//	                 (VPP's own af_packet naming; the agent creates it on the Linux netdev <if>, D-010)
//	anything else    alias{creator: ""} — a physical / pre-existing interface (DPDK NIC by its logical
//	                 name, F-startup-gen), never created or deleted by the agent
//	<name>.subinterfaces.<id>
//	                 interface.subinterface/<name>.<id> {parent: interface/<name>, exact-match}
//	                 + alias{creator: interface.subinterface/<name>.<id>}
//
// Attributes on interface/<n>: enabled → interface.admin-state (present ⇔ up), mtu → interface.mtu,
// mac → interface.mac-address, promiscuous → interface.promisc (present ⇔ on), rxMode →
// interface.rx-mode; vrf → interface-ip.table; ipv4/ipv6 → interface-ip; dhcpClient → dhcp.client.
// description is not VPP state: the agent keeps it in its stored desired state (D-073b).
// unnumbered is reported as agent.unsupported-field (no descriptor in this build).
package desired

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	afpacket "ngfw/agent/internal/descriptors/af_packet"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dhcp"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

// Sink receives the builder's objects and findings (the agent's projection).
type Sink interface {
	Add(k scheduler.Key, v proto.Message, pointer string)
	Errorf(pointer, rule, format string, a ...any)
	Warnf(pointer, rule, format string, a ...any)
}

// Ptr builds an RFC 6901 pointer from segments ("/" → "~1", "~" → "~0").
func Ptr(segs ...string) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteByte('/')
		b.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(s))
	}
	return b.String()
}

// Kind is how the agent realises an interface named in the document.
type Kind int

// Kinds.
const (
	// KindExisting: a physical or pre-existing interface (no creator).
	KindExisting Kind = iota
	// KindLoopback: loop<N>, created by core's interface.loopback.
	KindLoopback
	// KindHostInterface: host-<netdev>, an af_packet interface created on the Linux netdev.
	KindHostInterface
)

// netdev is a Linux interface name (IFNAMSIZ-1 = 15 bytes) in the alphabet of parentInterfaceName.
var hostRe = regexp.MustCompile(`^host-([A-Za-z0-9_-]{1,15})$`)

// KindOf classifies a logical interface name; for KindHostInterface it also returns the netdev.
func KindOf(name string) (Kind, string) {
	if _, ok := core.LoopbackInstance(name); ok {
		return KindLoopback, ""
	}
	if m := hostRe.FindStringSubmatch(name); m != nil {
		return KindHostInterface, m[1]
	}
	return KindExisting, ""
}

// SubName is the logical (and VPP) name of a sub-interface: "<parent>.<id>".
func SubName(parent, id string) string { return parent + "." + id }

var rxModes = map[string]iface.RxModeKind{
	"polling":   iface.RxModeKind_RX_MODE_KIND_POLLING,
	"interrupt": iface.RxModeKind_RX_MODE_KIND_INTERRUPT,
	"adaptive":  iface.RxModeKind_RX_MODE_KIND_ADAPTIVE,
}

// RxModeName is the configuration spelling of an rx mode ("" for unspecified).
func RxModeName(k iface.RxModeKind) string {
	for n, v := range rxModes {
		if v == k {
			return n
		}
	}
	return ""
}

// NetdevKind reports the rtnetlink link kind of a Linux netdev ("veth", "bridge", …; "" for a
// physical NIC) and whether it exists (subsystems.LinuxNetdevKind; a stub in unit tests).
type NetdevKind func(name string) (kind string, exists bool, err error)

// Interfaces emits the objects of every interface in ifs. vrfID maps a VRF name to its table id
// (false: unknown VRF). lookup (nil: no check) looks up the Linux netdev of a host-<netdev> name:
// af_packet attaches only to a veth (D-010/D-105), so a netdev that exists and is not a veth is a
// validation error at /interfaces/<name> — the management NIC can never be taken by configuration.
// A netdev that does not exist (yet) is not an error here: a vanished rig veth must not block
// unrelated commits; the af_packet Create checks again (subsystems' veth guard).
func Interfaces(s Sink, ifs map[string]*vrxv1.Interface, vrfID func(string) (uint32, bool), lookup NetdevKind) {
	for _, name := range sortedKeys(ifs) {
		itf := ifs[name]
		pt := Ptr("interfaces", name)
		alias := &iface.InterfaceAlias{Name: name}
		switch kind, netdev := KindOf(name); kind {
		case KindLoopback:
			inst, _ := core.LoopbackInstance(name)
			k := core.LoopbackKey(name)
			s.Add(k, &core.Loopback{Name: name, Instance: inst}, pt)
			alias.Creator = string(k)
		case KindHostInterface:
			if lookup != nil {
				switch k, ok, err := lookup(netdev); {
				case err != nil:
					s.Errorf(pt, "interfaces.af-packet-veth", "cannot check the Linux netdev %q of %s (af_packet attaches only to a veth): %v", netdev, name, err)
					continue
				case ok && k != "veth":
					what := "a physical (kind-less)"
					if k != "" {
						what = "a " + k
					}
					s.Errorf(pt, "interfaces.af-packet-veth", "%s: af_packet attaches only to a Linux veth (lab data path); %q is %s netdev", name, netdev, what)
					continue
				}
			}
			hi := &afpacket.HostInterface{Name: name, HostIfName: netdev, Mode: afpacket.Mode_MODE_ETHERNET}
			k := scheduler.Join(afpacket.HostInterfaceName, name)
			s.Add(k, hi, pt)
			alias.Creator = string(k)
		}
		s.Add(iface.AliasKey(name), alias, pt)
		ref := string(iface.AliasKey(name))

		common(s, name, ref, pt, commonFields{
			enabled: itf.GetEnabled(), mtu: itf.Mtu, vrf: itf.Vrf, ipv4: itf.GetIpv4(), ipv6: itf.GetIpv6(),
			unnumbered: itf.Unnumbered != nil, dhcp: itf.GetDhcpClient(),
		}, vrfID)
		if itf.Mac != nil {
			s.Add(scheduler.Join(iface.MacAddressName, name), &iface.MacAddress{Interface: ref, Mac: strings.ToLower(itf.GetMac())}, Ptr("interfaces", name, "mac"))
		}
		if itf.GetPromiscuous() {
			s.Add(scheduler.Join(iface.PromiscName, name), &iface.Promisc{Interface: ref}, Ptr("interfaces", name, "promiscuous"))
		}
		if itf.RxMode != nil {
			k, ok := rxModes[itf.GetRxMode()]
			if !ok {
				s.Errorf(Ptr("interfaces", name, "rxMode"), "interfaces.rx-mode", "rx mode %q is not polling, interrupt or adaptive", itf.GetRxMode())
			} else {
				s.Add(scheduler.Join(iface.RxModeName, name), &iface.RxMode{Interface: ref, Mode: k}, Ptr("interfaces", name, "rxMode"))
			}
		}

		for _, id := range sortedKeys(itf.GetSubinterfaces()) {
			sub := itf.GetSubinterfaces()[id]
			spt := Ptr("interfaces", name, "subinterfaces", id)
			n, err := strconv.ParseUint(id, 10, 32)
			if err != nil {
				s.Errorf(spt, "interfaces.subinterface-id", "sub-interface id %q is not a 32-bit decimal", id)
				continue
			}
			if sub.VlanId == nil {
				s.Errorf(Ptr("interfaces", name, "subinterfaces", id, "vlanId"), "interfaces.vlan-id", "sub-interface %s needs a vlanId", SubName(name, id))
				continue
			}
			so := &iface.Subinterface{
				Parent: ref, SubId: uint32(n), OuterVlan: sub.GetVlanId(), InnerVlan: sub.GetInnerVlanId(),
				Dot1Ad: sub.GetDot1Ad(), ExactMatch: true, // exact-match: routed (L3) sub-interfaces need it
			}
			sname := SubName(name, id)
			sk := scheduler.Join(iface.SubinterfaceName, sname)
			s.Add(sk, so, spt)
			s.Add(iface.AliasKey(sname), &iface.InterfaceAlias{Name: sname, Creator: string(sk)}, spt)
			common(s, sname, string(iface.AliasKey(sname)), spt, commonFields{
				enabled: sub.GetEnabled(), mtu: sub.Mtu, vrf: sub.Vrf, ipv4: sub.GetIpv4(), ipv6: sub.GetIpv6(),
				unnumbered: sub.Unnumbered != nil, dhcp: sub.GetDhcpClient(),
			}, vrfID)
		}
	}
}

type commonFields struct {
	enabled    bool
	mtu        *uint32
	vrf        *string
	ipv4, ipv6 []string
	unnumbered bool
	dhcp       *vrxv1.DhcpClient
}

// common emits what interfaces and sub-interfaces share. pt is the pointer of the (sub-)interface.
func common(s Sink, name, ref, pt string, f commonFields, vrfID func(string) (uint32, bool)) {
	if f.enabled {
		s.Add(scheduler.Join(iface.AdminStateName, name), &iface.AdminState{Interface: ref}, pt+"/enabled")
	}
	if f.mtu != nil {
		s.Add(scheduler.Join(iface.MtuName, name), &iface.Mtu{Interface: ref, Mtu: *f.mtu}, pt+"/mtu")
	}
	if f.unnumbered {
		s.Warnf(pt+"/unnumbered", "agent.unsupported-field", "%s/unnumbered is not implemented by this agent build (P08 wires admin state, MTU, MAC, promiscuous, rx mode, VRF, addresses, sub-interfaces, DHCP client)", pt)
	}
	if f.vrf != nil {
		id, ok := vrfID(*f.vrf)
		switch {
		case !ok:
			s.Errorf(pt+"/vrf", "interfaces.vrf-exists", "VRF %q does not exist", *f.vrf)
		case id != 0:
			s.Add(core.InterfaceTableKey(name), &core.InterfaceTable{Interface: name, TableId: id}, pt+"/vrf")
		}
	}
	for fam, list := range map[string][]string{"ipv4": f.ipv4, "ipv6": f.ipv6} {
		for i, a := range list {
			ap := pt + "/" + fam + "/" + strconv.Itoa(i)
			c, err := core.CanonAddrPrefix(a)
			if err != nil {
				s.Errorf(ap, "interfaces.address", "%v", err)
				continue
			}
			if isV6 := strings.Contains(c, ":"); isV6 != (fam == "ipv6") {
				s.Errorf(ap, "interfaces.address-family", "%s is not an %s address", a, fam)
				continue
			}
			s.Add(core.InterfaceAddrKey(name, c), &core.InterfaceAddress{Interface: name, Prefix: c}, ap)
		}
	}
	if f.dhcp != nil {
		c := dhcp.Client{Interface: name, Hostname: f.dhcp.GetHostname(), ClientID: f.dhcp.GetClientId(), SetBroadcastFlag: f.dhcp.GetSetBroadcastFlag()}
		s.Add(scheduler.Join(dhcp.NameClient, name), dfkit.Encode(c), pt+"/dhcpClient")
	}
}

// Live is the live view the assembler needs for interfaces the agent does not manage attributes of.
type Live interface {
	// State returns the live state of the interface with this logical name.
	State(name string) (*vrxv1.InterfaceState, bool)
}

// Assemble builds `interfaces` from retrieved objects. stored is the agent's stored desired state
// of the domain (descriptions, which physical interfaces the document names, which scalars it sets);
// tableName names a FIB table. An interface appears when the agent created it, holds an object on
// it, or the stored document names it and it exists.
func Assemble(kvs []scheduler.KV, stored map[string]*vrxv1.Interface, live Live, tableName func(uint32) string) map[string]*vrxv1.Interface {
	nodes := map[string]*node{}
	subParent := map[string]string{} // sub name → parent name
	subID := map[string]string{}
	exists := map[string]bool{}
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *iface.InterfaceAlias:
			exists[v.GetName()] = true
		case *iface.Subinterface:
			sname := kv.Key.ID()
			subParent[sname] = iface.RefID(v.GetParent())
			subID[sname] = strconv.FormatUint(uint64(v.GetSubId()), 10)
		}
	}
	get := func(name string) *node {
		if n, ok := nodes[name]; ok {
			return n
		}
		n := &node{}
		if _, ok := subParent[name]; ok {
			n.sub = &vrxv1.Subinterface{}
		} else {
			n.itf = &vrxv1.Interface{}
		}
		nodes[name] = n
		return n
	}
	// managed scalars
	enabled := map[string]bool{}
	promisc := map[string]bool{}
	mtu := map[string]uint32{}
	rx := map[string]string{}
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *core.Loopback:
			get(v.GetName())
		case *afpacket.HostInterface:
			get(v.GetName())
		case *iface.Subinterface:
			sname := kv.Key.ID()
			n := get(sname)
			n.sub.VlanId = proto.Uint32(v.GetOuterVlan())
			if v.GetInnerVlan() != 0 {
				n.sub.InnerVlanId = proto.Uint32(v.GetInnerVlan())
			}
			n.sub.Dot1Ad = proto.Bool(v.GetDot1Ad())
		case *iface.AdminState:
			get(iface.RefID(v.GetInterface()))
			enabled[iface.RefID(v.GetInterface())] = true
		case *iface.Mtu:
			get(iface.RefID(v.GetInterface()))
			mtu[iface.RefID(v.GetInterface())] = v.GetMtu()
		case *iface.MacAddress:
			get(iface.RefID(v.GetInterface())).setMac(v.GetMac())
		case *iface.Promisc:
			get(iface.RefID(v.GetInterface()))
			promisc[iface.RefID(v.GetInterface())] = true
		case *iface.RxMode:
			get(iface.RefID(v.GetInterface()))
			rx[iface.RefID(v.GetInterface())] = RxModeName(v.GetMode())
		case *core.InterfaceTable:
			get(v.GetInterface()).setVrf(tableName(v.GetTableId()))
		case *core.InterfaceAddress:
			get(v.GetInterface()).addAddr(v.GetPrefix())
		}
		if kv.Key.Descriptor() == dhcp.NameClient {
			var c dhcp.Client
			if err := dfkit.Decode(kv.Value, &c); err == nil {
				dc := &vrxv1.DhcpClient{SetBroadcastFlag: proto.Bool(c.SetBroadcastFlag)}
				if c.Hostname != "" {
					dc.Hostname = proto.String(c.Hostname)
				}
				if c.ClientID != "" {
					dc.ClientId = proto.String(c.ClientID)
				}
				get(c.Interface).setDHCP(dc)
			}
		}
	}
	// interfaces the stored document names that exist but carry no object of ours
	for name, itf := range stored {
		if exists[name] {
			get(name)
		}
		for id := range itf.GetSubinterfaces() {
			if sname := SubName(name, id); exists[sname] {
				if _, ok := subParent[sname]; ok {
					get(sname)
				}
			}
		}
	}

	out := map[string]*vrxv1.Interface{}
	storedOf := func(name string) (*vrxv1.Interface, *vrxv1.Subinterface) {
		if p, ok := subParent[name]; ok {
			return nil, stored[p].GetSubinterfaces()[subID[name]]
		}
		return stored[name], nil
	}
	for _, name := range sortedKeys(nodes) {
		n := nodes[name]
		st, _ := live.State(name)
		sItf, sSub := storedOf(name)
		// enabled: our admin-state object (present ⇔ up) when we manage the interface, else VPP's flag
		en := enabled[name] || (st != nil && !st.GetManaged() && st.GetAdminUp())
		var desiredMtu, desiredRx bool
		var desc *string
		if sItf != nil {
			desiredMtu, desiredRx, desc = sItf.Mtu != nil, sItf.RxMode != nil, sItf.Description
		}
		if sSub != nil {
			desiredMtu, desc = sSub.Mtu != nil, sSub.Description
		}
		var m *uint32
		if v, ok := mtu[name]; ok {
			m = proto.Uint32(v)
		} else if desiredMtu && st != nil && st.GetMtu() != 0 {
			m = proto.Uint32(st.GetMtu()) // configured but no object of ours: report what VPP has (drift)
		}
		if n.sub != nil {
			n.sub.Enabled = proto.Bool(en)
			n.sub.Mtu = m
			n.sub.Description = desc
			if n.sub.Vrf == nil {
				n.sub.Vrf = proto.String(tableName(0))
			}
			sortAddrs(n.sub.Ipv4)
			sortAddrs(n.sub.Ipv6)
			p := subParent[name]
			parent := out[p]
			if parent == nil {
				parent = &vrxv1.Interface{}
				if pn, ok := nodes[p]; ok && pn.itf != nil {
					parent = pn.itf
				}
				out[p] = parent
			}
			if parent.Subinterfaces == nil {
				parent.Subinterfaces = map[string]*vrxv1.Subinterface{}
			}
			parent.Subinterfaces[subID[name]] = n.sub
			continue
		}
		itf := n.itf
		if prev, ok := out[name]; ok { // a sub-interface was assembled first
			itf.Subinterfaces = prev.Subinterfaces
		}
		itf.Enabled = proto.Bool(en)
		itf.Promiscuous = proto.Bool(promisc[name])
		itf.Mtu = m
		if v, ok := rx[name]; ok {
			itf.RxMode = proto.String(v)
		} else if desiredRx && st != nil && st.GetRxMode() != "" {
			itf.RxMode = proto.String(st.GetRxMode())
		}
		itf.Description = desc
		if itf.Vrf == nil {
			itf.Vrf = proto.String(tableName(0))
		}
		sortAddrs(itf.Ipv4)
		sortAddrs(itf.Ipv6)
		out[name] = itf
	}
	// parents created only as holders of sub-interfaces still need their defaults
	for _, itf := range out {
		if itf.Enabled == nil {
			itf.Enabled = proto.Bool(false)
		}
		if itf.Promiscuous == nil {
			itf.Promiscuous = proto.Bool(false)
		}
		if itf.Vrf == nil {
			itf.Vrf = proto.String(tableName(0))
		}
	}
	return out
}

// node is one interface or sub-interface under assembly.
type node struct {
	itf *vrxv1.Interface
	sub *vrxv1.Subinterface
}

func (n *node) setMac(mac string) {
	if n.itf != nil {
		n.itf.Mac = proto.String(mac)
	}
}

func (n *node) setVrf(v string) {
	if n.itf != nil {
		n.itf.Vrf = proto.String(v)
	} else {
		n.sub.Vrf = proto.String(v)
	}
}

func (n *node) addAddr(p string) {
	v6 := strings.Contains(p, ":")
	switch {
	case n.itf != nil && v6:
		n.itf.Ipv6 = append(n.itf.Ipv6, p)
	case n.itf != nil:
		n.itf.Ipv4 = append(n.itf.Ipv4, p)
	case v6:
		n.sub.Ipv6 = append(n.sub.Ipv6, p)
	default:
		n.sub.Ipv4 = append(n.sub.Ipv4, p)
	}
}

func (n *node) setDHCP(d *vrxv1.DhcpClient) {
	if n.itf != nil {
		n.itf.DhcpClient = d
	} else {
		n.sub.DhcpClient = d
	}
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

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
