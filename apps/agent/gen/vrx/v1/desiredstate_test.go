// Compile-check for the generated contract (task P03). Hand-written: gen.sh deletes only *.pb.go.
//
// It proves the contract's central property: DesiredState is a 1:1 protobuf projection of the
// configuration document, so the protobuf JSON mapping of a parsed packages/schema example *is* a
// DesiredState — strict protojson (unknown fields are errors) must accept every valid example.
package vrxv1_test

import (
	"io/fs"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// examplesDir is packages/schema/examples relative to this package (apps/agent/gen/vrx/v1).
const examplesDir = "../../../../../packages/schema/examples"

// rootKeys mirrors ROOT_KEYS in packages/schema/src/index.ts (documented order, docs/04).
var rootKeys = []string{
	"system", "dataplane", "interfaces", "vrfs", "routing", "nat", "objects",
	"acl", "vpn", "tunnels", "services", "ha", "management",
}

func validExamples(t *testing.T) map[string][]byte {
	t.Helper()
	fsys := os.DirFS(examplesDir) // rooted FS: file names cannot escape the fixtures directory
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		t.Fatalf("read %s: %v", examplesDir, err)
	}
	out := map[string][]byte{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, "invalid-") {
			continue
		}
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		out[name] = b
	}
	for _, want := range []string{"minimal.json", "two-interfaces.json"} {
		if _, ok := out[want]; !ok {
			t.Fatalf("expected example %s in %s", want, examplesDir)
		}
	}
	return out
}

// TestDesiredStateMirrorsRootKeys pins field numbers 1..13 to the documented root keys.
func TestDesiredStateMirrorsRootKeys(t *testing.T) {
	fields := (&vrxv1.DesiredState{}).ProtoReflect().Descriptor().Fields()
	if fields.Len() != len(rootKeys) {
		t.Fatalf("DesiredState has %d fields, want %d root keys", fields.Len(), len(rootKeys))
	}
	for i, key := range rootKeys {
		f := fields.Get(i)
		if f.JSONName() != key || int(f.Number()) != i+1 {
			t.Errorf("field %d: json %q number %d, want %q / %d", i, f.JSONName(), f.Number(), key, i+1)
		}
	}
}

// TestDesiredStateFromSchemaExamples: every valid example parses with strict protojson and survives
// binary and JSON round trips unchanged.
func TestDesiredStateFromSchemaExamples(t *testing.T) {
	for name, doc := range validExamples(t) {
		t.Run(name, func(t *testing.T) {
			var ds vrxv1.DesiredState
			if err := protojson.Unmarshal(doc, &ds); err != nil { // strict: no DiscardUnknown
				t.Fatalf("protojson.Unmarshal: %v", err)
			}
			wire, err := proto.Marshal(&ds)
			if err != nil {
				t.Fatalf("proto.Marshal: %v", err)
			}
			var back vrxv1.DesiredState
			if err := proto.Unmarshal(wire, &back); err != nil {
				t.Fatalf("proto.Unmarshal: %v", err)
			}
			if !proto.Equal(&ds, &back) {
				t.Fatalf("binary round trip differs:\n%v\n%v", &ds, &back)
			}
			js, err := protojson.Marshal(&ds)
			if err != nil {
				t.Fatalf("protojson.Marshal: %v", err)
			}
			var again vrxv1.DesiredState
			if err := protojson.Unmarshal(js, &again); err != nil {
				t.Fatalf("protojson re-Unmarshal: %v", err)
			}
			if !proto.Equal(&ds, &again) {
				t.Fatalf("JSON round trip differs:\n%s", js)
			}
		})
	}
}

// TestTwoInterfacesExampleValues checks that the example's values land in the typed fields.
func TestTwoInterfacesExampleValues(t *testing.T) {
	var ds vrxv1.DesiredState
	if err := protojson.Unmarshal(validExamples(t)["two-interfaces.json"], &ds); err != nil {
		t.Fatal(err)
	}
	if got := ds.GetSystem().GetHostname(); got != "vrx-a" {
		t.Errorf("system.hostname = %q", got)
	}
	if got := ds.GetSystem().GetTimezone(); got != "UTC" {
		t.Errorf("system.timezone = %q", got)
	}
	if len(ds.GetInterfaces()) != 2 {
		t.Fatalf("interfaces: %d entries", len(ds.GetInterfaces()))
	}
	up := ds.GetInterfaces()["TenGigabitEthernet0/0/0"]
	if up == nil {
		t.Fatal("uplink missing")
	}
	if !up.GetEnabled() || up.GetDescription() != "uplink" || up.GetMtu() != 9000 || up.GetVrf() != "default" || up.GetRxMode() != "polling" {
		t.Errorf("uplink fields: %v", up)
	}
	if up.Mtu == nil || up.Mac != nil {
		t.Errorf("presence: mtu set=%v mac set=%v (want true/false)", up.Mtu != nil, up.Mac != nil)
	}
	if len(up.GetIpv4()) != 1 || up.GetIpv4()[0] != "10.0.0.1/24" || len(up.GetIpv6()) != 1 || up.GetIpv6()[0] != "2001:db8::1/64" {
		t.Errorf("uplink addresses: %v %v", up.GetIpv4(), up.GetIpv6())
	}
	access := ds.GetInterfaces()["TenGigabitEthernet0/0/1"]
	sub := access.GetSubinterfaces()["100"]
	if sub == nil || sub.GetVlanId() != 100 || sub.GetVrf() != "customer-a" || len(sub.GetIpv4()) != 1 || sub.GetIpv4()[0] != "192.168.100.1/24" {
		t.Errorf("subinterface 100: %v", sub)
	}
	if sub.InnerVlanId != nil {
		t.Errorf("inner_vlan_id should be unset")
	}
	if v := ds.GetVrfs()["customer-a"]; v == nil || v.GetId() != 10 || v.GetDescription() != "customer A" {
		t.Errorf("vrf customer-a: %v", v)
	}
	if v := ds.GetVrfs()["default"]; v == nil || v.GetId() != 0 {
		t.Errorf("vrf default: %v", v)
	}
	routes := ds.GetRouting().GetStatic()
	if len(routes) != 1 || routes[0].GetPrefix() != "0.0.0.0/0" || routes[0].GetVrf() != "default" {
		t.Fatalf("static routes: %v", routes)
	}
	nh := routes[0].GetNextHops()
	if len(nh) != 1 || nh[0].GetAddress() != "10.0.0.254" || nh[0].GetWeight() != 1 || nh[0].Interface != nil {
		t.Errorf("next hops: %v", nh)
	}
	// Domains absent from the document are absent from the message (prefault happens in Zod, not here).
	if ds.Nat != nil || ds.Vpn != nil || len(ds.GetObjects().GetAddresses()) != 0 {
		t.Errorf("unexpected domains populated")
	}
}

// TestTypedConstruction is the compile check for hand-built desired state in every domain (the
// shape descriptor factories and the API mapper use). It also pins the flattened-union convention.
func TestTypedConstruction(t *testing.T) {
	mtu := uint32(1500)
	desc := "loopback for tests"
	ds := &vrxv1.DesiredState{
		System:    &vrxv1.SystemConfig{Hostname: "vrx-a", Timezone: "UTC", Ntp: &vrxv1.SystemNtp{}, Dns: &vrxv1.SystemDns{}},
		Dataplane: &vrxv1.DataplaneConfig{Workers: proto.Uint32(2), PciWhitelist: []string{"0000:0b:00.0"}},
		Interfaces: map[string]*vrxv1.Interface{
			"loop700": {Enabled: true, Description: &desc, Mtu: &mtu, Ipv4: []string{"10.7.0.1/24"}, Vrf: "default", RxMode: "polling",
				Subinterfaces: map[string]*vrxv1.Subinterface{"10": {VlanId: 10, Enabled: true, Vrf: "default"}}},
		},
		Vrfs:    map[string]*vrxv1.Vrf{"default": {Id: 0}, "w7-a": {Id: 7001, Description: proto.String("slot 7")}},
		Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{{Prefix: "10.70.0.0/16", Vrf: "w7-a", NextHops: []*vrxv1.NextHop{{Address: "10.7.0.254", Weight: 1}}}}, Bgp: &vrxv1.BgpConfig{}},
		Nat: &vrxv1.NatConfig{Enabled: true, Mode: "ed", Inside: []string{"loop700"}, Pools: []*vrxv1.NatPool{{Name: "p1", Range: "10.7.1.1-10.7.1.10"}},
			StaticMappings: []*vrxv1.NatStaticMapping{{Name: "web", Protocol: proto.String("tcp"),
				Local: &vrxv1.NatStaticMapping_Local{Ip: "10.7.0.10", Port: proto.Uint32(80)}, External: &vrxv1.NatStaticMapping_External{Ip: proto.String("10.7.1.1"), Port: proto.Uint32(8080)}}}},
		Objects: &vrxv1.ObjectsConfig{Addresses: map[string]*vrxv1.AddressObject{"h1": {Type: "host", Address: proto.String("10.7.0.10")}},
			Services: map[string]*vrxv1.ServiceObject{"https": {Protocol: "tcp", DestinationPorts: []string{"443"}}}},
		Acl: &vrxv1.AclConfig{Lists: map[string]*vrxv1.AclList{"in": {Rules: []*vrxv1.AclRule{{Sequence: 10, Enabled: true, Action: "permit", IpVersion: "any",
			Source: &vrxv1.AddressMatch{Kind: "object", Name: proto.String("h1")}, Service: &vrxv1.ServiceMatch{Kind: "object", Name: proto.String("https")}}}}},
			Attachments: []*vrxv1.AclAttachment{{List: "in", Target: &vrxv1.AttachmentTarget{Kind: "interface", Interface: proto.String("loop700")}, Direction: "in", Sequence: 1, Enabled: true}}},
		Vpn: &vrxv1.VpnConfig{Ipsec: &vrxv1.IpsecConfig{Tunnels: map[string]*vrxv1.IpsecTunnel{"site-b": {Enabled: true, Engine: "strongswan", IkeVersion: 2, Mode: "tunnel", Protocol: "esp",
			LocalAddr: "10.7.0.1", RemoteAddr: "192.0.2.1", Auth: &vrxv1.IpsecAuth{Method: "psk", SecretRef: proto.String("psk/site-b")}, Proposal: "default", Vrf: "default"}}}},
		Tunnels:    &vrxv1.TunnelsConfig{Gre: map[string]*vrxv1.GreTunnel{"gre0": {}}},
		Services:   &vrxv1.ServicesConfig{Dhcp: &vrxv1.DhcpService{}},
		Ha:         &vrxv1.HaConfig{Vrrp: []*vrxv1.VrrpInstance{{}}},
		Management: &vrxv1.ManagementConfig{Users: []*vrxv1.ManagementUser{{Username: "admin", Role: "admin", Scope: "*"}}},
	}
	// Every root key is populated in this literal.
	m := ds.ProtoReflect()
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		if !m.Has(fields.Get(i)) {
			t.Errorf("literal leaves %s unset", fields.Get(i).JSONName())
		}
	}
	// The JSON mapping uses the Zod (lowerCamelCase) names.
	js, err := protojson.Marshal(ds)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"rxMode"`, `"vlanId"`, `"nextHops"`, `"staticMappings"`, `"pciWhitelist"`, `"ikeVersion"`, `"secretRef"`, `"ipVersion"`} {
		if !strings.Contains(string(js), want) {
			t.Errorf("JSON lacks %s: %s", want, js)
		}
	}
	// Envelope messages compile and carry the documented shapes.
	req := &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: ds, Subsystems: []string{"interfaces", "vrfs"}, ConfirmTimeoutSec: 120, Owner: "w7"}
	if req.GetDesiredState().GetInterfaces()["loop700"].GetMtu() != 1500 {
		t.Fatal("apply request does not carry the state")
	}
	act := &vrxv1.ActionRequest{Action: &vrxv1.ActionRequest_Ping{Ping: &vrxv1.PingAction{Target: "10.7.0.254", Count: 3}}}
	if act.GetPing().GetCount() != 3 {
		t.Fatal("oneof accessor")
	}
	out := &vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Done{Done: &vrxv1.ActionDone{Summary: "ok"}}}
	if out.GetDone().GetSummary() != "ok" {
		t.Fatal("done accessor")
	}
}
