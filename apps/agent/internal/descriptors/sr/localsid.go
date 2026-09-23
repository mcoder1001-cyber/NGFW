package sr

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	srapi "ngfw/agent/binapi/sr"
	"ngfw/agent/binapi/sr_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Plugin names the VPP component in ErrPluginNotLoaded (SRv6 is part of vnet).
const Plugin = "sr"

// LocalSidName is the descriptor name; keys are "sr.localsid/<sid>".
const LocalSidName = "sr.localsid"

// LocalSidDescriptor manages SRv6 local SIDs. Local SIDs carry no owner tag; on a shared
// VPP the Scope's address blocks attribute them (production: nil scope, all are ours).
type LocalSidDescriptor struct {
	client vpp.Client
	scope  *df6.Scope
}

// NewLocalSid returns the descriptor.
func NewLocalSid(c vpp.Client, scope *df6.Scope) *LocalSidDescriptor {
	return &LocalSidDescriptor{client: c, scope: scope}
}

// Name implements scheduler.Descriptor.
func (d *LocalSidDescriptor) Name() string { return LocalSidName }

func needsInterface(b Behavior) bool {
	return b == Behavior_END_X || b == Behavior_END_DX2 || b == Behavior_END_DX4 || b == Behavior_END_DX6
}

func needsLookup(b Behavior) bool {
	return b == Behavior_END_T || b == Behavior_END_DT4 || b == Behavior_END_DT6
}

// canonLocalSid validates l and returns its canonical form.
func canonLocalSid(l *LocalSid) (*LocalSid, error) {
	sid, err := df6.ParseAddr6(l.GetSid())
	if err != nil {
		return nil, err
	}
	b := l.GetBehavior()
	if _, ok := Behavior_name[int32(b)]; !ok || b == Behavior_BEHAVIOR_UNSPECIFIED {
		return nil, fmt.Errorf("%w: behavior %d", df6.ErrBadValue, b)
	}
	c := proto.Clone(l).(*LocalSid)
	c.Sid = sid.String()
	if needsInterface(b) != (l.GetInterface() != "") {
		return nil, fmt.Errorf("%w: interface is mandatory for END_X/END_DX* and forbidden otherwise", df6.ErrBadValue)
	}
	if !needsLookup(b) && l.GetLookupTable() != 0 {
		return nil, fmt.Errorf("%w: lookup_table only for END_T/END_DT*", df6.ErrBadValue)
	}
	switch b {
	case Behavior_END_X, Behavior_END_DX6:
		nh, err := df6.ParseAddr6(l.GetNextHop())
		if err != nil {
			return nil, err
		}
		c.NextHop = nh.String()
	case Behavior_END_DX4:
		nh, err := df6.ParseAddr4(l.GetNextHop())
		if err != nil {
			return nil, err
		}
		c.NextHop = nh.String()
	default:
		if l.GetNextHop() != "" {
			return nil, fmt.Errorf("%w: next_hop only for END_X/END_DX4/END_DX6", df6.ErrBadValue)
		}
	}
	if l.GetEndPsp() && b != Behavior_END && b != Behavior_END_X && b != Behavior_END_T {
		return nil, fmt.Errorf("%w: end_psp only for END/END_X/END_T", df6.ErrBadValue)
	}
	return c, nil
}

func (d *LocalSidDescriptor) cast(obj proto.Message) (*LocalSid, error) {
	l, ok := obj.(*LocalSid)
	if !ok {
		return nil, fmt.Errorf("%s: %w: %T", LocalSidName, df6.ErrBadValue, obj)
	}
	c, err := canonLocalSid(l)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", LocalSidName, err)
	}
	return c, nil
}

// KeyOf implements scheduler.Descriptor.
func (d *LocalSidDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	l, err := d.cast(obj)
	if err != nil {
		return scheduler.Join(LocalSidName, "invalid")
	}
	return scheduler.Join(LocalSidName, l.GetSid())
}

// Dependencies implements scheduler.Descriptor: the SID's table, the lookup table of
// END_T/END_DT*, the cross-connect interface of END_X/END_DX*.
func (d *LocalSidDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	l, err := d.cast(obj)
	if err != nil {
		return nil
	}
	deps := df6.VRFDeps(l.GetFibTable())
	if l.GetLookupTable() != l.GetFibTable() {
		deps = append(deps, df6.VRFDeps(l.GetLookupTable())...)
	}
	return append(deps, df6.InterfaceDeps(l.GetInterface())...)
}

func (d *LocalSidDescriptor) request(ctx context.Context, l *LocalSid, isDel bool) (*srapi.SrLocalsidAddDel, error) {
	sid, err := df6.IP6Of(l.GetSid())
	if err != nil {
		return nil, err
	}
	nh, err := df6.AddressOf(l.GetNextHop())
	if err != nil {
		return nil, err
	}
	req := &srapi.SrLocalsidAddDel{
		IsDel:     isDel,
		Localsid:  sid,
		EndPsp:    l.GetEndPsp(),
		Behavior:  sr_types.SrBehavior(l.GetBehavior()), //nolint:gosec // validated enum
		SwIfIndex: interface_types.InterfaceIndex(df6.NoInterface),
		FibTable:  l.GetFibTable(),
		NhAddr:    nh,
	}
	switch {
	case needsInterface(l.GetBehavior()):
		ifs, err := df6.DumpInterfaces(ctx, d.client, "")
		if err != nil {
			return nil, err
		}
		idx, err := ifs.Index(l.GetInterface())
		if err != nil {
			return nil, err
		}
		req.SwIfIndex = idx
	case needsLookup(l.GetBehavior()):
		// END_T / END_DT* carry the lookup table id in sw_if_index (as the CLI does).
		req.SwIfIndex = interface_types.InterfaceIndex(l.GetLookupTable())
	}
	return req, nil
}

// requireTables: VPP resolves the tables with fib_table_find and does not check the result
// for the lookup table (and crashes on a missing SID table at delete), so check first.
func (d *LocalSidDescriptor) requireTables(ctx context.Context, l *LocalSid) error {
	if err := df6.RequireTable(ctx, d.client, l.GetFibTable(), true); err != nil {
		return err
	}
	if needsLookup(l.GetBehavior()) {
		return df6.RequireTable(ctx, d.client, l.GetLookupTable(), l.GetBehavior() != Behavior_END_DT4)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *LocalSidDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	l, err := d.cast(obj)
	if err != nil {
		return nil, err
	}
	if err := d.requireTables(ctx, l); err != nil {
		return nil, fmt.Errorf("%s: %w", LocalSidName, err)
	}
	req, err := d.request(ctx, l, false)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", LocalSidName, err)
	}
	if _, err := srapi.NewServiceClient(d.client).SrLocalsidAddDel(ctx, req); err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("%s: sr_localsid_add_del: %w", LocalSidName, err))
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: every change recreates.
func (d *LocalSidDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor. A SID VPP no longer has is already deleted; the
// SID's table must still exist (VPP would index the FIB with ~0 otherwise).
func (d *LocalSidDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	l, err := d.cast(obj)
	if err != nil {
		return err
	}
	all, err := d.dump(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, r := range all {
		if df6.IP6String(r.Addr) == l.GetSid() && r.FibTable == l.GetFibTable() {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	if err := df6.RequireTable(ctx, d.client, l.GetFibTable(), true); err != nil {
		return fmt.Errorf("%s: %w", LocalSidName, err)
	}
	req, err := d.request(ctx, l, true)
	if err != nil {
		// The cross-connect interface may be gone already; the delete key is (sid, table).
		req = &srapi.SrLocalsidAddDel{IsDel: true, SwIfIndex: interface_types.InterfaceIndex(df6.NoInterface), FibTable: l.GetFibTable()}
		req.Localsid, _ = df6.IP6Of(l.GetSid())
	}
	if _, err := srapi.NewServiceClient(d.client).SrLocalsidAddDel(ctx, req); err != nil {
		return df6.PluginError(Plugin, fmt.Errorf("%s: sr_localsid_add_del (del): %w", LocalSidName, err))
	}
	return nil
}

func (d *LocalSidDescriptor) dump(ctx context.Context) ([]*srapi.SrLocalsidsDetails, error) {
	stream, err := srapi.NewServiceClient(d.client).SrLocalsidsDump(ctx, &srapi.SrLocalsidsDump{})
	if err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("%s: sr_localsids_dump: %w", LocalSidName, err))
	}
	recs, err := df6.Collect(stream.Recv)
	if err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("%s: sr_localsids_dump: %w", LocalSidName, err))
	}
	return recs, nil
}

// Retrieve implements scheduler.Descriptor: every classic-behaviour SID in scope. SIDs of
// uSID or plugin behaviours (srv6-ad/am/as, srv6-mobile) are not this descriptor's.
func (d *LocalSidDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	recs, err := d.dump(ctx)
	if err != nil {
		return nil, err
	}
	ifs, err := df6.DumpInterfaces(ctx, d.client, d.scope.OwnerOf())
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, r := range recs {
		b := Behavior(r.Behavior)
		if _, ok := Behavior_name[int32(b)]; !ok || b == Behavior_BEHAVIOR_UNSPECIFIED {
			continue
		}
		sid := df6.IP6String(r.Addr)
		if sid == "" || !d.scope.OwnsAddrString(sid) {
			continue
		}
		l := &LocalSid{Sid: sid, Behavior: b, EndPsp: r.EndPsp, FibTable: r.FibTable}
		switch {
		case needsInterface(b):
			l.Interface = ifs.NameOrEmpty(r.XconnectIfaceOrVrfTable)
		case needsLookup(b):
			l.LookupTable = r.XconnectIfaceOrVrfTable
		}
		if b == Behavior_END_X || b == Behavior_END_DX4 || b == Behavior_END_DX6 {
			l.NextHop = df6.AddressString(r.XconnectNhAddr)
		}
		out = append(out, scheduler.KV{Key: scheduler.Join(LocalSidName, sid), Value: l})
	}
	return out, nil
}
