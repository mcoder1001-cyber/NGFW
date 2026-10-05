package ravpn

import (
	"context"
	"errors"
	"github.com/strongswan/govici/vici"
	"iter"
	"ngfw/agent/internal/renderers/strongswan"
	"ngfw/agent/internal/vpp/bootid"
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
	startFail, observeFail, stopFail bool
}

func (u *lifecycleUnits) Start(context.Context, *NetworkPlan) (UnitIdentity, error) {
	*u.events = append(*u.events, "start")
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
