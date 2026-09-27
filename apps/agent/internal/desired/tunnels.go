package desired

// Tunnel interfaces builder and assembler (F-tunnels, WBS D6.6): `tunnels.gre|ipip|vxlan` ⇄ DF-6's
// tunnel descriptors plus the interface attributes of the tunnel interface.
//
//	tunnels.gre.<name>    → gre.tunnel/gre<instance>              (type l3|teb|erspan, p2p)
//	tunnels.ipip.<name>   → ipip.tunnel/ipip<instance>            (p2p|p2mp; dscp unset = copy the inner DSCP)
//	tunnels.vxlan.<name>  → vxlan.tunnel/vxlan_tunnel<instance>   (decap l2 = L2 tunnel, ip4|ip6 = is_l3)
//	every tunnel          → tunnels.meta/<vpp name>               (name, description, decap: VPP cannot hold them)
//	with `interfaces` in the transaction:
//	  → interface/<vpp name> (creator = the tunnel key), interface.admin-state, interface.mtu,
//	    interface-ip.table (vrf ≠ default), interface-ip/<addr> (ipv4/ipv6),
//	    l2.bridge-domain-member/<bd>/<vpp name> (bridgeDomain; L2 tunnels only)
//
// `instance` is mandatory in this build (the descriptors key a tunnel by its VPP name) and must lie in
// the agent's VPP id range (TD-8b, fail closed: no range → every tunnel is refused). Underlay VRF →
// the outer FIB table id; `vrf` → the overlay table of the tunnel interface. VXLAN-GPE, GTP-U, L2TPv3,
// PPPoE and 6RD have DF-6 descriptors but no schema keys yet (contract change) and are not projected.

import (
	"context"
	"net/netip"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/gre"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/ipip"
	"ngfw/agent/internal/descriptors/l2"
	"ngfw/agent/internal/descriptors/vxlan"
	"ngfw/agent/internal/scheduler"
)

// Rule ids of the agent-side tunnel checks (the schema's own rules live in packages/schema).
const (
	RuleTunnelInstance      = "tunnels.instance-required"
	RuleTunnelInstanceRange = "tunnels.instance-range"
	RuleTunnelInstanceDup   = "tunnels.instance-unique"
	RuleTunnelValue         = "tunnels.value"
	RuleTunnelDomain        = "tunnels.interfaces-domain"
)

// TunnelIDSpan is the id range tunnel instances must lie in (the agent's VPP id scope, TD-8b).
// All = every id; otherwise Lo..Hi, empty when Lo > Hi.
type TunnelIDSpan struct {
	Lo, Hi uint32
	All    bool
}

// Contains reports whether id is in the span.
func (r TunnelIDSpan) Contains(id uint32) bool { return r.All || (r.Lo <= id && id <= r.Hi) }

func (r TunnelIDSpan) String() string {
	switch {
	case r.All:
		return "all ids"
	case r.Lo > r.Hi:
		return "no id (no VPP id range is configured for this agent)"
	}
	return strconv.FormatUint(uint64(r.Lo), 10) + "–" + strconv.FormatUint(uint64(r.Hi), 10)
}

// ---- tunnels.meta ------------------------------------------------------------------------------

// TunnelMetaName is the agent-local descriptor holding what VPP cannot: the configuration name,
// description and VXLAN decap family of every tunnel this agent created.
const TunnelMetaName = "tunnels.meta"

// TunnelMetaSpec is one tunnels.meta entry; ID is the VPP interface name (gre5, vxlan_tunnel7 …).
type TunnelMetaSpec struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Decap       string `json:"decap,omitempty"`
}

// TunnelMetaKey is "tunnels.meta/<vpp name>".
func TunnelMetaKey(id string) scheduler.Key { return scheduler.Join(TunnelMetaName, id) }

// TunnelMetaValue encodes a spec as the descriptor's value.
func TunnelMetaValue(s TunnelMetaSpec) proto.Message { return dfkit.Encode(s) }

// DecodeTunnelMeta decodes a tunnels.meta value.
func DecodeTunnelMeta(v proto.Message) (TunnelMetaSpec, error) {
	var s TunnelMetaSpec
	err := dfkit.Decode(v, &s)
	return s, err
}

// TunnelMetaStore persists the table (subsystems: a JSON file in the state dir). Load returns a copy.
type TunnelMetaStore interface {
	Load() (map[string]TunnelMetaSpec, error)
	Save(map[string]TunnelMetaSpec) error
}

// TunnelMeta is the tunnels.meta descriptor: no VPP call; journaled and rolled back like any other.
type TunnelMeta struct{ store TunnelMetaStore }

// NewTunnelMeta returns the descriptor over store.
func NewTunnelMeta(store TunnelMetaStore) *TunnelMeta { return &TunnelMeta{store: store} }

// Name implements scheduler.Descriptor.
func (*TunnelMeta) Name() string { return TunnelMetaName }

// CheckPersistent (TD-11b): the table must survive an agent restart, or Retrieve loses the names.
func (d *TunnelMeta) CheckPersistent() error {
	if p, ok := d.store.(interface{ Persistent() bool }); ok && p.Persistent() {
		return nil
	}
	return dfkit.Specf("%s: metadata store %T does not survive an agent restart", TunnelMetaName, d.store)
}

// KeyOf implements scheduler.Descriptor.
func (*TunnelMeta) KeyOf(obj proto.Message) scheduler.Key {
	s, _ := DecodeTunnelMeta(obj)
	return TunnelMetaKey(s.ID)
}

// Dependencies implements scheduler.Descriptor (none: agent-local).
func (*TunnelMeta) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *TunnelMeta) put(obj proto.Message) error {
	s, err := DecodeTunnelMeta(obj)
	if err != nil {
		return err
	}
	if s.ID == "" || s.Name == "" {
		return dfkit.Specf("%s: id and name are required", TunnelMetaName)
	}
	m, err := d.store.Load()
	if err != nil {
		return err
	}
	m[s.ID] = s
	return d.store.Save(m)
}

// Create implements scheduler.Descriptor.
func (d *TunnelMeta) Create(_ context.Context, obj proto.Message) (any, error) {
	return nil, d.put(obj)
}

// Update implements scheduler.Descriptor.
func (d *TunnelMeta) Update(_ context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.put(newObj)
}

// Delete implements scheduler.Descriptor.
func (d *TunnelMeta) Delete(_ context.Context, obj proto.Message, _ any) error {
	s, err := DecodeTunnelMeta(obj)
	if err != nil {
		return err
	}
	m, err := d.store.Load()
	if err != nil {
		return err
	}
	if _, ok := m[s.ID]; !ok {
		return nil
	}
	delete(m, s.ID)
	return d.store.Save(m)
}

// Retrieve implements scheduler.Descriptor: the whole table, sorted by id.
func (d *TunnelMeta) Retrieve(context.Context) ([]scheduler.KV, error) {
	m, err := d.store.Load()
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.KV, 0, len(m))
	for _, id := range sortedKeys(m) {
		out = append(out, scheduler.KV{Key: TunnelMetaKey(id), Value: TunnelMetaValue(m[id])})
	}
	return out, nil
}

// ---- builder -----------------------------------------------------------------------------------

// tunnelCommon is what every kind shares (vrx.v1 tunnelCommon).
type tunnelCommon interface {
	GetEnabled() bool
	GetDescription() string
	GetSrc() string
	GetUnderlayVrf() string
	GetVrf() string
	GetIpv4() []string
	GetIpv6() []string
}

type tunnelBuild struct {
	s     Sink
	in    map[string]bool
	vrfID func(string) (uint32, bool)
	span  TunnelIDSpan
}

func canonAddr(a string) (string, error) {
	p, err := netip.ParseAddr(a)
	if err != nil {
		return "", err
	}
	return p.Unmap().String(), nil
}

// Tunnels projects tunnels.gre / .ipip / .vxlan. in is the set of authoritative domains of the
// transaction; vrfID maps VRF names to table ids; span is the agent's id range.
func Tunnels(s Sink, t *vrxv1.TunnelsConfig, in map[string]bool, vrfID func(string) (uint32, bool), span TunnelIDSpan) {
	if !in["tunnels"] || t == nil {
		return
	}
	b := tunnelBuild{s: s, in: in, vrfID: vrfID, span: span}
	seen := map[string]string{} // vpp name → pointer of the first tunnel
	for _, name := range sortedKeys(t.GetGre()) {
		g := t.GetGre()[name]
		pt := Ptr("tunnels", "gre", name)
		ifn, table, overlay, ok := b.common(pt, "gre", g, g.Instance, gre.InterfaceName, seen)
		if !ok {
			continue
		}
		v := &gre.Tunnel{Instance: g.GetInstance(), Mode: gre.TunnelMode_P2P, OuterTableId: table, SessionId: g.GetSessionId()}
		switch g.GetType() {
		case "", "l3":
			v.Type = gre.TunnelType_L3
		case "teb":
			v.Type = gre.TunnelType_TEB
		case "erspan":
			v.Type = gre.TunnelType_ERSPAN
		default:
			s.Errorf(pt+"/type", RuleTunnelValue, "unknown GRE type %q", g.GetType())
			continue
		}
		var err error
		if v.Src, err = canonAddr(g.GetSrc()); err != nil {
			s.Errorf(pt+"/src", RuleTunnelValue, "%v", err)
			continue
		}
		if v.Dst, err = canonAddr(g.GetDst()); err != nil {
			s.Errorf(pt+"/dst", RuleTunnelValue, "%v", err)
			continue
		}
		key := scheduler.Join(gre.TunnelName, ifn)
		s.Add(key, v, pt)
		b.meta(pt, TunnelMetaSpec{ID: ifn, Kind: "gre", Name: name, Description: g.GetDescription()})
		b.attributes(pt, ifn, key, g, g.Mtu, overlay, g.BridgeDomain, v.Type != gre.TunnelType_L3)
	}
	for _, name := range sortedKeys(t.GetIpip()) {
		p := t.GetIpip()[name]
		pt := Ptr("tunnels", "ipip", name)
		ifn, table, overlay, ok := b.common(pt, "ipip", p, p.Instance, ipip.InterfaceName, seen)
		if !ok {
			continue
		}
		v := &ipip.Tunnel{Instance: p.GetInstance(), TableId: table}
		switch p.GetMode() {
		case "", "p2p":
			v.Mode = ipip.TunnelMode_P2P
		case "p2mp":
			v.Mode = ipip.TunnelMode_MP
		default:
			s.Errorf(pt+"/mode", RuleTunnelValue, "unknown IPIP mode %q", p.GetMode())
			continue
		}
		if p.Dscp != nil {
			v.Dscp = p.GetDscp()
		} else {
			v.Flags = ipipCopyDSCP
		}
		var err error
		if v.Src, err = canonAddr(p.GetSrc()); err != nil {
			s.Errorf(pt+"/src", RuleTunnelValue, "%v", err)
			continue
		}
		if p.GetDst() != "" {
			if v.Dst, err = canonAddr(p.GetDst()); err != nil {
				s.Errorf(pt+"/dst", RuleTunnelValue, "%v", err)
				continue
			}
		}
		key := scheduler.Join(ipip.TunnelName, ifn)
		s.Add(key, v, pt)
		b.meta(pt, TunnelMetaSpec{ID: ifn, Kind: "ipip", Name: name, Description: p.GetDescription()})
		b.attributes(pt, ifn, key, p, p.Mtu, overlay, p.BridgeDomain, false)
	}
	for _, name := range sortedKeys(t.GetVxlan()) {
		x := t.GetVxlan()[name]
		pt := Ptr("tunnels", "vxlan", name)
		ifn, table, overlay, ok := b.common(pt, "vxlan", x, x.Instance, vxlan.InterfaceName, seen)
		if !ok {
			continue
		}
		decap := x.GetDecap()
		if decap == "" {
			decap = "l2"
		}
		if decap != "l2" && decap != "ip4" && decap != "ip6" {
			s.Errorf(pt+"/decap", RuleTunnelValue, "unknown VXLAN decap %q", decap)
			continue
		}
		v := &vxlan.Tunnel{Instance: x.GetInstance(), Vni: x.GetVni(), EncapVrfId: table, McastInterface: x.GetMcastInterface(),
			SrcPort: vxlanPort(x.SrcPort), DstPort: vxlanPort(x.DstPort), IsL3: decap != "l2"}
		var err error
		if v.Src, err = canonAddr(x.GetSrc()); err != nil {
			s.Errorf(pt+"/src", RuleTunnelValue, "%v", err)
			continue
		}
		if v.Dst, err = canonAddr(x.GetDst()); err != nil {
			s.Errorf(pt+"/dst", RuleTunnelValue, "%v", err)
			continue
		}
		key := scheduler.Join(vxlan.TunnelName, ifn)
		s.Add(key, v, pt)
		b.meta(pt, TunnelMetaSpec{ID: ifn, Kind: "vxlan", Name: name, Description: x.GetDescription(), Decap: decap})
		b.attributes(pt, ifn, key, x, x.Mtu, overlay, x.BridgeDomain, decap == "l2")
	}
}

// ipipCopyDSCP is TUNNEL_API_ENCAP_DECAP_FLAG_ENCAP_COPY_DSCP: an IPIP tunnel without dscp copies
// the inner DSCP (the schema's "omit to copy").
const ipipCopyDSCP = 4

func vxlanPort(p *uint32) uint32 {
	if p == nil || *p == vxlan.DefaultPort {
		return 0 // the descriptor's canonical form of VPP's default port
	}
	return *p
}

// common checks the instance and the VRFs of one tunnel and returns its VPP name, the underlay
// (outer) table id and the overlay table id.
func (b tunnelBuild) common(pt, kind string, c tunnelCommon, instance *uint32, ifName func(uint32) string, seen map[string]string) (string, uint32, uint32, bool) {
	if instance == nil {
		b.s.Errorf(pt+"/instance", RuleTunnelInstance, "this agent build needs a fixed instance for every %s tunnel (it names the engine interface)", kind)
		return "", 0, 0, false
	}
	if !b.span.Contains(*instance) {
		b.s.Errorf(pt+"/instance", RuleTunnelInstanceRange, "instance %d is outside this agent's VPP id range (%s)", *instance, b.span)
		return "", 0, 0, false
	}
	ifn := ifName(*instance)
	if first, dup := seen[ifn]; dup {
		b.s.Errorf(pt+"/instance", RuleTunnelInstanceDup, "instance %d is already used by %s", *instance, first)
		return "", 0, 0, false
	}
	seen[ifn] = pt
	underlay, ok := b.vrfID(vrfName(c.GetUnderlayVrf()))
	if !ok {
		b.s.Errorf(pt+"/underlayVrf", "tunnels.vrf-exists", "VRF %q does not exist", vrfName(c.GetUnderlayVrf()))
		return "", 0, 0, false
	}
	overlay, ok := b.vrfID(vrfName(c.GetVrf()))
	if !ok {
		b.s.Errorf(pt+"/vrf", "tunnels.vrf-exists", "VRF %q does not exist", vrfName(c.GetVrf()))
		return "", 0, 0, false
	}
	return ifn, underlay, overlay, true
}

func (b tunnelBuild) meta(pt string, m TunnelMetaSpec) {
	b.s.Add(TunnelMetaKey(m.ID), TunnelMetaValue(m), pt)
}

// attributes adds the interface-side objects of the tunnel interface ifn (created by creator).
func (b tunnelBuild) attributes(pt, ifn string, creator scheduler.Key, c tunnelCommon, mtu *uint32, overlay uint32, bd *uint32, isL2 bool) {
	enabled := c.GetEnabled() || !hasEnabled(c)
	if !b.in["interfaces"] {
		if enabled || mtu != nil || overlay != 0 || len(c.GetIpv4())+len(c.GetIpv6()) > 0 || bd != nil {
			b.s.Warnf(pt, RuleTunnelDomain, "%s: enabled, mtu, vrf, addresses and bridgeDomain are applied only in a transaction that includes `interfaces`", pt)
		}
		return
	}
	s := b.s
	ref := string(iface.AliasKey(ifn))
	s.Add(iface.AliasKey(ifn), &iface.InterfaceAlias{Name: ifn, Creator: string(creator)}, pt)
	if enabled {
		s.Add(scheduler.Join(iface.AdminStateName, ifn), &iface.AdminState{Interface: ref}, pt+"/enabled")
	}
	if mtu != nil {
		s.Add(scheduler.Join(iface.MtuName, ifn), &iface.Mtu{Interface: ref, Mtu: *mtu}, pt+"/mtu")
	}
	if overlay != 0 {
		s.Add(core.InterfaceTableKey(ifn), &core.InterfaceTable{Interface: ifn, TableId: overlay}, pt+"/vrf")
	}
	for _, f := range []struct {
		fam  string
		list []string
	}{{"ipv4", c.GetIpv4()}, {"ipv6", c.GetIpv6()}} {
		fam := f.fam
		for i, a := range f.list {
			cp, err := core.CanonAddrPrefix(a)
			if err != nil {
				s.Errorf(pt+"/"+fam+"/"+strconv.Itoa(i), RuleTunnelValue, "%v", err)
				continue
			}
			s.Add(core.InterfaceAddrKey(ifn, cp), &core.InterfaceAddress{Interface: ifn, Prefix: cp}, pt+"/"+fam+"/"+strconv.Itoa(i))
		}
	}
	if bd != nil {
		if !isL2 {
			s.Errorf(pt+"/bridgeDomain", RuleTunnelValue, "bridgeDomain applies to L2 tunnels only")
			return
		}
		s.Add(l2.MemberKey(*bd, ref), &l2.BridgeDomainMember{BridgeDomain: *bd, Interface: ref, PortType: l2.PortType_PORT_TYPE_NORMAL}, pt+"/bridgeDomain")
	}
}

// hasEnabled: enabled is optional in the proto; unset means the schema default (true).
func hasEnabled(c tunnelCommon) bool {
	switch v := c.(type) {
	case *vrxv1.GreTunnel:
		return v.Enabled != nil
	case *vrxv1.IpipTunnel:
		return v.Enabled != nil
	case *vrxv1.VxlanTunnel:
		return v.Enabled != nil
	}
	return true
}

// ---- assembler ---------------------------------------------------------------------------------

type tunnelAttrs struct {
	admin bool
	mtu   *uint32
	table uint32
	v4    []string
	v6    []string
	bd    *uint32
}

// AssembleTunnels adds tunnels.gre / .ipip / .vxlan to ds from the retrieved objects (when
// in["tunnels"]), names them from tunnels.meta (a tunnel without an entry is named by its VPP name),
// and removes what P08's assembler derived from the tunnel interfaces (interfaces.<vpp name>, unless
// the stored document names that interface). It returns kvs without the bridge-domain memberships of
// tunnel interfaces, so the bridge-l2 assembler that runs after it does not report them again as
// interfaces.<vpp name>.l2.
func AssembleTunnels(ds *vrxv1.DesiredState, kvs []scheduler.KV, in map[string]bool, tableName func(uint32) string, stored map[string]*vrxv1.Interface) []scheduler.KV {
	gres := map[string]*gre.Tunnel{}
	ipips := map[string]*ipip.Tunnel{}
	vxlans := map[string]*vxlan.Tunnel{}
	metas := map[string]TunnelMetaSpec{}
	attrs := map[string]*tunnelAttrs{}
	attr := func(n string) *tunnelAttrs {
		if a, ok := attrs[n]; ok {
			return a
		}
		a := &tunnelAttrs{}
		attrs[n] = a
		return a
	}
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *gre.Tunnel:
			gres[gre.InterfaceName(v.GetInstance())] = v
		case *ipip.Tunnel:
			ipips[ipip.InterfaceName(v.GetInstance())] = v
		case *vxlan.Tunnel:
			vxlans[vxlan.InterfaceName(v.GetInstance())] = v
		case *iface.AdminState:
			attr(iface.RefID(v.GetInterface())).admin = true
		case *iface.Mtu:
			attr(iface.RefID(v.GetInterface())).mtu = proto.Uint32(v.GetMtu())
		case *core.InterfaceTable:
			attr(v.GetInterface()).table = v.GetTableId()
		case *core.InterfaceAddress:
			a := attr(v.GetInterface())
			if pfx, err := netip.ParsePrefix(v.GetPrefix()); err == nil && pfx.Addr().Is4() {
				a.v4 = append(a.v4, v.GetPrefix())
			} else {
				a.v6 = append(a.v6, v.GetPrefix())
			}
		case *l2.BridgeDomainMember:
			attr(iface.RefID(v.GetInterface())).bd = proto.Uint32(v.GetBridgeDomain())
		}
		if kv.Key.Descriptor() == TunnelMetaName {
			if m, err := DecodeTunnelMeta(kv.Value); err == nil {
				metas[m.ID] = m
			}
		}
	}
	name := func(ifn, kind string) string {
		if m, ok := metas[ifn]; ok && m.Kind == kind {
			return m.Name
		}
		return ifn
	}
	desc := func(ifn string) *string {
		if m, ok := metas[ifn]; ok && m.Description != "" {
			return proto.String(m.Description)
		}
		return nil
	}
	fill := func(ifn string, en **bool, vrf **string, mtu **uint32, v4, v6 *[]string, bd **uint32) {
		if !in["interfaces"] {
			return
		}
		a := attr(ifn)
		*en = proto.Bool(a.admin)
		*vrf = proto.String(tableName(a.table))
		*mtu = a.mtu
		*v4 = append([]string{}, a.v4...)
		*v6 = append([]string{}, a.v6...)
		sortAddrs(*v4)
		sortAddrs(*v6)
		*bd = a.bd
	}
	if in["tunnels"] {
		out := struct {
			gre   map[string]*vrxv1.GreTunnel
			ipip  map[string]*vrxv1.IpipTunnel
			vxlan map[string]*vrxv1.VxlanTunnel
		}{map[string]*vrxv1.GreTunnel{}, map[string]*vrxv1.IpipTunnel{}, map[string]*vrxv1.VxlanTunnel{}}
		for _, ifn := range sortedKeys(gres) {
			v := gres[ifn]
			g := &vrxv1.GreTunnel{Instance: proto.Uint32(v.GetInstance()), Description: desc(ifn), Src: proto.String(v.GetSrc()),
				UnderlayVrf: proto.String(tableName(v.GetOuterTableId())), Type: proto.String(greTypeName(v.GetType()))}
			if v.GetDst() != "" {
				g.Dst = proto.String(v.GetDst())
			}
			if v.GetType() == gre.TunnelType_ERSPAN {
				g.SessionId = proto.Uint32(v.GetSessionId())
			}
			fill(ifn, &g.Enabled, &g.Vrf, &g.Mtu, &g.Ipv4, &g.Ipv6, &g.BridgeDomain)
			out.gre[name(ifn, "gre")] = g
		}
		for _, ifn := range sortedKeys(ipips) {
			v := ipips[ifn]
			p := &vrxv1.IpipTunnel{Instance: proto.Uint32(v.GetInstance()), Description: desc(ifn), Src: proto.String(v.GetSrc()),
				UnderlayVrf: proto.String(tableName(v.GetTableId())), Mode: proto.String("p2p")}
			if v.GetMode() == ipip.TunnelMode_MP {
				p.Mode = proto.String("p2mp")
			}
			if v.GetDst() != "" {
				p.Dst = proto.String(v.GetDst())
			}
			if v.GetFlags()&ipipCopyDSCP == 0 {
				p.Dscp = proto.Uint32(v.GetDscp())
			}
			fill(ifn, &p.Enabled, &p.Vrf, &p.Mtu, &p.Ipv4, &p.Ipv6, &p.BridgeDomain)
			out.ipip[name(ifn, "ipip")] = p
		}
		for _, ifn := range sortedKeys(vxlans) {
			v := vxlans[ifn]
			x := &vrxv1.VxlanTunnel{Instance: proto.Uint32(v.GetInstance()), Description: desc(ifn), Src: proto.String(v.GetSrc()),
				Dst: proto.String(v.GetDst()), UnderlayVrf: proto.String(tableName(v.GetEncapVrfId())), Vni: proto.Uint32(v.GetVni()),
				SrcPort: proto.Uint32(portOf(v.GetSrcPort())), DstPort: proto.Uint32(portOf(v.GetDstPort())), Decap: proto.String("l2")}
			if v.GetMcastInterface() != "" {
				x.McastInterface = proto.String(v.GetMcastInterface())
			}
			if v.GetIsL3() {
				x.Decap = proto.String("ip4")
				if m, ok := metas[ifn]; ok && (m.Decap == "ip4" || m.Decap == "ip6") {
					x.Decap = proto.String(m.Decap)
				}
			}
			fill(ifn, &x.Enabled, &x.Vrf, &x.Mtu, &x.Ipv4, &x.Ipv6, &x.BridgeDomain)
			out.vxlan[name(ifn, "vxlan")] = x
		}
		if len(out.gre)+len(out.ipip)+len(out.vxlan) > 0 {
			if ds.Tunnels == nil {
				ds.Tunnels = &vrxv1.TunnelsConfig{}
			}
			ds.Tunnels.Gre, ds.Tunnels.Ipip, ds.Tunnels.Vxlan = out.gre, out.ipip, out.vxlan
		}
	}
	// what P08's assemblers derived from the tunnel interfaces belongs to tunnels.*
	var drop []string
	for ifn := range gres {
		drop = append(drop, ifn)
	}
	for ifn := range ipips {
		drop = append(drop, ifn)
	}
	for ifn := range vxlans {
		drop = append(drop, ifn)
	}
	sort.Strings(drop)
	dropped := map[string]bool{}
	for _, ifn := range drop {
		if _, named := stored[ifn]; !named {
			delete(ds.Interfaces, ifn)
			dropped[ifn] = true
		}
	}
	if len(dropped) == 0 {
		return kvs
	}
	kept := make([]scheduler.KV, 0, len(kvs))
	for _, kv := range kvs {
		if m, ok := kv.Value.(*l2.BridgeDomainMember); ok && dropped[iface.RefID(m.GetInterface())] {
			continue
		}
		kept = append(kept, kv)
	}
	return kept
}

func portOf(p uint32) uint32 {
	if p == 0 {
		return vxlan.DefaultPort
	}
	return p
}

func greTypeName(t gre.TunnelType) string {
	switch t {
	case gre.TunnelType_TEB:
		return "teb"
	case gre.TunnelType_ERSPAN:
		return "erspan"
	}
	return "l3"
}
