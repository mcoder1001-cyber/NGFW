package nat44ed

import (
	"context"
	"fmt"
	"sort"

	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// VRFTableSpec is a nat44-ed VRF table (nat44_ed_add_del_vrf_table) with its routes — the
// list of VRFs whose FIBs are consulted for the table's outside traffic
// (nat44_ed_add_del_vrf_route). Route changes are applied in place.
type VRFTableSpec struct {
	Table  uint32   `json:"table"`
	Routes []uint32 `json:"routes"`
}

// Normalize sorts and de-duplicates the routes.
func (s *VRFTableSpec) Normalize() {
	if s.Routes == nil {
		s.Routes = []uint32{}
	}
	sort.Slice(s.Routes, func(i, j int) bool { return s.Routes[i] < s.Routes[j] })
	out := s.Routes[:0]
	for i, r := range s.Routes {
		if i == 0 || r != s.Routes[i-1] {
			out = append(out, r)
		}
	}
	s.Routes = out
}

func (p *Plugin) vrfRoute(ctx context.Context, table, vrf uint32, add bool) error {
	if _, err := p.svc.Nat44EdAddDelVrfRoute(ctx, &nat44_ed.Nat44EdAddDelVrfRoute{TableVrfID: table, VrfID: vrf, IsAdd: add}); err != nil {
		return fmt.Errorf("nat44_ed_add_del_vrf_route: %w", err)
	}
	return nil
}

func (p *Plugin) newVRFTable() *natcommon.Descriptor[VRFTableSpec] {
	return natcommon.New(natcommon.Ops[VRFTableSpec]{
		Name: NameVRFTable,
		ID:   func(s VRFTableSpec) string { return fmt.Sprintf("%d", s.Table) },
		Deps: func(s VRFTableSpec) []scheduler.Dependency {
			deps := natcommon.WithVRF(enableDep(), s.Table)
			for _, r := range s.Routes {
				deps = natcommon.WithVRF(deps, r)
			}
			return deps
		},
		Create: func(ctx context.Context, s VRFTableSpec) (any, error) {
			if _, err := p.svc.Nat44EdAddDelVrfTable(ctx, &nat44_ed.Nat44EdAddDelVrfTable{TableVrfID: s.Table, IsAdd: true}); err != nil {
				return nil, fmt.Errorf("nat44_ed_add_del_vrf_table: %w", err)
			}
			for _, r := range s.Routes {
				if err := p.vrfRoute(ctx, s.Table, r, true); err != nil {
					return nil, err
				}
			}
			return nil, nil
		},
		Update: func(ctx context.Context, o, n VRFTableSpec, meta any) (any, error) {
			have, want := map[uint32]bool{}, map[uint32]bool{}
			for _, r := range o.Routes {
				have[r] = true
			}
			for _, r := range n.Routes {
				want[r] = true
			}
			for _, r := range n.Routes {
				if !have[r] {
					if err := p.vrfRoute(ctx, n.Table, r, true); err != nil {
						return nil, err
					}
				}
			}
			for _, r := range o.Routes {
				if !want[r] {
					if err := p.vrfRoute(ctx, n.Table, r, false); err != nil {
						return nil, err
					}
				}
			}
			return meta, nil
		},
		Delete: func(ctx context.Context, s VRFTableSpec, _ any) error {
			if _, err := p.svc.Nat44EdAddDelVrfTable(ctx, &nat44_ed.Nat44EdAddDelVrfTable{TableVrfID: s.Table, IsAdd: false}); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat44_ed_add_del_vrf_table: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[VRFTableSpec], error) {
			// VPP 26.06 answers nat44_ed_vrf_tables_v2_dump with v1 nat44_ed_vrf_tables_details
			// messages (message-id mix-up in the plugin), which the generated v2 client rejects.
			// Drive the dump on a raw stream and accept either details type (same fields).
			stream, err := p.client.NewStream(ctx)
			if err != nil {
				return nil, err
			}
			defer func() { _ = stream.Close() }()
			if err := stream.SendMsg(&nat44_ed.Nat44EdVrfTablesV2Dump{}); err != nil {
				return nil, fmt.Errorf("nat44_ed_vrf_tables_v2_dump: %w", err)
			}
			if err := stream.SendMsg(&memclnt.ControlPing{}); err != nil {
				return nil, fmt.Errorf("control_ping: %w", err)
			}
			var out []natcommon.Item[VRFTableSpec]
			add := func(table uint32, routes []uint32) {
				if !p.scope.OwnsTable(table) {
					return
				}
				s := VRFTableSpec{Table: table, Routes: append([]uint32{}, routes...)}
				s.Normalize()
				out = append(out, natcommon.Item[VRFTableSpec]{Spec: s})
			}
			for {
				msg, err := stream.RecvMsg()
				if err != nil {
					return nil, fmt.Errorf("nat44_ed_vrf_tables_v2_dump: %w", err)
				}
				switch m := msg.(type) {
				case *memclnt.ControlPingReply:
					return out, nil
				case *nat44_ed.Nat44EdVrfTablesV2Details:
					add(m.TableVrfID, m.VrfIds)
				case *nat44_ed.Nat44EdVrfTablesDetails:
					add(m.TableVrfID, m.VrfIds)
				default:
					return nil, fmt.Errorf("nat44_ed_vrf_tables_v2_dump: unexpected message %T", msg)
				}
			}
		},
	})
}
