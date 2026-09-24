package agent

// DhcpLeases (F-kea-dhcp-relay, docs/contracts/proto.md §11): Kea lease pages with the daemons' status and pool
// usage, or the VPP DHCPv4 client state of one interface — read-only status, never part of Retrieve (§5). The DHCP
// runtime is the one subsystems registered for this agent's state dir and owner.

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/kea"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
)

// DHCP lease paging bounds.
const (
	dhcpDefaultPageSize = 100
	dhcpMaxPageSize     = 1000
	dhcpReadTimeout     = 20 * time.Second
)

func (g *server) DhcpLeases(ctx context.Context, req *vrxv1.DhcpLeasesRequest) (*vrxv1.DhcpLeasesResponse, error) {
	return g.svc.DhcpLeases(ctx, req)
}

// DhcpLeases implements the RPC.
func (s *Service) DhcpLeases(ctx context.Context, req *vrxv1.DhcpLeasesRequest) (*vrxv1.DhcpLeasesResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	var fams []int
	switch req.GetFamily() {
	case "":
		fams = []int{4, 6}
	case "ipv4":
		fams = []int{4}
	case "ipv6":
		fams = []int{6}
	default:
		return nil, status.Errorf(codes.InvalidArgument, "family %q: want ipv4, ipv6 or empty", req.GetFamily())
	}
	size := int(req.GetPageSize())
	switch {
	case size == 0:
		size = dhcpDefaultPageSize
	case size > dhcpMaxPageSize:
		return nil, status.Errorf(codes.InvalidArgument, "page_size %d above %d", size, dhcpMaxPageSize)
	}
	page := int(req.GetPage())
	if page == 0 {
		page = 1
	}
	rt := subsystems.DHCPFor(s.st.dir, s.owner) // st.dir is fixed at construction
	if rt == nil {
		return nil, status.Error(codes.Unimplemented, "the services domain (DHCP) is not wired in this agent")
	}
	resp := &vrxv1.DhcpLeasesResponse{Owner: s.owner, Page: uint32(page), PageSize: uint32(size), RetrievedAt: timestamppb.New(s.now())} //nolint:gosec // bounded above
	ctx, cancel := context.WithTimeout(ctx, dhcpReadTimeout)
	defer cancel()

	if ifn := req.GetInterface(); ifn != "" {
		if !s.vpp.Connected() {
			return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
		}
		leases, err := rt.Client.Leases(ctx)
		if err != nil {
			if errors.Is(err, vpp.ErrDisconnected) {
				return nil, status.Error(codes.Unavailable, err.Error())
			}
			return nil, status.Errorf(codes.Internal, "dhcp_client_dump: %v", err)
		}
		cl := &vrxv1.DhcpClientLease{Interface: ifn}
		if l, ok := leases[ifn]; ok {
			cl.Configured, cl.State, cl.Hostname, cl.Mac = true, l.State, l.Hostname, l.HostMAC
			if l.Address.IsValid() {
				cl.Address = l.Address.String()
			}
			if l.Router.IsValid() && !l.Router.IsUnspecified() {
				cl.Router = l.Router.String()
			}
			for _, d := range l.DNSServers {
				if !d.IsUnspecified() {
					cl.DnsServers = append(cl.DnsServers, d.String())
				}
			}
		}
		resp.Client = cl
		return resp, nil
	}

	if rt.Kea == nil {
		return resp, nil // VRX_KEA_MODE=off: no Kea daemons are managed
	}
	q := kea.LeaseQuery{Families: fams, Filter: req.GetFilter(), Offset: (page - 1) * size, Limit: size}
	names := map[int]map[uint32][2]string{} // family → subnet id → {server, subnet}
	if req.GetServer() != "" {
		q.SubnetIDs = map[int]map[uint32]bool{}
	}
	for _, fam := range fams {
		st := rt.Kea.Status(ctx, fam)
		ds := &vrxv1.DhcpServerStatus{Family: kea.FamilyName(fam), Running: st.Running, Active: st.Active,
			ActionRequired: st.ActionRequired, ReloadSec: st.ReloadSec, Error: st.Err}
		names[fam] = map[uint32][2]string{}
		for _, u := range st.Subnets {
			names[fam][u.ID] = [2]string{u.Server, u.Subnet}
			if req.GetServer() != "" {
				if u.Server != req.GetServer() {
					continue
				}
				if q.SubnetIDs[fam] == nil {
					q.SubnetIDs[fam] = map[uint32]bool{}
				}
				q.SubnetIDs[fam][u.ID] = true
			}
			ds.Subnets = append(ds.Subnets, &vrxv1.DhcpSubnetUsage{Server: u.Server, Subnet: u.Subnet, Prefix: u.Prefix, SubnetId: u.ID,
				Total: u.Total, Assigned: u.Assigned, Declined: u.Declined})
		}
		resp.Servers = append(resp.Servers, ds)
	}
	leases, total, truncated, err := rt.Kea.LeasePage(ctx, q)
	if err != nil {
		if errors.Is(err, kea.ErrInsecure) {
			return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
		}
		return nil, status.Errorf(codes.Internal, "leases: %v", err)
	}
	resp.Total, resp.Truncated = uint32(total), truncated //nolint:gosec // ≤ 2 × kea.MaxLeases
	for _, l := range leases {
		out := &vrxv1.DhcpLease{Family: kea.FamilyName(l.Family), Address: l.Address, HwAddress: l.HWAddress, ClientId: l.ClientID,
			Duid: l.DUID, Hostname: l.Hostname, SubnetId: l.SubnetID, ValidLifetimeSec: l.ValidLft, State: l.StateName(),
			LeaseType: l.Type, PrefixLen: l.PrefixLen}
		if n, ok := names[l.Family][l.SubnetID]; ok {
			out.Server, out.Subnet = n[0], n[1]
		}
		if l.CLTT > 0 {
			out.ExpiresAt = timestamppb.New(time.Unix(l.Expires(), 0))
		}
		resp.Leases = append(resp.Leases, out)
	}
	return resp, nil
}
