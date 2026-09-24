package agent

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip6_nd"
	"ngfw/agent/binapi/ip_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// newSvcGlobals is newSvc with the D-071 globals-owner flag set as given.
func newSvcGlobals(t *testing.T, v *coretest.VPP, dir string, globals bool) *Service {
	t.Helper()
	owned, err := ownertable.Open(dir, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := subsystems.Register(reg, subsystems.Env{Client: v, Owner: testOwner, StateDir: dir, Owned: owned, NetdevKind: fakeNetdevs, GlobalsOwner: globals})
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

// nraDoc uses every F-neighbors-ra leaf on the fake (slot 7 names: loop7xx, host-w7*, table 7xxx).
const nraAgentDoc = `{
  "vrfs": {"red": {"id": 7001, "proxyArpRanges": [{"low": "10.7.3.1", "high": "10.7.3.9"}]}},
  "interfaces": {
    "loop701": {"vrf": "red", "ipv4": ["10.7.1.1/24"], "ipv6": ["2001:db8:7:1::1/64"], "proxyArp": true,
      "ipv6Ra": {"suppress": false, "managed": true, "other": true, "lifetimeSec": 1800, "maxIntervalSec": 600, "minIntervalSec": 200,
        "prefixes": {"2001:db8:7:1::/64": {"validSec": 86400, "preferredSec": 14400, "offLink": false, "noAutoconfig": false}}}},
    "host-w7l0": {"ipv4": ["10.7.2.1/24"], "ipv6": ["2001:db8:7:2::1/64"], "proxyArp": false,
      "ipv6Ra": {"suppress": true, "managed": false, "other": false, "lifetimeSec": 600, "maxIntervalSec": 200, "minIntervalSec": 150},
      "subinterfaces": {"100": {"vlanId": 100, "ipv6": ["2001:db8:7:100::1/64"],
        "ipv6Ra": {"suppress": false, "managed": false, "other": false, "lifetimeSec": 0, "maxIntervalSec": 200, "minIntervalSec": 150}}}}
  },
  "routing": {"neighbors": {"static": [
    {"interface": "loop701", "ip": "10.7.1.50", "mac": "02:00:00:00:07:50", "noFibEntry": false},
    {"interface": "host-w7l0", "ip": "2001:db8:7:2::50", "mac": "02:00:00:00:72:50", "noFibEntry": true}
  ]}}
}`

// the canonical Retrieve of nraAgentDoc (P08's interface defaults + the feature leaves, lists sorted)
const nraAgentCanonical = `{
  "vrfs": {"red": {"id": 7001, "proxyArpRanges": [{"low": "10.7.3.1", "high": "10.7.3.9"}]}},
  "interfaces": {
    "loop701": {"enabled": false, "promiscuous": false, "vrf": "red", "ipv4": ["10.7.1.1/24"], "ipv6": ["2001:db8:7:1::1/64"], "proxyArp": true,
      "ipv6Ra": {"suppress": false, "managed": true, "other": true, "lifetimeSec": 1800, "maxIntervalSec": 600, "minIntervalSec": 200,
        "prefixes": {"2001:db8:7:1::/64": {"validSec": 86400, "preferredSec": 14400, "offLink": false, "noAutoconfig": false}}}},
    "host-w7l0": {"enabled": false, "promiscuous": false, "vrf": "default", "ipv4": ["10.7.2.1/24"], "ipv6": ["2001:db8:7:2::1/64"], "proxyArp": false,
      "ipv6Ra": {"suppress": true, "managed": false, "other": false, "lifetimeSec": 600, "maxIntervalSec": 200, "minIntervalSec": 150},
      "subinterfaces": {"100": {"vlanId": 100, "dot1ad": false, "enabled": false, "vrf": "default", "ipv6": ["2001:db8:7:100::1/64"],
        "ipv6Ra": {"suppress": false, "managed": false, "other": false, "lifetimeSec": 0, "maxIntervalSec": 200, "minIntervalSec": 150}}}}
  },
  "routing": {"neighbors": {"static": [
    {"interface": "host-w7l0", "ip": "2001:db8:7:2::50", "mac": "02:00:00:00:72:50", "noFibEntry": true},
    {"interface": "loop701", "ip": "10.7.1.50", "mac": "02:00:00:00:07:50", "noFibEntry": false}
  ]}}
}`

// the same interfaces without any feature leaf: what rollback leaves behind
const nraAgentBare = `{
  "vrfs": {"red": {"id": 7001}},
  "interfaces": {
    "loop701": {"vrf": "red", "ipv4": ["10.7.1.1/24"], "ipv6": ["2001:db8:7:1::1/64"]},
    "host-w7l0": {"ipv4": ["10.7.2.1/24"], "ipv6": ["2001:db8:7:2::1/64"],
      "subinterfaces": {"100": {"vlanId": 100, "ipv6": ["2001:db8:7:100::1/64"]}}}
  },
  "routing": {}
}`

func retrieveNra(t *testing.T, s *Service) *vrxv1.DesiredState {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces", "vrfs", "routing"}})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState()
}

func sameDoc(t *testing.T, what string, got *vrxv1.DesiredState, want string) {
	t.Helper()
	if !proto.Equal(got, doc(t, want)) {
		t.Fatalf("%s:\n%s", what, protojson.Format(got))
	}
}

func TestNeighborsRaApplyRetrieveRollback(t *testing.T) {
	v := coretest.New()
	m := coretest.NeighborsRaOf(v)
	s := newSvc(t, v, t.TempDir())
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "n1", DesiredState: doc(t, nraAgentDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	sameDoc(t, "Retrieve after apply", retrieveNra(t, s), nraAgentCanonical)
	// the model shows what VPP would
	if sup, pf, ok := m.RaSuppressed(v, "loop701"); !ok || sup || pf != 1 {
		t.Fatalf("loop701 RA suppressed=%v prefixes=%d ipv6=%v", sup, pf, ok)
	}
	if sup, _, ok := m.RaSuppressed(v, "host-w7l0.100"); !ok || sup {
		t.Fatal("sub-interface RA")
	}
	if !m.ProxyArpOn(v, "loop701") || m.ProxyArpOn(v, "host-w7l0") {
		t.Fatal("proxy arp")
	}
	if got := m.Neighbors(v, "loop701"); len(got) != 1 || got[0] != "10.7.1.50 02:00:00:00:07:50 static" {
		t.Fatalf("static %v", got)
	}
	if got := m.Ranges(); len(got) != 1 || got[0] != "10.7.3.1-10.7.3.9 t7001" {
		t.Fatalf("ranges %v", got)
	}
	// idempotent: the same document again plans nothing
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "n2", DesiredState: doc(t, nraAgentDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if sm := resp.GetSummary(); sm.GetCreated()+sm.GetUpdated()+sm.GetDeleted() != 0 {
		t.Fatalf("second apply changed %v", sm)
	}
	// rollback: the feature leaves removed, the interfaces stay
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "n3", DesiredState: doc(t, nraAgentBare)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got := retrieveNra(t, s)
	js := protojson.Format(got)
	for _, leaf := range []string{"ipv6Ra", "proxyArp", "proxyArpRanges", "neighbors"} {
		if strings.Contains(js, leaf) {
			t.Fatalf("rollback left %s:\n%s", leaf, js)
		}
	}
	if sup, pf, _ := m.RaSuppressed(v, "loop701"); !sup || pf != 0 {
		t.Fatal("RA not back to VPP's default")
	}
	if m.ProxyArpOn(v, "loop701") || len(m.Neighbors(v, "loop701")) != 0 || len(m.Ranges()) != 0 {
		t.Fatal("proxy arp / neighbours / ranges left")
	}
	if m.AllCalls() != 0 {
		t.Fatal("a dump named every interface")
	}
}

// Restart safety (FAST MODE DoD 3): the agent is down, the feature's objects vanish behind its back, the restarted
// agent's resync recreates them and Retrieve equals desired again.
func TestNeighborsRaRestartSimulation(t *testing.T) {
	v := coretest.New()
	m := coretest.NeighborsRaOf(v)
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "r1", DesiredState: doc(t, nraAgentDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	s.Close()
	ctx := context.Background()
	lo, _ := v.InterfaceByName("loop701")
	// simulated loss: RA reset to defaults, prefix withdrawn, proxy-ARP off, static neighbour and range deleted
	svc := ip6_nd.NewServiceClient(v)
	if _, err := svc.SwInterfaceIP6ndRaPrefix(ctx, &ip6_nd.SwInterfaceIP6ndRaPrefix{SwIfIndex: interface_types.InterfaceIndex(lo.Index), Prefix: mustPrefix(t, "2001:db8:7:1::/64"), IsNo: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SwInterfaceIP6ndRaConfig(ctx, &ip6_nd.SwInterfaceIP6ndRaConfig{SwIfIndex: interface_types.InterfaceIndex(lo.Index), Suppress: 1}); err != nil {
		t.Fatal(err)
	}
	for _, n := range m.Neighbors(v, "loop701") {
		m.Forget(v, "loop701", strings.Fields(n)[0])
	}
	s2 := newSvc(t, v, dir)
	resp := s2.Resync(ctx)
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if resp.GetSummary().GetCreated()+resp.GetSummary().GetUpdated() == 0 {
		t.Fatalf("resync recreated nothing: %v", resp.GetSummary())
	}
	sameDoc(t, "Retrieve after restart", retrieveNra(t, s2), nraAgentCanonical)
}

func TestNeighborsRaGlobalsOnlyForTheOwner(t *testing.T) {
	const withGlobals = `{"routing": {"neighbors": {"ipv6Limits": {"maxNumber": 20000, "maxAgeSec": 60, "recycle": true}, "dad": {"transmits": 3, "delayMs": 250}}}}`
	// a slot agent (not the globals owner): warnings, nothing applied, VPP untouched
	v := coretest.New()
	s := newSvcGlobals(t, v, t.TempDir(), false)
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: doc(t, withGlobals)})
	if err != nil {
		t.Fatal(err)
	}
	var warned []string
	for _, is := range rep.GetErrors() { // all findings; warnings carry ISSUE_SEVERITY_WARNING
		if is.GetRule() == "agent.unsupported-field" && is.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_WARNING && strings.HasPrefix(is.GetPointer(), "/routing/neighbors/") {
			warned = append(warned, is.GetPointer())
		}
	}
	if strings.Join(warned, ",") != "/routing/neighbors/dad,/routing/neighbors/ipv6Limits" {
		t.Fatalf("findings %v", rep.GetErrors())
	}
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "g0", DesiredState: doc(t, withGlobals)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(v.CallsNamed("ip_neighbor_config")) != 0 || len(v.CallsNamed("ip6_dad_enable_disable")) != 0 {
		t.Fatal("a non-owner changed a VPP-wide setting")
	}
	// the globals owner (product agent): applied, retrieved, converged
	v = coretest.New()
	g := newSvcGlobals(t, v, t.TempDir(), true)
	mustStatus(t, apply(t, g, &vrxv1.ApplyRequest{TxnId: "g1", DesiredState: doc(t, withGlobals)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, err := g.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"routing"}})
	if err != nil {
		t.Fatal(err)
	}
	sameDoc(t, "globals owner Retrieve", got.GetDesiredState(), `{"routing": {"neighbors": {"ipv6Limits": {"maxNumber": 20000, "maxAgeSec": 60, "recycle": true}, "dad": {"transmits": 3, "delayMs": 250}}}}`)
	resp := apply(t, g, &vrxv1.ApplyRequest{TxnId: "g2", DesiredState: doc(t, withGlobals)})
	if sm := resp.GetSummary(); sm.GetCreated()+sm.GetUpdated()+sm.GetDeleted() != 0 {
		t.Fatalf("not converged: %v", sm)
	}
	// removing them restores VPP's defaults (limits) and disables DAD
	mustStatus(t, apply(t, g, &vrxv1.ApplyRequest{TxnId: "g3", DesiredState: doc(t, `{"routing": {}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, _ = g.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"routing"}})
	sameDoc(t, "after removal", got.GetDesiredState(), `{"routing": {}}`)
	subsystems.Register(scheduler.NewRegistry(), subsystems.Env{Client: v, Owner: testOwner, StateDir: t.TempDir()}) //nolint:errcheck // reset the process-wide projection options for the next test
}

func TestNeighborsRaProxyNdOptIn(t *testing.T) {
	const nd = `{"interfaces": {"loop701": {"ipv6": ["2001:db8:7:1::1/64"], "proxyNd": ["2001:db8:7:1::99"]}}}`
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "p0", DesiredState: doc(t, nd)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(v.CallsNamed("ip6nd_proxy_add_del")) != 0 {
		t.Fatal("proxy ND applied without the opt-in (V12)")
	}
	t.Setenv("VRX_DF2_PROXY_ND", "1")
	v = coretest.New()
	s = newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, nd)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(v.CallsNamed("ip6nd_proxy_add_del")) != 1 {
		t.Fatal("opt-in proxy ND not applied")
	}
	got, _ := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces"}})
	if !strings.Contains(protojson.Format(got.GetDesiredState()), `"2001:db8:7:1::99"`) {
		t.Fatalf("retrieve %s", protojson.Format(got.GetDesiredState()))
	}
}

func TestNeighborsRaValidationFailureRollsBack(t *testing.T) {
	// the second RA prefix is not IPv6 → projection error with a pointer; nothing applied
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "v1", DesiredState: doc(t, `{"interfaces": {"loop701": {"ipv6": ["2001:db8:7:1::1/64"],
	  "ipv6Ra": {"suppress": false, "prefixes": {"10.0.0.0/24": {}}}}}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_FAILED)
	var ptrs []string
	for _, is := range resp.GetValidation().GetErrors() {
		if is.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
			ptrs = append(ptrs, is.GetPointer())
		}
	}
	if strings.Join(ptrs, ",") != "/interfaces/loop701/ipv6Ra/prefixes/10.0.0.0~124" {
		t.Fatalf("pointers %v", ptrs)
	}
	if _, ok := v.InterfaceByName("loop701"); ok {
		t.Fatal("an invalid transaction created something")
	}
}

func mustPrefix(t *testing.T, s string) ip_types.Prefix {
	t.Helper()
	p, err := netip.ParsePrefix(s)
	if err != nil {
		t.Fatal(err)
	}
	return df2.ToPrefix(p)
}

// ---- RPCs

type actionStream struct {
	grpc.ServerStreamingServer[vrxv1.ActionOutput]
	ctx context.Context
	out []*vrxv1.ActionOutput
}

func (a *actionStream) Context() context.Context         { return a.ctx }
func (a *actionStream) Send(o *vrxv1.ActionOutput) error { a.out = append(a.out, o); return nil }

func TestListNeighborsAndArpFlushRPCs(t *testing.T) {
	v := coretest.New()
	m := coretest.NeighborsRaOf(v)
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "l1", DesiredState: doc(t, nraAgentDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	v.AddInterface("loop301", "Loopback", "w3:loop301")
	m.Learn(v, "loop701", "10.7.1.77", "02:00:00:00:07:77", 3)
	m.Learn(v, "host-w7l0.100", "2001:db8:7:100::7", "02:00:00:00:71:07", 4)
	m.Learn(v, "loop301", "10.3.1.7", "02:00:00:00:03:07", 5)
	g := &server{svc: s, log: s.log}
	ctx := context.Background()

	resp, err := g.ListNeighbors(ctx, &vrxv1.ListNeighborsRequest{Owner: testOwner})
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, n := range resp.GetNeighbors() {
		rows = append(rows, n.GetInterface()+" "+n.GetIp()+" "+n.GetState()+" "+n.GetVrf())
	}
	want := "host-w7l0 2001:db8:7:2::50 static default|host-w7l0.100 2001:db8:7:100::7 dynamic default|loop701 10.7.1.50 static red|loop701 10.7.1.77 dynamic red"
	if strings.Join(rows, "|") != want || resp.GetTotal() != 4 || resp.GetOwner() != testOwner {
		t.Fatalf("list %v total %d", rows, resp.GetTotal())
	}
	resp, _ = g.ListNeighbors(ctx, &vrxv1.ListNeighborsRequest{Vrf: "red", State: "dynamic"})
	if len(resp.GetNeighbors()) != 1 || resp.GetNeighbors()[0].GetMac() != "02:00:00:00:07:77" || resp.GetNeighbors()[0].GetTableId() != 7001 {
		t.Fatalf("filtered %v", resp.GetNeighbors())
	}
	if _, err := g.ListNeighbors(ctx, &vrxv1.ListNeighborsRequest{Family: "ipx"}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("bad family: %v", err)
	}
	if _, err := g.ListNeighbors(ctx, &vrxv1.ListNeighborsRequest{Owner: "w3"}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("foreign owner: %v", err)
	}

	// flush one interface: learned entries go, the configured static entry stays
	st := &actionStream{ctx: ctx}
	if err := g.Action(&vrxv1.ActionRequest{Action: &vrxv1.ActionRequest_ArpFlush{ArpFlush: &vrxv1.ArpFlushAction{Interface: "loop701"}}}, st); err != nil {
		t.Fatal(err)
	}
	last := st.out[len(st.out)-1].GetDone()
	if last == nil || last.GetExitCode() != 0 || last.GetStats()["deleted"] != "1" || st.out[0].GetLine() != "loop701 ipv4: deleted 1 learned entries" {
		t.Fatalf("flush output %v", st.out)
	}
	if got := m.Neighbors(v, "loop701"); len(got) != 1 || !strings.HasSuffix(got[0], "static") {
		t.Fatalf("after flush %v", got)
	}
	// flush everything configured: the sub-interface too, never another owner's interface
	st = &actionStream{ctx: ctx}
	if err := g.Action(&vrxv1.ActionRequest{Action: &vrxv1.ActionRequest_ArpFlush{ArpFlush: &vrxv1.ArpFlushAction{}}}, st); err != nil {
		t.Fatal(err)
	}
	if d := st.out[len(st.out)-1].GetDone(); d.GetStats()["deleted"] != "1" || d.GetStats()["interfaces"] != "3" {
		t.Fatalf("flush all %v", st.out)
	}
	if len(m.Neighbors(v, "loop301")) != 1 || len(m.Neighbors(v, "host-w7l0.100")) != 0 {
		t.Fatal("flush all touched the wrong interfaces")
	}
	// a foreign interface is refused
	err = g.Action(&vrxv1.ActionRequest{Action: &vrxv1.ActionRequest_ArpFlush{ArpFlush: &vrxv1.ArpFlushAction{Interface: "loop301"}}}, &actionStream{ctx: ctx})
	if grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("foreign flush: %v", err)
	}
	// Retrieve is unchanged by the flush (static entries are configuration)
	sameDoc(t, "Retrieve after flush", retrieveNra(t, s), nraAgentCanonical)
	if m.AllCalls() != 0 || len(v.CallsNamed("ip_neighbor_flush")) != 0 {
		t.Fatal("~0 or ip_neighbor_flush used")
	}
}
