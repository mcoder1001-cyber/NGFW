package ravpn

import "testing"

func TestKernelFallbackRequiresRecordedIdentityAndDownState(t *testing.T) {
	plan := networkFixture()
	plan.NamespaceInode, plan.HostNamespaceInode = 123, 456
	plan.KernelLinks = []KernelLink{{Name: "ip6tnl0", Kind: "ip6tnl", Index: 2}}
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	link := observedLink{Name: "ip6tnl0", Index: 2}
	link.Info.Kind = "ip6tnl"
	if !matchesKernelLink(plan, link) {
		t.Fatal("recorded fallback refused")
	}
	for _, mutate := range []func(*observedLink){
		func(l *observedLink) { l.Name = "foreign0" },
		func(l *observedLink) { l.Index++ },
		func(l *observedLink) { l.Info.Kind = "dummy" },
		func(l *observedLink) { l.Flags = []string{"UP"} },
	} {
		changed := link
		mutate(&changed)
		if matchesKernelLink(plan, changed) {
			t.Fatal("foreign or activated device accepted")
		}
	}
	plan.NamespaceInode, plan.HostNamespaceInode = 0, 0
	if plan.Validate() == nil {
		t.Fatal("operator supplied baseline accepted")
	}
	plan.NamespaceInode, plan.HostNamespaceInode = 123, 456
	plan.KernelLinks[0].Name = "foreign0"
	if plan.Validate() == nil {
		t.Fatal("arbitrary device adopted")
	}
}
