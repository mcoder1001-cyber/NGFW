// Package sessionredirect implements the descriptor of VPP's ip_session_redirect plugin
// (D2.7): forwarding the sessions that match a classifier entry over a path list. Messages
// come from apps/agent/binapi/ip_session_redirect only; classifier tables are resolved
// through the classify package's Store.
package sessionredirect

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/fib_types"
	isr "ngfw/agent/binapi/ip_session_redirect"
	"ngfw/agent/internal/descriptors/classify"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Name is the descriptor name; keys are "ip-session-redirect.redirect/<table>/<hex(match)>".
const Name = "ip-session-redirect.redirect"

// MaxMatch is the fixed size of the match field in the ip_session_redirect API.
const MaxMatch = 80

// Descriptor manages session redirects (ip_session_redirect_add_v2 / _del).
type Descriptor struct {
	client vpp.Client
	owner  string
	store  classify.Store
}

// New returns the descriptor; store is the classify table store of the same owner.
func New(c vpp.Client, owner string, store classify.Store) *Descriptor {
	return &Descriptor{client: c, owner: owner, store: store}
}

// Meta is the runtime handle: the classifier table index.
type Meta struct{ TableIndex uint32 }

// Normalize returns r in the form Retrieve produces: trimmed match, canonical paths.
func Normalize(r *Redirect) (*Redirect, error) {
	n := proto.Clone(r).(*Redirect)
	n.Match = classify.TrimMatch(n.Match)
	paths, err := df2.NormalizePaths(n.Paths)
	if err != nil {
		return nil, err
	}
	n.Paths = paths
	return n, nil
}

// Name implements scheduler.Descriptor.
func (*Descriptor) Name() string { return Name }

// KeyOf implements scheduler.Descriptor.
func (*Descriptor) KeyOf(obj proto.Message) scheduler.Key {
	r := obj.(*Redirect)
	return scheduler.Join(Name, r.GetTable(), classify.MatchID(r.GetMatch()))
}

// Dependencies implements scheduler.Descriptor: the classifier table (mandatory) and the
// path interfaces (ordering only).
func (*Descriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	r := obj.(*Redirect)
	deps := []scheduler.Dependency{{Key: classify.TableKey(r.GetTable())}}
	for _, p := range r.GetPaths() {
		if p.GetInterface() != "" {
			deps = append(deps, scheduler.Dependency{Key: df2.InterfaceKey(p.GetInterface()), Optional: true})
		}
	}
	return deps
}

func (d *Descriptor) table(name string) (classify.TableRecord, error) {
	rec, ok := d.store.Get(name)
	if !ok {
		return classify.TableRecord{}, fmt.Errorf("%w: %q", classify.ErrNoSuchTable, name)
	}
	return rec, nil
}

func (d *Descriptor) add(ctx context.Context, r *Redirect, rec classify.TableRecord) error {
	if len(r.GetPaths()) == 0 {
		return fmt.Errorf("%s: at least one path is required", Name)
	}
	match, err := classify.PadMatch(r.GetMatch(), rec)
	if err != nil {
		return fmt.Errorf("%s: %w", Name, err)
	}
	if len(match) > MaxMatch {
		return fmt.Errorf("%s: table geometry needs %d match bytes, the API carries at most %d", Name, len(match), MaxMatch)
	}
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return err
	}
	paths, err := df2.EncodePaths(r.GetPaths(), ifs)
	if err != nil {
		return err
	}
	if len(paths) > 255 {
		return fmt.Errorf("%s: %d paths exceed 255", Name, len(paths))
	}
	nhProto := fib_types.FIB_API_PATH_NH_PROTO_IP4
	if r.GetIpv6() {
		nhProto = fib_types.FIB_API_PATH_NH_PROTO_IP6
	}
	req := &isr.IPSessionRedirectAddV2{
		TableIndex:  rec.Index,
		OpaqueIndex: r.GetOpaqueIndex(),
		Proto:       nhProto,
		IsPunt:      r.GetPunt(),
		MatchLen:    uint8(len(match)), //nolint:gosec // ≤ 80
		Match:       match,
		NPaths:      uint8(len(paths)), //nolint:gosec // checked
		Paths:       paths,
	}
	if _, err := isr.NewServiceClient(d.client).IPSessionRedirectAddV2(ctx, req); err != nil {
		return fmt.Errorf("ip_session_redirect_add_v2: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *Descriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	r, err := Normalize(obj.(*Redirect))
	if err != nil {
		return nil, err
	}
	rec, err := d.table(r.GetTable())
	if err != nil {
		return nil, err
	}
	if err := d.add(ctx, r, rec); err != nil {
		return nil, err
	}
	return Meta{TableIndex: rec.Index}, nil
}

// Update implements scheduler.Descriptor: VPP rejects re-adding an existing session with
// different contents (ip_session_redirect_add_v2 returned -52 on vrx-a), so every change is
// a delete + add by the scheduler.
func (*Descriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *Descriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(Meta)
	if !ok {
		return fmt.Errorf("%s: %w %T", Name, df2.ErrBadMeta, meta)
	}
	r := obj.(*Redirect)
	rec, err := d.table(r.GetTable())
	if err != nil {
		return err
	}
	match, err := classify.PadMatch(r.GetMatch(), rec)
	if err != nil {
		return fmt.Errorf("%s: %w", Name, err)
	}
	req := &isr.IPSessionRedirectDel{TableIndex: m.TableIndex, MatchLen: uint8(len(match)), Match: match} //nolint:gosec // ≤ 80
	if _, err := isr.NewServiceClient(d.client).IPSessionRedirectDel(ctx, req); err != nil {
		return fmt.Errorf("ip_session_redirect_del: %w", err)
	}
	return nil
}

// Retrieve dumps the redirects of every owned classifier table (ip_session_redirect_dump).
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	recs, err := classify.LiveTables(ctx, d.client, d.store)
	if err != nil {
		return nil, err
	}
	svc := isr.NewServiceClient(d.client)
	var out []scheduler.KV
	for _, rec := range recs {
		stream, err := svc.IPSessionRedirectDump(ctx, &isr.IPSessionRedirectDump{TableIndex: rec.Index})
		if err != nil {
			return nil, fmt.Errorf("ip_session_redirect_dump %d: %w", rec.Index, err)
		}
		details, err := df2.Collect(stream.Recv)
		if err != nil {
			return nil, fmt.Errorf("ip_session_redirect_dump %d: %w", rec.Index, err)
		}
		for _, det := range details {
			if det.TableIndex != rec.Index {
				continue
			}
			n := int(det.MatchLength)
			if n > len(det.Match) {
				n = len(det.Match)
			}
			v := &Redirect{
				Table:       rec.Name,
				Match:       classify.TrimMatch(det.Match[:n]),
				OpaqueIndex: det.OpaqueIndex,
				Punt:        det.IsPunt,
				Ipv6:        det.IsIP6,
				Paths:       df2.DecodePaths(det.Paths, ifs),
			}
			out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: Meta{TableIndex: rec.Index}})
		}
	}
	return out, nil
}

// Register registers the descriptor with r; store is the classify Store of the same owner.
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, c vpp.Client, owner string, store classify.Store) {
	r.Register(New(c, owner, store))
}
