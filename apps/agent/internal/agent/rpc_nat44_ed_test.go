package agent

import (
	"context"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/nat_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// F-nat44-ed-sessions: the `nat` domain through the whole service on the fake VPP (slot owner w7, not the globals
// owner): commit → Retrieve == canonical desired → re-apply empty → rollback removes every NAT object (the plugin
// stays enabled, D-071) → loss + resync recreates; NatSessions / NatSummary / the kill action over real gRPC.

const natIfDoc = `{
  "vrfs": {"cust": {"id": 7001}},
  "interfaces": {
    "host-w7l0": {"enabled": true, "ipv4": ["10.7.1.1/24"]},
    "host-w7w0": {"enabled": true, "ipv4": ["10.7.2.1/24"]}
  }%s
}`

const natPart = `,
  "nat": {
    "mode": "ed",
    "inside": ["host-w7l0"],
    "outside": ["host-w7w0"],
    "forwarding": false,
    "staticMappingOnly": false,
    "connectionTracking": false,
    "timeouts": {"udp": 300, "tcpEstablished": 7440, "tcpTransitory": 240, "icmp": 60},
    "pools": [{"name": "out", "range": "10.7.2.100-10.7.2.103", "twiceNat": false}, {"name": "wan", "interface": "host-w7w0", "twiceNat": false}],
    "staticMappings": [
      {"name": "web", "protocol": "tcp", "local": {"ip": "10.7.1.2", "port": 80}, "external": {"ip": "10.7.2.110", "port": 8080}},
      {"name": "one2one", "local": {"ip": "10.7.1.3"}, "external": {"ip": "10.7.2.111"}}
    ],
    "identityMappings": [{"ip": "10.7.2.112", "protocol": "udp", "port": 500}],
    "ipfix": {"enabled": false},
    "nat64": {"enabled": false, "timeouts": {"udp": 300}}
  }`

const canonicalNat = `{
  "mode": "ed",
  "inside": ["host-w7l0"],
  "outside": ["host-w7w0"],
  "pools": [{"range": "10.7.2.100-10.7.2.103", "twiceNat": false}, {"interface": "host-w7w0", "twiceNat": false}],
  "staticMappings": [
    {"name": "one2one", "local": {"ip": "10.7.1.3"}, "external": {"ip": "10.7.2.111"}, "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false},
    {"name": "web", "protocol": "tcp", "local": {"ip": "10.7.1.2", "port": 80}, "external": {"ip": "10.7.2.110", "port": 8080}, "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false}
  ],
  "identityMappings": [{"ip": "10.7.2.112", "protocol": "udp", "port": 500}]
}`

func natNat(t *testing.T, s *Service) *vrxv1.NatConfig {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"nat"}})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState().GetNat()
}

func TestNatDomainOnFake(t *testing.T) {
	v := coretest.New()
	n := v.Nat44ED()
	s := newSvc(t, v, t.TempDir())
	withNat := doc(t, strings.Replace(natIfDoc, "%s", natPart, 1))

	// the plugin is off: the non-owner's requirement fails, nothing of NAT is applied (the transaction rolls back)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "n0", DesiredState: withNat})
	if resp.GetStatus() == vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || !strings.Contains(protojson.Format(resp), "not the globals owner") {
		t.Fatalf("apply with the plugin off: %s", protojson.Format(resp))
	}
	if !n.Empty() {
		t.Fatal("NAT objects left after the failed apply")
	}

	// the plugin fixture (a test slot never enables it, D-071), then the commit
	n.NatEnable()
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "n1", DesiredState: withNat})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	ptrs := map[string]string{}
	for _, r := range resp.GetResults() {
		ptrs[r.GetKey()] = r.GetPointer() + " " + r.GetSubsystem()
	}
	for k, want := range map[string]string{
		"nat44-ed.enable/global":                        "/nat nat",
		"nat44-ed.interface-feature/host-w7l0/inside":   "/nat/inside/0 nat",
		"nat44-ed.address-pool/10.7.2.100-10.7.2.103/0": "/nat/pools/0 nat",
		"nat44-ed.interface-address/host-w7w0":          "/nat/pools/1 nat",
		"nat44-ed.static-mapping/web":                   "/nat/staticMappings/0 nat",
	} {
		if ptrs[k] != want {
			t.Errorf("result %s = %q, want %q", k, ptrs[k], want)
		}
	}
	if got, want := natNat(t, s), doc(t, `{"nat":`+canonicalNat+`}`).GetNat(); !proto.Equal(got, want) {
		t.Fatalf("Retrieve nat != canonical desired:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	// idempotent: an empty plan; DryRun reports the not-applied translators as warnings (drift skips them)
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "n2", DesiredState: withNat})
	if len(resp.GetResults()) != 0 {
		t.Fatalf("re-apply changed %v", resp.GetResults())
	}
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{DesiredState: withNat})
	if err != nil || !rep.GetOk() {
		t.Fatalf("dry run %v %v", err, rep)
	}
	var warns []string
	for _, e := range rep.GetErrors() {
		warns = append(warns, e.GetPointer()+" "+e.GetRule())
	}
	for _, w := range []string{"/nat/ipfix agent.unsupported-field"} { // nat64 {enabled: false} programs nothing (F-nat44-ei-64-66-nptv6)
		if !strings.Contains(strings.Join(warns, ","), w) {
			t.Errorf("dry run lacks %q: %v", w, warns)
		}
	}

	// loss behind the agent's back (dependents first, D-095c) + resync → recreated without an Apply
	n.Lock()
	n.Statics, n.Addrs, n.Idents = nil, nil, nil
	n.Features = map[uint32]nat_types.NatConfigFlags{}
	n.Unlock()
	s.Resync(context.Background())
	if got, want := natNat(t, s), doc(t, `{"nat":`+canonicalNat+`}`).GetNat(); !proto.Equal(got, want) {
		t.Fatalf("after resync:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}

	// rollback to a document without nat objects: every NAT object leaves VPP (Retrieve, not assumption); the plugin
	// stays enabled (a non-owner never disables it, D-071)
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "n3", DesiredState: doc(t, strings.Replace(natIfDoc, "%s", `, "nat": {}`, 1))})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if got := natNat(t, s); proto.Size(got) != 0 {
		t.Fatalf("Retrieve after rollback: %s", protojson.Format(got))
	}
	if !n.Empty() || !n.Enabled {
		t.Fatalf("after rollback: empty %v enabled %v", n.Empty(), n.Enabled)
	}
}

func natServer(t *testing.T, s *Service) vrxv1.DataplaneClient {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "a.sock")
	l, err := listenUnix(sock, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	vrxv1.RegisterDataplaneServer(g, &server{svc: s, stats: fakeStats{}, log: s.log})
	go func() { _ = g.Serve(l) }()
	t.Cleanup(g.Stop)
	cc, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	return vrxv1.NewDataplaneClient(cc)
}

func kill(t *testing.T, c vrxv1.DataplaneClient, a *vrxv1.NatSessionKillAction) ([]*vrxv1.ActionOutput, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := c.Action(ctx, &vrxv1.ActionRequest{Action: &vrxv1.ActionRequest_NatSessionKill{NatSessionKill: a}})
	if err != nil {
		return nil, err
	}
	var out []*vrxv1.ActionOutput
	for {
		o, err := st.Recv()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, o)
	}
}

func TestNatSessionsSummaryKillOverGRPC(t *testing.T) {
	v := coretest.New()
	n := v.Nat44ED()
	n.NatEnable()
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "k1", DesiredState: doc(t, strings.Replace(natIfDoc, "%s", natPart, 1))}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	for u := 0; u < 21; u++ { // 2 100 sessions of w7's users, 2 of another slot's
		for i := 0; i < 100; i++ {
			n.AddNatSession(0, 6, "10.7.1."+strconv.Itoa(10+u), uint16(10000+i), "10.7.2.100", uint16(20000+u*100+i), "10.7.2.2", 80) //nolint:gosec // test data
		}
	}
	n.AddNatSession(7001, 17, "10.7.1.10", 5353, "10.7.2.101", 6000, "10.7.2.3", 53)
	n.AddNatSession(0, 6, "10.9.1.5", 1, "10.9.2.100", 2, "10.9.2.2", 3)
	c := natServer(t, s)
	ctx := context.Background()

	// paged and bounded: pageSize 100 never returns more than 100, the message stays small
	r, err := c.NatSessions(ctx, &vrxv1.NatSessionsRequest{Limit: 100, Offset: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.GetSessions()) != 100 || r.GetTotalSessions() != 2101 || r.GetTotalUsers() != 22 || r.GetNextOffset() != 300 || r.GetOwner() != testOwner {
		t.Fatalf("page: %d sessions total %d users %d next %d", len(r.GetSessions()), r.GetTotalSessions(), r.GetTotalUsers(), r.GetNextOffset())
	}
	if sz := proto.Size(r); sz > 100*200 {
		t.Fatalf("a 100-session page is %d bytes", sz)
	}
	first := r.GetSessions()[0]
	if first.GetInsideAddress() != "10.7.1.12" || first.GetProtocol() != "tcp" || first.GetVrf() != "default" || first.GetExternalAddress() != "10.7.2.2" || first.GetPackets() != 2 {
		t.Fatalf("session %v", first)
	}
	// filter by VRF name (the table's configured name) and by protocol
	r, err = c.NatSessions(ctx, &vrxv1.NatSessionsRequest{Filter: &vrxv1.NatSessionFilter{Vrf: proto.String("cust")}})
	if err != nil || r.GetTotalSessions() != 1 || r.GetSessions()[0].GetVrf() != "cust" || r.GetSessions()[0].GetTableId() != 7001 {
		t.Fatalf("vrf filter %v %v", r, err)
	}
	r, err = c.NatSessions(ctx, &vrxv1.NatSessionsRequest{Filter: &vrxv1.NatSessionFilter{Protocol: proto.String("udp")}})
	if err != nil || r.GetTotalSessions() != 1 {
		t.Fatalf("protocol filter %v %v", r, err)
	}
	for name, req := range map[string]*vrxv1.NatSessionsRequest{
		"limit":  {Limit: 1001},
		"owner":  {Owner: "w3"},
		"filter": {Filter: &vrxv1.NatSessionFilter{InsideAddress: proto.String("nope")}},
		"vrf":    {Filter: &vrxv1.NatSessionFilter{Vrf: proto.String("nope")}},
	} {
		if _, err := c.NatSessions(ctx, req); grpcCode(err) != codes.InvalidArgument {
			t.Errorf("%s: %v", name, err)
		}
	}

	// summary: totals from the user dump, per pool from the scan
	sum, err := c.NatSummary(ctx, &vrxv1.NatSummaryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !sum.GetEnabled() || sum.GetSessionLimit() != 63*1024 || sum.GetTotalSessions() != 2101 || len(sum.GetPools()) != 2 ||
		sum.GetPools()[0].GetFirstAddress() != "10.7.2.100" || sum.GetPools()[0].GetAddresses() != 4 || sum.GetPools()[0].GetSessions() != 2101 ||
		sum.GetPools()[1].GetInterface() != "host-w7w0" || sum.GetPools()[1].GetFirstAddress() != "10.7.2.1" || sum.GetSessionsByProtocol()["tcp"] != 2100 {
		t.Fatalf("summary %s", protojson.Format(sum))
	}

	// kill: done exit 0, then 1 (gone); a foreign or malformed request is INVALID_ARGUMENT before any output
	k := &vrxv1.NatSessionKillAction{Protocol: "tcp", InsideAddress: "10.7.1.10", InsidePort: 10005, ExternalAddress: "10.7.2.2", ExternalPort: 80}
	out, err := kill(t, c, k)
	if err != nil || len(out) != 1 || out[0].GetDone().GetExitCode() != 0 || out[0].GetDone().GetStats()["inside_port"] != "10005" {
		t.Fatalf("kill: %v %v", out, err)
	}
	if n.SessionCount() != 2102-1 {
		t.Fatalf("sessions after kill: %d", n.SessionCount())
	}
	out, err = kill(t, c, k)
	if err != nil || len(out) != 1 || out[0].GetDone().GetExitCode() != 1 || !strings.Contains(out[0].GetDone().GetSummary(), "no such session") {
		t.Fatalf("second kill: %v %v", out, err)
	}
	if _, err := kill(t, c, &vrxv1.NatSessionKillAction{Protocol: "tcp", InsideAddress: "10.9.1.5", InsidePort: 1, ExternalAddress: "10.9.2.2", ExternalPort: 3}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("foreign kill: %v", err)
	}
	if _, err := kill(t, c, &vrxv1.NatSessionKillAction{Protocol: "tcp", InsideAddress: "10.7.1.10", InsidePort: 1}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("kill without the external endpoint: %v", err)
	}
	// every other action is still Unimplemented (the A4 switch's default)
	st, err := c.Action(ctx, &vrxv1.ActionRequest{Action: &vrxv1.ActionRequest_Ping{Ping: &vrxv1.PingAction{Target: "10.7.2.2"}}})
	if err == nil {
		_, err = st.Recv()
	}
	if grpcCode(err) != codes.Unimplemented {
		t.Fatalf("ping: %v", err)
	}
}

// Review H1: NatSummary is served from a per-agent cache for natSummaryTTL (single flight), and one computation visits
// at most MaxSummaryUserDumps users; NatSessions with a session-level filter visits at most MaxUserDumps users.
func TestNatSummaryCacheAndCaps(t *testing.T) {
	v := coretest.New()
	n := v.Nat44ED()
	n.NatEnable()
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "c1", DesiredState: doc(t, strings.Replace(natIfDoc, "%s", natPart, 1))}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	for u := 0; u < 300; u++ { // 300 inside hosts, one session each
		h := 256 + 10 + u
		n.AddNatSession(0, 6, "10.7."+strconv.Itoa(h/256)+"."+strconv.Itoa(h%256), 10000, "10.7.2.100", uint16(20000+u), "10.7.2.2", 80) //nolint:gosec // test data
	}
	clock := time.Date(2026, 9, 24, 20, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	ctx := context.Background()
	counts := func() (users, sessions int) {
		n.Lock()
		defer n.Unlock()
		return len(v.CallsNamed("nat44_user_dump")), n.SessionDumps
	}

	u0, d0 := counts()
	first, err := s.NatSummary(ctx, &vrxv1.NatSummaryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	u1, d1 := counts()
	if first.GetTotalSessions() != 300 || first.GetTotalUsers() != 300 || !first.GetTruncated() || u1-u0 != 1 || d1-d0 != natSummaryCaps.UserDumps {
		t.Fatalf("first summary: sessions %d users %d truncated %v, %d user dumps, %d session dumps (cap %d)", first.GetTotalSessions(), first.GetTotalUsers(), first.GetTruncated(), u1-u0, d1-d0, natSummaryCaps.UserDumps)
	}
	// within the TTL: the cached snapshot, no VPP call at all, same retrieved_at
	clock = clock.Add(natSummaryTTL - time.Second)
	n.AddNatSession(0, 6, "10.7.1.9", 1, "10.7.2.100", 1, "10.7.2.2", 80)
	again, err := s.NatSummary(ctx, &vrxv1.NatSummaryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if u2, d2 := counts(); u2 != u1 || d2 != d1 || !proto.Equal(again, first) {
		t.Fatalf("cached summary asked VPP again (%d/%d user dumps, %d/%d session dumps) or changed", u1, u2, d1, d2)
	}
	// concurrent callers share one computation (single flight)
	clock = clock.Add(2 * time.Second) // expired
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r, err := s.NatSummary(ctx, &vrxv1.NatSummaryRequest{}); err != nil || r.GetTotalSessions() != 301 {
				t.Errorf("concurrent summary %v %v", err, r.GetTotalSessions())
			}
		}()
	}
	wg.Wait()
	if u3, d3 := counts(); u3-u1 != 1 || d3-d1 != natSummaryCaps.UserDumps {
		t.Fatalf("8 concurrent callers after the TTL: %d user dumps, %d session dumps (want 1 and %d)", u3-u1, d3-d1, natSummaryCaps.UserDumps)
	}
	if r, _ := s.NatSummary(ctx, &vrxv1.NatSummaryRequest{}); !r.GetRetrievedAt().AsTime().Equal(clock) {
		t.Fatalf("retrieved_at %v, want the refresh time %v", r.GetRetrievedAt().AsTime(), clock)
	}

	// NatSessions with a session-level filter: at most MaxUserDumps users per call, truncated
	_, d4 := counts()
	r, err := s.NatSessions(ctx, &vrxv1.NatSessionsRequest{Filter: &vrxv1.NatSessionFilter{Port: proto.Uint32(80)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, d5 := counts(); d5-d4 != natCaps.UserDumps || !r.GetTruncated() || len(r.GetSessions()) != 100 {
		t.Fatalf("filtered NatSessions: %d session dumps (cap %d), truncated %v, %d sessions", d5-d4, natCaps.UserDumps, r.GetTruncated(), len(r.GetSessions()))
	}
}

// D-132: one VPP session walk at a time per agent — concurrent NatSessions and NatSummary calls never dump sessions
// in parallel (each nat44_user_session_v3_dump holds the worker barrier on a real VPP).
func TestNatWalksAreSerialised(t *testing.T) {
	v := coretest.New()
	n := v.Nat44ED()
	n.NatEnable()
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "w1", DesiredState: doc(t, strings.Replace(natIfDoc, "%s", natPart, 1))}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	for u := 0; u < 20; u++ {
		n.AddNatSession(0, 6, "10.7.1."+strconv.Itoa(10+u), 10000, "10.7.2.100", uint16(20000+u), "10.7.2.2", 80) //nolint:gosec // test data
	}
	// replace the model's session dump with a slow one that measures how many run at once
	var mu sync.Mutex
	inflight, peak, dumps := 0, 0, 0
	v.On("nat44_user_session_v3_dump", func(api.Message) ([]api.Message, error) {
		mu.Lock()
		inflight++
		dumps++
		peak = max(peak, inflight)
		mu.Unlock()
		time.Sleep(3 * time.Millisecond)
		mu.Lock()
		inflight--
		mu.Unlock()
		return nil, nil
	})
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := s.NatSessions(ctx, &vrxv1.NatSessionsRequest{Filter: &vrxv1.NatSessionFilter{Protocol: proto.String("tcp")}}); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := s.NatSessions(ctx, &vrxv1.NatSessionsRequest{Limit: 50}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := s.NatSummary(ctx, &vrxv1.NatSummaryRequest{}); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	if peak != 1 || dumps < 20 {
		t.Fatalf("%d session dumps, at most %d at once (want 1)", dumps, peak)
	}
	// a caller whose deadline passes while another walk runs gives up instead of queueing forever
	release, err := s.natWalk(ctx)
	if err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := s.NatSessions(short, &vrxv1.NatSessionsRequest{}); grpcCode(err) != codes.DeadlineExceeded {
		t.Fatalf("NatSessions while the walk slot is taken: %v", err)
	}
	release()
}
