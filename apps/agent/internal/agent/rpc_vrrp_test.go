package agent

// F-vrrp-config-sync: ha.vrrp (engine vpp) through the agent against the unit-test VPP model
// (coretest/vrrp.go): apply → Retrieve == desired, idempotent re-apply, agent restart (names kept), a VR
// lost behind the agent's back re-created, disable = stopped, rollback removes the VRs; the keepalived
// engine without a linux-cp pair is skipped with a warning; duplicate (interface, family, VRID) refused.

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
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

func vrrpRetrieve(t *testing.T, s *Service) *ngfwv1.HaConfig {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &ngfwv1.RetrieveRequest{Subsystems: []string{"interfaces", "ha"}})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState().GetHa()
}

func TestVrrpApplyRetrieveRestartRollback(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)

	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "v1", DesiredState: doc(t, vrrpDoc)}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n, run := v.Vrrp().Count(); n != 2 || run != 2 {
		t.Fatalf("VPP holds %d VRs (%d running), want 2 (2)", n, run)
	}
	want := &ngfwv1.HaConfig{}
	if err := protojson.Unmarshal([]byte(vrrpRetrieved), want); err != nil {
		t.Fatal(err)
	}
	if got := vrrpRetrieve(t, s); !proto.Equal(got, want) {
		t.Fatalf("Retrieve ha:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	if r := apply(t, s, &ngfwv1.ApplyRequest{TxnId: "v2", DesiredState: doc(t, vrrpDoc)}); changes(r) != 0 {
		t.Fatalf("re-apply changed %d objects", changes(r))
	}

	// agent restart (same state dir, same VPP): nothing to do, names kept
	s.Close()
	s2 := newSvc(t, v, dir)
	if r := apply(t, s2, &ngfwv1.ApplyRequest{TxnId: "v3", DesiredState: doc(t, vrrpDoc)}); changes(r) != 0 {
		t.Fatalf("apply after restart changed %d objects: %s", changes(r), protojson.Format(r))
	}
	if got := vrrpRetrieve(t, s2); !proto.Equal(got, want) {
		t.Fatalf("Retrieve after restart: %s", protojson.Format(got))
	}

	// VRs lost behind the agent's back (a VPP restart) are re-created and started
	v.Vrrp().Forget()
	mustStatus(t, apply(t, s2, &ngfwv1.ApplyRequest{TxnId: "v4", DesiredState: doc(t, vrrpDoc)}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n, run := v.Vrrp().Count(); n != 2 || run != 2 {
		t.Fatalf("after loss: %d VRs (%d running), want 2 (2)", n, run)
	}

	// disabled = configured but stopped
	disabled := strings.Replace(vrrpDoc, `"lan-v4": {`, `"lan-v4": {"enabled": false, `, 1)
	mustStatus(t, apply(t, s2, &ngfwv1.ApplyRequest{TxnId: "v5", DesiredState: doc(t, disabled)}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n, run := v.Vrrp().Count(); n != 2 || run != 1 {
		t.Fatalf("disabled: %d VRs (%d running), want 2 (1)", n, run)
	}
	if got := vrrpRetrieve(t, s2).GetVrrp()["lan-v4"]; got.GetEnabled() {
		t.Fatalf("disabled VR retrieved as enabled: %s", protojson.Format(got))
	}

	// rollback to no VRs: every VR goes, the interfaces stay
	mustStatus(t, apply(t, s2, &ngfwv1.ApplyRequest{TxnId: "v6", DesiredState: doc(t, `{"interfaces": {"loop7201": {"ipv4": ["10.7.2.2/24"]}, "loop7202": {"ipv4": ["10.7.3.2/24"]}}, "ha": {}}`)}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n, _ := v.Vrrp().Count(); n != 0 {
		t.Fatalf("rollback left %d VRs", n)
	}
	if got := vrrpRetrieve(t, s2); len(got.GetVrrp()) != 0 {
		t.Fatalf("Retrieve after rollback: %s", protojson.Format(got))
	}
}

func TestVrrpDryRunFindings(t *testing.T) {
	s := newSvc(t, coretest.New(), t.TempDir())
	rep, err := s.DryRun(context.Background(), &ngfwv1.DryRunRequest{TxnId: "d1", DesiredState: doc(t, `{
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
