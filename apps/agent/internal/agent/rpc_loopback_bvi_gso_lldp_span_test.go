package agent

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/gso"
	"ngfw/agent/internal/descriptors/lldp"
	"ngfw/agent/internal/descriptors/nsim"
	"ngfw/agent/internal/descriptors/span"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// withoutWriteOnly drops the objects of write-only descriptors (D-063: Retrieve never reports them), so a
// projection round trip compares only what Retrieve can give back (projection_test.go).
func withoutWriteOnly(kvs []scheduler.KV) []scheduler.KV {
	var out []scheduler.KV
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case lldp.NameGlobal, lldp.NameInterface, nsim.ConfigName, nsim.CrossConnectName, nsim.OutputName:
			continue
		}
		out = append(out, kv)
	}
	return out
}

// lbgsDoc: bridge domain "lan" (7101) with loopback BVI loop7101 (GSO on, mirrored to loop7102 both ways at
// the device level and rx in the L2 path), a monitor loopback loop7102, LLDP on loop7101, the nsim model
// with a cross-connect of loop7103/loop7104 and the output feature on loop7102, and a non-empty SNMP
// member no feature implements.
const lbgsDoc = `{
  "system": {"hostname": "vrx-w7"},
  "vrfs": {"default": {"id": 0}},
  "interfaces": {
    "loop7101": {"enabled": true, "vrf": "default", "ipv4": ["10.7.101.1/24"], "gso": true,
      "l2": {"bridgeDomain": "lan", "shg": 0, "bvi": true, "uuFwd": false, "macFilter": false},
      "mirror": [{"destination": "loop7102", "direction": "both", "level": "device"},
                 {"destination": "loop7102", "direction": "rx", "level": "l2"}]},
    "loop7102": {"enabled": true, "vrf": "default"},
    "loop7103": {"enabled": true, "vrf": "default"},
    "loop7104": {"enabled": true, "vrf": "default"}
  },
  "routing": {"l2": {"bridgeDomains": {"lan": {"id": 7101, "flood": true, "uuFlood": true, "forward": true, "learn": true, "arpTerm": false, "macAgeMin": 0}}}},
  "services": {
    "lldp": {"enabled": true, "systemName": "vrx-w7", "txHold": 4, "txIntervalSec": 30, "interfaces": [{"interface": "loop7101", "portDescription": "w7 bvi", "mgmtIpv4": "10.7.101.1"}]},
    "nsim": {"delayMs": 20, "bandwidthMbps": 100, "packetSize": 1500, "dropFraction": 0.01,
      "crossConnect": {"a": "loop7103", "b": "loop7104"}, "outputInterfaces": ["loop7102"]},
    "snmp": {"enabled": true}
  }
}`

// lbgsBase is lbgsDoc after the rollback: the interfaces and the bridge domain stay, every feature leaf goes.
const lbgsBase = `{
  "system": {"hostname": "vrx-w7"},
  "vrfs": {"default": {"id": 0}},
  "interfaces": {
    "loop7101": {"enabled": true, "vrf": "default", "ipv4": ["10.7.101.1/24"],
      "l2": {"bridgeDomain": "lan", "shg": 0, "bvi": true, "uuFwd": false, "macFilter": false}},
    "loop7102": {"enabled": true, "vrf": "default"},
    "loop7103": {"enabled": true, "vrf": "default"},
    "loop7104": {"enabled": true, "vrf": "default"}
  },
  "routing": {"l2": {"bridgeDomains": {"lan": {"id": 7101, "flood": true, "uuFlood": true, "forward": true, "learn": true, "arpTerm": false, "macAgeMin": 0}}}},
  "services": {}
}`

var lbgsDomains = []string{"interfaces", "vrfs", "routing", "services"}

// newSvcOwner is newSvc with D-071's globals-owner flag.
func newSvcOwner(t *testing.T, v *coretest.VPP, dir string, globalsOwner bool) *Service {
	t.Helper()
	owned, err := ownertable.Open(dir, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := subsystems.Register(reg, subsystems.Env{Client: v, Owner: testOwner, StateDir: dir, Owned: owned, NetdevKind: fakeNetdevs, GlobalsOwner: globalsOwner})
	if err != nil {
		t.Fatal(err)
	}
	w.Connected(context.Background())
	sched := scheduler.New(reg, nil)
	sched.VerifyRetries = 0
	svc, err := NewService(ServiceConfig{Owner: testOwner, Version: "test", VPP: v, Scheduler: sched, StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind()})
	if err != nil {
		t.Fatal(err)
	}
	svc.retryMin, svc.retryMax = 1<<62, 1<<62
	t.Cleanup(svc.Close)
	return svc
}

func lbgsRetrieve(t *testing.T, s *Service) *vrxv1.DesiredState {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: lbgsDomains})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState()
}

// checkLbgsApplied asserts the fake VPP state of lbgsDoc and Retrieve's report of it.
func checkLbgsApplied(t *testing.T, v *coretest.VPP, s *Service, owner bool) {
	t.Helper()
	if got := v.GsoCount(); len(got) != 1 || got["loop7101"] != 1 {
		t.Fatalf("gso (stacked enables must be exactly 1): %v", got)
	}
	spans := v.Spans()
	if len(spans) != 2 || spans["loop7101→loop7102/device"] != 3 || spans["loop7101→loop7102/l2"] != 1 {
		t.Fatalf("spans %v", spans)
	}
	if got := v.Lldp(); len(got) != 1 || got["loop7101"] != "w7 bvi" {
		t.Fatalf("lldp %v", got)
	}
	if bm := v.BridgeMembers(); bm["loop7101"] != 7101 {
		t.Fatalf("BVI membership %v", bm)
	}
	n := v.Nsim()
	if owner {
		if !n.Configured || n.Configures != 1 || n.Config.DelayInUsec != 20000 || n.Config.BandwidthInBitsPerSecond != 100000000 || n.Config.PacketsPerDrop != 100 {
			t.Fatalf("nsim model %+v", n)
		}
		if n.CrossCount != 1 || len(n.Output) != 1 {
			t.Fatalf("nsim cross-connect / output %+v", n)
		}
		if g := v.LldpGlobal(); g == nil || g.SystemName != "vrx-w7" || g.TxHold != 4 || g.TxInterval != 30 {
			t.Fatalf("lldp global %+v", g)
		}
	} else if n.Configured || v.LldpGlobal() != nil {
		t.Fatalf("a slot agent set VPP-globals: nsim %+v lldp %+v", n, v.LldpGlobal())
	}
	got := lbgsRetrieve(t, s)
	itf := got.GetInterfaces()["loop7101"]
	if !itf.GetGso() || itf.GetL2().GetBvi() != true || itf.GetL2().GetBridgeDomain() != "lan" {
		t.Fatalf("retrieve loop7101 %s", protojson.Format(itf))
	}
	want := doc(t, lbgsDoc).GetInterfaces()["loop7101"].GetMirror()
	if len(itf.GetMirror()) != 2 || !proto.Equal(itf.GetMirror()[0], want[0]) || !proto.Equal(itf.GetMirror()[1], want[1]) {
		t.Fatalf("retrieve mirror (document order) %v", itf.GetMirror())
	}
	if got.GetServices() == nil || proto.Size(got.GetServices()) != 0 {
		t.Fatalf("services must be present and empty (write-only): %v", got.GetServices())
	}
	// Retrieve reproduces every retrievable object of the document
	if a, b := project(got, lbgsDomains, nil, nil), project(doc(t, lbgsDoc), lbgsDomains, nil, nil); !sameKVs(withoutWriteOnly(b.kvs), withoutWriteOnly(a.kvs)) {
		t.Fatalf("project(Retrieve) != project(desired) for the retrievable objects")
	}
}

func issueRules(r *vrxv1.ValidationReport) map[string]string {
	out := map[string]string{}
	for _, i := range r.GetErrors() { // every finding, ERROR first
		out[i.GetPointer()] = i.GetRule()
	}
	return out
}

func hasErrors(r *vrxv1.ValidationReport) bool {
	for _, i := range r.GetErrors() {
		if i.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
			return true
		}
	}
	return false
}

func TestLoopbackBviGsoLldpSpanSlotAgent(t *testing.T) {
	fakeBootIdentity(t)
	v := coretest.New()
	dir := t.TempDir()
	s := newSvcOwner(t, v, dir, false)
	ds := doc(t, lbgsDoc)

	// DryRun notes: write-only LLDP, VPP-global LLDP fields and nsim unsupported on a slot agent, snmp unsupported
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: ds, Subsystems: lbgsDomains})
	if err != nil {
		t.Fatal(err)
	}
	rules := issueRules(rep)
	for p, want := range map[string]string{
		"/services/lldp":               "agent.write-only",
		"/services/lldp/systemName":    "agent.unsupported-field",
		"/services/lldp/txHold":        "agent.unsupported-field",
		"/services/lldp/txIntervalSec": "agent.unsupported-field",
		"/services/nsim":               "agent.unsupported-field",
		"/services/snmp":               "agent.unsupported-field",
	} {
		if rules[p] != want {
			t.Errorf("DryRun %s: rule %q, want %q (all: %v)", p, rules[p], want, rules)
		}
	}
	if hasErrors(rep) || !rep.GetOk() {
		t.Fatalf("errors %v", rep.GetErrors())
	}

	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "a1", DesiredState: ds, Subsystems: lbgsDomains})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	checkLbgsApplied(t, v, s, false)

	// idempotent: the same document again sends nothing that changes VPP
	v.Reset()
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "a2", DesiredState: doc(t, lbgsDoc), Subsystems: lbgsDomains}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	for _, c := range v.Calls() {
		if n := c.GetMessageName(); !strings.HasSuffix(n, "_dump") && n != "control_ping" && n != "sw_interface_get_table" && !strings.HasSuffix(n, "_get") && n != "feature_is_enabled" {
			t.Fatalf("idempotent apply sent %s", n)
		}
	}

	// resync without loss: write-only lldp.interface is re-applied (adopted: already enabled), GSO is not stacked twice
	mustStatus(t, s.Resync(context.Background()), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	checkLbgsApplied(t, v, s, false)

	// restart simulation (D-095c): the agent stops, the dependents and then the interfaces are deleted behind its
	// back, a new agent on the same state dir resyncs from its stored desired state
	s.Close()
	v.ClearLoopbackBviGsoLldpSpan()
	for _, n := range []string{"loop7101", "loop7102", "loop7103", "loop7104"} {
		v.DeleteInterface(n)
	}
	s2 := newSvcOwner(t, v, dir, false)
	r := s2.Resync(context.Background())
	mustStatus(t, r, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if r.GetSummary().GetCreated() == 0 {
		t.Fatalf("resync created nothing: %v", r)
	}
	checkLbgsApplied(t, v, s2, false)

	// rollback: the dependents go before the loopbacks' leaves; nothing of the feature remains
	mustStatus(t, apply(t, s2, &vrxv1.ApplyRequest{TxnId: "rb", DesiredState: doc(t, lbgsBase), Subsystems: lbgsDomains}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if g, sp, l := v.GsoCount(), v.Spans(), v.Lldp(); len(g)+len(sp)+len(l) != 0 {
		t.Fatalf("after rollback gso %v span %v lldp %v", g, sp, l)
	}
	got := lbgsRetrieve(t, s2)
	for name, itf := range got.GetInterfaces() {
		if itf.Gso != nil || len(itf.GetMirror()) > 0 {
			t.Fatalf("after rollback %s: %s", name, protojson.Format(itf))
		}
	}
	// and the loopbacks themselves (P08's creator) on the full rollback
	mustStatus(t, apply(t, s2, &vrxv1.ApplyRequest{TxnId: "rb2", DesiredState: doc(t, `{"vrfs": {"default": {"id": 0}}}`), Subsystems: lbgsDomains}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	for _, i := range v.Ifaces {
		if strings.HasPrefix(i.Name, "loop71") {
			t.Fatalf("loopback %s left after the rollback", i.Name)
		}
	}
}

func TestLoopbackBviGsoLldpSpanGlobalsOwner(t *testing.T) {
	fakeBootIdentity(t)
	t.Setenv("VRX_NSIM", "lab") // review M2: nsim needs the lab gate on top of the globals owner
	v := coretest.New()
	dir := t.TempDir()
	s := newSvcOwner(t, v, dir, true)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "a1", DesiredState: doc(t, lbgsDoc), Subsystems: lbgsDomains})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	checkLbgsApplied(t, v, s, true)
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: doc(t, lbgsDoc), Subsystems: lbgsDomains})
	if err != nil {
		t.Fatal(err)
	}
	if r := issueRules(rep); r["/services/nsim"] != "agent.write-only" || r["/services/lldp/txHold"] != "" {
		t.Fatalf("owner notes %v", r)
	}

	// resyncs (write-only objects re-applied) never reconfigure the model or stack the features
	for i := 0; i < 2; i++ {
		mustStatus(t, s.Resync(context.Background()), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	}
	checkLbgsApplied(t, v, s, true)

	// a changed model is applied once more (VPP reallocates); an unchanged one never
	changed := doc(t, lbgsDoc)
	changed.Services.Nsim.DelayMs = proto.Float64(30)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "a2", DesiredState: changed, Subsystems: lbgsDomains}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n := v.Nsim(); n.Configures != 2 || n.Config.DelayInUsec != 30000 {
		t.Fatalf("nsim after change %+v", n)
	}

	// rollback: cross-connect and output feature removed; VPP cannot unconfigure the model (it stays, inert)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "rb", DesiredState: doc(t, lbgsBase), Subsystems: lbgsDomains}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n := v.Nsim(); n.CrossCount != 0 || len(n.Output) != 0 {
		t.Fatalf("nsim after rollback %+v", n)
	}
	if g, sp, l := v.GsoCount(), v.Spans(), v.Lldp(); len(g)+len(sp)+len(l) != 0 {
		t.Fatalf("after rollback gso %v span %v lldp %v", g, sp, l)
	}
}

func TestLoopbackBviGsoLldpSpanValidation(t *testing.T) {
	v := coretest.New()
	s := newSvcOwner(t, v, t.TempDir(), false)
	bad := doc(t, `{"interfaces": {"loop7101": {"mirror": [{"destination": "loop7101"}, {"destination": "loop7102", "direction": "sideways"}]}, "loop7102": {}},
	  "services": {"lldp": {"enabled": true, "interfaces": [{"interface": "loop7102", "mgmtIpv6": "10.0.0.1"}]}}}`)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "bad", DesiredState: bad, Subsystems: lbgsDomains})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_FAILED)
	got := map[string]bool{}
	for _, i := range resp.GetValidation().GetErrors() {
		got[i.GetPointer()] = true
	}
	for _, p := range []string{"/interfaces/loop7101/mirror/0/destination", "/interfaces/loop7101/mirror/1/direction", "/services/lldp/interfaces/0/mgmtIpv6"} {
		if !got[p] {
			t.Errorf("no error at %s: %v", p, resp.GetValidation().GetErrors())
		}
	}
	if len(v.Spans())+len(v.Lldp()) != 0 {
		t.Fatal("a failed validation applied something")
	}
}

func TestLldpNeighbors(t *testing.T) {
	fakeBootIdentity(t)
	v := coretest.New()
	s := newSvcOwner(t, v, t.TempDir(), false)
	prev := vppClock
	vppClock = func(context.Context, *Service) (float64, bool) { return 100, true }
	t.Cleanup(func() { vppClock = prev })
	ds := doc(t, `{"interfaces": {"loop7101": {}, "loop7102": {}, "loop7103": {}},
	  "services": {"lldp": {"enabled": true, "interfaces": [{"interface": "loop7103"}, {"interface": "loop7101"}, {"interface": "loop7102"}]}}}`)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "a", DesiredState: ds, Subsystems: lbgsDomains}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	// another owner's interface with LLDP (made behind the agent's back) is never reported
	foreign := v.AddInterface("loop9999", "Loopback", "w9:loop9999")
	v.EnableLldp(foreign)

	r, err := s.LldpNeighbors(context.Background(), &vrxv1.LldpNeighborsRequest{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if r.GetTotal() != 3 || len(r.GetNeighbors()) != 2 || r.GetNeighbors()[0].GetInterface() != "loop7101" || r.GetNeighbors()[1].GetInterface() != "loop7102" {
		t.Fatalf("page 1 %v", r)
	}
	if n := r.GetNeighbors()[0]; n.GetHeard() || n.GetChassisId() != "" || n.GetLastSentSecAgo() != 90 || n.GetLastHeardSecAgo() != 0 {
		t.Fatalf("neighbour fields %v", n)
	}
	r, err = s.LldpNeighbors(context.Background(), &vrxv1.LldpNeighborsRequest{Offset: 2, Limit: 2})
	if err != nil || len(r.GetNeighbors()) != 1 || r.GetNeighbors()[0].GetInterface() != "loop7103" {
		t.Fatalf("page 2 %v %v", r, err)
	}
	if _, err := s.LldpNeighbors(context.Background(), &vrxv1.LldpNeighborsRequest{Limit: 1001}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("limit 1001: %v", err)
	}
	if _, err := s.LldpNeighbors(context.Background(), &vrxv1.LldpNeighborsRequest{Owner: "w9"}); grpcCode(err) == codes.OK {
		t.Fatal("foreign owner accepted")
	}
	v.SetConnected(false)
	if _, err := s.LldpNeighbors(context.Background(), &vrxv1.LldpNeighborsRequest{}); grpcCode(err) != codes.Unavailable {
		t.Fatalf("disconnected: %v", err)
	}
}

func TestLldpIDFormat(t *testing.T) {
	for _, c := range []struct {
		b          []byte
		mac, isNet bool
		want       string
	}{
		{[]byte{0x02, 0, 0, 0, 0x70, 1}, true, false, "02:00:00:00:70:01"},
		{[]byte{1, 10, 7, 0, 1}, false, true, "10.7.0.1"},
		{[]byte("Ethernet1/1"), false, false, "Ethernet1/1"},
		{[]byte{0, 1, 0xff}, false, false, "00:01:ff"},
		{nil, true, false, ""},
	} {
		if got := lldpID(c.b, c.mac, c.isNet); got != c.want {
			t.Errorf("lldpID(%v) = %q, want %q", c.b, got, c.want)
		}
	}
	if ago(100, true, 40) != 60 || ago(100, true, 0) != 0 || ago(100, false, 40) != 0 {
		t.Fatal("ago")
	}
	_ = gso.Name
	_ = span.NameMirror
}
