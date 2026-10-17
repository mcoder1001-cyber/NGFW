package agent

// PR #238 review R4 #1 (docs/tech-debt.md, 2026-10-17): a block list that selects all interfaces — the auto-block overlay
// always does — must target only the interface rows the projection keeps. An unbound seeded NIC (still a Linux kernel
// interface) or a NIC released to the host has no interface alias, so a binding for it would fail the plan with a
// missing dependency; every interfaces/security commit was refused while such a list existed and new automatic blocks
// were enforced nowhere (AutoBlockSet returned Unavailable).

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/autoblock"
	descacl "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/renderers/nftables"
	"ngfw/agent/internal/scheduler"
)

// lan is bound (VPP owns it), ens192 is still a kernel NIC (fakeNetdevs: a physical netdev with no rtnetlink kind,
// not bound yet) and ens161 is released to the host; auto-block is enabled.
const nicsAutoBlockDoc = `{"interfaces":{
  "lan":{"enabled":true,"physical":{"pci":"0000:04:00.0","owner":"dataplane","builtIn":true}},
  "ens192":{"enabled":true,"physical":{"pci":"0000:0b:00.0","owner":"dataplane","builtIn":true}},
  "ens161":{"enabled":true,"physical":{"pci":"0000:13:00.0","owner":"host","builtIn":true}}},
 "security":{"autoBlock":{"enabled":true,"maxEntries":100}}}`

func autoBlockEntry(now time.Time) *ngfwv1.AutoBlockRuntimeEntry {
	return &ngfwv1.AutoBlockRuntimeEntry{Source: "192.0.2.7", ExpiresAt: timestamppb.New(now.Add(time.Minute))}
}

// The projection: the overlay's all-interfaces list binds the bound NIC only; the unbound and the released row get no
// acl.interface-binding (nothing to depend on) and keep their nic-not-bound / nic-released notes.
func TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs(t *testing.T) {
	now := time.Now()
	view, err := autoblock.Overlay(doc(t, nicsAutoBlockDoc), []*ngfwv1.AutoBlockRuntimeEntry{autoBlockEntry(now)}, now)
	if err != nil {
		t.Fatal(err)
	}
	p := project(view, []string{"interfaces", "acl"}, nil, fakeNetdevs)
	if p.hasErrors() {
		t.Fatalf("must not error: %+v", p.issues)
	}
	if !hasKey(p.kvs, descacl.KeyInterfaceBinding("lan")) {
		t.Fatalf("the bound NIC must get the auto-block binding: %v", p.kvs)
	}
	for _, name := range []string{"ens192", "ens161"} {
		if hasKey(p.kvs, descacl.KeyInterfaceBinding(name)) {
			t.Fatalf("no binding for %s: its interface alias is never projected", name)
		}
	}
	if len(warnsOf(p, "agent.nic-not-bound")) != 1 || len(warnsOf(p, "agent.nic-released")) != 1 {
		t.Fatalf("want one nic-not-bound and one nic-released note: %+v", p.issues)
	}
}

// End to end on the fake VPP: with an unbound and a released NIC in the running configuration, a new automatic block is
// enforced (AutoBlockSet succeeds, the bound NIC carries the auto-block ACL) and an interfaces commit while entries
// are live is still accepted — DryRun reports no error, Apply is APPLIED and the new interface is bound too.
func TestAutoBlockWithUnboundAndReleasedNICsOnFake(t *testing.T) {
	v := coretest.New()
	v.AddInterface("lan", "dpdk", "")
	s, _ := newACLSvc(t, v, t.TempDir(), false)
	host := &autoBlockHostMemory{}
	reg := scheduler.NewRegistry()
	for _, d := range s.sched.Registry().Descriptors() {
		if d.Name() != nftables.DescriptorName {
			reg.Register(d)
		}
	}
	reg.Register(host)
	s.sched = scheduler.New(reg, nil)
	s.sched.VerifyRetries = 0
	ctx := context.Background()
	ds := doc(t, nicsAutoBlockDoc)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "nics-config", DesiredState: ds}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	now := time.Now()
	s.now = func() time.Time { return now }
	resp, err := s.AutoBlockSet(ctx, &ngfwv1.AutoBlockSetRequest{Owner: testOwner, Entries: []*ngfwv1.AutoBlockRuntimeEntry{autoBlockEntry(now)}})
	if err != nil || resp.GetActiveEntries() != 1 {
		t.Fatalf("auto-block set with an unbound and a released NIC: %v %v", resp, err)
	}
	blockIndex := ownedACL(t, v, "_gb.auto-block.i00")
	boundTo := func(name string) bool {
		_, acls := v.ACL().Binding(loopIndex(t, v, name))
		for _, idx := range acls {
			if idx == blockIndex {
				return true
			}
		}
		return false
	}
	if !boundTo("lan") || host.value == nil {
		t.Fatal("the bound NIC must carry the auto-block ACL and the host rule must be installed")
	}

	update := proto.Clone(ds).(*ngfwv1.DesiredState)
	update.Interfaces["loop712"] = &ngfwv1.Interface{Ipv4: []string{"10.71.2.1/24"}}
	report, err := s.DryRun(ctx, &ngfwv1.DryRunRequest{TxnId: "nics-dry", DesiredState: update, Subsystems: []string{"interfaces"}})
	if err != nil {
		t.Fatal(err)
	}
	notes := map[string]int{}
	for _, issue := range report.GetErrors() {
		if issue.GetSeverity() == ngfwv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
			t.Fatalf("interfaces commit with live auto-block entries: %v", issue)
		}
		notes[issue.GetRule()]++
	}
	if notes["agent.nic-not-bound"] != 1 || notes["agent.nic-released"] != 1 {
		t.Fatalf("want the nic-not-bound and nic-released notes: %v", report.GetErrors())
	}
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "nics-update", DesiredState: update, Subsystems: []string{"interfaces"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !boundTo("lan") || !boundTo("loop712") {
		t.Fatal("the auto-block ACL must stay on the bound NIC and reach the new interface")
	}
}
