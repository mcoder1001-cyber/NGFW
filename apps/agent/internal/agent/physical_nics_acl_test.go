package agent

// PR #238 review R4 #1 (PR #241): a block list that selects all interfaces — the auto-block overlay always does —
// targets only the interface rows the projection keeps. An unbound seeded NIC (still a Linux kernel interface) or a
// NIC released to the host has no interface alias, so a binding for it failed the plan with a missing dependency;
// every interfaces/security commit was refused and new automatic blocks were enforced nowhere (AutoBlockSet returned
// Unavailable) while such a list or live entries existed.

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/autoblock"
	descacl "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/renderers/nftables"
	"ngfw/agent/internal/scheduler"
)

// Three seeded physical rows: lan is bound (VPP owns it; no kernel netdev of that name), ens192 is still a kernel NIC
// (not bound yet) and ens161 is released to the host.
const nicRowsJSON = `"lan":{"enabled":true,"physical":{"pci":"0000:04:00.0","owner":"dataplane","builtIn":true}},
  "ens192":{"enabled":true,"physical":{"pci":"0000:0b:00.0","owner":"dataplane","builtIn":true}},
  "ens161":{"enabled":true,"physical":{"pci":"0000:13:00.0","owner":"host","builtIn":true}}`

// nicsAutoBlockDoc: the rows with auto-block enabled (the overlay selects all interfaces).
const nicsAutoBlockDoc = `{"interfaces":{` + nicRowsJSON + `},"security":{"autoBlock":{"enabled":true,"maxEntries":100}}}`

// nicsAllInterfacesDoc: the rows plus a loopback, with a user block list on all interfaces.
const nicsAllInterfacesDoc = `{"interfaces":{` + nicRowsJSON + `,"loop711":{"ipv4":["10.71.1.1/24"]}},
 "acl":{"globalBlocking":{"lists":{"bad":{"enabled":true,"source":{"kind":"upload"},"allInterfaces":true,"direction":"both","protectHost":false,"log":false,"entries":["192.0.2.7/32"]}}}}}`

// nicLookup is the netdev lookup of these tests: a name in kernel is a Linux kernel NIC (present, no rtnetlink
// kind), the other seeded rows have no kernel netdev (handed to VPP), everything else is fakeNetdevs.
func nicLookup(kernel map[string]bool) desired.NetdevKind {
	return func(name string) (string, bool, error) {
		switch {
		case kernel[name]:
			return "", true, nil
		case name == "lan" || name == "ens192" || name == "ens161":
			return "", false, nil
		}
		return fakeNetdevs(name)
	}
}

// nicsSvc is an acl service on the fake VPP with the in-memory host descriptor (no nftables) and the nicLookup over
// kernel, which the test may change between transactions.
func nicsSvc(t *testing.T, v *coretest.VPP, kernel map[string]bool) (*Service, *autoBlockHostMemory) {
	t.Helper()
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
	s.netdevKind = nicLookup(kernel)
	return s, host
}

func autoBlockEntry(now time.Time) *ngfwv1.AutoBlockRuntimeEntry {
	return &ngfwv1.AutoBlockRuntimeEntry{Source: "192.0.2.7", ExpiresAt: timestamppb.New(now.Add(time.Minute))}
}

// boundTo reports whether the fake VPP interface name carries the ACL acl.
func boundTo(t *testing.T, v *coretest.VPP, name string, acl uint32) bool {
	t.Helper()
	_, acls := v.ACL().Binding(loopIndex(t, v, name))
	for _, idx := range acls {
		if idx == acl {
			return true
		}
	}
	return false
}

// The projection: the overlay's all-interfaces list binds the bound NIC only; the unbound and the released row get no
// acl.interface-binding (nothing to depend on) and keep their nic-not-bound / nic-released notes; the unbound row is
// also named at /security/autoBlock, so the drop is visible in an acl-only transaction too (the released row only
// through agent.nic-released).
func TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs(t *testing.T) {
	now := time.Now()
	view, err := autoblock.Overlay(doc(t, nicsAutoBlockDoc), []*ngfwv1.AutoBlockRuntimeEntry{autoBlockEntry(now)}, now)
	if err != nil {
		t.Fatal(err)
	}
	p := project(view, []string{"interfaces", "acl"}, nil, nicLookup(map[string]bool{"ens192": true}))
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
	// the NIC awaiting the engine is reported at /security/autoBlock (the field the operator configured); the
	// released NIC only through its agent.nic-released note
	w := warnsOf(p, "acl.global-blocking")
	if len(w) != 1 || w[0].pointer != "/security/autoBlock" || w[0].severity != ngfwv1.IssueSeverity_ISSUE_SEVERITY_WARNING ||
		w[0].message != "Auto-block selects all interfaces, but these physical NICs are not bound to the data plane yet and are skipped: ens192" {
		t.Fatalf("want one warning for the unbound NIC at /security/autoBlock: %+v", p.issues)
	}
}

// Review R1 #1 / R4 #1: the kernel is observed once per projection — the acl block lists reuse the interfaces split
// instead of looking the netdevs up again, so the two domains cannot disagree and the acl domain adds no netlink cost.
func TestProjectAutoBlockOverlayAddsNoNetdevLookups(t *testing.T) {
	now := time.Now()
	view, err := autoblock.Overlay(doc(t, nicsAutoBlockDoc), []*ngfwv1.AutoBlockRuntimeEntry{autoBlockEntry(now)}, now)
	if err != nil {
		t.Fatal(err)
	}
	count := func(domains []string) map[string]int {
		counts := map[string]int{}
		inner := nicLookup(map[string]bool{"ens192": true})
		p := project(view, domains, nil, func(name string) (string, bool, error) { counts[name]++; return inner(name) })
		if p.hasErrors() {
			t.Fatalf("%v: %+v", domains, p.issues)
		}
		return counts
	}
	only, withACL := count([]string{"interfaces"}), count([]string{"interfaces", "acl"})
	if only["ens192"] != 1 || only["ens161"] != 0 {
		t.Fatalf("interfaces split: one lookup per engine-owned row, none for a released one: %v", only)
	}
	for name, n := range withACL {
		if n != only[name] {
			t.Fatalf("the acl projection must not look netdevs up again: %v vs %v", withACL, only)
		}
	}
}

// End to end on the fake VPP with auto-block: with an unbound and a released NIC in the running configuration, a new
// automatic block is enforced (AutoBlockSet succeeds, the bound NIC carries the auto-block ACL); an interfaces commit
// while entries are live is accepted (DryRun has no error, Apply is APPLIED) and binds the new interface too; a NIC
// handed to VPP later is bound by the connect resync together with its own configuration, a second resync changes
// nothing; releasing a NIC VPP still has removes our binding and takes it admin-down (review R1 #2, R4 #5).
func TestAutoBlockWithUnboundAndReleasedNICsOnFake(t *testing.T) {
	v := coretest.New()
	v.AddInterface("lan", "dpdk", "")
	kernel := map[string]bool{"ens192": true}
	s, host := nicsSvc(t, v, kernel)
	var logs bytes.Buffer
	s.log = slog.New(slog.NewTextHandler(&logs, nil))
	ctx := context.Background()
	ds := doc(t, nicsAutoBlockDoc)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "nics-config", DesiredState: ds}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	now := time.Now()
	s.now = func() time.Time { return now }
	resp, err := s.AutoBlockSet(ctx, &ngfwv1.AutoBlockSetRequest{Owner: testOwner, Entries: []*ngfwv1.AutoBlockRuntimeEntry{autoBlockEntry(now)}})
	if err != nil || resp.GetActiveEntries() != 1 {
		t.Fatalf("auto-block set with an unbound and a released NIC: %v %v", resp, err)
	}
	block := ownedACL(t, v, "_gb.auto-block.i00")
	if !boundTo(t, v, "lan", block) || host.value == nil {
		t.Fatal("the bound NIC must carry the auto-block ACL and the host rule must be installed")
	}
	// the runtime reconcile returns no report: the skipped NIC is logged, once per change of the warnings (review R1
	// V1/N1/N2): one WARN line, none for a repeated snapshot, a true all-clear line when the lists are gone (an empty
	// snapshot removes Auto-block's list) and the WARN again when entries return
	warnLine := `level=WARN msg="block lists: /security/autoBlock — Auto-block selects all interfaces, but these physical NICs are not bound to the data plane yet and are skipped: ens192"`
	clearLine := `level=INFO msg="block lists: no list on all interfaces skips a NIC awaiting the engine any more"`
	if n := strings.Count(logs.String(), warnLine); n != 1 || strings.Contains(logs.String(), "ens161") {
		t.Fatalf("want one WARN line for the NIC awaiting the engine (none for the released one), got %d:\n%s", n, logs.String())
	}
	if _, err := s.AutoBlockSet(ctx, &ngfwv1.AutoBlockSetRequest{Owner: testOwner, Entries: []*ngfwv1.AutoBlockRuntimeEntry{autoBlockEntry(now)}}); err != nil || strings.Count(logs.String(), "block lists:") != 1 {
		t.Fatalf("a repeated snapshot logs nothing new: %v\n%s", err, logs.String())
	}
	if _, err := s.AutoBlockSet(ctx, &ngfwv1.AutoBlockSetRequest{Owner: testOwner}); err != nil || strings.Count(logs.String(), clearLine) != 1 {
		t.Fatalf("an empty snapshot removes Auto-block's list: the all-clear line must be true and logged once: %v\n%s", err, logs.String())
	}
	if _, err := s.AutoBlockSet(ctx, &ngfwv1.AutoBlockSetRequest{Owner: testOwner, Entries: []*ngfwv1.AutoBlockRuntimeEntry{autoBlockEntry(now)}}); err != nil || strings.Count(logs.String(), warnLine) != 2 {
		t.Fatalf("entries returning while the NIC is still unbound log the WARN again: %v\n%s", err, logs.String())
	}
	block = ownedACL(t, v, "_gb.auto-block.i00") // the empty snapshot removed the ACL; the returning entries recreated it

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
	if !boundTo(t, v, "lan", block) || !boundTo(t, v, "loop712", block) {
		t.Fatal("the auto-block ACL must stay on the bound NIC and reach the new interface")
	}

	// ens192 is handed to VPP (start-up apply, VPP restart): the connect resync binds it with its own configuration
	v.AddInterface("ens192", "dpdk", "")
	delete(kernel, "ens192")
	mustStatus(t, s.Resync(ctx), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !boundTo(t, v, "ens192", block) || !v.AdminUp("ens192") {
		t.Fatalf("a NIC bound later must come up with the auto-block ACL: %s", v.Snapshot())
	}
	if r := s.Resync(ctx); r.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED || len(r.GetResults()) != 0 {
		t.Fatalf("a second resync must change nothing: %v", r)
	}
	// the next runtime reconcile finds nothing skipped any more: the all-clear line, this time because of the bind
	if _, err := s.AutoBlockSet(ctx, &ngfwv1.AutoBlockSetRequest{Owner: testOwner, Entries: []*ngfwv1.AutoBlockRuntimeEntry{autoBlockEntry(now)}}); err != nil || s.autoBlock.skipped != "" || strings.Count(logs.String(), clearLine) != 2 {
		t.Fatalf("after the bind nothing is skipped: %v %q\n%s", err, s.autoBlock.skipped, logs.String())
	}

	// lan released while VPP still has it: the commit is accepted, our binding leaves it and it goes admin-down
	released := proto.Clone(update).(*ngfwv1.DesiredState)
	released.Interfaces["lan"].Physical.Owner = proto.String("host")
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "nics-release", DesiredState: released, Subsystems: []string{"interfaces"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if boundTo(t, v, "lan", block) || v.AdminUp("lan") {
		t.Fatalf("a released NIC keeps no block-list binding and is admin-down: %s", v.Snapshot())
	}
	if !boundTo(t, v, "ens192", block) || !boundTo(t, v, "loop712", block) {
		t.Fatal("the other interfaces keep the auto-block ACL")
	}
}

// A user block list on all interfaces over the same rows (review R4 #5): applied with the bound NIC and the loopback
// bound only; Retrieve equals the configuration (the record covers the narrower binding set); a second apply and a
// resync change nothing; a NIC handed to VPP later is bound by the resync.
func TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake(t *testing.T) {
	v := coretest.New()
	v.AddInterface("lan", "dpdk", "")
	kernel := map[string]bool{"ens192": true}
	s, _ := nicsSvc(t, v, kernel)
	ctx := context.Background()
	want := doc(t, nicsAllInterfacesDoc)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "all1", DesiredState: want}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	bad := ownedACL(t, v, "_gb.bad.i00")
	if !boundTo(t, v, "lan", bad) || !boundTo(t, v, "loop711", bad) {
		t.Fatalf("the bound NIC and the loopback carry the list: %s", v.Snapshot())
	}
	if got := retrieveACL(t, s); !proto.Equal(got, want.GetAcl()) {
		t.Fatalf("Retrieve != desired:\n%s\nwant\n%s", protojson.Format(got), protojson.Format(want.GetAcl()))
	}
	if sm := apply(t, s, &ngfwv1.ApplyRequest{TxnId: "all2", DesiredState: want}).GetSummary(); sm.GetCreated()+sm.GetUpdated()+sm.GetDeleted() != 0 {
		t.Fatalf("second apply not empty: %v", sm)
	}
	if r := s.Resync(ctx); r.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED || len(r.GetResults()) != 0 {
		t.Fatalf("resync must change nothing: %v", r)
	}
	v.AddInterface("ens192", "dpdk", "")
	delete(kernel, "ens192")
	mustStatus(t, s.Resync(ctx), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !boundTo(t, v, "ens192", bad) {
		t.Fatalf("a NIC bound later gets the all-interfaces list: %s", v.Snapshot())
	}
	if got := retrieveACL(t, s); !proto.Equal(got, want.GetAcl()) {
		t.Fatalf("Retrieve != desired after the bind:\n%s", protojson.Format(got))
	}
}
