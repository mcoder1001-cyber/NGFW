package agent

// BondState (F-bonding, docs/contracts/proto.md §11 "F-bonding"): the live bond table of this agent — every bond
// interface that carries its owner tag — from dumps only: sw_interface_dump (names, admin/link state),
// sw_bond_interface_dump, sw_member_interface_dump per bond and the lacp plugin's sw_interface_lacp_dump. Read-only.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	bondapi "ngfw/agent/binapi/bond"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/lacp"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/bond"
	"ngfw/agent/internal/descriptors/dfkit"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/vpp"
)

// BondState implements the BondState RPC.
func (g *server) BondState(ctx context.Context, req *vrxv1.BondStateRequest) (*vrxv1.BondStateResponse, error) {
	return g.svc.BondState(ctx, req)
}

// BondState returns the live state of this agent's bonds (names empty = all).
func (s *Service) BondState(ctx context.Context, req *vrxv1.BondStateRequest) (*vrxv1.BondStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	bonds, err := bondTable(ctx, s.vpp, s.owner)
	if err != nil {
		if errors.Is(err, vpp.ErrDisconnected) {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "bond state: %v", err)
	}
	want := map[string]bool{}
	for _, n := range req.GetNames() {
		want[n] = true
	}
	resp := &vrxv1.BondStateResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now())}
	for _, b := range bonds {
		if len(want) == 0 || want[b.GetName()] {
			resp.Bonds = append(resp.Bonds, b)
		}
	}
	return resp, nil
}

// bondTable dumps this owner's bonds, sorted by name, members sorted by interface.
func bondTable(ctx context.Context, c vpp.Client, owner string) ([]*vrxv1.BondStatus, error) {
	t, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return nil, err
	}
	svc := bondapi.NewServiceClient(c)
	stream, err := svc.SwBondInterfaceDump(ctx, &bondapi.SwBondInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(iface.AllInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("sw_bond_interface_dump: %w", err)
	}
	details, err := dfkit.Drain(stream, stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("sw_bond_interface_dump: %w", err)
	}
	var lacpBy map[uint32]*lacp.SwInterfaceLacpDetails
	var out []*vrxv1.BondStatus
	for _, bd := range details {
		idx := uint32(bd.SwIfIndex)
		key, ok := t.KeyFor(idx)
		if !ok || key.Descriptor() != bond.BondName {
			continue // another owner's (or an untagged) bond
		}
		st := &vrxv1.BondStatus{
			Name: key.ID(), VppName: t.VPPName(idx), SwIfIndex: idx, Id: bd.ID,
			Mode:              desired.BondModeName(bond.Mode(bd.Mode)),    //nolint:gosec // binapi enum values 1–5
			LoadBalance:       desired.BondLBName(bond.LoadBalance(bd.Lb)), //nolint:gosec // binapi enum values 0–5
			NumaOnly:          bd.NumaOnly,
			MemberCount:       bd.Members,
			ActiveMemberCount: bd.ActiveMembers,
		}
		st.AdminUp, st.LinkUp = flagsOf(t, idx)
		mstream, err := svc.SwMemberInterfaceDump(ctx, &bondapi.SwMemberInterfaceDump{SwIfIndex: bd.SwIfIndex})
		if err != nil {
			return nil, fmt.Errorf("sw_member_interface_dump %s: %w", st.Name, err)
		}
		members, err := dfkit.Drain(mstream, mstream.Recv)
		if err != nil {
			return nil, fmt.Errorf("sw_member_interface_dump %s: %w", st.Name, err)
		}
		if bd.Mode == bondapi.BOND_API_MODE_LACP && len(members) > 0 && lacpBy == nil {
			if lacpBy, err = lacpTable(ctx, c); err != nil {
				return nil, err
			}
		}
		for _, m := range members {
			mi := uint32(m.SwIfIndex)
			name, ok := t.Logical(mi)
			if !ok {
				name = t.VPPName(mi) // a member we cannot name (another owner's): still part of our bond's state
			}
			ms := &vrxv1.BondMemberStatus{
				Interface: name, SwIfIndex: mi, Passive: m.IsPassive, LongTimeout: m.IsLongTimeout,
				Weight: m.Weight, IsLocalNuma: m.IsLocalNuma,
			}
			ms.AdminUp, ms.LinkUp = flagsOf(t, mi)
			if l, ok := lacpBy[mi]; ok && bd.Mode == bondapi.BOND_API_MODE_LACP {
				ms.Lacp = lacpStateOf(l)
			}
			st.Members = append(st.Members, ms)
		}
		sort.Slice(st.Members, func(i, j int) bool { return st.Members[i].GetInterface() < st.Members[j].GetInterface() })
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GetName() < out[j].GetName() })
	return out, nil
}

func flagsOf(t *iface.Table, idx uint32) (adminUp, linkUp bool) {
	d, ok := t.Details(idx)
	if !ok {
		return false, false
	}
	return d.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0, d.Flags&interface_types.IF_STATUS_API_FLAG_LINK_UP != 0
}

// lacpTable is sw_interface_lacp_dump by member sw_if_index; empty when the lacp plugin is not loaded.
func lacpTable(ctx context.Context, c vpp.Client) (map[uint32]*lacp.SwInterfaceLacpDetails, error) {
	out := map[uint32]*lacp.SwInterfaceLacpDetails{}
	stream, err := lacp.NewServiceClient(c).SwInterfaceLacpDump(ctx, &lacp.SwInterfaceLacpDump{})
	if err == nil {
		var ds []*lacp.SwInterfaceLacpDetails
		if ds, err = dfkit.Drain(stream, stream.Recv); err == nil {
			for _, d := range ds {
				out[uint32(d.SwIfIndex)] = d
			}
			return out, nil
		}
	}
	if errors.Is(dfkit.PluginError("lacp", err), dfkit.ErrPluginNotLoaded) {
		return out, nil
	}
	return nil, fmt.Errorf("sw_interface_lacp_dump: %w", err)
}

// LACP state machine names (src/plugins/lacp/{rx,tx,mux,ptx}_machine.h, foreach_lacp_*_sm_state).
var (
	lacpRxStates  = []string{"initialize", "port-disabled", "expired", "lacp-disabled", "defaulted", "current"}
	lacpTxStates  = []string{"transmit"}
	lacpMuxStates = []string{"detached", "waiting", "attached", "collecting-distributing"}
	lacpPtxStates = []string{"no-periodic", "fast-periodic", "slow-periodic", "periodic-tx"}
	// LACP state octet bits 0–7 (src/plugins/lacp/protocol.h foreach_lacp_state, 802.1AX).
	lacpStateBits = []string{"activity", "timeout", "aggregation", "synchronization", "collecting", "distributing", "defaulted", "expired"}
)

func stateName(names []string, v uint32) string {
	if int(v) < len(names) {
		return names[v]
	}
	return strconv.FormatUint(uint64(v), 10)
}

func lacpStateOf(d *lacp.SwInterfaceLacpDetails) *vrxv1.BondLacpState {
	port := func(prio uint16, sys [6]uint8, key, portPrio, portNum uint16, state uint8) *vrxv1.BondLacpPort {
		p := &vrxv1.BondLacpPort{
			SystemPriority: uint32(prio), System: net.HardwareAddr(sys[:]).String(), Key: uint32(key),
			PortPriority: uint32(portPrio), PortNumber: uint32(portNum), State: uint32(state),
		}
		for bit, n := range lacpStateBits {
			if state&(1<<bit) != 0 {
				p.StateFlags = append(p.StateFlags, n)
			}
		}
		return p
	}
	return &vrxv1.BondLacpState{
		RxState: stateName(lacpRxStates, d.RxState), TxState: stateName(lacpTxStates, d.TxState),
		MuxState: stateName(lacpMuxStates, d.MuxState), PtxState: stateName(lacpPtxStates, d.PtxState),
		Actor:   port(d.ActorSystemPriority, d.ActorSystem, d.ActorKey, d.ActorPortPriority, d.ActorPortNumber, d.ActorState),
		Partner: port(d.PartnerSystemPriority, d.PartnerSystem, d.PartnerKey, d.PartnerPortPriority, d.PartnerPortNumber, d.PartnerState),
	}
}
