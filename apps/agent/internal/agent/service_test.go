package agent

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
)

const testOwner = "w7"

// fakeNetdevs stands in for the host's Linux netdevs (D-105 veth rule): every name is a veth except
// ens192 (a physical NIC), br-lab (a bridge) and gone0 (missing).
func fakeNetdevs(name string) (string, bool, error) {
	switch name {
	case "ens192":
		return "", true, nil
	case "br-lab":
		return "bridge", true, nil
	case "gone0":
		return "", false, nil
	}
	return "veth", true, nil
}

// newSvc builds a service over the fake VPP v with its state in dir.
func newSvc(t *testing.T, v *coretest.VPP, dir string) *Service {
	t.Helper()
	owned, err := ownertable.Open(dir, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := subsystems.Register(reg, subsystems.Env{Client: v, Owner: testOwner, StateDir: dir, Owned: owned, NetdevKind: fakeNetdevs})
	if err != nil {
		t.Fatal(err)
	}
	w.Connected(context.Background()) // boot identity for the claim stores (P08)
	sched := scheduler.New(reg, nil)
	sched.VerifyRetries = 0
	svc, err := NewService(ServiceConfig{Owner: testOwner, Version: "test", VPP: v, Scheduler: sched, StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind()})
	if err != nil {
		t.Fatal(err)
	}
	svc.retryMin, svc.retryMax = time.Hour, time.Hour // tests drive retries explicitly
	t.Cleanup(svc.Close)
	return svc
}

// doc parses a configuration document (protobuf JSON = the API's JSON, contract §1).
func doc(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatalf("doc: %v", err)
	}
	return ds
}

const sampleDoc = `{
  "system": {"hostname": "vrx-a"},
  "vrfs": {"default": {"id": 0}, "red": {"id": 7001}},
  "interfaces": {
    "loop701": {"vrf": "red", "ipv4": ["10.7.1.1/24"], "ipv6": ["2001:db8:7::1/64"]},
    "loop702": {"vrf": "red", "ipv4": ["10.7.2.1/24"]}
  },
  "routing": {"static": [
    {"prefix": "10.7.100.0/24", "vrf": "red", "distance": 1, "nextHops": [{"address": "10.7.1.254", "weight": 1}]},
    {"prefix": "10.7.200.0/24", "vrf": "default", "nextHops": [{"interface": "loop701", "weight": 1}]}
  ]}
}`

// canonical is what Retrieve must return for sampleDoc (owned objects only; default VRF not
// owned; distance only when set).
const canonicalDoc = `{
  "vrfs": {"red": {"id": 7001}},
  "interfaces": {
    "loop701": {"enabled": false, "promiscuous": false, "vrf": "red", "ipv4": ["10.7.1.1/24"], "ipv6": ["2001:db8:7::1/64"]},
    "loop702": {"enabled": false, "promiscuous": false, "vrf": "red", "ipv4": ["10.7.2.1/24"]}
  },
  "routing": {"static": [
    {"prefix": "10.7.200.0/24", "vrf": "default", "blackhole": false, "nextHops": [{"interface": "loop701", "weight": 1}]},
    {"prefix": "10.7.100.0/24", "vrf": "red", "distance": 1, "blackhole": false, "nextHops": [{"address": "10.7.1.254", "weight": 1}]}
  ]}
}`

func apply(t *testing.T, s *Service, req *vrxv1.ApplyRequest) *vrxv1.ApplyResponse {
	t.Helper()
	resp, err := s.Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("apply %s: %v", req.GetTxnId(), err)
	}
	return resp
}

func mustStatus(t *testing.T, resp *vrxv1.ApplyResponse, want vrxv1.ApplyStatus) {
	t.Helper()
	if resp.GetStatus() != want {
		t.Fatalf("status %s, want %s: %s results=%v validation=%v", resp.GetStatus(), want, resp.GetMessage(), resp.GetResults(), resp.GetValidation())
	}
}

func grpcCode(err error) codes.Code { return status.Code(err) }

func TestApplyRetrieveIdempotent(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if resp.GetSummary().GetCreated() != 12 || len(resp.GetResults()) != 12 { // P08: + interface/loop701, interface/loop702 aliases
		t.Fatalf("summary %v results %d", resp.GetSummary(), len(resp.GetResults()))
	}
	for _, r := range resp.GetResults() {
		if r.GetCode() != vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_OK || r.GetPointer() == "" || r.GetSubsystem() == "" {
			t.Fatalf("result %v", r)
		}
	}
	// Retrieve == canonical desired.
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if want := doc(t, canonicalDoc); !proto.Equal(got.GetDesiredState(), want) {
		t.Fatalf("retrieve:\n got %s\nwant %s", protojson.Format(got.GetDesiredState()), protojson.Format(want))
	}
	if strings.Join(got.GetSubsystems(), ",") != strings.Join(implementedDomains(), ",") || got.GetOwner() != testOwner { // every implemented domain (wave A adds some)
		t.Fatalf("retrieve meta %v %s", got.GetSubsystems(), got.GetOwner())
	}
	// Idempotent: same state, new txn → empty plan, no results.
	v.Reset()
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: doc(t, sampleDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(resp.GetResults()) != 0 || resp.GetSummary().GetUnchanged() != 12 {
		t.Fatalf("second apply %v", resp)
	}
	for _, c := range v.Calls() {
		if n := c.GetMessageName(); !strings.HasSuffix(n, "_dump") && n != "control_ping" && n != "sw_interface_get_table" {
			t.Fatalf("idempotent apply sent %s", n)
		}
	}
	h := s.Health()
	if h.GetLastTxnId() != "t2" || h.GetDegraded() || h.GetOwner() != testOwner || h.GetLastReconcileAt() == nil {
		t.Fatalf("health %v", h)
	}
}

func TestApplyRequestValidation(t *testing.T) {
	s := newSvc(t, coretest.New(), t.TempDir())
	ctx := context.Background()
	cases := []struct {
		req  *vrxv1.ApplyRequest
		code codes.Code
	}{
		{&vrxv1.ApplyRequest{}, codes.InvalidArgument},
		{&vrxv1.ApplyRequest{DesiredState: &vrxv1.DesiredState{}}, codes.InvalidArgument},
		{&vrxv1.ApplyRequest{TxnId: "x", Owner: "w3"}, codes.InvalidArgument},
		{&vrxv1.ApplyRequest{TxnId: "x", Subsystems: []string{"bogus"}}, codes.InvalidArgument},
		{&vrxv1.ApplyRequest{TxnId: "x", Subsystems: []string{"routing.bgp"}}, codes.InvalidArgument},
		{&vrxv1.ApplyRequest{TxnId: "x", Subsystems: []string{"nat"}}, codes.Unimplemented},
		{&vrxv1.ApplyRequest{ConfirmTxnId: "nope"}, codes.FailedPrecondition},
		{&vrxv1.ApplyRequest{TxnId: "same", ConfirmTxnId: "same"}, codes.InvalidArgument},
	}
	for i, c := range cases {
		if _, err := s.Apply(ctx, c.req); grpcCode(err) != c.code {
			t.Errorf("case %d: %v, want %s", i, err, c.code)
		}
	}
	// Owner equal to ours is fine.
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "ok", Owner: testOwner, DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	// VPP down → UNAVAILABLE.
	v := coretest.New()
	s2 := newSvc(t, v, t.TempDir())
	v.SetConnected(false)
	if _, err := s2.Apply(ctx, &vrxv1.ApplyRequest{TxnId: "d", DesiredState: doc(t, sampleDoc)}); grpcCode(err) != codes.Unavailable {
		t.Fatalf("disconnected: %v", err)
	}
	if _, err := s2.Retrieve(ctx, &vrxv1.RetrieveRequest{}); grpcCode(err) != codes.Unavailable {
		t.Fatalf("disconnected retrieve: %v", err)
	}
}

func TestTxnIDRetryIdempotency(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	first := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)})
	v.DeleteInterface("loop702") // would be repaired by a real re-apply
	again := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)})
	if !proto.Equal(first, again) {
		t.Fatalf("retry returned a different response")
	}
	if _, ok := v.InterfaceByName("loop702"); ok {
		t.Fatal("retry touched the data plane")
	}
	if _, err := s.Apply(context.Background(), &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, `{"vrfs":{}}`)}); grpcCode(err) != codes.Aborted {
		t.Fatalf("reuse with different content: %v", err)
	}
}

func TestDomainPresence(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	before := v.Snapshot()
	// A partial document (only system) never wipes interfaces/vrfs/routing.
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: doc(t, `{"system":{"hostname":"x"}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if v.Snapshot() != before {
		t.Fatal("partial document changed the data plane")
	}
	// routing present but empty → authoritative: every owned route deleted.
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "t3", DesiredState: doc(t, `{"routing":{}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if resp.GetSummary().GetDeleted() != 2 || v.RouteCount() != 0 {
		t.Fatalf("routing {} → %v routes=%d", resp.GetSummary(), v.RouteCount())
	}
	// vrfs named in subsystems while interfaces still use the VRF → dependency error, nothing changed.
	before = v.Snapshot()
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "t4", Subsystems: []string{"vrfs"}})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_FAILED)
	if resp.GetValidation().GetOk() || len(resp.GetValidation().GetErrors()) == 0 || v.Snapshot() != before {
		t.Fatalf("expected a dependency failure: %v", resp)
	}
	// subsystems narrow: interfaces+vrfs named with an empty document → everything owned goes.
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "t5", Subsystems: []string{"interfaces", "vrfs"}})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if kvs, _ := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{}); len(kvs.GetDesiredState().GetInterfaces())+len(kvs.GetDesiredState().GetVrfs()) != 0 {
		t.Fatalf("left over %v", kvs)
	}
}

func TestValidationFailure(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "bad", DesiredState: doc(t, `{
	  "interfaces": {"loop701": {"vrf": "nope", "ipv4": ["10.7.1.1/33"]}},
	  "routing": {"static": [{"prefix": "10.7.9.0/24", "vrf": "missing", "nextHops": [{"address": "10.7.1.1"}]}]}
	}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_FAILED)
	ptrs := map[string]string{}
	for _, e := range resp.GetValidation().GetErrors() {
		ptrs[e.GetPointer()] = e.GetRule()
	}
	for p, rule := range map[string]string{
		"/interfaces/loop701/vrf":    "interfaces.vrf-exists",
		"/interfaces/loop701/ipv4/0": "interfaces.address",
		"/routing/static/0/vrf":      "routing.static.vrf-exists",
	} {
		if ptrs[p] != rule {
			t.Errorf("pointer %s: rule %q, want %q (all: %v)", p, ptrs[p], rule, ptrs)
		}
	}
	if len(v.CallsNamed("create_loopback_instance")) != 0 {
		t.Fatal("validation failure touched VPP")
	}
}

func TestRollbackReported(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	before := v.Snapshot()
	// loop703 gets an address overlapping loop701's subnet in the same table → VPP refuses.
	bad := doc(t, sampleDoc)
	bad.Interfaces["loop703"] = &vrxv1.Interface{Vrf: proto.String("red"), Ipv4: []string{"10.7.1.2/24"}}
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: bad})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK)
	var failed, reverted int
	for _, r := range resp.GetResults() {
		switch r.GetCode() {
		case vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_FAILED:
			failed++
			if r.GetPointer() != "/interfaces/loop703/ipv4/0" || r.GetSubsystem() != "interfaces" || !strings.Contains(r.GetMessage(), "sw_interface_add_del_address") {
				t.Fatalf("failed result %v", r)
			}
		case vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_REVERTED:
			reverted++
		}
	}
	if failed != 1 || reverted != 3 || resp.GetSummary().GetReverted() != 3 { // P08: + the loop703 alias
		t.Fatalf("failed=%d reverted=%d %v", failed, reverted, resp.GetResults())
	}
	if v.Snapshot() != before {
		t.Fatal("rollback did not restore the previous state")
	}
	if s.Health().GetLastTxnId() != "t1" {
		t.Fatal("a rolled back txn became the last txn")
	}
}

func collect(t *testing.T, sub *subscriber, n int) []*vrxv1.Event {
	t.Helper()
	var out []*vrxv1.Event
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for len(out) < n {
		evs, err := sub.next(ctx)
		if err != nil {
			t.Fatalf("waiting for %d events, got %v", n, out)
		}
		out = append(out, evs...)
	}
	return out
}

func kinds(evs []*vrxv1.Event) string {
	var k []string
	for _, e := range evs {
		k = append(k, strings.TrimPrefix(e.GetKind().String(), "EVENT_KIND_")+":"+e.GetTxnId())
	}
	return strings.Join(k, ",")
}

func TestConfirmFlow(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "base", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, sampleDoc), ConfirmTimeoutSec: 60})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if resp.GetConfirmDeadline() == nil {
		t.Fatal("no confirm deadline")
	}
	h := s.Health()
	if h.GetPendingConfirmTxnId() != "p1" || h.GetConfirmDeadline() == nil || h.GetLastTxnId() != "base" {
		t.Fatalf("health %v", h)
	}
	// A new apply without confirming → FAILED_PRECONDITION.
	if _, err := s.Apply(context.Background(), &vrxv1.ApplyRequest{TxnId: "p2", DesiredState: doc(t, sampleDoc)}); grpcCode(err) != codes.FailedPrecondition {
		t.Fatalf("apply while pending: %v", err)
	}
	// Confirm.
	c := apply(t, s, &vrxv1.ApplyRequest{ConfirmTxnId: "p1"})
	mustStatus(t, c, vrxv1.ApplyStatus_APPLY_STATUS_CONFIRMED)
	if h := s.Health(); h.GetPendingConfirmTxnId() != "" || h.GetLastTxnId() != "p1" {
		t.Fatalf("health after confirm %v", h)
	}
	if _, err := s.Apply(context.Background(), &vrxv1.ApplyRequest{ConfirmTxnId: "p1"}); grpcCode(err) != codes.FailedPrecondition {
		t.Fatalf("second confirm: %v", err)
	}
	// Confirm-and-apply in one call.
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p3", DesiredState: doc(t, `{"routing":{}}`), ConfirmTimeoutSec: 60}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "p4", ConfirmTxnId: "p3", DesiredState: doc(t, `{"system":{}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if h := s.Health(); h.GetLastTxnId() != "p4" || h.GetPendingConfirmTxnId() != "" {
		t.Fatalf("health after confirm-and-apply %v", h)
	}
}

func TestConfirmTimeoutReverts(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "base", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	baseline := v.Snapshot()
	sub := s.events().subscribe(&vrxv1.StreamEventsRequest{})
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, sampleDoc), ConfirmTimeoutSec: 1}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, ok := v.InterfaceByName("loop701"); !ok {
		t.Fatal("pending txn not applied")
	}
	evs := collect(t, sub, 5)
	if got := kinds(evs); got != "RECONCILE_START:p1,RECONCILE_DONE:p1,CONFIRM_REVERTED:p1,RECONCILE_START:,RECONCILE_DONE:" {
		t.Fatalf("events %s", got)
	}
	for i, e := range evs {
		if e.GetSeq() != uint64(i+1) {
			t.Fatalf("seq %d at %d", e.GetSeq(), i)
		}
	}
	if evs[4].GetMessage() != "APPLY_STATUS_APPLIED" || evs[4].GetSummary().GetDeleted() == 0 {
		t.Fatalf("revert done %v", evs[4])
	}
	if v.Snapshot() != baseline {
		t.Fatalf("not reverted:\n%s\n%s", baseline, v.Snapshot())
	}
	if _, err := s.Apply(context.Background(), &vrxv1.ApplyRequest{ConfirmTxnId: "p1"}); grpcCode(err) != codes.FailedPrecondition {
		t.Fatalf("confirm after revert: %v", err)
	}
	if h := s.Health(); h.GetPendingConfirmTxnId() != "" || h.GetLastTxnId() != "base" {
		t.Fatalf("health %v", h)
	}
}

func TestPendingSurvivesRestartAndRevertsAfterDeadline(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, sampleDoc), ConfirmTimeoutSec: 1}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	s.Close() // agent stops before the deadline (timer gone with the process)
	time.Sleep(1100 * time.Millisecond)
	s2 := newSvc(t, v, dir)
	if s2.Health().GetPendingConfirmTxnId() != "p1" {
		t.Fatal("pending txn not persisted")
	}
	s2.Resync(context.Background())
	if _, ok := v.InterfaceByName("loop701"); ok {
		t.Fatal("expired pending txn not reverted after restart")
	}
	if h := s2.Health(); h.GetPendingConfirmTxnId() != "" {
		t.Fatalf("health %v", h)
	}
}

func TestResyncRecreatesAfterLoss(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	want := v.Snapshot()
	s.Close()
	// Simulated loss while the agent is down: loopbacks and the VRF deleted behind its back.
	v.DeleteInterface("loop701")
	v.DeleteInterface("loop702")
	v.DeleteTable(7001, false)
	v.DeleteTable(7001, true)
	s2 := newSvc(t, v, dir)
	sub := s2.events().subscribe(&vrxv1.StreamEventsRequest{Kinds: []vrxv1.EventKind{vrxv1.EventKind_EVENT_KIND_RECONCILE_DONE}})
	resp := s2.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if resp.GetSummary().GetCreated() == 0 {
		t.Fatalf("resync created nothing: %v", resp)
	}
	got, err := s2.Retrieve(context.Background(), &vrxv1.RetrieveRequest{})
	if err != nil || !proto.Equal(got.GetDesiredState(), doc(t, canonicalDoc)) {
		t.Fatalf("after resync: %v %s", err, protojson.Format(got.GetDesiredState()))
	}
	evs := collect(t, sub, 1)
	if evs[0].GetTxnId() != "" || evs[0].GetSummary().GetCreated() != resp.GetSummary().GetCreated() {
		t.Fatalf("RECONCILE_DONE %v", evs[0])
	}
	// A second restart with nothing lost changes nothing (kill -9 acceptance).
	_ = want
	v.Reset()
	s3 := newSvc(t, v, dir)
	resp = s3.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if resp.GetSummary().GetCreated()+resp.GetSummary().GetUpdated()+resp.GetSummary().GetDeleted() != 0 {
		t.Fatalf("converged resync changed something: %v", resp.GetSummary())
	}
	for _, c := range v.Calls() {
		n := c.GetMessageName()
		if t4, ok := c.(*ip.IPTableAddDel); ok && t4.IsAdd {
			continue // resync re-asserts the VRF's API lock (idempotent, scheduler.Reapplier)
		}
		if !strings.HasSuffix(n, "_dump") && n != "control_ping" && n != "sw_interface_get_table" {
			t.Fatalf("converged resync sent %s", n)
		}
	}
}

func TestResyncDeletesOwnedLeftovers(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, `{"interfaces":{"loop701":{}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	// Drift while down: an extra owned loopback and a foreign one.
	idx := v.AddInterface("loop709", "Loopback", testOwner+":loop709")
	v.AddInterface("loop309", "Loopback", "w3:loop309")
	_ = idx
	s2 := newSvc(t, v, dir)
	resp := s2.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, ok := v.InterfaceByName("loop709"); ok {
		t.Fatal("owned leftover not deleted")
	}
	if _, ok := v.InterfaceByName("loop309"); !ok {
		t.Fatal("foreign interface deleted")
	}
}

func TestDryRun(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	sub := s.events().subscribe(&vrxv1.StreamEventsRequest{})
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: doc(t, `{
	  "vrfs": {"red": {"id": 7001, "description": "x"}},
	  "interfaces": {"loop701": {"vrf": "red", "mtu": 1500, "ipv4": ["10.7.1.1/24"]}}
	}`)})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.GetOk() || len(rep.GetPlan()) != 6 || rep.GetSummary().GetCreated() != 6 { // P08: + alias, interface.mtu
		t.Fatalf("report %v", rep)
	}
	if rep.GetPlan()[0].GetKey() != "vrf/7001" || rep.GetPlan()[0].GetOp() != vrxv1.ApplyOperation_APPLY_OPERATION_CREATE || rep.GetPlan()[0].GetCode() != vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_UNSPECIFIED {
		t.Fatalf("plan[0] %v", rep.GetPlan()[0])
	}
	var warn int
	for _, e := range rep.GetErrors() {
		if e.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_WARNING && e.GetRule() == "agent.unsupported-field" {
			warn++
		}
	}
	if warn != 0 { // P08 implements interfaces.mtu; vrfs.description is kept by the agent (D-073b)
		t.Fatalf("warnings %v", rep.GetErrors())
	}
	for _, c := range v.Calls() {
		if n := c.GetMessageName(); !strings.HasSuffix(n, "_dump") && n != "control_ping" && n != "sw_interface_get_table" {
			t.Fatalf("dry run sent %s", n)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if evs, err := sub.next(ctx); err == nil {
		t.Fatalf("dry run emitted events %v", evs)
	}
	// Invalid document → ok=false with pointers, no plan.
	rep, err = s.DryRun(context.Background(), &vrxv1.DryRunRequest{DesiredState: doc(t, `{"interfaces":{"loop701":{"vrf":"nope"}}}`)})
	if err != nil || rep.GetOk() || len(rep.GetPlan()) != 0 || rep.GetErrors()[0].GetPointer() != "/interfaces/loop701/vrf" {
		t.Fatalf("invalid dry run %v %v", err, rep)
	}
	if _, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{Owner: "w1"}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("owner: %v", err)
	}
}

// M4: an owned interface in the default table is reported with vrf "default" (the Zod default).
func TestDefaultVRFNoDrift(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	d := `{"interfaces":{"loop705":{"vrf":"default","ipv4":["10.7.5.1/24"]},"loop706":{}}}`
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "d1", DesiredState: doc(t, d)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces"}})
	want := doc(t, `{"interfaces":{"loop705":{"enabled":false,"promiscuous":false,"vrf":"default","ipv4":["10.7.5.1/24"]},"loop706":{"enabled":false,"promiscuous":false,"vrf":"default"}}}`)
	if err != nil || !proto.Equal(got.GetDesiredState(), want) {
		t.Fatalf("retrieve %v %s", err, protojson.Format(got.GetDesiredState()))
	}
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "d2", DesiredState: want})
	if len(resp.GetResults()) != 0 {
		t.Fatalf("re-applying the retrieved document changed something: %v", resp.GetResults())
	}
}

// D-073b: descriptions are returned from the agent's stored state for existing objects.
func TestDescriptionsRoundTrip(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	d := `{"vrfs":{"red":{"id":7001,"description":"customer red"}},"routing":{"static":[{"prefix":"10.7.66.0/24","vrf":"red","distance":1,"blackhole":true,"description":"sink"}]}}`
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "d1", DesiredState: doc(t, d)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"vrfs", "routing"}})
	if err != nil || !proto.Equal(got.GetDesiredState(), doc(t, d)) {
		t.Fatalf("retrieve %v %s", err, protojson.Format(got.GetDesiredState()))
	}
	// An object that does not exist in VPP gets no description invented.
	v.DeleteTable(7001, false)
	v.DeleteTable(7001, true)
	got, _ = s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"vrfs"}})
	if len(got.GetDesiredState().GetVrfs()) != 0 {
		t.Fatalf("vrf invented: %v", got.GetDesiredState())
	}
}

// L3: a non-empty domain this build does not implement is applied as "not managed" and DryRun
// says so.
func TestUnimplementedDomainWarning(t *testing.T) {
	s := newSvc(t, coretest.New(), t.TempDir())
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{DesiredState: doc(t, `{"system":{"hostname":"x"},"nat":{},"vrfs":{"red":{"id":7001}}}`)})
	if err != nil || !rep.GetOk() {
		t.Fatalf("%v %v", err, rep)
	}
	var rules []string
	for _, e := range rep.GetErrors() {
		rules = append(rules, e.GetPointer()+" "+e.GetRule())
	}
	if strings.Join(rules, ",") != "/system agent.unimplemented-domain" {
		t.Fatalf("warnings %v", rules)
	}
}

func TestRetrieveSubsystems(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetDesiredState().GetVrfs()) != 0 || got.GetDesiredState().GetRouting() != nil || got.GetDesiredState().GetInterfaces()["loop701"].GetVrf() != "red" {
		t.Fatalf("interfaces only: %s", protojson.Format(got.GetDesiredState()))
	}
	if _, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"acl"}}); grpcCode(err) != codes.Unimplemented {
		t.Fatalf("acl: %v", err)
	}
	if _, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"x"}}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("x: %v", err)
	}
}

func TestEventBufferOverflow(t *testing.T) {
	b := newBus()
	sub := b.subscribe(&vrxv1.StreamEventsRequest{Kinds: []vrxv1.EventKind{vrxv1.EventKind_EVENT_KIND_LINK_UP}, Interfaces: []string{"loop701"}})
	sub.size = 3
	n := "loop701"
	other := "loop702"
	for i := 0; i < 5; i++ {
		b.publish(&vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_LINK_UP, Interface: &n})
	}
	b.publish(&vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_LINK_UP, Interface: &other}) // filtered
	b.publish(&vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_RECONCILE_START})            // filtered
	evs, err := sub.next(context.Background())
	if err != nil || len(evs) != 4 || evs[0].GetKind() != vrxv1.EventKind_EVENT_KIND_ERROR || evs[0].GetMessage() != "dropped 2 events" {
		t.Fatalf("events %v %v", err, evs)
	}
	if evs[0].GetSeq() != 1 || evs[3].GetSeq() != 4 {
		t.Fatal("seq")
	}
}

func TestMetricsExposition(t *testing.T) {
	m := newMetrics()
	m.setVPP(true)
	m.observe(vrxv1.ApplyStatus_APPLY_STATUS_APPLIED, 30*time.Millisecond, &vrxv1.ApplySummary{Created: 3})
	m.observe(vrxv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK, 2*time.Second, &vrxv1.ApplySummary{Reverted: 1})
	var b strings.Builder
	m.write(&b)
	out := b.String()
	for _, want := range []string{
		"vrx_agent_vpp_connected 1",
		`vrx_agent_reconcile_total{status="APPLY_STATUS_APPLIED"} 1`,
		"vrx_agent_reconcile_errors_total 1",
		`vrx_agent_reconcile_duration_seconds_bucket{le="0.05"} 1`,
		`vrx_agent_reconcile_duration_seconds_bucket{le="+Inf"} 2`,
		"vrx_agent_reconcile_duration_seconds_count 2",
		`vrx_agent_reconcile_operations_total{op="created"} 3`,
		"vrx_agent_retrieve_unsupported_objects 0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

// TestGRPCRoundTrip drives the real gRPC server over a unix socket.
func TestGRPCRoundTrip(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	sock := filepath.Join(t.TempDir(), "agent.sock")
	l, err := listenUnix(sock, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	vrxv1.RegisterDataplaneServer(g, &server{svc: s, stats: fakeStats{}, log: s.log})
	go func() { _ = g.Serve(l) }()
	defer g.Stop()

	cc, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cc.Close() }()
	c := vrxv1.NewDataplaneClient(cc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	h, err := c.Health(ctx, &vrxv1.HealthRequest{})
	if err != nil || h.GetOwner() != testOwner || !h.GetVppConnected() || strings.Join(h.GetSubsystems(), ",") != strings.Join(implementedDomains(), ",") {
		t.Fatalf("health %v %v", err, h)
	}
	evs, err := c.StreamEvents(ctx, &vrxv1.StreamEventsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond) // subscription registered
	resp, err := c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: "g1", DesiredState: doc(t, sampleDoc)})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("apply %v %v", err, resp)
	}
	e1, err := evs.Recv()
	if err != nil || e1.GetKind() != vrxv1.EventKind_EVENT_KIND_RECONCILE_START || e1.GetSeq() != 1 || e1.GetTxnId() != "g1" {
		t.Fatalf("event 1 %v %v", err, e1)
	}
	e2, err := evs.Recv()
	if err != nil || e2.GetKind() != vrxv1.EventKind_EVENT_KIND_RECONCILE_DONE || e2.GetSummary().GetCreated() != 12 {
		t.Fatalf("event 2 %v %v", err, e2)
	}
	got, err := c.Retrieve(ctx, &vrxv1.RetrieveRequest{})
	if err != nil || !proto.Equal(got.GetDesiredState(), doc(t, canonicalDoc)) {
		t.Fatalf("retrieve %v", err)
	}
	if _, err := c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: "g2", Owner: "w9"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("owner mismatch over grpc: %v", err)
	}
	act, err := c.Action(ctx, &vrxv1.ActionRequest{})
	if err == nil {
		_, err = act.Recv()
	}
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("action: %v", err)
	}
	st, err := c.StreamStats(ctx, &vrxv1.StreamStatsRequest{IntervalMs: 200, Interfaces: []string{"loop701"}})
	if err != nil {
		t.Fatal(err)
	}
	b1, err := st.Recv()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := st.Recv()
	if err != nil || b2.GetSeq() <= b1.GetSeq() || len(b1.GetInterfaceCounters()) != 1 || b1.GetInterfaceCounters()[0].GetRxPackets() != 7 || b1.GetIntervalMs() != 200 {
		t.Fatalf("stats %v %v %v", err, b1, b2)
	}
	if _, err := c.StreamStats(ctx, &vrxv1.StreamStatsRequest{IntervalMs: 10}); err == nil {
		bad, _ := c.StreamStats(ctx, &vrxv1.StreamStatsRequest{IntervalMs: 10})
		if _, err := bad.Recv(); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("interval 10: %v", err)
		}
	}
}

type fakeStats struct{}

func (fakeStats) InterfaceStats() ([]api.InterfaceCounters, error) {
	return []api.InterfaceCounters{
		{InterfaceIndex: 2, InterfaceName: "loop702", Rx: api.InterfaceCounterCombined{Packets: 1}},
		{InterfaceIndex: 1, InterfaceName: "loop701", Rx: api.InterfaceCounterCombined{Packets: 7, Bytes: 700}, RxErrors: 1, TxErrors: 2},
	}, nil
}

var _ net.Listener = (*net.UnixListener)(nil)

// fakeConn adds connection-state notifications to the fake VPP model.
type fakeConn struct {
	*coretest.VPP
	states chan vpp.ConnState
}

func (f *fakeConn) States() <-chan vpp.ConnState { return f.states }
func (f *fakeConn) Close()                       {}

func TestWatchVPPResyncsOnEveryConnect(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	fc := &fakeConn{VPP: v, states: make(chan vpp.ConnState, 4)}
	a := &Agent{log: s.log, conn: fc, svc: s, metrics: s.metrics}
	sub := s.events().subscribe(&vrxv1.StreamEventsRequest{Kinds: []vrxv1.EventKind{
		vrxv1.EventKind_EVENT_KIND_VPP_CONNECTED, vrxv1.EventKind_EVENT_KIND_VPP_DISCONNECTED,
		vrxv1.EventKind_EVENT_KIND_RECONCILE_START, vrxv1.EventKind_EVENT_KIND_RECONCILE_DONE,
	}})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.watchVPP(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	fc.states <- vpp.ConnState{Connected: true}
	evs := collect(t, sub, 3)
	// VPP "restarted": everything gone, disconnect, reconnect → resync recreates it.
	v.DeleteInterface("loop701")
	v.DeleteInterface("loop702")
	v.DeleteTable(7001, false)
	v.DeleteTable(7001, true)
	fc.states <- vpp.ConnState{Connected: false}
	fc.states <- vpp.ConnState{Connected: true}
	evs = append(evs, collect(t, sub, 4)...)
	if got := kinds(evs); got != "VPP_CONNECTED:,RECONCILE_START:,RECONCILE_DONE:,VPP_DISCONNECTED:,VPP_CONNECTED:,RECONCILE_START:,RECONCILE_DONE:" {
		t.Fatalf("events %s", got)
	}
	if evs[6].GetSummary().GetCreated() == 0 || evs[6].GetMessage() != "APPLY_STATUS_APPLIED" {
		t.Fatalf("resync after reconnect %v", evs[6])
	}
	if h := s.Health(); h.GetVppVersion() != "26.06-fake" {
		t.Fatalf("vpp version %q", h.GetVppVersion())
	}
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{})
	if err != nil || !proto.Equal(got.GetDesiredState(), doc(t, canonicalDoc)) {
		t.Fatalf("after reconnect resync: %v", err)
	}
}

func TestDegradedWhenRollbackFails(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	sub := s.events().subscribe(&vrxv1.StreamEventsRequest{Kinds: []vrxv1.EventKind{vrxv1.EventKind_EVENT_KIND_DEGRADED}})
	// loop701 is created, then the overlapping address on loop702 fails, and deleting loop701
	// during the rollback fails too.
	v.Fail("delete_loopback", errors.New("stuck"))
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "d1", DesiredState: doc(t, `{"interfaces":{"loop701":{"ipv4":["10.7.1.1/24"]},"loop702":{"ipv4":["10.7.1.2/24"]}}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_DEGRADED)
	if !s.Health().GetDegraded() {
		t.Fatal("health not degraded")
	}
	if evs := collect(t, sub, 1); evs[0].GetKind() != vrxv1.EventKind_EVENT_KIND_DEGRADED {
		t.Fatalf("event %v", evs)
	}
	var revertFailed int
	for _, r := range resp.GetResults() {
		if r.GetCode() == vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_REVERT_FAILED {
			revertFailed++
		}
	}
	if revertFailed == 0 {
		t.Fatalf("results %v", resp.GetResults())
	}
	// A later successful transaction clears the flag.
	v.Reply("delete_loopback", &interfaces.DeleteLoopbackReply{})
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "d2", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if s.Health().GetDegraded() {
		t.Fatal("still degraded after a successful apply")
	}
}

func TestBlackholeRoute(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "b1", DesiredState: doc(t, `{"routing":{"static":[{"prefix":"10.7.66.0/24","vrf":"default","distance":1,"blackhole":true}]}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"routing"}})
	if err != nil || !proto.Equal(got.GetDesiredState(), doc(t, `{"routing":{"static":[{"prefix":"10.7.66.0/24","vrf":"default","distance":1,"blackhole":true}]}}`)) {
		t.Fatalf("blackhole retrieve %v %s", err, protojson.Format(got.GetDesiredState()))
	}
	for _, bad := range []string{
		`{"routing":{"static":[{"prefix":"10.7.67.0/24","blackhole":false}]}}`,
		`{"routing":{"static":[{"prefix":"10.7.67.0/24","blackhole":true,"nextHops":[{"address":"10.7.1.1"}]}]}}`,
	} {
		resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "bad-" + bad, DesiredState: doc(t, bad)})
		mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_FAILED)
		if resp.GetValidation().GetErrors()[0].GetRule() != "routing.static.blackhole" {
			t.Fatalf("rule %v", resp.GetValidation())
		}
	}
}

// H3: VPP disconnected at the confirm deadline → the revert fails, stays owed (persisted, the
// transaction stays pending), and the next resync converges to the confirmed baseline — it never
// re-applies the unconfirmed config.
func TestRevertOwedWhenVPPDownAtDeadline(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "base", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	baseline := v.Snapshot()
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, sampleDoc), ConfirmTimeoutSec: 1}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	v.SetConnected(false)
	deadline := time.Now().Add(3 * time.Second)
	for !s.Health().GetDegraded() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	h := s.Health()
	if !h.GetDegraded() || h.GetPendingConfirmTxnId() != "p1" {
		t.Fatalf("after failed revert: %v", h)
	}
	if _, err := s.Apply(context.Background(), &vrxv1.ApplyRequest{ConfirmTxnId: "p1"}); grpcCode(err) != codes.FailedPrecondition {
		t.Fatalf("confirm of an owed revert: %v", err)
	}
	// The owed revert survives an agent restart too.
	s.Close()
	s2 := newSvc(t, v, dir)
	if !s2.st.meta.Reverting || s2.st.meta.PendingTxnID != "p1" || !proto.Equal(s2.st.desired, s2.st.confirm) {
		t.Fatalf("persisted state: reverting=%v pending=%q desired==confirmed %v", s2.st.meta.Reverting, s2.st.meta.PendingTxnID, proto.Equal(s2.st.desired, s2.st.confirm))
	}
	// VPP back → resync performs the revert.
	v.SetConnected(true)
	resp := s2.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if v.Snapshot() != baseline {
		t.Fatalf("resync did not converge to the confirmed baseline:\n%s\n%s", baseline, v.Snapshot())
	}
	if h := s2.Health(); h.GetPendingConfirmTxnId() != "" || h.GetDegraded() {
		t.Fatalf("health after the owed revert: %v", h)
	}
	// A converged resync afterwards changes nothing.
	resp = s2.Resync(context.Background())
	if resp.GetSummary().GetCreated()+resp.GetSummary().GetDeleted()+resp.GetSummary().GetUpdated() != 0 {
		t.Fatalf("second resync %v", resp.GetSummary())
	}
}

// M1: a confirm after the deadline is rejected, also after a restart before the first resync.
func TestLateConfirmRejectedAfterRestart(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, sampleDoc), ConfirmTimeoutSec: 1}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	s.Close()
	time.Sleep(1100 * time.Millisecond)
	s2 := newSvc(t, v, dir) // no resync yet (VPP not connected in real life)
	if _, err := s2.Apply(context.Background(), &vrxv1.ApplyRequest{ConfirmTxnId: "p1"}); grpcCode(err) != codes.FailedPrecondition {
		t.Fatalf("late confirm accepted: %v", err)
	}
	s2.Resync(context.Background())
	if _, ok := v.InterfaceByName("loop701"); ok {
		t.Fatal("expired transaction not reverted")
	}
}

// M3: crash injection between the write stages never leaves a pending transaction confirmed.
func TestStateCrashInjection(t *testing.T) {
	defer func() { saveHook = nil }()
	crash := errors.New("injected crash")
	for _, stage := range []string{"state", "mirror"} {
		t.Run(stage, func(t *testing.T) {
			v := coretest.New()
			dir := t.TempDir()
			s := newSvc(t, v, dir)
			mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "base", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
			saveHook = func(st string) error {
				if st == stage {
					return crash
				}
				return nil
			}
			mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, sampleDoc), ConfirmTimeoutSec: 60}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
			saveHook = nil
			s.Close()
			s2 := newSvc(t, v, dir)
			switch stage {
			case "state": // crashed before the single state file was replaced: old, consistent state
				if s2.st.meta.PendingTxnID != "" || s2.st.meta.LastTxnID != "base" || len(s2.st.desired.GetInterfaces()) != 0 {
					t.Fatalf("state after crash before write: pending=%q last=%q desired=%v", s2.st.meta.PendingTxnID, s2.st.meta.LastTxnID, s2.st.desired)
				}
			case "mirror": // state file written: the pending transaction is known and not confirmed
				if s2.st.meta.PendingTxnID != "p1" || len(s2.st.desired.GetInterfaces()) != 2 || len(s2.st.confirm.GetInterfaces()) != 0 {
					t.Fatalf("state after crash before mirror: pending=%q", s2.st.meta.PendingTxnID)
				}
			}
			if s2.st.meta.PendingTxnID == "" && len(s2.st.desired.GetInterfaces()) != 0 {
				t.Fatal("pending document without pending marker")
			}
		})
	}
}

// Pre-review state dirs (desired.pb + confirmed.pb + json without documents) still load.
func TestStateMigratesOldLayout(t *testing.T) {
	dir := t.TempDir()
	old := &vrxv1.DesiredState{Vrfs: map[string]*vrxv1.Vrf{"red": {Id: proto.Uint32(7001)}}}
	b, _ := proto.Marshal(old)
	if err := os.WriteFile(filepath.Join(dir, "desired.pb"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "confirmed.pb"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, stateFile), []byte(`{"owner":"w7","managed":["vrfs"],"confirmed_managed":["vrfs"],"last_txn_id":"t0"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := loadState(dir, "w7")
	if err != nil || !proto.Equal(st.desired, old) || !proto.Equal(st.confirm, old) {
		t.Fatalf("migration: %v %v", err, st)
	}
}

// revertObstacle builds the re-review N2 scenario: the confirmed baseline has route 10.7.99.0/24,
// pending txn p1 removes it, another owner (w7x) takes the prefix before the deadline, so the
// revert fails deterministically (claim rule) while VPP stays connected.
func revertObstacle(t *testing.T, v *coretest.VPP, s *Service) *scheduler.Scheduler {
	t.Helper()
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "base", DesiredState: doc(t, `{"routing":{"static":[{"prefix":"10.7.99.0/24","blackhole":true}]}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, `{"routing":{}}`), ConfirmTimeoutSec: 1}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	reg := scheduler.NewRegistry()
	core.Register(reg, core.Env{Client: v, Owner: "w7x", Owned: ownertable.NewMemory()})
	other := scheduler.New(reg, nil)
	if r := other.Apply(context.Background(), []scheduler.KV{{Key: "ip.route/0/10.7.99.0/24", Value: &core.Route{Prefix: "10.7.99.0/24"}}}, nil); r.Outcome != scheduler.OutcomeApplied {
		t.Fatal(r.Err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !s.Health().GetDegraded() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if h := s.Health(); !h.GetDegraded() || h.GetPendingConfirmTxnId() != "p1" {
		t.Fatalf("revert did not fail as staged: %v", h)
	}
	return other
}

// N2: an unrevertable owed revert does not block Apply: a new Apply supersedes it and converges.
func TestNewApplySupersedesOwedRevert(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	revertObstacle(t, v, s)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "fix1", DesiredState: doc(t, `{"routing":{}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if h := s.Health(); h.GetPendingConfirmTxnId() != "" || h.GetDegraded() || h.GetLastTxnId() != "fix1" {
		t.Fatalf("health after superseding apply: %v", h)
	}
	if s.st.meta.Reverting || s.st.meta.PendingTxnID != "" {
		t.Fatal("owed revert not dropped")
	}
	// A later apply converges normally.
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "fix2", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}},"routing":{}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !v.HasTable(7001, false) {
		t.Fatal("later apply not applied")
	}
}

// N2: the owed revert is retried on a backoff timer (no VPP reconnect) and converges once the
// obstacle is gone.
func TestOwedRevertRetriedByTimer(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	_ = s.lock(context.Background())
	s.retryMin, s.retryMax = 50*time.Millisecond, 200*time.Millisecond
	s.unlock()
	other := revertObstacle(t, v, s)
	if r := other.Apply(context.Background(), nil, nil); r.Outcome != scheduler.OutcomeApplied { // obstacle removed
		t.Fatal(r.Err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for s.Health().GetPendingConfirmTxnId() != "" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if h := s.Health(); h.GetPendingConfirmTxnId() != "" || h.GetDegraded() {
		t.Fatalf("timer retry did not converge: %v", h)
	}
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"routing"}})
	if err != nil || len(got.GetDesiredState().GetRouting().GetStatic()) != 1 {
		t.Fatalf("baseline route not restored: %v %v", err, got)
	}
}

// L-a: a VRF with id 0 under another name is table 0 by id: unbound interfaces and table-0
// routes Retrieve with that name, no drift.
func TestTableZeroNamedByID(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	d := `{"vrfs":{"main":{"id":0}},"interfaces":{"loop705":{"vrf":"main"}},"routing":{"static":[{"prefix":"10.7.66.0/24","vrf":"main","distance":1,"blackhole":true}]}}`
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "z1", DesiredState: doc(t, d)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces", "routing"}})
	want := doc(t, `{"interfaces":{"loop705":{"enabled":false,"promiscuous":false,"vrf":"main"}},"routing":{"static":[{"prefix":"10.7.66.0/24","vrf":"main","distance":1,"blackhole":true}]}}`)
	if err != nil || !proto.Equal(got.GetDesiredState(), want) {
		t.Fatalf("retrieve %v %s", err, protojson.Format(got.GetDesiredState()))
	}
}
