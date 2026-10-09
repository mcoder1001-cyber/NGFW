package pppoe

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCarrierRenderPrivatePeerWithoutUnit(t *testing.T) {
	spec, err := NewCarrierSpec("ngfw", "pppwan", "wanraw", 1492)
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(t.TempDir(), "ppp")
	r := New(WithPaths(CarrierPaths(base, filepath.Join(t.TempDir(), "state"))))
	files, err := r.RenderCarrier(Session{Carrier: &spec, Iface: spec.Logical, HostIf: spec.RawHost(), Username: "test", Password: "NGFW_TEST_PSK_F-pppoe-client-wiring", MTU: 1492, IPv6: "dhcpv6"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files[filepath.Join(base, "peers", "carrier")]; !ok {
		t.Fatal("fixed peer missing")
	}
	for path, f := range files {
		if strings.HasSuffix(path, ".service") {
			t.Fatal("generated unit", path)
		}
		if strings.Contains(path, "ngfw-ipv6-") {
			text := string(f.Content)
			if strings.Contains(text, "accept_ra\").write_text") || !strings.Contains(text, "/etc/ppp/ngfw-ipv6-") {
				t.Fatal("namespace helper path or sysctl boundary incorrect")
			}
		}
	}
}
