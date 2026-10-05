package ravpn

import (
	"context"
	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/scheduler"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func requireRootOwnedFixture(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("positive root-owned descriptor fixture requires UID 0; production ownership checks remain enforced")
	}
}

func TestPersistentEngineRecordPhasesAndLinkedForeignRecords(t *testing.T) {
	requireRootOwnedFixture(t)
	dir := t.TempDir()
	store, e := NewFileEngineStore(dir, "w19")
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()
	_, _, _, units, _, _, spec := lifecycleFixture(t)
	record := EngineRecord{Spec: spec, Unit: units.id}
	if store.Save(record) != nil {
		t.Fatal("pending save")
	}
	record.Ready = true
	if store.Save(record) != nil {
		t.Fatal("ready transition")
	}
	records, e := store.List()
	if e != nil || len(records) != 1 || !records[0].Ready || records[0].Unit != units.id {
		t.Fatal("recovery", e)
	}
	name := filepath.Join(dir, "ra-engine", spec.Instance+".json")
	linked := filepath.Join(dir, "foreign.json")
	if os.Link(name, linked) != nil {
		t.Fatal("link fixture")
	}
	if _, e := store.List(); e == nil {
		t.Fatal("linked ownership record trusted")
	}
	if store.Remove(spec.Instance) == nil {
		t.Fatal("linked record removed")
	}
	if _, e := os.Stat(linked); e != nil {
		t.Fatal("foreign link deleted")
	}
	if os.Remove(linked) != nil {
		t.Fatal("remove owned fixture link")
	}
	if store.Remove(spec.Instance) != nil {
		t.Fatal("safe removal")
	}
}
func TestPersistentEngineRecordRefusesSymlinkStateParent(t *testing.T) {
	dir := t.TempDir()
	actual := filepath.Join(dir, "actual")
	if os.Mkdir(actual, 0700) != nil {
		t.Fatal("fixture")
	}
	link := filepath.Join(dir, "link")
	if os.Symlink(actual, link) != nil {
		t.Fatal("fixture")
	}
	if s, e := NewFileEngineStore(link, "w19"); e == nil {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
		t.Fatal("symlink parent trusted")
	}
}
func TestFailedStoreInventoryStillStopsKnownOwnedGeneration(t *testing.T) {
	requireRootOwnedFixture(t)
	r, _, _, units, store, events, spec := lifecycleFixture(t)
	if _, e := r.Create(context.Background(), spec); e != nil {
		t.Fatal(e)
	}
	units.stopFail = true
	if r.StopAll(context.Background()) == nil {
		t.Fatal("failed stop ignored")
	}
	if len(store.records) != 1 || len(r.active) != 1 {
		t.Fatal("owned failure forgotten")
	}
	units.stopFail = false
	if r.StopAll(context.Background()) != nil || len(r.active) != 0 || len(*events) == 0 {
		t.Fatal("retry failed")
	}
}

func TestPersistentEngineReadinessIsReadOnlyAndRejectsUnsafeLocation(t *testing.T) {
	requireRootOwnedFixture(t)
	root := t.TempDir()
	store := &LazyEngineStore{StateDir: root, Owner: "w19"}
	if store.Preflight(context.Background()) != nil {
		t.Fatal("protected unused location refused")
	}
	if _, err := os.Lstat(filepath.Join(root, "ra-engine")); !os.IsNotExist(err) {
		t.Fatal("preflight created durable directory")
	}
	// #nosec G302 -- deliberate unsafe mode tests rejection; restored below in this owned temporary fixture.
	if err := os.Chmod(root, 0777); err != nil {
		t.Fatal(err)
	}
	if store.Preflight(context.Background()) == nil {
		t.Fatal("writable state location accepted")
	}
	// #nosec G302 -- restore owner-only searchable fixture directory after the unsafe-mode negative.
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	foreign := t.TempDir()
	if err := os.Symlink(foreign, filepath.Join(root, "ra-engine")); err != nil {
		t.Fatal(err)
	}
	if store.Preflight(context.Background()) == nil {
		t.Fatal("symlink generation directory accepted")
	}
	if _, err := os.Lstat(filepath.Join(root, "ra-engine")); err != nil {
		t.Fatal("foreign link changed")
	}
}

// This fixture uses the actual EngineDescriptor/Runtime and scheduler undo.
// Transport descriptors model only owned object storage and enforce the same runtime barrier.
type guardedLifecycleTransport struct {
	name         string
	desired      map[scheduler.Key]proto.Message
	rows         map[scheduler.Key]scheduler.KV
	runtime      *Runtime
	plan         *NetworkPlan
	undoAttempts int
}

func (d *guardedLifecycleTransport) Name() string { return d.name }
func (d *guardedLifecycleTransport) KeyOf(value proto.Message) scheduler.Key {
	for key, wanted := range d.desired {
		if proto.Equal(value, wanted) {
			return key
		}
	}
	return ""
}
func (d *guardedLifecycleTransport) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (d *guardedLifecycleTransport) Create(ctx context.Context, value proto.Message) (any, error) {
	if d.runtime.TransportGuard(ctx, d.plan) != nil {
		return nil, ErrEngine
	}
	key := d.KeyOf(value)
	d.rows[key] = scheduler.KV{Key: key, Value: proto.Clone(value)}
	return nil, nil
}
func (d *guardedLifecycleTransport) Update(ctx context.Context, _, value proto.Message, _ any) (any, error) {
	return d.Create(ctx, value)
}
func (d *guardedLifecycleTransport) Delete(ctx context.Context, value proto.Message, _ any) error {
	d.undoAttempts++
	if d.runtime.TransportGuard(ctx, d.plan) != nil {
		return ErrEngine
	}
	delete(d.rows, d.KeyOf(value))
	return nil
}
func (d *guardedLifecycleTransport) Retrieve(context.Context) ([]scheduler.KV, error) {
	rows := []scheduler.KV{}
	for _, row := range d.rows {
		rows = append(rows, row)
	}
	return rows, nil
}
func (p *lifecyclePreparation) Validate(context.Context, EngineSpec) error {
	if p.fail {
		return ErrEngine
	}
	return nil
}

func TestSchedulerContinuingUndoCannotRemoveTransportAfterEngineStopFails(t *testing.T) {
	r, v, p, u, store, _, spec := lifecycleFixture(t)
	p.loadFail = true
	u.stopFail = true
	objects, err := TransportObjects(spec)
	if err != nil {
		t.Fatal(err)
	}
	registry := scheduler.NewRegistry()
	families := map[string]*guardedLifecycleTransport{}
	for _, object := range objects {
		name := object.Key.Descriptor()
		descriptor := families[name]
		if descriptor == nil {
			descriptor = &guardedLifecycleTransport{name: name, desired: map[scheduler.Key]proto.Message{}, rows: map[scheduler.Key]scheduler.KV{}, runtime: r, plan: v.plan}
			families[name] = descriptor
			registry.Register(descriptor)
		}
		descriptor.desired[object.Key] = object.Value
	}
	registry.Register(&EngineDescriptor{Runtime: r})
	value, err := spec.Proto()
	if err != nil {
		t.Fatal(err)
	}
	wanted := append(objects, scheduler.KV{Key: scheduler.Join(EngineName, spec.Instance), Value: value})
	reconciler := scheduler.New(registry, nil)
	reconciler.VerifyRetries = 0
	result := reconciler.Apply(context.Background(), wanted, nil)
	if result.Outcome != scheduler.OutcomeDegraded {
		t.Fatal("failed owned stop must degrade rollback", result.Outcome, result.Err)
	}
	attempts, remaining := 0, 0
	for _, descriptor := range families {
		attempts += descriptor.undoAttempts
		remaining += len(descriptor.rows)
	}
	if attempts == 0 || remaining != len(objects) {
		t.Fatal("continuing undo removed transport or did not exercise barrier", attempts, remaining, len(objects))
	}
	if !u.started || len(store.records) != 1 || r.active[spec.Instance].record.Ready {
		t.Fatal("failed live generation forgotten")
	}
}

func TestSchedulerPlanRejectsNewCredentialsBeforeMutatingExistingOrForeignObjects(t *testing.T) {
	r, v, p, u, store, events, spec := lifecycleFixture(t)
	objects, err := TransportObjects(spec)
	if err != nil {
		t.Fatal(err)
	}
	registry := scheduler.NewRegistry()
	families := map[string]*guardedLifecycleTransport{}
	for _, object := range objects {
		name := object.Key.Descriptor()
		descriptor := families[name]
		if descriptor == nil {
			descriptor = &guardedLifecycleTransport{name: name, desired: map[scheduler.Key]proto.Message{}, rows: map[scheduler.Key]scheduler.KV{}, runtime: r, plan: v.plan}
			families[name] = descriptor
			registry.Register(descriptor)
		}
		descriptor.desired[object.Key] = object.Value
		descriptor.rows[object.Key] = object
	}
	registry.Register(&EngineDescriptor{Runtime: r})
	value, err := spec.Proto()
	if err != nil {
		t.Fatal(err)
	}
	foreignKey := scheduler.Join("fixture.foreign", "preserve")
	foreign := &guardedLifecycleTransport{name: "fixture.foreign", desired: map[scheduler.Key]proto.Message{foreignKey: value}, rows: map[scheduler.Key]scheduler.KV{foreignKey: {Key: foreignKey, Value: value}}, runtime: r, plan: v.plan}
	registry.Register(foreign)
	if _, err := r.Create(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	before := len(*events)
	changed := spec
	changed.Fingerprints = map[string]string{"cert/server": "hmac:" + strings.Repeat("b", 64)}
	newValue, err := changed.Proto()
	if err != nil {
		t.Fatal(err)
	}
	wanted := append(objects, scheduler.KV{Key: scheduler.Join(EngineName, spec.Instance), Value: newValue})
	// Foreign state remains in the readback even when it would be absent from this desired set.
	p.fail = true
	reconciler := scheduler.New(registry, nil)
	plan, err := reconciler.Plan(context.Background(), wanted, nil)
	if err != nil || plan == nil || len(plan.Issues) == 0 {
		t.Fatal("bad sealed credentials accepted", err)
	}
	if len(*events) != before || !u.started || !r.active[spec.Instance].record.Ready || len(store.records) != 1 {
		t.Fatal("readonly plan quiesced existing daemon")
	}
	if store.records[spec.Instance].Spec.Fingerprints["cert/server"] != spec.Fingerprints["cert/server"] {
		t.Fatal("readonly plan replaced old immutable credentials")
	}
	for _, descriptor := range families {
		if len(descriptor.rows) != len(descriptor.desired) || descriptor.undoAttempts != 0 {
			t.Fatal("readonly plan changed owned transport")
		}
	}
	if len(foreign.rows) != 1 || foreign.undoAttempts != 0 {
		t.Fatal("bad new credentials changed foreign state")
	}
}
