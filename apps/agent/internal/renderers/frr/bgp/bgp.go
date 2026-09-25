// Package bgp renders `routing.bgp` into FRR's `router bgp` block (the `bgp` section of the RF-1 framework, P12) and
// reads BGP state back (`show bgp vrf all summary json`: the `bgpSummary` state reader, the `bgp-neighbors` 1 Hz poller
// and Summary for the RoutingState RPC). Filters are `routing.policy` objects rendered by package policy; this package
// only names them. Canonical forms follow FRR 10.7's `show running-config` (the framework's convergence check fails on
// anything else): docs/agent/renderers/frr-bgp.md.
//
// Rendering rules:
//   - `no bgp default ipv4-unicast` always: a family is active for a peer exactly when the document lists it under
//     `afi` (an absent family is not activated, schema), never implicitly;
//   - `no bgp ebgp-requires-policy` when ebgpRequiresPolicy is false (FRR's default is on, RFC 8212);
//   - peer groups before neighbours, neighbours by address; per peer only the attributes the document sets
//     (a member inherits the rest from its peer group, as in FRR);
//   - timers: FRR takes keepalive and hold together — a missing one is derived (hold = 3 × keepalive, keepalive =
//     hold / 3) and both are rendered;
//   - `passwordRef` through rc.Secret only (D-051/D-072: the value never leaves frr.conf and FRR; Retrieve, DryRun and
//     errors are redacted by the framework); without a secret resolver the render fails with frr.ErrNoSecretResolver;
//   - an address-family block is rendered only when it has content (FRR never prints an empty one).
package bgp

import (
	"cmp"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strconv"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/policy"
)

// OrderBGP places `router bgp` in the protocol range (after the IGPs' 420–480, as FRR prints it).
const OrderBGP = 500

// Name is the section name.
const Name = "bgp"

func init() {
	frr.RegisterSection(Section{})
	frr.RegisterStateReader(frr.StateReader{Key: SummaryReader, Command: ShowSummary})
	frr.RegisterPoller(PollerNeighbors, PollNeighbors)
}

// Section is the frr.Section of `routing.bgp`.
type Section struct{}

// Name implements frr.Section.
func (Section) Name() string { return Name }

// Order implements frr.Section.
func (Section) Order() int { return OrderBGP }

// Render implements frr.Section.
func (Section) Render(rc *frr.RenderContext) ([]string, error) {
	return Render(rc.Desired.GetRouting().GetBgp(), rc)
}

// Secrets resolves passwordRef values and maps VPP interface names (the parts of frr.RenderContext Render uses).
type Secrets interface {
	Secret(ref string) (string, error)
	MapInterface(vppName string) (string, bool)
}

// peer is the part of a neighbour or peer group Render uses.
type peer interface {
	GetRemoteAs() uint32
	GetDescription() string
	GetUpdateSource() string
	GetEbgpMultihop() uint32
	GetPasswordRef() string
	GetKeepaliveSec() uint32
	GetHoldTimeSec() uint32
	GetBfd() bool
	GetAfi() *vrxv1.BgpAfi
}

// Render returns the `router bgp` block for b (nil = BGP not configured).
func Render(b *vrxv1.BgpConfig, rc Secrets) ([]string, error) {
	if b == nil {
		return nil, nil
	}
	base := policy.P("routing", "bgp")
	if b.GetAsn() == 0 {
		return nil, policy.Errf(base.At("asn"), "asn is required (1–4294967295)")
	}
	head := fmt.Sprintf("router bgp %d", b.GetAsn())
	if v := b.GetVrf(); v != "" && v != frr.DefaultVRF {
		name, err := frr.VRFName(v)
		if err != nil {
			return nil, policy.Wrap(base.At("vrf"), err)
		}
		head += " vrf " + name
	}
	out := []string{head}
	if rid := b.GetRouterId(); rid != "" {
		a, err := netip.ParseAddr(rid)
		if err != nil || !a.Is4() {
			return nil, policy.Wrap(base.At("routerId"), fmt.Errorf("%w: router id %q is not a dotted quad", renderers.ErrUnsafe, rid))
		}
		out = append(out, " bgp router-id "+a.String())
	}
	if b.EbgpRequiresPolicy != nil && !b.GetEbgpRequiresPolicy() {
		out = append(out, " no bgp ebgp-requires-policy")
	}
	out = append(out, " no bgp default ipv4-unicast")
	if b.GetGracefulRestart() {
		out = append(out, " bgp graceful-restart")
	}

	groups := b.GetPeerGroups()
	groupNames := slices.Sorted(maps.Keys(groups))
	for _, name := range groupNames {
		path := base.At("peerGroups", name)
		if _, err := policy.ObjectName("peer group", name); err != nil {
			return nil, policy.Wrap(path, err)
		}
		out = append(out, " neighbor "+name+" peer-group")
		lines, err := peerLines(name, groups[name], path, rc)
		if err != nil {
			return nil, err
		}
		out = append(out, lines...)
	}
	nbrs, err := sortedNeighbors(b.GetNeighbors(), base.At("neighbors"))
	if err != nil {
		return nil, err
	}
	for _, n := range nbrs {
		path := base.At("neighbors", n.key)
		nb := n.cfg
		if pg := nb.GetPeerGroup(); pg != "" {
			if _, ok := groups[pg]; !ok {
				return nil, policy.Errf(path.At("peerGroup"), "peer group %q does not exist", pg)
			}
		} else if nb.GetRemoteAs() == 0 {
			return nil, policy.Errf(path.At("remoteAs"), "remoteAs is required without a peer group")
		}
		// FRR prints remote-as before peer-group for a member with an AS of its own
		if nb.GetRemoteAs() != 0 {
			out = append(out, fmt.Sprintf(" neighbor %s remote-as %d", n.addr, nb.GetRemoteAs()))
		}
		if pg := nb.GetPeerGroup(); pg != "" {
			out = append(out, fmt.Sprintf(" neighbor %s peer-group %s", n.addr, pg))
		}
		lines, err := peerLines(n.addr, nb, path, rc)
		if err != nil {
			return nil, err
		}
		out = append(out, lines...)
		if nb.GetShutdown() {
			out = append(out, " neighbor "+n.addr+" shutdown")
		}
	}

	for _, af := range []afi{afiIPv4, afiIPv6} {
		lines, err := afBlock(af, b, groupNames, nbrs)
		if err != nil {
			return nil, err
		}
		out = append(out, lines...)
	}
	return append(out, "exit"), nil
}

type neighbor struct {
	key  string // document key
	addr string // canonical address
	ip   netip.Addr
	cfg  *vrxv1.BgpNeighbor
}

func sortedNeighbors(m map[string]*vrxv1.BgpNeighbor, path policy.Path) ([]neighbor, error) {
	out := make([]neighbor, 0, len(m))
	seen := map[netip.Addr]string{}
	for k, v := range m {
		a, err := netip.ParseAddr(k)
		if err != nil || a.Zone() != "" || a.IsUnspecified() || a.IsMulticast() || a.IsLoopback() {
			return nil, policy.Errf(path.At(k), "key %q is not a unicast neighbour address", k)
		}
		if prev, dup := seen[a]; dup {
			return nil, policy.Errf(path.At(k), "%q and %q are the same address", prev, k)
		}
		seen[a] = k
		out = append(out, neighbor{key: k, addr: a.String(), ip: a, cfg: v})
	}
	slices.SortFunc(out, func(x, y neighbor) int { return x.ip.Compare(y.ip) })
	return out, nil
}

// peerLines renders the session attributes shared by neighbours and peer groups.
func peerLines(id string, p peer, path policy.Path, rc Secrets) ([]string, error) {
	var out []string
	add := func(format string, a ...any) {
		out = append(out, fmt.Sprintf(" neighbor %s "+format, append([]any{id}, a...)...))
	}
	if _, isGroup := p.(*vrxv1.BgpPeerGroup); isGroup && p.GetRemoteAs() != 0 {
		add("remote-as %d", p.GetRemoteAs())
	}
	if d := p.GetDescription(); d != "" {
		desc, err := frr.Description(d)
		if err != nil {
			return nil, policy.Wrap(path.At("description"), err)
		}
		add("description %s", desc)
	}
	if h := p.GetEbgpMultihop(); h != 0 {
		switch {
		case h > 255:
			return nil, policy.Errf(path.At("ebgpMultihop"), "ebgpMultihop %d not in 1–255", h)
		case h == 255:
			add("ebgp-multihop") // FRR prints the maximum TTL without a number
		default:
			add("ebgp-multihop %d", h)
		}
	}
	if ref := p.GetPasswordRef(); ref != "" {
		pw, err := rc.Secret(ref)
		if err != nil {
			return nil, policy.Wrap(path.At("passwordRef"), err)
		}
		add("password %s", pw)
	}
	if k, h := p.GetKeepaliveSec(), p.GetHoldTimeSec(); k != 0 || h != 0 {
		switch {
		case k == 0:
			k = max(h/3, 1)
		case h == 0:
			h = 3 * k
		}
		if k > 65535 || h > 65535 || (h != 0 && h < 3) || h <= k {
			return nil, policy.Errf(path.At("holdTimeSec"), "timers keepalive %d hold %d (hold must be ≥ 3 and greater than keepalive, both ≤ 65535)", k, h)
		}
		add("timers %d %d", k, h)
	}
	if src := p.GetUpdateSource(); src != "" {
		if a, err := netip.ParseAddr(src); err == nil {
			if a.Zone() != "" || a.IsUnspecified() || a.IsMulticast() {
				return nil, policy.Errf(path.At("updateSource"), "updateSource %q is not a unicast address", src)
			}
			add("update-source %s", a)
		} else {
			linux, ok := rc.MapInterface(src)
			if !ok {
				return nil, policy.Errf(path.At("updateSource"), "interface %q has no Linux interface for FRR (interfaces.%s.lcp)", src, src)
			}
			name, err := frr.IfName(linux)
			if err != nil {
				return nil, policy.Wrap(path.At("updateSource"), err)
			}
			add("update-source %s", name)
		}
	}
	if p.GetBfd() {
		add("bfd")
	}
	return out, nil
}

type afi struct {
	key   string // document key
	frr   string // FRR address-family keyword
	is6   bool
	get   func(*vrxv1.BgpAfi) *vrxv1.BgpAddressFamily
	plFam string
}

var (
	afiIPv4 = afi{key: "ipv4Unicast", frr: "ipv4 unicast", get: (*vrxv1.BgpAfi).GetIpv4Unicast, plFam: "ipv4"}
	afiIPv6 = afi{key: "ipv6Unicast", frr: "ipv6 unicast", is6: true, get: (*vrxv1.BgpAfi).GetIpv6Unicast, plFam: "ipv6"}
)

// afBlock renders one `address-family … exit-address-family` block, or nothing when it has no content.
func afBlock(af afi, b *vrxv1.BgpConfig, groups []string, nbrs []neighbor) ([]string, error) {
	var in []string
	nets := slices.Clone(b.GetNetworks())
	slices.SortStableFunc(nets, func(x, y *vrxv1.BgpNetwork) int { return cmp.Compare(x.GetPrefix(), y.GetPrefix()) })
	var pfxs []netip.Prefix
	for i, n := range nets {
		path := policy.P("routing", "bgp", "networks").Index(i)
		p, err := netip.ParsePrefix(n.GetPrefix())
		if err != nil || p.Masked() != p {
			return nil, policy.Errf(path.At("prefix"), "prefix %q is not a network prefix", n.GetPrefix())
		}
		if p.Addr().Is6() != af.is6 {
			continue
		}
		pfxs = append(pfxs, p)
		line := "  network " + p.String()
		if rm := n.GetRouteMap(); rm != "" {
			if _, err := policy.ObjectName("route map", rm); err != nil {
				return nil, policy.Wrap(path.At("routeMap"), err)
			}
			line += " route-map " + rm
		}
		in = append(in, line)
	}
	// FRR sorts networks by prefix: keep the address order, not the text order
	sortByPrefix(in, pfxs)
	redist, err := redistribute(b.GetRedistribute())
	if err != nil {
		return nil, err
	}
	activeFamily := len(pfxs) > 0
	var peers []string
	for _, g := range groups {
		lines, active, err := peerAF(g, af, b.GetPeerGroups()[g].GetAfi(), policy.P("routing", "bgp", "peerGroups", g))
		if err != nil {
			return nil, err
		}
		activeFamily = activeFamily || active
		peers = append(peers, lines...)
	}
	for _, n := range nbrs {
		lines, active, err := peerAF(n.addr, af, n.cfg.GetAfi(), policy.P("routing", "bgp", "neighbors", n.key))
		if err != nil {
			return nil, err
		}
		activeFamily = activeFamily || active
		peers = append(peers, lines...)
	}
	// redistribution is rendered in IPv4 always and in IPv6 when the family is in use
	if !af.is6 || activeFamily {
		in = append(in, redist...)
	}
	in = append(in, peers...)
	if len(in) == 0 {
		return nil, nil
	}
	return append(append([]string{" !", " address-family " + af.frr}, in...), " exit-address-family"), nil
}

func sortByPrefix(lines []string, pfxs []netip.Prefix) {
	idx := make([]int, len(pfxs))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int {
		return cmp.Or(pfxs[a].Addr().Compare(pfxs[b].Addr()), cmp.Compare(pfxs[a].Bits(), pfxs[b].Bits()))
	})
	sorted := make([]string, len(idx))
	for i, k := range idx {
		sorted[i] = lines[k]
	}
	copy(lines, sorted)
}

// redistribute renders `redistribute <source> [metric N] [route-map R]` in FRR's source order.
func redistribute(r *vrxv1.Redistribute) ([]string, error) {
	if r == nil {
		return nil, nil
	}
	var out []string
	for _, src := range []struct {
		name string
		opt  *vrxv1.RedistributeOptions
	}{
		{"connected", r.GetConnected()}, {"static", r.GetStatic()}, {"rip", r.GetRip()},
		{"ospf", r.GetOspf()}, {"isis", r.GetIsis()},
	} {
		if src.opt == nil {
			continue
		}
		line := "  redistribute " + src.name
		if src.opt.Metric != nil {
			line += " metric " + strconv.FormatUint(uint64(src.opt.GetMetric()), 10)
		}
		if rm := src.opt.GetRouteMap(); rm != "" {
			if _, err := policy.ObjectName("route map", rm); err != nil {
				return nil, policy.Wrap(policy.P("routing", "bgp", "redistribute", src.name, "routeMap"), err)
			}
			line += " route-map " + rm
		}
		out = append(out, line)
	}
	if r.GetBgp() != nil {
		return nil, policy.Errf(policy.P("routing", "bgp", "redistribute", "bgp"), "BGP cannot redistribute into itself")
	}
	return out, nil
}

// peerAF renders one peer's lines inside an address family; active reports whether the peer activates it.
func peerAF(id string, af afi, a *vrxv1.BgpAfi, path policy.Path) ([]string, bool, error) {
	f := af.get(a)
	if f == nil || (f.Enabled != nil && !f.GetEnabled()) {
		return nil, false, nil
	}
	path = path.At("afi", af.key)
	out := []string{"  neighbor " + id + " activate"}
	if f.GetNextHopSelf() {
		out = append(out, "  neighbor "+id+" next-hop-self")
	}
	if f.GetDefaultOriginate() {
		out = append(out, "  neighbor "+id+" default-originate")
	}
	if f.GetSoftReconfig() {
		out = append(out, "  neighbor "+id+" soft-reconfiguration inbound")
	}
	for _, x := range []struct {
		name, dir, kind, field string
	}{
		{f.GetPrefixListIn(), "in", "prefix-list", "prefixListIn"}, {f.GetPrefixListOut(), "out", "prefix-list", "prefixListOut"},
		{f.GetRouteMapIn(), "in", "route-map", "routeMapIn"}, {f.GetRouteMapOut(), "out", "route-map", "routeMapOut"},
	} {
		if x.name == "" {
			continue
		}
		if _, err := policy.ObjectName(x.kind, x.name); err != nil {
			return nil, false, policy.Wrap(path.At(x.field), err)
		}
		out = append(out, fmt.Sprintf("  neighbor %s %s %s %s", id, x.kind, x.name, x.dir))
	}
	if n := f.GetMaximumPrefixes(); n != 0 {
		out = append(out, fmt.Sprintf("  neighbor %s maximum-prefix %d", id, n))
	}
	return out, true, nil
}
