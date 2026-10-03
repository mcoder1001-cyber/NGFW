package desired

// F-det44-map-dslite-cnat: `nat.dslite` onto the dslite descriptors (docs/agent/descriptors/dslite.md) and back.
//
//	nat.dslite.aftr {ipv6, ipv4?}  → dslite.aftr/global (VPP global, D-071: a slot agent only requires it)
//	nat.dslite.b4 {ipv6, ipv4?}    → dslite.b4/global   (VPP global, D-071; CE side beyond the address is out of scope)
//	nat.dslite.pools[i] {range}    → dslite.pool/<first>-<last> (claim-store ownership like nat64 pools)
//
// `enabled: false` keeps the configuration and programs nothing.

import (
	"net/netip"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/dslite"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

func dsliteEndpoint(s Sink, e *ngfwv1.DsliteConfig_Endpoint, name, key string) {
	if e == nil {
		return
	}
	pt := Ptr("nat", "dslite", key)
	v6, err := netip.ParseAddr(e.GetIpv6())
	if err != nil || !v6.Is6() || v6.Is4In6() || v6.IsUnspecified() {
		s.Errorf(pt+"/ipv6", "nat.dslite-valid", "%q is not an IPv6 address", e.GetIpv6())
		return
	}
	spec := dslite.EndpointSpec{IPv6: v6.String()}
	if e.Ipv4 != nil {
		v4, err := netip.ParseAddr(e.GetIpv4())
		if err != nil || !v4.Is4() {
			s.Errorf(pt+"/ipv4", "nat.dslite-valid", "%q is not an IPv4 address", e.GetIpv4())
			return
		}
		spec.IPv4 = v4.String()
	}
	spec.Normalize()
	natAdd(s, name, dslite.Singleton, &spec, pt)
}

func dsliteBuild(s Sink, d *ngfwv1.DsliteConfig) {
	if d == nil || !d.GetEnabled() {
		return
	}
	dsliteEndpoint(s, d.GetAftr(), dslite.NameAftr, "aftr")
	dsliteEndpoint(s, d.GetB4(), dslite.NameB4, "b4")
	for i, p := range d.GetPools() {
		pt := Ptr("nat", "dslite", "pools", strconv.Itoa(i))
		first, last, err := ParseNatRange(p.GetRange())
		if err != nil {
			s.Errorf(pt+"/range", "nat.pools-valid", "%v", err)
			continue
		}
		if sz := natRangeSize(first, last); sz > MaxPoolAddresses {
			s.Errorf(pt+"/range", "nat.pool-size", "pool range %q has %d addresses; the agent adds at most %d per range (split it)", p.GetRange(), sz, MaxPoolAddresses)
			continue
		}
		spec := dslite.PoolSpec{First: first.String(), Last: last.String()}
		spec.Normalize()
		natAdd(s, dslite.NamePool, dslite.PoolID(spec), &spec, pt)
	}
}

// assembleDslite builds `nat.dslite` from retrieved objects: AFTR/B4 only where retrieved (globals owner), pools
// merged back into ranges by the descriptor.
func assembleDslite(out *ngfwv1.NatConfig, kvs []scheduler.KV) {
	var (
		anyDS    bool
		aftr, b4 *dslite.EndpointSpec
		pools    []dslite.PoolSpec
	)
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case dslite.NameAftr:
			if v, err := natcommon.Decode[dslite.EndpointSpec](kv.Value); err == nil {
				aftr = &v
			}
		case dslite.NameB4:
			if v, err := natcommon.Decode[dslite.EndpointSpec](kv.Value); err == nil {
				b4 = &v
			}
		case dslite.NamePool:
			if v, err := natcommon.Decode[dslite.PoolSpec](kv.Value); err == nil {
				pools = append(pools, v)
			}
		default:
			continue
		}
		anyDS = true
	}
	if !anyDS {
		return
	}
	d := &ngfwv1.DsliteConfig{Enabled: proto.Bool(true)}
	ep := func(e *dslite.EndpointSpec) *ngfwv1.DsliteConfig_Endpoint {
		if e == nil {
			return nil
		}
		r := &ngfwv1.DsliteConfig_Endpoint{Ipv6: proto.String(e.IPv6)}
		if e.IPv4 != "" {
			r.Ipv4 = proto.String(e.IPv4)
		}
		return r
	}
	d.Aftr, d.B4 = ep(aftr), ep(b4)
	sort.Slice(pools, func(a, b int) bool { return natAddrLess(pools[a].First, pools[b].First) })
	for _, p := range pools {
		r := p.First
		if p.Last != p.First {
			r += "-" + p.Last
		}
		d.Pools = append(d.Pools, &ngfwv1.DsliteConfig_Pool{Range: proto.String(r)})
	}
	out.Dslite = d
}
