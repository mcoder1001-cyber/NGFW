package desired

// F-unbound-chrony-syslog: the DNS part of the `services` domain.
//
//	services.dns.resolvers           → unbound.config/vrx                 Value = unbound.Input(services.dns)
//	services.dns.vppCache (enabled)  → dns.name-server/<ip> per upstream  (DF-8, VPP-global, write-only)
//	                                   + dns.enable/global                 (applied by the globals owner, required —
//	                                                                        never set — by every other agent, D-071)
//
// The Unbound object is a singleton: one daemon serves every resolver (RF-3), so the resolvers are one object whose
// Value is the render input. It exists while the document has a resolver (enabled or not); without one the
// reconciler deletes it and Unbound goes back to the idle rendering. VPP's dns plugin has no getter (D-063): the VPP
// cache is never part of a Retrieve result, which DryRun says with an agent.unsupported-field note so /state/drift
// does not compare it.

import (
	"net/netip"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	dnsd "ngfw/agent/internal/descriptors/dns"
	"ngfw/agent/internal/renderers/unbound"
	"ngfw/agent/internal/scheduler"
)

func init() {
	ServicesImplemented["dns"] = true
}

// DNS projects services.dns (see the file comment).
func DNS(s Sink, ds *vrxv1.DesiredState) {
	dns := ds.GetServices().GetDns()
	if in := unbound.Input(dns); in != nil {
		s.Add(unbound.Key, in, Ptr("services", "dns", "resolvers"))
	}
	vc := dns.GetVppCache()
	if vc == nil {
		return
	}
	s.Warnf(Ptr("services", "dns", "vppCache"), "agent.unsupported-field",
		"services.dns.vppCache is a write-only VPP global (no getter, D-063): the globals owner applies it and every other agent only requires it (D-071); Retrieve never reports it")
	if !vc.GetEnabled() {
		return
	}
	seen := map[string]bool{}
	for i, u := range vc.GetUpstreams() {
		pt := Ptr("services", "dns", "vppCache", "upstreams", strconv.Itoa(i))
		a, err := netip.ParseAddr(u)
		if err != nil || a.Zone() != "" {
			s.Errorf(pt, "services.dns-vpp-cache-upstream", "%q is not an IP address", u)
			continue
		}
		addr := a.Unmap().String()
		if seen[addr] {
			s.Errorf(pt, "services.dns-vpp-cache-upstream", "upstream %s twice", addr)
			continue
		}
		seen[addr] = true
		s.Add(scheduler.Join(dnsd.NameNameServer, addr), dnsd.NameServer{Address: addr}.Proto(), pt)
	}
	if len(seen) == 0 {
		s.Errorf(Ptr("services", "dns", "vppCache", "upstreams"), "services.dns-vpp-cache-upstream", "the VPP DNS cache needs an upstream name server (VPP refuses to enable without one)")
		return
	}
	s.Add(dnsd.KeyEnable, dnsd.Enable{Enabled: true}.Proto(), Ptr("services", "dns", "vppCache", "enabled"))
}

// AssembleDNS adds the resolvers of a retrieved Unbound object to ds (a drifted configuration is reported by its
// descriptor as a *structpb.Struct and contributes nothing, so the drift stays visible).
//
// The VPP cache objects are write-only: a Retrieve never carries them (the scheduler skips write-only descriptors),
// so vppCache is assembled only from KVs that do carry them — the projection's own output in the round-trip tests.
func AssembleDNS(ds *vrxv1.DesiredState, kvs []scheduler.KV) {
	var upstreams []string
	enabled := false
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case unbound.Name:
			in, ok := kv.Value.(*vrxv1.DnsService)
			if kv.Key != unbound.Key || !ok {
				continue
			}
			dnsOf(ds).Resolvers = in.GetResolvers()
		case dnsd.NameNameServer:
			upstreams = append(upstreams, kv.Key.ID())
		case dnsd.NameEnable:
			var e dnsd.Enable
			if st, ok := kv.Value.(*structpb.Struct); ok && dfkit.Decode(st, &e) == nil {
				enabled = e.Enabled
			}
		}
	}
	if enabled && len(upstreams) > 0 {
		sort.Strings(upstreams)
		dnsOf(ds).VppCache = &vrxv1.DnsService_VppCache{Enabled: proto.Bool(true), Upstreams: upstreams}
	}
}

// dnsOf returns ds.services.dns, allocated.
func dnsOf(ds *vrxv1.DesiredState) *vrxv1.DnsService {
	svc := servicesOf(ds)
	if svc.Dns == nil {
		svc.Dns = &vrxv1.DnsService{}
	}
	return svc.Dns
}

// servicesOf returns ds.services, allocated.
func servicesOf(ds *vrxv1.DesiredState) *vrxv1.ServicesConfig {
	if ds.Services == nil {
		ds.Services = &vrxv1.ServicesConfig{}
	}
	return ds.Services
}
