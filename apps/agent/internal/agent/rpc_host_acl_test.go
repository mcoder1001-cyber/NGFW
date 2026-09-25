package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/renderers/nftables"
)

// F-host-acl-nftables: the acl domain's host firewall end to end through the service on the fake VPP.
// A test owner is never the product owner, so without VRX_HOST_ACL_NETNS the family runs in mode check:
// every rendering is validated with `nft -c` (allowed in the root netns) and nothing is loaded. The
// namespace path (mode netns) is covered by renderers/nftables' integration test and the topology run.

const hostACLJSON = `{
  "objects": {
    "addresses": {"admins": {"type": "network", "prefix": "10.7.0.0/24"}},
    "services": {"ssh": {"protocol": "tcp", "destinationPorts": ["22"]}}
  },
  "acl": {
    "host": {"mgmt": {"description": "management", "rules": [
      {"sequence": 10, "action": "accept", "source": {"kind": "object", "name": "admins"}, "service": {"kind": "object", "name": "ssh"}},
      {"sequence": 20, "action": "drop", "service": {"kind": "inline", "spec": {"protocol": "tcp", "destinationPorts": ["22", "443"]}}}
    ]}},
    "hostAttachments": [{"list": "mgmt", "chain": "input", "priority": 0}],
    "hostSettings": {"antiLockout": {"enabled": true, "sources": ["10.7.0.0/24"], "ports": [22]}}
  }
}`

func requireNft(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(nftables.NftBin); err != nil || os.Geteuid() != 0 {
		t.Skip("needs nft and root for `nft -c`")
	}
	t.Setenv(nftables.EnvNetns, "")
	t.Setenv(nftables.EnvMode, "")
}

func TestHostACLDomainOnFake(t *testing.T) {
	requireNft(t)
	v := coretest.New()
	dir := t.TempDir()
	s := newObjectsSvc(t, v, dir)
	if !strings.Contains(strings.Join(s.Health().GetSubsystems(), ","), "acl") {
		t.Fatalf("acl not in Health.subsystems: %v", s.Health().GetSubsystems())
	}
	want := doc(t, hostACLJSON)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "h1", DesiredState: want})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	var found bool
	for _, r := range resp.GetResults() {
		if r.GetKey() == string(nftables.Key) {
			found = r.GetPointer() == "/acl" && r.GetSubsystem() == "acl"
		}
	}
	if !found {
		t.Fatalf("no %s result at /acl: %v", nftables.Key, resp.GetResults())
	}
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"acl"}})
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got.GetDesiredState().GetAcl(), want.GetAcl()) {
		t.Fatalf("Retrieve(acl) != desired:\n got %s\nwant %s", protojson.Format(got.GetDesiredState().GetAcl()), protojson.Format(want.GetAcl()))
	}
	again := apply(t, s, &vrxv1.ApplyRequest{TxnId: "h2", DesiredState: want})
	mustStatus(t, again, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n := again.GetSummary().GetCreated() + again.GetSummary().GetUpdated() + again.GetSummary().GetDeleted(); n != 0 {
		t.Errorf("second Apply is not empty: %v", again.GetSummary())
	}

	st, err := s.HostACLState(context.Background(), &vrxv1.HostAclStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if st.GetTable() != "vrx_"+testOwner || st.GetMode() != nftables.ModeCheck || st.GetPresent() || !st.GetInSync() || len(st.GetChains()) != 1 {
		t.Fatalf("state %v", st)
	}
	rules := st.GetChains()[0].GetRules()
	if last := rules[len(rules)-1]; last.GetPointer() != "/acl/host/mgmt/rules/1" || last.GetVerdict() != "drop" {
		t.Errorf("last rule %v", last)
	}

	// Anti-lockout off + a rule that drops management SSH → DryRun error at the rule's pointer.
	bad := doc(t, hostACLJSON)
	bad.GetAcl().GetHostSettings().GetAntiLockout().Enabled = proto.Bool(false)
	bad.GetAcl().GetHost()["mgmt"].GetRules()[0].Source = &vrxv1.AddressMatch{Kind: proto.String("prefix"), Prefix: proto.String("10.7.0.0/25")}
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{DesiredState: bad})
	if err != nil {
		t.Fatal(err)
	}
	if rep.GetOk() || len(rep.GetErrors()) == 0 || rep.GetErrors()[0].GetRule() != nftables.RuleAntiLockout || rep.GetErrors()[0].GetPointer() != "/acl/host/mgmt/rules/1" {
		t.Fatalf("anti-lockout DryRun: %v", rep)
	}
	if resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "h3", DesiredState: bad}); resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_FAILED {
		t.Fatalf("Apply of a lockout must fail validation: %v", resp)
	}

	// F-acl's leaves are reported, not applied.
	withLists := doc(t, hostACLJSON)
	withLists.GetAcl().Lists = map[string]*vrxv1.AclList{"l": {}}
	rep, _ = s.DryRun(context.Background(), &vrxv1.DryRunRequest{DesiredState: withLists})
	var warned bool
	for _, e := range rep.GetErrors() {
		warned = warned || (e.GetPointer() == "/acl/lists" && e.GetRule() == "agent.unsupported-field")
	}
	if !warned || !rep.GetOk() {
		t.Errorf("acl.lists must be an unsupported-field warning: %v", rep)
	}

	// Removing the host firewall deletes the object (and the store).
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "h4", DesiredState: doc(t, `{"objects": {}, "acl": {}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, err = s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"acl"}})
	if err != nil || got.GetDesiredState().GetAcl() != nil {
		t.Fatalf("after removal: %v %v", got.GetDesiredState().GetAcl(), err)
	}
}

// A restarted agent (new service on the same state dir) retrieves the applied host firewall from its
// store, and the resync plan is empty.
func TestHostACLSurvivesRestart(t *testing.T) {
	requireNft(t)
	v := coretest.New()
	dir := t.TempDir()
	s := newObjectsSvc(t, v, dir)
	want := doc(t, hostACLJSON)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "r1", DesiredState: want}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	s.Close()

	s2 := newObjectsSvc(t, v, dir)
	resp := s2.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, err := s2.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"acl"}})
	if err != nil || !proto.Equal(got.GetDesiredState().GetAcl(), want.GetAcl()) {
		t.Fatalf("after restart: %v %v", got.GetDesiredState().GetAcl(), err)
	}
}
