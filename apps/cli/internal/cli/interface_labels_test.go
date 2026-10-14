package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"ngfw/cli/internal/api"
)

// The first physical port has no VPP state, config or speed. It still reserves
// Ethernet1 so aliases in operational tables agree with the full web inventory.
const ethernetInventory = `{"retrievedAt":"now","countersAt":"now","items":[
 {"name":"wan","kind":"interface","physical":{"pci":"0000:02:00.0"},
  "state":{"type":"dpdk","vppName":"GigabitEthernet0/2/0","linkSpeedKbps":"0"},
  "hostInventory":{"pci":"0000:02:00.0","netdev":"ens224"},
  "config":{"enabled":true,"mtu":1500,"peer":"wan"},"counters":null},
 {"name":"ens192","kind":"interface","hostInventory":{"pci":"0000:01:00.0","netdev":"ens192"},
  "inventoryOnly":true,"state":null,"config":null,"counters":null},
 {"name":"wan.100","kind":"subinterface","parent":"wan",
  "state":{"type":"subinterface","vppName":"GigabitEthernet0/2/0.100"},
  "config":{"enabled":true,"vlanId":100},"counters":null},
 {"name":"native10","kind":"interface","state":{"type":"VMXNET3","vppName":"vmxnet3-10"},"config":{"enabled":true}},
 {"name":"native2","kind":"interface","state":{"type":"af_xdp","vppName":"vmxnet3-2"},"config":{"enabled":true}},
 {"name":"tap0","kind":"interface","state":{"type":"virtio","vppName":"tap0"},"config":{"enabled":true}},
 {"name":"loop0","kind":"interface","config":{"enabled":true}}
]}`

func inventoryResponse(f *fakeAPI, raw string) {
	f.override["GET /api/v1/state/interfaces"] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, raw)
	}
}

func TestInterfaceLabelsMatchFullPhysicalInventory(t *testing.T) {
	var inventory struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(ethernetInventory), &inventory); err != nil {
		t.Fatal(err)
	}
	for _, reverse := range []bool{false, true} {
		if reverse {
			slices.Reverse(inventory.Items)
		}
		raw, err := json.Marshal(inventory)
		if err != nil {
			t.Fatal(err)
		}
		labels := newInterfaceLabels(raw)
		for name, want := range map[string]string{
			"ens192": "Ethernet1", "wan": "Ethernet2", "native2": "Ethernet3", "native10": "Ethernet4",
			"ens224": "Ethernet2", "GigabitEthernet0/2/0": "Ethernet2", "wan.100": "Ethernet2.100",
			"GigabitEthernet0/2/0.100": "Ethernet2.100", "ens224.100.200": "Ethernet2.100.200",
			"tap0": "tap0", "loop0": "loop0", "absent": "absent", "wan..2": "wan..2", "wan.note": "wan.note",
		} {
			if got := labels.label(name); got != want {
				t.Errorf("reverse=%v label(%q)=%q, want %q", reverse, name, got, want)
			}
		}
		for label, want := range map[string]string{"Ethernet2": "wan", "Ethernet2.100.200": "wan.100.200", "tap0": "tap0", "wan": "wan"} {
			if got, ok := labels.resolve(label); !ok || got != want {
				t.Errorf("resolve(%q)=%q,%v, want %q,true", label, got, ok, want)
			}
		}
	}
}

func TestInterfaceLabelsNativeFallbackAndIdentityCollisions(t *testing.T) {
	labels := newInterfaceLabels(json.RawMessage(`{"items":[
 {"name":"Ethernet1","kind":"interface","state":{"type":"rdma","vppName":"native10"}},
 {"name":"native2","kind":"interface","state":{"type":"avf"}},
 {"name":"secondary","kind":"interface","state":{"type":"virtio","vppName":"native2"}}
]}`))
	if labels.label("native2") != "Ethernet1" || labels.label("Ethernet1") != "Ethernet2" {
		t.Fatalf("native numeric fallback ordering: %v", labels.labels)
	}
	if got, ok := labels.resolve("Ethernet1"); !ok || got != "native2" {
		t.Fatalf("display alias must precede colliding raw name: %q,%v", got, ok)
	}
	// A secondary VPP spelling never overwrites the actual logical name native2.
	if labels.label("native2") != "Ethernet1" {
		t.Fatal("secondary alias overwrote logical identity")
	}
	invalid := interfaceLabels{labels: map[string]string{"ens.1": "Ethernet1.1"}}
	if got := invalid.label("ens.1..2"); got != "ens.1..2" {
		t.Fatalf("invalid VLAN suffix was changed to %q", got)
	}
}

func TestHumanInterfacesUseAliasesButJSONKeepsRawIdentityAndShape(t *testing.T) {
	f := newFake(t)
	inventoryResponse(f, ethernetInventory)
	r := f.ngfw(t, nil, "", "show", "interfaces")
	if r.code != 0 {
		t.Fatal(r.stderr)
	}
	for _, want := range []string{"Ethernet2", "Ethernet2.100", "Ethernet3", "Ethernet4", "tap0", "loop0"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("table missing %q: %s", want, r.stdout)
		}
	}
	for _, absent := range []string{"Ethernet1", "Unknown", "ens192", "wan", "native"} {
		if strings.Contains(r.stdout, absent) {
			t.Errorf("table unexpectedly contains %q: %s", absent, r.stdout)
		}
	}
	for _, name := range []string{"Ethernet2", "wan"} {
		r = f.ngfw(t, nil, "", "show", "interfaces", name)
		if r.code != 0 || !strings.Contains(r.stdout, "Interface Ethernet2 (retrieved now)") || !strings.Contains(r.stdout, "peer wan") {
			t.Fatalf("detail %q must select wan without translating config text: %d %q %q", name, r.code, r.stdout, r.stderr)
		}
	}
	if r = f.ngfw(t, nil, "", "show", "interfaces", "Ethernet1"); r.code != ExitNotFound {
		t.Fatalf("hidden host alias became a retrieved interface: %d %s", r.code, r.stdout)
	}
	r = f.ngfw(t, nil, "", "--json", "show", "interfaces", "wan")
	var detail map[string]any
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &detail) != nil || len(detail) != 3 || detail["name"] != "wan" {
		t.Fatalf("selected JSON changed identity/shape: %d %q %q", r.code, r.stdout, r.stderr)
	}
	r = f.ngfw(t, nil, "", "--json", "show", "interfaces")
	var got, want any
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &got) != nil || json.Unmarshal([]byte(ethernetInventory), &want) != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("full JSON is not the unmodified API answer: %d %q %q", r.code, r.stdout, r.stderr)
	}
	a := &App{Stdin: os.Stdin, Stdout: io.Discard, Stderr: io.Discard, Getenv: func(string) string { return "" }}
	c, err := api.New(f.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c.Cred, a.client = api.Key("ngfwk_test"), c
	cs, _ := a.candidates(context.Background(), "show interfaces ")
	var names []string
	for _, c := range cs {
		names = append(names, c.Text)
	}
	if !slices.Contains(names, "Ethernet2") || !slices.Contains(names, "Ethernet2.100") || slices.Contains(names, "Ethernet1") || slices.Contains(names, "wan") {
		t.Errorf("completion must use the visible aliases: %v", names)
	}
}

func TestHumanAliasSelectionRejectsDuplicateLabels(t *testing.T) {
	f := newFake(t)
	inventoryResponse(f, `{"items":[
 {"name":"wan","kind":"interface","physical":{"pci":"0000:01:00.0"},"config":{"mtu":1500}},
 {"name":"Ethernet1","kind":"interface","state":{"type":"loopback"},"config":{"mtu":9000}}
]}`)
	r := f.ngfw(t, nil, "", "show", "interfaces", "Ethernet1")
	if r.code != ExitUsage || !strings.Contains(r.stderr, "ambiguous interface display name") {
		t.Fatalf("ambiguous alias silently selected a row: %d %q %q", r.code, r.stdout, r.stderr)
	}
	r = f.ngfw(t, nil, "", "--json", "show", "interfaces", "Ethernet1")
	if r.code != 0 || !strings.Contains(r.stdout, `"name":"Ethernet1"`) || !strings.Contains(r.stdout, `"mtu":9000`) {
		t.Fatalf("machine lookup must use the exact raw name: %d %q %q", r.code, r.stdout, r.stderr)
	}
}

func TestRoutesLabelInterfacesAndTolerateMissingInventory(t *testing.T) {
	f := newFake(t)
	inventoryResponse(f, ethernetInventory)
	f.override["GET /api/v1/state/routes"] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, `{"page":1,"pageSize":500,"total":1,"items":[{"vrf":"default","prefix":"0.0.0.0/0","origin":"static","nextHops":[{"address":"192.0.2.1","interface":"GigabitEthernet0/2/0.100"}],"distance":1}]}`)
	}
	r := f.ngfw(t, nil, "", "show", "ip", "route")
	if r.code != 0 || !strings.Contains(r.stdout, "192.0.2.1 via Ethernet2.100") || strings.Contains(r.stdout, "GigabitEthernet") {
		t.Fatalf("human route aliases: %d %q %q", r.code, r.stdout, r.stderr)
	}
	before := len(f.requests())
	r = f.ngfw(t, nil, "", "--json", "show", "ip", "route")
	if r.code != 0 || !strings.Contains(r.stdout, `"interface":"GigabitEthernet0/2/0.100"`) || strings.Contains(r.stdout, "Ethernet2.100") {
		t.Fatalf("route JSON identity changed: %d %q %q", r.code, r.stdout, r.stderr)
	}
	for _, req := range f.requests()[before:] {
		if strings.Contains(req, "/state/interfaces") {
			t.Fatalf("machine route output fetched display inventory: %q", req)
		}
	}
	f.override["GET /api/v1/state/interfaces"] = func(w http.ResponseWriter, _ *http.Request) {
		problem(w, http.StatusServiceUnavailable, "inventory unavailable")
	}
	r = f.ngfw(t, nil, "", "show", "ip", "route")
	if r.code != 0 || !strings.Contains(r.stdout, "192.0.2.1 via GigabitEthernet0/2/0.100") {
		t.Fatalf("optional inventory failure hid routes: %d %q %q", r.code, r.stdout, r.stderr)
	}
}
