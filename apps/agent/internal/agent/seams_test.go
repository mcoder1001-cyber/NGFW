package agent

// TD-8 (D-119 M2): the agent seams — feature events reach StreamEvents, Env.Resync reaches
// Service.Resync, the id range goes through Env and fails closed, dynamic desired sources (S1) are
// merged into every transaction under the txn lock, and feature metrics collectors reach /metrics.
// Default inert (no hook, no source, no collector: nothing changes) is covered here and by every
// other test of this package, which runs without them.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
)

// ---- Start's wiring (fake VPP) ---------------------------------------------------------------

// startFake runs Start against a fake VPP (dialVPP) and returns the agent and its connection.
func startFake(t *testing.T, cfg Config) (*Agent, *fakeConn) {
	t.Helper()
	fc := &fakeConn{VPP: coretest.New(), states: make(chan vpp.ConnState, 4)}
	old := dialVPP
	dialVPP = func(string, vpp.ConnOptions) vppConn { return fc }
	t.Cleanup(func() { dialVPP = old })
	a, err := Start(context.Background(), cfg, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Stop)
	return a, fc
}

// noEvent fails when sub delivers anything within d.
func noEvent(t *testing.T, sub *subscriber, d time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	if evs, err := sub.next(ctx); err == nil {
		t.Fatalf("unexpected events %s", kinds(evs))
	}
}

func TestStartWiresFeatureEventsAndResync(t *testing.T) {
	cfg := testConfig(t)
	cfg.IDs = subsystems.IDScope{Range: &subsystems.IDRange{Lo: 7000, Hi: 7999}}
	a, fc := startFake(t, cfg)
	sub := a.svc.events().subscribe(&vrxv1.StreamEventsRequest{})

	// Env.Publish → the service's bus → every StreamEvents subscriber, as a copy with its own seq.
	ev := &vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_LINK_UP, Interface: proto.String("loop701"), Message: "feature", Seq: 99, Attributes: map[string]string{"k": "v"}}
	a.wiring.Publish(ev)
	ev.Message = "changed after publish"
	a.wiring.Publish(&vrxv1.Event{Message: "unspecified kind is dropped"})
	a.wiring.Publish(nil)
	got := collect(t, sub, 1)
	if len(got) != 1 || got[0].GetMessage() != "feature" || got[0].GetSeq() != 1 || got[0].GetTs() == nil || got[0].GetInterface() != "loop701" || got[0].GetAttributes()["k"] != "v" {
		t.Fatalf("feature event: %v", got)
	}
	noEvent(t, sub, 100*time.Millisecond)

	// Env.Resync while VPP is disconnected: dropped (the connect resyncs anyway), never blocks.
	for range 5 {
		a.wiring.RequestResync()
	}
	noEvent(t, sub, 200*time.Millisecond)
	fc.states <- vpp.ConnState{Connected: true}
	if k := kinds(collect(t, sub, 3)); k != "VPP_CONNECTED:,RECONCILE_START:,RECONCILE_DONE:" {
		t.Fatalf("connect: %s", k)
	}
	// Env.Resync while connected → Service.Resync (RECONCILE_START/DONE with an empty txn id).
	a.wiring.RequestResync()
	evs := collect(t, sub, 2)
	if k := kinds(evs); k != "RECONCILE_START:,RECONCILE_DONE:" || !strings.HasPrefix(evs[0].GetMessage(), "resync") {
		t.Fatalf("requested resync: %s %q", k, evs[0].GetMessage())
	}
	noEvent(t, sub, 100*time.Millisecond)

	// The id range goes through Env: the wiring hands the families exactly cfg.IDs.
	if r, err := a.wiring.IDRange(); err != nil || r == nil || *r != (subsystems.IDRange{Lo: 7000, Hi: 7999}) {
		t.Fatalf("wiring id range %v %v", r, err)
	}
}

func TestStartIDRangeFailsClosed(t *testing.T) {
	a, _ := startFake(t, testConfig(t)) // a Config built in code: zero IDs
	if r, err := a.wiring.IDRange(); r == nil || !r.Empty() || !errors.Is(err, subsystems.ErrNoIDRange) {
		t.Fatalf("zero IDs: %v %v (want the empty range and ErrNoIDRange)", r, err)
	}
}

func TestConfigFromEnvIDRange(t *testing.T) {
	t.Setenv(subsystems.EnvTableBase, "")
	t.Setenv(subsystems.EnvIDRange, "")
	cfg := ConfigFromEnv()
	// TD-8b (D-129 Q3): neither variable set refuses start-up; the error cites the host rule (V5: §12,
	// §11 is the trace ban).
	if err := cfg.Validate(); cfg.IDs != (subsystems.IDScope{}) || !errors.Is(err, subsystems.ErrNoIDRange) ||
		!strings.Contains(err.Error(), "shared-host-rules.md §12") || strings.Contains(err.Error(), "§11") {
		t.Fatalf("unset: %+v %v (want no id and start-up refused with ErrNoIDRange citing §12)", cfg.IDs, err)
	}
	t.Setenv(subsystems.EnvTableBase, "3000")
	if cfg := ConfigFromEnv(); cfg.IDs.Range == nil || *cfg.IDs.Range != (subsystems.IDRange{Lo: 3000, Hi: 3999}) || cfg.Validate() != nil {
		t.Fatalf("slot 3: %+v", cfg.IDs)
	}
	t.Setenv(subsystems.EnvTableBase, "")
	t.Setenv(subsystems.EnvIDRange, "all")
	if cfg := ConfigFromEnv(); !cfg.IDs.All || cfg.IDs.Range != nil || cfg.Validate() != nil {
		t.Fatalf("all: %+v", cfg.IDs)
	}
	for _, bad := range [][2]string{{"3000", "all"}, {"0", ""}, {"x", ""}, {"", "ALL"}, {"", "1000-1999"}} {
		t.Setenv(subsystems.EnvTableBase, bad[0])
		t.Setenv(subsystems.EnvIDRange, bad[1])
		cfg := ConfigFromEnv()
		if cfg.Validate() == nil || cfg.IDs != (subsystems.IDScope{}) {
			t.Errorf("%s=%q %s=%q: start allowed with %+v", subsystems.EnvTableBase, bad[0], subsystems.EnvIDRange, bad[1], cfg.IDs)
		}
	}
}

// ---- metrics collectors ------------------------------------------------------------------------

func TestMetricsCollectors(t *testing.T) {
	old := collectorTimeout
	collectorTimeout = 200 * time.Millisecond
	t.Cleanup(func() { collectorTimeout = old })
	a, _ := startFake(t, testConfig(t))
	scrape := func() string {
		rec := httptest.NewRecorder()
		a.metrics.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("scrape: %d", rec.Code)
		}
		return rec.Body.String()
	}
	base := scrape()
	if strings.Contains(base, "collector") {
		t.Fatalf("no collector: the exposition must be unchanged\n%s", base)
	}
	fail := true
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(a.wiring.AddMetricsCollector(subsystems.MetricsCollector{Name: "zeta", Collect: func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "# HELP vrx_zeta_up Test.\n# TYPE vrx_zeta_up gauge\nvrx_zeta_up 1") // no trailing newline
		return err
	}}))
	must(a.wiring.AddMetricsCollector(subsystems.MetricsCollector{Name: "alpha", Collect: func(_ context.Context, w io.Writer) error {
		_, _ = io.WriteString(w, "vrx_alpha_partial 1\n")
		if fail {
			return errors.New("stats segment unavailable")
		}
		return nil
	}}))
	must(a.wiring.AddMetricsCollector(subsystems.MetricsCollector{Name: "slow", Collect: func(ctx context.Context, w io.Writer) error {
		_, _ = io.WriteString(w, "vrx_slow_partial 1\n")
		<-ctx.Done() // honours its deadline: the scrape's context ends first here
		return ctx.Err()
	}}))
	must(a.wiring.AddMetricsCollector(subsystems.MetricsCollector{Name: "boom", Collect: func(context.Context, io.Writer) error {
		panic("collector bug") // R3: contained, counted as an error
	}}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the scraper went away: slow is dropped, the others still run
	var b strings.Builder
	a.metrics.writeCtx(ctx, &b)
	out := b.String()
	if !strings.HasPrefix(out, base) {
		t.Fatal("the agent's families changed")
	}
	for _, want := range []string{
		"vrx_zeta_up 1\n# HELP vrx_agent_metrics_collector_errors_total",
		`vrx_agent_metrics_collector_errors_total{collector="alpha"} 1`,
		`vrx_agent_metrics_collector_errors_total{collector="boom"} 1`,
		`vrx_agent_metrics_collector_errors_total{collector="slow"} 1`,
		`vrx_agent_metrics_collector_errors_total{collector="zeta"} 0`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "vrx_alpha_partial") || strings.Contains(out, "vrx_slow_partial") {
		t.Fatalf("a failed collector's output was served\n%s", out)
	}
	fail = false
	out = scrape() // through the HTTP handler: "slow" runs into collectorTimeout
	if !strings.Contains(out, "vrx_alpha_partial 1\n") || !strings.Contains(out, `vrx_agent_metrics_collector_errors_total{collector="alpha"} 1`) ||
		!strings.Contains(out, `vrx_agent_metrics_collector_errors_total{collector="slow"} 2`) || strings.Contains(out, "vrx_slow_partial") {
		t.Fatalf("second scrape\n%s", out)
	}
}

// ---- dynamic desired sources (S1) ------------------------------------------------------------

const dynDesc = "test.dyn"

// memDesc is an in-memory descriptor instance of no domain: one object per interface name, which
// depends on that loopback (a dynamic object with a configuration dependency).
type memDesc struct {
	mu       sync.Mutex
	objs     map[string]bool
	fail     map[string]error // Create of these names fails (VPP rejects the object)
	failDel  map[string]error // Delete of these names fails (VPP refuses to delete it; TD-8b)
	onCreate func()           // called by Create outside mu (a descriptor that calls sync)
}

func (d *memDesc) Name() string { return dynDesc }
func (d *memDesc) KeyOf(o proto.Message) scheduler.Key {
	return scheduler.Join(dynDesc, o.(*wrapperspb.StringValue).GetValue())
}
func (d *memDesc) Dependencies(o proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{{Key: scheduler.Join(core.LoopbackName, o.(*wrapperspb.StringValue).GetValue())}}
}
func (d *memDesc) Create(_ context.Context, o proto.Message) (any, error) {
	d.mu.Lock()
	hook := d.onCreate
	d.mu.Unlock()
	if hook != nil {
		hook()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	name := o.(*wrapperspb.StringValue).GetValue()
	if err := d.fail[name]; err != nil {
		return nil, err
	}
	d.objs[name] = true
	return nil, nil
}
func (d *memDesc) failOn(name string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail == nil {
		d.fail = map[string]error{}
	}
	if err == nil {
		delete(d.fail, name)
		return
	}
	d.fail[name] = err
}
func (d *memDesc) Update(_ context.Context, _, _ proto.Message, meta any) (any, error) {
	return meta, nil
}
func (d *memDesc) Delete(_ context.Context, o proto.Message, _ any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	name := o.(*wrapperspb.StringValue).GetValue()
	if err := d.failDel[name]; err != nil {
		return err
	}
	delete(d.objs, name)
	return nil
}
func (d *memDesc) failDeleteOn(name string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failDel == nil {
		d.failDel = map[string]error{}
	}
	if err == nil {
		delete(d.failDel, name)
		return
	}
	d.failDel[name] = err
}
func (d *memDesc) Retrieve(context.Context) ([]scheduler.KV, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []scheduler.KV
	for v := range d.objs {
		out = append(out, scheduler.KV{Key: scheduler.Join(dynDesc, v), Value: wrapperspb.String(v)})
	}
	return out, nil
}
func (d *memDesc) list() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	for v := range d.objs {
		out = append(out, v)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}
func (d *memDesc) drop(v string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.objs, v)
}

// learnedSource is a feature's cached daemon state (e.g. FRR's labels): the interface names it
// learned. Desired keeps those whose loopback the document still has.
type learnedSource struct {
	mu        sync.Mutex
	learned   map[string]bool
	extra     []scheduler.KV // a buggy source: keys it must not produce
	views     []*vrxv1.DesiredState
	panicNow  bool   // a buggy source: Desired panics
	onDesired func() // called by Desired before it takes mu (a source that calls sync from Desired)
}

func (s *learnedSource) set(names ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.learned = map[string]bool{}
	for _, n := range names {
		s.learned[n] = true
	}
}

func (s *learnedSource) desired(doc *vrxv1.DesiredState) []scheduler.KV {
	s.mu.Lock()
	hook := s.onDesired
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.panicNow {
		panic("assignment to entry in nil map") // the probe-C bug: a nil map in the source cache
	}
	s.views = append(s.views, doc)
	var out []scheduler.KV
	for n := range s.learned {
		if _, ok := doc.GetInterfaces()[n]; ok {
			out = append(out, scheduler.KV{Key: scheduler.Join(dynDesc, n), Value: wrapperspb.String(n)})
		}
	}
	return append(out, s.extra...)
}

func (s *learnedSource) lastView() *vrxv1.DesiredState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.views[len(s.views)-1]
}

// newSrcSvc is newSvc with the memDesc instance registered and one dynamic source over it, added
// through the wiring (AddDynamicSource → DynamicSources → ServiceConfig.Sources), as agent.Start does.
func newSrcSvc(t *testing.T, v *coretest.VPP, run func(context.Context, subsystems.SyncFunc)) (*Service, *memDesc, *learnedSource) {
	t.Helper()
	md := &memDesc{objs: map[string]bool{}}
	s, src := newSrcSvcIn(t, v, t.TempDir(), md, run)
	return s, md, src
}

// newSrcSvcIn builds the service over state dir and the dynamic objects md (the "VPP" of test.dyn), so
// a second call over the same dir and md is an agent restart with VPP intact (a cold source cache).
func newSrcSvcIn(t *testing.T, v *coretest.VPP, dir string, md *memDesc, run func(context.Context, subsystems.SyncFunc)) (*Service, *learnedSource) {
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
	w.Connected(context.Background())
	reg.Register(md)
	src := &learnedSource{learned: map[string]bool{}}
	if err := w.AddDynamicSource(subsystems.DynamicSource{Name: "test-sync", Descriptors: []string{dynDesc}, Desired: src.desired, Run: run}); err != nil {
		t.Fatal(err)
	}
	sched := scheduler.New(reg, nil)
	sched.VerifyRetries = 0
	svc, err := NewService(ServiceConfig{Owner: testOwner, Version: "test", VPP: v, Scheduler: sched, StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind(), Sources: w.DynamicSources()})
	if err != nil {
		t.Fatal(err)
	}
	svc.retryMin, svc.retryMax = time.Hour, time.Hour
	t.Cleanup(svc.Close)
	return svc, src
}

func TestDynamicSourceMergedIntoEveryTransaction(t *testing.T) {
	v := coretest.New()
	s, md, src := newSrcSvc(t, v, nil)
	src.set("loop701", "loop702", "loop799") // loop799 is not in the document: left out by Desired
	// The source's first sync (nothing to do yet: no loopback) puts it in sync (R1).
	if err := s.sourceSync("test-sync")(context.Background()); err != nil || md.list() != "" {
		t.Fatalf("first sync: %v %q", err, md.list())
	}

	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if md.list() != "loop701,loop702" {
		t.Fatalf("dynamic objects after the config apply: %q", md.list())
	}
	for _, r := range resp.GetResults() {
		if strings.HasPrefix(r.GetKey(), dynDesc+"/") && (r.GetPointer() != "" || r.GetSubsystem() != "") {
			t.Fatalf("a dynamic object has a pointer or a domain: %v", r)
		}
	}
	// The view is the stored document after the transaction, as a copy.
	if !proto.Equal(src.lastView(), s.st.desired) || src.lastView() == s.st.desired {
		t.Fatal("Desired did not get a copy of the post-transaction document")
	}
	// Retrieve is the configuration only.
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{})
	if err != nil || !proto.Equal(got.GetDesiredState(), doc(t, canonicalDoc)) {
		t.Fatalf("retrieve: %v", err)
	}
	// Idempotent: nothing to do on a repeat.
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: doc(t, sampleDoc)})
	if len(resp.GetResults()) != 0 {
		t.Fatalf("repeat apply: %v", resp.GetResults())
	}

	// A config change that removes what a dynamic object depends on deletes both in one transaction
	// (without the merge, the dynamic object would block the loopback's delete).
	noLoop702 := doc(t, sampleDoc)
	delete(noLoop702.Interfaces, "loop702")
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t3", DesiredState: noLoop702}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if md.list() != "loop701" {
		t.Fatalf("after removing loop702: %q", md.list())
	}
	// An Apply authoritative for another domain still carries the dynamic objects (never deleted).
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t4", Subsystems: []string{"routing"}, DesiredState: noLoop702}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if md.list() != "loop701" {
		t.Fatalf("after a routing-only apply: %q", md.list())
	}

	// DryRun plans what Apply would do, dynamic objects included.
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: doc(t, sampleDoc)})
	if err != nil || !rep.GetOk() {
		t.Fatalf("dry run: %v %v", err, rep)
	}
	var planned []string
	for _, op := range rep.GetPlan() {
		planned = append(planned, op.GetKey())
	}
	if !strings.Contains(strings.Join(planned, " "), dynDesc+"/loop702") {
		t.Fatalf("dry run plan misses the dynamic object: %v", planned)
	}

	// Resync repairs a lost dynamic object (VPP restart).
	md.drop("loop701")
	mustStatus(t, s.Resync(context.Background()), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if md.list() != "loop701" {
		t.Fatalf("after resync: %q", md.list())
	}
	// Confirm revert re-merges the source against the confirmed baseline.
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, sampleDoc), ConfirmTimeoutSec: 60}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if md.list() != "loop701,loop702" {
		t.Fatalf("pending: %q", md.list())
	}
	s.revert("p1")
	if md.list() != "loop701" || s.Health().GetPendingConfirmTxnId() != "" {
		t.Fatalf("after the confirm revert: %q", md.list())
	}
}

func TestDynamicSourceSyncIsScoped(t *testing.T) {
	v := coretest.New()
	s, md, src := newSrcSvc(t, v, nil)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if md.list() != "" {
		t.Fatalf("nothing learned yet: %q", md.list())
	}
	stored := proto.Clone(s.st.desired)
	last := s.Health().GetLastTxnId()
	sync := s.sourceSync("test-sync")
	sub := s.events().subscribe(&vrxv1.StreamEventsRequest{})

	src.set("loop701")
	if err := sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if md.list() != "loop701" {
		t.Fatalf("after sync: %q", md.list())
	}
	evs := collect(t, sub, 2)
	if k := kinds(evs); k != "RECONCILE_START:,RECONCILE_DONE:" || evs[0].GetAttributes()["source"] != "test-sync" || evs[1].GetSummary().GetCreated() != 1 || evs[1].GetMessage() != "APPLY_STATUS_APPLIED" {
		t.Fatalf("sync events: %s %v", k, evs)
	}
	// The source stops producing an object → the next sync deletes it.
	src.set()
	if err := sync(context.Background()); err != nil || md.list() != "" {
		t.Fatalf("sync after unlearning: %v %q", err, md.list())
	}
	// Scope: a sync never touches configuration objects — config drift is left to a resync.
	v.DeleteInterface("loop702")
	src.set("loop701")
	if err := sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := v.InterfaceByName("loop702"); ok {
		t.Fatal("a source sync repaired a configuration object")
	}
	// A sync with nothing to do emits no event and counts no reconcile (R7).
	quiet := s.events().subscribe(&vrxv1.StreamEventsRequest{})
	var before strings.Builder
	s.metrics.write(&before)
	if err := sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	noEvent(t, quiet, 100*time.Millisecond)
	var after strings.Builder
	s.metrics.write(&after)
	if before.String() != after.String() {
		t.Fatal("an empty sync changed the metrics")
	}
	if !proto.Equal(s.st.desired, stored) || s.Health().GetLastTxnId() != last {
		t.Fatal("a source sync changed the stored document or the last transaction")
	}
	mustStatus(t, s.Resync(context.Background()), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, ok := v.InterfaceByName("loop702"); !ok || md.list() != "loop701" {
		t.Fatalf("resync: loop702 %v, dynamic %q", ok, md.list())
	}

	// VPP disconnected → UNAVAILABLE (the reconnect resync includes the source); ctx done → its error.
	v.SetConnected(false)
	if err := sync(context.Background()); grpcCode(err) != codes.Unavailable {
		t.Fatalf("disconnected: %v", err)
	}
	v.SetConnected(true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sync(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	if err := s.sourceSync("nope")(context.Background()); err == nil {
		t.Fatal("sync of an unknown source")
	}
}

// A source that produces a key outside its descriptors never fails a config transaction (R2): the
// transaction leaves it out and names it; its own sync fails and applies nothing.
func TestDynamicSourceKeyOutsideItsDescriptorsIsLeftOut(t *testing.T) {
	v := coretest.New()
	s, md, src := syncedSrcSvc(t, v, "loop701")
	bad := scheduler.Join(core.LoopbackName, "loop777")
	src.mu.Lock()
	src.extra = []scheduler.KV{{Key: bad, Value: wrapperspb.String("x")}}
	src.mu.Unlock()
	sub := s.events().subscribe(&vrxv1.StreamEventsRequest{Kinds: []vrxv1.EventKind{vrxv1.EventKind_EVENT_KIND_ERROR}})

	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: withLoop703(t)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, ok := v.InterfaceByName("loop703"); !ok || md.list() != "loop701" {
		t.Fatalf("after the commit: loop703 %v, dynamic %q", ok, md.list())
	}
	last := resp.GetResults()[len(resp.GetResults())-1]
	if last.GetKey() != string(bad) || last.GetCode() != vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_SKIPPED || !strings.Contains(last.GetMessage(), "outside its descriptors") || last.GetPointer() != "" {
		t.Fatalf("the left-out source is not reported: %v", last)
	}
	if ev := collect(t, sub, 1)[0]; ev.GetAttributes()["source"] != "test-sync" || ev.GetAttributes()["reason"] != "invalid" || ev.GetAttributes()["key"] != string(bad) {
		t.Fatalf("ERROR event %v", ev)
	}
	err := s.sourceSync("test-sync")(context.Background())
	if err == nil || !strings.Contains(err.Error(), "outside its descriptors") {
		t.Fatalf("sync: %v", err)
	}
	if md.list() != "loop701" {
		t.Fatalf("a failed sync applied something: %q", md.list())
	}
	var b strings.Builder
	s.metrics.write(&b)
	for _, want := range []string{`vrx_agent_dynamic_source_errors_total{source="test-sync",reason="invalid"} 2`, `vrx_agent_dynamic_source_errors_total{source="test-sync",reason="panic"} 0`} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q in\n%s", want, b.String())
		}
	}
}

func TestNewServiceRefusesUnregisteredSourceDescriptor(t *testing.T) {
	desired := func(*vrxv1.DesiredState) []scheduler.KV { return nil }
	for _, srcs := range [][]subsystems.DynamicSource{
		{{Name: "a", Descriptors: []string{"not.registered"}, Desired: desired}},
		{{Name: "b", Descriptors: []string{core.LoopbackName}}}, // no Desired
	} {
		_, err := NewService(ServiceConfig{Owner: testOwner, StateDir: t.TempDir(), Scheduler: scheduler.New(scheduler.NewRegistry(), nil), Sources: srcs})
		if err == nil {
			t.Fatalf("accepted %+v", srcs)
		}
	}
}

// The dynamic sources' loops start once, after the first resync, and stop with the agent.
func TestDynamicSourceRunLifecycle(t *testing.T) {
	v := coretest.New()
	var starts, stops int
	var mu sync.Mutex
	started := make(chan string, 4)
	var s *Service
	run := func(ctx context.Context, sync subsystems.SyncFunc) {
		mu.Lock()
		starts++
		mu.Unlock()
		reconciled := !s.Health().GetLastReconcileAt().AsTime().IsZero()
		err := sync(ctx)
		started <- map[bool]string{true: "after resync", false: "before resync"}[reconciled] + " " + errString(err)
		<-ctx.Done()
		mu.Lock()
		stops++
		mu.Unlock()
	}
	s, md, src := newSrcSvc(t, v, run)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	src.set("loop702")
	fc := &fakeConn{VPP: v, states: make(chan vpp.ConnState, 4)}
	a := &Agent{log: s.log, conn: fc, svc: s, metrics: s.metrics}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.watchVPP(ctx); close(done) }()

	fc.states <- vpp.ConnState{Connected: true}
	select {
	case got := <-started:
		if got != "after resync ok" {
			t.Fatalf("run: %s", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the source loop did not start")
	}
	if md.list() != "loop702" {
		t.Fatalf("dynamic objects: %q", md.list())
	}
	fc.states <- vpp.ConnState{Connected: false}
	fc.states <- vpp.ConnState{Connected: true}
	cancel()
	<-done
	a.wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if starts != 1 || stops != 1 {
		t.Fatalf("starts %d stops %d (want one loop for the agent's lifetime)", starts, stops)
	}
}

func errString(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}

// ---- S1 failure semantics (TD-8 fix round 1: the review's probes A–E) --------------------------

const errLabelInUse = "VPP: label already in use (-1)"

// withLoop703 is sampleDoc plus the loopback loop703.
func withLoop703(t *testing.T) *vrxv1.DesiredState {
	t.Helper()
	ds := doc(t, sampleDoc)
	ds.Interfaces["loop703"] = &vrxv1.Interface{}
	return ds
}

// syncedSrcSvc is newSrcSvc after the config apply of sampleDoc and one successful sync of names.
func syncedSrcSvc(t *testing.T, v *coretest.VPP, names ...string) (*Service, *memDesc, *learnedSource) {
	t.Helper()
	s, md, src := newSrcSvc(t, v, nil)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	src.set(names...)
	if err := s.sourceSync("test-sync")(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := strings.Join(names, ","); md.list() != want {
		t.Fatalf("dynamic objects after the first sync: %q, want %q", md.list(), want)
	}
	return s, md, src
}

// Probe A (R2): a dynamic object that VPP rejects never fails the user's valid commit. The transaction
// runs once more without the dynamic sources, and the response, an ERROR event and the log name the
// source and the key.
func TestDynamicSourceFailureDoesNotFailTheCommit(t *testing.T) {
	v := coretest.New()
	s, md, src := syncedSrcSvc(t, v, "loop701")
	md.failOn("loop703", errors.New(errLabelInUse))
	src.set("loop701", "loop703")
	sub := s.events().subscribe(&vrxv1.StreamEventsRequest{Kinds: []vrxv1.EventKind{vrxv1.EventKind_EVENT_KIND_ERROR}})

	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: withLoop703(t)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, ok := v.InterfaceByName("loop703"); !ok || md.list() != "loop701" {
		t.Fatalf("after the commit: loop703 %v, dynamic objects %q (want the commit applied, the source left alone)", ok, md.list())
	}
	var skipped *vrxv1.ObjectResult
	for _, r := range resp.GetResults() {
		if r.GetKey() == dynDesc+"/loop703" {
			skipped = r
		}
	}
	if skipped == nil || skipped.GetCode() != vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_SKIPPED || !strings.Contains(skipped.GetMessage(), "test-sync") ||
		!strings.Contains(skipped.GetMessage(), errLabelInUse) || skipped.GetPointer() != "" || skipped.GetSubsystem() != "" {
		t.Fatalf("the skipped dynamic object is not reported: %v", resp.GetResults())
	}
	evs := collect(t, sub, 1)
	if a := evs[0].GetAttributes(); a["source"] != "test-sync" || a["key"] != dynDesc+"/loop703" || evs[0].GetTxnId() != "t2" {
		t.Fatalf("ERROR event %v", evs[0])
	}
	if s.Health().GetLastTxnId() != "t2" || s.Health().GetDegraded() {
		t.Fatalf("health after the fallback: %v", s.Health())
	}

	// TD-8b (V1): only the key is quarantined; the source stays in sync. Its sync applies the rest
	// and says which object is held back; the agent's key retry (backoff) brings it in.
	if !s.source("test-sync").inSync.Load() || strings.Join(quarantinedKeys(s, "test-sync"), ",") != dynDesc+"/loop703" {
		t.Fatalf("after the commit: in sync %v, quarantined %v (want the source in sync, loop703 quarantined)", s.source("test-sync").inSync.Load(), quarantinedKeys(s, "test-sync"))
	}
	sync := s.sourceSync("test-sync")
	if err := sync(context.Background()); !isQuarantinedErr(err) || !strings.Contains(err.Error(), errLabelInUse) {
		t.Fatalf("sync while loop703 is quarantined: %v", err)
	}
	if _, ok := v.InterfaceByName("loop703"); !ok || md.list() != "loop701" {
		t.Fatalf("the sync touched more than its own objects: loop703 %v, dynamic %q", ok, md.list())
	}
	md.failOn("loop703", nil)
	retryQuarantinedNow(t, s, "test-sync")
	if md.list() != "loop701,loop703" || len(quarantinedKeys(s, "test-sync")) != 0 {
		t.Fatalf("key retry once VPP accepts loop703: dynamic %q, quarantined %v", md.list(), quarantinedKeys(s, "test-sync"))
	}
	if err := sync(context.Background()); err != nil {
		t.Fatalf("sync with nothing quarantined: %v", err)
	}
	// In sync again, the source rides in config transactions: removing loop703 deletes both at once.
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t3", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if md.list() != "loop701" {
		t.Fatalf("after removing loop703: %q", md.list())
	}
}

// Probe B (R2): the resync after a VPP restart rebuilds the configuration even when VPP rejects a
// dynamic object; the data plane is never left empty and DEGRADED because of a source. Since TD-8b
// (V1) the rejected object alone is quarantined: the rest of the source is rebuilt too (probe G).
func TestDynamicSourceFailureDoesNotRollBackTheResync(t *testing.T) {
	v := coretest.New()
	s, md, _ := syncedSrcSvc(t, v, "loop701", "loop702")
	v.DeleteInterface("loop701") // VPP restarted: everything is gone ...
	v.DeleteInterface("loop702")
	v.DeleteTable(7001, false)
	v.DeleteTable(7001, true)
	md.drop("loop701")
	md.drop("loop702")
	md.failOn("loop702", errors.New(errLabelInUse)) // ... and VPP now rejects one dynamic object

	resp := s.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if s.Health().GetDegraded() {
		t.Fatal("degraded after a resync whose only failure was a dynamic object")
	}
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{})
	if err != nil || !proto.Equal(got.GetDesiredState(), doc(t, canonicalDoc)) {
		t.Fatalf("the configuration was not rebuilt: %v", err)
	}
	if md.list() != "loop701" {
		t.Fatalf("dynamic objects %q (want loop701 rebuilt, only loop702 quarantined)", md.list())
	}
	md.failOn("loop702", nil)
	retryQuarantinedNow(t, s, "test-sync")
	if err := s.sourceSync("test-sync")(context.Background()); err != nil || md.list() != "loop701,loop702" {
		t.Fatalf("sync after the key retry: %v, dynamic %q", err, md.list())
	}
}

// noPanic runs f and fails the test instead of crashing when f panics.
func noPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s panicked: %v", what, r)
		}
	}()
	f()
}

// Probe C (R3): a panic in a source's Desired is a source error, never an agent crash (and so never a
// crash loop through the first resync).
func TestDynamicSourcePanicIsContained(t *testing.T) {
	v := coretest.New()
	s, md, src := syncedSrcSvc(t, v, "loop701")
	src.mu.Lock()
	src.panicNow = true
	src.mu.Unlock()

	var resp *vrxv1.ApplyResponse
	noPanic(t, "Apply", func() { resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: withLoop703(t)}) })
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, ok := v.InterfaceByName("loop703"); !ok || md.list() != "loop701" {
		t.Fatalf("after the commit: loop703 %v, dynamic %q", ok, md.list())
	}
	var rep *vrxv1.ValidationReport
	var err error
	noPanic(t, "DryRun", func() {
		rep, err = s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: withLoop703(t)})
	})
	if err != nil || !rep.GetOk() {
		t.Fatalf("dry run: %v %v", err, rep)
	}
	noPanic(t, "sync", func() { err = s.sourceSync("test-sync")(context.Background()) })
	if err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("sync of a panicking source: %v", err)
	}
	noPanic(t, "Resync", func() { resp = s.Resync(context.Background()) })
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	// The transaction lock is free again.
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t3", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
}

// Probe D (R6): sync called from inside Desired, or from a descriptor call, would wait for the
// transaction lock its own goroutine holds; it is refused at once instead.
func TestDynamicSourceSyncInsideATransactionRefused(t *testing.T) {
	v := coretest.New()
	s, md, src := newSrcSvc(t, v, nil)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	sync := s.sourceSync("test-sync")
	var inner error
	var took time.Duration
	try := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		start := time.Now()
		inner = sync(ctx)
		took = time.Since(start)
	}
	armed := true
	src.mu.Lock()
	src.onDesired = func() {
		if armed {
			armed = false
			try()
		}
	}
	src.mu.Unlock()
	src.set("loop701")
	if err := sync(context.Background()); err != nil || md.list() != "loop701" {
		t.Fatalf("outer sync: %v, dynamic %q", err, md.list())
	}
	if inner == nil || took > time.Second || !strings.Contains(inner.Error(), "inside") {
		t.Fatalf("sync from inside Desired: %v after %v (want an immediate refusal)", inner, took)
	}

	src.mu.Lock()
	src.onDesired = nil
	src.mu.Unlock()
	inner, armed = nil, true
	md.mu.Lock()
	md.onCreate = func() {
		if armed {
			armed = false
			try()
		}
	}
	md.mu.Unlock()
	src.set("loop701", "loop702")
	if err := sync(context.Background()); err != nil || md.list() != "loop701,loop702" {
		t.Fatalf("outer sync: %v, dynamic %q", err, md.list())
	}
	if inner == nil || took > time.Second || !strings.Contains(inner.Error(), "inside") {
		t.Fatalf("sync from inside a descriptor call: %v after %v (want an immediate refusal)", inner, took)
	}
}

// Probe E (R1): an agent restart with VPP intact deletes no dynamic object. Until the source's first
// sync has filled its cache, it takes part in no transaction (resync, Apply, DryRun).
func TestDynamicSourceAgentRestartKeepsDynamicObjects(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	md := &memDesc{objs: map[string]bool{}}
	s1, src1 := newSrcSvcIn(t, v, dir, md, nil)
	mustStatus(t, apply(t, s1, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	src1.set("loop701", "loop702")
	if err := s1.sourceSync("test-sync")(context.Background()); err != nil || md.list() != "loop701,loop702" {
		t.Fatalf("first process: %v %q", err, md.list())
	}
	s1.Close()

	s2, src2 := newSrcSvcIn(t, v, dir, md, nil) // the new process: its source cache is empty
	resp := s2.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if resp.GetSummary().GetDeleted() != 0 || md.list() != "loop701,loop702" {
		t.Fatalf("first resync after an agent restart: summary %v, dynamic %q (want nothing deleted)", resp.GetSummary(), md.list())
	}
	rep, err := s2.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: withLoop703(t)})
	if err != nil || !rep.GetOk() {
		t.Fatalf("dry run: %v %v", err, rep)
	}
	for _, op := range rep.GetPlan() {
		if strings.HasPrefix(op.GetKey(), dynDesc+"/") {
			t.Fatalf("dry run before the first sync plans a dynamic object: %v", op)
		}
	}
	mustStatus(t, apply(t, s2, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: withLoop703(t)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if md.list() != "loop701,loop702" {
		t.Fatalf("a commit before the first sync touched dynamic objects: %q", md.list())
	}
	// The first sync after Run filled the cache reconciles them; from then on the source rides along.
	src2.set("loop701", "loop703")
	if err := s2.sourceSync("test-sync")(context.Background()); err != nil || md.list() != "loop701,loop703" {
		t.Fatalf("first sync: %v, dynamic %q", err, md.list())
	}
	mustStatus(t, apply(t, s2, &vrxv1.ApplyRequest{TxnId: "t3", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if md.list() != "loop701" {
		t.Fatalf("removing loop703 after the first sync: %q", md.list())
	}
}

// A key or a source left out of a transaction rejoins on its own: the agent retries it with backoff
// (TD-8b: a rejected object is quarantined alone, its source stays in sync; a source whose output is
// invalid is left out as a whole).
func TestDynamicSourceLeftOutRejoinsThroughTheRetry(t *testing.T) {
	v := coretest.New()
	s, md, src := syncedSrcSvc(t, v, "loop701")
	s.retryMin, s.retryMax = 20*time.Millisecond, 40*time.Millisecond
	md.failOn("loop703", errors.New(errLabelInUse))
	src.set("loop701", "loop703")
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: withLoop703(t)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !s.source("test-sync").inSync.Load() || len(quarantinedKeys(s, "test-sync")) != 1 {
		t.Fatalf("after the commit: in sync %v, quarantined %v (want in sync, loop703 quarantined)", s.source("test-sync").inSync.Load(), quarantinedKeys(s, "test-sync"))
	}
	time.Sleep(150 * time.Millisecond) // a few key retries that VPP still rejects
	if md.list() != "loop701" {
		t.Fatalf("dynamic objects while VPP rejects loop703: %q", md.list())
	}
	md.failOn("loop703", nil)
	eventually(t, func() bool { return md.list() == "loop701,loop703" && len(quarantinedKeys(s, "test-sync")) == 0 }, func() string {
		return fmt.Sprintf("the key did not rejoin: dynamic %q, quarantined %v", md.list(), quarantinedKeys(s, "test-sync"))
	})

	// A source whose output is invalid is left out as a whole, and its own retry rejoins it.
	bad := scheduler.Join(core.LoopbackName, "loop777")
	src.mu.Lock()
	src.extra = []scheduler.KV{{Key: bad, Value: wrapperspb.String("x")}}
	src.mu.Unlock()
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t3", DesiredState: withLoop703(t)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if s.source("test-sync").inSync.Load() || md.list() != "loop701,loop703" {
		t.Fatalf("an invalid source: in sync %v, dynamic %q (want out of sync, objects left as they are)", s.source("test-sync").inSync.Load(), md.list())
	}
	src.mu.Lock()
	src.extra = nil
	src.mu.Unlock()
	eventually(t, func() bool { return md.list() == "loop701,loop703" && s.source("test-sync").inSync.Load() }, func() string {
		return fmt.Sprintf("the source did not rejoin: dynamic %q", md.list())
	})
}

// eventually polls ok for up to 5 s (a loaded host) and fails with why.
func eventually(t *testing.T, ok func() bool, why func() string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal(why())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The agent syncs a source without Run once after the first resync; a Run that panics or returns
// early stops its source until the agent restarts (R3): out of sync, its objects left as they are.
func TestDynamicSourceStartAndRunFailure(t *testing.T) {
	start := func(t *testing.T, run func(context.Context, subsystems.SyncFunc)) (*Service, *memDesc, *learnedSource) {
		t.Helper()
		v := coretest.New()
		s, md, src := newSrcSvc(t, v, run)
		mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
		src.set("loop701")
		a := &Agent{log: s.log, svc: s, metrics: s.metrics}
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(func() { cancel(); a.wg.Wait() })
		a.startSources(ctx)
		return s, md, src
	}
	t.Run("no Run", func(t *testing.T) {
		s, md, _ := start(t, nil)
		if md.list() != "loop701" || !s.source("test-sync").inSync.Load() {
			t.Fatalf("after start: dynamic %q, in sync %v", md.list(), s.source("test-sync").inSync.Load())
		}
	})
	for name, tc := range map[string]struct {
		run    func(context.Context, subsystems.SyncFunc)
		reason string
	}{
		"Run panics": {func(ctx context.Context, sync subsystems.SyncFunc) {
			_ = sync(ctx)
			panic("run bug")
		}, "panic"},
		"Run returns early": {func(ctx context.Context, sync subsystems.SyncFunc) { _ = sync(ctx) }, "stopped"},
	} {
		t.Run(name, func(t *testing.T) {
			var s *Service
			var md *memDesc
			var src *learnedSource
			noPanic(t, "startSources", func() { s, md, src = start(t, tc.run) })
			deadline := time.Now().Add(5 * time.Second)
			for !s.source("test-sync").stoppedNow(s) {
				if time.Now().After(deadline) {
					t.Fatal("the source was not stopped")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if md.list() != "loop701" || s.source("test-sync").inSync.Load() {
				t.Fatalf("after the stop: dynamic %q, in sync %v", md.list(), s.source("test-sync").inSync.Load())
			}
			// Out of sync: a commit leaves its objects alone, and a stray sync is refused.
			src.set("loop702")
			mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: withLoop703(t)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
			if err := s.sourceSync("test-sync")(context.Background()); grpcCode(err) != codes.FailedPrecondition || md.list() != "loop701" || s.source("test-sync").inSync.Load() {
				t.Fatalf("a stopped source: sync %v, dynamic %q", err, md.list())
			}
			var b strings.Builder
			s.metrics.write(&b)
			if want := `vrx_agent_dynamic_source_errors_total{source="test-sync",reason="` + tc.reason + `"} 1`; !strings.Contains(b.String(), want) {
				t.Fatalf("missing %q in\n%s", want, b.String())
			}
		})
	}
}

// stoppedNow reads ds.stopped under the transaction lock.
func (ds *dynSource) stoppedNow(s *Service) bool {
	_ = s.lock(context.Background())
	defer s.unlock()
	return ds.stopped
}

// Env.Resync storm guard (R8/R9): a request right after a requested resync is deferred to the end of
// the interval, and the requests made meanwhile coalesce into one resync.
func TestRequestedResyncsAreRateLimited(t *testing.T) {
	old := resyncMinInterval
	resyncMinInterval = 500 * time.Millisecond
	t.Cleanup(func() { resyncMinInterval = old })
	a, fc := startFake(t, testConfig(t))
	sub := a.svc.events().subscribe(&vrxv1.StreamEventsRequest{Kinds: []vrxv1.EventKind{vrxv1.EventKind_EVENT_KIND_RECONCILE_START}})
	fc.states <- vpp.ConnState{Connected: true}
	collect(t, sub, 1) // the connect resync
	a.wiring.RequestResync()
	collect(t, sub, 1) // the first requested resync runs at once
	start := time.Now()
	for range 5 {
		a.wiring.RequestResync()
		time.Sleep(20 * time.Millisecond)
	}
	collect(t, sub, 1)
	if d := time.Since(start); d < 300*time.Millisecond {
		t.Fatalf("a requested resync right after another ran after %v (want it deferred)", d)
	}
	noEvent(t, sub, 700*time.Millisecond) // the five requests coalesced into one
}
