package agent

// F-default-vpp-nics (D-164): project() drops a seeded physical NIC (KindExisting + `physical`) that is still a Linux
// kernel interface (the netdev lookup says it exists) with an agent.nic-not-bound warning, so DryRun/Apply succeed
// and no alias is emitted for it (its Retrieve row is absent → "awaiting dataplane"). Once the NIC is bound to VPP
// (its kernel netdev is gone), the interface projects normally.

import (
	"testing"

	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
)

func physicalDoc(owner string) *ngfwv1.DesiredState {
	return &ngfwv1.DesiredState{
		Interfaces: map[string]*ngfwv1.Interface{
			"ens161": {
				Enabled:  proto.Bool(true),
				Ipv4:     []string{"10.0.0.1/24"},
				Physical: &ngfwv1.InterfacePhysical{Pci: proto.String("0000:04:00.0"), Owner: proto.String(owner), BuiltIn: proto.Bool(true)},
			},
		},
	}
}

func hasKey(kvs []scheduler.KV, k scheduler.Key) bool {
	for _, kv := range kvs {
		if kv.Key == k {
			return true
		}
	}
	return false
}

func warnsOf(p *projected, rule string) []issue {
	var out []issue
	for _, is := range p.issues {
		if is.rule == rule {
			out = append(out, is)
		}
	}
	return out
}

func TestProjectPhysicalNicAwaitingDataplane(t *testing.T) {
	// the netdev still exists as a kernel interface: the NIC is not bound to VPP yet
	kernelExists := func(string) (string, bool, error) { return "", true, nil }
	p := project(physicalDoc("dataplane"), []string{"interfaces"}, nil, kernelExists)

	if p.hasErrors() {
		t.Fatalf("must not error (DryRun/Apply succeed): %+v", p.issues)
	}
	if hasKey(p.kvs, iface.AliasKey("ens161")) {
		t.Fatal("no alias must be emitted for a physical NIC that is not bound (its VPP counterpart is absent)")
	}
	w := warnsOf(p, "agent.nic-not-bound")
	if len(w) != 1 || w[0].pointer != "/interfaces/ens161" {
		t.Fatalf("want one agent.nic-not-bound warning at /interfaces/ens161, got %+v", p.issues)
	}
}

func TestProjectPhysicalNicBoundProjectsNormally(t *testing.T) {
	// the kernel netdev is gone (handed to DPDK): VPP owns it, project as an ordinary physical interface
	boundToDpdk := func(string) (string, bool, error) { return "", false, nil }
	p := project(physicalDoc("dataplane"), []string{"interfaces"}, nil, boundToDpdk)

	if len(warnsOf(p, "agent.nic-not-bound")) != 0 {
		t.Fatalf("a bound physical NIC must not warn nic-not-bound: %+v", p.issues)
	}
	if !hasKey(p.kvs, iface.AliasKey("ens161")) {
		t.Fatal("a bound physical NIC must get its interface alias")
	}
}

func TestProjectPhysicalNicNoLookupProjectsNormally(t *testing.T) {
	p := project(physicalDoc("dataplane"), []string{"interfaces"}, nil, nil)
	if len(warnsOf(p, "agent.nic-not-bound")) != 0 || !hasKey(p.kvs, iface.AliasKey("ens161")) {
		t.Fatalf("without a netdev lookup the physical NIC projects normally: %+v", p.issues)
	}
}

// review R2R4 #3: after handover a linux-cp tap mirrors the VPP interface under the same name (rtnetlink kind "tun");
// that is not the unbound kernel NIC, so the interface projects normally (no flapping nic-not-bound).
func TestProjectPhysicalNicLcpTapIsNotUnbound(t *testing.T) {
	lcpTap := func(string) (string, bool, error) { return "tun", true, nil }
	p := project(physicalDoc("dataplane"), []string{"interfaces"}, nil, lcpTap)
	if len(warnsOf(p, "agent.nic-not-bound")) != 0 {
		t.Fatalf("an lcp tap of the same name must not mark the NIC unbound: %+v", p.issues)
	}
	if !hasKey(p.kvs, iface.AliasKey("ens161")) {
		t.Fatal("the bound physical NIC (mirrored by an lcp tap) must get its interface alias")
	}
}

// review R1R3 #12: the linux-cp pair of an unbound seeded NIC is not projected either (no VPP interface to pair).
func TestProjectPhysicalNicUnboundHasNoLcpPair(t *testing.T) {
	ds := physicalDoc("dataplane")
	ds.Interfaces["ens161"].Lcp = &ngfwv1.InterfaceLcp{}
	kernelExists := func(string) (string, bool, error) { return "", true, nil }
	p := project(ds, []string{"interfaces"}, nil, kernelExists)
	if hasKey(p.kvs, scheduler.Join(lcp.NameItfPair, "ens161")) {
		t.Fatal("no linux-cp pair for a NIC that is not bound to the data plane")
	}
	bound := func(string) (string, bool, error) { return "", false, nil }
	p = project(ds, []string{"interfaces"}, nil, bound)
	if !hasKey(p.kvs, scheduler.Join(lcp.NameItfPair, "ens161")) {
		t.Fatal("a bound physical NIC keeps its linux-cp pair")
	}
}

// review R1R3 #15: a NIC released to the host is never configured on the data plane and is not reported as
// "not bound yet" — it gets an informational agent.nic-released note instead, bound or not.
func TestProjectPhysicalNicReleasedToHost(t *testing.T) {
	for _, lookup := range []desired.NetdevKind{nil, func(string) (string, bool, error) { return "", true, nil }} {
		p := project(physicalDoc("host"), []string{"interfaces"}, nil, lookup)
		if p.hasErrors() || len(warnsOf(p, "agent.nic-not-bound")) != 0 || hasKey(p.kvs, iface.AliasKey("ens161")) {
			t.Fatalf("released NIC: %+v kvs=%d", p.issues, len(p.kvs))
		}
		if n := warnsOf(p, "agent.nic-released"); len(n) != 1 || n[0].severity != ngfwv1.IssueSeverity_ISSUE_SEVERITY_INFO {
			t.Fatalf("want one INFO agent.nic-released, got %+v", p.issues)
		}
	}
}
