package ravpn

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"ngfw/agent/internal/renderers/strongswan"
)

// UnitIdentity is freshly verified against the fixed unit cgroup, executable,
// PID start ticks and owned private network namespace. PID alone is never trust.
type UnitIdentity struct {
	PID            int
	StartTicks     uint64
	NamespaceInode uint64
}

func (u UnitIdentity) Valid() bool { return u.PID > 1 && u.StartTicks != 0 && u.NamespaceInode != 0 }
func (u UnitIdentity) Generation(instance string) string {
	return fmt.Sprintf("%s:%d:%d", instance, u.PID, u.StartTicks)
}

type UnitSupervisor interface {
	Start(context.Context, *NetworkPlan) (UnitIdentity, error)
	Observe(context.Context, *NetworkPlan) (UnitIdentity, error)
	Stop(context.Context, *NetworkPlan, UnitIdentity) error
}
type HandoffVerifier interface {
	Verify(context.Context, EngineSpec) (*NetworkPlan, Handoff, error)
}
type SnapshotRecovery interface {
	Recover(context.Context, EngineSpec) (*PreparedEngine, error)
}
type EngineRecord struct {
	Spec    EngineSpec
	Handoff Handoff
	Unit    UnitIdentity
}
type EngineStore interface {
	List() ([]EngineRecord, error)
	Save(EngineRecord) error
	Remove(string) error
}
type engineGeneration struct {
	record   EngineRecord
	prepared *PreparedEngine
}

// Runtime serializes lifecycle and observations. Its callbacks are injected in
// disposable tests; constructing it never starts a system daemon.
type Runtime struct {
	mu          sync.Mutex
	owner       string
	verifier    HandoffVerifier
	preparation SnapshotPreparation
	units       UnitSupervisor
	store       EngineStore
	dial        func(context.Context, *NetworkPlan, UnitIdentity) (strongswan.ViciConn, error)
	active      map[string]*engineGeneration
}

func NewRuntime(owner string, v HandoffVerifier, p SnapshotPreparation, u UnitSupervisor, s EngineStore) *Runtime {
	return &Runtime{owner: owner, verifier: v, preparation: p, units: u, store: s, active: map[string]*engineGeneration{}, dial: verifiedDial}
}
func verifiedDial(ctx context.Context, plan *NetworkPlan, unit UnitIdentity) (strongswan.ViciConn, error) {
	if !unit.Valid() || unit.NamespaceInode != plan.NamespaceInode {
		return nil, ErrEngine
	}
	path := InstanceRoot + "/" + plan.Instance + "/daemon/vici.sock"
	if strongswan.RestrictRAVICISocket(ctx, path, unit.PID) != nil {
		return nil, ErrEngine
	}
	return strongswan.DialRAVICI(ctx, path, unit.PID)
}
func (r *Runtime) configured() bool {
	return r != nil && safeOwnerName(r.owner) && r.verifier != nil && r.preparation != nil && r.units != nil && r.store != nil
}
func (r *Runtime) create(ctx context.Context, s EngineSpec) (EngineRecord, error) {
	if !r.configured() || s.Validate() != nil || s.Owner != r.owner {
		return EngineRecord{}, ErrEngine
	}
	if _, exists := r.active[s.Instance]; exists {
		return EngineRecord{}, ErrEngine
	}
	plan, handoff, err := r.verifier.Verify(ctx, s)
	if err != nil {
		return EngineRecord{}, ErrEngine
	}
	p, err := r.preparation.Prepare(ctx, s)
	if err != nil || p == nil || p.Load == nil || p.Cleanup == nil {
		return EngineRecord{}, ErrEngine
	}
	unit, err := r.units.Start(ctx, plan)
	record := EngineRecord{Spec: s, Handoff: handoff, Unit: unit}
	generation := &engineGeneration{record: record, prepared: p}
	r.active[s.Instance] = generation // retain partial effects for the scheduler's rollback
	if err != nil || !unit.Valid() {
		return record, ErrEngine
	}
	observed, err := r.units.Observe(ctx, plan)
	if err != nil || observed != unit {
		return record, ErrEngine
	}
	client, err := r.dial(ctx, plan, unit)
	if err != nil {
		return record, ErrEngine
	}
	defer client.Close()
	if p.Load(ctx, client) != nil {
		return record, ErrEngine
	}
	// A second readback closes transport/VPP boot races during start/load.
	_, after, err := r.verifier.Verify(ctx, s)
	if err != nil || after != handoff {
		return record, ErrEngine
	}
	if _, err := strongswan.ObserveRASessions(ctx, client, s.Profile, unit.Generation(s.Instance), s.Configuration.GetPools()); err != nil {
		return record, ErrEngine
	}
	if r.store.Save(record) != nil {
		return record, ErrEngine
	}
	return record, nil
}
func (r *Runtime) Create(ctx context.Context, s EngineSpec) (EngineRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.create(ctx, s)
}
func (r *Runtime) stop(ctx context.Context, g *engineGeneration) error {
	plan, err := ReadAgentPlan(g.record.Spec.Instance)
	if err != nil {
		return ErrEngine
	}
	// Stop uses the recorded process identity, independently of current VPP state;
	// this must succeed before namespace/TAP repair or credential cleanup.
	if r.units.Stop(ctx, plan, g.record.Unit) != nil {
		return ErrEngine
	}
	if g.prepared == nil || g.prepared.Cleanup == nil || g.prepared.Cleanup(ctx) != nil {
		return ErrEngine
	}
	if r.store.Remove(g.record.Spec.Instance) != nil {
		return ErrEngine
	}
	delete(r.active, g.record.Spec.Instance)
	return nil
}
func (r *Runtime) Delete(ctx context.Context, s EngineSpec) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.configured() || s.Owner != r.owner {
		return ErrEngine
	}
	g := r.active[s.Instance]
	if g == nil {
		return ErrEngine
	}
	return r.stop(ctx, g)
}
func (r *Runtime) verify(ctx context.Context, g *engineGeneration) (*NetworkPlan, error) {
	plan, handoff, err := r.verifier.Verify(ctx, g.record.Spec)
	if err != nil || handoff != g.record.Handoff {
		return nil, ErrEngine
	}
	unit, err := r.units.Observe(ctx, plan)
	if err != nil || unit != g.record.Unit {
		return nil, ErrEngine
	}
	return plan, nil
}
func (r *Runtime) Recover(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.configured() {
		return ErrEngine
	}
	records, err := r.store.List()
	if err != nil || len(records) > 64 {
		return ErrEngine
	}
	recoverer, ok := r.preparation.(SnapshotRecovery)
	if !ok && len(records) > 0 {
		return ErrEngine
	}
	for _, record := range records {
		if record.Spec.Validate() != nil || record.Spec.Owner != r.owner {
			return ErrEngine
		}
		if _, exists := r.active[record.Spec.Instance]; exists {
			continue
		}
		g := &engineGeneration{record: record}
		if _, err := r.verify(ctx, g); err != nil {
			return ErrEngine
		}
		p, err := recoverer.Recover(ctx, record.Spec)
		if err != nil || p == nil || p.Cleanup == nil {
			return ErrEngine
		}
		g.prepared = p
		r.active[record.Spec.Instance] = g
	}
	return nil
}
func (r *Runtime) Records(ctx context.Context) ([]EngineRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.configured() {
		return nil, ErrEngine
	}
	out := make([]EngineRecord, 0, len(r.active))
	for _, g := range r.active {
		if _, err := r.verify(ctx, g); err != nil {
			return nil, ErrEngine
		}
		out = append(out, g.record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec.Profile < out[j].Spec.Profile })
	return out, nil
}
func (r *Runtime) Operational(ctx context.Context) bool {
	records, err := r.Records(ctx)
	return err == nil && len(records) > 0
}
func (r *Runtime) Sessions(ctx context.Context, profile string) ([]strongswan.RASession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.active[InstanceID(r.owner, profile)]
	if g == nil {
		return nil, ErrEngine
	}
	plan, err := r.verify(ctx, g)
	if err != nil {
		return nil, ErrEngine
	}
	client, err := r.dial(ctx, plan, g.record.Unit)
	if err != nil {
		return nil, ErrEngine
	}
	defer client.Close()
	sessions, err := strongswan.ObserveRASessions(ctx, client, profile, g.record.Unit.Generation(plan.Instance), g.record.Spec.Configuration.GetPools())
	if err != nil || len(sessions) > 200 {
		return nil, ErrEngine
	}
	return sessions, nil
}
func (r *Runtime) Disconnect(ctx context.Context, profile, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !ValidInstance(id) {
		return ErrEngine
	}
	g := r.active[InstanceID(r.owner, profile)]
	if g == nil {
		return ErrEngine
	}
	plan, err := r.verify(ctx, g)
	if err != nil {
		return ErrEngine
	}
	client, err := r.dial(ctx, plan, g.record.Unit)
	if err != nil {
		return ErrEngine
	}
	defer client.Close()
	if strongswan.DisconnectRASession(ctx, client, profile, g.record.Unit.Generation(plan.Instance), id, g.record.Spec.Configuration.GetPools()) != nil {
		return ErrEngine
	}
	return nil
}

// StopAll is called before VPP disconnect repair and shutdown. It never removes
// transport first. A failed stop retains the generation and fails closed.
func (r *Runtime) StopAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, g := range r.active {
		if err := r.stop(ctx, g); err != nil {
			return err
		}
	}
	return nil
}
