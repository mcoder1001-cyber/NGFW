package contracttest

import (
	"encoding/json"
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// Document corpora, relative to this package (apps/agent/internal/contracttest).
const (
	// The schema package's example documents (valid RootConfig documents; `invalid-*` are negative).
	examplesDir = "../../../../packages/schema/examples"
	// Proto-local documents exercising the domains the schema examples do not touch yet.
	fixturesDir = "../../../../packages/proto/test/fixtures"
)

// rootKeys mirrors ROOT_KEYS in packages/schema/src/index.ts (documented order, docs/04).
var rootKeys = []string{
	"system", "dataplane", "interfaces", "vrfs", "routing", "nat", "objects",
	"acl", "vpn", "tunnels", "services", "ha", "management",
}

// strict is the decoder the agent must use for documents coming from the API: unknown fields are
// errors (DiscardUnknown=false is the default; spelled out because it is the point of the test).
var strict = protojson.UnmarshalOptions{DiscardUnknown: false}

func readJSONDir(t *testing.T, dir, prefix string) map[string][]byte {
	t.Helper()
	fsys := os.DirFS(dir) // rooted FS: file names cannot escape the fixtures directory
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
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
		out[prefix+name] = b
	}
	return out
}

// corpus is every valid document: all schema examples plus the proto-local fixtures.
func corpus(t *testing.T) map[string][]byte {
	t.Helper()
	docs := readJSONDir(t, examplesDir, "examples/")
	for k, v := range readJSONDir(t, fixturesDir, "fixtures/") {
		docs[k] = v
	}
	for _, want := range []string{"examples/minimal.json", "examples/two-interfaces.json", "fixtures/all-domains.json"} {
		if _, ok := docs[want]; !ok {
			t.Fatalf("expected document %s in the corpus", want)
		}
	}
	return docs
}

func sortedKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

// TestStrictDecodeOfEveryDocument: every document of the corpus decodes with strict protojson,
// survives binary and JSON round trips unchanged, and its JSON re-encoding has exactly the key set
// of the source document (explicit presence, D-039: nothing is dropped, nothing is invented).
func TestStrictDecodeOfEveryDocument(t *testing.T) {
	docs := corpus(t)
	for _, name := range sortedKeys(docs) {
		doc := docs[name]
		t.Run(name, func(t *testing.T) {
			var ds vrxv1.DesiredState
			if err := strict.Unmarshal(doc, &ds); err != nil {
				t.Fatalf("strict protojson.Unmarshal: %v", err)
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
			if err := strict.Unmarshal(js, &again); err != nil {
				t.Fatalf("protojson re-Unmarshal: %v", err)
			}
			if !proto.Equal(&ds, &again) {
				t.Fatalf("JSON round trip differs:\n%s", js)
			}
			// Key-set fidelity: the projection neither drops nor invents keys.
			var want, got any
			if err := json.Unmarshal(doc, &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(js, &got); err != nil {
				t.Fatal(err)
			}
			wantPaths, gotPaths := keyPaths(want, ""), keyPaths(got, "")
			if strings.Join(wantPaths, "\n") != strings.Join(gotPaths, "\n") {
				t.Fatalf("key paths differ\n document: %v\n re-encoded: %v", wantPaths, gotPaths)
			}
		})
	}
}

// keyPaths lists every JSON pointer-ish path of a decoded JSON value (objects and arrays), sorted.
func keyPaths(v any, prefix string) []string {
	var out []string
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			p := prefix + "/" + k
			out = append(out, p)
			out = append(out, keyPaths(child, p)...)
		}
	case []any:
		for i, child := range x {
			p := prefix + "/" + strings.Repeat("-", 0) + itoa(i)
			out = append(out, p)
			out = append(out, keyPaths(child, p)...)
		}
	}
	sort.Strings(out)
	return out
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

// TestStrictDecodeRejectsUnknownFields: the negative side of strictness — unknown root keys and
// unknown nested keys are errors, so a schema field without a proto field cannot slip through.
func TestStrictDecodeRejectsUnknownFields(t *testing.T) {
	fsys := os.DirFS(examplesDir)
	bad, err := fs.ReadFile(fsys, "invalid-unknown-root-key.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"invalid-unknown-root-key.json": bad,
		"nested unknown leaf":           []byte(`{"system":{"hostname":"vrx-a","bogusField":1}}`),
		"removed password_hash (D-040)": []byte(`{"management":{"users":[{"username":"a","role":"admin","scope":"*","passwordHash":"$6$x"}]}}`),
		"old corelist string (F2)":      []byte(`{"dataplane":{"corelist":"2-5,8"}}`),
		"old banner string (F2)":        []byte(`{"system":{"banner":"hello"}}`),
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			var ds vrxv1.DesiredState
			if err := strict.Unmarshal(doc, &ds); err == nil {
				t.Fatalf("strict decode accepted %s", doc)
			}
		})
	}
}

// TestExplicitPresenceSurvivesJSON is the F3 probe from the review: proto3 default values that the
// document sets explicitly (`id: 0`, `enabled: false`) must come back out of protojson, and fields
// the document does not set must stay absent (no re-filled defaults, no false drift).
func TestExplicitPresenceSurvivesJSON(t *testing.T) {
	doc := []byte(`{"vrfs":{"default":{"id":0}},` +
		`"interfaces":{"x":{"enabled":false,"vrf":"default","rxMode":"polling"}},` +
		`"acl":{"lists":{"l":{"rules":[{"sequence":1,"enabled":false,"action":"permit","ipVersion":"any"}]}}}}`)
	var ds vrxv1.DesiredState
	if err := strict.Unmarshal(doc, &ds); err != nil {
		t.Fatal(err)
	}
	vrf := ds.GetVrfs()["default"]
	if vrf.Id == nil || vrf.GetId() != 0 {
		t.Fatalf("vrfs.default.id lost its presence: %v", vrf)
	}
	iface := ds.GetInterfaces()["x"]
	if iface.Enabled == nil || iface.GetEnabled() || iface.Mtu != nil || iface.Promiscuous != nil {
		t.Fatalf("interface presence: enabled=%v mtu=%v promiscuous=%v", iface.Enabled, iface.Mtu, iface.Promiscuous)
	}
	js, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(&ds)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"id":0`, `"enabled":false`} {
		if !strings.Contains(compactJSON(t, js), want) {
			t.Errorf("re-encoded JSON lacks %s: %s", want, js)
		}
	}
	if strings.Count(compactJSON(t, js), `"enabled":false`) != 2 {
		t.Errorf("both explicit enabled:false must be emitted: %s", js)
	}
	for _, absent := range []string{`"mtu"`, `"promiscuous"`, `"log"`, `"description"`} {
		if strings.Contains(string(js), absent) {
			t.Errorf("re-encoded JSON invents %s: %s", absent, js)
		}
	}
}

func compactJSON(t *testing.T, b []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// TestNoSecretLeaves enforces D-040 structurally: no field of the DesiredState subtree carries
// authentication material; secrets are referenced through `*_ref` names only.
func TestNoSecretLeaves(t *testing.T) {
	forbidden := []string{"password_hash", "password", "secret", "private_key", "preshared_key", "psk", "pin", "key"}
	seen := map[protoreflect.FullName]bool{}
	var walk func(md protoreflect.MessageDescriptor, path string)
	walk = func(md protoreflect.MessageDescriptor, path string) {
		if seen[md.FullName()] {
			return
		}
		seen[md.FullName()] = true
		fields := md.Fields()
		for i := 0; i < fields.Len(); i++ {
			f := fields.Get(i)
			name := string(f.Name())
			p := path + "." + name
			if f.Kind() != protoreflect.MessageKind || f.IsMap() {
				for _, bad := range forbidden {
					if name == bad {
						t.Errorf("%s: field named %q is authentication material — only *_ref may cross the boundary", p, name)
					}
				}
			}
			if f.IsMap() {
				if v := f.MapValue(); v.Kind() == protoreflect.MessageKind {
					walk(v.Message(), p+"[]")
				}
				continue
			}
			if f.Kind() == protoreflect.MessageKind {
				walk(f.Message(), p)
			}
		}
	}
	walk((&vrxv1.DesiredState{}).ProtoReflect().Descriptor(), "DesiredState")
	if _, has := seen["vrx.v1.ManagementUser"]; !has {
		t.Fatal("walk did not reach ManagementUser")
	}
	if f := (&vrxv1.ManagementUser{}).ProtoReflect().Descriptor().Fields().ByNumber(4); f != nil {
		t.Errorf("ManagementUser field 4 must stay reserved (was password_hash), found %s", f.Name())
	}
}

// TestExplicitPresenceEverywhere enforces D-039 structurally: every scalar (non-message,
// non-repeated, non-map) field reachable from DesiredState has explicit presence.
func TestExplicitPresenceEverywhere(t *testing.T) {
	seen := map[protoreflect.FullName]bool{}
	var walk func(md protoreflect.MessageDescriptor, path string)
	walk = func(md protoreflect.MessageDescriptor, path string) {
		if seen[md.FullName()] {
			return
		}
		seen[md.FullName()] = true
		fields := md.Fields()
		for i := 0; i < fields.Len(); i++ {
			f := fields.Get(i)
			p := path + "." + string(f.Name())
			switch {
			case f.IsMap():
				if v := f.MapValue(); v.Kind() == protoreflect.MessageKind {
					walk(v.Message(), p+"[]")
				}
			case f.IsList():
				if f.Kind() == protoreflect.MessageKind {
					walk(f.Message(), p+"[]")
				}
			case f.Kind() == protoreflect.MessageKind:
				walk(f.Message(), p)
			default:
				if !f.HasPresence() {
					t.Errorf("%s: scalar without explicit presence (mark it `optional`, D-039)", p)
				}
			}
		}
	}
	walk((&vrxv1.DesiredState{}).ProtoReflect().Descriptor(), "DesiredState")
	if len(seen) < 100 {
		t.Fatalf("walk covered only %d messages", len(seen))
	}
}

// TestTwoInterfacesExampleValues checks that the example's values land in the typed fields.
func TestTwoInterfacesExampleValues(t *testing.T) {
	var ds vrxv1.DesiredState
	if err := strict.Unmarshal(corpus(t)["examples/two-interfaces.json"], &ds); err != nil {
		t.Fatal(err)
	}
	if got := ds.GetSystem().GetHostname(); got != "vrx-a" {
		t.Errorf("system.hostname = %q", got)
	}
	if got := ds.GetSystem().GetTimezone(); got != "UTC" {
		t.Errorf("system.timezone = %q", got)
	}
	if ds.GetSystem().Banner != nil {
		t.Errorf("system.banner should be unset")
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
	if up.Mtu == nil || up.Mac != nil || up.RxMode == nil || up.Promiscuous != nil {
		t.Errorf("presence: mtu=%v mac=%v rxMode=%v promiscuous=%v (want set/unset/set/unset)", up.Mtu != nil, up.Mac != nil, up.RxMode != nil, up.Promiscuous != nil)
	}
	if len(up.GetIpv4()) != 1 || up.GetIpv4()[0] != "10.0.0.1/24" || len(up.GetIpv6()) != 1 || up.GetIpv6()[0] != "2001:db8::1/64" {
		t.Errorf("uplink addresses: %v %v", up.GetIpv4(), up.GetIpv6())
	}
	access := ds.GetInterfaces()["TenGigabitEthernet0/0/1"]
	sub := access.GetSubinterfaces()["100"]
	if sub == nil || sub.GetVlanId() != 100 || sub.GetVrf() != "customer-a" || len(sub.GetIpv4()) != 1 || sub.GetIpv4()[0] != "192.168.100.1/24" {
		t.Errorf("subinterface 100: %v", sub)
	}
	if sub.InnerVlanId != nil || sub.Dot1Ad != nil || sub.Enabled != nil {
		t.Errorf("inner_vlan_id/dot1ad/enabled should be unset (not in the document)")
	}
	if v := ds.GetVrfs()["customer-a"]; v == nil || v.GetId() != 10 || v.GetDescription() != "customer A" {
		t.Errorf("vrf customer-a: %v", v)
	}
	if v := ds.GetVrfs()["default"]; v == nil || v.Id == nil || v.GetId() != 0 {
		t.Errorf("vrf default (id 0 must be present): %v", v)
	}
	routes := ds.GetRouting().GetStatic()
	if len(routes) != 1 || routes[0].GetPrefix() != "0.0.0.0/0" || routes[0].GetVrf() != "default" || routes[0].Distance != nil {
		t.Fatalf("static routes: %v", routes)
	}
	nh := routes[0].GetNextHops()
	if len(nh) != 1 || nh[0].GetAddress() != "10.0.0.254" || nh[0].Address == nil || nh[0].GetWeight() != 1 || nh[0].Interface != nil {
		t.Errorf("next hops: %v", nh)
	}
	// Domains absent from the document are absent from the message (prefault happens in Zod, not here).
	if ds.Nat != nil || ds.Vpn != nil || len(ds.GetObjects().GetAddresses()) != 0 {
		t.Errorf("unexpected domains populated")
	}
}

// TestAllDomainsFixtureValues spot-checks the P02a shapes fixed by the review (D-042) in the
// proto-local fixture: banner object, corelist array, optional rx_mode, interface-only next hop,
// user without password material.
func TestAllDomainsFixtureValues(t *testing.T) {
	var ds vrxv1.DesiredState
	if err := strict.Unmarshal(corpus(t)["fixtures/all-domains.json"], &ds); err != nil {
		t.Fatal(err)
	}
	if b := ds.GetSystem().GetBanner(); b.GetLogin() == "" || b.Motd != nil {
		t.Errorf("banner: %v", b)
	}
	if cl := ds.GetDataplane().GetCorelist(); len(cl) != 4 || cl[0] != 2 || cl[3] != 8 {
		t.Errorf("corelist: %v", cl)
	}
	if ds.GetDataplane().GetTxQueues() != 2 || ds.GetDataplane().GetWorkers() != 4 {
		t.Errorf("dataplane: %v", ds.GetDataplane())
	}
	loop := ds.GetInterfaces()["loop0"]
	if loop == nil || loop.RxMode != nil || !loop.GetPromiscuous() {
		t.Errorf("loop0: %v", loop)
	}
	sub := ds.GetInterfaces()["TenGigabitEthernet0/0/1"].GetSubinterfaces()["200"]
	if !sub.GetDot1Ad() || sub.GetInnerVlanId() != 20 {
		t.Errorf("qinq sub: %v", sub)
	}
	var ifaceRoute *vrxv1.StaticRoute
	for _, r := range ds.GetRouting().GetStatic() {
		if r.GetPrefix() == "192.0.2.0/24" {
			ifaceRoute = r
		}
	}
	if ifaceRoute == nil || len(ifaceRoute.GetNextHops()) != 1 || ifaceRoute.GetNextHops()[0].Address != nil || ifaceRoute.GetNextHops()[0].GetInterface() != "loop0" || ifaceRoute.GetDistance() != 5 {
		t.Errorf("interface route: %v", ifaceRoute)
	}
	users := ds.GetManagement().GetUsers()
	if len(users) != 2 || users[0].GetUsername() != "admin" || len(users[0].GetSshKeys()) != 1 || users[1].GetDisabled() != true || users[1].GetFullName() == "" {
		t.Errorf("users: %v", users)
	}
	if ds.GetNat().GetStaticMappings()[0].GetLocal().GetPort() != 8080 || ds.GetNat().GetMap().GetDomains()[0].GetRules()[0].GetPsid() != 1 {
		t.Errorf("nat: %v", ds.GetNat())
	}
	if ds.GetVpn().GetIpsec().GetTunnels()["site-b"].GetRekey().GetEspBytes() != 1073741824 {
		t.Errorf("64-bit esp_bytes: %v", ds.GetVpn().GetIpsec().GetTunnels()["site-b"].GetRekey())
	}
	if len(ds.GetAcl().GetLists()["edge-in"].GetRules()) != 2 || ds.GetObjects().GetAddresses()["r1"].GetType() != "range" {
		t.Errorf("acl/objects: %v %v", ds.GetAcl(), ds.GetObjects())
	}
}

// TestTypedConstruction is the compile check for hand-built desired state in every domain (the
// shape descriptor factories and the API mapper use). Explicit presence means scalars are pointers
// in Go (proto.String/Bool/Uint32 helpers); accessors (Get*) hide that for readers.
func TestTypedConstruction(t *testing.T) {
	ds := &vrxv1.DesiredState{
		System: &vrxv1.SystemConfig{Hostname: proto.String("vrx-a"), Timezone: proto.String("UTC"),
			Banner: &vrxv1.SystemBanner{Login: proto.String("authorised access only")}, Dns: &vrxv1.SystemDns{}},
		Dataplane: &vrxv1.DataplaneConfig{Workers: proto.Uint32(2), Corelist: []uint32{2, 3}, PciWhitelist: []string{"0000:0b:00.0"}},
		Interfaces: map[string]*vrxv1.Interface{
			"loop700": {Enabled: proto.Bool(true), Description: proto.String("loopback for tests"), Mtu: proto.Uint32(1500), Ipv4: []string{"10.7.0.1/24"},
				Vrf: proto.String("default"), RxMode: proto.String("polling"),
				Subinterfaces: map[string]*vrxv1.Subinterface{"10": {VlanId: proto.Uint32(10), Enabled: proto.Bool(true), Vrf: proto.String("default")}}},
		},
		Vrfs: map[string]*vrxv1.Vrf{"default": {Id: proto.Uint32(0)}, "w7-a": {Id: proto.Uint32(7001), Description: proto.String("slot 7")}},
		Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{{Prefix: proto.String("10.70.0.0/16"), Vrf: proto.String("w7-a"),
			NextHops: []*vrxv1.NextHop{{Address: proto.String("10.7.0.254"), Weight: proto.Uint32(1)}}}}, Bgp: &vrxv1.BgpConfig{}},
		Nat: &vrxv1.NatConfig{Enabled: proto.Bool(true), Mode: proto.String("ed"), Inside: []string{"loop700"},
			Pools: []*vrxv1.NatPool{{Name: proto.String("p1"), Range: proto.String("10.7.1.1-10.7.1.10")}},
			StaticMappings: []*vrxv1.NatStaticMapping{{Name: proto.String("web"), Protocol: proto.String("tcp"),
				Local:    &vrxv1.NatStaticMapping_Local{Ip: proto.String("10.7.0.10"), Port: proto.Uint32(80)},
				External: &vrxv1.NatStaticMapping_External{Ip: proto.String("10.7.1.1"), Port: proto.Uint32(8080)}}}},
		Objects: &vrxv1.ObjectsConfig{Addresses: map[string]*vrxv1.AddressObject{"h1": {Type: proto.String("host"), Address: proto.String("10.7.0.10")}},
			Services: map[string]*vrxv1.ServiceObject{"https": {Protocol: proto.String("tcp"), DestinationPorts: []string{"443"}}}},
		Acl: &vrxv1.AclConfig{Lists: map[string]*vrxv1.AclList{"in": {Rules: []*vrxv1.AclRule{{Sequence: proto.Uint32(10), Enabled: proto.Bool(true),
			Action: proto.String("permit"), IpVersion: proto.String("any"),
			Source:  &vrxv1.AddressMatch{Kind: proto.String("object"), Name: proto.String("h1")},
			Service: &vrxv1.ServiceMatch{Kind: proto.String("object"), Name: proto.String("https")}}}}},
			Attachments: []*vrxv1.AclAttachment{{List: proto.String("in"), Target: &vrxv1.AttachmentTarget{Kind: proto.String("interface"), Interface: proto.String("loop700")},
				Direction: proto.String("in"), Sequence: proto.Uint32(1), Enabled: proto.Bool(true)}}},
		Vpn: &vrxv1.VpnConfig{Ipsec: &vrxv1.IpsecConfig{Tunnels: map[string]*vrxv1.IpsecTunnel{"site-b": {Enabled: proto.Bool(true), Engine: proto.String("strongswan"),
			IkeVersion: proto.Uint32(2), Mode: proto.String("tunnel"), Protocol: proto.String("esp"), LocalAddr: proto.String("10.7.0.1"), RemoteAddr: proto.String("192.0.2.1"),
			Auth:     &vrxv1.IpsecAuth{Method: proto.String("psk"), SecretRef: proto.String("psk/site-b")},
			Proposal: proto.String("default"), Vrf: proto.String("default"), Rekey: &vrxv1.IpsecRekey{EspBytes: proto.Uint64(1 << 40)}}}}},
		Tunnels:    &vrxv1.TunnelsConfig{Gre: map[string]*vrxv1.GreTunnel{"gre0": {}}},
		Services:   &vrxv1.ServicesConfig{Dhcp: &vrxv1.DhcpService{}},
		Ha:         &vrxv1.HaConfig{Vrrp: []*vrxv1.VrrpInstance{{}}},
		Management: &vrxv1.ManagementConfig{Users: []*vrxv1.ManagementUser{{Username: proto.String("admin"), Role: proto.String("admin"), Scope: proto.String("*"), SshKeys: []string{"ssh-ed25519 AAAAC3 test"}}}},
	}
	// Every root key is populated in this literal.
	m := ds.ProtoReflect()
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		if !m.Has(fields.Get(i)) {
			t.Errorf("literal leaves %s unset", fields.Get(i).JSONName())
		}
	}
	// The JSON mapping uses the Zod (lowerCamelCase) names; 64-bit values are JSON strings.
	js, err := protojson.Marshal(ds)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"rxMode"`, `"vlanId"`, `"nextHops"`, `"staticMappings"`, `"pciWhitelist"`, `"ikeVersion"`, `"secretRef"`, `"ipVersion"`, `"corelist"`, `"sshKeys"`, `"espBytes":"1099511627776"`} {
		if !strings.Contains(compactJSON(t, js), want) {
			t.Errorf("JSON lacks %s: %s", want, js)
		}
	}
	// Envelope messages compile and carry the documented shapes (renamed enums, F10).
	req := &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: ds, Subsystems: []string{"interfaces", "vrfs"}, ConfirmTimeoutSec: 120, Owner: "w7"}
	if req.GetDesiredState().GetInterfaces()["loop700"].GetMtu() != 1500 {
		t.Fatal("apply request does not carry the state")
	}
	res := &vrxv1.ObjectResult{Key: "interface/loop700", Op: vrxv1.ApplyOperation_APPLY_OPERATION_CREATE, Code: vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_OK}
	issue := &vrxv1.ValidationIssue{Pointer: "/interfaces/loop700/vrf", Severity: vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR, Rule: "interfaces.vrf-exists"}
	if res.GetOp() != vrxv1.ApplyOperation_APPLY_OPERATION_CREATE || issue.GetSeverity() != vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
		t.Fatal("enum accessors")
	}
	ev := &vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_LINK_UP, Interface: proto.String("loop700")}
	if ev.Interface == nil || (&vrxv1.Event{}).Interface != nil {
		t.Fatal("Event.interface presence")
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
