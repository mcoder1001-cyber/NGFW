package agent

import (
	"context"
	"google.golang.org/protobuf/types/known/timestamppb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/renderers/nftables"
	"ngfw/agent/internal/scheduler"
	"testing"
	"time"
)

func TestInterfaceScopePreservesExternalACLWithoutOverlay(t *testing.T) {
	v := coretest.New()
	idx := v.AddACL(testOwner + ":external-reference")
	s, _ := newACLSvc(t, v, t.TempDir(), false)
	ds := doc(t, `{"interfaces":{"loop711":{"ipv4":["10.71.1.1/24"]}}}`)
	ctx := context.Background()
	report, err := s.DryRun(ctx, &ngfwv1.DryRunRequest{TxnId: "scope-dry", DesiredState: ds, Subsystems: []string{"interfaces"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range report.GetErrors() {
		if issue.GetSeverity() == ngfwv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
			t.Fatalf("DryRun: %v", issue)
		}
	}
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "scope-apply", DesiredState: ds, Subsystems: []string{"interfaces"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !v.ACL().Has(idx) {
		t.Fatal("unrelated interfaces transaction deleted owned external ACL")
	}
	for _, call := range v.CallsNamed("acl_del") {
		t.Fatalf("unrequested ACL delete: %v", call)
	}
}

func TestInterfaceScopeReprojectsActiveOverlayAndSecurityRemoval(t *testing.T) {
	v := coretest.New()
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
	ds := doc(t, `{"interfaces":{"loop711":{"ipv4":["10.71.1.1/24"]}},"security":{"autoBlock":{"enabled":true,"maxEntries":100}}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "overlay-config", DesiredState: ds}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	now := time.Now()
	s.now = func() time.Time { return now }
	_, err := s.AutoBlockSet(context.Background(), &ngfwv1.AutoBlockSetRequest{Owner: testOwner, Entries: []*ngfwv1.AutoBlockRuntimeEntry{{Source: "192.0.2.7", ExpiresAt: timestamppb.New(now.Add(time.Minute))}}})
	if err != nil {
		t.Fatal(err)
	}
	update := doc(t, `{"interfaces":{"loop711":{"ipv4":["10.71.1.1/24"]},"loop712":{"ipv4":["10.71.2.1/24"]}}}`)
	planned, err := s.DryRun(context.Background(), &ngfwv1.DryRunRequest{TxnId: "overlay-interface-dry", DesiredState: update, Subsystems: []string{"interfaces"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range planned.GetErrors() {
		if issue.GetSeverity() == ngfwv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
			t.Fatalf("active overlay DryRun: %v", issue)
		}
	}
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "overlay-interface", DesiredState: update, Subsystems: []string{"interfaces"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if contains(s.st.meta.Managed, "acl") {
		t.Fatal("implicit overlay scope changed stored authoritative domains")
	}
	blockIndex := ownedACL(t, v, "_gb.auto-block.i00")
	_, acls := v.ACL().Binding(loopIndex(t, v, "loop712"))
	found := false
	for _, idx := range acls {
		if idx == blockIndex {
			found = true
		}
	}
	if !found || host.value == nil {
		t.Fatalf("new interface lost active overlay: %v", acls)
	}
	// Removing the AutoBlock field entirely must still use the stored/runtime
	// context to expand ACL scope and remove the prior live enforcement.
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "overlay-remove", DesiredState: &ngfwv1.DesiredState{Security: &ngfwv1.SecurityConfig{}}, Subsystems: []string{"security"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(v.ACL().ACLs()) != 0 || host.value != nil {
		t.Fatal("removed security context left stale enforcement")
	}
	for _, name := range []string{"loop711", "loop712"} {
		if _, remaining := v.ACL().Binding(loopIndex(t, v, name)); len(remaining) != 0 {
			t.Fatalf("removed overlay remains bound: %s %v", name, remaining)
		}
	}
}

func TestDisabledAutoBlockDoesNotBroadenUnrelatedInterfaceScope(t *testing.T) {
	v := coretest.New()
	s, _ := newACLSvc(t, v, t.TempDir(), false)
	disabled := doc(t, `{"security":{"autoBlock":{"enabled":false}}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "disabled-config", DesiredState: disabled, Subsystems: []string{"security"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	idx := v.AddACL(testOwner + ":external-reference")
	ds := doc(t, `{"interfaces":{"loop711":{"ipv4":["10.71.1.1/24"]}}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "disabled-interface", DesiredState: ds, Subsystems: []string{"interfaces"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !v.ACL().Has(idx) {
		t.Fatal("inactive auto-block context broadened interfaces into ACL authority")
	}
}
