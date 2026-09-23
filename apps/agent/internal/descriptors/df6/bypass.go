package df6

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// BypassSpec describes a per-interface feature toggle (sw_interface_set_vxlan_bypass,
// sw_interface_set_vxlan_gpe_bypass, sw_interface_set_gtpu_bypass, and — through ToggleSpec —
// l2tpv3_interface_enable_disable): enable/disable for IPv4 and IPv6 on one interface. VPP has
// no dump for these features, so the descriptors are write-only (Retrieve →
// ErrRetrieveUnsupported) and P05 re-applies them on every resync (D-063).
//
// Idempotency (D-076, review H3): most of these handlers call vnet_feature_enable_disable
// directly, which does not deduplicate — a second enable inserts the node twice. So the
// descriptor records, per (interface, family), a claim "<name>@vpp-<boot>" once the enable
// succeeded on the running VPP (BootID) and skips the re-add while that VPP instance is
// unchanged; after a VPP restart the feature is gone and is enabled exactly once again.
//
// Ownership (D-071, review H4/M1): the interface is resolved by logical name through DF-1's
// resolver (foreign-tagged interfaces are refused); an untagged interface is claimed for the
// descriptor. Delete re-resolves the interface, checks it is still the one in Meta (when Meta
// is known) and still ours, and only disables a family whose enable this agent recorded.
type BypassSpec[T proto.Message] struct {
	// Name is the descriptor name ("vxlan.bypass").
	Name string
	// Plugin is the VPP plugin for ErrPluginNotLoaded.
	Plugin string
	// Fields returns the interface name and the desired IPv4 / IPv6 state of obj.
	Fields func(obj T) (iface string, ipv4, ipv6 bool)
	// Set sends the enable/disable message for one family.
	Set func(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, ipv6, enable bool) error
	// Families are the family names used in claim ids (default "ip4", "ip6").
	Families [2]string
	// Probe (optional) reads whether the feature of a family is enabled on idx
	// (feature_is_enabled, see FeatureProbe). With a probe the actual VPP state decides —
	// correct across interface re-creation with a reused sw_if_index and across VPP restarts;
	// without one the per-boot records keyed by name + sw_if_index do (D-076/D-080).
	Probe func(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, ipv6 bool) (bool, error)
	// ResetBeforeEnable: the plugin guards enable/disable with its own per-sw_if_index bitmap
	// that VPP does not clear when the interface is deleted (vxlan: bm_ip4/6_bypass_enabled_by_sw_if).
	// A recreated interface that reuses the index then reads as "already enabled" and the
	// enable is ignored; when the probe says the feature is off, a disable is sent first to
	// clear the stale bit (a no-op when the bit is clear). Only for bitmap-guarded plugins.
	ResetBeforeEnable bool
}

// FeatureProbe returns a Probe built on feature_is_enabled for the given ip4 / ip6 arc and
// feature node names ("" = family not supported).
func FeatureProbe(arc4, node4, arc6, node6 string) func(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, ipv6 bool) (bool, error) {
	return func(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, ipv6 bool) (bool, error) {
		arc, node := arc4, node4
		if ipv6 {
			arc, node = arc6, node6
		}
		if node == "" {
			return false, nil
		}
		rep, err := feature.NewServiceClient(c).FeatureIsEnabled(ctx, &feature.FeatureIsEnabled{ArcName: arc, FeatureName: node, SwIfIndex: idx})
		if err != nil {
			return false, fmt.Errorf("feature_is_enabled %s/%s: %w", arc, node, err)
		}
		return rep.IsEnabled, nil
	}
}

// BypassDescriptor is the scheduler.Descriptor built from a BypassSpec.
type BypassDescriptor[T proto.Message] struct {
	spec   BypassSpec[T]
	client vpp.Client
	owner  string
	claims ClaimStore
}

// NewBypassDescriptor returns the descriptor for spec.
func NewBypassDescriptor[T proto.Message](spec BypassSpec[T], c vpp.Client, owner string, opts ...Option) *BypassDescriptor[T] {
	if spec.Families == [2]string{} {
		spec.Families = [2]string{"ip4", "ip6"}
	}
	return &BypassDescriptor[T]{spec: spec, client: c, owner: owner, claims: BuildOptions(owner, opts).Claims}
}

func (d *BypassDescriptor[T]) cast(obj proto.Message) (T, error) {
	t, ok := obj.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("%s: %w: %T", d.spec.Name, ErrBadValue, obj)
	}
	return t, nil
}

// Name implements scheduler.Descriptor.
func (d *BypassDescriptor[T]) Name() string { return d.spec.Name }

// KeyOf implements scheduler.Descriptor: "<name>/<interface>".
func (d *BypassDescriptor[T]) KeyOf(obj proto.Message) scheduler.Key {
	t, err := d.cast(obj)
	if err != nil {
		return scheduler.Join(d.spec.Name, "invalid")
	}
	iface, _, _ := d.spec.Fields(t)
	if iface == "" {
		return scheduler.Join(d.spec.Name, "invalid")
	}
	return scheduler.Join(d.spec.Name, iface)
}

// Dependencies implements scheduler.Descriptor: the interface (alias key interface/<name>).
func (d *BypassDescriptor[T]) Dependencies(obj proto.Message) []scheduler.Dependency {
	t, err := d.cast(obj)
	if err != nil {
		return nil
	}
	iface, _, _ := d.spec.Fields(t)
	return InterfaceDeps(iface)
}

// claimID keys the per-boot record on the logical name AND the sw_if_index (D-080, review
// N1): an interface recreated on the same VPP boot gets a new index and a fresh record.
func (d *BypassDescriptor[T]) claimID(iface string, idx interface_types.InterfaceIndex, ipv6 bool) string {
	fam := d.spec.Families[0]
	if ipv6 {
		fam = d.spec.Families[1]
	}
	return fmt.Sprintf("%s@%d/%s", iface, idx, fam)
}

// ensure makes family ipv6 of the interface enabled (want) or disabled (!want), sending a
// message only when the recorded state for this VPP boot differs.
func (d *BypassDescriptor[T]) ensure(ctx context.Context, iface string, idx interface_types.InterfaceIndex, boot string, ipv6, want bool) error {
	id := d.claimID(iface, idx, ipv6)
	holder := BootHolder(d.spec.Name, boot)
	on := d.claims.Claimed(id, holder)
	if d.spec.Probe != nil {
		var err error
		if on, err = d.spec.Probe(ctx, d.client, idx, ipv6); err != nil {
			return PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
		}
	}
	if on == want {
		if want {
			return d.claims.Claim(id, holder)
		}
		return d.claims.Release(id, holder)
	}
	if want && d.spec.Probe != nil && d.spec.ResetBeforeEnable {
		if err := d.spec.Set(ctx, d.client, idx, ipv6, false); err != nil {
			return PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
		}
	}
	if err := d.spec.Set(ctx, d.client, idx, ipv6, want); err != nil {
		return PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	if want {
		return d.claims.Claim(id, holder)
	}
	return d.claims.Release(id, holder)
}

// resolve finds the interface by logical name and checks it is ours (tagged, or untagged and
// claimable); meta, when known, must still be the same sw_if_index.
func (d *BypassDescriptor[T]) resolve(ctx context.Context, iface string, meta any, claim bool) (interface_types.InterfaceIndex, error) {
	ifs, err := DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return 0, err
	}
	idx, err := ifs.Index(iface)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", d.spec.Name, err)
	}
	if m, ok := meta.(IfMeta); ok && m.SwIfIndex != uint32(idx) {
		return 0, fmt.Errorf("%s: %w: %s is now sw_if_index %d, not %d", d.spec.Name, ErrNotOurs, iface, idx, m.SwIfIndex)
	}
	if claim {
		if err := ifs.ClaimIfUntagged(uint32(idx), d.spec.Name); err != nil {
			return 0, err
		}
	} else if !ifs.Owns(uint32(idx), d.spec.Name) {
		return 0, fmt.Errorf("%s: %w: interface %s", d.spec.Name, ErrNotOurs, iface)
	}
	return idx, nil
}

func (d *BypassDescriptor[T]) apply(ctx context.Context, iface string, idx interface_types.InterfaceIndex, want4, want6 bool) error {
	boot, err := BootID(ctx, d.client)
	if err != nil {
		return err
	}
	if err := d.ensure(ctx, iface, idx, boot, false, want4); err != nil {
		return err
	}
	return d.ensure(ctx, iface, idx, boot, true, want6)
}

// Create implements scheduler.Descriptor (idempotent across resyncs, see BypassSpec).
func (d *BypassDescriptor[T]) Create(ctx context.Context, obj proto.Message) (any, error) {
	t, err := d.cast(obj)
	if err != nil {
		return nil, err
	}
	iface, v4, v6 := d.spec.Fields(t)
	if iface == "" {
		return nil, fmt.Errorf("%s: %w: interface is mandatory", d.spec.Name, ErrBadValue)
	}
	if !v4 && !v6 {
		return nil, fmt.Errorf("%s: %w: at least one of ipv4/ipv6 must be set", d.spec.Name, ErrBadValue)
	}
	idx, err := d.resolve(ctx, iface, nil, true)
	if err != nil {
		return nil, err
	}
	if err := d.apply(ctx, iface, idx, v4, v6); err != nil {
		return nil, err
	}
	return IfMeta{SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor: toggles the families that changed in place.
func (d *BypassDescriptor[T]) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, err := d.cast(oldObj)
	if err != nil {
		return nil, err
	}
	n, err := d.cast(newObj)
	if err != nil {
		return nil, err
	}
	oi, _, _ := d.spec.Fields(o)
	ni, n4, n6 := d.spec.Fields(n)
	if oi != ni {
		return nil, scheduler.ErrRecreate
	}
	if !n4 && !n6 {
		return nil, fmt.Errorf("%s: %w: at least one of ipv4/ipv6 must be set", d.spec.Name, ErrBadValue)
	}
	idx, err := d.resolve(ctx, ni, meta, false)
	if err != nil {
		return nil, err
	}
	if err := d.apply(ctx, ni, idx, n4, n6); err != nil {
		return nil, err
	}
	return IfMeta{SwIfIndex: uint32(idx)}, nil
}

// Delete implements scheduler.Descriptor: disables the families this agent enabled on the
// running VPP, after re-verifying the interface. A missing interface means nothing is left.
func (d *BypassDescriptor[T]) Delete(ctx context.Context, obj proto.Message, meta any) error {
	t, err := d.cast(obj)
	if err != nil {
		return err
	}
	iface, _, _ := d.spec.Fields(t)
	idx, err := d.resolve(ctx, iface, meta, false)
	if err != nil {
		if IsNoSuchInterface(err) {
			return nil
		}
		return err
	}
	return d.apply(ctx, iface, idx, false, false)
}

// Retrieve implements scheduler.Descriptor: VPP has no dump for these features.
func (d *BypassDescriptor[T]) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", d.spec.Name, ErrRetrieveUnsupported)
}
