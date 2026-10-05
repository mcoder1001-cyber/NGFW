package subsystems

import (
	"context"
	"google.golang.org/protobuf/proto"
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
	if strings.HasPrefix(name, "interface/") {
		name = strings.TrimPrefix(name, "interface/")
	}
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
	inner   scheduler.Descriptor
	private bool
	reader  ravpn.DescriptorReader
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
func (d *raScopedDescriptor) Create(ctx context.Context, v proto.Message) (any, error) {
	if raObject(v) != d.private || d.guard(ctx, v) != nil {
		return nil, ravpn.ErrEngine
	}
	return d.inner.Create(ctx, v)
}
func (d *raScopedDescriptor) Update(ctx context.Context, a, b proto.Message, m any) (any, error) {
	if raObject(a) != d.private || raObject(b) != d.private || d.guard(ctx, a) != nil || d.guard(ctx, b) != nil {
		return nil, ravpn.ErrEngine
	}
	return d.inner.Update(ctx, a, b, m)
}
func (d *raScopedDescriptor) Delete(ctx context.Context, v proto.Message, m any) error {
	if raObject(v) != d.private || d.guard(ctx, v) != nil {
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
		if ok && proof && !receipt.Pending && receipt.Endpoint != nil && proto.Equal(tap, receipt.Endpoint) {
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
var raCurrent desired.RAEnv

func RARuntimeFor(owner string) *ravpn.Runtime {
	raRuntimeMu.RLock()
	defer raRuntimeMu.RUnlock()
	return raRuntimes[owner]
}
func RAProjection() desired.RAEnv { raRuntimeMu.RLock(); defer raRuntimeMu.RUnlock(); return raCurrent }
func (w *Wiring) registerRAController(reg scheduler.Registry) error {
	reader := registryReader(reg)
	store := &ravpn.LazyEngineStore{StateDir: w.env.StateDir, Owner: w.env.Owner}
	verifier := &ravpn.RegistryVerifier{Registry: reader, Boot: func(ctx context.Context) (bootid.Identity, error) { return bootid.Current(ctx, w.env.Client) }}
	runtime := ravpn.NewRuntime(w.env.Owner, verifier, nil, ravpn.SystemdUnits{}, store)
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
		if reader == nil || w.env.Client == nil || (!span.All && (span.Lo > span.Hi || span.Lo > 8191)) {
			return ravpn.ErrEngine
		}
		id, e := bootid.Current(ctx, w.env.Client)
		if e != nil || !id.Complete() || id.PID <= 0 {
			return ravpn.ErrEngine
		}
		for name := range raSharedFamilies {
			if _, ok := reader.Get("remote-access." + name); !ok {
				return ravpn.ErrEngine
			}
		}
		return nil
	})
	reg.Register(&ravpn.EngineDescriptor{Runtime: runtime})
	raRuntimeMu.Lock()
	raRuntimes[w.env.Owner] = runtime
	raCurrent = desired.RAEnv{Owner: w.env.Owner, IDs: span, Ready: runtime.Ready}
	raRuntimeMu.Unlock()
	w.OnClose(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if e := runtime.StopAll(ctx); e != nil {
			w.env.Log.Error("remote-access shutdown refused", "reason", "verified generation stop failed")
		}
		store.Close()
		raRuntimeMu.Lock()
		if raRuntimes[w.env.Owner] == runtime {
			delete(raRuntimes, w.env.Owner)
			if raCurrent.Owner == w.env.Owner {
				raCurrent = desired.RAEnv{}
			}
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
	rt.SetPreparation(&ravpn.SealedPreparation{Resolver: cache})
	raRuntimeMu.Lock()
	if raCurrent.Owner == owner {
		raCurrent.SecretRef = cache.Ref
	}
	raRuntimeMu.Unlock()
	return nil
}
