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
	r.Register(NewInterface(client, owner, opts...))
}

// RegisterGlobals registers this package's VPP-global singleton descriptors, constructed as the
// globals owner (D-071). Call it only in the designated globals owner's agent (config
// globalsOwner: true — never a test slot on the shared host), before Register.
func RegisterGlobals(r scheduler.Registry, client vpp.Client, opts ...Option) {
	opts = append(opts, WithGlobals(dfkit.GlobalsOwner(true)))
	r.Register(NewGlobal(client, opts...))
}

// Option configures the descriptors of this package.
type Option func(*options)

type options struct {
	ifaceKey dfkit.KeyFunc
	globals  dfkit.Globals
}

func buildOptions(opts []Option) options {
	o := options{ifaceKey: dfkit.DefaultInterfaceKey}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithGlobals sets the D-071 role for the VPP-global sflow.global (default: not the globals
// owner — the parameters are then only required, never set or reset).
func WithGlobals(g dfkit.Globals) Option { return func(o *options) { o.globals = g } }

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
type GlobalDescriptor struct {
	client vpp.Client
	o      options
}

var _ scheduler.Descriptor = (*GlobalDescriptor)(nil)

// NewGlobal returns the sflow.global descriptor.
func NewGlobal(client vpp.Client, opts ...Option) *GlobalDescriptor {
	return &GlobalDescriptor{client: client, o: buildOptions(opts)}
}

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
	if !d.o.globals.Owner() {
		if err := s.Validate(); err != nil {
			return err
		}
		return d.o.globals.Require(ctx, NameGlobal, s.Proto(), func(ctx context.Context) (proto.Message, bool, error) {
			cur, err := d.Current(ctx)
			return cur.Proto(), err == nil, err
		})
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
	if !d.o.globals.Owner() {
		return nil
	}
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

// Retrieve implements scheduler.Descriptor: the getters' values while they differ from the
// defaults (for a non-owner: write-only requirement, dfkit.Globals).
func (d *GlobalDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if !d.o.globals.Owner() {
		return d.o.globals.NonOwnerRetrieve(NameGlobal)
	}
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

// InterfaceDescriptor manages sflow.interface objects: key sflow.interface/<interface> (logical
// name, D-069): this owner's tagged interfaces, or untagged ones claimed after a successful enable
// (D-071, review H1).
//
// The hw_if_index gap: sflow_interface_details carries only hw_if_index and no VPP API maps it to
// a sw_if_index. The descriptor keeps a hw → sw map bound to the D-080 VPP boot identity (cleared
// when VPP restarts, review M1). It learns an entry only deterministically, in Create: from the
// dump difference of its own enable when exactly one hw index appeared, otherwise (and when the
// interface was already enabled, e.g. after an agent restart) by disabling it, dumping and
// re-enabling it — the hw index that disappears is its own. Retrieve is read-only (review M2): it
// reports learned entries that are still enabled; an enabled interface it has not learned is not
// reported, so the scheduler runs Create once, which learns it. All calls are serialised.
type InterfaceDescriptor struct {
	client vpp.Client
	owner  string
	o      options

	mu         sync.Mutex
	learned    map[uint32]uint32 // hw_if_index → sw_if_index
	learnedFor string            // boot identity the map belongs to
}

var _ scheduler.Descriptor = (*InterfaceDescriptor)(nil)

// NewInterface returns the sflow.interface descriptor.
func NewInterface(client vpp.Client, owner string, opts ...Option) *InterfaceDescriptor {
	return &InterfaceDescriptor{client: client, owner: owner, o: buildOptions(opts), learned: map[uint32]uint32{}}
}

// epoch drops the map when VPP's boot identity changed (M1). Caller holds d.mu.
func (d *InterfaceDescriptor) epoch(ctx context.Context) error {
	id, err := dfkit.IdentitySource(ctx, d.client)
	if err != nil {
		return err
	}
	if id != d.learnedFor {
		d.learned = map[uint32]uint32{}
		d.learnedFor = id
	}
	return nil
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

// Create implements scheduler.Descriptor. An already enabled interface (VALUE_EXIST) is accepted
// only if it is ours (tagged, or claimed before an agent restart); the claim is recorded only after
// VPP accepted the enable (review H1).
func (d *InterfaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	s, err := d.spec(obj)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.epoch(ctx); err != nil {
		return nil, err
	}
	tg, err := dfkit.ResolveTarget(ctx, d.client, s.Interface, d.owner, NameInterface)
	if err != nil {
		return nil, err
	}
	before, err := d.enabledHw(ctx)
	if err != nil {
		return nil, err
	}
	err = d.enable(ctx, tg.Index, true)
	switch {
	case dfkit.IsVPPError(err, api.VALUE_EXIST):
		if aerr := tg.Adopt(); aerr != nil {
			return nil, aerr
		}
		if d.hwOf(tg.Index) == 0 {
			if err := d.learnByToggle(ctx, tg.Index); err != nil {
				return nil, err
			}
		}
	case err != nil:
		return nil, fmt.Errorf("sflow_enable_disable(%s, enable=1): %w", s.Interface, err)
	default:
		after, err := d.enabledHw(ctx)
		if err != nil {
			return nil, err
		}
		var fresh []uint32
		for hw := range after {
			if !before[hw] {
				fresh = append(fresh, hw)
			}
		}
		if len(fresh) == 1 {
			d.learned[fresh[0]] = tg.Index
		} else if err := d.learnByToggle(ctx, tg.Index); err != nil { // someone else enabled concurrently
			return nil, err
		}
		if err := tg.Claim(); err != nil {
			return nil, err
		}
	}
	return InterfaceMeta{SwIfIndex: tg.Index, HwIfIndex: d.hwOf(tg.Index)}, nil
}

// learnByToggle finds the hw index of our enabled interface sw: disable, dump, re-enable — the
// index that disappeared is ours. It writes the data plane only for our own interface and only on
// the Create (write) path. Ambiguous results (a concurrent change) learn nothing; the next
// resync's Create tries again. Caller holds d.mu.
func (d *InterfaceDescriptor) learnByToggle(ctx context.Context, sw uint32) error {
	before, err := d.enabledHw(ctx)
	if err != nil {
		return err
	}
	if err := d.enable(ctx, sw, false); err != nil {
		return fmt.Errorf("sflow learn (disable): %w", err)
	}
	mid, derr := d.enabledHw(ctx)
	if err := d.enable(ctx, sw, true); err != nil && !dfkit.IsVPPError(err, api.VALUE_EXIST) {
		return fmt.Errorf("sflow learn (re-enable): %w", err)
	}
	if derr != nil {
		return derr
	}
	var gone []uint32
	for hw := range before {
		if !mid[hw] {
			gone = append(gone, hw)
		}
	}
	if len(gone) == 1 {
		d.learned[gone[0]] = sw
	}
	return nil
}

func (d *InterfaceDescriptor) hwOf(sw uint32) uint32 {
	for hw, s := range d.learned {
		if s == sw {
			return hw
		}
	}
	return 0
}

// Update implements scheduler.Descriptor: the object has no field besides its key; re-apply.
func (d *InterfaceDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor. It re-resolves the interface right before acting by
// index and never touches an unclaimed untagged interface (H1). sflow_enable_disable(0) on a
// disabled interface is a harmless VALUE_EXIST (host-verified): that is the D-074 existence check.
func (d *InterfaceDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	s, err := d.spec(obj)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	tg, ok, err := dfkit.ResolveForDelete(ctx, d.client, s.Interface, d.owner, NameInterface)
	if err != nil || !ok {
		return err
	}
	err = d.enable(ctx, tg.Index, false)
	if err != nil && !dfkit.IsVPPError(err, api.VALUE_EXIST, api.INVALID_SW_IF_INDEX) {
		return fmt.Errorf("sflow_enable_disable(%s, enable=0): %w", s.Interface, err)
	}
	for hw, sw := range d.learned {
		if sw == tg.Index {
			delete(d.learned, hw)
		}
	}
	return tg.Release()
}

// Retrieve implements scheduler.Descriptor: read-only (M2) — learned entries (current VPP boot
// identity) whose hw index is enabled and whose interface is still ours.
func (d *InterfaceDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.epoch(ctx); err != nil {
		return nil, err
	}
	hws, err := d.enabledHw(ctx)
	if err != nil {
		return nil, err
	}
	for hw := range d.learned { // learned but no longer enabled
		if !hws[hw] {
			delete(d.learned, hw)
		}
	}
	if len(d.learned) == 0 {
		return nil, nil
	}
	ifaces, err := dfkit.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for hw, sw := range d.learned {
		name, mine := ifaces.Reportable(sw, NameInterface)
		if !mine {
			continue
		}
		out = append(out, scheduler.KV{Key: scheduler.Join(NameInterface, name), Value: Interface{Interface: name}.Proto(),
			Meta: InterfaceMeta{SwIfIndex: sw, HwIfIndex: hw}})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return dfkit.Dedupe(out), nil
}
