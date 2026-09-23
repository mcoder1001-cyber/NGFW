package registry

import (
	"sort"
	"testing"

	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
)

func TestRegister(t *testing.T) {
	r := scheduler.NewRegistry()
	Register(r, df7test.NewFake(), Config{Owner: df7test.Owner})
	names := r.Names()
	sort.Strings(names)
	if len(names) != 28 {
		t.Fatalf("%d descriptors: %v", len(names), names)
	}
	for _, g := range []string{"lb.conf", "lldp.global", "bfd.echo-source", "igmp.group-prefix"} {
		if _, ok := r.Get(g); ok {
			t.Fatalf("%s registered for a non-owner (D-071)", g)
		}
	}
	all := scheduler.NewRegistry()
	Register(all, df7test.NewFake(), Config{Owner: df7test.Owner, GlobalsOwner: true})
	if all.Len() != 32 {
		t.Fatalf("globals owner: %d descriptors: %v", all.Len(), all.Names())
	}
	for _, n := range all.Names() {
		if !scheduler.ValidName(n) {
			t.Fatalf("invalid descriptor name %q", n)
		}
	}
	t.Logf("DF-7 descriptors: %v", all.Names())
}
