package desired

// F-kea-dhcp-relay: the `services` domain's DHCP part.
//
//	services.dhcp.servers (family ipv4) → kea.dhcp4/vrx  Value = kea.Input(document, 4)
//	services.dhcp.servers (family ipv6) → kea.dhcp6/vrx  Value = kea.Input(document, 6)
//	services.dhcp.relays.<name>         → dhcp.proxy/<rx table>/<server table>/<server> per server (enabled relays)
//	                                      + dhcp.relay/<name> (the relay record, dhcp_relay.go)
//
// The Kea objects are singletons: one kea-dhcp4 (kea-dhcp6) daemon serves every server of its family (RF-3), so the
// whole family is one object whose Value is the render input (renderers/kea/input.go). Other `services` sub-keys are
// not projected by this file; ServicesUnsupported reports them as agent.unsupported-field so that /state/drift does
// not compare what the agent never applies.

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/kea"
	"ngfw/agent/internal/scheduler"
)

// ServicesImplemented lists the `services` sub-keys (JSON names) that this build projects. A feature that projects
// another sub-key adds it here from an init() in its own file (e.g. `ServicesImplemented["dns"] = true`).
var ServicesImplemented = map[string]bool{"dhcp": true}

// ServicesUnsupported warns agent.unsupported-field for every non-empty `services` sub-key that no builder projects.
func ServicesUnsupported(s Sink, svc *vrxv1.ServicesConfig) {
	if svc == nil {
		return
	}
	m := svc.ProtoReflect()
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if ServicesImplemented[fd.JSONName()] || !m.Has(fd) {
			continue
		}
		if fd.Kind() == protoreflect.MessageKind && proto.Size(m.Get(fd).Message().Interface()) == 0 {
			continue
		}
		s.Warnf(Ptr("services", fd.JSONName()), "agent.unsupported-field", "services.%s is not applied by this agent build", fd.JSONName())
	}
}

// Kea projects services.dhcp.servers onto the two Kea singletons (only for a family that has servers; a family
// without servers has no object, so the reconciler deletes it — the daemon goes back to the idle configuration).
// "One Kea VRF per family" is the semantic rule services.kea-dhcp-relay-one-vrf-per-family (API, 400) and the
// renderer's own check at apply time (ErrInvalid); it is not a projection error because the P02c example
// services-dhcp-dns.json (two IPv4 servers in two VRFs) must stay projectable (questions Q3).
func Kea(s Sink, ds *vrxv1.DesiredState) {
	servers := ds.GetServices().GetDhcp().GetServers()
	bad := false
	for _, name := range sortedKeys(servers) {
		if _, ok := kea.FamilyOf(servers[name]); !ok {
			s.Errorf(Ptr("services", "dhcp", "servers", name, "family"), "services.dhcp-family", "unknown family %q (ipv4 or ipv6)", servers[name].GetFamily())
			bad = true
		}
	}
	if bad {
		return
	}
	for _, fam := range []int{4, 6} {
		if in := kea.Input(ds, fam); in != nil {
			s.Add(kea.Key(fam), in, Ptr("services", "dhcp", "servers"))
		}
	}
}

// AssembleKea adds the servers of the retrieved Kea objects to ds (a Kea configuration that drifted from its own
// input is reported by its descriptor as a *structpb.Struct and contributes nothing, so the drift is visible).
func AssembleKea(ds *vrxv1.DesiredState, kvs []scheduler.KV) {
	for _, kv := range kvs {
		d := kv.Key.Descriptor()
		if d != kea.NameDhcp4 && d != kea.NameDhcp6 {
			continue
		}
		in, ok := kv.Value.(*vrxv1.DesiredState)
		if !ok {
			continue
		}
		for name, srv := range in.GetServices().GetDhcp().GetServers() {
			dhcpOf(ds).Servers[name] = proto.Clone(srv).(*vrxv1.DhcpServer)
		}
	}
}

// dhcpOf returns ds.services.dhcp with both maps allocated.
func dhcpOf(ds *vrxv1.DesiredState) *vrxv1.DhcpService {
	if ds.Services == nil {
		ds.Services = &vrxv1.ServicesConfig{}
	}
	if ds.Services.Dhcp == nil {
		ds.Services.Dhcp = &vrxv1.DhcpService{}
	}
	if ds.Services.Dhcp.Servers == nil {
		ds.Services.Dhcp.Servers = map[string]*vrxv1.DhcpServer{}
	}
	if ds.Services.Dhcp.Relays == nil {
		ds.Services.Dhcp.Relays = map[string]*vrxv1.DhcpRelay{}
	}
	return ds.Services.Dhcp
}
