package ravpn

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/strongswan/govici/vici"
	"google.golang.org/protobuf/proto"
	"iter"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/strongswan"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type lifecycleVerifier struct {
	plan    *NetworkPlan
	handoff Handoff
	fail    bool
	calls   int
	change  bool
}

func (v *lifecycleVerifier) Verify(context.Context, EngineSpec) (*NetworkPlan, Handoff, error) {
	v.calls++
	if v.fail {
		return nil, Handoff{}, ErrEngine
	}
	h := v.handoff
	if v.change && v.calls > 1 {
		h.VPPBoot.StartTime++
	}
	return v.plan, h, nil
}

type lifecyclePreparation struct {
	events    *[]string
	fail      bool
	loadFail  bool
	readyFail bool
}

func (p *lifecyclePreparation) Prepare(context.Context, EngineSpec) (*PreparedEngine, error) {
	*p.events = append(*p.events, "prepare")
	if p.fail {
		return nil, ErrEngine
	}
	return p.Recover(context.Background(), EngineSpec{})
}
func (p *lifecyclePreparation) Recover(context.Context, EngineSpec) (*PreparedEngine, error) {
	return &PreparedEngine{ConnectionName: "ra-road", Load: func(context.Context, strongswan.ViciConn) error {
		*p.events = append(*p.events, "load")
		if p.loadFail {
			return ErrEngine
		}
		return nil
	}, Cleanup: func(context.Context) error { *p.events = append(*p.events, "cleanup"); return nil }}, nil
}
func (p *lifecyclePreparation) Preflight(context.Context) error {
	if p.readyFail {
		return ErrEngine
	}
	return nil
}

type lifecycleUnits struct {
	events                           *[]string
	id                               UnitIdentity
	started                          bool
	startFail, observeFail, stopFail bool
}

func (u *lifecycleUnits) Start(context.Context, *NetworkPlan) (UnitIdentity, error) {
	*u.events = append(*u.events, "start")
	u.started = true
	if u.startFail {
		return u.id, ErrEngine
	}
	return u.id, nil
}
func (u *lifecycleUnits) Observe(context.Context, *NetworkPlan) (UnitIdentity, error) {
	if u.observeFail {
		return UnitIdentity{}, ErrEngine
	}
	return u.id, nil
}
func (u *lifecycleUnits) Stop(context.Context, *NetworkPlan, UnitIdentity) error {
	*u.events = append(*u.events, "stop")
	if u.stopFail {
		return ErrEngine
	}
	u.started = false
	return nil
}

func (u *lifecycleUnits) Inactive(context.Context, *NetworkPlan) error {
	if u.started {
		return ErrEngine
	}
	return nil
}

type lifecycleStore struct{ records map[string]EngineRecord }

func (s *lifecycleStore) List() ([]EngineRecord, error) {
	out := []EngineRecord{}
	for _, r := range s.records {
		out = append(out, r)
	}
	return out, nil
}
func (s *lifecycleStore) Save(r EngineRecord) error { s.records[r.Spec.Instance] = r; return nil }
func (s *lifecycleStore) Remove(id string) error    { delete(s.records, id); return nil }

type lifecycleVICI struct{}

func (lifecycleVICI) Call(_ context.Context, cmd string, _ *vici.Message) (*vici.Message, error) {
	m := vici.NewMessage()
	switch cmd {
	case "get-conns":
		_ = m.Set("conns", []string{"ra-road"})
	case "stats":
		child := vici.NewMessage()
		_ = child.Set("total", "0")
		_ = m.Set("ikesas", child)
	case "list-sas":
	default:
		return nil, errors.New("unexpected call")
	}
	return m, nil
}
func (lifecycleVICI) CallStreaming(context.Context, string, string, *vici.Message) iter.Seq2[*vici.Message, error] {
	return func(func(*vici.Message, error) bool) {}
}
func (lifecycleVICI) Subscribe(...string) error      { return nil }
func (lifecycleVICI) Unsubscribe(...string) error    { return nil }
func (lifecycleVICI) NotifyEvents(chan<- vici.Event) {}
func (lifecycleVICI) Close() error                   { return nil }
func lifecycleFixture(t *testing.T) (*Runtime, *lifecycleVerifier, *lifecyclePreparation, *lifecycleUnits, *lifecycleStore, *[]string, EngineSpec) {
	s := controllerSpec(t)
	p, e := BuildNetworkPlan(s.Owner, s.Profile, s.Configuration)
	if e != nil {
		t.Fatal(e)
	}
	p.NamespaceInode = 101
	p.HostNamespaceInode = 102
	v := &lifecycleVerifier{plan: p, handoff: Handoff{Format: 1, Instance: s.Instance, NamespaceInode: 101, HostNamespaceInode: 102, VPPBoot: bootid.Identity{BootID: "test", PID: 4, StartTime: 5}, OuterIndex: 6, InnerIndex: 7}}
	events := []string{}
	prep := &lifecyclePreparation{events: &events}
	units := &lifecycleUnits{events: &events, id: UnitIdentity{BootID: "test", PID: 20, StartTicks: 30, NamespaceInode: 101}}
	store := &lifecycleStore{records: map[string]EngineRecord{}}
	rt := NewRuntime(s.Owner, v, prep, units, store)
	// This logical lifecycle fixture has no kernel namespace bindings or host inventory.
	rt.inventory = func(context.Context, string) ([]*NetworkPlan, error) { return nil, nil }
	rt.readPlan = func(string) (*NetworkPlan, error) { return p, nil }
	rt.dial = func(context.Context, *NetworkPlan, UnitIdentity) (strongswan.ViciConn, error) {
		return lifecycleVICI{}, nil
	}
	rt.SetReadiness(func(context.Context) error { return nil })
	return rt, v, prep, units, store, &events, s
}
func TestLifecycleFailureStopPrecedesCleanupAndRetainsFailedStop(t *testing.T) {
	for _, failure := range []string{"transport", "prepare", "start", "observe", "load", "changed-boot"} {
		t.Run(failure, func(t *testing.T) {
			r, v, p, u, store, events, s := lifecycleFixture(t)
			switch failure {
			case "transport":
				v.fail = true
			case "prepare":
				p.fail = true
			case "start":
				u.startFail = true
			case "observe":
				u.observeFail = true
			case "load":
				p.loadFail = true
			case "changed-boot":
				v.change = true
			}
			record, e := r.Create(context.Background(), s)
			if e == nil {
				t.Fatal("failed activation succeeded")
			}
			if failure == "transport" || failure == "prepare" {
				if len(store.records) != 0 || strings.Contains(strings.Join(*events, ","), "start") {
					t.Fatal("daemon mutated before prerequisites")
				}
				return
			}
			if record.Unit != u.id || len(store.records) != 1 {
				t.Fatal("partial owned unit identity lost")
			}
			u.stopFail = true
			if r.Delete(context.Background(), s) == nil || strings.Contains(strings.Join(*events, ","), "cleanup") {
				t.Fatal("cleanup despite failed stop")
			}
			if r.Ready(context.Background()) == nil {
				t.Fatal("failed generation advertised ready")
			}
			u.stopFail = false
			if r.Delete(context.Background(), s) != nil {
				t.Fatal("owned rollback failed")
			}
			joined := strings.Join(*events, ",")
			if !strings.HasSuffix(joined, "stop,cleanup") || len(store.records) != 0 {
				t.Fatal(joined)
			}
		})
	}
}
func TestLifecycleRestartRecoversAndVPPRepairStopsOldGenerationFirst(t *testing.T) {
	r, v, p, u, store, events, s := lifecycleFixture(t)
	if _, e := r.Create(context.Background(), s); e != nil {
		t.Fatal(e)
	}
	if r.Ready(context.Background()) != nil {
		t.Fatal("verified readiness refused")
	}
	next := NewRuntime(s.Owner, v, p, u, store)
	next.readPlan = r.readPlan
	next.inventory = r.inventory
	next.dial = r.dial
	next.SetReadiness(func(context.Context) error { return nil })
	if next.Recover(context.Background()) != nil {
		t.Fatal("restart recover")
	}
	records, e := next.Records(context.Background())
	if e != nil || len(records) != 1 || records[0].Unit != u.id {
		t.Fatal("restart invented generation")
	}
	v.fail = true
	if _, e = next.Records(context.Background()); e == nil {
		t.Fatal("stale VPP handoff accepted")
	}
	if next.StopAll(context.Background()) != nil {
		t.Fatal("stop depended on live VPP")
	}
	if !strings.HasSuffix(strings.Join(*events, ","), "stop,cleanup") || len(store.records) != 0 {
		t.Fatal("old generation retained")
	}
}
func TestReadinessFirstActivationAndUnavailableComponents(t *testing.T) {
	r, _, p, _, _, _, _ := lifecycleFixture(t)
	if r.Ready(context.Background()) != nil {
		t.Fatal("first enable circular dependency")
	}
	p.readyFail = true
	if r.Ready(context.Background()) == nil {
		t.Fatal("unverified install accepted")
	}
}

type sessionVICI struct {
	lifecycleVICI
	events     chan<- vici.Event
	active     bool
	terminates int
}

func sessionMessage(values ...any) *vici.Message {
	m := vici.NewMessage()
	for len(values) >= 2 {
		_ = m.Set(values[0].(string), values[1])
		values = values[2:]
	}
	return m
}
func (f *sessionVICI) NotifyEvents(c chan<- vici.Event) { f.events = c }
func (f *sessionVICI) Call(ctx context.Context, cmd string, in *vici.Message) (*vici.Message, error) {
	switch cmd {
	case "stats":
		count := "0"
		if f.active {
			count = "1"
		}
		return sessionMessage("ikesas", sessionMessage("total", count)), nil
	case "list-sas":
		if f.active {
			child := sessionMessage("state", "INSTALLED", "if-id-in", "00000001", "if-id-out", "00000001", "bytes-in", "18446744073709551615", "bytes-out", "9007199254740993")
			sa := sessionMessage("uniqueid", "1", "state", "ESTABLISHED", "remote-eap-id", "client", "remote-vips", []string{"10.10.0.5"}, "established", "42", "child-sas", sessionMessage("protected-1", child))
			f.events <- vici.Event{Name: "list-sa", Message: sessionMessage("ra-road", sa)}
		}
		return sessionMessage("success", "yes"), nil
	case "terminate":
		f.terminates++
		f.active = false
		return sessionMessage("success", "yes"), nil
	}
	return f.lifecycleVICI.Call(ctx, cmd, in)
}
func TestLifecycleSessionIDRejectsReusedSAAfterRestart(t *testing.T) {
	rt, _, _, units, _, _, spec := lifecycleFixture(t)
	fake := &sessionVICI{}
	rt.dial = func(context.Context, *NetworkPlan, UnitIdentity) (strongswan.ViciConn, error) { return fake, nil }
	ctx := context.Background()
	if _, err := rt.Create(ctx, spec); err != nil {
		t.Fatal(err)
	}
	fake.active = true
	before, err := rt.Sessions(ctx, spec.Profile)
	if err != nil || len(before) != 1 || before[0].BytesIn != ^uint64(0) || before[0].BytesOut != 9007199254740993 {
		t.Fatal("observed exact counters lost", err, before)
	}
	oldID := before[0].ID
	if err := rt.Delete(ctx, spec); err != nil {
		t.Fatal(err)
	}
	fake.active = false
	units.id.StartTicks++
	if _, err := rt.Create(ctx, spec); err != nil {
		t.Fatal(err)
	}
	fake.active = true
	after, err := rt.Sessions(ctx, spec.Profile)
	if err != nil || len(after) != 1 || after[0].ID == oldID {
		t.Fatal("reused SA retained stale opaque ID", err)
	}
	if rt.Disconnect(ctx, spec.Profile, oldID) == nil || fake.terminates != 0 {
		t.Fatal("stale ID terminated replacement SA")
	}
	if err := rt.Disconnect(ctx, spec.Profile, after[0].ID); err != nil || fake.terminates != 1 {
		t.Fatal("verified replacement disconnect failed", err)
	}
}

func TestLifecycleUnloadFailureStillStopsAndCleansOwnedDaemon(t *testing.T) {
	rt, _, _, _, store, events, spec := lifecycleFixture(t)
	ctx := context.Background()
	if _, err := rt.Create(ctx, spec); err != nil {
		t.Fatal(err)
	}
	rt.active[spec.Instance].prepared.Unload = func(context.Context, strongswan.ViciConn) error {
		*events = append(*events, "unload-refused")
		return ErrEngine
	}
	if err := rt.Delete(ctx, spec); err == nil {
		t.Fatal("failed unload readback claimed success")
	}
	if len(rt.active) != 0 || len(store.records) != 0 {
		t.Fatal("stopped generation was retained")
	}
	joined := strings.Join(*events, ",")
	if !strings.HasSuffix(joined, "unload-refused,stop,cleanup") {
		t.Fatal("unload failure prevented verified stop/cleanup", joined)
	}
}

func TestReadinessRejectsReplacedActiveUnitAndChangedHandoff(t *testing.T) {
	rt, verifier, _, units, _, _, spec := lifecycleFixture(t)
	ctx := context.Background()
	if _, err := rt.Create(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if rt.Ready(ctx) != nil {
		t.Fatal("verified activation unavailable")
	}
	units.id.StartTicks++
	if rt.Ready(ctx) == nil {
		t.Fatal("replacement process advertised readiness")
	}
	units.id.StartTicks--
	verifier.fail = true
	if rt.Ready(ctx) == nil {
		t.Fatal("changed handoff advertised readiness")
	}
}

func TestLifecycleRestartStopsRecordedDaemonEvenWhenSealedCacheUnavailable(t *testing.T) {
	old, verifier, _, units, store, events, spec := lifecycleFixture(t)
	ctx := context.Background()
	if _, err := old.Create(ctx, spec); err != nil {
		t.Fatal(err)
	}
	next := NewRuntime(spec.Owner, verifier, nil, units, store)
	next.readPlan = old.readPlan
	next.inventory = old.inventory
	if next.StopAll(ctx) == nil {
		t.Fatal("missing credential recovery claimed completed cleanup")
	}
	if !strings.HasSuffix(strings.Join(*events, ","), "stop") || len(store.records) != 1 {
		t.Fatal("cache failure prevented owned stop or discarded recovery record")
	}
}
func TestReadinessRefusesUnrecoveredPersistentGeneration(t *testing.T) {
	old, verifier, prep, units, store, _, spec := lifecycleFixture(t)
	ctx := context.Background()
	if _, err := old.Create(ctx, spec); err != nil {
		t.Fatal(err)
	}
	next := NewRuntime(spec.Owner, verifier, prep, units, store)
	next.readPlan = old.readPlan
	next.inventory = old.inventory
	next.dial = old.dial
	next.SetReadiness(func(context.Context) error { return nil })
	if next.Ready(ctx) == nil {
		t.Fatal("unrecovered persisted generation advertised activation")
	}
	if next.Recover(ctx) != nil || next.Ready(ctx) != nil {
		t.Fatal("verified recovered generation unavailable")
	}
}

func TestTransportGuardRequiresPositiveInactiveAndNoPendingReceipt(t *testing.T) {
	r, v, _, u, store, _, s := lifecycleFixture(t)
	ctx := context.Background()
	if err := r.TransportGuard(ctx, v.plan); err != nil {
		t.Fatal(err)
	}
	u.started = true
	if r.TransportGuard(ctx, v.plan) == nil {
		t.Fatal("untracked live daemon accepted")
	}
	u.started = false
	store.records[s.Instance] = EngineRecord{Spec: s, Ready: false}
	if r.TransportGuard(ctx, v.plan) == nil {
		t.Fatal("persistent pending generation accepted")
	}
	delete(store.records, s.Instance)
	if _, err := r.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	u.stopFail = true
	if r.Delete(ctx, s) == nil {
		t.Fatal("failed stop accepted")
	}
	if r.TransportGuard(ctx, v.plan) == nil {
		t.Fatal("failed stop released transport")
	}
	if len(store.records) != 1 || r.active[s.Instance].record.Ready {
		t.Fatal("failed generation forgotten")
	}
}

func TestQuiesceUnchangedDefaultsAndRestoreExistingGeneration(t *testing.T) {
	r, _, _, _, _, events, s := lifecycleFixture(t)
	ctx := context.Background()
	if _, err := r.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	enabled := true
	equivalent := s
	equivalent.Configuration = proto.Clone(s.Configuration).(*ngfwv1.RemoteAccessProfile)
	equivalent.Configuration.Enabled = &enabled
	before := len(*events)
	changed, err := r.ChangeRequired(ctx, []EngineSpec{equivalent})
	if err != nil || changed {
		t.Fatal("equivalent defaults require stop", err)
	}
	previous, err := r.QuiesceChanged(ctx, []EngineSpec{equivalent})
	if err != nil || len(previous) != 0 || len(*events) != before {
		t.Fatal("unchanged generation stopped", err)
	}
	if err := r.Restore(ctx, []EngineSpec{s}); err != nil || len(*events) != before {
		t.Fatal("existing generation recreated", err)
	}
}

func TestRecoverPendingGenerationDoesNotMutateDaemon(t *testing.T) {
	r, v, p, u, store, events, s := lifecycleFixture(t)
	ctx := context.Background()
	if _, err := r.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	record := store.records[s.Instance]
	record.Ready = false
	store.records[s.Instance] = record
	next := NewRuntime(s.Owner, v, p, u, store)
	next.readPlan = r.readPlan
	next.inventory = r.inventory
	next.dial = r.dial
	before := len(*events)
	if next.Recover(ctx) == nil {
		t.Fatal("pending generation reported ready")
	}
	if len(*events) != before || !u.started || len(store.records) != 1 {
		t.Fatal("readonly recovery mutated daemon or receipt")
	}
	if next.TransportGuard(ctx, v.plan) == nil {
		t.Fatal("pending recovered daemon released transport")
	}
}

type profileUnits struct {
	ids      map[string]UnitIdentity
	live     map[string]bool
	failStop string
	events   *[]string
}

func (u *profileUnits) Start(_ context.Context, p *NetworkPlan) (UnitIdentity, error) {
	*u.events = append(*u.events, "start:"+p.Instance)
	u.live[p.Instance] = true
	return u.ids[p.Instance], nil
}
func (u *profileUnits) Observe(_ context.Context, p *NetworkPlan) (UnitIdentity, error) {
	if !u.live[p.Instance] {
		return UnitIdentity{}, ErrEngine
	}
	return u.ids[p.Instance], nil
}
func (u *profileUnits) Stop(_ context.Context, p *NetworkPlan, id UnitIdentity) error {
	*u.events = append(*u.events, "stop:"+p.Instance)
	if p.Instance == u.failStop || id != u.ids[p.Instance] {
		return ErrEngine
	}
	u.live[p.Instance] = false
	return nil
}
func (u *profileUnits) Inactive(_ context.Context, p *NetworkPlan) error {
	if u.live[p.Instance] {
		return ErrEngine
	}
	return nil
}

type profileVICI struct {
	lifecycleVICI
	name string
}

func (v profileVICI) Call(ctx context.Context, cmd string, msg *vici.Message) (*vici.Message, error) {
	if cmd == "get-conns" {
		m := vici.NewMessage()
		if err := m.Set("conns", []string{"ra-" + v.name}); err != nil {
			return nil, err
		}
		return m, nil
	}
	return v.lifecycleVICI.Call(ctx, cmd, msg)
}

type profileVerifier map[string]*lifecycleVerifier

func (v profileVerifier) Verify(ctx context.Context, s EngineSpec) (*NetworkPlan, Handoff, error) {
	if item := v[s.Instance]; item != nil {
		return item.Verify(ctx, s)
	}
	return nil, Handoff{}, ErrEngine
}

func TestPartialQuiesceRestoresStoppedProfileAndRetainsFailedStop(t *testing.T) {
	r, v, _, _, store, events, first := lifecycleFixture(t)
	second := first
	second.Profile = "second"
	second.Instance = InstanceID(second.Owner, second.Profile)
	second.Configuration = proto.Clone(first.Configuration).(*ngfwv1.RemoteAccessProfile)
	second.OuterID = first.OuterID + 2
	second.InnerID = first.InnerID + 2
	plan, err := BuildNetworkPlan(second.Owner, second.Profile, second.Configuration)
	if err != nil {
		t.Fatal(err)
	}
	plan.NamespaceInode = 201
	plan.HostNamespaceInode = 202
	vh := v.handoff
	vh.Instance = second.Instance
	vh.NamespaceInode = 201
	vh.HostNamespaceInode = 202
	verifiers := profileVerifier{first.Instance: v, second.Instance: &lifecycleVerifier{plan: plan, handoff: vh}}
	units := &profileUnits{ids: map[string]UnitIdentity{first.Instance: {BootID: "test", PID: 20, StartTicks: 30, NamespaceInode: 101}, second.Instance: {BootID: "test", PID: 21, StartTicks: 31, NamespaceInode: 201}}, live: map[string]bool{}, events: events}
	r.units = units
	r.verifier = verifiers
	r.dial = func(_ context.Context, p *NetworkPlan, _ UnitIdentity) (strongswan.ViciConn, error) {
		name := first.Profile
		if p.Instance == second.Instance {
			name = second.Profile
		}
		return profileVICI{name: name}, nil
	}
	r.readPlan = func(instance string) (*NetworkPlan, error) {
		if value := verifiers[instance]; value != nil {
			return value.plan, nil
		}
		return nil, ErrEngine
	}
	ctx := context.Background()
	for _, s := range []EngineSpec{first, second} {
		if _, err := r.Create(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	ordered := []EngineSpec{first, second}
	if ordered[0].Instance > ordered[1].Instance {
		ordered[0], ordered[1] = ordered[1], ordered[0]
	}
	units.failStop = ordered[1].Instance
	previous, err := r.QuiesceChanged(ctx, nil)
	if err == nil || len(previous) != 2 {
		t.Fatal("partial stop outcome missing", err)
	}
	if units.live[ordered[0].Instance] || !units.live[ordered[1].Instance] {
		t.Fatal("partial stop did not retain failed daemon")
	}
	// Failed generation first in restoration must not prevent the other profile's recovery.
	if r.Restore(ctx, []EngineSpec{ordered[1], ordered[0]}) == nil {
		t.Fatal("failed stop reported restored")
	}
	if !units.live[ordered[0].Instance] || !r.active[ordered[0].Instance].record.Ready {
		t.Fatal("successfully stopped profile not restored")
	}
	if !units.live[ordered[1].Instance] || r.active[ordered[1].Instance].record.Ready || len(store.records) != 2 {
		t.Fatal("failed stop ownership forgotten")
	}
	if r.TransportGuard(ctx, verifiers[ordered[1].Instance].plan) == nil {
		t.Fatal("failed generation transport released")
	}
}

func TestStopAllRefusesUnobservedUnitWithoutAnyEngineRecord(t *testing.T) {
	rt, verifier, _, units, store, _, _ := lifecycleFixture(t)
	rt.inventory = func(context.Context, string) ([]*NetworkPlan, error) { return []*NetworkPlan{verifier.plan}, nil }
	units.started = true // Interrupted launch; process exists but no captured identity/record.
	if len(store.records) != 0 || len(rt.active) != 0 {
		t.Fatal("fixture adopted a generation")
	}
	if err := rt.StopAll(context.Background()); err == nil {
		t.Fatal("untracked live unit permitted global transport repair")
	}
	if !units.started || len(store.records) != 0 || len(rt.active) != 0 {
		t.Fatal("unknown live unit was adopted or stopped")
	}
	units.started = false
	if err := rt.StopAll(context.Background()); err != nil {
		t.Fatal("positive inactivity refused", err)
	}
	rt.inventory = func(context.Context, string) ([]*NetworkPlan, error) { return nil, ErrEngine }
	if err := rt.StopAll(context.Background()); err == nil {
		t.Fatal("unknown protected inventory treated as empty")
	}
}

func TestReadinessRefusesUnobservedProtectedUnitWithoutCircularActiveRequirement(t *testing.T) {
	rt, verifier, _, units, store, _, spec := lifecycleFixture(t)
	rt.SetReadiness(func(context.Context) error { return nil })
	rt.inventory = func(context.Context, string) ([]*NetworkPlan, error) { return []*NetworkPlan{verifier.plan}, nil }
	units.started = true
	if err := rt.Ready(context.Background()); err == nil {
		t.Fatal("unrecorded live unit advertised activation readiness")
	}
	if len(store.records) != 0 || len(rt.active) != 0 {
		t.Fatal("readiness invented a generation")
	}
	units.started = false
	if err := rt.Ready(context.Background()); err != nil {
		t.Fatal("verified installed fixture with no active profile refused first enable", err)
	}
	if _, err := rt.Create(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if err := rt.Ready(context.Background()); err != nil {
		t.Fatal("freshly verified ready active unit refused readiness", err)
	}
	verifier.plan.HostNamespaceInode++
	if err := rt.Ready(context.Background()); err == nil {
		t.Fatal("protected namespace epoch replacement accepted")
	}
}

func TestProtectedInventorySkipsValidForeignNamespaceBeforeBindingProbe(t *testing.T) {
	requireRootOwnedFixture(t)
	_, verifier, _, _, _, _, _ := lifecycleFixture(t)
	root := filepath.Join(t.TempDir(), "inventory")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	foreign := *verifier.plan
	foreign.Owner = "foreign-owner"
	foreign.Instance = InstanceID(foreign.Owner, foreign.Profile)
	if err := foreign.Validate(); err != nil {
		t.Fatal("foreign fixture plan", err)
	}
	folder := filepath.Join(root, foreign.Instance)
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(&foreign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodePrivatePlan(data, foreign.Instance); err != nil {
		t.Fatal("foreign fixture manifest", err)
	}
	manifest := filepath.Join(folder, "network.json")
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- root is this test's private TempDir; hold it to verify foreign-owner inventory isolation.
	file, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readInventoryManifest(int(file.Fd()), foreign.Instance); err != nil {
		t.Fatal("held public manifest stage", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	probes := 0
	read := func(string) (*NetworkPlan, error) { probes++; return nil, ErrEngine }
	plans, err := ownedNamespaceInventoryAt(context.Background(), root, verifier.plan.Owner, read)
	if err != nil || len(plans) != 0 || probes != 0 {
		t.Fatal("foreign namespace was probed/adopted or blocked another owner", err)
	}
	// #nosec G304 -- manifest is the fixed network.json beneath the private test root and validated fixture instance.
	if after, err := os.ReadFile(manifest); err != nil || string(after) != string(data) {
		t.Fatal("foreign manifest changed")
	}
	// Public ownership alone is insufficient for the current owner: full binding
	// proof remains mandatory and its error must not turn into empty inventory.
	if _, err := ownedNamespaceInventoryAt(context.Background(), root, foreign.Owner, read); err == nil || probes != 1 {
		t.Fatal("owned namespace omitted held-binding verification")
	}
	if err := os.Link(manifest, filepath.Join(folder, "foreign-hardlink")); err != nil {
		t.Fatal(err)
	}
	if _, err := ownedNamespaceInventoryAt(context.Background(), root, verifier.plan.Owner, read); err == nil {
		t.Fatal("foreign ownership with multiple links accepted")
	}
	if err := os.Remove(filepath.Join(folder, "foreign-hardlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(manifest, filepath.Join(folder, "safe-original")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("safe-original", manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := ownedNamespaceInventoryAt(context.Background(), root, verifier.plan.Owner, read); err == nil {
		t.Fatal("symlink foreign ownership accepted")
	}
	nested := filepath.Join(folder, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "ancestor-alias")
	if err := os.Symlink(folder, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := ownedNamespaceInventoryAt(context.Background(), filepath.Join(alias, "nested"), verifier.plan.Owner, read); err == nil {
		t.Fatal("symlink ancestor authenticated inventory root")
	}

}

func TestInitializationGateRefusesActivationAndPreservesInstalledChecks(t *testing.T) {
	r, verifier, preparation, _, _, events, spec := lifecycleFixture(t)
	r.RequireInitialization()
	if r.Ready(context.Background()) == nil {
		t.Fatal("uninitialized startup advertised activation readiness")
	}
	if _, err := r.Create(context.Background(), spec); err == nil || verifier.calls != 0 || len(*events) != 0 {
		t.Fatal("uninitialized startup reached transport verification, snapshot or unit")
	}
	r.SetInitializationReady(true)
	if r.Ready(context.Background()) != nil {
		t.Fatal("successful initialization introduced a circular active-profile prerequisite")
	}
	preparation.readyFail = true
	if r.Ready(context.Background()) == nil {
		t.Fatal("initialization bypassed actual installed component proof")
	}
	preparation.readyFail = false
	r.SetInitializationReady(false)
	if r.Ready(context.Background()) == nil {
		t.Fatal("failed reconnect initialization retained old readiness")
	}
}

type stoppedRepairFunc func(context.Context, *NetworkPlan) error

func (f stoppedRepairFunc) ExportExistingRepair(ctx context.Context, p *NetworkPlan) error {
	return f(ctx, p)
}

func TestStoppedRepairValidatesWholeInventoryBeforeMutation(t *testing.T) {
	for _, mode := range []string{"empty", "missing", "foreign", "duplicate", "live", "callback-failure", "canceled", "success"} {
		t.Run(mode, func(t *testing.T) {
			r, v, _, units, _, _, _ := lifecycleFixture(t)
			plan := *v.plan
			plans := []*NetworkPlan{&plan}
			if mode == "empty" {
				plans = nil
			}
			if mode == "foreign" {
				plan.Owner = "foreign"
			}
			if mode == "duplicate" {
				plans = append(plans, &plan)
			}
			if mode == "live" {
				units.started = true
			}
			r.SetNamespaceInventory(func(context.Context, string) ([]*NetworkPlan, error) { return plans, nil })
			calls := 0
			var callback NamespaceHandoffStoppedRepair = stoppedRepairFunc(func(ctx context.Context, p *NetworkPlan) error {
				calls++
				// The real implementation invokes this same guard; it must not deadlock.
				if err := r.TransportGuard(ctx, p); err != nil {
					return err
				}
				if mode == "callback-failure" {
					return ErrEngine
				}
				return nil
			})
			if mode == "missing" || mode == "empty" {
				callback = nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			err := r.RepairStoppedExports(ctx, callback)
			expected := mode == "success" || mode == "empty"
			if (err == nil) != expected {
				t.Fatalf("result %v", err)
			}
			if mode == "success" || mode == "callback-failure" {
				if calls != 1 {
					t.Fatal("repair not executed exactly once", calls)
				}
			} else if calls != 0 {
				t.Fatal("mutation before whole inventory proof", calls)
			}
		})
	}
}
