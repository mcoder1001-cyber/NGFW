// Package ripng renders and observes RIPng routing configuration.
package ripng

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"strings"
	"testing"
)

func TestRipngCanonicalAndHostile(t *testing.T) {
	cfg := &ngfwv1.RipngConfig{Networks: []string{"2001:db8::/64"}, Interfaces: map[string]*ngfwv1.RipInterface{"loop0": {Passive: proto.Bool(true)}}, Redistribute: &ngfwv1.Redistribute{Ospf: &ngfwv1.RedistributeOptions{}}}
	mapper := func(name string) (string, bool) { return "eth0", name == "loop0" }
	lines, e := Render(cfg, mapper)
	if e != nil {
		t.Fatal(e)
	}
	rendered := strings.Join(lines, "\n")
	for _, want := range []string{"router ripng", " network 2001:db8::/64", " network eth0", " passive-interface eth0", " redistribute ospf6"} {
		if !strings.Contains(rendered, want) {
			t.Fatal(want, rendered)
		}
	}
	if strings.Contains(rendered, "version 2") {
		t.Fatal("RIPng is not RIPv2")
	}
	for _, network := range []string{"192.0.2.0/24", "2001:db8::1/64", "2001:db8::/64\nrouter bgp 1"} {
		cfg.Networks = []string{network}
		if _, e := Render(cfg, mapper); e == nil {
			t.Fatal("hostile network")
		}
	}
}
