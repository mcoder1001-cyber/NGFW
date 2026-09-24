package mactime

import (
	"context"
	"fmt"
	"strconv"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/interface_types"
	mactimeapi "ngfw/agent/binapi/mactime"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// EnableDescriptor implements mactime.enable/<interface> (mactime_enable_disable).
//
// VPP stacks the feature on every enable (vnet_config_add_feature has no duplicate check), so an
// enable is applied once per interface and VPP boot (D-076): the applied-once record in the
// owner's persisted BootStore holds "<sw_if_index>/<logical name>" under the D-080 boot identity.
// The state is read back with feature_is_enabled("device-input", "mactime", sw_if_index), which
// answers true for an index the arc's vector does not reach yet (vnet_feature_is_enabled returns
// VNET_API_ERROR_INVALID_SW_IF_INDEX, cast to bool): the readback therefore counts only together
// with this boot's record, and a Create that finds the feature "on" without a record disables once
// before it enables (a no-op for such an index, and it removes a stale enable of a lost record).
type EnableDescriptor struct {
	client vpp.Client
	owner  string
	store  dfkit.BootStore
}

var _ scheduler.Descriptor = (*EnableDescriptor)(nil)

// NewEnable returns the mactime.enable descriptor with the applied-once store (P08 Wiring.BootStore).
func NewEnable(c vpp.Client, owner string, store dfkit.BootStore) *EnableDescriptor {
	if store == nil {
		store = dfkit.NewMemoryBootStore()
	}
	return &EnableDescriptor{client: c, owner: owner, store: store}
}

// EnableMeta is the runtime handle.
type EnableMeta struct {
	SwIfIndex uint32
	Name      string
}

// Name implements scheduler.Descriptor.
func (*EnableDescriptor) Name() string { return EnableName }

// KeyOf implements scheduler.Descriptor.
func (*EnableDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	e, err := decodeEnable(obj)
	if err != nil {
		return scheduler.Join(EnableName, "invalid")
	}
	return EnableKey(e.Interface)
}

// Dependencies implements scheduler.Descriptor: the interface alias (D-065) — the filter is
// disabled before the interface goes away (V19 family).
func (*EnableDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	e, err := decodeEnable(obj)
	if err != nil {
		return nil
	}
	return []scheduler.Dependency{{Key: dfkit.DefaultInterfaceKey(e.Interface)}}
}

func recordValue(idx uint32, name string) string {
	return strconv.FormatUint(uint64(idx), 10) + "/" + name
}

// IsEnabled reads feature_is_enabled for the mactime node on idx's device-input arc (see the type
// comment for the out-of-range caveat).
func IsEnabled(ctx context.Context, c vpp.Client, idx uint32) (bool, error) {
	r, err := feature.NewServiceClient(c).FeatureIsEnabled(ctx, &feature.FeatureIsEnabled{
		ArcName: arcDeviceInput, FeatureName: featureMactime, SwIfIndex: interface_types.InterfaceIndex(idx),
	})
	if err != nil {
		return false, fmt.Errorf("feature_is_enabled %s/%s %d: %w", arcDeviceInput, featureMactime, idx, err)
	}
	return r.IsEnabled, nil
}

func (d *EnableDescriptor) set(ctx context.Context, idx uint32, on bool) error {
	_, err := mactimeapi.NewServiceClient(d.client).MactimeEnableDisable(ctx, &mactimeapi.MactimeEnableDisable{
		EnableDisable: on, SwIfIndex: interface_types.InterfaceIndex(idx),
	})
	if err != nil {
		return dfkit.PluginError("mactime", fmt.Errorf("mactime_enable_disable (%v) %d: %w", on, idx, err))
	}
	return nil
}

// Create implements scheduler.Descriptor (idempotent: re-applied on every resync of a lost object).
func (d *EnableDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	e, err := decodeEnable(obj)
	if err != nil {
		return nil, err
	}
	tg, err := dfkit.ResolveTarget(ctx, d.client, e.Interface, d.owner, EnableName)
	if err != nil {
		return nil, err
	}
	key := EnableKey(e.Interface)
	value := recordValue(tg.Index, e.Interface)
	applied, id, err := dfkit.AppliedThisBoot(ctx, d.client, d.store, key, value)
	if err != nil {
		return nil, err
	}
	on, err := IsEnabled(ctx, d.client, tg.Index)
	if err != nil {
		return nil, err
	}
	meta := EnableMeta{SwIfIndex: tg.Index, Name: e.Interface}
	if applied && on {
		return meta, tg.Claim()
	}
	if on { // no record of this boot: normalise to "off" first (see the type comment)
		if err := d.set(ctx, tg.Index, false); err != nil {
			return nil, err
		}
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
func (*EnableDescriptor) Update(_ context.Context, _, _ proto.Message, meta any) (any, error) {
	return meta, nil
}

// Delete implements scheduler.Descriptor: disable once on the interface that still has the name
// and index of the record (D-071/D-074: indexes are reused), then forget the record.
func (d *EnableDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	e, err := decodeEnable(obj)
	if err != nil {
		return err
	}
	key := EnableKey(e.Interface)
	tg, ok, err := dfkit.ResolveForDelete(ctx, d.client, e.Interface, d.owner, EnableName)
	if err != nil {
		return err
	}
	if m, isMeta := meta.(EnableMeta); ok && isMeta && m.SwIfIndex == tg.Index {
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

// Retrieve implements scheduler.Descriptor: an interface this owner can report (tagged, or
// untagged with this holder's claim) whose applied-once record matches this boot, index and name
// and whose mactime feature is on.
func (d *EnableDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := dfkit.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	var id bootid.Identity
	var out []scheduler.KV
	for _, idx := range t.T.Indexes() {
		if i := t.ByIndex[idx]; i.SupIndex != idx {
			continue // sub-interfaces: mactime runs on hardware interfaces only
		}
		name, ok := t.Reportable(idx, EnableName)
		if !ok {
			continue
		}
		key := EnableKey(name)
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
			out = append(out, scheduler.KV{Key: key, Value: Enable{Interface: name}.Proto(), Meta: EnableMeta{SwIfIndex: idx, Name: name}})
		}
	}
	return dfkit.Dedupe(out), nil
}
