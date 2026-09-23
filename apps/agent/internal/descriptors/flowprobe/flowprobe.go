// Package flowprobe holds the reconciler descriptors for VPP's flowprobe plugin (IPFIX flow
// records per packet, task DF-8, WBS D7.6): the global record/timer parameters and flowprobe on
// an interface. Records go to the IPFIX default exporter (ipfix.default-exporter — flowprobe
// uses exporter 0 only). Message names come only from apps/agent/binapi/flowprobe;
// docs/agent/descriptors/flowprobe.md is the object ↔ message table.
//
// VPP accepts parameter changes only while no interface has flowprobe enabled (any owner's):
// flowprobe.params therefore recreates on every change, which makes the scheduler remove and
// re-add the dependent flowprobe.interface objects around it.
package flowprobe

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/binapi/flowprobe"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameParams    = "flowprobe.params"
	NameInterface = "flowprobe.interface"
)

// Default timers of VPP (seconds).
const (
	DefaultActiveTimer  = 15
	DefaultPassiveTimer = 120
)

// Params is the flowprobe.params singleton: which record fields are exported and the flow
// timers (seconds; 0 = off; passive must be ≥ active when set).
type Params struct {
	RecordL2     bool   `json:"record_l2"`
	RecordL3     bool   `json:"record_l3"`
	RecordL4     bool   `json:"record_l4"`
	ActiveTimer  uint32 `json:"active_timer"`
	PassiveTimer uint32 `json:"passive_timer"`
}

// Interface is flowprobe on one interface: the variant (ip4, ip6 or l2 — one per interface) and
// the direction (rx, tx or both).
type Interface struct {
	Interface string `json:"interface"`
	Which     string `json:"which"`
	Direction string `json:"direction"`
}

// Proto returns the canonical structpb document.
func (s Params) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s Interface) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Validate checks that something is recorded (record 0 means "unset" to VPP) and the timers.
func (s Params) Validate() error {
	if !s.RecordL2 && !s.RecordL3 && !s.RecordL4 {
		return dfkit.Specf("flowprobe params: record at least one of l2, l3, l4")
	}
	if s.ActiveTimer == ^uint32(0) || s.PassiveTimer == ^uint32(0) {
		return dfkit.Specf("flowprobe params: timers must be explicit (default active %d, passive %d)", DefaultActiveTimer, DefaultPassiveTimer)
	}
	if s.PassiveTimer > 0 && s.ActiveTimer > s.PassiveTimer {
		return dfkit.Specf("flowprobe params: passive timer %d must be ≥ active timer %d", s.PassiveTimer, s.ActiveTimer)
	}
	return nil
}

var (
	whichToAPI = map[string]flowprobe.FlowprobeWhich{
		"ip4": flowprobe.FLOWPROBE_WHICH_IP4, "ip6": flowprobe.FLOWPROBE_WHICH_IP6, "l2": flowprobe.FLOWPROBE_WHICH_L2,
	}
	dirToAPI = map[string]flowprobe.FlowprobeDirection{
		"rx": flowprobe.FLOWPROBE_DIRECTION_RX, "tx": flowprobe.FLOWPROBE_DIRECTION_TX, "both": flowprobe.FLOWPROBE_DIRECTION_BOTH,
	}
)

// Validate checks the variant and direction.
func (s Interface) Validate() error {
	if s.Interface == "" {
		return dfkit.Specf("flowprobe interface: interface is empty")
	}
	if _, ok := whichToAPI[s.Which]; !ok {
		return dfkit.Specf("flowprobe interface: which %q must be ip4, ip6 or l2", s.Which)
	}
	if _, ok := dirToAPI[s.Direction]; !ok {
		return dfkit.Specf("flowprobe interface: direction %q must be rx, tx or both", s.Direction)
	}
	return nil
}

// Option configures the descriptors of this package.
type Option func(*options)

type options struct {
	ifaceKey    dfkit.KeyFunc
	exporterKey scheduler.Key
	globals     dfkit.Globals
}

func buildOptions(opts []Option) options {
	o := options{ifaceKey: dfkit.DefaultInterfaceKey, exporterKey: "ipfix.default-exporter/global"}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithInterfaceKey sets the interface key scheme of Dependencies (default "interface/<name>", D-065).
func WithInterfaceKey(f dfkit.KeyFunc) Option {
	return func(o *options) {
		if f != nil {
			o.ifaceKey = f
		}
	}
}

// WithGlobals sets the D-071 role for the VPP-global flowprobe.params (default: not the globals
// owner — the parameters are then only required, never set or reset).
func WithGlobals(g dfkit.Globals) Option { return func(o *options) { o.globals = g } }

// Register constructs and registers the flowprobe descriptors.
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...Option) {
	r.Register(NewParams(client, opts...))
	r.Register(NewInterface(client, owner, opts...))
}

// ---- flowprobe.params -----------------------------------------------------------------------

// ParamsID is the object id of the singleton (key flowprobe.params/global).
const ParamsID = "global"

// KeyParams is the key of the singleton.
var KeyParams = scheduler.Join(NameParams, ParamsID)

// ParamsDescriptor manages the flowprobe.params singleton (flowprobe_set_params /
// flowprobe_get_params). Retrieve reports it while a record flag is set; Delete sets record 0
// (VPP's "unset", no interface can be enabled then) and the default timers.
type ParamsDescriptor struct {
	client vpp.Client
	o      options
}

var _ scheduler.Descriptor = (*ParamsDescriptor)(nil)

// NewParams returns the flowprobe.params descriptor.
func NewParams(client vpp.Client, opts ...Option) *ParamsDescriptor {
	return &ParamsDescriptor{client: client, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*ParamsDescriptor) Name() string { return NameParams }

// KeyOf implements scheduler.Descriptor.
func (*ParamsDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyParams }

// Dependencies implements scheduler.Descriptor: none.
func (*ParamsDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *ParamsDescriptor) set(ctx context.Context, flags flowprobe.FlowprobeRecordFlags, active, passive uint32) error {
	_, err := flowprobe.NewServiceClient(d.client).FlowprobeSetParams(ctx, &flowprobe.FlowprobeSetParams{
		RecordFlags: flags, ActiveTimer: active, PassiveTimer: passive,
	})
	if dfkit.IsVPPError(err, api.UNSUPPORTED) {
		return fmt.Errorf("flowprobe_set_params: flowprobe is enabled on some interface (VPP allows changes only while none is): %w", err)
	}
	if err != nil {
		return fmt.Errorf("flowprobe_set_params: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor (idempotent while no interface is enabled).
func (d *ParamsDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	var s Params
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if !d.o.globals.Owner() {
		return nil, d.o.globals.Require(ctx, NameParams, s.Proto(), d.current)
	}
	var flags flowprobe.FlowprobeRecordFlags
	if s.RecordL2 {
		flags |= flowprobe.FLOWPROBE_RECORD_FLAG_L2
	}
	if s.RecordL3 {
		flags |= flowprobe.FLOWPROBE_RECORD_FLAG_L3
	}
	if s.RecordL4 {
		flags |= flowprobe.FLOWPROBE_RECORD_FLAG_L4
	}
	return nil, d.set(ctx, flags, s.ActiveTimer, s.PassiveTimer)
}

// Update implements scheduler.Descriptor: recreate, so the scheduler takes the dependent
// flowprobe.interface objects down first (see package doc).
func (*ParamsDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: record 0 + default timers (globals owner only).
func (d *ParamsDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	if !d.o.globals.Owner() {
		return nil
	}
	return d.set(ctx, 0, DefaultActiveTimer, DefaultPassiveTimer)
}

// Retrieve implements scheduler.Descriptor: flowprobe_get_params while a record flag is set (for
// a non-owner: write-only requirement, dfkit.Globals).
func (d *ParamsDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if !d.o.globals.Owner() {
		return d.o.globals.NonOwnerRetrieve(NameParams)
	}
	v, ok, err := d.current(ctx)
	if err != nil || !ok {
		return nil, err
	}
	return []scheduler.KV{{Key: KeyParams, Value: v}}, nil
}

// current reads flowprobe_get_params (ok=false while no record flag is set).
func (d *ParamsDescriptor) current(ctx context.Context) (proto.Message, bool, error) {
	rep, err := flowprobe.NewServiceClient(d.client).FlowprobeGetParams(ctx, &flowprobe.FlowprobeGetParams{})
	if err != nil {
		return nil, false, fmt.Errorf("flowprobe_get_params: %w", err)
	}
	if rep.RecordFlags == 0 {
		return nil, false, nil
	}
	s := Params{
		RecordL2:    rep.RecordFlags&flowprobe.FLOWPROBE_RECORD_FLAG_L2 != 0,
		RecordL3:    rep.RecordFlags&flowprobe.FLOWPROBE_RECORD_FLAG_L3 != 0,
		RecordL4:    rep.RecordFlags&flowprobe.FLOWPROBE_RECORD_FLAG_L4 != 0,
		ActiveTimer: rep.ActiveTimer, PassiveTimer: rep.PassiveTimer,
	}
	return s.Proto(), true, nil
}

// ---- flowprobe.interface --------------------------------------------------------------------

// InterfaceMeta is the Meta of a flowprobe.interface object.
type InterfaceMeta struct {
	SwIfIndex uint32
}

// InterfaceDescriptor manages flowprobe.interface objects: key flowprobe.interface/<interface>
// (logical name, D-069): this owner's tagged interface, or an untagged one claimed on Create (D-071).
type InterfaceDescriptor struct {
	client vpp.Client
	owner  string
	o      options
}

var _ scheduler.Descriptor = (*InterfaceDescriptor)(nil)

// NewInterface returns the flowprobe.interface descriptor.
func NewInterface(client vpp.Client, owner string, opts ...Option) *InterfaceDescriptor {
	return &InterfaceDescriptor{client: client, owner: owner, o: buildOptions(opts)}
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

// Dependencies implements scheduler.Descriptor: the interface, the params (VPP refuses an
// interface before record flags are set) and the default exporter (optional — records are only
// sent once a collector is set).
func (d *InterfaceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	var s Interface
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil
	}
	return []scheduler.Dependency{
		{Key: d.o.ifaceKey(s.Interface)},
		{Key: KeyParams},
		{Key: d.o.exporterKey, Optional: true},
	}
}

func (d *InterfaceDescriptor) addDel(ctx context.Context, obj proto.Message, meta any, add bool) (any, error) {
	var s Interface
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	// resolve the logical name on every call, never a Meta index (reused after a VPP restart)
	_ = meta
	resolve := dfkit.ResolveInterface
	if add {
		resolve = func(ctx context.Context, c vpp.Client, name, owner string) (uint32, error) {
			return dfkit.ResolveAndClaim(ctx, c, name, owner, NameInterface)
		}
	}
	idx, err := resolve(ctx, d.client, s.Interface, d.owner)
	if err != nil {
		return nil, err
	}
	_, err = flowprobe.NewServiceClient(d.client).FlowprobeInterfaceAddDel(ctx, &flowprobe.FlowprobeInterfaceAddDel{
		IsAdd: add, Which: whichToAPI[s.Which], Direction: dirToAPI[s.Direction], SwIfIndex: interface_types.InterfaceIndex(idx),
	})
	if err != nil {
		return nil, fmt.Errorf("flowprobe_interface_add_del(%s %s %s, add=%t): %w", s.Interface, s.Which, s.Direction, add, err)
	}
	return InterfaceMeta{SwIfIndex: idx}, nil
}

// Create implements scheduler.Descriptor. VPP answers ENTRY_ALREADY_EXISTS for any variant
// already on the interface; Create then succeeds only if the retrieved state is identical.
func (d *InterfaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	meta, err := d.addDel(ctx, obj, nil, true)
	if dfkit.IsVPPError(err, api.ENTRY_ALREADY_EXISTS) {
		kvs, rerr := d.Retrieve(ctx)
		if rerr == nil {
			if cur, ok := findKV(kvs, d.KeyOf(obj)); ok && proto.Equal(cur.Value, obj) {
				return cur.Meta, nil
			}
		}
	}
	return meta, err
}

// Update implements scheduler.Descriptor: variant and direction change needs disable + enable.
func (*InterfaceDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor. It first checks that flowprobe is still on the
// interface (D-074); NO_SUCH_ENTRY / INVALID_SW_IF_INDEX / a vanished interface count as deleted.
func (d *InterfaceDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		return err
	}
	var name string
	if s, derr := decodeIface(obj); derr == nil {
		name = s.Interface
	}
	if _, ok := findKV(kvs, d.KeyOf(obj)); ok {
		_, err = d.addDel(ctx, obj, meta, false)
		if err != nil && !dfkit.IsVPPError(err, api.NO_SUCH_ENTRY, api.INVALID_SW_IF_INDEX) && !errors.Is(err, dfkit.ErrNoInterface) {
			return err
		}
	}
	return dfkit.Claims(d.owner).Release(name, NameInterface)
}

func decodeIface(obj proto.Message) (Interface, error) {
	var s Interface
	err := dfkit.Decode(obj, &s)
	return s, err
}

var (
	whichFromAPI = map[flowprobe.FlowprobeWhich]string{
		flowprobe.FLOWPROBE_WHICH_IP4: "ip4", flowprobe.FLOWPROBE_WHICH_IP6: "ip6", flowprobe.FLOWPROBE_WHICH_L2: "l2",
	}
	dirFromAPI = map[flowprobe.FlowprobeDirection]string{
		flowprobe.FLOWPROBE_DIRECTION_RX: "rx", flowprobe.FLOWPROBE_DIRECTION_TX: "tx", flowprobe.FLOWPROBE_DIRECTION_BOTH: "both",
	}
)

// Retrieve implements scheduler.Descriptor: flowprobe_interface_dump, owned interfaces.
func (d *InterfaceDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	stream, err := flowprobe.NewServiceClient(d.client).FlowprobeInterfaceDump(ctx,
		&flowprobe.FlowprobeInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(dfkit.AllInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("flowprobe_interface_dump: %w", err)
	}
	details, err := dfkit.Drain(stream, stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("flowprobe_interface_dump: %w", err)
	}
	if len(details) == 0 {
		return nil, nil
	}
	ifaces, err := dfkit.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, det := range details {
		idx := uint32(det.SwIfIndex)
		name, ok := ifaces.Reportable(idx, NameInterface)
		if !ok {
			continue
		}
		s := Interface{Interface: name, Which: whichFromAPI[det.Which], Direction: dirFromAPI[det.Direction]}
		out = append(out, scheduler.KV{Key: scheduler.Join(NameInterface, name), Value: s.Proto(), Meta: InterfaceMeta{SwIfIndex: idx}})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return dfkit.Dedupe(out), nil
}

func findKV(kvs []scheduler.KV, k scheduler.Key) (scheduler.KV, bool) {
	for _, kv := range kvs {
		if kv.Key == k {
			return kv, true
		}
	}
	return scheduler.KV{}, false
}
