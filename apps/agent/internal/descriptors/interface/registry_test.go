package iface_test

import (
	"slices"
	"testing"

	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/scheduler"
)

// TestRegisterAllDF1 registers every DF-1 plugin into one registry (as P05 will): no duplicate or
// invalid names, and exactly the object types docs/agent/descriptors/*.md publish.
func TestRegisterAllDF1(t *testing.T) {
	r := scheduler.NewRegistry()
	registerAll(r, ifacetest.New(), owner, t.TempDir())
	want := []string{
		"af-packet.host-interface",
		"bond.bond", "bond.member",
		"interface.admin-state", "interface.mac-address", "interface.mtu", "interface.promisc",
		"interface.rx-mode", "interface.rx-placement", "interface.subinterface",
		"l2.bridge-domain", "l2.bridge-domain-member", "l2.fib-entry", "l2.flags", "l2.vlan-tag-rewrite", "l2.xconnect",
		"l3xc.l3xc",
		"memif.memif", "memif.socket",
		"tapv2.tap",
	}
	got := r.Names()
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("registered %q\nwant       %q", got, want)
	}
	for _, n := range got {
		if !scheduler.ValidName(n) {
			t.Errorf("invalid descriptor name %q", n)
		}
	}
}
