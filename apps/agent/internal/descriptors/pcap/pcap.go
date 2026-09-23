// Package pcap holds the reconciler descriptors for VPP's built-in pcap dispatch capture (vnet
// interface.api, task DF-8, WBS D8.2): the capture itself and the filter function it uses.
// Message names come only from apps/agent/binapi/interface; docs/agent/descriptors/pcap.md is
// the object ↔ message table.
//
// Both objects are VPP-global singletons without a getter: Retrieve returns
// ErrRetrieveUnsupported (write-only, D-063). There is exactly one capture per VPP: Create fails
// with ErrCaptureBusy when a capture this process did not start is running, and Delete stops
// only a capture this process started ("clear only what you started").
//
// The capture file: VPP accepts a bare file name and writes /tmp/<name> (unformat_vlib_tmpfile
// rejects "/" and ".."), so files cannot live under /run/vrx-test/<prefix>/; tests use
// "<prefix>-….pcap" and remove the file.
package pcap

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameCapture        = "pcap.capture"
	NameFilterFunction = "pcap.filter-function"
)

// AnyInterface is the Interface value capturing on every interface (sw_if_index 0 in
// pcap_trace_on means "any", it does not refer to local0).
const AnyInterface = "any"

// FileDir is where VPP writes capture files.
const FileDir = "/tmp"

// ErrCaptureBusy means a pcap capture this process did not start is running (one per VPP).
var ErrCaptureBusy = errors.New("a pcap capture is already running on this VPP")

// Capture is the pcap.capture singleton (pcap_trace_on / pcap_trace_off): what to capture (rx,
// tx, drop — at least one), on which interface ("any" or an owned interface), how much (packets,
// bytes per packet 32..9000), whether the classifier/BPF filter applies, an optional drop error
// ("node/error") and the file name under /tmp.
type Capture struct {
	Rx                bool   `json:"rx"`
	Tx                bool   `json:"tx"`
	Drop              bool   `json:"drop"`
	Interface         string `json:"interface"`
	MaxPackets        uint32 `json:"max_packets"`
	MaxBytesPerPacket uint32 `json:"max_bytes_per_packet"`
	Filter            bool   `json:"filter"`
	Error             string `json:"error"`
	File              string `json:"file"`
}

// FilterFunction is the pcap.filter-function singleton (pcap_set_filter_function): the trace
// filter function pcap's Filter flag uses — "vnet_is_packet_traced" (classifier, VPP default) or
// "bpf_trace_filter" (trace.bpf-filter).
type FilterFunction struct {
	Name string `json:"name"`
}

// DefaultFilterFunction is VPP's default pcap filter function.
const DefaultFilterFunction = "vnet_is_packet_traced"

// Proto returns the canonical structpb document.
func (s Capture) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s FilterFunction) Proto() *structpb.Struct { return dfkit.Encode(s) }

func checkChars(what, v string, maxLen int, extra string) error {
	if len(v) > maxLen {
		return dfkit.Specf("%s %q longer than %d bytes", what, v, maxLen)
	}
	for _, r := range v {
		ok := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '-' || r == '_' || r == '.'
		for _, e := range extra {
			ok = ok || r == e
		}
		if !ok {
			return dfkit.Specf("%s %q: invalid character %q", what, v, r)
		}
	}
	return nil
}

// Validate checks what VPP rejects or would misread.
func (s Capture) Validate() error {
	switch {
	case !s.Rx && !s.Tx && !s.Drop:
		return dfkit.Specf("pcap capture: enable at least one of rx, tx, drop")
	case s.Interface == "":
		return dfkit.Specf("pcap capture: interface is empty (use %q)", AnyInterface)
	case s.MaxPackets == 0:
		return dfkit.Specf("pcap capture: max_packets must be > 0")
	case s.MaxBytesPerPacket < 32 || s.MaxBytesPerPacket > 9000:
		return dfkit.Specf("pcap capture: max_bytes_per_packet %d outside 32..9000", s.MaxBytesPerPacket)
	case s.File == "" || s.File == "." || s.File == "..":
		return dfkit.Specf("pcap capture: file name is required")
	}
	if err := checkChars("file", s.File, 63, ""); err != nil {
		return err
	}
	return checkChars("error", s.Error, 127, "/")
}

// Validate checks the function name.
func (s FilterFunction) Validate() error {
	if s.Name == "" {
		return dfkit.Specf("pcap filter function: name is empty")
	}
	return checkChars("filter function", s.Name, 63, "")
}

// Register constructs and registers the pcap descriptors.
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...Option) {
	r.Register(NewFilterFunction(client, opts...))
	r.Register(NewCapture(client, owner, opts...))
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

// WithGlobals sets the D-071 role for the VPP-global pcap.filter-function (default: not the
// globals owner — VPP has no getter, so a non-owner's Create fails with ErrNotGlobalsOwner).
func WithGlobals(g dfkit.Globals) Option { return func(o *options) { o.globals = g } }

// WithInterfaceKey sets the interface key scheme of Dependencies (default "interface/<name>", D-065).
func WithInterfaceKey(f dfkit.KeyFunc) Option {
	return func(o *options) {
		if f != nil {
			o.ifaceKey = f
		}
	}
}

// ---- pcap.capture ---------------------------------------------------------------------------

// CaptureID is the object id of the singleton (key pcap.capture/global).
const CaptureID = "global"

// KeyCapture is the key of the singleton.
var KeyCapture = scheduler.Join(NameCapture, CaptureID)

// CaptureDescriptor manages the pcap.capture singleton (see the package doc).
type CaptureDescriptor struct {
	client vpp.Client
	owner  string
	o      options

	mu      sync.Mutex
	started *Capture // the capture this process started (nil: none)
}

var _ scheduler.Descriptor = (*CaptureDescriptor)(nil)

// NewCapture returns the pcap.capture descriptor.
func NewCapture(client vpp.Client, owner string, opts ...Option) *CaptureDescriptor {
	return &CaptureDescriptor{client: client, owner: owner, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*CaptureDescriptor) Name() string { return NameCapture }

// KeyOf implements scheduler.Descriptor.
func (*CaptureDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyCapture }

// Dependencies implements scheduler.Descriptor: the interface when not "any", and the filter
// function (optional).
func (d *CaptureDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	var s Capture
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil
	}
	deps := []scheduler.Dependency{{Key: KeyFilterFunction, Optional: true}}
	if s.Interface != AnyInterface && s.Interface != "" {
		deps = append(deps, scheduler.Dependency{Key: d.o.ifaceKey(s.Interface)})
	}
	return deps
}

// Create implements scheduler.Descriptor. VPP refuses a second capture (INVALID_VALUE): that is
// success when it is this process's identical capture (write-only re-apply), ErrCaptureBusy
// otherwise.
func (d *CaptureDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	var s Capture
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	var idx uint32 // 0 = any
	if s.Interface != AnyInterface {
		var err error
		if idx, err = dfkit.ResolveInterface(ctx, d.client, s.Interface, d.owner); err != nil {
			return nil, err
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := interfaces.NewServiceClient(d.client).PcapTraceOn(ctx, &interfaces.PcapTraceOn{
		CaptureRx: s.Rx, CaptureTx: s.Tx, CaptureDrop: s.Drop, Filter: s.Filter,
		MaxPackets: s.MaxPackets, MaxBytesPerPacket: s.MaxBytesPerPacket,
		SwIfIndex: interface_types.InterfaceIndex(idx), Error: s.Error, Filename: s.File,
	})
	if dfkit.IsVPPError(err, api.INVALID_VALUE, api.INVALID_VALUE_2) {
		if d.started != nil && *d.started == s {
			return nil, nil
		}
		return nil, fmt.Errorf("pcap_trace_on: %w (%v)", ErrCaptureBusy, err)
	}
	if err != nil {
		return nil, fmt.Errorf("pcap_trace_on(%s): %w", s.File, err)
	}
	d.started = &s
	return nil, nil
}

// Update implements scheduler.Descriptor: a running capture cannot be changed — recreate.
func (*CaptureDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: pcap_trace_off (VPP writes the file if packets were
// captured) — only when this process started the running capture.
func (d *CaptureDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started == nil {
		return nil
	}
	_, err := interfaces.NewServiceClient(d.client).PcapTraceOff(ctx, &interfaces.PcapTraceOff{})
	// VALUE_EXIST: nothing was running; NO_SUCH_ENTRY: stopped, but no packet was captured (VPP
	// writes no file then). Both leave the capture off.
	if err != nil && !dfkit.IsVPPError(err, api.VALUE_EXIST, api.NO_SUCH_ENTRY) {
		return fmt.Errorf("pcap_trace_off: %w", err)
	}
	d.started = nil
	return nil
}

// Retrieve implements scheduler.Descriptor: VPP has no pcap status message (write-only, D-063).
func (*CaptureDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(NameCapture)
}

// ---- pcap.filter-function -------------------------------------------------------------------

// FilterFunctionID is the object id of the singleton (key pcap.filter-function/global).
const FilterFunctionID = "global"

// KeyFilterFunction is the key of the singleton.
var KeyFilterFunction = scheduler.Join(NameFilterFunction, FilterFunctionID)

// FilterFunctionDescriptor manages the pcap.filter-function singleton; Delete restores
// DefaultFilterFunction.
type FilterFunctionDescriptor struct {
	client vpp.Client
	o      options
}

var _ scheduler.Descriptor = (*FilterFunctionDescriptor)(nil)

// NewFilterFunction returns the pcap.filter-function descriptor.
func NewFilterFunction(client vpp.Client, opts ...Option) *FilterFunctionDescriptor {
	return &FilterFunctionDescriptor{client: client, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*FilterFunctionDescriptor) Name() string { return NameFilterFunction }

// KeyOf implements scheduler.Descriptor.
func (*FilterFunctionDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyFilterFunction }

// Dependencies implements scheduler.Descriptor: the BPF filter when it is the BPF function
// (optional: only orders the plan).
func (*FilterFunctionDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	var s FilterFunction
	if err := dfkit.Decode(obj, &s); err != nil || s.Name != "bpf_trace_filter" {
		return nil
	}
	return []scheduler.Dependency{{Key: "trace.bpf-filter/global", Optional: true}}
}

func (d *FilterFunctionDescriptor) set(ctx context.Context, name string) error {
	_, err := interfaces.NewServiceClient(d.client).PcapSetFilterFunction(ctx, &interfaces.PcapSetFilterFunction{FilterFunctionName: name})
	if err != nil {
		return fmt.Errorf("pcap_set_filter_function(%s): %w", name, err)
	}
	return nil
}

func (d *FilterFunctionDescriptor) apply(ctx context.Context, obj proto.Message) error {
	var s FilterFunction
	if err := dfkit.Decode(obj, &s); err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	if !d.o.globals.Owner() {
		return d.o.globals.Require(ctx, NameFilterFunction, obj, nil)
	}
	return d.set(ctx, s.Name)
}

// Create implements scheduler.Descriptor (idempotent; an unknown name fails with VPP's -1).
func (d *FilterFunctionDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.apply(ctx, obj)
}

// Update implements scheduler.Descriptor.
func (d *FilterFunctionDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.apply(ctx, newObj)
}

// Delete implements scheduler.Descriptor: back to the classifier function (globals owner only).
func (d *FilterFunctionDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	if !d.o.globals.Owner() {
		return nil
	}
	return d.set(ctx, DefaultFilterFunction)
}

// Retrieve implements scheduler.Descriptor: no getter (write-only, D-063).
func (*FilterFunctionDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(NameFilterFunction)
}
