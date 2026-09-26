package desired

// P12 builder and assembler of the linux-cp pairs (wave-A-hotspots A2: projection.go calls them under its anchors).
//
//	interfaces.<n>.lcp{hostIfName, hostIfType, netns}   → lcp.itf-pair/<n>   (DF-8; depends on interface/<n>)
//
// Canonical form (Retrieve): hostIfName is left out when it equals the VPP name (the schema's default), hostIfType is
// always set ("tap" default), netns is left out when empty (the linux-cp default namespace).

import (
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/lcpmap"
	"ngfw/agent/internal/scheduler"
)

// Lcp projects `interfaces.<n>.lcp` of the parent interfaces into lcp.itf-pair objects.
func Lcp(s Sink, ifs map[string]*vrxv1.Interface) {
	for _, name := range sortedKeys(ifs) {
		l := ifs[name].GetLcp()
		if ifs[name].Lcp == nil {
			continue
		}
		pt := Ptr("interfaces", name, "lcp")
		host, err := lcpmap.HostName(name, l)
		if err != nil {
			s.Errorf(pt+"/hostIfName", "routing.bgp-lcp-host-name", "%v", err)
			continue
		}
		if ns := l.GetNetns(); ns != "" {
			// review M4: linux-nl hears a pair only when its netns equals the linux-cp default netns at pair-add time
			// (VPP lcp_interface.c); the agent cannot see the box's startup value, so it warns at the field
			s.Warnf(pt+"/netns", "routing.bgp-lcp-netns",
				"netns %q must equal the box's linux-cp default netns (startup.conf linux-cp { default netns }); otherwise linux-nl ignores this pair: no routes, addresses, admin state or MTU reach VPP through it — leave it empty to use the default", ns)
		}
		pair := lcp.ItfPair{Interface: name, HostIfName: host, HostIfType: lcpmap.HostType(l), Netns: l.GetNetns()}
		if err := pair.Validate(); err != nil {
			s.Errorf(pt, "routing.bgp-lcp-pair", "%v", err)
			continue
		}
		s.Add(scheduler.Join(lcp.NameItfPair, name), pair.Proto(), pt)
	}
}

// AssembleLcp adds `interfaces.<n>.lcp` from the retrieved lcp.itf-pair objects (an interface the assembled document
// does not list yet gets an entry of its own).
func AssembleLcp(ds *vrxv1.DesiredState, kvs []scheduler.KV) {
	for _, kv := range kvs {
		if kv.Key.Descriptor() != lcp.NameItfPair {
			continue
		}
		var p lcp.ItfPair
		if err := dfkit.Decode(kv.Value, &p); err != nil || p.Interface == "" {
			continue
		}
		l := &vrxv1.InterfaceLcp{HostIfType: strPtr(p.HostIfType)}
		if p.HostIfName != p.Interface {
			l.HostIfName = strPtr(p.HostIfName)
		}
		if p.Netns != "" {
			l.Netns = strPtr(p.Netns)
		}
		if ds.Interfaces == nil {
			ds.Interfaces = map[string]*vrxv1.Interface{}
		}
		itf := ds.Interfaces[p.Interface]
		if itf == nil {
			itf = &vrxv1.Interface{}
			ds.Interfaces[p.Interface] = itf
		}
		itf.Lcp = l
	}
}

func strPtr(s string) *string { return &s }
