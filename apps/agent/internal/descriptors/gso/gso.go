// Package gso holds the descriptor of VPP's software generic segmentation offload on an interface
// (F-loopback-bvi-gso-lldp-span, WBS D1.8), built on descriptors/dfkit (D-077):
//
//	gso.interface/<interface>  feature_gso_enable_disable: the gso-ip4 / gso-ip6 nodes on the
//	                           ip4-output / ip6-output arcs and the gso-l2-* nodes on the L2 output
//	                           arcs (vnet/gso/gso.c vnet_sw_interface_gso_enable_disable)
//
// VPP 26.06 has no dump or getter of the GSO setting, and vnet_feature_enable_disable stacks a
// feature on every enable (vnet_config_add_feature has no duplicate check; a disable removes one
// instance). So an enable is applied once per interface and VPP boot (D-076): the applied-once
// record in the owner's persisted BootStore holds "<sw_if_index>/<logical name>" under the D-080
// boot identity. The state is read back with feature_is_enabled("ip4-output", "gso-ip4"), which
// answers true for an index the arc's vector never reached (vnet_feature_is_enabled returns
// VNET_API_ERROR_INVALID_SW_IF_INDEX, cast to bool) — so the read-back counts only together with
// this boot's record (the mactime.enable pattern, F-bridge-l2), and Retrieve reports an interface
// only when both agree. Messages come only from apps/agent/binapi/{gso,feature}.
//
// See docs/agent/descriptors/gso.md.
package gso

import (
	"context"
	"fmt"
	"strconv"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/feature"
	gsoapi "ngfw/agent/binapi/gso"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// Name is the descriptor name; keys are "gso.interface/<logical interface name>".
const Name = "gso.interface"

// VPP facts (vnet/gso/gso.c): the read-back arc and node; maxNormalise bounds the disables that
// clear a stale stacked enable before this owner's own one.
const (
	maxNormalise = 16

	arcIP4Output = "ip4-output"
	featureGSO4  = "gso-ip4"
)

// Interface is the desired value of gso.interface/<name> (dfkit structpb stand-in, D-055): GSO is
// on while the object exists.
type Interface struct {
	Interface string `json:"interface"`
}

// Proto encodes i.
func (i Interface) Proto() proto.Message { return dfkit.Encode(i) }

func decode(obj proto.Message) (Interface, error) {
	var i Interface
	if err := dfkit.Decode(obj, &i); err != nil {
		return i, err
	}
	if i.Interface == "" {
		return i, dfkit.Specf("gso.interface needs an interface")
	}
	return i, nil
}

// Key is "gso.interface/<name>".
func Key(name string) scheduler.Key { return scheduler.Join(Name, name) }

// Meta is the runtime handle.
type Meta struct {
	SwIfIndex uint32
	Name      string
}

// Descriptor implements gso.interface.
type Descriptor struct {
	client vpp.Client
	owner  string
	store  dfkit.BootStore
}

var _ scheduler.Descriptor = (*Descriptor)(nil)

// New returns the descriptor with the owner's applied-once store (subsystems.Wiring.BootStore);
// nil = in memory (tests only).
func New(c vpp.Client, owner string, store dfkit.BootStore) *Descriptor {
	if store == nil {
		store = dfkit.NewMemoryBootStore()
	}
	return &Descriptor{client: c, owner: owner, store: store}
}

// Register registers gso.interface with r.
func Register(r scheduler.Registry, c vpp.Client, owner string, store dfkit.BootStore) {
	r.Register(New(c, owner, store))
}

// Name implements scheduler.Descriptor.
func (*Descriptor) Name() string { return Name }

// KeyOf implements scheduler.Descriptor.
func (*Descriptor) KeyOf(obj proto.Message) scheduler.Key {
	i, err := decode(obj)
	if err != nil {
		return scheduler.Join(Name, "invalid")
	}
	return Key(i.Interface)
}

// Dependencies implements scheduler.Descriptor: the interface alias (D-065), so GSO is disabled
// before the interface goes away (V19/V21: feature state survives an interface delete).
func (*Descriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	i, err := decode(obj)
	if err != nil {
		return nil
	}
	return []scheduler.Dependency{{Key: dfkit.DefaultInterfaceKey(i.Interface)}}
}

func recordValue(idx uint32, name string) string {
	return strconv.FormatUint(uint64(idx), 10) + "/" + name
}

// IsEnabled reads feature_is_enabled("ip4-output", "gso-ip4") for idx (see the package comment for
// the out-of-range caveat).
func IsEnabled(ctx context.Context, c vpp.Client, idx uint32) (bool, error) {
	r, err := feature.NewServiceClient(c).FeatureIsEnabled(ctx, &feature.FeatureIsEnabled{
		ArcName: arcIP4Output, FeatureName: featureGSO4, SwIfIndex: interface_types.InterfaceIndex(idx),
	})
	if err != nil {
		return false, fmt.Errorf("feature_is_enabled %s/%s %d: %w", arcIP4Output, featureGSO4, idx, err)
	}
	return r.IsEnabled, nil
}

func (d *Descriptor) set(ctx context.Context, idx uint32, on bool) error {
	_, err := gsoapi.NewServiceClient(d.client).FeatureGsoEnableDisable(ctx, &gsoapi.FeatureGsoEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx), EnableDisable: on,
	})
	if err != nil {
		return dfkit.PluginError("gso", fmt.Errorf("feature_gso_enable_disable (%v) %d: %w", on, idx, err))
	}
	return nil
}

// Create implements scheduler.Descriptor (idempotent: re-applied on every resync of a lost object).
func (d *Descriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	i, err := decode(obj)
	if err != nil {
		return nil, err
	}
	tg, err := dfkit.ResolveTarget(ctx, d.client, i.Interface, d.owner, Name)
	if err != nil {
		return nil, err
	}
	key := Key(i.Interface)
	value := recordValue(tg.Index, i.Interface)
	applied, id, err := dfkit.AppliedThisBoot(ctx, d.client, d.store, key, value)
	if err != nil {
		return nil, err
	}
	on, err := IsEnabled(ctx, d.client, tg.Index)
	if err != nil {
		return nil, err
	}
	meta := Meta{SwIfIndex: tg.Index, Name: i.Interface}
	if applied && on {
		return meta, tg.Claim()
	}
	// no record of this boot: normalise to "off" first — one disable per stacked instance (a stale or
	// inherited enable, V21), bounded; a no-op for an index the arc never reached
	for n := 0; on && n < maxNormalise; n++ {
		if err := d.set(ctx, tg.Index, false); err != nil {
			return nil, err
		}
		if on, err = IsEnabled(ctx, d.client, tg.Index); err != nil {
			return nil, err
		}
	}
	if on {
		return nil, fmt.Errorf("%s: GSO on %s (%d) is still enabled after %d disables; not stacking another enable", Name, i.Interface, tg.Index, maxNormalise)
	}
	if err := d.set(ctx, tg.Index, true); err != nil {
		return nil, err
	}
	if err := d.store.Put(dfkit.BootRecord{Key: string(key), Identity: id, Value: value}); err != nil {
		return nil, fmt.Errorf("%s: record: %w", key, err)
	}
	return meta, tg.Claim()
}

// Update implements scheduler.Descriptor: the value is only the interface (= the key).
func (*Descriptor) Update(_ context.Context, _, _ proto.Message, meta any) (any, error) {
	return meta, nil
}

// Delete implements scheduler.Descriptor: disable once on the interface that still has the name
// and index of the handle (D-071/D-074: indexes are reused), then forget the record.
func (d *Descriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	i, err := decode(obj)
	if err != nil {
		return err
	}
	key := Key(i.Interface)
	tg, ok, err := dfkit.ResolveForDelete(ctx, d.client, i.Interface, d.owner, Name)
	if err != nil {
		return err
	}
	if m, isMeta := meta.(Meta); ok && isMeta && m.SwIfIndex == tg.Index {
		on, err := IsEnabled(ctx, d.client, tg.Index)
		if err != nil {
			return err
		}
		if on {
			if err := d.set(ctx, tg.Index, false); err != nil {
				return err
			}
		}
		if err := tg.Release(); err != nil {
			return err
		}
	}
	return d.store.Delete(string(key))
}

// Retrieve implements scheduler.Descriptor: an interface this owner can report (tagged, or untagged
// with this holder's claim) whose applied-once record matches this boot, index and name and whose
// gso-ip4 feature is on.
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := dfkit.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	var id bootid.Identity
	var out []scheduler.KV
	for _, idx := range t.T.Indexes() {
		name, ok := t.Reportable(idx, Name)
		if !ok {
			continue
		}
		key := Key(name)
		rec, ok := d.store.Get(string(key))
		if !ok || rec.Value != recordValue(idx, name) {
			continue
		}
		if id.IsZero() {
			if id, err = dfkit.IdentitySource(ctx, d.client); err != nil {
				return nil, err
			}
		}
		if !bootid.Matches(rec.Identity, id) {
			continue
		}
		on, err := IsEnabled(ctx, d.client, idx)
		if err != nil {
			return nil, err
		}
		if on {
			out = append(out, scheduler.KV{Key: key, Value: Interface{Interface: name}.Proto(), Meta: Meta{SwIfIndex: idx, Name: name}})
		}
	}
	return dfkit.Dedupe(out), nil
}
