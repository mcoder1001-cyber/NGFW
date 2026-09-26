package lcpmap_test

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/lcpmap"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
)

func doc(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}

func TestHostName(t *testing.T) {
	for _, tc := range []struct {
		vpp, host, want string
		ok              bool
	}{
		{"loop0", "", "loop0", true},
		{"host-w8l0", "w8-l0", "w8-l0", true},
		{"TenGigabitEthernet0/0/0", "", "", false},
		{"TenGigabitEthernet0/0/0", "te0", "te0", true},
		{"loop0", "10.0.0.1", "", false},
		{"loop0", "a-name-that-is-too-long", "", false},
	} {
		var l *vrxv1.InterfaceLcp
		if tc.host != "" {
			l = &vrxv1.InterfaceLcp{HostIfName: &tc.host}
		}
		got, err := lcpmap.HostName(tc.vpp, l)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("HostName(%q, %q) = %q, %v", tc.vpp, tc.host, got, err)
		}
	}
	if lcpmap.HostType(nil) != "tap" || lcpmap.HostType(&vrxv1.InterfaceLcp{HostIfType: new(string)}) != "tap" {
		t.Fatal("default host type")
	}
}

func TestMapperAndAddressLines(t *testing.T) {
	ds := doc(t, `{"interfaces": {
	  "host-w8l0": {"ipv4": ["10.8.1.1/24", "10.8.1.9/24"], "ipv6": ["fe80::1/64", "2001:db8:8:1::1/64"], "lcp": {"hostIfName": "w8-l0"}},
	  "loop0": {"ipv4": ["10.8.255.1/32"], "lcp": {}},
	  "loop1": {"ipv4": ["10.8.255.2/32"]},
	  "Gi0/1/0": {"ipv4": ["10.8.5.1/24"], "lcp": {}}}}`)
	m := lcpmap.FromDesired(ds)
	if len(m) != 2 || m["host-w8l0"] != "w8-l0" || m["loop0"] != "loop0" {
		t.Fatalf("map %v (the invalid Gi0/1/0 is left out, loop1 has no pair)", m)
	}
	mp := &lcpmap.Mapper{}
	if _, ok := mp.Map("loop0"); ok {
		t.Fatal("empty mapper maps")
	}
	mp.Set(m)
	if n, ok := mp.Map("host-w8l0"); !ok || n != "w8-l0" {
		t.Fatal(n, ok)
	}
	r := frr.New(renderers.NewRecordingRunner(), frr.WithSections(), frr.WithInterfaceMapper(mp.Map),
		frr.WithInterfaceLines(frr.NamedInterfaceLines{Name: lcpmap.LinesName, Fn: lcpmap.AddressLines}))
	ds.Interfaces["host-w8l0"].Description = strPtr("lan side")
	files, err := r.Render(context.Background(), ds)
	if err != nil {
		t.Fatal(err)
	}
	conf := string(files[r.Paths().ConfFile()].Content)
	want := "interface loop0\n ip address 10.8.255.1/32\nexit\n!\ninterface w8-l0\n description lan side\n ip address 10.8.1.1/24\n ip address 10.8.1.9/24\n ipv6 address 2001:db8:8:1::1/64\nexit\n"
	if !strings.Contains(conf, want) {
		t.Fatalf("frr.conf:\n%s\nwant block:\n%s", conf, want)
	}
	if strings.Contains(conf, "fe80::1") || strings.Contains(conf, "10.8.255.2") {
		t.Fatalf("link-local or unpaired address rendered:\n%s", conf)
	}
}

func strPtr(s string) *string { return &s }
