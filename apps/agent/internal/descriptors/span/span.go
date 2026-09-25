// Package span holds the reconciler descriptor for VPP port mirroring (task DF-7, WBS D1.10):
// span.mirror copies the rx, tx or both directions of a source interface to a destination
// interface (sw_interface_span_enable_disable), at the device level or in the L2 path
// (is_l2), retrieved with sw_interface_span_dump for both levels. The destination may be any
// interface, e.g. a GRE/ERSPAN tunnel built by DF-6: this package depends on its interface key
// only.
//
// Messages come only from apps/agent/binapi/span. Values are *structpb.Struct documents of
// Mirror (D-055). Ownership: a mirror belongs to the owner of its source interface.
// docs/agent/descriptors/span.md is the object ↔ message table.
package span

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/span"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// NameMirror is the descriptor name.
const NameMirror = "span.mirror"

// Mirrored directions.
const (
	StateRx   = "rx"
	StateTx   = "tx"
	StateBoth = "both"
)

var states = map[string]span.SpanState{
	StateRx:   span.SPAN_STATE_API_RX,
	StateTx:   span.SPAN_STATE_API_TX,
	StateBoth: span.SPAN_STATE_API_RX_TX,
}

// Mirror is the desired state of one span.mirror object. L2 selects the L2 feature path
// (bridged traffic) instead of the device level.
type Mirror struct {
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination,omitempty"`
	State       string `json:"state,omitempty"`
	L2          bool   `json:"l2,omitempty"`
}

// Validate checks m.
func (m Mirror) Validate() error {
	if m.Source == "" || m.Destination == "" {
		return df7.Specf("span mirror needs source and destination")
	}
	if m.Source == m.Destination {
		return df7.Specf("span mirror source and destination are both %q", m.Source)
	}
	if _, ok := states[m.State]; !ok {
		return df7.Specf("span state %q: want rx, tx or both", m.State)
	}
	return nil
}

func level(l2 bool) string {
	if l2 {
		return "l2"
	}
	return "device"
}

// Key is "span.mirror/<source>/<destination>/<device|l2>".
func Key(src, dst string, l2 bool) scheduler.Key {
	return scheduler.Join(NameMirror, src, dst, level(l2))
}

// Meta holds both interface indexes.
type Meta struct{ From, To uint32 }

// Descriptor manages span.mirror objects.
type Descriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*Descriptor)(nil)

// New returns the span.mirror descriptor.
func New(c vpp.Client, owner string, opts ...df7.Option) *Descriptor {
	return &Descriptor{df7.NewBase(NameMirror, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *Descriptor) KeyOf(obj proto.Message) scheduler.Key {
	m, _ := df7.Decode[Mirror](obj)
	return Key(m.Source, m.Destination, m.L2)
}

// Dependencies implements scheduler.Descriptor: source and destination interfaces.
func (d *Descriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	m, _ := df7.Decode[Mirror](obj)
	return []scheduler.Dependency{d.Opts.IfaceDep(m.Source), d.Opts.IfaceDep(m.Destination)}
}

func (d *Descriptor) set(ctx context.Context, meta Meta, state span.SpanState, l2 bool) error {
	_, err := span.NewServiceClient(d.Client).SwInterfaceSpanEnableDisable(ctx, &span.SwInterfaceSpanEnableDisable{
		SwIfIndexFrom: interface_types.InterfaceIndex(meta.From), SwIfIndexTo: interface_types.InterfaceIndex(meta.To), State: state, IsL2: l2})
	return d.Wrap(fmt.Sprintf("sw_interface_span_enable_disable %d→%d state %d l2=%v", meta.From, meta.To, state, l2), err)
}

// Create implements scheduler.Descriptor.
func (d *Descriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	m, err := df7.DecodeValid[Mirror](obj)
	if err != nil {
		return nil, err
	}
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	tg, err := d.Target(ctx, m.Source, string(Key(m.Source, m.Destination, m.L2)))
	if err != nil {
		return nil, err
	}
	to, err := ifs.Resolve(m.Destination)
	if err != nil {
		return nil, err
	}
	meta := Meta{From: tg.Index, To: to}
	if tg.Untagged {
		// an existing mirror on an untagged source is never adopted (review M1)
		exists, err := d.exists(ctx, meta, m.L2)
		if err != nil {
			return nil, err
		}
		if exists {
			if err := tg.Adopt(); err != nil {
				return nil, err
			}
		}
	}
	undo, err := claimFirst(tg) // TD-11b, review M3: the claim before the VPP write
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, meta, states[m.State], m.L2); err != nil {
		return nil, undo(err)
	}
	return meta, nil
}

// exists reports whether VPP mirrors meta.From to meta.To at the level.
func (d *Descriptor) exists(ctx context.Context, meta Meta, l2 bool) (bool, error) {
	stream, err := span.NewServiceClient(d.Client).SwInterfaceSpanDump(ctx, &span.SwInterfaceSpanDump{IsL2: l2})
	if err != nil {
		return false, d.Wrap("sw_interface_span_dump", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return false, d.Wrap("sw_interface_span_dump", err)
	}
	for _, det := range dets {
		if uint32(det.SwIfIndexFrom) == meta.From && uint32(det.SwIfIndexTo) == meta.To {
			return true, nil
		}
	}
	return false, nil
}

// reresolve finds both interfaces again by logical name (D-071: never trust a stored index) and
// checks the mirror is ours on this VPP instance.
func (d *Descriptor) reresolve(ctx context.Context, m Mirror) (Meta, dfkit.Target, bool, error) {
	tg, found, err := d.Detach(ctx, m.Source, string(Key(m.Source, m.Destination, m.L2)))
	if err != nil || !found {
		return Meta{}, tg, false, err
	}
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return Meta{}, tg, false, err
	}
	to, err := ifs.Resolve(m.Destination)
	if errors.Is(err, df7.ErrNoSuchInterface) {
		// F-loopback-bvi-gso-lldp-span: a destination deleted behind the agent's back leaves the session in
		// VPP's span bookkeeping of OUR source (span.c has no interface-delete hook; V-new), which Retrieve
		// reports under the index spelling "#<sw_if_index>". Clearing that bit touches only our source's state.
		if idx, ok := staleIndex(m.Destination); ok {
			return Meta{From: tg.Index, To: idx}, tg, true, nil
		}
		return Meta{}, tg, false, nil
	}
	if err != nil {
		return Meta{}, tg, false, err
	}
	return Meta{From: tg.Index, To: to}, tg, true, nil
}

// staleIndex parses the "#<sw_if_index>" spelling df7.Interfaces.Name gives an index VPP no longer names.
func staleIndex(name string) (uint32, bool) {
	rest, ok := strings.CutPrefix(name, "#")
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseUint(rest, 10, 32)
	return uint32(n), err == nil
}

// Update implements scheduler.Descriptor: a new direction set is applied in place.
func (d *Descriptor) Update(ctx context.Context, oldObj, newObj proto.Message, _ any) (any, error) {
	o, err := df7.Decode[Mirror](oldObj)
	if err != nil {
		return nil, err
	}
	n, err := df7.DecodeValid[Mirror](newObj)
	if err != nil {
		return nil, err
	}
	if o.Source != n.Source || o.Destination != n.Destination || o.L2 != n.L2 {
		return nil, scheduler.ErrRecreate
	}
	m, _, found, err := d.reresolve(ctx, n)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s: %w: %s or %s", NameMirror, df7.ErrNoSuchInterface, n.Source, n.Destination)
	}
	return m, d.set(ctx, m, states[n.State], n.L2)
}

// Delete implements scheduler.Descriptor: re-resolve both interfaces and set state disabled;
// a vanished interface is success.
func (d *Descriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	o, err := df7.Decode[Mirror](obj)
	if err != nil {
		return err
	}
	m, tg, found, err := d.reresolve(ctx, o)
	if err != nil || !found {
		return err
	}
	if err := d.set(ctx, m, span.SPAN_STATE_API_DISABLED, o.L2); err != nil {
		return err
	}
	return tg.Release()
}

// Retrieve implements scheduler.Descriptor: sw_interface_span_dump for the device and the L2
// level; mirrors whose source interface is owned.
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, l2 := range []bool{false, true} {
		stream, err := span.NewServiceClient(d.Client).SwInterfaceSpanDump(ctx, &span.SwInterfaceSpanDump{IsL2: l2})
		if err != nil {
			return nil, d.Wrap("sw_interface_span_dump", err)
		}
		dets, err := df7.Collect(stream.Recv)
		if err != nil {
			return nil, d.Wrap("sw_interface_span_dump", err)
		}
		for _, det := range dets {
			dst := ifs.Name(uint32(det.SwIfIndexTo))
			src, ok := ifs.Owned(uint32(det.SwIfIndexFrom), func(n string) string { return string(Key(n, dst, l2)) })
			if !ok {
				continue
			}
			state := fmt.Sprintf("#%d", det.State)
			for n, s := range states {
				if s == det.State {
					state = n
				}
			}
			m := Mirror{Source: src, Destination: dst, State: state, L2: l2}
			out = append(out, df7.KV(Key(m.Source, m.Destination, l2), m, Meta{From: uint32(det.SwIfIndexFrom), To: uint32(det.SwIfIndexTo)}))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Register registers the span descriptor.
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df7.Option) {
	r.Register(New(c, owner, opts...))
}

// claimFirst records the claim on an untagged target before the VPP write and returns undo, which releases the claim
// again when this Create made it (TD-11b's dfkit.Target.ClaimFirst, which this branch's base predates; the swap is
// mechanical at the rebase — review M3). Our tagged interfaces need no claim.
func claimFirst(tg dfkit.Target) (undo func(error) error, err error) {
	had := tg.Claimed()
	if err := tg.Claim(); err != nil {
		return nil, err
	}
	return func(err error) error {
		if had {
			return err
		}
		if rerr := tg.Release(); rerr != nil {
			return errors.Join(err, rerr)
		}
		return err
	}, nil
}
