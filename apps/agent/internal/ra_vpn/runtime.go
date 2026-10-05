package ravpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/renderers/strongswan"
)

// UnitIdentity is freshly verified against the fixed unit cgroup, executable,
// PID start ticks and owned private network namespace. PID alone is never trust.
type UnitIdentity struct {
	BootID         string
	PID            int
	StartTicks     uint64
	NamespaceInode uint64
}

// Valid reports whether the complete process and namespace identity is usable.
func (u UnitIdentity) Valid() bool {
	return u.PID > 1 && u.StartTicks != 0 && u.NamespaceInode != 0 && u.BootID != ""
}

// Generation binds session identity to the exact kernel boot and daemon process.
func (u UnitIdentity) Generation(instance string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%d:%d", instance, u.BootID, u.PID, u.StartTicks, u.NamespaceInode)))
	return hex.EncodeToString(sum[:])
}

// UnitRetirement proves a persisted generation is retired without stopping a replacement.
type UnitRetirement interface {
	Retired(context.Context, EngineSpec, UnitIdentity) (bool, error)
}

// UnitSupervisor controls only a verified fixed private engine unit.
type UnitSupervisor interface {
	Start(context.Context, *NetworkPlan) (UnitIdentity, error)
	Observe(context.Context, *NetworkPlan) (UnitIdentity, error)
	Stop(context.Context, *NetworkPlan, UnitIdentity) error
}

// UnitQuiescence proves the fixed unit has no active process or untracked
// private socket before a transport mutation. Errors never imply inactivity.
type UnitQuiescence interface {
	Inactive(context.Context, *NetworkPlan) error
}

// HandoffVerifier reads back the owned transport before daemon activation.
type HandoffVerifier interface {
	Verify(context.Context, EngineSpec) (*NetworkPlan, Handoff, error)
}

// SnapshotRecovery reconstructs an authenticated existing credential generation without overwriting it.
type SnapshotRecovery interface {
	Recover(context.Context, EngineSpec) (*PreparedEngine, error)
}

// EngineRecord persists the public transport and process identity of one generation.
type EngineRecord struct {
	Ready   bool
	Spec    EngineSpec
	Handoff Handoff
	Unit    UnitIdentity
}

// EngineStore stores public generation records without secret material.
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
	mu                     sync.Mutex
	owner                  string
	verifier               HandoffVerifier
	preparation            SnapshotPreparation
	units                  UnitSupervisor
	store                  EngineStore
	dial                   func(context.Context, *NetworkPlan, UnitIdentity) (strongswan.ViciConn, error)
	readPlan               func(string) (*NetworkPlan, error)
	inventory              NamespacePlanInventory
	readiness              func(context.Context) error
	active                 map[string]*engineGeneration
	initializationRequired bool
	initializationReady    bool
}

// NewRuntime defines the public private-engine lifecycle contract.
func NewRuntime(owner string, v HandoffVerifier, p SnapshotPreparation, u UnitSupervisor, s EngineStore) *Runtime {
	return &Runtime{owner: owner, verifier: v, preparation: p, units: u, store: s, active: map[string]*engineGeneration{}, dial: verifiedDial, readPlan: ReadAgentPlan, inventory: ownedNamespaceInventory}
}

// ownedNamespaceInventory is independent of EngineRecord persistence and VPP
// handoff. Even an interrupted start with no observed identity must leave its
// protected full-instance plan visible to the global quiescence barrier.
func ownedNamespaceInventory(ctx context.Context, owner string) ([]*NetworkPlan, error) {
	return ownedNamespaceInventoryAt(ctx, InstanceRoot, owner, ReadAgentPlan)
}

func ownedNamespaceInventoryAt(ctx context.Context, rootPath, owner string, readPlan func(string) (*NetworkPlan, error)) ([]*NetworkPlan, error) {
	if ctx.Err() != nil {
		return nil, ErrEngine
	}
	fd, err := openInventoryDirectory(rootPath)
	if err == unix.ENOENT {
		return nil, nil
	}
	if err != nil {
		return nil, ErrEngine
	}
	root := os.NewFile(uintptr(fd), rootPath)
	defer func() { _ = root.Close() }()
	var before, after unix.Stat_t
	if unix.Fstat(fd, &before) != nil || before.Uid != 0 || before.Mode&0077 != 0 {
		return nil, ErrEngine
	}
	entries, err := root.ReadDir(4097)
	if err != nil && err != io.EOF || len(entries) > 4096 {
		return nil, ErrEngine
	}
	var plans []*NetworkPlan
	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, ErrEngine
		}
		if !ValidInstance(entry.Name()) {
			continue
		}
		if !entry.IsDir() {
			return nil, ErrEngine
		}
		manifest, err := readInventoryManifest(fd, entry.Name())
		if err != nil {
			return nil, ErrEngine
		}
		if manifest.Owner != owner {
			continue
		} // Protected ownership is known; never adopt/probe foreign NETNS.
		if readPlan == nil {
			return nil, ErrEngine
		}
		plan, err := readPlan(entry.Name())
		if err != nil || plan == nil || plan.Validate() != nil || plan.Instance != manifest.Instance || plan.Owner != manifest.Owner || plan.Profile != manifest.Profile || plan.NamespaceInode != manifest.NamespaceInode || plan.HostNamespaceInode != manifest.HostNamespaceInode {
			return nil, ErrEngine
		}
		plans = append(plans, plan)
		if len(plans) > 64 {
			return nil, ErrEngine
		}
	}
	if unix.Fstat(fd, &after) != nil || before.Ino != after.Ino || before.Dev != after.Dev || before.Mtim != after.Mtim || before.Ctim != after.Ctim {
		return nil, ErrEngine
	}
	fresh, err := openInventoryDirectory(rootPath)
	if err != nil {
		return nil, ErrEngine
	}
	var current unix.Stat_t
	err = unix.Fstat(fresh, &current)
	_ = unix.Close(fresh)
	if err != nil || before.Ino != current.Ino || before.Dev != current.Dev || before.Mode != current.Mode || before.Uid != current.Uid {
		return nil, ErrEngine
	}
	return plans, nil
}

func openInventoryDirectory(path string) (int, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return -1, ErrEngine
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) > 32 {
		return -1, ErrEngine
	}
	descriptor, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range parts {
		next, err := unix.Openat(descriptor, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(descriptor)
		if err != nil {
			return -1, err
		}
		descriptor = next
		var info unix.Stat_t
		if unix.Fstat(descriptor, &info) != nil || info.Uid != 0 || info.Mode&0022 != 0 && info.Mode&unix.S_ISVTX == 0 {
			_ = unix.Close(descriptor)
			return -1, ErrEngine
		}
	}
	return descriptor, nil
}

// readInventoryManifest authenticates root-private public ownership metadata
// without opening any foreign owner's network namespace. The current owner is
// still required to pass the full held-binding ReadAgentPlan verification.
func readInventoryManifest(root int, instance string) (*NetworkPlan, error) {
	if !ValidInstance(instance) {
		return nil, ErrEngine
	}
	directory, err := unix.Openat(root, instance, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrEngine
	}
	defer func() { _ = unix.Close(directory) }()
	var info unix.Stat_t
	if unix.Fstat(directory, &info) != nil || info.Uid != 0 || info.Mode&0077 != 0 {
		return nil, ErrEngine
	}
	descriptor, err := unix.Openat(directory, "network.json", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrEngine
	}
	file := os.NewFile(uintptr(descriptor), "protected public namespace manifest")
	defer func() { _ = file.Close() }()
	var before, after unix.Stat_t
	if unix.Fstat(descriptor, &before) != nil || before.Uid != 0 || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Mode&0077 != 0 || before.Nlink != 1 || before.Size < 1 || before.Size > 16384 {
		return nil, ErrEngine
	}
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil || len(data) > 16384 || unix.Fstat(descriptor, &after) != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Mode != after.Mode || before.Uid != after.Uid || before.Gid != after.Gid || before.Nlink != after.Nlink || before.Size != after.Size || before.Mtim != after.Mtim || before.Ctim != after.Ctim {
		return nil, ErrEngine
	}
	plan, err := DecodePrivatePlan(data, instance)
	if err != nil {
		return nil, ErrEngine
	}
	return plan, nil
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
	if !r.configured() || s.Validate() != nil || s.Owner != r.owner || r.initializationRequired && !r.initializationReady {
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
	if unit.Valid() && r.store.Save(record) != nil {
		return record, ErrEngine
	}
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
	defer func() { _ = client.Close() }()
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
	record.Ready = true
	generation.record = record
	if r.store.Save(record) != nil {
		return record, ErrEngine
	}
	return record, nil
}

// Create starts a verified isolated engine generation.
func (r *Runtime) Create(ctx context.Context, s EngineSpec) (EngineRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.create(ctx, s)
}
func (r *Runtime) stop(ctx context.Context, g *engineGeneration) error {
	wasReady := g.record.Ready
	g.record.Ready = false
	plan, err := r.readPlan(g.record.Spec.Instance)
	if err != nil {
		return ErrEngine
	}
	unloadFailed := false
	if wasReady && g.prepared != nil && g.prepared.Unload != nil {
		// VICI cleanup is useful readback, but never prevents stopping an owned
		// generation during daemon/VPP failure or transport repair.
		observed, observeErr := r.units.Observe(ctx, plan)
		if observeErr == nil && observed == g.record.Unit {
			client, dialErr := r.dial(ctx, plan, g.record.Unit)
			if dialErr == nil {
				unloadFailed = g.prepared.Unload(ctx, client) != nil
				_ = client.Close()
			} else {
				unloadFailed = true
			}
		} else {
			unloadFailed = true
		}
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
	if unloadFailed {
		return ErrEngine
	}
	return nil
}

// Delete stops the verified owned generation before cleanup.
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

// Recover defines the public private-engine lifecycle contract.
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
		if !record.Ready {
			return ErrEngine
		} // Retrieve/Plan recovery must remain read-only.
	}
	return nil
}

// Records defines the public private-engine lifecycle contract.
func (r *Runtime) Records(ctx context.Context) ([]EngineRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.configured() {
		return nil, ErrEngine
	}
	out := make([]EngineRecord, 0, len(r.active))
	for _, g := range r.active {
		if !g.record.Ready {
			return nil, ErrEngine
		}
		if _, err := r.verify(ctx, g); err != nil {
			return nil, ErrEngine
		}
		out = append(out, g.record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec.Profile < out[j].Spec.Profile })
	return out, nil
}

// Operational defines the public private-engine lifecycle contract.
func (r *Runtime) Operational(ctx context.Context) bool {
	records, err := r.Records(ctx)
	return err == nil && len(records) > 0
}

// Sessions defines the public private-engine lifecycle contract.
func (r *Runtime) Sessions(ctx context.Context, profile string) ([]strongswan.RASession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.active[InstanceID(r.owner, profile)]
	if g == nil || !g.record.Ready {
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
	defer func() { _ = client.Close() }()
	sessions, err := strongswan.ObserveRASessions(ctx, client, profile, g.record.Unit.Generation(plan.Instance), g.record.Spec.Configuration.GetPools())
	if err != nil || len(sessions) > 200 {
		return nil, ErrEngine
	}
	return sessions, nil
}

// Disconnect defines the public private-engine lifecycle contract.
func (r *Runtime) Disconnect(ctx context.Context, profile, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !ValidInstance(id) {
		return ErrEngine
	}
	g := r.active[InstanceID(r.owner, profile)]
	if g == nil || !g.record.Ready {
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
	defer func() { _ = client.Close() }()
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
	if r.store == nil {
		return nil
	}
	for _, g := range r.active {
		if e := r.stop(ctx, g); e != nil {
			return e
		}
	}
	records, e := r.store.List()
	if e != nil {
		return ErrEngine
	}
	if len(records) > 64 {
		return ErrEngine
	}
	for _, record := range records {
		if record.Spec.Owner != r.owner {
			return ErrEngine
		}
		if retirement, ok := r.units.(UnitRetirement); ok {
			retired, e := retirement.Retired(ctx, record.Spec, record.Unit)
			if e != nil {
				return ErrEngine
			}
			if retired {
				if r.store.Remove(record.Spec.Instance) != nil {
					return ErrEngine
				}
				continue
			}
		}
		if _, exists := r.active[record.Spec.Instance]; exists {
			continue
		}
		recoverer, ok := r.preparation.(SnapshotRecovery)
		if !ok {
			_ = r.stopRecorded(ctx, record)
			return ErrEngine
		}
		p, e := recoverer.Recover(ctx, record.Spec)
		if e != nil || p == nil || p.Cleanup == nil {
			_ = r.stopRecorded(ctx, record)
			return ErrEngine
		}
		r.active[record.Spec.Instance] = &engineGeneration{record: record, prepared: p}
	}
	for _, g := range r.active {
		if e := r.stop(ctx, g); e != nil {
			return e
		}
	}
	return r.verifyInventoryQuiescence(ctx, false)
}

// verifyInventoryQuiescence is called under r.mu. Ready may retain instances
// whose complete active identity was freshly verified by its caller; global
// StopAll requires every owned protected instance to be positively inactive.
func (r *Runtime) verifyInventoryQuiescence(ctx context.Context, permitReady bool) error {
	if r.inventory == nil {
		return ErrEngine
	}
	plans, err := r.inventory(ctx, r.owner)
	if err != nil || len(plans) > 64 {
		return ErrEngine
	}
	inactive, ok := r.units.(UnitQuiescence)
	for _, plan := range plans {
		if plan == nil || plan.Validate() != nil || plan.NamespaceInode == 0 || plan.HostNamespaceInode == 0 || plan.NamespaceInode == plan.HostNamespaceInode || plan.Owner != r.owner {
			return ErrEngine
		}
		if permitReady {
			if generation, exists := r.active[plan.Instance]; exists {
				record := generation.record
				if !record.Ready || record.Spec.Profile != plan.Profile || record.Handoff.NamespaceInode != plan.NamespaceInode || record.Handoff.HostNamespaceInode != plan.HostNamespaceInode {
					return ErrEngine
				}
				continue
			}
		}
		if !ok || inactive.Inactive(ctx, plan) != nil {
			return ErrEngine
		}
	}
	return nil
}

// stopRecorded can stop the authenticated daemon without credential recovery.
// A failed sealed cache never permits replacement adoption or snapshot removal.
func (r *Runtime) stopRecorded(ctx context.Context, record EngineRecord) error {
	plan, err := r.readPlan(record.Spec.Instance)
	if err != nil || plan.Owner != r.owner || plan.Profile != record.Spec.Profile || plan.Instance != record.Spec.Instance {
		return ErrEngine
	}
	return r.units.Stop(ctx, plan, record.Unit)
}

// SetNamespaceInventory binds trusted private fixture inventory. Every returned
// plan still requires complete owner validation and positive unit inactivity.
func (r *Runtime) SetNamespaceInventory(inventory NamespacePlanInventory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inventory = inventory
}

// RequireInitialization prevents activation readiness until an explicit startup
// initializer succeeds. It never bypasses installed component verification.
func (r *Runtime) RequireInitialization() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initializationRequired = true
	r.initializationReady = false
}

// SetInitializationReady records a startup result; Ready still independently
// verifies installed components, transport ownership and observed generations.
func (r *Runtime) SetInitializationReady(ready bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initializationRequired = true
	r.initializationReady = ready
}

// SetReadiness defines the public private-engine lifecycle contract.
func (r *Runtime) SetReadiness(check func(context.Context) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readiness = check
}

// Ready defines the public private-engine lifecycle contract.
func (r *Runtime) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.configured() || r.readiness == nil || r.initializationRequired && !r.initializationReady {
		return ErrEngine
	}
	for _, g := range r.active {
		if !g.record.Ready {
			return ErrEngine
		}
		if _, err := r.verify(ctx, g); err != nil {
			return ErrEngine
		}
	}
	if protected, ok := r.store.(interface{ Preflight(context.Context) error }); ok && protected.Preflight(ctx) != nil {
		return ErrEngine
	}
	records, err := r.store.List()
	if err != nil || len(records) > 64 {
		return ErrEngine
	}
	for _, record := range records {
		g, exists := r.active[record.Spec.Instance]
		if !exists || !g.record.Ready || g.record.Unit != record.Unit || record.Spec.Owner != r.owner {
			return ErrEngine
		}
	}
	if r.verifyInventoryQuiescence(ctx, true) != nil {
		return ErrEngine
	}
	p, ok := r.preparation.(EngineReadiness)
	if !ok || p.Preflight(ctx) != nil || r.readiness(ctx) != nil {
		return ErrEngine
	}
	return nil
}

// SetPreparation defines the public private-engine lifecycle contract.
func (r *Runtime) SetPreparation(p SnapshotPreparation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.preparation = p
}

// TransportGuard refuses mutation while an active, pending, persisted or
// untracked unit can still use this instance. It never infers inactivity from errors.
func (r *Runtime) TransportGuard(ctx context.Context, plan *NetworkPlan) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.configured() || plan == nil || plan.Validate() != nil || plan.Owner != r.owner {
		return ErrEngine
	}
	if _, exists := r.active[plan.Instance]; exists {
		return ErrEngine
	}
	records, err := r.store.List()
	if err != nil || len(records) > 64 {
		return ErrEngine
	}
	for _, record := range records {
		if record.Spec.Owner != r.owner || record.Spec.Instance == plan.Instance {
			return ErrEngine
		}
	}
	inactive, ok := r.units.(UnitQuiescence)
	if !ok || inactive.Inactive(ctx, plan) != nil {
		return ErrEngine
	}
	return nil
}
func equalEngineSpec(a, b EngineSpec) (bool, error) {
	av, err := a.Proto()
	if err != nil {
		return false, ErrEngine
	}
	bv, err := b.Proto()
	if err != nil {
		return false, ErrEngine
	}
	return proto.Equal(av, bv), nil
}

// QuiesceChanged stops only changed/removed profiles after full transaction
// validation. Returned immutable specifications support verified rollback recovery.
func (r *Runtime) QuiesceChanged(ctx context.Context, wanted []EngineSpec) ([]EngineSpec, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.configured() || len(wanted) > 64 {
		return nil, ErrEngine
	}
	targets := map[string]EngineSpec{}
	for _, spec := range wanted {
		if spec.Validate() != nil || spec.Owner != r.owner {
			return nil, ErrEngine
		}
		if _, exists := targets[spec.Instance]; exists {
			return nil, ErrEngine
		}
		targets[spec.Instance] = spec
	}
	changed := []string{}
	for instance, g := range r.active {
		if !g.record.Ready {
			return nil, ErrEngine
		}
		target, exists := targets[instance]
		if exists {
			same, err := equalEngineSpec(g.record.Spec, target)
			if err != nil {
				return nil, err
			}
			if same {
				continue
			}
		}
		changed = append(changed, instance)
	}
	sort.Strings(changed)
	previous := make([]EngineSpec, 0, len(changed))
	for _, instance := range changed {
		g := r.active[instance]
		previous = append(previous, g.record.Spec)
		if err := r.stop(ctx, g); err != nil {
			return previous, ErrEngine
		}
	}
	return previous, nil
}

// Restore recreates previous generations only after full old transport and
// immutable sealed credential verification. Failed partial effects remain tracked.
func (r *Runtime) Restore(ctx context.Context, previous []EngineSpec) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.configured() || len(previous) > 64 {
		return ErrEngine
	}
	for _, spec := range previous {
		if spec.Validate() != nil || spec.Owner != r.owner {
			return ErrEngine
		}
	}
	failed := false
	for _, spec := range previous {
		if existing, exists := r.active[spec.Instance]; exists {
			same, err := equalEngineSpec(existing.record.Spec, spec)
			if err != nil || !same || !existing.record.Ready {
				failed = true
				continue
			}
			if _, err := r.verify(ctx, existing); err != nil {
				failed = true
				continue
			}
			continue
		}
		if r.validateActivation(ctx, spec) != nil {
			failed = true
			continue
		}
		record, err := r.create(ctx, spec)
		if err != nil {
			if record.Spec.Instance != "" {
				if g := r.active[record.Spec.Instance]; g != nil {
					_ = r.stop(ctx, g)
				}
			}
			failed = true
		}
	}
	if failed {
		return ErrEngine
	}
	return nil
}

// validateActivation is read-only and runs with the runtime lock held.
func (r *Runtime) validateActivation(ctx context.Context, spec EngineSpec) error {
	if !r.configured() || spec.Owner != r.owner {
		return ErrEngine
	}
	if g, exists := r.active[spec.Instance]; exists {
		if !g.record.Ready {
			return ErrEngine
		}
		_, err := r.verify(ctx, g)
		return err
	}
	records, err := r.store.List()
	if err != nil || len(records) > 64 {
		return ErrEngine
	}
	for _, record := range records {
		if record.Spec.Owner != r.owner || record.Spec.Instance == spec.Instance {
			return ErrEngine
		}
	}
	plan, err := BuildNetworkPlan(spec.Owner, spec.Profile, spec.Configuration)
	if err != nil {
		return ErrEngine
	}
	inactive, ok := r.units.(UnitQuiescence)
	if !ok || inactive.Inactive(ctx, plan) != nil {
		return ErrEngine
	}
	return nil
}

// ChangeRequired reports whether this VPN transaction replaces a running engine.
// Dormant/native-only VPN transactions do not require an RA engine installation.
func (r *Runtime) ChangeRequired(ctx context.Context, wanted []EngineSpec) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.store == nil || ctx.Err() != nil {
		return false, ErrEngine
	}
	records, err := r.store.List()
	if err != nil || len(records) > 64 {
		return false, ErrEngine
	}
	for _, record := range records {
		if _, exists := r.active[record.Spec.Instance]; !exists {
			return false, ErrEngine
		}
	}
	targets := map[string]EngineSpec{}
	for _, spec := range wanted {
		if spec.Owner != r.owner || spec.Validate() != nil {
			return false, ErrEngine
		}
		if _, exists := targets[spec.Instance]; exists {
			return false, ErrEngine
		}
		targets[spec.Instance] = spec
	}
	for instance, g := range r.active {
		target, exists := targets[instance]
		if !exists {
			return true, nil
		}
		same, err := equalEngineSpec(g.record.Spec, target)
		if err != nil {
			return false, ErrEngine
		}
		if !same {
			return true, nil
		}
	}
	return false, nil
}
