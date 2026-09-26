package desired

// F-neighbors-ra: projection of the neighbour / RA leaves onto DF-2's descriptors (used name for name, D-104), and the
// assembly of what they retrieve back into the configuration document.
//
//	interfaces.<if>[.subinterfaces.<id>].ipv6Ra        → ip6-nd.ra-config/<if>  (only when it differs from VPP's
//	                                                       fresh state: DF-2's Retrieve omits default-state interfaces)
//	                                    .ipv6Ra.prefixes → ip6-nd.ra-prefix/<if>/<prefix>
//	                                    .proxyArp        → arp.proxy-interface/<if>          (true only)
//	                                    .proxyNd[]       → ip6-nd.proxy/<if>/<ip6>           (opt-in, VRX_DF2_PROXY_ND=1, V12)
//	vrfs.<name>.proxyArpRanges[]                         → arp.proxy-range/<table>/<low>-<high>
//	routing.neighbors.static[]                           → ip-neighbor.neighbor/<if>/<ip>
//	routing.neighbors.ipv4Limits / ipv6Limits            → ip-neighbor.config/<ipv4|ipv6>    (globals owner only, D-071;
//	                                                       both families always, VPP defaults when unset — DF-2's
//	                                                       Retrieve always reports both)
//	routing.neighbors.dad                                → ip6-nd.dad/global                 (globals owner only)
//
// A leaf this agent does not apply (VPP-wide settings on a non-owner, proxy ND without the opt-in) is reported as
// agent.unsupported-field, which also keeps it out of the API's drift view.

import (
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/arp"
	"ngfw/agent/internal/descriptors/df2"
	ip6nd "ngfw/agent/internal/descriptors/ip6_nd"
	ipneighbor "ngfw/agent/internal/descriptors/ip_neighbor"
	"ngfw/agent/internal/scheduler"
)

// EnvProxyNd opts in to DF-2's proxy-ND descriptor (D-064, docs/vpp-code-track.md V12): VPP aborted after the first
// ip6nd_proxy_add_del on the shared host, so the product exposes proxy ND as experimental and off by default.
const EnvProxyNd = "VRX_DF2_PROXY_ND"

// ProxyNdEnabled reports whether this process opted in to proxy ND (VRX_DF2_PROXY_ND=1).
func ProxyNdEnabled() bool { return os.Getenv(EnvProxyNd) == "1" }

// NeighborsRaOptions are the registration facts the projection must agree with: which of DF-2's conditional
// descriptors the agent registered.
type NeighborsRaOptions struct {
	// GlobalsOwner: ip-neighbor.config and ip6-nd.dad are registered (D-071).
	GlobalsOwner bool
	// ProxyNd: ip6-nd.proxy is registered (VRX_DF2_PROXY_ND=1).
	ProxyNd bool
}

var neighborsRaOpts atomic.Pointer[NeighborsRaOptions]

// ConfigureNeighborsRa records what subsystems registered (called once at start-up; tests set it as they need).
func ConfigureNeighborsRa(o NeighborsRaOptions) { neighborsRaOpts.Store(&o) }

// NeighborsRaSettings returns the recorded options (zero value: neither globals nor proxy ND).
func NeighborsRaSettings() NeighborsRaOptions {
	if o := neighborsRaOpts.Load(); o != nil {
		return *o
	}
	return NeighborsRaOptions{}
}

// ruleUnsupported is the DryRun warning rule of a leaf this build does not apply (the API's drift view skips it;
// shared by neighbors_ra.go and nat.go).
const ruleUnsupported = "agent.unsupported-field"

// raNode is one (sub-)interface with its F-neighbors-ra leaves.
type raNode struct {
	name     string
	pointer  string
	ra       *vrxv1.Ipv6Ra
	proxyArp *bool
	proxyNd  []string
}

func raNodes(ifs map[string]*vrxv1.Interface) []raNode {
	var out []raNode
	for _, name := range sortedKeys(ifs) {
		itf := ifs[name]
		out = append(out, raNode{name, Ptr("interfaces", name), itf.GetIpv6Ra(), itf.ProxyArp, itf.GetProxyNd()})
		for _, id := range sortedKeys(itf.GetSubinterfaces()) {
			sub := itf.GetSubinterfaces()[id]
			out = append(out, raNode{SubName(name, id), Ptr("interfaces", name, "subinterfaces", id), sub.GetIpv6Ra(), sub.ProxyArp, sub.GetProxyNd()})
		}
	}
	return out
}

// RaConfigOf is the DF-2 value of an ipv6Ra leaf (normalised the way Retrieve reports it). Unset scalars take the
// schema defaults (VPP's own).
func RaConfigOf(name string, ra *vrxv1.Ipv6Ra) *ip6nd.RaConfig {
	u := func(p *uint32, def uint32) uint32 {
		if p == nil {
			return def
		}
		return *p
	}
	c := &ip6nd.RaConfig{
		Interface:      name,
		Suppress:       ra.Suppress == nil || ra.GetSuppress(),
		Managed:        ra.GetManaged(),
		Other:          ra.GetOther(),
		RouterLifetime: u(ra.LifetimeSec, ip6nd.DefaultRouterLifetime),
		MaxInterval:    u(ra.MaxIntervalSec, ip6nd.DefaultMaxInterval),
		MinInterval:    u(ra.MinIntervalSec, ip6nd.DefaultMinInterval),
	}
	return ip6nd.NormalizeRaConfig(c)
}

// IsDefaultRaConfig reports whether c (normalised) is VPP's fresh-interface RA state — what DF-2's Retrieve omits.
func IsDefaultRaConfig(c *ip6nd.RaConfig) bool {
	return c.GetSuppress() == ip6nd.DefaultSuppress && !c.GetManaged() && !c.GetOther() && !c.GetSuppressLinkLayerOption() &&
		!c.GetSendUnicast() && !c.GetCease() && c.GetRouterLifetime() == ip6nd.DefaultRouterLifetime &&
		c.GetMaxInterval() == ip6nd.DefaultMaxInterval && c.GetMinInterval() == ip6nd.DefaultMinInterval &&
		c.GetInitialCount() == ip6nd.DefaultInitialCount && c.GetInitialInterval() == ip6nd.DefaultInitialInterval
}

// NeighborsRa emits the F-neighbors-ra objects of the domains in scope (in). vrfID maps a VRF name to its table.
func NeighborsRa(s Sink, ds *vrxv1.DesiredState, in map[string]bool, vrfID func(string) (uint32, bool)) {
	opts := NeighborsRaSettings()
	if in["interfaces"] {
		for _, n := range raNodes(ds.GetInterfaces()) {
			if n.ra != nil {
				if c := RaConfigOf(n.name, n.ra); !IsDefaultRaConfig(c) {
					s.Add(scheduler.Join(ip6nd.RaConfigName, n.name), c, n.pointer+"/ipv6Ra")
				}
				for _, pf := range sortedKeys(n.ra.GetPrefixes()) {
					o := n.ra.GetPrefixes()[pf]
					pt := Ptr(append(ptrSegs(n.pointer), "ipv6Ra", "prefixes", pf)...)
					canon, err := df2.ParsePrefix(pf)
					if err != nil || !canon.Addr().Is6() {
						s.Errorf(pt, "interfaces.neighbors-ra-prefix", "%q is not an IPv6 prefix", pf)
						continue
					}
					p := ip6nd.NormalizeRaPrefix(&ip6nd.RaPrefix{
						Interface: n.name, Prefix: canon.Masked().String(),
						ValidLifetime: o.GetValidSec(), PreferredLifetime: o.GetPreferredSec(),
						OffLink: o.GetOffLink(), NoAutoconfig: o.GetNoAutoconfig(),
					})
					s.Add(scheduler.Join(ip6nd.RaPrefixName, n.name, p.GetPrefix()), p, pt)
				}
			}
			if n.proxyArp != nil && *n.proxyArp {
				s.Add(scheduler.Join(arp.InterfaceName, n.name), &arp.ProxyInterface{Interface: n.name}, n.pointer+"/proxyArp")
			}
			if len(n.proxyNd) > 0 && !opts.ProxyNd {
				s.Warnf(n.pointer+"/proxyNd", ruleUnsupported, "proxy ND is experimental and off in this agent (start it with %s=1; VPP V12): not applied", EnvProxyNd)
			} else {
				for i, a := range n.proxyNd {
					ip, err := df2.ParseAddr(a)
					if err != nil || !ip.Is6() {
						s.Errorf(n.pointer+"/proxyNd/"+strconv.Itoa(i), "interfaces.neighbors-ra-proxy-nd", "%q is not an IPv6 address", a)
						continue
					}
					s.Add(scheduler.Join(ip6nd.ProxyNdName, n.name, ip.String()), &ip6nd.ProxyNd{Interface: n.name, Address: ip.String()}, n.pointer+"/proxyNd/"+strconv.Itoa(i))
				}
			}
		}
	}
	if in["vrfs"] {
		for _, name := range sortedKeys(ds.GetVrfs()) {
			v := ds.GetVrfs()[name]
			if len(v.GetProxyArpRanges()) == 0 {
				continue
			}
			table, ok := vrfID(name)
			if !ok || v.Id == nil {
				continue // the VRF itself is reported by the vrfs projection
			}
			for i, r := range v.GetProxyArpRanges() {
				pt := Ptr("vrfs", name, "proxyArpRanges", strconv.Itoa(i))
				lo, e1 := df2.ParseAddr(r.GetLow())
				hi, e2 := df2.ParseAddr(r.GetHigh())
				if e1 != nil || e2 != nil || !lo.Is4() || !hi.Is4() || hi.Less(lo) {
					s.Errorf(pt, "vrfs.neighbors-ra-proxy-arp-range", "proxy-ARP range %s–%s: two IPv4 addresses, low ≤ high", r.GetLow(), r.GetHigh())
					continue
				}
				pr := &arp.ProxyRange{TableId: table, Low: lo.String(), High: hi.String()}
				s.Add(scheduler.Join(arp.RangeName, strconv.FormatUint(uint64(table), 10), lo.String()+"-"+hi.String()), pr, pt)
			}
		}
	}
	if in["routing"] {
		nb := ds.GetRouting().GetNeighbors()
		for i, st := range nb.GetStatic() {
			pt := Ptr("routing", "neighbors", "static", strconv.Itoa(i))
			ip, err := df2.ParseAddr(st.GetIp())
			if err != nil {
				s.Errorf(pt+"/ip", "routing.neighbors-ra-static", "%v", err)
				continue
			}
			mac, err := df2.ParseMAC(st.GetMac())
			if err != nil {
				s.Errorf(pt+"/mac", "routing.neighbors-ra-static", "%v", err)
				continue
			}
			v := &ipneighbor.Neighbor{Interface: st.GetInterface(), IpAddress: ip.String(), MacAddress: df2.MACString(mac), NoFibEntry: st.GetNoFibEntry()}
			s.Add(scheduler.Join(ipneighbor.NeighborName, v.Interface, v.IpAddress), v, pt)
		}
		limits := []struct {
			af  df2.AddressFamily
			id  string
			key string
			l   *vrxv1.NeighborLimits
		}{{df2.AddressFamily_IPV4, "ipv4", "ipv4Limits", nb.GetIpv4Limits()}, {df2.AddressFamily_IPV6, "ipv6", "ipv6Limits", nb.GetIpv6Limits()}}
		for _, l := range limits {
			pt := Ptr("routing", "neighbors", l.key)
			if !opts.GlobalsOwner {
				if l.l != nil {
					s.Warnf(pt, ruleUnsupported, "neighbour-table limits are VPP-wide: only the globals owner applies them (D-071); not applied by this agent")
				}
				continue
			}
			c := &ipneighbor.Config{Af: l.af, MaxNumber: ipneighbor.DefaultMaxNumber, MaxAge: ipneighbor.DefaultMaxAge, Recycle: ipneighbor.DefaultRecycle}
			if l.l != nil {
				if l.l.MaxNumber != nil {
					c.MaxNumber = l.l.GetMaxNumber()
				}
				c.MaxAge, c.Recycle = l.l.GetMaxAgeSec(), l.l.GetRecycle()
			} else {
				pt = Ptr("routing")
			}
			s.Add(scheduler.Join(ipneighbor.ConfigName, l.id), c, pt)
		}
		if d := nb.GetDad(); d != nil {
			pt := Ptr("routing", "neighbors", "dad")
			if !opts.GlobalsOwner {
				s.Warnf(pt, ruleUnsupported, "duplicate address detection is VPP-wide: only the globals owner applies it (D-071); not applied by this agent")
			} else {
				tx, delay := uint32(ip6nd.DefaultDadTransmits), float64(ip6nd.DefaultDadRetransmitDelay)
				if d.Transmits != nil {
					tx = d.GetTransmits()
				}
				if d.DelayMs != nil {
					delay = float64(d.GetDelayMs()) / 1000
				}
				s.Add(scheduler.Join(ip6nd.DadName, "global"), &ip6nd.Dad{Transmits: tx, RetransmitDelay: delay}, pt)
			}
		}
	}
}

// ptrSegs splits a pointer built with Ptr back into its (unescaped) segments.
func ptrSegs(p string) []string {
	var out []string
	for _, s := range strings.Split(strings.TrimPrefix(p, "/"), "/") {
		out = append(out, strings.NewReplacer("~1", "/", "~0", "~").Replace(s))
	}
	return out
}

// AssembleNeighborsRa adds the retrieved F-neighbors-ra objects to ds (after desired.Assemble and the routes).
// stored is the agent's stored `interfaces` document: for an interface it names with a leaf VPP shows in its default
// ("off") state, the leaf is reported in that state — VPP's own value, in the document's representation — so a
// configuration that holds defaults is not drift. tableName names a FIB table.
func AssembleNeighborsRa(ds *vrxv1.DesiredState, kvs []scheduler.KV, in map[string]bool, stored map[string]*vrxv1.Interface, tableName func(uint32) string) {
	if in["interfaces"] {
		assembleInterfaces(ds, kvs, stored, tableName)
	}
	if in["vrfs"] {
		var ranges []*arp.ProxyRange
		for _, kv := range kvs {
			if r, ok := kv.Value.(*arp.ProxyRange); ok {
				ranges = append(ranges, r)
			}
		}
		sort.Slice(ranges, func(i, j int) bool { return rangeLess(ranges[i], ranges[j]) })
		for _, r := range ranges {
			name := tableName(r.GetTableId())
			if ds.Vrfs == nil {
				ds.Vrfs = map[string]*vrxv1.Vrf{}
			}
			v := ds.Vrfs[name]
			if v == nil {
				v = &vrxv1.Vrf{Id: proto.Uint32(r.GetTableId())}
				ds.Vrfs[name] = v
			}
			v.ProxyArpRanges = append(v.ProxyArpRanges, &vrxv1.ProxyArpRange{Low: proto.String(r.GetLow()), High: proto.String(r.GetHigh())})
		}
	}
	if in["routing"] {
		assembleRouting(ds, kvs)
	}
}

func rangeLess(a, b *arp.ProxyRange) bool {
	if a.GetTableId() != b.GetTableId() {
		return a.GetTableId() < b.GetTableId()
	}
	la, _ := df2.ParseAddr(a.GetLow())
	lb, _ := df2.ParseAddr(b.GetLow())
	if la != lb {
		return la.Less(lb)
	}
	ha, _ := df2.ParseAddr(a.GetHigh())
	hb, _ := df2.ParseAddr(b.GetHigh())
	return ha.Less(hb)
}

// raTarget is the assembled (sub-)interface message that holds the leaves of one logical name.
type raTarget struct {
	ra       **vrxv1.Ipv6Ra
	proxyArp **bool
	proxyNd  *[]string
}

func assembleInterfaces(ds *vrxv1.DesiredState, kvs []scheduler.KV, stored map[string]*vrxv1.Interface, tableName func(uint32) string) {
	target := func(name string, create bool) (raTarget, bool) {
		if itf, ok := ds.Interfaces[name]; ok {
			return raTarget{&itf.Ipv6Ra, &itf.ProxyArp, &itf.ProxyNd}, true
		}
		if i := strings.LastIndexByte(name, '.'); i > 0 {
			if parent, ok := ds.Interfaces[name[:i]]; ok {
				if sub, ok := parent.GetSubinterfaces()[name[i+1:]]; ok {
					return raTarget{&sub.Ipv6Ra, &sub.ProxyArp, &sub.ProxyNd}, true
				}
			}
		}
		if !create {
			return raTarget{}, false
		}
		// an object on an interface desired.Assemble did not report (e.g. an untagged NIC the document no longer names)
		if ds.Interfaces == nil {
			ds.Interfaces = map[string]*vrxv1.Interface{}
		}
		itf := &vrxv1.Interface{Enabled: proto.Bool(false), Promiscuous: proto.Bool(false), Vrf: proto.String(tableName(0))}
		ds.Interfaces[name] = itf
		return raTarget{&itf.Ipv6Ra, &itf.ProxyArp, &itf.ProxyNd}, true
	}
	raOf := func(t raTarget) *vrxv1.Ipv6Ra {
		if *t.ra == nil {
			*t.ra = defaultIpv6Ra()
		}
		return *t.ra
	}
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *ip6nd.RaConfig:
			t, _ := target(v.GetInterface(), true)
			ra := raOf(t)
			ra.Suppress, ra.Managed, ra.Other = proto.Bool(v.GetSuppress()), proto.Bool(v.GetManaged()), proto.Bool(v.GetOther())
			ra.LifetimeSec, ra.MaxIntervalSec, ra.MinIntervalSec = proto.Uint32(v.GetRouterLifetime()), proto.Uint32(v.GetMaxInterval()), proto.Uint32(v.GetMinInterval())
		case *ip6nd.RaPrefix:
			t, _ := target(v.GetInterface(), true)
			ra := raOf(t)
			if ra.Prefixes == nil {
				ra.Prefixes = map[string]*vrxv1.Ipv6RaPrefix{}
			}
			ra.Prefixes[v.GetPrefix()] = &vrxv1.Ipv6RaPrefix{
				ValidSec: proto.Uint32(v.GetValidLifetime()), PreferredSec: proto.Uint32(v.GetPreferredLifetime()),
				OffLink: proto.Bool(v.GetOffLink()), NoAutoconfig: proto.Bool(v.GetNoAutoconfig()),
			}
		case *arp.ProxyInterface:
			t, _ := target(v.GetInterface(), true)
			*t.proxyArp = proto.Bool(true)
		case *ip6nd.ProxyNd:
			t, _ := target(v.GetInterface(), true)
			*t.proxyNd = append(*t.proxyNd, v.GetAddress())
		}
	}
	// the stored document's "off" leaves on interfaces that exist: VPP is in the default state (nothing retrieved)
	for name, itf := range stored {
		nodes := []struct {
			name string
			ra   *vrxv1.Ipv6Ra
			pa   *bool
		}{{name, itf.GetIpv6Ra(), itf.ProxyArp}}
		for id, sub := range itf.GetSubinterfaces() {
			nodes = append(nodes, struct {
				name string
				ra   *vrxv1.Ipv6Ra
				pa   *bool
			}{SubName(name, id), sub.GetIpv6Ra(), sub.ProxyArp})
		}
		for _, n := range nodes {
			t, ok := target(n.name, false)
			if !ok {
				continue
			}
			if n.ra != nil && *t.ra == nil {
				*t.ra = defaultIpv6Ra()
			}
			if n.pa != nil && !*n.pa && *t.proxyArp == nil {
				*t.proxyArp = proto.Bool(false)
			}
		}
	}
	for _, itf := range ds.Interfaces {
		sort.Slice(itf.ProxyNd, func(i, j int) bool { return addrLess(itf.ProxyNd[i], itf.ProxyNd[j]) })
		for _, sub := range itf.GetSubinterfaces() {
			sort.Slice(sub.ProxyNd, func(i, j int) bool { return addrLess(sub.ProxyNd[i], sub.ProxyNd[j]) })
		}
	}
}

// defaultIpv6Ra is VPP's fresh-interface RA state in the document's form.
func defaultIpv6Ra() *vrxv1.Ipv6Ra {
	return &vrxv1.Ipv6Ra{
		Suppress: proto.Bool(ip6nd.DefaultSuppress), Managed: proto.Bool(false), Other: proto.Bool(false),
		LifetimeSec: proto.Uint32(ip6nd.DefaultRouterLifetime), MaxIntervalSec: proto.Uint32(ip6nd.DefaultMaxInterval),
		MinIntervalSec: proto.Uint32(ip6nd.DefaultMinInterval),
	}
}

func addrLess(a, b string) bool {
	x, e1 := df2.ParseAddr(a)
	y, e2 := df2.ParseAddr(b)
	if e1 != nil || e2 != nil {
		return a < b
	}
	return x.Less(y)
}

func assembleRouting(ds *vrxv1.DesiredState, kvs []scheduler.KV) {
	nb := &vrxv1.NeighborsConfig{}
	var statics []*ipneighbor.Neighbor
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *ipneighbor.Neighbor:
			statics = append(statics, v)
		case *ipneighbor.Config:
			if v.GetMaxNumber() == ipneighbor.DefaultMaxNumber && v.GetMaxAge() == ipneighbor.DefaultMaxAge && v.GetRecycle() == ipneighbor.DefaultRecycle {
				continue // VPP defaults = "absent" in the document
			}
			l := &vrxv1.NeighborLimits{MaxNumber: proto.Uint32(v.GetMaxNumber()), MaxAgeSec: proto.Uint32(v.GetMaxAge()), Recycle: proto.Bool(v.GetRecycle())}
			if v.GetAf() == df2.AddressFamily_IPV6 {
				nb.Ipv6Limits = l
			} else {
				nb.Ipv4Limits = l
			}
		case *ip6nd.Dad:
			nb.Dad = &vrxv1.NeighborDad{Transmits: proto.Uint32(v.GetTransmits()), DelayMs: proto.Uint32(uint32(math.Round(v.GetRetransmitDelay() * 1000)))}
		}
	}
	sort.Slice(statics, func(i, j int) bool {
		if statics[i].GetInterface() != statics[j].GetInterface() {
			return statics[i].GetInterface() < statics[j].GetInterface()
		}
		return addrLess(statics[i].GetIpAddress(), statics[j].GetIpAddress())
	})
	for _, v := range statics {
		nb.Static = append(nb.Static, &vrxv1.StaticNeighbor{
			Interface: proto.String(v.GetInterface()), Ip: proto.String(v.GetIpAddress()),
			Mac: proto.String(v.GetMacAddress()), NoFibEntry: proto.Bool(v.GetNoFibEntry()),
		})
	}
	if proto.Size(nb) == 0 {
		return
	}
	if ds.Routing == nil {
		ds.Routing = &vrxv1.RoutingConfig{}
	}
	ds.Routing.Neighbors = nb
}
