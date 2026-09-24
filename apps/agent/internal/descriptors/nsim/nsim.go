// Package nsim holds the descriptors of VPP's network delay simulator (F-loopback-bvi-gso-lldp-span,
// WBS D1.9 — a lab tool, "not a product feature"), built on descriptors/dfkit (D-077):
//
//	nsim.config/global         nsim_configure2: delay, bandwidth, average packet size, loss (the
//	                           VPP-wide model and its scheduler wheels)
//	nsim.cross-connect/global  nsim_cross_connect_enable_disable: the one pair of interfaces VPP
//	                           joins through the simulator (nsim_main_t holds a single pair)
//	nsim.output/<interface>    nsim_output_feature_enable_disable: the simulator on one interface's
//	                           output (interface-output arc)
//
// All three are VPP-global or depend on the VPP-global model, so only the globals owner registers
// them (D-071, RegisterGlobals). VPP 26.06 has no dump or getter for any of them: they are
// write-only (D-063) and Retrieve returns ErrRetrieveUnsupported. Their VPP operations are not
// idempotent — nsim_configure2 frees and reallocates the wheels (frames in flight are lost), and
// both enables stack a feature on every call — so each Create is applied once per VPP boot and
// value (D-076): the applied-once record in the owner's persisted BootStore holds the applied
// parameters or "<sw_if_index>/<logical name>" pairs under the D-080 boot identity. VPP cannot
// unconfigure the model: deleting nsim.config/global only forgets the record (the model stays,
// inert without a cross-connect or output interface). Messages only from apps/agent/binapi/nsim.
//
// See docs/agent/descriptors/nsim.md.
package nsim

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	nsimapi "ngfw/agent/binapi/nsim"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	ConfigName       = "nsim.config"
	CrossConnectName = "nsim.cross-connect"
	OutputName       = "nsim.output"
)

// GlobalID is the id of the two singletons.
const GlobalID = "global"

// VPP bounds (plugins/nsim/nsim.c nsim_configure).
const (
	PacketSizeMin = 64
	PacketSizeMax = 9000
)

// ConfigKey is "nsim.config/global".
func ConfigKey() scheduler.Key { return scheduler.Join(ConfigName, GlobalID) }

// CrossConnectKey is "nsim.cross-connect/global".
func CrossConnectKey() scheduler.Key { return scheduler.Join(CrossConnectName, GlobalID) }

// OutputKey is "nsim.output/<interface>".
func OutputKey(name string) scheduler.Key { return scheduler.Join(OutputName, name) }

// Config is the desired value of nsim.config/global, in VPP's units.
type Config struct {
	DelayUsec      uint32  `json:"delay_usec"`
	BandwidthBps   float64 `json:"bandwidth_bps"`
	PacketSize     uint32  `json:"packet_size"`
	PacketsPerDrop uint32  `json:"packets_per_drop"`
}

// Validate checks c against VPP's bounds.
func (c Config) Validate() error {
	switch {
	case c.DelayUsec == 0:
		return dfkit.Specf("nsim delay must be > 0")
	case c.BandwidthBps < 1 || c.BandwidthBps > math.MaxUint64/2:
		return dfkit.Specf("nsim bandwidth %v bit/s out of range", c.BandwidthBps)
	case c.PacketSize < PacketSizeMin || c.PacketSize > PacketSizeMax:
		return dfkit.Specf("nsim packet size %d outside %d–%d", c.PacketSize, PacketSizeMin, PacketSizeMax)
	}
	return nil
}

// Proto encodes c.
func (c Config) Proto() proto.Message { return dfkit.Encode(c) }

// record is the applied-once value of the model.
func (c Config) record() string {
	return fmt.Sprintf("%d/%.0f/%d/%d", c.DelayUsec, c.BandwidthBps, c.PacketSize, c.PacketsPerDrop)
}

// CrossConnect is the desired value of nsim.cross-connect/global: two logical interface names.
type CrossConnect struct {
	A string `json:"a"`
	B string `json:"b"`
}

// Validate checks x.
func (x CrossConnect) Validate() error {
	if x.A == "" || x.B == "" || x.A == x.B {
		return dfkit.Specf("nsim cross-connect needs two different interfaces")
	}
	return nil
}

// Proto encodes x.
func (x CrossConnect) Proto() proto.Message { return dfkit.Encode(x) }

// Output is the desired value of nsim.output/<interface>.
type Output struct {
	Interface string `json:"interface"`
}

// Proto encodes o.
func (o Output) Proto() proto.Message { return dfkit.Encode(o) }

func decode[T any](obj proto.Message) (T, error) {
	var v T
	err := dfkit.Decode(obj, &v)
	return v, err
}

// ---- nsim.config ------------------------------------------------------------------------------

// ConfigDescriptor manages nsim.config/global.
type ConfigDescriptor struct {
	client vpp.Client
	store  dfkit.BootStore
}

var _ scheduler.Descriptor = (*ConfigDescriptor)(nil)

// NewConfig returns the nsim.config descriptor.
func NewConfig(c vpp.Client, store dfkit.BootStore) *ConfigDescriptor {
	if store == nil {
		store = dfkit.NewMemoryBootStore()
	}
	return &ConfigDescriptor{client: c, store: store}
}

// Name implements scheduler.Descriptor.
func (*ConfigDescriptor) Name() string { return ConfigName }

// KeyOf implements scheduler.Descriptor.
func (*ConfigDescriptor) KeyOf(proto.Message) scheduler.Key { return ConfigKey() }

// Dependencies implements scheduler.Descriptor: none.
func (*ConfigDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor: nsim_configure2 once per VPP boot and value.
func (d *ConfigDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	c, err := decode[Config](obj)
	if err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	applied, id, err := dfkit.AppliedThisBoot(ctx, d.client, d.store, ConfigKey(), c.record())
	if err != nil || applied {
		return nil, err
	}
	if _, err := nsimapi.NewServiceClient(d.client).NsimConfigure2(ctx, &nsimapi.NsimConfigure2{
		DelayInUsec: c.DelayUsec, AveragePacketSize: c.PacketSize, BandwidthInBitsPerSecond: uint64(c.BandwidthBps),
		PacketsPerDrop: c.PacketsPerDrop,
	}); err != nil {
		return nil, dfkit.PluginError("nsim", fmt.Errorf("nsim_configure2: %w", err))
	}
	if err := d.store.Put(dfkit.BootRecord{Key: string(ConfigKey()), Identity: id, Value: c.record()}); err != nil {
		return nil, fmt.Errorf("%s: record: %w", ConfigKey(), err)
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: a new model replaces the old one (VPP reallocates).
func (d *ConfigDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: VPP 26.06 cannot unconfigure nsim; the model stays
// (inert without a cross-connect or output interface) and the record is forgotten, so the next
// Create configures again.
func (d *ConfigDescriptor) Delete(context.Context, proto.Message, any) error {
	return d.store.Delete(string(ConfigKey()))
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (*ConfigDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(ConfigName)
}

// ---- nsim.cross-connect -----------------------------------------------------------------------

// CrossConnectMeta is the runtime handle: both indexes.
type CrossConnectMeta struct{ A, B uint32 }

// CrossConnectDescriptor manages nsim.cross-connect/global.
type CrossConnectDescriptor struct {
	client vpp.Client
	owner  string
	store  dfkit.BootStore
}

var _ scheduler.Descriptor = (*CrossConnectDescriptor)(nil)

// NewCrossConnect returns the nsim.cross-connect descriptor.
func NewCrossConnect(c vpp.Client, owner string, store dfkit.BootStore) *CrossConnectDescriptor {
	if store == nil {
		store = dfkit.NewMemoryBootStore()
	}
	return &CrossConnectDescriptor{client: c, owner: owner, store: store}
}

// Name implements scheduler.Descriptor.
func (*CrossConnectDescriptor) Name() string { return CrossConnectName }

// KeyOf implements scheduler.Descriptor.
func (*CrossConnectDescriptor) KeyOf(proto.Message) scheduler.Key { return CrossConnectKey() }

// Dependencies implements scheduler.Descriptor: the model (VPP refuses the enable before it is
// configured) and both interface aliases (disabled before either interface goes away, V19/V21).
func (*CrossConnectDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	x, err := decode[CrossConnect](obj)
	if err != nil {
		return nil
	}
	return []scheduler.Dependency{{Key: ConfigKey()}, {Key: dfkit.DefaultInterfaceKey(x.A)}, {Key: dfkit.DefaultInterfaceKey(x.B)}}
}

func pairValue(a, b dfkit.Target) string {
	return strconv.FormatUint(uint64(a.Index), 10) + "/" + a.Name + "," + strconv.FormatUint(uint64(b.Index), 10) + "/" + b.Name
}

func (d *CrossConnectDescriptor) set(ctx context.Context, a, b uint32, on bool) error {
	_, err := nsimapi.NewServiceClient(d.client).NsimCrossConnectEnableDisable(ctx, &nsimapi.NsimCrossConnectEnableDisable{
		EnableDisable: on, SwIfIndex0: interface_types.InterfaceIndex(a), SwIfIndex1: interface_types.InterfaceIndex(b),
	})
	if err != nil {
		return dfkit.PluginError("nsim", fmt.Errorf("nsim_cross_connect_enable_disable (%v) %d %d: %w", on, a, b, err))
	}
	return nil
}

// Create implements scheduler.Descriptor: enable once per VPP boot and pair.
func (d *CrossConnectDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	x, err := decode[CrossConnect](obj)
	if err != nil {
		return nil, err
	}
	if err := x.Validate(); err != nil {
		return nil, err
	}
	a, err := dfkit.ResolveTarget(ctx, d.client, x.A, d.owner, CrossConnectName)
	if err != nil {
		return nil, err
	}
	b, err := dfkit.ResolveTarget(ctx, d.client, x.B, d.owner, CrossConnectName)
	if err != nil {
		return nil, err
	}
	value := pairValue(a, b)
	meta := CrossConnectMeta{A: a.Index, B: b.Index}
	applied, id, err := dfkit.AppliedThisBoot(ctx, d.client, d.store, CrossConnectKey(), value)
	if err != nil {
		return nil, err
	}
	if !applied {
		if err := d.set(ctx, a.Index, b.Index, true); err != nil {
			return nil, err
		}
		if err := d.store.Put(dfkit.BootRecord{Key: string(CrossConnectKey()), Identity: id, Value: value}); err != nil {
			return nil, fmt.Errorf("%s: record: %w", CrossConnectKey(), err)
		}
	}
	if err := a.Claim(); err != nil {
		return nil, err
	}
	return meta, b.Claim()
}

// Update implements scheduler.Descriptor: another pair is a recreate.
func (*CrossConnectDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: disable once with the recorded pair when both interfaces
// still have its names and indexes, then forget the record.
func (d *CrossConnectDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	x, err := decode[CrossConnect](obj)
	if err != nil {
		return err
	}
	a, okA, err := dfkit.ResolveForDelete(ctx, d.client, x.A, d.owner, CrossConnectName)
	if err != nil {
		return err
	}
	b, okB, err := dfkit.ResolveForDelete(ctx, d.client, x.B, d.owner, CrossConnectName)
	if err != nil {
		return err
	}
	if okA && okB {
		applied, _, err := dfkit.AppliedThisBoot(ctx, d.client, d.store, CrossConnectKey(), pairValue(a, b))
		if err != nil {
			return err
		}
		if applied {
			if err := d.set(ctx, a.Index, b.Index, false); err != nil {
				return err
			}
		}
		if err := a.Release(); err != nil {
			return err
		}
		if err := b.Release(); err != nil {
			return err
		}
	}
	return d.store.Delete(string(CrossConnectKey()))
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (*CrossConnectDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(CrossConnectName)
}

// ---- nsim.output ------------------------------------------------------------------------------

// OutputMeta is the runtime handle.
type OutputMeta struct{ SwIfIndex uint32 }

// OutputDescriptor manages nsim.output/<interface>.
type OutputDescriptor struct {
	client vpp.Client
	owner  string
	store  dfkit.BootStore
}

var _ scheduler.Descriptor = (*OutputDescriptor)(nil)

// NewOutput returns the nsim.output descriptor.
func NewOutput(c vpp.Client, owner string, store dfkit.BootStore) *OutputDescriptor {
	if store == nil {
		store = dfkit.NewMemoryBootStore()
	}
	return &OutputDescriptor{client: c, owner: owner, store: store}
}

// Name implements scheduler.Descriptor.
func (*OutputDescriptor) Name() string { return OutputName }

// KeyOf implements scheduler.Descriptor.
func (*OutputDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	o, err := decode[Output](obj)
	if err != nil || o.Interface == "" {
		return scheduler.Join(OutputName, "invalid")
	}
	return OutputKey(o.Interface)
}

// Dependencies implements scheduler.Descriptor: the model and the interface alias.
func (*OutputDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o, err := decode[Output](obj)
	if err != nil {
		return nil
	}
	return []scheduler.Dependency{{Key: ConfigKey()}, {Key: dfkit.DefaultInterfaceKey(o.Interface)}}
}

func (d *OutputDescriptor) set(ctx context.Context, idx uint32, on bool) error {
	_, err := nsimapi.NewServiceClient(d.client).NsimOutputFeatureEnableDisable(ctx, &nsimapi.NsimOutputFeatureEnableDisable{
		EnableDisable: on, SwIfIndex: interface_types.InterfaceIndex(idx),
	})
	if err != nil {
		return dfkit.PluginError("nsim", fmt.Errorf("nsim_output_feature_enable_disable (%v) %d: %w", on, idx, err))
	}
	return nil
}

// Create implements scheduler.Descriptor: enable once per VPP boot, index and name.
func (d *OutputDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, err := decode[Output](obj)
	if err != nil {
		return nil, err
	}
	if o.Interface == "" {
		return nil, dfkit.Specf("nsim.output needs an interface")
	}
	tg, err := dfkit.ResolveTarget(ctx, d.client, o.Interface, d.owner, OutputName)
	if err != nil {
		return nil, err
	}
	value := strconv.FormatUint(uint64(tg.Index), 10) + "/" + o.Interface
	applied, id, err := dfkit.AppliedThisBoot(ctx, d.client, d.store, OutputKey(o.Interface), value)
	if err != nil {
		return nil, err
	}
	if !applied {
		if err := d.set(ctx, tg.Index, true); err != nil {
			return nil, err
		}
		if err := d.store.Put(dfkit.BootRecord{Key: string(OutputKey(o.Interface)), Identity: id, Value: value}); err != nil {
			return nil, fmt.Errorf("%s: record: %w", OutputKey(o.Interface), err)
		}
	}
	return OutputMeta{SwIfIndex: tg.Index}, tg.Claim()
}

// Update implements scheduler.Descriptor: the value is only the interface (= the key).
func (*OutputDescriptor) Update(_ context.Context, _, _ proto.Message, meta any) (any, error) {
	return meta, nil
}

// Delete implements scheduler.Descriptor: disable once on the interface that still has the recorded
// name and index, then forget the record.
func (d *OutputDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	o, err := decode[Output](obj)
	if err != nil {
		return err
	}
	key := OutputKey(o.Interface)
	tg, ok, err := dfkit.ResolveForDelete(ctx, d.client, o.Interface, d.owner, OutputName)
	if err != nil {
		return err
	}
	if ok {
		value := strconv.FormatUint(uint64(tg.Index), 10) + "/" + o.Interface
		applied, _, err := dfkit.AppliedThisBoot(ctx, d.client, d.store, key, value)
		if err != nil {
			return err
		}
		if applied {
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

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (*OutputDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(OutputName)
}

// RegisterGlobals registers the three nsim descriptors. Only the globals owner calls it (D-071): the
// model and the cross-connect pair are VPP-wide, and an output feature without this owner's model
// would run on another agent's parameters. store is the owner's persisted applied-once store.
func RegisterGlobals(r scheduler.Registry, c vpp.Client, owner string, store dfkit.BootStore) {
	r.Register(NewConfig(c, store))
	r.Register(NewCrossConnect(c, owner, store))
	r.Register(NewOutput(c, owner, store))
}
