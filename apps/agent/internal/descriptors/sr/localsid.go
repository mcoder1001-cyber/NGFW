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

// LocalSidDescriptor manages SRv6 local SIDs. Local SIDs carry no owner tag; a SID is ours
// only through the claim written by our own Create (D-071, df6.KeyedDescriptor).
type LocalSidDescriptor = df6.KeyedDescriptor[*LocalSid]

// NewLocalSid returns the descriptor.
func NewLocalSid(c vpp.Client, owner string, opts ...df6.Option) *LocalSidDescriptor {
	return df6.NewKeyedDescriptor(localSidSpec(owner), c, owner, opts...)
}

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

// requireTables: VPP resolves the tables with fib_table_find and does not check the result
// for the lookup table (and crashes on a missing SID table at delete), so check first.
func requireTables(ctx context.Context, c vpp.Client, l *LocalSid) error {
	if err := df6.RequireTable(ctx, c, l.GetFibTable(), true); err != nil {
		return err
	}
	if needsLookup(l.GetBehavior()) {
		return df6.RequireTable(ctx, c, l.GetLookupTable(), l.GetBehavior() != Behavior_END_DT4)
	}
	return nil
}

func localSidRequest(ctx context.Context, c vpp.Client, owner string, l *LocalSid, isDel bool) (*srapi.SrLocalsidAddDel, error) {
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
		ifs, err := df6.DumpInterfaces(ctx, c, owner)
		if err != nil {
			return nil, err
		}
		idx, err := ifs.Index(l.GetInterface()) // logical name; foreign interfaces refused
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

func localSidSpec(owner string) df6.KeyedSpec[*LocalSid] {
	return df6.KeyedSpec[*LocalSid]{
		Name:   LocalSidName,
		Plugin: Plugin,
		Canon:  canonLocalSid,
		ID:     func(l *LocalSid) string { return l.GetSid() },
		Deps: func(l *LocalSid) []scheduler.Dependency {
			deps := df6.VRFDeps(l.GetFibTable())
			if l.GetLookupTable() != l.GetFibTable() {
				deps = append(deps, df6.VRFDeps(l.GetLookupTable())...)
			}
			return append(deps, df6.InterfaceDeps(l.GetInterface())...)
		},
		Add: func(ctx context.Context, c vpp.Client, l *LocalSid) error {
			if err := requireTables(ctx, c, l); err != nil {
				return err
			}
			req, err := localSidRequest(ctx, c, owner, l, false)
			if err != nil {
				return err
			}
			if _, err := srapi.NewServiceClient(c).SrLocalsidAddDel(ctx, req); err != nil {
				return fmt.Errorf("sr_localsid_add_del: %w", err)
			}
			return nil
		},
		// Del gets the SID as VPP has it; the delete key is (sid, fib table), and the table must
		// exist (VPP indexes the FIB with fib_table_find's result unchecked).
		Del: func(ctx context.Context, c vpp.Client, l *LocalSid) error {
			if err := df6.RequireTable(ctx, c, l.GetFibTable(), true); err != nil {
				return err
			}
			sid, err := df6.IP6Of(l.GetSid())
			if err != nil {
				return err
			}
			req := &srapi.SrLocalsidAddDel{IsDel: true, Localsid: sid, SwIfIndex: interface_types.InterfaceIndex(df6.NoInterface), FibTable: l.GetFibTable()}
			if _, err := srapi.NewServiceClient(c).SrLocalsidAddDel(ctx, req); err != nil {
				return fmt.Errorf("sr_localsid_add_del (del): %w", err)
			}
			return nil
		},
		// Identity: a SID we claimed must still be in the table we put it in.
		Identity: func(want, have *LocalSid) bool { return want.GetFibTable() == have.GetFibTable() },
		List: func(ctx context.Context, c vpp.Client) ([]*LocalSid, error) {
			stream, err := srapi.NewServiceClient(c).SrLocalsidsDump(ctx, &srapi.SrLocalsidsDump{})
			if err != nil {
				return nil, fmt.Errorf("sr_localsids_dump: %w", err)
			}
			recs, err := df6.Collect(stream.Recv)
			if err != nil {
				return nil, fmt.Errorf("sr_localsids_dump: %w", err)
			}
			ifs, err := df6.DumpInterfaces(ctx, c, owner)
			if err != nil {
				return nil, err
			}
			var out []*LocalSid
			for _, r := range recs {
				b := Behavior(r.Behavior)
				if _, ok := Behavior_name[int32(b)]; !ok || b == Behavior_BEHAVIOR_UNSPECIFIED {
					continue // uSID / plugin behaviours are not this descriptor's
				}
				sid := df6.IP6String(r.Addr)
				if sid == "" {
					continue
				}
				l := &LocalSid{Sid: sid, Behavior: b, EndPsp: r.EndPsp, FibTable: r.FibTable}
				switch {
				case needsInterface(b):
					l.Interface = ifs.NameOrEmpty(r.XconnectIfaceOrVrfTable) // logical name
				case needsLookup(b):
					l.LookupTable = r.XconnectIfaceOrVrfTable
				}
				if b == Behavior_END_X || b == Behavior_END_DX4 || b == Behavior_END_DX6 {
					l.NextHop = df6.AddressString(r.XconnectNhAddr)
				}
				out = append(out, l)
			}
			return out, nil
		},
	}
}
