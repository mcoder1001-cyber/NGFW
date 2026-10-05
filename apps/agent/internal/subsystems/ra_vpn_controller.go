package subsystems

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/core"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/desired"
	ravpn "ngfw/agent/internal/ra_vpn"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/secretchannel"
	"ngfw/agent/internal/vpp/bootid"
	"strings"
	"sync"
	"time"
)

// RA attributes have a disjoint descriptor namespace. Normal authoritative
// interfaces/routing/ACL transactions must not delete a live RA handoff.
var raSharedFamilies = map[string]bool{core.InterfaceTableName: true, core.InterfaceAddrName: true, core.RouteName: true, iface.AliasName: true, iface.AdminStateName: true, acl.NameInterfaceBinding: true}

func raInterface(name string) bool {
	name = strings.TrimPrefix(name, "interface/")
	if len(name) != 28 || !strings.HasPrefix(name, "ra_") || (name[27] != 'o' && name[27] != 'i') {
		return false
	}
	for _, c := range name[3:27] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}
func raObject(v proto.Message) bool {
	switch x := v.(type) {
	case *core.InterfaceAddress:
		return raInterface(x.Interface)
	case *core.InterfaceTable:
		return raInterface(x.Interface)
	case *core.Route:
		for _, p := range x.Paths {
			if raInterface(p.Interface) {
				return true
			}
		}
	case *iface.InterfaceAlias:
		return raInterface(x.Name)
	case *iface.AdminState:
		return raInterface(x.Interface)
	}
	if b, e := acl.InterfaceBindingFromProto(v); e == nil {
		return raInterface(b.Interface)
	}
	return false
}

type raScopedDescriptor struct {
	inner         scheduler.Descriptor
	private       bool
	reader        ravpn.DescriptorReader
	mutationGuard func(context.Context, string) error
}

func (d *raScopedDescriptor) Name() string {
	if d.private {
		return "remote-access." + d.inner.Name()
	}
	return d.inner.Name()
}
func (d *raScopedDescriptor) KeyOf(v proto.Message) scheduler.Key {
	k := d.inner.KeyOf(v)
	return scheduler.Join(d.Name(), k.ID())
}
func (d *raScopedDescriptor) Dependencies(v proto.Message) []scheduler.Dependency {
	deps := d.inner.Dependencies(v)
	if d.private {
		for i, dep := range deps {
			if raSharedFamilies[dep.Key.Descriptor()] && raInterface(dep.Key.ID()) {
				deps[i].Key = ravpn.PrivateKey(dep.Key)
			}
		}
	}
	return deps
}
func (d *raScopedDescriptor) Stage() scheduler.Stage { return scheduler.StageOf(d.inner) }

// Preserve optional descriptor semantics, especially observe-only interface aliases.
func (d *raScopedDescriptor) DeleteOnAbsence() bool {
	if a, ok := d.inner.(scheduler.AbsenceDeleter); ok {
		return a.DeleteOnAbsence()
	}
	return true
}
func (d *raScopedDescriptor) Normalize(v proto.Message) proto.Message {
	if n, ok := d.inner.(scheduler.Normalizer); ok {
		return n.Normalize(v)
	}
	return v
}
func (d *raScopedDescriptor) ProvidedKeys(v proto.Message) []scheduler.Key {
	p, ok := d.inner.(scheduler.KeyProvider)
	if !ok {
		return nil
	}
	keys := p.ProvidedKeys(v)
	if d.private {
		for i, k := range keys {
			keys[i] = ravpn.PrivateKey(k)
		}
	}
	return keys
}
func (d *raScopedDescriptor) Reapply(ctx context.Context, v proto.Message, meta any) error {
	r, ok := d.inner.(scheduler.Reapplier)
	if !ok {
		return nil
	}
	if raObject(v) != d.private || d.guard(ctx, v) != nil || d.mutation(ctx, v) != nil {
		return ravpn.ErrEngine
	}
	return r.Reapply(ctx, v, meta)
}

// Validate rejects conflicting ownership before the scheduler's first mutation.
func (d *raScopedDescriptor) Validate(ctx context.Context, key scheduler.Key, v proto.Message, view scheduler.ReadOnlyView) error {
	if ctx.Err() != nil || raObject(v) != d.private || d.KeyOf(v) != key {
		return ravpn.ErrEngine
	}
	other := d.inner.Name()
	if !d.private {
		other = "remote-access." + other
	}
	if _, exists := view.Get(scheduler.Join(other, key.ID())); exists {
		return ravpn.ErrEngine
	}
	if validator, ok := d.inner.(scheduler.Validator); ok {
		return validator.Validate(ctx, d.inner.KeyOf(v), v, view)
	}
	return nil
}
func (d *raScopedDescriptor) Create(ctx context.Context, v proto.Message) (any, error) {
	if raObject(v) != d.private || d.guard(ctx, v) != nil || d.mutation(ctx, v) != nil {
		return nil, ravpn.ErrEngine
	}
	return d.inner.Create(ctx, v)
}
func (d *raScopedDescriptor) Update(ctx context.Context, a, b proto.Message, m any) (any, error) {
	if raObject(a) != d.private || raObject(b) != d.private || d.guard(ctx, a) != nil || d.guard(ctx, b) != nil || d.mutation(ctx, a) != nil || d.mutation(ctx, b) != nil {
		return nil, ravpn.ErrEngine
	}
	return d.inner.Update(ctx, a, b, m)
}
func (d *raScopedDescriptor) Delete(ctx context.Context, v proto.Message, m any) error {
	if raObject(v) != d.private || d.guard(ctx, v) != nil || d.mutation(ctx, v) != nil {
		return ravpn.ErrEngine
	}
	return d.inner.Delete(ctx, v, m)
}
func (d *raScopedDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	rows, e := d.inner.Retrieve(ctx)
	if e != nil {
		return nil, e
	}
	out := []scheduler.KV{}
	for _, kv := range rows {
		if !d.private && raObject(kv.Value) {
			private := &raScopedDescriptor{inner: d.inner, private: true, reader: d.reader}
			if private.guard(ctx, kv.Value) != nil {
				return nil, ravpn.ErrEngine
			}
		}
		if raObject(kv.Value) == d.private {
			if d.guard(ctx, kv.Value) != nil {
				return nil, ravpn.ErrEngine
			}
			kv.Key = scheduler.Join(d.Name(), kv.Key.ID())
			out = append(out, kv)
		}
	}
	return out, nil
}

type raFilteringRegistry struct {
	scheduler.Registry
	byName map[string]scheduler.Descriptor
}

func (r *raFilteringRegistry) Register(d scheduler.Descriptor) {
	if raSharedFamilies[d.Name()] {
		normal := &raScopedDescriptor{inner: d, reader: r}
		private := &raScopedDescriptor{inner: d, private: true, reader: r}
		r.byName[normal.Name()] = normal
		r.byName[private.Name()] = private
		r.Registry.Register(normal)
		r.Registry.Register(private)
		return
	}
	r.byName[d.Name()] = d
	r.Registry.Register(d)
}
func init() {
	for name := range raSharedFamilies {
		Domains[VPN] = append(Domains[VPN], "remote-access."+name)
	}
	Domains[VPN] = append(Domains[VPN], ravpn.EngineName)
}

func registryReader(r scheduler.Registry) ravpn.DescriptorReader {
	x, _ := r.(ravpn.DescriptorReader)
	return x
}
func (d *raScopedDescriptor) guard(ctx context.Context, v proto.Message) error {
	if !d.private {
		if d.inner.Name() == core.RouteName {
			rows, e := d.inner.Retrieve(ctx)
			if e != nil {
				return e
			}
			key := d.inner.KeyOf(v)
			for _, row := range rows {
				if row.Key == key && raObject(row.Value) {
					return ravpn.ErrEngine
				}
			}
		}
		return nil
	}
	if d.inner.Name() == core.RouteName {
		rows, e := d.inner.Retrieve(ctx)
		if e != nil {
			return e
		}
		key := d.inner.KeyOf(v)
		for _, row := range rows {
			if row.Key == key && !raObject(row.Value) {
				return ravpn.ErrEngine
			}
		}
	}
	if d.reader == nil {
		return ravpn.ErrEngine
	}
	taps, ok := d.reader.Get(tapv2.TapName)
	if !ok {
		return ravpn.ErrEngine
	}
	rows, e := taps.Retrieve(ctx)
	if e != nil {
		return ravpn.ErrEngine
	}
	owned := map[string]bool{}
	for _, row := range rows {
		tap, ok := row.Value.(*tapv2.Tap)
		receipt, proof := row.Meta.(ravpn.TAPReceipt)
		if ok && proof && !receipt.Pending && receipt.Endpoint != nil && receipt.Boot.Complete() && receipt.Boot.PID > 0 && receipt.NamespaceInode != 0 && receipt.HostNamespaceInode != 0 && receipt.NamespaceInode != receipt.HostNamespaceInode && receipt.Index != ^uint32(0) && ravpn.ValidInstance(receipt.Instance) && (tap.Name == ravpn.LinkName(receipt.Instance, true) || tap.Name == ravpn.LinkName(receipt.Instance, false)) && proto.Equal(tap, receipt.Endpoint) {
			owned[tap.Name] = true
		}
	}
	names := []string{}
	switch x := v.(type) {
	case *core.InterfaceAddress:
		names = append(names, x.Interface)
	case *core.InterfaceTable:
		names = append(names, x.Interface)
	case *core.Route:
		for _, p := range x.Paths {
			if raInterface(p.Interface) {
				names = append(names, p.Interface)
			}
		}
	case *iface.InterfaceAlias:
		names = append(names, x.Name)
	case *iface.AdminState:
		names = append(names, strings.TrimPrefix(x.Interface, "interface/"))
	default:
		b, e := acl.InterfaceBindingFromProto(v)
		if e != nil {
			return ravpn.ErrEngine
		}
		names = append(names, b.Interface)
	}
	if len(names) == 0 {
		return ravpn.ErrEngine
	}
	for _, name := range names {
		if !owned[name] {
			return ravpn.ErrEngine
		}
	}
	return nil
}

func raObjectNames(v proto.Message) []string {
	var names []string
	switch x := v.(type) {
	case *core.InterfaceAddress:
		names = append(names, x.Interface)
	case *core.InterfaceTable:
		names = append(names, x.Interface)
	case *core.Route:
		for _, p := range x.Paths {
			if raInterface(p.Interface) {
				names = append(names, p.Interface)
			}
		}
	case *iface.InterfaceAlias:
		names = append(names, x.Name)
	case *iface.AdminState:
		names = append(names, strings.TrimPrefix(x.Interface, "interface/"))
	default:
		if binding, err := acl.InterfaceBindingFromProto(v); err == nil {
			names = append(names, binding.Interface)
		}
	}
	return names
}
func (d *raScopedDescriptor) mutation(ctx context.Context, v proto.Message) error {
	if !d.private {
		return nil
	}
	if d.mutationGuard == nil || d.reader == nil {
		return ravpn.ErrEngine
	}
	taps, ok := d.reader.Get(tapv2.TapName)
	if !ok {
		return ravpn.ErrEngine
	}
	rows, err := taps.Retrieve(ctx)
	if err != nil {
		return ravpn.ErrEngine
	}
	instances := map[string]string{}
	for _, row := range rows {
		tap, ok := row.Value.(*tapv2.Tap)
		receipt, proof := row.Meta.(ravpn.TAPReceipt)
		if ok && proof {
			instances[tap.Name] = receipt.Instance
		}
	}
	for _, name := range raObjectNames(v) {
		instance, exists := instances[name]
		if !exists || d.mutationGuard(ctx, instance) != nil {
			return ravpn.ErrEngine
		}
	}
	return nil
}

func (d *raScopedDescriptor) CheckPersistent() error {
	if check, ok := d.inner.(interface{ CheckPersistent() error }); ok {
		return check.CheckPersistent()
	}
	if _, ok := d.inner.(interface{ RecordsNoOwnership() }); ok {
		return nil
	}
	return ravpn.ErrEngine
}

func (r *raFilteringRegistry) Get(name string) (scheduler.Descriptor, bool) {
	d, ok := r.byName[name]
	return d, ok
}

var raRuntimeMu sync.RWMutex
var raRuntimes = map[string]*ravpn.Runtime{}
var raEnvs = map[string]desired.RAEnv{}

func RARuntimeFor(owner string) *ravpn.Runtime {
	raRuntimeMu.RLock()
	defer raRuntimeMu.RUnlock()
	return raRuntimes[owner]
}
func RAEnvFor(owner string) desired.RAEnv {
	raRuntimeMu.RLock()
	defer raRuntimeMu.RUnlock()
	return raEnvs[owner]
}
func (w *Wiring) registerRAController(reg scheduler.Registry) error {
	reader := registryReader(reg)
	store := &ravpn.LazyEngineStore{StateDir: w.env.StateDir, Owner: w.env.Owner}
	verifier := &ravpn.RegistryVerifier{Registry: reader, Boot: func(ctx context.Context) (bootid.Identity, error) { return bootid.Current(ctx, w.env.Client) }, Tables: func(ctx context.Context, index uint32) (uint32, uint32, error) {
		service := interfaces.NewServiceClient(w.env.Client)
		v4, err := service.SwInterfaceGetTable(ctx, &interfaces.SwInterfaceGetTable{SwIfIndex: interface_types.InterfaceIndex(index)})
		if err != nil {
			return 0, 0, ravpn.ErrEngine
		}
		v6, err := service.SwInterfaceGetTable(ctx, &interfaces.SwInterfaceGetTable{SwIfIndex: interface_types.InterfaceIndex(index), IsIPv6: true})
		if err != nil {
			return 0, 0, ravpn.ErrEngine
		}
		return v4.VrfID, v6.VrfID, nil
	}}
	var preparation ravpn.SnapshotPreparation
	var units ravpn.UnitSupervisor = ravpn.SystemdUnits{Observation: ravpn.NewSystemdUnitObservation()}
	if w.env.RA != nil {
		preparation = w.env.RA.Preparation
		if w.env.RA.Units != nil {
			units = w.env.RA.Units
		}
	}
	runtime := ravpn.NewRuntime(w.env.Owner, verifier, preparation, units, store)
	if w.env.RA != nil && w.env.RA.Inventory != nil {
		runtime.SetNamespaceInventory(w.env.RA.Inventory)
	}
	if reader != nil {
		for name := range raSharedFamilies {
			if descriptor, ok := reader.Get("remote-access." + name); ok {
				if private, ok := descriptor.(*raScopedDescriptor); ok {
					private.mutationGuard = func(ctx context.Context, instance string) error {
						plan, err := ravpn.ReadAgentPlan(instance)
						if err != nil {
							return ravpn.ErrEngine
						}
						return runtime.TransportGuard(ctx, plan)
					}
				}
			}
		}
		if descriptor, ok := reader.Get(ravpn.NamespaceName); ok {
			if namespace, ok := descriptor.(*ravpn.NamespaceDescriptor); ok {
				namespace.Guard = runtime.TransportGuard
				w.raRepair, _ = namespace.Handoff.(ravpn.NamespaceHandoffStoppedRepair)
				if fixed, ok := namespace.Handoff.(*ravpn.FixedNamespaceHandoff); ok {
					fixed.Guard = runtime.TransportGuard
				}
				w.configureRAInitialization(runtime, namespace.Handoff)
			}
		}
		if descriptor, ok := reader.Get(tapv2.TapName); ok {
			if tap, ok := descriptor.(*ravpn.GuardedTAP); ok {
				tap.Guard = runtime.TransportGuard
			}
		}
	}
	ids, e := w.IDRange()
	if e != nil {
		ids = NoIDs()
	}
	span := desired.TunnelIDSpan{All: ids == nil}
	if ids != nil {
		span.Lo = ids.Lo
		span.Hi = ids.Hi
	}
	runtime.SetReadiness(func(ctx context.Context) error {
		if readiness, ok := units.(ravpn.EngineReadiness); ok && readiness.Preflight(ctx) != nil {
			return ravpn.ErrEngine
		}
		if reader == nil || w.env.Client == nil || (!span.All && (span.Lo > min(span.Hi, 8191) || uint64(min(span.Hi, 8191))-uint64(span.Lo)+1 < 2)) {
			return ravpn.ErrEngine
		}
		if ravpn.HostPrerequisites() != nil {
			return ravpn.ErrEngine
		}
		descriptor, exists := reader.Get(ravpn.NamespaceName)
		if !exists {
			return ravpn.ErrEngine
		}
		namespace, ok := descriptor.(*ravpn.NamespaceDescriptor)
		if !ok || namespace.Handoff == nil || namespace.Handoff.Preflight(ctx) != nil {
			return ravpn.ErrEngine
		}
		id, e := bootid.Current(ctx, w.env.Client)
		if e != nil || !id.Complete() || id.PID <= 0 {
			return ravpn.ErrEngine
		}
		for name := range raSharedFamilies {
			d, ok := reader.Get("remote-access." + name)
			if !ok {
				return ravpn.ErrEngine
			}
			if _, err := d.Retrieve(ctx); err != nil {
				return ravpn.ErrEngine
			}
		}
		if w.env.RA != nil && w.env.RA.Readiness != nil {
			if w.env.RA.Readiness(ctx) != nil {
				return ravpn.ErrEngine
			}
		}
		return nil
	})
	raRuntimeMu.Lock()
	if _, exists := raRuntimes[w.env.Owner]; exists {
		raRuntimeMu.Unlock()
		return ravpn.ErrEngine // Existing/failed owner state requires a verified recovery handoff.
	}
	raRuntimes[w.env.Owner] = runtime
	env := desired.RAEnv{Owner: w.env.Owner, IDs: span, Ready: runtime.Ready}
	if w.env.RA != nil {
		env.SecretRef = w.env.RA.SecretRef
	}
	raEnvs[w.env.Owner] = env
	raRuntimeMu.Unlock()
	reg.Register(&ravpn.EngineDescriptor{Runtime: runtime})
	w.OnClose(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if e := runtime.StopAll(ctx); e != nil {
			w.env.Log.Error("remote-access shutdown refused", "reason", "verified generation stop failed")
			return // Keep the failed owned generation and its store available for recovery.
		}
		store.Close()
		raRuntimeMu.Lock()
		if raRuntimes[w.env.Owner] == runtime {
			delete(raRuntimes, w.env.Owner)
			delete(raEnvs, w.env.Owner)
		}
		raRuntimeMu.Unlock()
	})
	return nil
}
func SetRASecrets(owner string, cache *secretchannel.Store) error {
	rt := RARuntimeFor(owner)
	if rt == nil || cache == nil {
		return ravpn.ErrEngine
	}
	rt.SetPreparation(&ravpn.SealedPreparation{Resolver: cache, Readiness: func(ctx context.Context) error {
		if ctx.Err() != nil {
			return ravpn.ErrEngine
		}
		id, e := cache.ID(nil)
		if e != nil || !strings.HasPrefix(id, "hmac:") || !ravpn.ValidInstance(strings.TrimPrefix(id, "hmac:")) {
			return ravpn.ErrEngine
		}
		return nil
	}})
	raRuntimeMu.Lock()
	env := raEnvs[owner]
	env.SecretRef = cache.Ref
	raEnvs[owner] = env
	raRuntimeMu.Unlock()
	return nil
}

func (w *Wiring) StopRA(ctx context.Context) error {
	rt := RARuntimeFor(w.env.Owner)
	if rt == nil {
		return nil
	}
	return rt.StopAll(ctx)
}

// RAControllerOptions is a trusted Go construction seam for disposable private
// fixtures, not an environment/config/API bypass. Verification of real VPP
// ownership, policy, IDs, routes and persistent records is always retained.
type RAControllerOptions struct {
	Inventory   ravpn.NamespacePlanInventory
	Handoff     ravpn.NamespaceHandoff
	SecretRef   func(context.Context, string) (string, error)
	Preparation ravpn.SnapshotPreparation
	Units       ravpn.UnitSupervisor
	Readiness   func(context.Context) error
}

// SetVirtualAddressSource preserves the existing VRRP address classifier across
// the scoped wrapper so ordinary reconciliation does not remove active VIPs.
func (d *raScopedDescriptor) SetVirtualAddressSource(source core.VirtualAddressSource) {
	if hook, ok := d.inner.(interface {
		SetVirtualAddressSource(core.VirtualAddressSource)
	}); ok {
		hook.SetVirtualAddressSource(source)
	}
}

// raStartupInitialization binds initialization to this exact wiring/runtime.
// Source preparation performs no transport or manager mutation; target supplier
// provisioning runs only after the global owned-unit inactivity barrier.
type raStartupInitialization struct {
	runtime *ravpn.Runtime
	source  ravpn.NamespaceHandoffSourceInitialization
	targets ravpn.NamespaceHandoffInitialization
}

func (w *Wiring) configureRAInitialization(runtime *ravpn.Runtime, handoff ravpn.NamespaceHandoff) {
	source, sourceOK := handoff.(ravpn.NamespaceHandoffSourceInitialization)
	targets, targetsOK := handoff.(ravpn.NamespaceHandoffInitialization)
	// Explicit trusted fixture handoffs may omit both startup phases. The
	// production default always requires both, including an incomplete provider.
	required := w.env.RA == nil || w.env.RA.Handoff == nil || sourceOK || targetsOK
	if !required {
		return
	}
	runtime.RequireInitialization()
	w.raStartup = &raStartupInitialization{runtime: runtime, source: source, targets: targets}
}

func (w *Wiring) initializeRASource(ctx context.Context) error {
	startup := w.raStartup
	if startup == nil {
		return nil
	}
	startup.runtime.SetInitializationReady(false)
	if startup.source == nil {
		return ravpn.ErrEngine
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if startup.source.InitializeSource(bounded) != nil {
		return ravpn.ErrEngine
	}
	return nil
}

func (w *Wiring) initializeRATargets(ctx context.Context) error {
	startup := w.raStartup
	if startup == nil {
		return nil
	}
	if startup.targets == nil {
		return ravpn.ErrEngine
	}
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := startup.targets.Initialize(bounded); err != nil {
		var failure *ravpn.SupplierInitializationFailure
		if errors.As(err, &failure) && failure != nil && failure.Stage >= 1 && failure.Stage <= 9 && w.env.Log != nil {
			if failure.Stage == 6 && failure.PublisherStage >= 1 && failure.PublisherStage <= 24 {
				w.env.Log.Warn("remote-access supplier initialization refused", "stage", failure.Stage, "publisher_stage", failure.PublisherStage, "deadline_exceeded", failure.DeadlineExceeded)
			} else {
				w.env.Log.Warn("remote-access supplier initialization refused", "stage", failure.Stage)
			}
		}
		return ravpn.ErrEngine
	}
	startup.runtime.SetInitializationReady(true)
	return nil
}
