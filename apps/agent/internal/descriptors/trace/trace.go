// Package trace holds the reconciler descriptor for VPP's BPF trace filter (bpf_trace_filter
// plugin, task DF-8, WBS D8.2): a pcap-filter expression VPP compiles with libpcap and that the
// packet tracer and pcap capture use when their filter function is "bpf_trace_filter"
// (pcap.filter-function). Message names come only from apps/agent/binapi/bpf_trace_filter;
// docs/agent/descriptors/trace.md is the object ↔ message table and lists what VPP 26.06 on
// this host does not offer (tracedump's trace_set_filters / trace_v2_dump and tracenode are not
// built, Trace Path has no API).
package trace

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/binapi/bpf_trace_filter"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// NameBPFFilter is the descriptor name.
const NameBPFFilter = "trace.bpf-filter"

// Plugin is the plugin name used in ErrPluginNotLoaded.
const Plugin = "bpf_trace_filter"

// MaxExpressionLen bounds the filter expression (user input; libpcap compiles it inside VPP).
const MaxExpressionLen = 1024

// BPFFilter is the trace.bpf-filter singleton: a pcap-filter(7) expression and whether libpcap
// optimises the compiled program.
type BPFFilter struct {
	Expression string `json:"expression"`
	Optimize   bool   `json:"optimize"`
}

// Proto returns the canonical structpb document.
func (s BPFFilter) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Validate bounds the expression and restricts it to the pcap-filter(7) alphabet: letters,
// digits, blanks and . : / [ ] ( ) & | ! = < > - + * % ^ ~ _ , — no quotes, backslashes, ";",
// "$" or backticks. It never reaches a shell; the restriction keeps VPP's parser input sane.
func (s BPFFilter) Validate() error {
	if s.Expression == "" || len(s.Expression) > MaxExpressionLen {
		return dfkit.Specf("bpf filter: expression length must be 1..%d", MaxExpressionLen)
	}
	for _, r := range s.Expression {
		ok := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		switch r {
		case ' ', '.', ':', '/', '[', ']', '(', ')', '&', '|', '!', '=', '<', '>', '-', '+', '*', '%', '^', '~', '_', ',':
			ok = true
		}
		if !ok {
			return dfkit.Specf("bpf filter: invalid character %q in expression", r)
		}
	}
	return nil
}

// Register constructs and registers the trace descriptors.
func Register(r scheduler.Registry, client vpp.Client) {
	r.Register(NewBPFFilter(client))
}

// BPFFilterID is the object id of the singleton (key trace.bpf-filter/global).
const BPFFilterID = "global"

// KeyBPFFilter is the key of the singleton.
var KeyBPFFilter = scheduler.Join(NameBPFFilter, BPFFilterID)

// BPFFilterDescriptor manages the trace.bpf-filter singleton (bpf_trace_filter_set_v2). VPP has
// no getter (`show bpf trace filter` is CLI only): Retrieve returns ErrRetrieveUnsupported
// (write-only, D-063). Create replaces any program; Delete removes it (is_add=0). VPP frees the
// old program before compiling the new one, so a Create that fails to compile leaves no filter
// at all (the reconciler's rollback re-applies the previous value).
type BPFFilterDescriptor struct{ client vpp.Client }

var _ scheduler.Descriptor = (*BPFFilterDescriptor)(nil)

// NewBPFFilter returns the trace.bpf-filter descriptor.
func NewBPFFilter(client vpp.Client) *BPFFilterDescriptor {
	return &BPFFilterDescriptor{client: client}
}

// Name implements scheduler.Descriptor.
func (*BPFFilterDescriptor) Name() string { return NameBPFFilter }

// KeyOf implements scheduler.Descriptor.
func (*BPFFilterDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyBPFFilter }

// Dependencies implements scheduler.Descriptor: none.
func (*BPFFilterDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *BPFFilterDescriptor) apply(ctx context.Context, obj proto.Message) error {
	var s BPFFilter
	if err := dfkit.Decode(obj, &s); err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	_, err := bpf_trace_filter.NewServiceClient(d.client).BpfTraceFilterSetV2(ctx, &bpf_trace_filter.BpfTraceFilterSetV2{
		IsAdd: true, Optimize: s.Optimize, Filter: s.Expression,
	})
	if err != nil {
		return fmt.Errorf("bpf_trace_filter_set_v2 (VPP answers -1 when libpcap cannot compile the expression): %w", dfkit.PluginError(Plugin, err))
	}
	return nil
}

// Create implements scheduler.Descriptor (idempotent: VPP recompiles and replaces).
func (d *BPFFilterDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.apply(ctx, obj)
}

// Update implements scheduler.Descriptor (replaces the program in place).
func (d *BPFFilterDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.apply(ctx, newObj)
}

// Delete implements scheduler.Descriptor: removes the program (a no-op when none is set).
func (d *BPFFilterDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	_, err := bpf_trace_filter.NewServiceClient(d.client).BpfTraceFilterSetV2(ctx, &bpf_trace_filter.BpfTraceFilterSetV2{IsAdd: false})
	if err != nil {
		return fmt.Errorf("bpf_trace_filter_set_v2(del): %w", dfkit.PluginError(Plugin, err))
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: no getter (write-only, D-063).
func (*BPFFilterDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(NameBPFFilter)
}
