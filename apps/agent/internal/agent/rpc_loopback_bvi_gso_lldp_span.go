package agent

// F-loopback-bvi-gso-lldp-span live-state RPC (docs/contracts/proto.md §11): LldpNeighbors is read-only
// and built from dumps only, like InterfaceState (P08): lldp_dump (DF-7's lldp.Neighbours) for the
// LLDP-enabled interfaces and what was heard on them, sw_interface_dump for their logical names.

import (
	"context"
	"encoding/hex"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	lldpapi "ngfw/agent/binapi/lldp"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/lldp"
	"ngfw/agent/internal/vpp/bootid"
)

// Page size bounds of LldpNeighbors.
const (
	lldpNeighborsDefaultLimit = 100
	lldpNeighborsMaxLimit     = 1000
)

// LldpNeighbors implements the LldpNeighbors RPC.
func (g *server) LldpNeighbors(ctx context.Context, req *vrxv1.LldpNeighborsRequest) (*vrxv1.LldpNeighborsResponse, error) {
	return g.svc.LldpNeighbors(ctx, req)
}

var lldpChassisSubtypes = map[lldpapi.ChassisIDSubtype]string{
	lldpapi.CHASSIS_ID_SUBTYPE_CHASSIS_COMP: "chassis-component",
	lldpapi.CHASSIS_ID_SUBTYPE_INTF_ALIAS:   "interface-alias",
	lldpapi.CHASSIS_ID_SUBTYPE_PORT_COMP:    "port-component",
	lldpapi.CHASSIS_ID_SUBTYPE_MAC_ADDR:     "mac-address",
	lldpapi.CHASSIS_ID_SUBTYPE_NET_ADDR:     "network-address",
	lldpapi.CHASSIS_ID_SUBTYPE_INTF_NAME:    "interface-name",
	lldpapi.CHASSIS_ID_SUBTYPE_LOCAL:        "local",
}

var lldpPortSubtypes = map[lldpapi.PortIDSubtype]string{
	lldpapi.PORT_ID_SUBTYPE_INTF_ALIAS:       "interface-alias",
	lldpapi.PORT_ID_SUBTYPE_PORT_COMP:        "port-component",
	lldpapi.PORT_ID_SUBTYPE_MAC_ADDR:         "mac-address",
	lldpapi.PORT_ID_SUBTYPE_NET_ADDR:         "network-address",
	lldpapi.PORT_ID_SUBTYPE_INTF_NAME:        "interface-name",
	lldpapi.PORT_ID_SUBTYPE_AGENT_CIRCUIT_ID: "agent-circuit-id",
	lldpapi.PORT_ID_SUBTYPE_LOCAL:            "local",
}

// lldpID formats a chassis or port id: a 6-byte MAC as "aa:bb:cc:dd:ee:ff", a network address (IANA
// family 1 = IPv4, 2 = IPv6, then the address) as the address, printable ASCII as text, else hex.
func lldpID(b []byte, isMAC, isNet bool) string {
	switch {
	case len(b) == 0:
		return ""
	case isMAC && len(b) == 6:
		return iface.FormatMAC([6]uint8(b))
	case isNet && len(b) == 5 && b[0] == 1:
		return netip.AddrFrom4([4]byte(b[1:])).String()
	case isNet && len(b) == 17 && b[0] == 2:
		return netip.AddrFrom16([16]byte(b[1:])).String()
	}
	printable := true
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			printable = false
			break
		}
	}
	if printable {
		return string(b)
	}
	parts := make([]string, len(b))
	for i, c := range b {
		parts[i] = hex.EncodeToString([]byte{c})
	}
	return strings.Join(parts, ":")
}

// vppClock estimates VPP's vlib clock (seconds since VPP started; the clock of lldp_dump's last_heard
// and last_sent): the host's uptime minus the VPP process start time (/proc/<pid>/stat field 22, the
// D-080 boot identity). VPP 26.06 exposes no getter for vlib_time_now (show_vpe_system_time is Unix
// time), so the estimate is early by VPP's start-up time (plugin loading, about a second). ok=false
// when the process start time is unknown.
var vppClock = func(ctx context.Context, s *Service) (float64, bool) {
	id, err := bootid.Current(ctx, s.vpp)
	if err != nil || id.StartTime == 0 {
		return 0, false
	}
	raw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, false
	}
	f := strings.Fields(string(raw))
	if len(f) == 0 {
		return 0, false
	}
	up, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0, false
	}
	const userHZ = 100 // Linux USER_HZ (clock ticks of /proc/<pid>/stat) on every supported architecture
	return up - float64(id.StartTime)/userHZ, true
}

// ago is now − t on VPP's clock (0 when t is 0 = never, or when the clock is unknown).
func ago(now float64, known bool, t float64) float64 {
	if !known || t <= 0 || now < t {
		return 0
	}
	return now - t
}

// LldpNeighbors implements the RPC: one entry per LLDP-enabled interface this agent can name, ordered
// by interface name, paged.
func (s *Service) LldpNeighbors(ctx context.Context, req *vrxv1.LldpNeighborsRequest) (*vrxv1.LldpNeighborsResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	limit := req.GetLimit()
	switch {
	case limit == 0:
		limit = lldpNeighborsDefaultLimit
	case limit > lldpNeighborsMaxLimit:
		return nil, status.Errorf(codes.InvalidArgument, "limit %d exceeds %d", limit, lldpNeighborsMaxLimit)
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	table, err := lldp.Neighbours(ctx, s.vpp)
	if err != nil {
		return nil, grpcVPPError(err, "lldp_dump")
	}
	t, err := iface.Dump(ctx, s.vpp, s.owner)
	if err != nil {
		return nil, grpcVPPError(err, "lldp neighbours")
	}
	now, known := vppClock(ctx, s)
	all := make([]*vrxv1.LldpNeighbor, 0, len(table))
	for idx, n := range table {
		name, ok := t.Logical(idx)
		if !ok {
			continue // another owner's interface (or one VPP no longer lists): never reported
		}
		e := &vrxv1.LldpNeighbor{
			Interface: name, SwIfIndex: idx, Heard: n.LastHeard > 0, Ttl: uint32(n.TTL),
			LastHeardSecAgo: ago(now, known, n.LastHeard), LastSentSecAgo: ago(now, known, n.LastSent),
		}
		if e.Heard {
			e.ChassisIdSubtype = lldpChassisSubtypes[n.ChassisType]
			e.ChassisId = lldpID(n.ChassisID, n.ChassisType == lldpapi.CHASSIS_ID_SUBTYPE_MAC_ADDR, n.ChassisType == lldpapi.CHASSIS_ID_SUBTYPE_NET_ADDR)
			e.PortIdSubtype = lldpPortSubtypes[n.PortType]
			e.PortId = lldpID(n.PortID, n.PortType == lldpapi.PORT_ID_SUBTYPE_MAC_ADDR, n.PortType == lldpapi.PORT_ID_SUBTYPE_NET_ADDR)
		}
		all = append(all, e)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].GetInterface() != all[j].GetInterface() {
			return all[i].GetInterface() < all[j].GetInterface()
		}
		return all[i].GetSwIfIndex() < all[j].GetSwIfIndex()
	})
	resp := &vrxv1.LldpNeighborsResponse{Total: uint32(len(all)), Owner: s.owner, RetrievedAt: timestamppb.New(s.now())} //nolint:gosec // one entry per interface
	start := int(req.GetOffset())
	if start >= len(all) {
		return resp, nil
	}
	end := min(start+int(limit), len(all))
	resp.Neighbors = all[start:end]
	return resp, nil
}
