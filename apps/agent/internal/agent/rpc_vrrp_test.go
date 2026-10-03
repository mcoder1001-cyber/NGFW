package agent

// F-vrrp-config-sync: ha.vrrp (engine vpp) through the agent against the unit-test VPP model
// (coretest/vrrp.go): apply → Retrieve == desired, idempotent re-apply, agent restart (names kept), a VR
// lost behind the agent's back re-created, disable = stopped, rollback removes the VRs; the keepalived
// engine without a linux-cp pair is skipped with a warning; duplicate (interface, family, VRID) refused.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrrpapi "ngfw/agent/binapi/vrrp"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

const vrrpDoc = `{
  "interfaces": {"loop7201": {"ipv4": ["10.7.2.2/24"]}, "loop7202": {"ipv4": ["10.7.3.2/24"]}},
  "ha": {"vrrp": {
    "lan-v4": {"description": "LAN gateway", "interface": "loop7201", "vrId": 10, "priority": 200,
      "advertisementIntervalMs": 500, "addresses": ["10.7.2.1"], "track": [{"interface": "loop7202", "priorityDecrement": 50}]},
    "wan-uni": {"interface": "loop7202", "vrId": 11, "unicast": {"peers": ["10.7.3.3"]}, "addresses": ["10.7.3.1"], "preempt": false}
  }}
}`

const vrrpRetrieved = `{"vrrp": {
  "lan-v4": {"enabled": true, "description": "LAN gateway", "interface": "loop7201", "vrId": 10, "addressFamily": "ipv4",
    "priority": 200, "advertisementIntervalMs": 500, "preempt": true, "acceptMode": false, "addresses": ["10.7.2.1"],
    "engine": "vpp", "track": [{"interface": "loop7202", "priorityDecrement": 50}]},
  "wan-uni": {"enabled": true, "interface": "loop7202", "vrId": 11, "addressFamily": "ipv4", "priority": 100,
    "advertisementIntervalMs": 1000, "preempt": false, "acceptMode": false, "unicast": {"peers": ["10.7.3.3"]},
    "addresses": ["10.7.3.1"], "engine": "vpp"}
}}`

// vrrpGates pins every input of the ha.vrrp engine gates (review S-rva R1 B2): the tests never depend on
// the ambient VRX_TEST_PREFIX / VRX_VRRP_VPP / VRX_KEEPALIVED of a slot or `tools/ci.sh full` (w12).
func vrrpGates(t *testing.T, vpp, keepalived, prefix string) {
	t.Helper()
	t.Setenv("VRX_VRRP_VPP", vpp)
	t.Setenv("VRX_KEEPALIVED", keepalived)
	t.Setenv("VRX_TEST_PREFIX", prefix)
}

func vrrpRetrieve(t *testing.T, s *Service) *vrxv1.HaConfig {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces", "ha"}})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState().GetHa()
}

func TestVrrpApplyRetrieveRestartRollback(t *testing.T) {
	vrrpGates(t, "on", "off", "") // the VPP engine explicitly on (RV-A R4 M1 gate), keepalived off
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)

	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "v1", DesiredState: doc(t, vrrpDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n, run := v.Vrrp().Count(); n != 2 || run != 2 {
		t.Fatalf("VPP holds %d VRs (%d running), want 2 (2)", n, run)
	}
	want := &vrxv1.HaConfig{}
	if err := protojson.Unmarshal([]byte(vrrpRetrieved), want); err != nil {
		t.Fatal(err)
	}
	if got := vrrpRetrieve(t, s); !proto.Equal(got, want) {
		t.Fatalf("Retrieve ha:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	if r := apply(t, s, &vrxv1.ApplyRequest{TxnId: "v2", DesiredState: doc(t, vrrpDoc)}); changes(r) != 0 {
		t.Fatalf("re-apply changed %d objects", changes(r))
	}

	// agent restart (same state dir, same VPP): nothing to do, names kept
	s.Close()
	s2 := newSvc(t, v, dir)
	if r := apply(t, s2, &vrxv1.ApplyRequest{TxnId: "v3", DesiredState: doc(t, vrrpDoc)}); changes(r) != 0 {
		t.Fatalf("apply after restart changed %d objects: %s", changes(r), protojson.Format(r))
	}
	if got := vrrpRetrieve(t, s2); !proto.Equal(got, want) {
		t.Fatalf("Retrieve after restart: %s", protojson.Format(got))
	}

	// VRs lost behind the agent's back (a VPP restart) are re-created and started
	v.Vrrp().Forget()
	mustStatus(t, apply(t, s2, &vrxv1.ApplyRequest{TxnId: "v4", DesiredState: doc(t, vrrpDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n, run := v.Vrrp().Count(); n != 2 || run != 2 {
		t.Fatalf("after loss: %d VRs (%d running), want 2 (2)", n, run)
	}

	// disabled = configured but stopped
	disabled := strings.Replace(vrrpDoc, `"lan-v4": {`, `"lan-v4": {"enabled": false, `, 1)
	mustStatus(t, apply(t, s2, &vrxv1.ApplyRequest{TxnId: "v5", DesiredState: doc(t, disabled)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n, run := v.Vrrp().Count(); n != 2 || run != 1 {
		t.Fatalf("disabled: %d VRs (%d running), want 2 (1)", n, run)
	}
	if got := vrrpRetrieve(t, s2).GetVrrp()["lan-v4"]; got.GetEnabled() {
		t.Fatalf("disabled VR retrieved as enabled: %s", protojson.Format(got))
	}

	// rollback to no VRs: every VR goes, the interfaces stay
	mustStatus(t, apply(t, s2, &vrxv1.ApplyRequest{TxnId: "v6", DesiredState: doc(t, `{"interfaces": {"loop7201": {"ipv4": ["10.7.2.2/24"]}, "loop7202": {"ipv4": ["10.7.3.2/24"]}}, "ha": {}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n, _ := v.Vrrp().Count(); n != 0 {
		t.Fatalf("rollback left %d VRs", n)
	}
	if got := vrrpRetrieve(t, s2); len(got.GetVrrp()) != 0 {
		t.Fatalf("Retrieve after rollback: %s", protojson.Format(got))
	}
}

func TestVrrpDryRunFindings(t *testing.T) {
	// both engines on: keepalived needs a slot prefix (TestPaths) — "on" alone is refused (review S-rva R4 M3)
	vrrpGates(t, "on", "on", testOwner)
	s := newSvc(t, coretest.New(), t.TempDir())
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: doc(t, `{
	  "interfaces": {"loop7201": {}},
	  "ha": {"vrrp": {
	    "a": {"interface": "loop7201", "vrId": 5, "addresses": ["10.7.2.1"]},
	    "b": {"interface": "loop7201", "vrId": 5, "addresses": ["10.7.2.9"]},
	    "k": {"interface": "loop7201", "vrId": 6, "engine": "keepalived", "addresses": ["10.7.2.3"]}
	  }}}`)})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, is := range rep.GetErrors() {
		if strings.HasPrefix(is.GetRule(), "ha.") || strings.HasPrefix(is.GetRule(), "agent.unimplemented") {
			got = append(got, is.GetSeverity().String()+" "+is.GetPointer()+" "+is.GetRule())
		}
	}
	want := []string{
		"ISSUE_SEVERITY_ERROR /ha/vrrp/b/vrId ha.vrrp-duplicate-vrid",
		"ISSUE_SEVERITY_WARNING /ha/vrrp/k/interface ha.vrrp-keepalived-no-lcp",
	}
	if rep.GetOk() || strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("dry run:\n got %v\nwant %v", got, want)
	}
}

const vrrpGatedDoc = `{
  "interfaces": {"loop7301": {"lcp": {"hostIfName": "lan0"}}, "loop7302": {}},
  "ha": {"vrrp": {
    "v": {"interface": "loop7302", "vrId": 21, "addresses": ["10.7.5.1"]},
    "k": {"interface": "loop7301", "vrId": 20, "engine": "keepalived", "addresses": ["10.7.4.1"]}
  }}}`

// TestVrrpEnginesGatedOff (RV-A R4 M1/M2): a lab-slot agent (owner w7, VRX_VRRP_VPP / VRX_KEEPALIVED
// unset → both off) never writes vrrp_vr_* on the shared VPP nor stages keepalived. Both engines are
// reported as configured-but-not-applied warnings and the commit still succeeds.
func TestVrrpEnginesGatedOff(t *testing.T) {
	vrrpGates(t, "", "", "") // all unset, no slot prefix, no VRX_VPP_ID_RANGE=all: both engines default off
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)

	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "g1", DesiredState: doc(t, vrrpGatedDoc)})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, is := range rep.GetErrors() {
		if strings.HasPrefix(is.GetRule(), "ha.vrrp-") {
			got = append(got, is.GetSeverity().String()+" "+is.GetPointer()+" "+is.GetRule())
		}
	}
	want := []string{
		"ISSUE_SEVERITY_WARNING /ha/vrrp/k/engine ha.vrrp-keepalived-disabled",
		"ISSUE_SEVERITY_WARNING /ha/vrrp/v/engine ha.vrrp-vpp-disabled",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("gated dry run:\n got %v\nwant %v", got, want)
	}

	// the warnings do not block the commit, and nothing reaches the shared VPP
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "g2", DesiredState: doc(t, vrrpGatedDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n, _ := v.Vrrp().Count(); n != 0 {
		t.Fatalf("a gated agent wrote %d VRs to the shared VPP", n)
	}
	if got := vrrpRetrieve(t, s); len(got.GetVrrp()) != 0 {
		t.Fatalf("gated agent reports VRs: %s", protojson.Format(got))
	}
}

// TestVrrpRollbackRestoresVppAndMeta (RV-A R1 M4): a VR create refused mid-transaction rolls the whole
// commit back — VPP and the agent-local vrrp-meta-<owner>.json return to the pre-transaction state
// (the earlier "rollback" test only forward-applied `ha: {}`).
func TestVrrpRollbackRestoresVppAndMeta(t *testing.T) {
	vrrpGates(t, "on", "off", "")
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	metaFile := filepath.Join(dir, "vrrp-meta-"+testOwner+".json")

	// pre-transaction state: one VR (lan-v4) applied, its name in vrrp-meta
	pre := `{"interfaces": {"loop7201": {"ipv4": ["10.7.2.2/24"]}, "loop7202": {"ipv4": ["10.7.3.2/24"]}},
	  "ha": {"vrrp": {"lan-v4": {"interface": "loop7201", "vrId": 10, "addresses": ["10.7.2.1"]}}}}`
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "pre", DesiredState: doc(t, pre)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n, _ := v.Vrrp().Count(); n != 1 {
		t.Fatalf("pre: %d VRs, want 1", n)
	}
	metaBefore, err := os.ReadFile(metaFile) //nolint:gosec // test reads the agent's own state file in a temp dir
	if err != nil || !strings.Contains(string(metaBefore), "lan-v4") {
		t.Fatalf("pre: vrrp-meta must hold lan-v4 (the rollback check is not vacuous): %q %v", metaBefore, err)
	}

	// mid-transaction refusal: the shared VPP rejects the new VR (wan-uni) create
	v.On("vrrp_vr_update", func(req api.Message) ([]api.Message, error) {
		if r := req.(*vrrpapi.VrrpVrUpdate); r.VrrpIndex == ^uint32(0) {
			return []api.Message{&vrrpapi.VrrpVrUpdateReply{Retval: int32(api.INVALID_VALUE)}}, nil
		}
		return []api.Message{&vrrpapi.VrrpVrUpdateReply{}}, nil
	})
	both := `{"interfaces": {"loop7201": {"ipv4": ["10.7.2.2/24"]}, "loop7202": {"ipv4": ["10.7.3.2/24"]}},
	  "ha": {"vrrp": {
	    "lan-v4": {"interface": "loop7201", "vrId": 10, "addresses": ["10.7.2.1"]},
	    "wan-uni": {"interface": "loop7202", "vrId": 11, "addresses": ["10.7.3.1"]}
	  }}}`
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "fail", DesiredState: doc(t, both)}), vrxv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK)

	// VPP is back to exactly the pre-transaction VR (no partial wan-uni), and vrrp-meta is unchanged
	if n, _ := v.Vrrp().Count(); n != 1 {
		t.Fatalf("rollback left %d VRs, want the pre-transaction 1", n)
	}
	if got := vrrpRetrieve(t, s).GetVrrp(); len(got) != 1 || got["lan-v4"] == nil || got["wan-uni"] != nil {
		t.Fatalf("Retrieve after rollback: %v", got)
	}
	if metaAfter, _ := os.ReadFile(metaFile); string(metaAfter) != string(metaBefore) || strings.Contains(string(metaAfter), "wan-uni") { //nolint:gosec // test reads the agent's own state file in a temp dir
		t.Fatalf("vrrp-meta not restored:\nbefore %s\nafter  %s", metaBefore, metaAfter)
	}
}

// TestHaClusterUnsupported (RV-A R3 M1): `ha` is a registered domain, so ha.cluster no longer trips
// agent.unimplemented-domain; desired.Vrrp reports it as agent.unsupported-field instead of applying
// nothing silently.
func TestHaClusterUnsupported(t *testing.T) {
	vrrpGates(t, "", "", "")
	s := newSvc(t, coretest.New(), t.TempDir())
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "hc1",
		DesiredState: doc(t, `{"ha": {"cluster": {"enabled": true, "configSync": true}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, is := range rep.GetErrors() {
		if strings.HasPrefix(is.GetPointer(), "/ha") {
			got = append(got, is.GetSeverity().String()+" "+is.GetPointer()+" "+is.GetRule())
		}
	}
	want := []string{"ISSUE_SEVERITY_WARNING /ha/cluster agent.unsupported-field"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("ha.cluster findings:\n got %v\nwant %v", got, want)
	}
}
