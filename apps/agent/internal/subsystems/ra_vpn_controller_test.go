package subsystems

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
	"log/slog"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/ownertable"
	ravpn "ngfw/agent/internal/ra_vpn"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
	"ngfw/agent/internal/vpp/ifsanitize/sanitizetest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type raMemoryDescriptor struct {
	name   string
	rows   map[scheduler.Key]scheduler.KV
	fail   bool
	writes int
}

func (d *raMemoryDescriptor) Name() string { return d.name }
func (d *raMemoryDescriptor) KeyOf(v proto.Message) scheduler.Key {
	if route, ok := v.(*core.Route); ok {
		return scheduler.Join(d.name, core.RouteKey(route.TableId, route.Prefix).ID())
	}
	tap, _ := v.(*tapv2.Tap)
	return scheduler.Join(d.name, tap.Name)
}
func (d *raMemoryDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (d *raMemoryDescriptor) Create(_ context.Context, v proto.Message) (any, error) {
	d.writes++
	k := d.KeyOf(v)
	d.rows[k] = scheduler.KV{Key: k, Value: proto.Clone(v)}
	return nil, nil
}
func (d *raMemoryDescriptor) Update(ctx context.Context, _, v proto.Message, _ any) (any, error) {
	return d.Create(ctx, v)
}
func (d *raMemoryDescriptor) Delete(_ context.Context, v proto.Message, _ any) error {
	d.writes++
	delete(d.rows, d.KeyOf(v))
	return nil
}
func (d *raMemoryDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	if d.fail {
		return nil, errors.New("stale boot")
	}
	out := []scheduler.KV{}
	for _, v := range d.rows {
		out = append(out, v)
	}
	return out, nil
}
func (d *raMemoryDescriptor) RecordsNoOwnership() {}
func TestRAScopedOwnershipNormalApplyAndRollbackRemainDisjoint(t *testing.T) {
	ctx := context.Background()
	reg := scheduler.NewRegistry()
	rr := &raFilteringRegistry{Registry: reg, byName: map[string]scheduler.Descriptor{}}
	raw := &raMemoryDescriptor{name: core.RouteName, rows: map[scheduler.Key]scheduler.KV{}}
	rr.Register(raw)
	name := ravpn.LinkName(ravpn.InstanceID("w19", "road"), false)
	tap := &tapv2.Tap{Name: name}
	key := scheduler.Join(tapv2.TapName, name)
	taps := &raMemoryDescriptor{name: tapv2.TapName, rows: map[scheduler.Key]scheduler.KV{key: {Key: key, Value: tap, Meta: ravpn.TAPReceipt{Instance: ravpn.InstanceID("w19", "road"), NamespaceInode: 101, HostNamespaceInode: 102, Boot: bootid.Identity{BootID: "test", PID: 3, StartTime: 4}, Endpoint: proto.Clone(tap).(*tapv2.Tap)}}}}
	rr.Register(taps)
	normal, _ := reg.Get(core.RouteName)
	private, _ := reg.Get("remote-access." + core.RouteName)
	private.(*raScopedDescriptor).mutationGuard = func(context.Context, string) error { return nil }
	lan := &core.Route{TableId: 7, Prefix: "192.0.2.0/24", Paths: []*core.RoutePath{{Interface: "loop7", Weight: 1}}}
	road := &core.Route{TableId: 8, Prefix: "10.19.0.0/24", Paths: []*core.RoutePath{{Interface: name, Address: "198.18.19.3", Weight: 1}}}
	if _, e := normal.Create(ctx, lan); e != nil {
		t.Fatal(e)
	}
	if _, e := private.Create(ctx, road); e != nil {
		t.Fatal(e)
	}
	n, e := normal.Retrieve(ctx)
	if e != nil || len(n) != 1 || !proto.Equal(n[0].Value, lan) {
		t.Fatal("normal authority includes private route")
	}
	p, e := private.Retrieve(ctx)
	if e != nil || len(p) != 1 || !proto.Equal(p[0].Value, road) {
		t.Fatal("private authority includes ordinary route")
	}
	if _, e := normal.Update(ctx, lan, lan, nil); e != nil {
		t.Fatal(e)
	}
	if _, e := private.Retrieve(ctx); e != nil {
		t.Fatal("normal apply damaged RA")
	}
	colliding := proto.Clone(road).(*core.Route)
	colliding.Paths[0].Interface = "loop8"
	if _, e := normal.Create(ctx, colliding); e == nil {
		t.Fatal("normal route overwrote private prefix")
	}
	if _, e := normal.Create(ctx, road); e == nil {
		t.Fatal("ordinary desired adopted reserved RA path")
	}
	if e := private.Delete(ctx, road, nil); e != nil {
		t.Fatal(e)
	}
	n, e = normal.Retrieve(ctx)
	if e != nil || len(n) != 1 || !proto.Equal(n[0].Value, lan) {
		t.Fatal("RA rollback deleted ordinary route")
	}
	if len(raw.rows) != 1 {
		t.Fatal("private leftover")
	}
}
func TestRAScopedForeignReservedNameAndStaleBootFailBeforeMutation(t *testing.T) {
	ctx := context.Background()
	reg := scheduler.NewRegistry()
	rr := &raFilteringRegistry{Registry: reg, byName: map[string]scheduler.Descriptor{}}
	raw := &raMemoryDescriptor{name: core.RouteName, rows: map[scheduler.Key]scheduler.KV{}}
	rr.Register(raw)
	name := ravpn.LinkName(ravpn.InstanceID("w19", "foreign"), false)
	tap := &tapv2.Tap{Name: name}
	key := scheduler.Join(tapv2.TapName, name)
	taps := &raMemoryDescriptor{name: tapv2.TapName, rows: map[scheduler.Key]scheduler.KV{key: {Key: key, Value: tap}}}
	rr.Register(taps)
	private, _ := reg.Get("remote-access." + core.RouteName)
	private.(*raScopedDescriptor).mutationGuard = func(context.Context, string) error { return nil }
	route := &core.Route{Prefix: "10.19.0.0/24", Paths: []*core.RoutePath{{Interface: name, Weight: 1}}}
	if _, e := private.Create(ctx, route); e == nil || raw.writes != 0 {
		t.Fatal("foreign name adopted")
	}
	taps.rows[key] = scheduler.KV{Key: key, Value: tap, Meta: ravpn.TAPReceipt{Endpoint: proto.Clone(tap).(*tapv2.Tap)}}
	taps.fail = true
	if _, e := private.Create(ctx, route); e == nil || raw.writes != 0 {
		t.Fatal("stale boot adopted")
	}
}

func TestRAEnvironmentTwoOwnersAndCloseAreIsolated(t *testing.T) {
	regA, regB := scheduler.NewRegistry(), scheduler.NewRegistry()
	wA := &Wiring{env: Env{Owner: "w19-ra-a", StateDir: t.TempDir(), Log: slog.Default(), IDs: IDScope{}, RA: &RAControllerOptions{Inventory: func(context.Context, string) ([]*ravpn.NetworkPlan, error) { return nil, nil }}}}
	wB := &Wiring{env: Env{Owner: "w19-ra-b", StateDir: t.TempDir(), Log: slog.Default(), IDs: IDScope{}, RA: &RAControllerOptions{Inventory: func(context.Context, string) ([]*ravpn.NetworkPlan, error) { return nil, nil }}}}
	if wA.registerRAController(regA) != nil || wB.registerRAController(regB) != nil {
		t.Fatal("register")
	}
	defer wA.Close()
	defer wB.Close()
	a, b := RAEnvFor(wA.env.Owner), RAEnvFor(wB.env.Owner)
	if a.Owner != wA.env.Owner || b.Owner != wB.env.Owner || RARuntimeFor(a.Owner) == RARuntimeFor(b.Owner) {
		t.Fatal("last wiring adopted foreign owner")
	}
	if RAEnvFor("missing").Owner != "" {
		t.Fatal("singleton fallback")
	}
	wB.Close()
	if RAEnvFor(a.Owner).Owner != a.Owner || RARuntimeFor(a.Owner) == nil || RARuntimeFor(b.Owner) != nil || RAEnvFor(b.Owner).Owner != "" {
		t.Fatal("close damaged another owner")
	}
}

type raView map[scheduler.Key]proto.Message

func (v raView) Get(k scheduler.Key) (proto.Message, bool) { x, ok := v[k]; return x, ok }
func (v raView) List(name string) []scheduler.KV {
	var out []scheduler.KV
	for k, x := range v {
		if k.Descriptor() == name {
			out = append(out, scheduler.KV{Key: k, Value: x})
		}
	}
	return out
}

type raObserveOnly struct{ *raMemoryDescriptor }

func (*raObserveOnly) DeleteOnAbsence() bool { return false }
func TestRAScopedValidationRejectsCollisionBeforeAnyWrite(t *testing.T) {
	raw := &raMemoryDescriptor{name: core.RouteName, rows: map[scheduler.Key]scheduler.KV{}}
	normal := &raScopedDescriptor{inner: raw}
	route := &core.Route{TableId: 7, Prefix: "192.0.2.0/24", Paths: []*core.RoutePath{{Interface: "loop7", Weight: 1}}}
	key := normal.KeyOf(route)
	view := raView{scheduler.Join("remote-access."+core.RouteName, key.ID()): route}
	if normal.Validate(context.Background(), key, route, view) == nil || raw.writes != 0 {
		t.Fatal("collision was not rejected without mutation")
	}
	reserved := proto.Clone(route).(*core.Route)
	reserved.Paths[0].Interface = ravpn.LinkName(ravpn.InstanceID("w19", "road"), false)
	if normal.Validate(context.Background(), normal.KeyOf(reserved), reserved, raView{}) == nil {
		t.Fatal("normal desired reserved path accepted")
	}
	wrapped := &raScopedDescriptor{inner: &raObserveOnly{raw}, private: true}
	if wrapped.DeleteOnAbsence() {
		t.Fatal("observe-only semantics lost")
	}
}

func TestRAEnvironmentFailedCloseCannotBeReplacedBySameOwner(t *testing.T) {
	owner := "w19-ra-failed-close"
	state := t.TempDir()
	if err := os.Mkdir(filepath.Join(state, "ra-engine"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "ra-engine", "unknown"), []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	first := &Wiring{env: Env{Owner: owner, StateDir: state, Log: slog.Default(), IDs: IDScope{}}}
	if first.registerRAController(scheduler.NewRegistry()) != nil {
		t.Fatal("first register")
	}
	old := RARuntimeFor(owner)
	first.Close() // Ambiguous protected inventory makes stop/cleanup fail closed.
	if RARuntimeFor(owner) != old {
		t.Fatal("failed close discarded recovery state")
	}
	second := &Wiring{env: Env{Owner: owner, StateDir: state, Log: slog.Default(), IDs: IDScope{}}}
	registry := scheduler.NewRegistry()
	if second.registerRAController(registry) == nil || RARuntimeFor(owner) != old {
		t.Fatal("new wiring replaced failed generation")
	}
	if _, err := register(registry, second.env); err == nil {
		t.Fatal("duplicate owner reached descriptor registration")
	}
	if registry.Len() != 0 {
		t.Fatal("duplicate owner registered descriptors")
	}

	if _, exists := registry.Get(ravpn.EngineName); exists {
		t.Fatal("failed construction registered new engine descriptor")
	}
	if data, err := os.ReadFile(filepath.Join(state, "ra-engine", "unknown")); err != nil || string(data) != "foreign" {
		t.Fatal("foreign inventory mutated")
	}
	// Only test-owned global registration is released; no persistent file is adopted/deleted.
	raRuntimeMu.Lock()
	delete(raRuntimes, owner)
	delete(raEnvs, owner)
	raRuntimeMu.Unlock()
}

func TestRAEnvironmentFailedPersistentConstructionReleasesOnlyNewOwner(t *testing.T) {
	t.Chdir(t.TempDir()) // Legacy empty-StateDir constructors remain inside this disposable fixture.
	owner := "w19-ra-failed-construction"
	if _, err := Register(scheduler.NewRegistry(), Env{Owner: owner, Log: slog.Default()}); err == nil {
		t.Fatal("nonpersistent wiring accepted")
	}
	if RARuntimeFor(owner) != nil || RAEnvFor(owner).Owner != "" {
		t.Fatal("failed construction retained new owner callbacks")
	}
	// A subsequent construction reaches the persistence guard, not duplicate ownership.
	if _, err := Register(scheduler.NewRegistry(), Env{Owner: owner, Log: slog.Default()}); err == nil || strings.Contains(err.Error(), "already registered") {
		t.Fatal("failed construction stranded owner", err)
	}
}

func TestRAReconnectFailurePreventsSentinelAndEveryNativeVPPCall(t *testing.T) {
	owner := "ra-reconnect-events"
	state := t.TempDir()
	t.Setenv(EnvHostServicesDir, t.TempDir())
	v := coretest.New()
	model := sanitizetest.NewModel()
	model.Install(v.Client)
	owned, err := ownertable.Open(state, owner)
	if err != nil {
		t.Fatal(err)
	}
	w, err := Register(scheduler.NewRegistry(), Env{Client: v, Owner: owner, StateDir: state, Owned: owned, GlobalsOwner: true, Log: slog.Default(), RA: &RAControllerOptions{Inventory: func(context.Context, string) ([]*ravpn.NetworkPlan, error) { return nil, nil }}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	v.Reset()
	w.Connected(context.Background())
	calls := v.Calls()
	sentinelWrite, ping := -1, -1
	for index, call := range calls {
		switch call.GetMessageName() {
		case "classify_add_del_table":
			if sentinelWrite < 0 {
				sentinelWrite = index
			}
		case "control_ping":
			if ping < 0 {
				ping = index
			}
		}
	}
	if model.Created != 1 || len(calls) == 0 || calls[0].GetMessageName() != "classify_table_ids" || sentinelWrite < 0 || ping < sentinelWrite {
		t.Fatal("sentinel no longer precedes native VPP reconnect work")
	}
	// A corrupt protected engine inventory makes the REAL StopRA refuse cleanup.
	// No successful VPP read/write may occur after that failure, including sentinel repair.
	engineRoot := filepath.Join(state, "ra-engine")
	if err := os.Mkdir(engineRoot, 0700); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(engineRoot, "foreign")
	if err := os.WriteFile(unknown, []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	v.Reset()
	w.Connected(context.Background())
	if len(v.Calls()) != 0 || model.Created != 1 {
		t.Fatal("failed RA cleanup reached sentinel/native VPP")
	}
	if data, err := os.ReadFile(unknown); err != nil || string(data) != "foreign" {
		t.Fatal("failed RA cleanup adopted foreign inventory")
	}
	// Only this fixture-created sentinel file and its empty directory are removed
	// so exact wiring cleanup can finish under both root and nonroot execution.
	if err := os.Remove(unknown); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(engineRoot); err != nil {
		t.Fatal(err)
	}
}

func TestRAMountTargetUsesHeldNSFSAndRejectsProcessReplacement(t *testing.T) {
	identity := (bootid.Reader{}).ForPID(os.Getpid())
	target, err := raMountTarget(identity)
	if err != nil || target.MountInode == 0 || target.Boot != identity {
		t.Fatal("held own mount namespace refused", err)
	}
	changed := identity
	changed.StartTime++
	if _, err := raMountTarget(changed); err == nil {
		t.Fatal("reused process identity accepted")
	}
	if _, err := raMountTarget(bootid.Identity{}); err == nil {
		t.Fatal("unknown identity accepted")
	}
	// A proc namespace symlink inode is not the namespace inode returned by its held FD.
	var link unix.Stat_t
	if err := unix.Lstat("/proc/self/ns/mnt", &link); err != nil {
		t.Fatal(err)
	}
	if target.MountInode == link.Ino {
		t.Fatal("proc symlink inode adopted as NSFS identity")
	}
}

type raUnobservedUnit struct {
	live            bool
	inactivityCalls int
	stopCalls       int
}

func (*raUnobservedUnit) Start(context.Context, *ravpn.NetworkPlan) (ravpn.UnitIdentity, error) {
	return ravpn.UnitIdentity{}, ravpn.ErrEngine
}
func (*raUnobservedUnit) Observe(context.Context, *ravpn.NetworkPlan) (ravpn.UnitIdentity, error) {
	return ravpn.UnitIdentity{}, ravpn.ErrEngine
}
func (u *raUnobservedUnit) Stop(context.Context, *ravpn.NetworkPlan, ravpn.UnitIdentity) error {
	u.stopCalls++
	return ravpn.ErrEngine
}
func (u *raUnobservedUnit) Inactive(context.Context, *ravpn.NetworkPlan) error {
	u.inactivityCalls++
	if u.live {
		return ravpn.ErrEngine
	}
	return nil
}

func TestRAReconnectUnobservedRestartUnitPreventsEveryVPPEvent(t *testing.T) {
	owner := "ra-restart-no-record"
	state := t.TempDir()
	t.Setenv(EnvHostServicesDir, t.TempDir())
	v := coretest.New()
	model := sanitizetest.NewModel()
	model.Install(v.Client)
	owned, err := ownertable.Open(state, owner)
	if err != nil {
		t.Fatal(err)
	}
	plan := &ravpn.NetworkPlan{Format: 1, Owner: owner, Profile: "road", Instance: ravpn.InstanceID(owner, "road"), NamespaceInode: 101, HostNamespaceInode: 102, LocalAddress: "192.0.2.1", Outer: ravpn.Link{VPP: "198.18.0.0/31", Namespace: "198.18.0.1/31"}, Inner: ravpn.Link{VPP: "198.18.1.0/31", Namespace: "198.18.1.1/31"}, Pools: []string{"10.10.0.0/24"}}
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	units := &raUnobservedUnit{live: true}
	var startupEvents []string
	handoff := &raInitializationFixture{events: &startupEvents, repairCheck: func() {
		if len(v.Calls()) != 0 || model.Created != 0 {
			t.Fatal("repair ran after native VPP mutation")
		}
	}}
	// This fixture never activates; a concrete unconfigured sealed preparer keeps
	// the transport guard assembled while installed readiness remains false.
	options := &RAControllerOptions{Preparation: &ravpn.SealedPreparation{}, Handoff: handoff, Units: units, Inventory: func(_ context.Context, currentOwner string) ([]*ravpn.NetworkPlan, error) {
		if currentOwner != owner {
			t.Fatal("foreign inventory owner")
		}
		return []*ravpn.NetworkPlan{plan}, nil
	}}
	w, err := Register(scheduler.NewRegistry(), Env{Client: v, Owner: owner, StateDir: state, Owned: owned, GlobalsOwner: true, Log: slog.Default(), RA: options})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	foreign := filepath.Join(state, "foreign-object")
	if err := os.WriteFile(foreign, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(state, "ra-engine")); !os.IsNotExist(err) {
		t.Fatal("fixture already has engine receipt")
	}
	v.Reset()
	w.Connected(context.Background())
	if strings.Join(startupEvents, ",") != "source" {
		t.Fatal("target provisioning ran before positive global inactivity", startupEvents)
	}
	if len(v.Calls()) != 0 || model.Created != 0 || units.inactivityCalls != 1 || units.stopCalls != 0 {
		t.Fatal("no-record unknown unit reached VPP or was adopted")
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "preserve" {
		t.Fatal("foreign object changed")
	}
	if _, err := os.Stat(filepath.Join(state, "ra-engine")); !os.IsNotExist(err) {
		t.Fatal("unknown unit acquired invented receipt")
	}
	units.live = false // Explicit positive fixture-manager process exit proof permits teardown.
	if err := w.StopRA(context.Background()); err != nil {
		t.Fatal(err)
	}
	handoff.repairErr = ravpn.ErrEngine
	v.Reset()
	w.Connected(context.Background())
	if strings.Join(startupEvents, ",") != "source,source,targets,repair" || len(v.Calls()) != 0 || model.Created != 0 {
		t.Fatal("failed stopped repair reached VPP", startupEvents, v.Calls())
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "preserve" {
		t.Fatal("failed repair changed foreign object")
	}
	handoff.repairErr = nil
	v.Reset()
	w.Connected(context.Background())
	if strings.Join(startupEvents, ",") != "source,source,targets,repair,source,targets,repair" || model.Created != 1 {
		t.Fatal("safe reconnect did not repair before native sentinel", startupEvents)
	}
}

// Test-only self namespace observation; production uses manager-held descriptors.
// raMountTarget reads a held kernel namespace handle, bound to a complete process
// identity before and after its inode is read. Profile data never selects a path.
func raMountTarget(identity bootid.Identity) (ravpn.MountTarget, error) {
	if identity.PID <= 0 || !identity.Complete() || !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return ravpn.MountTarget{}, ravpn.ErrBoundary
	}
	//nolint:gosec // G304: the positive PID comes from verified VPP or fixed PID1; only a kernel NSFS path is opened.
	file, err := os.Open("/proc/" + strconv.Itoa(identity.PID) + "/ns/mnt")
	if err != nil {
		return ravpn.MountTarget{}, ravpn.ErrBoundary
	}
	defer func() { _ = file.Close() }()
	var stat unix.Stat_t
	var filesystem unix.Statfs_t
	namespaceType, typeError := unix.IoctlRetInt(int(file.Fd()), unix.NS_GET_NSTYPE)
	if typeError != nil || namespaceType != unix.CLONE_NEWNS || unix.Fstat(int(file.Fd()), &stat) != nil || unix.Fstatfs(int(file.Fd()), &filesystem) != nil || filesystem.Type != unix.NSFS_MAGIC || stat.Ino == 0 || !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return ravpn.MountTarget{}, ravpn.ErrBoundary
	}
	return ravpn.MountTarget{Boot: identity, MountInode: stat.Ino}, nil
}

func TestRADefaultFactoryUsesLazyManagerProvider(t *testing.T) {
	t.Setenv(EnvHostServicesDir, t.TempDir())
	client := coretest.New()
	state := t.TempDir()
	owned, err := ownertable.Open(state, "ra-default-manager-factory")
	if err != nil {
		t.Fatal(err)
	}
	registry := scheduler.NewRegistry()
	w, err := Register(registry, Env{Client: client, Owner: "ra-default-manager-factory", StateDir: state, Owned: owned, Log: slog.Default()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	descriptor, ok := registry.Get(ravpn.NamespaceName)
	if !ok {
		t.Fatal("namespace descriptor missing")
	}
	handoff, ok := descriptor.(*ravpn.NamespaceDescriptor).Handoff.(*ravpn.FixedNamespaceHandoff)
	if !ok || handoff.Targets != nil || handoff.Provider == nil {
		t.Fatal("default factory retained direct proc namespace path")
	}
	provider, ok := handoff.Provider.(*ravpn.SystemdNamespaceTargets)
	if !ok || provider.ExpectedVPP == nil {
		t.Fatal("connected VPP expectation missing")
	}
	w.env.Client = nil
	defer func() { w.env.Client = client }()
	if _, err := provider.ExpectedVPP(context.Background()); err == nil {
		t.Fatal("missing VPP connection accepted")
	}
	if RARuntimeFor(w.env.Owner).Ready(context.Background()) == nil {
		t.Fatal("unfinished/missing installed dependencies advertised ready")
	}
}

type raInitializationFixture struct {
	events               *[]string
	sourceErr, targetErr error
	repairErr            error
	repairCheck          func()
}

func (f *raInitializationFixture) Preflight(context.Context) error { return ravpn.ErrEngine }
func (f *raInitializationFixture) Export(context.Context, *ravpn.NetworkPlan) error {
	return ravpn.ErrEngine
}
func (f *raInitializationFixture) Verify(context.Context, *ravpn.NetworkPlan) error {
	return ravpn.ErrEngine
}
func (f *raInitializationFixture) Remove(context.Context, *ravpn.NetworkPlan) error {
	return ravpn.ErrEngine
}
func (f *raInitializationFixture) InitializeSource(context.Context) error {
	*f.events = append(*f.events, "source")
	return f.sourceErr
}
func (f *raInitializationFixture) ExportExistingRepair(context.Context, *ravpn.NetworkPlan) error {
	*f.events = append(*f.events, "repair")
	if f.repairCheck != nil {
		f.repairCheck()
	}
	return f.repairErr
}

func (f *raInitializationFixture) Initialize(context.Context) error {
	*f.events = append(*f.events, "targets")
	return f.targetErr
}

func TestRAInitializationIsBoundToExactWiringAndRequiresBothPhases(t *testing.T) {
	ctx := context.Background()
	first := ravpn.NewRuntime("first", nil, nil, nil, nil)
	second := ravpn.NewRuntime("second", nil, nil, nil, nil)
	var events []string
	fixture := &raInitializationFixture{events: &events}
	w := &Wiring{env: Env{Owner: "first", RA: &RAControllerOptions{Handoff: fixture}}}
	w.configureRAInitialization(first, fixture)
	other := &Wiring{env: Env{Owner: "second"}}
	other.configureRAInitialization(second, nil)
	if w.raStartup.runtime != first || other.raStartup.runtime != second {
		t.Fatal("foreign wiring runtime adopted")
	}
	if err := other.initializeRASource(ctx); err == nil {
		t.Fatal("missing production source initializer accepted")
	}
	if err := other.initializeRATargets(ctx); err == nil {
		t.Fatal("missing production target initializer accepted")
	}
	fixture.sourceErr = ravpn.ErrEngine
	if err := w.initializeRASource(ctx); err == nil {
		t.Fatal("failed source initializer accepted")
	}
	fixture.sourceErr = nil
	if err := w.initializeRASource(ctx); err != nil {
		t.Fatal(err)
	}
	fixture.targetErr = ravpn.ErrEngine
	if err := w.initializeRATargets(ctx); err == nil {
		t.Fatal("failed target initializer accepted")
	}
	fixture.targetErr = nil
	if err := w.initializeRATargets(ctx); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "source,source,targets,targets" {
		t.Fatalf("events %v", events)
	}
	// Successful phase callbacks cannot manufacture installed engine readiness.
	if err := first.Ready(ctx); err == nil {
		t.Fatal("initializer callbacks manufactured readiness")
	}
}

func TestRASupplierInitializationJournalContainsOnlyWhitelistedStage(t *testing.T) {
	for _, stage := range []uint8{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 255} {
		t.Run(fmt.Sprint(stage), func(t *testing.T) {
			var output bytes.Buffer
			var events []string
			fixture := &raInitializationFixture{events: &events, targetErr: fmt.Errorf("private-marker raw-unit-output: %w", &ravpn.SupplierInitializationFailure{Stage: stage})}
			runtime := ravpn.NewRuntime("journal-owner", nil, nil, nil, nil)
			w := &Wiring{env: Env{Owner: "journal-owner", RA: &RAControllerOptions{Handoff: fixture}, Log: slog.New(slog.NewJSONHandler(&output, nil))}}
			w.configureRAInitialization(runtime, fixture)
			err := w.initializeRATargets(context.Background())
			if err != ravpn.ErrEngine {
				t.Fatal("public failure was not redacted", err)
			}
			text := output.String()
			if strings.Contains(text, "private-marker") || strings.Contains(text, "raw-unit-output") {
				t.Fatal("private error escaped", text)
			}
			if stage >= 1 && stage <= 9 {
				if !strings.Contains(text, fmt.Sprintf("\"stage\":%d", stage)) {
					t.Fatal("bounded stage missing", text)
				}
			} else if text != "" {
				t.Fatal("unknown stage escaped", text)
			}
		})
	}
}

func TestRASupplierPublisherJournalOmitsUntrustedInnerFields(t *testing.T) {
	innerStages := []uint8{0, 25, 255}
	for stage := uint8(1); stage <= 24; stage++ {
		innerStages = append(innerStages, stage)
	}
	for _, outer := range []uint8{0, 1, 5, 6, 7, 9, 10, 255} {
		for _, inner := range innerStages {
			for _, deadline := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d-%d-%t", outer, inner, deadline), func(t *testing.T) {
					var output bytes.Buffer
					var events []string
					fixture := &raInitializationFixture{events: &events, targetErr: fmt.Errorf("private-marker raw-output: %w", &ravpn.SupplierInitializationFailure{Stage: outer, PublisherStage: inner, DeadlineExceeded: deadline})}
					runtime := ravpn.NewRuntime("inner-journal-owner", nil, nil, nil, nil)
					w := &Wiring{env: Env{Owner: "inner-journal-owner", RA: &RAControllerOptions{Handoff: fixture}, Log: slog.New(slog.NewJSONHandler(&output, nil))}}
					w.configureRAInitialization(runtime, fixture)
					if err := w.initializeRATargets(context.Background()); err != ravpn.ErrEngine {
						t.Fatal("public failure not redacted", err)
					}
					text := output.String()
					if strings.Contains(text, "private-marker") || strings.Contains(text, "raw-output") {
						t.Fatal("raw diagnostic escaped", text)
					}
					if outer == 0 || outer > 9 {
						if text != "" {
							t.Fatal("unknown outer stage escaped", text)
						}
						return
					}
					var row map[string]any
					if err := json.Unmarshal(output.Bytes(), &row); err != nil {
						t.Fatal(err)
					}
					publisher, hasPublisher := row["publisher_stage"]
					flag, hasDeadline := row["deadline_exceeded"]
					expected := outer == 6 && inner >= 1 && inner <= 24
					if hasPublisher != expected || hasDeadline != expected {
						t.Fatal("inner fields outside bounded stage6", row)
					}
					if expected && (publisher != float64(inner) || flag != deadline) {
						t.Fatal("incorrect fixed diagnostics", row)
					}
				})
			}
		}
	}
}
