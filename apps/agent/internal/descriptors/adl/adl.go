// Package adl implements the descriptors of VPP's adl plugin (allow/deny lists, D2.4):
// the per-interface ADL switch and the per-interface allow-list table binding. The plugin
// has no dump message: adl.interface reads its presence back through the feature arc
// (binapi/feature feature_is_enabled, confirmed against VPP's error encoding — V23 a);
// adl.allowlist is write-only (Retrieve returns df2.ErrRetrieveUnsupported, D-063) and not in
// the default Register (docs/agent/descriptors/adl.md). Messages come from
// apps/agent/binapi/adl and binapi/feature only.
//
// # The allow-list call is not safe to send naively (VPP 26.06, plugins/adl/adl.c)
//
// adl_allowlist_enable_disable touches all three ADL families (ip4, ip6, "default" = non-IP)
// on every call: a family whose flag is set gets one more allow-list feature (the add is not
// idempotent: a repeat stacks a second instance), a family whose flag is clear gets one
// removed — and removing a feature that is not configured stores config index ~0 for that
// family (vnet_config_del_feature returns ~0 and the caller keeps it), which adl-input then
// dereferences for the next packet of that family: a crash vector while adl-input is enabled
// (source analysis; never reproduced on the shared VPP). The "default" family's allow-list
// node is a stub ("BUG: stub function called", the frame's buffers are not freed), so it must
// never stay configured. The descriptor therefore only ever sends two-call sequences that never
// remove a family that is not configured and never leave "default" on:
//
//	add    (1,1,1) then (ip4, ip6, 0)   → every checked family holds two identical instances,
//	                                      the others and "default" none
//	remove (!ip4, !ip6, 1) then (0,0,0) → back to none everywhere
//
// and keeps an applied-once record per interface (D-076, keyed by the D-080 boot identity and
// the sw_if_index) so that a resync never stacks a third instance. adl.interface depends
// (optionally) on adl.allowlist, so the allow-list is configured while adl-input is still off,
// adl-input is switched off before the allow-list is removed, and an allow-list recreate
// re-creates adl.interface around it (V-new in docs/vpp-code-track.md, F-rpf-adl-pbr).
package adl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	adlapi "ngfw/agent/binapi/adl"
	featureapi "ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// Descriptor names; keys are "adl.interface/<interface>" and "adl.allowlist/<interface>".
const (
	InterfaceName = "adl.interface"
	AllowlistName = "adl.allowlist"
)

// Meta is the runtime handle of both descriptors.
type Meta struct{ SwIfIndex uint32 }

// resolve returns the interface an object is created on (another owner's is refused) and
// whether it is untagged (then the object is claimed).
func resolve(ctx context.Context, c vpp.Client, owner, name string) (interface_types.InterfaceIndex, bool, error) {
	ifs, err := df2.DumpInterfaces(ctx, c, owner)
	if err != nil {
		return 0, false, err
	}
	idx, untagged, err := ifs.Resolve(name)
	return interface_types.InterfaceIndex(idx), untagged, err
}

// The ADL input feature (plugins/adl/adl.c): arc device-input, node adl-input.
const (
	adlArc     = "device-input"
	adlFeature = "adl-input"
	// controlFeature is the device-input arc's end node (vnet/devices/devices.c): registered as a
	// feature, but never part of a config's feature list (it is the config's end node), so
	// feature_is_enabled(device-input, ethernet-input, idx) is true only when the query itself
	// failed — the V23 (a) control query.
	controlFeature = "ethernet-input"
	// local0 never carries ADL (DF-2 refuses it; VPP's adl config skips the local device class):
	// adl-input reading true there means VPP does not know the adl-input feature.
	local0 = 0
)

// InterfaceDescriptor enables the ADL input feature (adl_interface_enable_disable). The adl
// plugin has no dump, but the feature arc does: Retrieve reads presence with
// feature_is_enabled(device-input, adl-input) on this owner's interfaces, confirmed (V23 a).
type InterfaceDescriptor struct {
	client vpp.Client
	owner  string
	opts   df2.Options
}

// NewInterface returns the descriptor for the given owner; df2.WithClaims attributes ADL on
// untagged interfaces.
func NewInterface(c vpp.Client, owner string, opts ...df2.Option) *InterfaceDescriptor {
	return &InterfaceDescriptor{client: c, owner: owner, opts: df2.BuildOptions(opts...)}
}

// Name implements scheduler.Descriptor.
func (*InterfaceDescriptor) Name() string { return InterfaceName }

// KeyOf implements scheduler.Descriptor.
func (*InterfaceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(InterfaceName, obj.(*Interface).GetInterface())
}

// Dependencies implements scheduler.Descriptor: the interface, and — optional, ordering only —
// the interface's allow-list binding, which must be configured before adl-input starts sending
// packets through it and removed only after adl-input is off (see the package doc).
func (*InterfaceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	name := obj.(*Interface).GetInterface()
	return []scheduler.Dependency{
		df2.InterfaceDep(name),
		{Key: scheduler.Join(AllowlistName, name), Optional: true},
	}
}

func (d *InterfaceDescriptor) set(ctx context.Context, idx interface_types.InterfaceIndex, enable bool) error {
	if _, err := adlapi.NewServiceClient(d.client).AdlInterfaceEnableDisable(ctx, &adlapi.AdlInterfaceEnableDisable{SwIfIndex: idx, EnableDisable: enable}); err != nil {
		return fmt.Errorf("adl_interface_enable_disable: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *InterfaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	idx, untagged, err := resolve(ctx, d.client, d.owner, obj.(*Interface).GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, idx, true); err != nil {
		return nil, err
	}
	if err := df2.Claim(d.opts.Claims, untagged, d.KeyOf(obj)); err != nil {
		return nil, err
	}
	return Meta{SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor: the interface is the key.
func (*InterfaceDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *InterfaceDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(Meta)
	if !ok {
		return fmt.Errorf("%s: %w %T", InterfaceName, df2.ErrBadMeta, meta)
	}
	if skip, err := df2.SkipDelete(ctx, d.client, d.owner, m.SwIfIndex, obj.(df2.Named), d.KeyOf(obj), d.opts.Claims); err != nil {
		return err
	} else if skip {
		return df2.Release(d.opts.Claims, d.KeyOf(obj)) // interface gone or index reused: nothing of ours to remove
	}
	if err := d.set(ctx, interface_types.InterfaceIndex(m.SwIfIndex), false); err != nil {
		return err
	}
	return df2.Release(d.opts.Claims, d.KeyOf(obj))
}

// featureProbe answers feature_is_enabled on the device-input arc with VPP's error encoding
// undone (V23 a): vl_api_feature_is_enabled_t_handler stores vnet_feature_is_enabled's negative
// VNET_API_ERROR_* in a bool, so an unknown arc or feature and a sw_if_index beyond the arc's
// config vector ("certainly not enabled" in VPP's own comment) all read as enabled. A "true" for
// adl-input counts only when (1) the control query of the arc's end node (never enabled as a
// feature) is false for the same interface — the index is inside the vector — and (2) adl-input
// is false on local0 — VPP knows the feature. Otherwise the true was an error and ADL is not on.
type featureProbe struct {
	svc featureapi.RPCService
	// adlKnown caches check (2) for one Retrieve.
	adlKnown *bool
}

func (p *featureProbe) query(ctx context.Context, feature string, idx uint32) (bool, error) {
	rep, err := p.svc.FeatureIsEnabled(ctx, &featureapi.FeatureIsEnabled{ArcName: adlArc, FeatureName: feature, SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		return false, err
	}
	return rep.IsEnabled, nil
}

// adlEnabled reports whether adl-input is really enabled on idx.
func (p *featureProbe) adlEnabled(ctx context.Context, idx uint32) (bool, error) {
	on, err := p.query(ctx, adlFeature, idx)
	if err != nil || !on {
		return false, err
	}
	ctl, err := p.query(ctx, controlFeature, idx)
	if err != nil {
		return false, err
	}
	if ctl {
		return false, nil // idx beyond the arc's config vector: the query failed, nothing is enabled
	}
	if p.adlKnown == nil {
		onLocal0, err := p.query(ctx, adlFeature, local0)
		if err != nil {
			return false, err
		}
		known := !onLocal0
		p.adlKnown = &known
	}
	return *p.adlKnown, nil
}

// Retrieve reports ADL on every interface of this owner (tagged, or untagged and claimed)
// where adl-input is enabled on the device-input arc, confirmed by the featureProbe (V23 a).
func (d *InterfaceDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	probe := &featureProbe{svc: featureapi.NewServiceClient(d.client)}
	var out []scheduler.KV
	for _, idx := range ifs.Candidates() {
		name, _ := ifs.Name(idx)
		v := &Interface{Interface: name}
		if !ifs.OwnsObject(idx, d.KeyOf(v), d.opts.Claims) {
			continue
		}
		on, err := probe.adlEnabled(ctx, idx)
		if df2.InterfaceVanished(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("feature_is_enabled %s/%s %d: %w", adlArc, adlFeature, idx, err)
		}
		if on {
			out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: Meta{SwIfIndex: idx}})
		}
	}
	return out, nil
}

// ErrDefaultADL is returned for an allow-list with default_adl set: VPP 26.06's
// default-adl-allowlist node is a stub that drops the frame's buffers without freeing them.
var ErrDefaultADL = errors.New("default_adl: the default (non-IP) allow-list node of VPP 26.06 is a stub that leaks buffers; non-IP frames are always passed")

// AllowlistOption configures the adl.allowlist descriptor.
type AllowlistOption func(*allowlistOptions)

type allowlistOptions struct {
	claims df2.ClaimStore
	boot   dfkit.BootStore
}

// WithAllowlistClaims sets the claim store for allow-lists on untagged interfaces (the store
// adl.interface gets through df2.WithClaims).
func WithAllowlistClaims(s df2.ClaimStore) AllowlistOption {
	return func(o *allowlistOptions) { o.claims = s }
}

// WithBootStore sets the persisted D-076 applied-once store (the product agent passes
// subsystems.Wiring.BootStore; default: in memory, tests only).
func WithBootStore(s dfkit.BootStore) AllowlistOption {
	return func(o *allowlistOptions) { o.boot = s }
}

// AllowlistDescriptor binds an interface's ADL check to a FIB table of allowed prefixes
// (adl_allowlist_enable_disable), with the call sequences of the package doc.
type AllowlistDescriptor struct {
	client vpp.Client
	owner  string
	o      allowlistOptions
	mu     sync.Mutex // serialises the two-call sequences with their record
}

// NewAllowlist returns the descriptor for the given owner.
func NewAllowlist(c vpp.Client, owner string, opts ...AllowlistOption) *AllowlistDescriptor {
	o := allowlistOptions{}
	for _, f := range opts {
		f(&o)
	}
	if o.claims == nil {
		o.claims = df2.BuildOptions().Claims
	}
	if o.boot == nil {
		o.boot = dfkit.NewMemoryBootStore()
	}
	return &AllowlistDescriptor{client: c, owner: owner, o: o}
}

// Name implements scheduler.Descriptor.
func (*AllowlistDescriptor) Name() string { return AllowlistName }

// KeyOf implements scheduler.Descriptor.
func (*AllowlistDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(AllowlistName, obj.(*Allowlist).GetInterface())
}

// Dependencies implements scheduler.Descriptor: the interface and the allow-list table. (The
// adl.interface ↔ adl.allowlist ordering is expressed by adl.interface, see its Dependencies.)
func (*AllowlistDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	a := obj.(*Allowlist)
	return append([]scheduler.Dependency{df2.InterfaceDep(a.GetInterface())}, df2.VRFDeps(a.GetFibId())...)
}

// applied is the value of the applied-once record: what this owner configured on which index.
type applied struct {
	SwIfIndex uint32 `json:"sw_if_index"`
	FibID     uint32 `json:"fib_id"`
	IP4       bool   `json:"ip4"`
	IP6       bool   `json:"ip6"`
}

func (a applied) encode() string {
	raw, err := json.Marshal(a)
	if err != nil {
		panic(fmt.Sprintf("adl: encode %+v: %v", a, err)) // a plain struct: cannot fail
	}
	return string(raw)
}

func decodeApplied(s string) (applied, bool) {
	var a applied
	return a, json.Unmarshal([]byte(s), &a) == nil
}

// call sends one adl_allowlist_enable_disable with the three family flags.
func (d *AllowlistDescriptor) call(ctx context.Context, idx, fib uint32, ip4, ip6, def bool) error {
	req := &adlapi.AdlAllowlistEnableDisable{SwIfIndex: interface_types.InterfaceIndex(idx), FibID: fib, IP4: ip4, IP6: ip6, DefaultAdl: def}
	if _, err := adlapi.NewServiceClient(d.client).AdlAllowlistEnableDisable(ctx, req); err != nil {
		return fmt.Errorf("adl_allowlist_enable_disable(ip4=%v ip6=%v default=%v): %w", ip4, ip6, def, err)
	}
	return nil
}

// add is the add sequence: (1,1,1) then (ip4, ip6, 0). A failure of the second call undoes the
// first with (0,0,0), which removes exactly what the first added.
func (d *AllowlistDescriptor) add(ctx context.Context, a applied) error {
	if err := d.call(ctx, a.SwIfIndex, a.FibID, true, true, true); err != nil {
		return err
	}
	if err := d.call(ctx, a.SwIfIndex, a.FibID, a.IP4, a.IP6, false); err != nil {
		if uerr := d.call(ctx, a.SwIfIndex, a.FibID, false, false, false); uerr != nil {
			return errors.Join(err, fmt.Errorf("undo: %w", uerr))
		}
		return err
	}
	return nil
}

// remove is the remove sequence for what add configured: (!ip4, !ip6, 1) then (0,0,0).
func (d *AllowlistDescriptor) remove(ctx context.Context, a applied) error {
	if err := d.call(ctx, a.SwIfIndex, a.FibID, !a.IP4, !a.IP6, true); err != nil {
		return err
	}
	return d.call(ctx, a.SwIfIndex, a.FibID, false, false, false)
}

// record returns this owner's applied-once record for key when it belongs to the running VPP
// instance, and the current boot identity.
func (d *AllowlistDescriptor) record(ctx context.Context, key scheduler.Key) (applied, bool, bootid.Identity, error) {
	id, err := dfkit.IdentitySource(ctx, d.client)
	if err != nil {
		return applied{}, false, id, err
	}
	r, ok := d.o.boot.Get(string(key))
	if !ok || !bootid.Matches(r.Identity, id) {
		return applied{}, false, id, nil
	}
	a, ok := decodeApplied(r.Value)
	return a, ok, id, nil
}

// Create implements scheduler.Descriptor: the add sequence, once per VPP instance and
// interface index (a resync of the same value finds the record and sends nothing).
func (d *AllowlistDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	a := obj.(*Allowlist)
	if !a.GetIp4() && !a.GetIp6() {
		return nil, fmt.Errorf("%s: at least one of ip4/ip6 must be set", AllowlistName)
	}
	if a.GetDefaultAdl() {
		return nil, fmt.Errorf("%s: %w", AllowlistName, ErrDefaultADL)
	}
	idx, untagged, err := resolve(ctx, d.client, d.owner, a.GetInterface())
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	key := d.KeyOf(obj)
	want := applied{SwIfIndex: uint32(idx), FibID: a.GetFibId(), IP4: a.GetIp4(), IP6: a.GetIp6()}
	prev, ok, id, err := d.record(ctx, key)
	if err != nil {
		return nil, err
	}
	switch {
	case ok && prev == want:
		// applied on this VPP instance and index already (D-076): nothing to send
	case ok && prev.SwIfIndex == want.SwIfIndex:
		// another value applied earlier on this index (desired changed while the agent was down)
		if err := d.remove(ctx, prev); err != nil {
			return nil, err
		}
		if err := d.add(ctx, want); err != nil {
			return nil, err
		}
	default:
		if err := d.add(ctx, want); err != nil {
			return nil, err
		}
	}
	if err := d.o.boot.Put(dfkit.BootRecord{Key: string(key), Identity: id.String(), Value: want.encode()}); err != nil {
		return nil, fmt.Errorf("%s: applied-once record: %w", AllowlistName, err)
	}
	if err := df2.Claim(d.o.claims, untagged, key); err != nil {
		return nil, err
	}
	return Meta{SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor: every change is a recreate (remove + add), which the
// scheduler wraps with adl.interface's recreate, so adl-input is off meanwhile.
func (*AllowlistDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete runs the remove sequence for what the record says this owner applied on the running
// VPP instance; without such a record (VPP restarted, or the interface index changed) there is
// nothing of ours in VPP and only the record and claim are dropped.
func (d *AllowlistDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(Meta)
	if !ok {
		return fmt.Errorf("%s: %w %T", AllowlistName, df2.ErrBadMeta, meta)
	}
	key := d.KeyOf(obj)
	d.mu.Lock()
	defer d.mu.Unlock()
	forget := func() error {
		if err := d.o.boot.Delete(string(key)); err != nil {
			return err
		}
		return df2.Release(d.o.claims, key)
	}
	if skip, err := df2.SkipDelete(ctx, d.client, d.owner, m.SwIfIndex, obj.(df2.Named), key, d.o.claims); err != nil {
		return err
	} else if skip {
		return forget() // interface gone or index reused: nothing of ours to remove
	}
	prev, ok, _, err := d.record(ctx, key)
	if err != nil {
		return err
	}
	if ok && prev.SwIfIndex == m.SwIfIndex {
		if err := d.remove(ctx, prev); err != nil {
			return err
		}
	}
	return forget()
}

// Retrieve is unsupported: the adl plugin has no dump and the allow-list table is not
// visible anywhere else (write-only, D-063; not in the default Register).
func (*AllowlistDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", AllowlistName, df2.ErrRetrieveUnsupported)
}

// Register registers the adl descriptor VPP can read back (adl.interface) with r.
// adl.allowlist is write-only (no readback in VPP 26.06) and only registered by
// RegisterWriteOnly, for a reconciler that implements D-063.
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df2.Option) {
	r.Register(NewInterface(c, owner, opts...))
}

// RegisterWriteOnly registers adl.allowlist. Its Retrieve returns df2.ErrRetrieveUnsupported:
// only a reconciler implementing D-063 (re-apply on every resync, never delete on absence,
// skip verification) may register it; removing it from the desired state does not disable
// it in VPP after an agent restart. The product agent passes WithBootStore (persisted, D-076)
// and WithAllowlistClaims (the adl.interface claim store).
func RegisterWriteOnly(r scheduler.Registry, c vpp.Client, owner string, opts ...AllowlistOption) {
	r.Register(NewAllowlist(c, owner, opts...))
}
