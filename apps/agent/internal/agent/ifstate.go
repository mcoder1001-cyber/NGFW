package agent

// InterfaceState (P08, docs/contracts/proto.md §8a): the live interface table — what the API's
// /api/v1/state/interfaces serves — built from dumps only (sw_interface_dump, ip_address_dump,
// sw_interface_get_table, sw_interface_rx_placement_dump). The same table feeds Retrieve's
// assembly of interfaces the agent does not manage attributes of (desired.Live).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/vpp"
)

// claimHolders are the DF-1 descriptors whose claims make an untagged interface "managed".
var claimHolders = []string{iface.AdminStateName, iface.MtuName, iface.MacAddressName, iface.PromiscName, iface.RxModeName}

// ifTable is desired.Live over a snapshot of the interface table.
type ifTable map[string]*vrxv1.InterfaceState

func (t ifTable) State(name string) (*vrxv1.InterfaceState, bool) {
	st, ok := t[name]
	return st, ok
}

// typeOf names the interface kind reported in InterfaceState.type.
func typeOf(d *ifapi.SwInterfaceDetails) string {
	if d.Type == interface_types.IF_API_TYPE_SUB {
		return "sub-interface"
	}
	dev := strings.TrimRight(d.InterfaceDevType, "\x00")
	switch dev {
	case "Loopback":
		return "loopback"
	case "":
		return "other"
	}
	return dev // af-packet, tap, bond, memif, dpdk, …
}

// interfaceTable dumps the live table: every interface with a logical name for this owner.
func (s *Service) interfaceTable(ctx context.Context) (ifTable, error) {
	t, err := iface.Dump(ctx, s.vpp, s.owner)
	if err != nil {
		return nil, err
	}
	rx, err := rxModes(ctx, s.vpp)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	stored := s.storedIfs
	s.mu.Unlock()
	claims := iface.Claims(s.owner)
	svc := ifapi.NewServiceClient(s.vpp)
	ipSvc := ip.NewServiceClient(s.vpp)
	out := ifTable{}
	for _, idx := range t.Indexes() {
		name, ok := t.Logical(idx)
		if !ok {
			continue
		}
		if _, dup := out[name]; dup {
			continue // an untagged interface with our interface's name: ours wins (alias Retrieve rule)
		}
		d, _ := t.Details(idx)
		st := &vrxv1.InterfaceState{
			Name: name, VppName: t.VPPName(idx), SwIfIndex: idx, Type: typeOf(d),
			AdminUp:       d.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0,
			LinkUp:        d.Flags&interface_types.IF_STATUS_API_FLAG_LINK_UP != 0,
			LinkMtu:       uint32(d.LinkMtu),
			Mac:           iface.FormatMAC(d.L2Address),
			RxMode:        rx[idx],
			LinkSpeedKbps: uint64(d.LinkSpeed),
		}
		if len(d.Mtu) > 0 {
			st.Mtu = d.Mtu[0]
		}
		if st.Mtu == 0 && d.Type != interface_types.IF_API_TYPE_SUB {
			st.Mtu = st.LinkMtu
		}
		if _, ours := t.OwnedID(idx); ours {
			st.Managed = true
		} else {
			for _, h := range claimHolders {
				if claims.Claimed(t.VPPName(idx), h) {
					st.Managed = true
					break
				}
			}
		}
		if d.Type == interface_types.IF_API_TYPE_SUB {
			if p, ok := t.Logical(d.SupSwIfIndex); ok {
				st.Parent = p
			}
			if d.SubNumberOfTags >= 1 {
				st.VlanId = uint32(d.SubOuterVlanID)
			}
			if d.SubNumberOfTags >= 2 {
				st.InnerVlanId = uint32(d.SubInnerVlanID)
			}
		}
		tbl, err := svc.SwInterfaceGetTable(ctx, &ifapi.SwInterfaceGetTable{SwIfIndex: interface_types.InterfaceIndex(idx)})
		if err != nil {
			return nil, fmt.Errorf("sw_interface_get_table %s: %w", name, err)
		}
		st.TableId = tbl.VrfID
		st.Vrf = s.tableName(tbl.VrfID)
		for _, v6 := range []bool{false, true} {
			addrs, err := dumpAddrs(ctx, ipSvc, idx, v6)
			if err != nil {
				return nil, fmt.Errorf("ip_address_dump %s: %w", name, err)
			}
			if v6 {
				st.Ipv6 = addrs
			} else {
				st.Ipv4 = addrs
			}
		}
		st.Description = descriptionOf(stored, name, st.Parent)
		out[name] = st
	}
	return out, nil
}

func dumpAddrs(ctx context.Context, svc ip.RPCService, idx uint32, v6 bool) ([]string, error) {
	stream, err := svc.IPAddressDump(ctx, &ip.IPAddressDump{SwIfIndex: interface_types.InterfaceIndex(idx), IsIPv6: v6})
	if err != nil {
		return nil, err
	}
	var out []string
	for {
		a, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if p, err := core.CanonAddrPrefix(a.Prefix.String()); err == nil {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// rxModes returns the rx mode of queue 0 per sw_if_index.
func rxModes(ctx context.Context, c vpp.Client) (map[uint32]string, error) {
	stream, err := ifapi.NewServiceClient(c).SwInterfaceRxPlacementDump(ctx, &ifapi.SwInterfaceRxPlacementDump{SwIfIndex: interface_types.InterfaceIndex(iface.AllInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_rx_placement_dump: %w", err)
	}
	out := map[uint32]string{}
	for {
		p, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("sw_interface_rx_placement_dump: %w", err)
		}
		if p.QueueID != 0 {
			continue
		}
		switch p.Mode {
		case interface_types.RX_MODE_API_INTERRUPT:
			out[uint32(p.SwIfIndex)] = "interrupt"
		case interface_types.RX_MODE_API_ADAPTIVE:
			out[uint32(p.SwIfIndex)] = "adaptive"
		default:
			out[uint32(p.SwIfIndex)] = "polling"
		}
	}
	return out, nil
}

// descriptionOf returns the stored description of an interface or sub-interface (D-073b).
func descriptionOf(stored map[string]*vrxv1.Interface, name, parent string) string {
	if itf, ok := stored[name]; ok {
		return itf.GetDescription()
	}
	if parent != "" && strings.HasPrefix(name, parent+".") {
		if sub, ok := stored[parent].GetSubinterfaces()[strings.TrimPrefix(name, parent+".")]; ok {
			return sub.GetDescription()
		}
	}
	return ""
}

// tableName names a FIB table: a VRF of the stored desired state, "default" for 0, else the id.
func (s *Service) tableName(id uint32) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for n, v := range s.vrfIDs {
		if v == id {
			return n
		}
	}
	if id == 0 {
		return "default"
	}
	return fmt.Sprint(id)
}

// InterfaceState implements the InterfaceState RPC.
func (s *Service) InterfaceState(ctx context.Context, req *vrxv1.InterfaceStateRequest) (*vrxv1.InterfaceStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	tbl, err := s.interfaceTable(ctx)
	if err != nil {
		if errors.Is(err, vpp.ErrDisconnected) {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "interface state: %v", err)
	}
	want := map[string]bool{}
	for _, n := range req.GetNames() {
		want[n] = true
	}
	resp := &vrxv1.InterfaceStateResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now())}
	for _, name := range sortedMapKeys(tbl) {
		if len(want) > 0 && !want[name] {
			continue
		}
		resp.Interfaces = append(resp.Interfaces, tbl[name])
	}
	return resp, nil
}

var _ desired.Live = ifTable(nil)
