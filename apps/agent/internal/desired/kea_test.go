package desired

import (
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dhcp"
	"ngfw/agent/internal/renderers/kea"
	"ngfw/agent/internal/scheduler"
)

type sink struct {
	kvs      []scheduler.KV
	pointers map[scheduler.Key]string
	errs     []string
	warns    []string
}

func newSink() *sink { return &sink{pointers: map[scheduler.Key]string{}} }

func (s *sink) Add(k scheduler.Key, v proto.Message, pointer string) {
	s.kvs = append(s.kvs, scheduler.KV{Key: k, Value: v})
	s.pointers[k] = pointer
}
func (s *sink) Errorf(pointer, rule, format string, a ...any) {
	s.errs = append(s.errs, pointer+" "+rule+": "+fmt.Sprintf(format, a...))
}
func (s *sink) Warnf(pointer, rule, _ string, _ ...any) { s.warns = append(s.warns, pointer+" "+rule) }

var vrfIDs = map[string]uint32{"default": 0, "w2-lan": 2001, "w2-wan": 2002}

func vrfID(n string) (uint32, bool) { id, ok := vrfIDs[n]; return id, ok }

func lanDoc() *vrxv1.DesiredState {
	return &vrxv1.DesiredState{
		Interfaces: map[string]*vrxv1.Interface{
			"host-w2l0": {Vrf: proto.String("w2-lan"), Ipv4: []string{"10.2.1.1/24"}},
			"host-w2w0": {Vrf: proto.String("w2-lan"), Ipv4: []string{"10.2.2.1/24"}},
		},
		Services: &vrxv1.ServicesConfig{
			Dhcp: &vrxv1.DhcpService{
				Servers: map[string]*vrxv1.DhcpServer{
					"lan": {Enabled: proto.Bool(true), Family: proto.String("ipv4"), Vrf: proto.String("w2-lan"), Interfaces: []string{"host-w2l0"},
						LeaseTimeSec: proto.Uint32(3600), Authoritative: proto.Bool(true),
						Subnets: map[string]*vrxv1.DhcpSubnet{"lan": {Subnet: proto.String("10.2.1.0/24"),
							Pools: []*vrxv1.DhcpPool{{Start: proto.String("10.2.1.100"), End: proto.String("10.2.1.150")}}}}},
					"lan6": {Family: proto.String("ipv6"), Vrf: proto.String("w2-lan"), Interfaces: []string{"host-w2l0"}},
				},
				Relays: map[string]*vrxv1.DhcpRelay{
					"to-kea": {Enabled: proto.Bool(true), Family: proto.String("ipv4"), Vrf: proto.String("w2-lan"), Interfaces: []string{"host-w2l0"},
						Servers: []string{"10.2.2.2", "10.2.2.3"}, SourceAddress: proto.String("10.2.2.1"), Description: proto.String("relay")},
					"off": {Enabled: proto.Bool(false), Family: proto.String("ipv4"), Vrf: proto.String("w2-wan"), Servers: []string{"10.9.9.9"}, SourceAddress: proto.String("10.9.9.1")},
				},
			},
			Snmp: &vrxv1.SnmpService{Enabled: proto.Bool(false)},
			Dns:  &vrxv1.DnsService{},
		},
	}
}

func keys(kvs []scheduler.KV) string {
	var out []string
	for _, kv := range kvs {
		out = append(out, string(kv.Key))
	}
	return strings.Join(out, " ")
}

// TestDHCPProjectionRoundTrip: builder → (the descriptors would retrieve exactly these values) → assembler returns the
// document's services.dhcp unchanged.
func TestDHCPProjectionRoundTrip(t *testing.T) {
	ds := lanDoc()
	s := newSink()
	DHCP(s, ds, vrfID)
	if len(s.errs) != 0 {
		t.Fatalf("errors %v", s.errs)
	}
	want := "kea.dhcp4/vrx kea.dhcp6/vrx dhcp.relay/off dhcp.proxy/2001/2001/10.2.2.2 dhcp.proxy/2001/2001/10.2.2.3 dhcp.relay/to-kea"
	if got := keys(s.kvs); got != want {
		t.Fatalf("keys\n got %s\nwant %s", got, want)
	}
	if strings.Join(s.warns, ",") != "/services/snmp agent.unsupported-field" {
		t.Fatalf("warnings %v (an empty dns is no warning)", s.warns)
	}
	if s.pointers["dhcp.proxy/2001/2001/10.2.2.3"] != "/services/dhcp/relays/to-kea/servers/1" {
		t.Fatalf("pointer %s", s.pointers["dhcp.proxy/2001/2001/10.2.2.3"])
	}
	var p dhcp.Proxy
	if err := dfkit.Decode(s.kvs[3].Value, &p); err != nil || p.Src != "10.2.2.1" || p.RxVRF != 2001 || p.ServerVRF != 2001 {
		t.Fatalf("proxy %+v %v", p, err)
	}
	if !proto.Equal(s.kvs[0].Value, kea.Input(ds, 4)) {
		t.Fatal("kea.dhcp4 value is not kea.Input")
	}

	out := &vrxv1.DesiredState{}
	AssembleDHCP(out, s.kvs)
	if !proto.Equal(out.GetServices().GetDhcp(), ds.GetServices().GetDhcp()) {
		t.Fatalf("round trip:\n got %v\nwant %v", out.GetServices().GetDhcp(), ds.GetServices().GetDhcp())
	}

	// VPP lost one relay server: the record reports the servers VPP has → the assembled relay shows it
	for i, kv := range s.kvs {
		if kv.Key == "dhcp.relay/to-kea" {
			var rec dhcp.Relay
			_ = dfkit.Decode(kv.Value, &rec)
			rec.Servers = []string{"10.2.2.2"}
			s.kvs[i].Value = rec.Proto()
		}
	}
	out = &vrxv1.DesiredState{}
	AssembleDHCP(out, s.kvs)
	if got := out.GetServices().GetDhcp().GetRelays()["to-kea"].GetServers(); len(got) != 1 {
		t.Fatalf("drifted relay servers %v", got)
	}

	// an empty services domain still assembles services.dhcp (empty maps)
	out = &vrxv1.DesiredState{}
	AssembleDHCP(out, nil)
	if out.GetServices().GetDhcp() == nil {
		t.Fatal("services.dhcp must be present")
	}
}

func TestDHCPProjectionErrors(t *testing.T) {
	ds := lanDoc()
	d := ds.Services.Dhcp
	d.Servers["other"] = &vrxv1.DhcpServer{Family: proto.String("ipv4"), Vrf: proto.String("w2-wan"), Interfaces: []string{"host-w2w0"}}
	d.Servers["bad"] = &vrxv1.DhcpServer{Family: proto.String("ipx")}
	d.Relays["second-src"] = &vrxv1.DhcpRelay{Family: proto.String("ipv4"), Vrf: proto.String("w2-lan"), Servers: []string{"10.2.2.9"}, SourceAddress: proto.String("10.2.1.1")}
	d.Relays["novrf"] = &vrxv1.DhcpRelay{Family: proto.String("ipv4"), Vrf: proto.String("nope"), Servers: []string{"10.2.2.9"}, SourceAddress: proto.String("10.2.1.1")}
	d.Relays["fam"] = &vrxv1.DhcpRelay{Family: proto.String("ipv6"), Vrf: proto.String("w2-wan"), Servers: []string{"10.2.2.9"}, SourceAddress: proto.String("fd00::1")}
	s := newSink()
	DHCP(s, ds, vrfID)
	got := strings.Join(s.errs, "\n")
	for _, want := range []string{
		"/services/dhcp/servers/bad/family services.dhcp-family",
		"/services/dhcp/relays/fam/servers/0 services.dhcp-relay-address",
		"/services/dhcp/relays/novrf/vrf services.vrf-exists",
		"/services/dhcp/relays/to-kea/sourceAddress services.kea-dhcp-relay-relay-source-per-vrf",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	// one VRF per family is the renderer's check at apply time (and the semantic rule), not a projection error
	ds = lanDoc()
	ds.Services.Dhcp.Servers["other"] = &vrxv1.DhcpServer{Family: proto.String("ipv4"), Vrf: proto.String("w2-wan"), Interfaces: []string{"host-w2w0"}}
	s = newSink()
	Kea(s, ds)
	if len(s.errs) != 0 || len(s.kvs) != 2 {
		t.Fatalf("two v4 VRFs: %v %v", s.errs, keys(s.kvs))
	}
}

func TestServicesUnsupported(t *testing.T) {
	s := newSink()
	ServicesUnsupported(s, nil)
	ServicesUnsupported(s, &vrxv1.ServicesConfig{Dhcp: &vrxv1.DhcpService{}, Ntp: &vrxv1.NtpService{Enabled: proto.Bool(true)}})
	if strings.Join(s.warns, ",") != "/services/ntp agent.unsupported-field" {
		t.Fatalf("warnings %v", s.warns)
	}
}
