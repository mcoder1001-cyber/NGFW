package agent

// F-qos-flat at agent level, on the coretest VPP model (coretest/qos_flat.go): the services.qos projection applied
// through the real wiring (subsystems.Register with persisted stores), Retrieve, agent restart, VPP restart,
// rollback, DryRun findings and the two RPCs.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.fd.io/govpp/adapter"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	binqos "ngfw/agent/binapi/qos"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// newQoSSvc is newSvc with the slot's id range (egress map ids 7000–7999, D-071).
func newQoSSvc(t *testing.T, v *coretest.VPP, dir string) *Service {
	t.Helper()
	owned, err := ownertable.Open(dir, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := subsystems.Register(reg, subsystems.Env{Client: v, Owner: testOwner, StateDir: dir, Owned: owned, NetdevKind: fakeNetdevs,
		IDs: subsystems.IDScope{Range: &subsystems.IDRange{Lo: 7000, Hi: 7999}}})
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
	svc.retryMin, svc.retryMax = time.Hour, time.Hour
	t.Cleanup(svc.Close)
	return svc
}

// vppBoot fakes the D-080 boot identity of the modelled VPP (the coretest control_ping has no PID); restart()
// models a VPP restart for the applied-once records and the claims.
func vppBoot(t *testing.T) (restart func()) {
	t.Helper()
	var pid atomic.Int64
	pid.Store(4100)
	prev := dfkit.IdentitySource
	dfkit.IdentitySource = func(context.Context, vpp.Client) (bootid.Identity, error) {
		return bootid.Identity{BootID: "qos-boot", PID: int(pid.Load()), StartTime: uint64(pid.Load())}, nil //nolint:gosec // G115: test PID
	}
	t.Cleanup(func() { dfkit.IdentitySource = prev })
	return func() { pid.Add(1) }
}

const qosAgentDoc = `{
  "interfaces": {"loop7101": {}, "loop7102": {}, "loop7103": {}},
  "services": {"qos": {
    "policers": {
      "gold": {"description": "customer", "type": "2r3c-rfc2698", "rateUnit": "kbps", "cir": 20000, "eir": 40000, "cb": "25000", "eb": "50000",
               "round": "closest", "colorAware": false, "conformAction": {"action": "transmit"},
               "exceedAction": {"action": "mark-and-transmit", "dscp": 10}, "violateAction": {"action": "drop"}},
      "pps": {"type": "1r2c", "rateUnit": "pps", "cir": 1000, "cb": "100", "round": "closest", "colorAware": false,
              "conformAction": {"action": "transmit"}, "exceedAction": {"action": "drop"}, "violateAction": {"action": "drop"}}
    },
    "shapers": {"up": {"description": "drop-based", "rateKbps": 50000}},
    "maps": {"remark": {"id": 7001, "rows": {"ip": [{"from": 46, "to": 34}]}}, "pcp": {"rows": {"ip": [{"from": 46, "to": 5}]}}},
    "interfaces": {
      "loop7101": {"description": "customer", "policer": {"input": "gold"}, "shaper": "up", "record": "vlan", "store": {"source": "ip", "value": 0}},
      "loop7102": {"policer": {"output": "pps"}, "record": "ip", "mark": {"map": "remark", "output": "ip"}},
      "loop7103": {"policer": {"input": "pps"}}
    }
  }}
}`

// what Retrieve reports for qosAgentDoc: no write-only attachments (loop7103 had only one), names and
// descriptions from the qos.meta record, the auto-numbered map without its id.
const qosRetrieved = `{"services": {"qos": {
  "policers": {
    "gold": {"description": "customer", "type": "2r3c-rfc2698", "rateUnit": "kbps", "cir": 20000, "eir": 40000, "cb": "25000", "eb": "50000",
             "round": "closest", "colorAware": false, "conformAction": {"action": "transmit"},
             "exceedAction": {"action": "mark-and-transmit", "dscp": 10}, "violateAction": {"action": "drop"}},
    "pps": {"type": "1r2c", "rateUnit": "pps", "cir": 1000, "cb": "100", "round": "closest", "colorAware": false,
            "conformAction": {"action": "transmit"}, "exceedAction": {"action": "drop"}, "violateAction": {"action": "drop"}}
  },
  "shapers": {"up": {"description": "drop-based", "rateKbps": 50000}},
  "maps": {"remark": {"id": 7001, "rows": {"ip": [{"from": 46, "to": 34}]}}, "pcp": {"rows": {"ip": [{"from": 46, "to": 5}]}}},
  "interfaces": {
    "loop7101": {"description": "customer", "record": "vlan", "store": {"source": "ip", "value": 0}},
    "loop7102": {"record": "ip", "mark": {"map": "remark", "output": "ip"}}
  }
}}}`

func ifIndex(t *testing.T, v *coretest.VPP, name string) uint32 {
	t.Helper()
	i, ok := v.InterfaceByName(name)
	if !ok {
		t.Fatalf("no interface %s", name)
	}
	return i.Index
}

// features asserts one policer feature instance per applied attachment (none stacked, D-076).
func features(t *testing.T, v *coretest.VPP, want int) {
	t.Helper()
	got := [4]int{
		v.PolicerFeatures("in", ifIndex(t, v, "loop7101")), v.PolicerFeatures("out", ifIndex(t, v, "loop7101")),
		v.PolicerFeatures("out", ifIndex(t, v, "loop7102")), v.PolicerFeatures("in", ifIndex(t, v, "loop7103")),
	}
	if got != [4]int{want, want, want, want} {
		t.Fatalf("policer features in/out loop7101, out loop7102, in loop7103 = %v, want %d each", got, want)
	}
	if n := v.UnseenUnapplies(); n != 0 {
		t.Fatalf("%d policer un-applies on an interface without a policer (VPP out-of-bounds write)", n)
	}
}

func retrieveQoS(t *testing.T, s *Service) *vrxv1.QosService {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"services"}})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState().GetServices().GetQos()
}

func sameQoS(t *testing.T, got *vrxv1.QosService, want string) {
	t.Helper()
	if w := doc(t, want).GetServices().GetQos(); !proto.Equal(got, w) {
		t.Fatalf("services.qos\n got %s\nwant %s", protojson.Format(got), protojson.Format(w))
	}
}

// TestQoSApplyRetrieveRollback: commit → VPP has the policers, the shaper's egress policer, the maps (7001 fixed,
// 7000 allocated), records, the store and the mark; every attachment applied exactly once, also over repeated
// resyncs; Retrieve reports the document minus the write-only attachments; rollback removes everything and never
// un-applies where nothing was applied.
func TestQoSApplyRetrieveRollback(t *testing.T) {
	vppBoot(t)
	v := coretest.New()
	dir := t.TempDir()
	s := newQoSSvc(t, v, dir)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "q1", DesiredState: doc(t, qosAgentDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	pols := v.QoSPolicers()
	for _, n := range []string{"w7:gold", "w7:pps", "w7:shaper:up"} {
		if _, ok := pols[n]; !ok {
			t.Fatalf("policer %s missing: %v", n, pols)
		}
	}
	if sh := pols["w7:shaper:up"].Details; sh.Cir != 50000 || sh.Cb != 62500 || sh.Type != 0 || sh.ExceedAction.Type != 0 {
		t.Fatalf("shaper policer %+v", sh) // 1r2c (type 0), exceed → drop (action 0)
	}
	if maps := v.QoSMaps(); len(maps) != 2 || maps[0] != 7000 || maps[1] != 7001 {
		t.Fatalf("maps %v", maps)
	}
	if mk := v.QoSMarks(binqos.QOS_API_SOURCE_IP); mk[ifIndex(t, v, "loop7102")] != 7001 {
		t.Fatalf("marks %v", mk)
	}
	if v.QoSRecords(ifIndex(t, v, "loop7101"), binqos.QOS_API_SOURCE_VLAN) != 1 || v.QoSRecords(ifIndex(t, v, "loop7102"), binqos.QOS_API_SOURCE_IP) != 1 {
		t.Fatal("records")
	}
	features(t, v, 1)
	sameQoS(t, retrieveQoS(t, s), qosRetrieved)

	// idempotent and applied once: a repeat and two resyncs change nothing in VPP
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "q2", DesiredState: doc(t, qosAgentDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	for i := 0; i < 2; i++ {
		mustStatus(t, s.Resync(context.Background()), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	}
	features(t, v, 1)

	// rollback to "no QoS": everything goes, marks before maps, attachments un-applied (applied in this lifetime)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "q3", DesiredState: doc(t, `{"interfaces": {"loop7101": {}, "loop7102": {}, "loop7103": {}}, "services": {}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(v.QoSPolicers()) != 0 || len(v.QoSMaps()) != 0 || len(v.QoSMarks(binqos.QOS_API_SOURCE_IP)) != 0 {
		t.Fatalf("rollback left policers %v maps %v", v.QoSPolicers(), v.QoSMaps())
	}
	features(t, v, 0)
	sameQoS(t, retrieveQoS(t, s), `{"services": {"qos": {}}}`)
	if _, err := os.Stat(filepath.Join(dir, "qos-"+testOwner+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("qos.meta record left behind: %v", err)
	}
}

// TestQoSAgentRestartRecreates (FAST-MODE restart safety): the agent stops, its objects are deleted behind its
// back, a new agent on the same state dir resyncs and rebuilds them — and does not apply the still-present
// attachments a second time (the persisted applied-once records, D-076).
func TestQoSAgentRestartRecreates(t *testing.T) {
	vppBoot(t)
	v := coretest.New()
	dir := t.TempDir()
	s := newQoSSvc(t, v, dir)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "r1", DesiredState: doc(t, qosAgentDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	s.Close()
	if !v.DeleteQoSPolicer("w7:gold") {
		t.Fatal("no gold to delete")
	}
	v.DeleteQoSMap(7000)
	s2 := newQoSSvc(t, v, dir)
	resp := s2.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if resp.GetSummary().GetCreated() < 2 {
		t.Fatalf("resync recreated %d objects", resp.GetSummary().GetCreated())
	}
	gold, ok := v.QoSPolicers()["w7:gold"]
	if !ok || len(v.QoSMaps()) != 2 {
		t.Fatalf("not rebuilt: %v %v", v.QoSPolicers(), v.QoSMaps())
	}
	features(t, v, 1)
	// VPP binds by pool index and the re-created policer has a new one: the attachment was re-pointed
	if b, ok := v.PolicerBinding("in", ifIndex(t, v, "loop7101")); !ok || b != gold.Index {
		t.Fatalf("loop7101 input still bound to pool index %d, gold is now %d", b, gold.Index)
	}
	sameQoS(t, retrieveQoS(t, s2), qosRetrieved)
}

// TestQoSVPPRestart: after a VPP restart (tables gone, new boot identity) the resync applies every attachment once
// more; a Delete after a second restart sends no un-apply at all (VPP would write out of bounds).
func TestQoSVPPRestart(t *testing.T) {
	restart := vppBoot(t)
	v := coretest.New()
	s := newQoSSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "v1", DesiredState: doc(t, qosAgentDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	restart()
	v.QoSRestart()
	mustStatus(t, s.Resync(context.Background()), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	features(t, v, 1)
	sameQoS(t, retrieveQoS(t, s), qosRetrieved)
	restart()
	v.QoSRestart()
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "v2", DesiredState: doc(t, `{"interfaces": {"loop7101": {}, "loop7102": {}, "loop7103": {}}, "services": {}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	features(t, v, 0) // includes UnseenUnapplies == 0
}

// TestQoSDryRun: the agent's own findings carry pointers (store source, egress exclusivity), and the write-only
// attachments are marked for the drift view.
func TestQoSDryRun(t *testing.T) {
	v := coretest.New()
	s := newQoSSvc(t, v, t.TempDir())
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: doc(t, `{
	  "interfaces": {"loop7101": {}},
	  "services": {"qos": {"policers": {"p": {"cir": 10, "cb": "1500"}}, "shapers": {"s": {"rateKbps": 10}},
	    "interfaces": {"loop7101": {"store": {"source": "vlan", "value": 1}, "shaper": "s", "policer": {"output": "p"}}}}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, is := range rep.GetErrors() {
		if is.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
			got = append(got, is.GetSeverity().String()+" "+is.GetPointer()+" "+is.GetRule())
		}
	}
	want := []string{
		"ISSUE_SEVERITY_ERROR /services/qos/interfaces/loop7101/shaper services.qos-flat-egress",
		"ISSUE_SEVERITY_ERROR /services/qos/interfaces/loop7101/store/source services.qos-flat-store-source",
	}
	if rep.GetOk() || strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("dry run %v", got)
	}
	rep, err = s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d2", DesiredState: doc(t, qosAgentDoc)})
	if err != nil || !rep.GetOk() {
		t.Fatalf("dry run %v %v", rep, err)
	}
	var wo []string
	for _, is := range rep.GetErrors() {
		if is.GetRule() == "agent.write-only" {
			wo = append(wo, is.GetPointer())
		}
	}
	if strings.Join(wo, " ") != "/services/qos/interfaces/loop7101/policer /services/qos/interfaces/loop7101/shaper /services/qos/interfaces/loop7102/policer /services/qos/interfaces/loop7103" {
		t.Fatalf("write-only notes %v", wo)
	}
}

type fakeCounters struct {
	m   map[uint32][3]qosCounter
	err error
}

func (f fakeCounters) PolicerCounters() (map[uint32][3]qosCounter, error) { return f.m, f.err }

// TestQoSPolicerRPCs: QosPolicerState lists this owner's policers and shapers with the configuration spelling and
// the counters of their pool index; QosPolicerReset refills one (NOT_FOUND for an unknown one); both are refused for
// a foreign owner, and a second walk waits at most qosWalkWait (D-132).
func TestQoSPolicerRPCs(t *testing.T) {
	vppBoot(t)
	v := coretest.New()
	s := newQoSSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "s1", DesiredState: doc(t, qosAgentDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	v.QoSPolicers() // indexes: gold 0, pps 1, shaper:up 2 (creation order of the plan)
	idx := map[string]uint32{}
	for n, p := range v.QoSPolicers() {
		idx[strings.TrimPrefix(n, testOwner+":")] = p.Index
	}
	counters := fakeCounters{m: map[uint32][3]qosCounter{idx["gold"]: {{10, 1000}, {2, 200}, {1, 100}}}}
	ctx := context.Background()
	resp, err := s.qosPolicerState(ctx, &vrxv1.QosPolicerStateRequest{}, counters)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range resp.GetPolicers() {
		names = append(names, p.GetKind()+":"+p.GetName())
	}
	if strings.Join(names, " ") != "policer:gold policer:pps shaper:shaper:up" || resp.GetOwner() != testOwner || resp.GetCountersError() != "" {
		t.Fatalf("policers %v (%s)", names, resp.GetCountersError())
	}
	gold := resp.GetPolicers()[0]
	if gold.GetType() != "2r3c-rfc2698" || gold.GetRateUnit() != "kbps" || gold.GetCir() != 20000 || gold.GetEir() != 40000 || gold.GetCb() != 25000 ||
		gold.GetConform().GetPackets() != 10 || gold.GetExceed().GetBytes() != 200 || gold.GetViolate().GetPackets() != 1 || gold.GetCurrentLimit() == 0 {
		t.Fatalf("gold %v", gold)
	}
	if sh := resp.GetPolicers()[2]; sh.GetCir() != 50000 || sh.GetCb() != 62500 || sh.GetType() != "1r2c" || sh.GetConform().GetPackets() != 0 {
		t.Fatalf("shaper %v", sh)
	}
	resp, err = s.qosPolicerState(ctx, &vrxv1.QosPolicerStateRequest{Names: []string{"pps"}}, fakeCounters{err: errors.New("stats segment gone")})
	if err != nil || len(resp.GetPolicers()) != 1 || resp.GetCountersError() != "stats segment gone" {
		t.Fatalf("filtered %v %v", resp, err)
	}
	// through the gRPC adapter: no stats segment in a unit test → counters_error, the rest valid
	g := &server{svc: s}
	if r, err := g.QosPolicerState(ctx, &vrxv1.QosPolicerStateRequest{Owner: testOwner}); err != nil || len(r.GetPolicers()) != 3 || r.GetCountersError() == "" {
		t.Fatalf("grpc state %v %v", r, err)
	}
	if _, err := g.QosPolicerState(ctx, &vrxv1.QosPolicerStateRequest{Owner: "w9"}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("foreign owner: %v", err)
	}

	rr, err := g.QosPolicerReset(ctx, &vrxv1.QosPolicerResetRequest{Name: "shaper:up"})
	if err != nil || rr.GetIndex() != idx["shaper:up"] || v.QoSPolicers()["w7:shaper:up"].Resets != 1 {
		t.Fatalf("reset %v %v", rr, err)
	}
	if _, err := g.QosPolicerReset(ctx, &vrxv1.QosPolicerResetRequest{Name: "nope"}); grpcCode(err) != codes.NotFound {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := g.QosPolicerReset(ctx, &vrxv1.QosPolicerResetRequest{Name: "a b"}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("bad name: %v", err)
	}

	// D-132: one walk at a time; the second caller gives up after qosWalkWait with UNAVAILABLE
	prev := qosWalkWait
	qosWalkWait = 50 * time.Millisecond
	t.Cleanup(func() { qosWalkWait = prev })
	release, err := s.qosWalk(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.QosPolicerState(ctx, &vrxv1.QosPolicerStateRequest{}); grpcCode(err) != codes.Unavailable {
		t.Fatalf("busy walk: %v", err)
	}
	release()
	if _, err := g.QosPolicerState(ctx, &vrxv1.QosPolicerStateRequest{}); err != nil {
		t.Fatalf("after release: %v", err)
	}
}

func TestSumPolicerCounters(t *testing.T) {
	cc := func(p, b uint64) adapter.CombinedCounter { return adapter.CombinedCounter{p, b} }
	got := sumPolicerCounters([]adapter.StatEntry{
		{StatIdentifier: adapter.StatIdentifier{Name: []byte("/net/policer/conform")}, Data: adapter.CombinedCounterStat{{cc(1, 10), cc(2, 20)}, {cc(3, 30)}}},
		{StatIdentifier: adapter.StatIdentifier{Name: []byte("/net/policer/violate")}, Data: adapter.CombinedCounterStat{{cc(0, 0), cc(5, 50)}}},
		{StatIdentifier: adapter.StatIdentifier{Name: []byte("/if/rx")}, Data: adapter.CombinedCounterStat{{cc(9, 9)}}},
	})
	if got[0][0] != (qosCounter{4, 40}) || got[1][0] != (qosCounter{2, 20}) || got[1][2] != (qosCounter{5, 50}) || got[0][1] != (qosCounter{}) {
		t.Fatalf("counters %v", got)
	}
}
