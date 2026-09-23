// Package sflow holds the reconciler descriptors for VPP's sflow plugin (random packet sampling,
// task DF-8, WBS D7.6): the global sampling parameters and sFlow on an interface. Export to a
// collector is hsflowd's job (out of scope). Message names come only from
// apps/agent/binapi/sflow; docs/agent/descriptors/sflow.md is the object ↔ message table.
//
// VPP API quirk: sflow_enable_disable takes a sw_if_index in its field named hw_if_index, but
// sflow_interface_details reports the real hw_if_index, and no VPP API maps hw_if_index back to
// a sw_if_index. InterfaceDescriptor learns the mapping when it enables an interface; for
// enabled hw indexes it has not learned (after an agent restart, or another owner's interfaces)
// Retrieve probes the owned interfaces — see InterfaceDescriptor.
package sflow

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/sflow"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameGlobal    = "sflow.global"
	NameInterface = "sflow.interface"
)

// VPP defaults (sflow.h) — the state Delete restores and Retrieve treats as "not configured".
const (
	DefaultSamplingRate    = 10000
	DefaultPollingInterval = 20
	DefaultHeaderBytes     = 128
	DefaultDirection       = "rx"
)

// Global is the sflow.global singleton: 1-in-N sampling rate (0 = sampling off), counter polling
// interval (s), sampled header bytes (64..256 in steps of 32), direction (rx, tx, both) and drop
// monitoring.
type Global struct {
	SamplingRate    uint32 `json:"sampling_rate"`
	PollingInterval uint32 `json:"polling_interval"`
	HeaderBytes     uint32 `json:"header_bytes"`
	Direction       string `json:"direction"`
	DropMonitoring  bool   `json:"drop_monitoring"`
}

// Interface is sFlow sampling on one interface.
type Interface struct {
	Interface string `json:"interface"`
}

// Proto returns the canonical structpb document.
func (s Global) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s Interface) Proto() *structpb.Struct { return dfkit.Encode(s) }

// DefaultGlobal is VPP's initial sflow.global state.
func DefaultGlobal() Global {
	return Global{SamplingRate: DefaultSamplingRate, PollingInterval: DefaultPollingInterval, HeaderBytes: DefaultHeaderBytes, Direction: DefaultDirection}
}

var (
	dirToAPI   = map[string]uint32{"rx": 1, "tx": 2, "both": 3} // sflow_common.h SFLOW_DIRN_*
	dirFromAPI = map[uint32]string{1: "rx", 2: "tx", 3: "both"}
)

// Validate checks the direction and the header size (VPP silently rounds it otherwise, which
// would make the retrieved value differ from the desired one forever).
func (s Global) Validate() error {
	if _, ok := dirToAPI[s.Direction]; !ok {
		return dfkit.Specf("sflow: direction %q must be rx, tx or both", s.Direction)
	}
	if s.HeaderBytes < 64 || s.HeaderBytes > 256 || s.HeaderBytes%32 != 0 {
		return dfkit.Specf("sflow: header_bytes %d must be 64..256 in steps of 32", s.HeaderBytes)
	}
	return nil
}

// Register constructs and registers the sflow descriptors.
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...Option) {
	r.Register(NewGlobal(client))
	r.Register(NewInterface(client, owner, opts...))
}

// Option configures the descriptors of this package.
type Option func(*options)

type options struct{ ifaceKey dfkit.KeyFunc }

// WithInterfaceKey sets the interface key scheme of Dependencies (default "interface/<name>", D-065).
func WithInterfaceKey(f dfkit.KeyFunc) Option {
	return func(o *options) {
		if f != nil {
			o.ifaceKey = f
		}
	}
}

// ---- sflow.global ---------------------------------------------------------------------------

// GlobalID is the object id of the singleton (key sflow.global/global).
const GlobalID = "global"

// KeyGlobal is the key of the singleton.
var KeyGlobal = scheduler.Join(NameGlobal, GlobalID)

// GlobalDescriptor manages the sflow.global singleton through the five *_set messages; Retrieve
// reads the five *_get messages and reports the object while it differs from VPP's defaults;
// Delete restores the defaults.
type GlobalDescriptor struct{ client vpp.Client }

var _ scheduler.Descriptor = (*GlobalDescriptor)(nil)

// NewGlobal returns the sflow.global descriptor.
func NewGlobal(client vpp.Client) *GlobalDescriptor { return &GlobalDescriptor{client: client} }

// Name implements scheduler.Descriptor.
func (*GlobalDescriptor) Name() string { return NameGlobal }

// KeyOf implements scheduler.Descriptor.
func (*GlobalDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyGlobal }

// Dependencies implements scheduler.Descriptor: none.
func (*GlobalDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *GlobalDescriptor) set(ctx context.Context, s Global) error {
	if err := s.Validate(); err != nil {
		return err
	}
	svc := sflow.NewServiceClient(d.client)
	var drop uint32
	if s.DropMonitoring {
		drop = 1
	}
	steps := []struct {
		name string
		call func() error
	}{
		{"sflow_sampling_rate_set", func() error {
			_, err := svc.SflowSamplingRateSet(ctx, &sflow.SflowSamplingRateSet{SamplingN: s.SamplingRate})
			return err
		}},
		{"sflow_polling_interval_set", func() error {
			_, err := svc.SflowPollingIntervalSet(ctx, &sflow.SflowPollingIntervalSet{PollingS: s.PollingInterval})
			return err
		}},
		{"sflow_header_bytes_set", func() error {
			_, err := svc.SflowHeaderBytesSet(ctx, &sflow.SflowHeaderBytesSet{HeaderB: s.HeaderBytes})
			return err
		}},
		{"sflow_direction_set", func() error {
			_, err := svc.SflowDirectionSet(ctx, &sflow.SflowDirectionSet{SamplingD: dirToAPI[s.Direction]})
			return err
		}},
		{"sflow_drop_monitoring_set", func() error {
			_, err := svc.SflowDropMonitoringSet(ctx, &sflow.SflowDropMonitoringSet{DropM: drop})
			return err
		}},
	}
	for _, st := range steps {
		if err := st.call(); err != nil {
			return fmt.Errorf("%s: %w", st.name, err)
		}
	}
	return nil
}

func (d *GlobalDescriptor) apply(ctx context.Context, obj proto.Message) error {
	var s Global
	if err := dfkit.Decode(obj, &s); err != nil {
		return err
	}
	return d.set(ctx, s)
}

// Create implements scheduler.Descriptor (idempotent).
func (d *GlobalDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.apply(ctx, obj)
}

// Update implements scheduler.Descriptor (in place; VPP re-programs enabled interfaces).
func (d *GlobalDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.apply(ctx, newObj)
}

// Delete implements scheduler.Descriptor: back to VPP's defaults.
func (d *GlobalDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	return d.set(ctx, DefaultGlobal())
}

// Current reads the five getters.
func (d *GlobalDescriptor) Current(ctx context.Context) (Global, error) {
	svc := sflow.NewServiceClient(d.client)
	n, err := svc.SflowSamplingRateGet(ctx, &sflow.SflowSamplingRateGet{})
	if err != nil {
		return Global{}, fmt.Errorf("sflow_sampling_rate_get: %w", err)
	}
	p, err := svc.SflowPollingIntervalGet(ctx, &sflow.SflowPollingIntervalGet{})
	if err != nil {
		return Global{}, fmt.Errorf("sflow_polling_interval_get: %w", err)
	}
	h, err := svc.SflowHeaderBytesGet(ctx, &sflow.SflowHeaderBytesGet{})
	if err != nil {
		return Global{}, fmt.Errorf("sflow_header_bytes_get: %w", err)
	}
	dir, err := svc.SflowDirectionGet(ctx, &sflow.SflowDirectionGet{})
	if err != nil {
		return Global{}, fmt.Errorf("sflow_direction_get: %w", err)
	}
	m, err := svc.SflowDropMonitoringGet(ctx, &sflow.SflowDropMonitoringGet{})
	if err != nil {
		return Global{}, fmt.Errorf("sflow_drop_monitoring_get: %w", err)
	}
	ds, ok := dirFromAPI[dir.SamplingD]
	if !ok {
		ds = fmt.Sprintf("unknown-%d", dir.SamplingD)
	}
	return Global{SamplingRate: n.SamplingN, PollingInterval: p.PollingS, HeaderBytes: h.HeaderB, Direction: ds, DropMonitoring: m.DropM != 0}, nil
}

// Retrieve implements scheduler.Descriptor: the getters' values while they differ from the defaults.
func (d *GlobalDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	cur, err := d.Current(ctx)
	if err != nil {
		return nil, err
	}
	if cur == DefaultGlobal() {
		return nil, nil
	}
	return []scheduler.KV{{Key: KeyGlobal, Value: cur.Proto()}}, nil
}

// ---- sflow.interface ------------------------------------------------------------------------

// InterfaceMeta is the Meta of an sflow.interface: the sw_if_index and the hw_if_index VPP
// reports for it (0 until learned).
type InterfaceMeta struct {
	SwIfIndex uint32
	HwIfIndex uint32
}

// InterfaceDescriptor manages sflow.interface objects: key sflow.interface/<interface>, owned
// interfaces only.
//
// Retrieve: sflow_interface_dump gives the enabled hw_if_indexes. Those learned at Create map to
// owned interfaces directly. If unlearned indexes remain, every owned interface not yet known to
// be enabled is probed with sflow_enable_disable(enable=1): VALUE_EXIST means it is enabled
// (reported); success means it was not, and it is disabled again at once (only this agent's own
// interfaces are ever probed, and only while unlearned enabled indexes exist). The mapping found
// this way is learned when it is unambiguous. All calls of one descriptor are serialised.
type InterfaceDescriptor struct {
	client vpp.Client
	owner  string
	o      options

	mu      sync.Mutex
	learned map[uint32]string // hw_if_index → interface name
}

var _ scheduler.Descriptor = (*InterfaceDescriptor)(nil)

// NewInterface returns the sflow.interface descriptor.
func NewInterface(client vpp.Client, owner string, opts ...Option) *InterfaceDescriptor {
	o := options{ifaceKey: dfkit.DefaultInterfaceKey}
	for _, opt := range opts {
		opt(&o)
	}
	return &InterfaceDescriptor{client: client, owner: owner, o: o, learned: map[uint32]string{}}
}

// Name implements scheduler.Descriptor.
func (*InterfaceDescriptor) Name() string { return NameInterface }

// KeyOf implements scheduler.Descriptor.
func (*InterfaceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	var s Interface
	if err := dfkit.Decode(obj, &s); err != nil {
		return scheduler.Join(NameInterface, "invalid")
	}
	return scheduler.Join(NameInterface, s.Interface)
}

// Dependencies implements scheduler.Descriptor: the interface and the global parameters (optional:
// VPP's defaults are usable).
func (d *InterfaceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	var s Interface
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil
	}
	return []scheduler.Dependency{{Key: d.o.ifaceKey(s.Interface)}, {Key: KeyGlobal, Optional: true}}
}

func (d *InterfaceDescriptor) enable(ctx context.Context, swIfIndex uint32, on bool) error {
	_, err := sflow.NewServiceClient(d.client).SflowEnableDisable(ctx, &sflow.SflowEnableDisable{
		EnableDisable: on, HwIfIndex: interface_types.InterfaceIndex(swIfIndex), // VPP reads a sw_if_index here
	})
	return err
}

func (d *InterfaceDescriptor) enabledHw(ctx context.Context) (map[uint32]bool, error) {
	stream, err := sflow.NewServiceClient(d.client).SflowInterfaceDump(ctx,
		&sflow.SflowInterfaceDump{HwIfIndex: interface_types.InterfaceIndex(dfkit.AllInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("sflow_interface_dump: %w", err)
	}
	details, err := dfkit.Drain(stream, stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("sflow_interface_dump: %w", err)
	}
	out := make(map[uint32]bool, len(details))
	for _, det := range details {
		out[uint32(det.HwIfIndex)] = true
	}
	return out, nil
}

func (d *InterfaceDescriptor) spec(obj proto.Message) (Interface, error) {
	var s Interface
	if err := dfkit.Decode(obj, &s); err != nil {
		return s, err
	}
	if s.Interface == "" {
		return s, dfkit.Specf("sflow interface: interface is empty")
	}
	return s, nil
}

// Create implements scheduler.Descriptor; VALUE_EXIST (already enabled) is success. The
// hw_if_index is learned from the dump difference.
func (d *InterfaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	s, err := d.spec(obj)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	idx, err := dfkit.ResolveInterface(ctx, d.client, s.Interface, d.owner)
	if err != nil {
		return nil, err
	}
	before, err := d.enabledHw(ctx)
	if err != nil {
		return nil, err
	}
	if err := d.enable(ctx, idx, true); err != nil {
		if dfkit.IsVPPError(err, api.VALUE_EXIST) {
			return InterfaceMeta{SwIfIndex: idx, HwIfIndex: d.hwOf(s.Interface)}, nil
		}
		return nil, fmt.Errorf("sflow_enable_disable(%s, enable=1): %w", s.Interface, err)
	}
	after, err := d.enabledHw(ctx)
	if err != nil {
		return nil, err
	}
	meta := InterfaceMeta{SwIfIndex: idx}
	for hw := range after {
		if !before[hw] {
			meta.HwIfIndex = hw
			d.learned[hw] = s.Interface
		}
	}
	return meta, nil
}

func (d *InterfaceDescriptor) hwOf(name string) uint32 {
	for hw, n := range d.learned {
		if n == name {
			return hw
		}
	}
	return 0
}

// Update implements scheduler.Descriptor: the object has no field besides its key; re-apply.
func (d *InterfaceDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor; VALUE_EXIST (already disabled) and a vanished
// interface count as deleted.
func (d *InterfaceDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	s, err := d.spec(obj)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	var idx uint32
	if m, ok := meta.(InterfaceMeta); ok {
		idx = m.SwIfIndex
	} else if idx, err = dfkit.ResolveInterface(ctx, d.client, s.Interface, d.owner); err != nil {
		return err
	}
	err = d.enable(ctx, idx, false)
	if err != nil && !dfkit.IsVPPError(err, api.VALUE_EXIST, api.INVALID_SW_IF_INDEX) {
		return fmt.Errorf("sflow_enable_disable(%s, enable=0): %w", s.Interface, err)
	}
	for hw, n := range d.learned {
		if n == s.Interface {
			delete(d.learned, hw)
		}
	}
	return nil
}

// Retrieve implements scheduler.Descriptor (see the type doc for the probe).
func (d *InterfaceDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	hws, err := d.enabledHw(ctx)
	if err != nil {
		return nil, err
	}
	if len(hws) == 0 {
		d.learned = map[uint32]string{}
		return nil, nil
	}
	ifaces, err := dfkit.DumpInterfaces(ctx, d.client)
	if err != nil {
		return nil, err
	}
	enabled := map[string]InterfaceMeta{}
	var unknown []uint32
	for hw := range hws {
		name, ok := d.learned[hw]
		if i, exists := ifaces.ByName[name]; ok && exists && i.Owned(d.owner) {
			enabled[name] = InterfaceMeta{SwIfIndex: i.Index, HwIfIndex: hw}
			continue
		}
		delete(d.learned, hw)
		unknown = append(unknown, hw)
	}
	for hw := range d.learned { // learned but no longer enabled
		if !hws[hw] {
			delete(d.learned, hw)
		}
	}
	if len(unknown) > 0 {
		found, err := d.probe(ctx, ifaces, enabled)
		if err != nil {
			return nil, err
		}
		if len(unknown) == 1 && len(found) == 1 { // unambiguous: learn it
			d.learned[unknown[0]] = found[0]
			m := enabled[found[0]]
			m.HwIfIndex = unknown[0]
			enabled[found[0]] = m
		}
	}
	out := make([]scheduler.KV, 0, len(enabled))
	for name, m := range enabled {
		out = append(out, scheduler.KV{Key: scheduler.Join(NameInterface, name), Value: Interface{Interface: name}.Proto(), Meta: m})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// probe finds the owned (non-sub) interfaces that are enabled but not yet in enabled, adding them.
func (d *InterfaceDescriptor) probe(ctx context.Context, ifaces *dfkit.Ifaces, enabled map[string]InterfaceMeta) ([]string, error) {
	var found []string
	names := make([]string, 0, len(ifaces.ByName))
	for n := range ifaces.ByName {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		i := ifaces.ByName[name]
		if _, known := enabled[name]; known || !i.Owned(d.owner) || i.SupIndex != i.Index {
			continue
		}
		err := d.enable(ctx, i.Index, true)
		switch {
		case dfkit.IsVPPError(err, api.VALUE_EXIST):
			enabled[name] = InterfaceMeta{SwIfIndex: i.Index}
			found = append(found, name)
		case err == nil: // was disabled: undo at once
			if err := d.enable(ctx, i.Index, false); err != nil {
				return nil, fmt.Errorf("sflow probe of %s: undo enable: %w", name, err)
			}
		default:
			return nil, fmt.Errorf("sflow probe of %s: %w", name, err)
		}
	}
	return found, nil
}
