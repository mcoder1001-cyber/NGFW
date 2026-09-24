package agent

// TD-8 (D-119 M2): the agent seams — feature events reach StreamEvents, Env.Resync reaches
// Service.Resync, the id range goes through Env and fails closed, dynamic desired sources (S1) are
// merged into every transaction under the txn lock, and feature metrics collectors reach /metrics.
// Default inert (no hook, no source, no collector: nothing changes) is covered here and by every
// other test of this package, which runs without them.

import (
	"context"
	"errors"
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
	if r, err := a.wiring.IDRange(); r != nil || !errors.Is(err, subsystems.ErrNoIDRange) {
		t.Fatalf("zero IDs: %v %v (want ErrNoIDRange)", r, err)
	}
}

func TestConfigFromEnvIDRange(t *testing.T) {
	t.Setenv(subsystems.EnvTableBase, "")
	t.Setenv(subsystems.EnvIDRange, "")
	cfg := ConfigFromEnv()
	if cfg.IDs != (subsystems.IDScope{}) || cfg.Validate() != nil {
		t.Fatalf("unset: %+v %v (want no id, start allowed: families fail closed)", cfg.IDs, cfg.Validate())
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
	mu   sync.Mutex
	objs map[string]bool
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
	defer d.mu.Unlock()
	d.objs[o.(*wrapperspb.StringValue).GetValue()] = true
	return nil, nil
}
func (d *memDesc) Update(_ context.Context, _, _ proto.Message, meta any) (any, error) {
	return meta, nil
}
func (d *memDesc) Delete(_ context.Context, o proto.Message, _ any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.objs, o.(*wrapperspb.StringValue).GetValue())
	return nil
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
	mu      sync.Mutex
	learned map[string]bool
	extra   []scheduler.KV // a buggy source: keys it must not produce
	views   []*vrxv1.DesiredState
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
	defer s.mu.Unlock()
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
	dir := t.TempDir()
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
	md := &memDesc{objs: map[string]bool{}}
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
	return svc, md, src
}

func TestDynamicSourceMergedIntoEveryTransaction(t *testing.T) {
	v := coretest.New()
	s, md, src := newSrcSvc(t, v, nil)
	src.set("loop701", "loop702", "loop799") // loop799 is not in the document: left out by Desired

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

func TestDynamicSourceKeyOutsideItsDescriptorsFails(t *testing.T) {
	v := coretest.New()
	s, md, src := newSrcSvc(t, v, nil)
	src.set("loop701")
	src.extra = []scheduler.KV{{Key: scheduler.Join(core.LoopbackName, "loop777"), Value: wrapperspb.String("x")}}
	err := s.sourceSync("test-sync")(context.Background())
	if err == nil || !strings.Contains(err.Error(), "outside its descriptors") {
		t.Fatalf("sync: %v", err)
	}
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_FAILED)
	if errs := resp.GetValidation().GetErrors(); len(errs) == 0 || errs[0].GetRule() != "agent.dynamic-source" || errs[0].GetPointer() != "" || resp.GetValidation().GetOk() {
		t.Fatalf("validation: %v", resp.GetValidation())
	}
	if md.list() != "" || len(v.Ifaces) != 1 {
		t.Fatalf("something was applied: %q, %d interfaces", md.list(), len(v.Ifaces))
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
