package agent

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/ikev2"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

var nativeStateMu sync.Mutex

func (g *server) ikev2Action(req *vrxv1.Ikev2Action, stream grpc.ServerStreamingServer[vrxv1.ActionOutput]) error {
	ctx := stream.Context()
	s := g.svc
	if req.GetTunnel() == "" {
		return status.Error(codes.InvalidArgument, "tunnel is required")
	}
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	if err := ikev2.RequireSafeState(ctx, s.vpp); err != nil {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	kvs, err := subsystems.IKEv2Profiles(ctx, s.owner)
	if err != nil {
		return status.Error(codes.Unavailable, "native profiles unavailable")
	}
	p := nativeProfileFromKVs(req.GetTunnel(), kvs)
	if p == nil {
		return status.Error(codes.NotFound, "owned native tunnel not found")
	}
	switch req.GetOperation() {
	case "initiate":
		if p.GetResponder() == nil {
			return status.Error(codes.FailedPrecondition, "tunnel has no fixed responder interface")
		}
		err = ikev2.InitiateSAInit(ctx, s.vpp, s.owner, p.Name)
	case "rekey", "delete-sa":
		sas, e := ikev2.SAs(ctx, s.vpp, s.owner)
		if e != nil {
			return status.Error(codes.Unavailable, "native SAs unavailable")
		}
		found := false
		for _, sa := range sas {
			if sa.Profile != p.Name {
				continue
			}
			if req.GetOperation() == "delete-sa" && sa.ISPI == req.GetIkeSpi() {
				found = true
				err = ikev2.DeleteIKESA(ctx, s.vpp, sa.ISPI)
				break
			}
			if req.GetOperation() == "rekey" {
				for _, ch := range sa.Children {
					if ch.ISPI == req.GetChildSpi() || ch.RSPI == req.GetChildSpi() {
						if !nativeLocalInitiator(p, sa) {
							return status.Error(codes.FailedPrecondition, "VPP supports local CHILD rekey only as IKE initiator; request rekey on the peer for a responder tunnel")
						}
						found = true
						err = ikev2.RekeyChildSA(ctx, s.vpp, ch.ISPI)
						break
					}
				}
			}
		}
		if !found {
			return status.Error(codes.NotFound, "SA does not belong to this tunnel")
		}
	default:
		return status.Error(codes.InvalidArgument, "operation must be initiate, rekey or delete-sa")
	}
	if err != nil {
		return status.Errorf(codes.Unavailable, "native IKE action failed: %v", err)
	}
	if err := stream.Send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Line{Line: "native IKE action accepted"}}); err != nil {
		return err
	}
	return stream.Send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Done{Done: &vrxv1.ActionDone{ExitCode: 0}}})
}

func (s *Service) nativeIpsecState(ctx context.Context, req *vrxv1.IpsecStateRequest, statsPaths ...string) (*vrxv1.IpsecStateResponse, error) {
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	if err := ikev2.RequireSafeState(ctx, s.vpp); err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	nativeStateMu.Lock()
	defer nativeStateMu.Unlock()
	states, err := ikev2.SAs(ctx, s.vpp, s.owner)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "native IPsec state: %v", err)
	}
	profiles, err := subsystems.IKEv2Profiles(ctx, s.owner)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "native IPsec profile state unavailable")
	}
	statsPath := "/run/vpp/stats.sock"
	if len(statsPaths) > 0 && statsPaths[0] != "" {
		statsPath = statsPaths[0]
	}
	counters, err := ikev2.SACounters(ctx, s.vpp, statsPath, states)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "native IPsec counters: %v", err)
	}
	resp := &vrxv1.IpsecStateResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now()), DaemonVersion: "vpp-ikev2"}
	wanted := func(name string) bool {
		if len(req.GetTunnels()) == 0 {
			return true
		}
		for _, n := range req.GetTunnels() {
			if n == name {
				return true
			}
		}
		return false
	}
	for _, kv := range profiles {
		p, ok := kv.Value.(*vpnpb.Ikev2Profile)
		if !ok || !wanted(p.Name) {
			continue
		}
		c := &vrxv1.IpsecConnState{Name: s.owner + "-" + p.Name, Tunnel: p.Name, Version: "2", LocalAuth: p.GetAuth().GetMethod(), RemoteAuth: p.GetAuth().GetMethod(), LocalId: p.GetLocalId().GetValue(), RemoteId: p.GetRemoteId().GetValue()}
		c.Children = []*vrxv1.IpsecChildConn{{Name: p.Name, Mode: "tunnel", RekeySec: int64(p.GetLifetime().GetSeconds()), LocalTs: []string{p.GetLocalTs().GetStartAddr() + "-" + p.GetLocalTs().GetEndAddr()}, RemoteTs: []string{p.GetRemoteTs().GetStartAddr() + "-" + p.GetRemoteTs().GetEndAddr()}}}
		resp.Conns = append(resp.Conns, c)
	}
	for _, sa := range states {
		if !wanted(sa.Profile) {
			continue
		}
		state := sa.State
		if state == "IKEV2_STATE_AUTHENTICATED" || state == "AUTHENTICATED" {
			state = "ESTABLISHED"
		}
		profile := nativeProfileFromKVs(sa.Profile, profiles)
		initiator := nativeLocalInitiator(profile, sa)
		local, remote := sa.RAddr, sa.IAddr
		localID, remoteID := sa.RID, sa.IID
		if initiator {
			local, remote = sa.IAddr, sa.RAddr
			localID, remoteID = sa.IID, sa.RID
		}
		v := &vrxv1.IpsecIkeSa{Name: s.owner + "-" + sa.Profile, Tunnel: sa.Profile, UniqueId: strconv.FormatUint(sa.ISPI, 16), Version: "2", State: state, LocalHost: local, RemoteHost: remote, Initiator: initiator, EncrAlg: sa.Encryption, IntegAlg: sa.Integrity, PrfAlg: sa.PRF, DhGroup: sa.DH, EstablishedSec: int64(sa.Uptime)}
		if localID != nil {
			v.LocalId = localID.Value
		}
		if remoteID != nil {
			v.RemoteId = remoteID.Value
		}
		for _, child := range sa.Children {
			spiIn, spiOut := nativeChildDirection(initiator, child)
			in, out := counters[spiIn], counters[spiOut]
			v.Children = append(v.Children, &vrxv1.IpsecChildSa{Name: sa.Profile, UniqueId: strconv.FormatUint(uint64(child.Index), 10), State: "INSTALLED", Mode: "tunnel", Protocol: "ESP", SpiIn: fmt.Sprintf("%08x", spiIn), SpiOut: fmt.Sprintf("%08x", spiOut), EncrAlg: child.Encryption, IntegAlg: child.Integrity, Esn: child.ESN, Encap: in.Encap || out.Encap, BytesIn: in.Bytes, PacketsIn: in.Packets, BytesOut: out.Bytes, PacketsOut: out.Packets, InstallSec: int64(child.Uptime)})
		}
		resp.Sas = append(resp.Sas, v)
	}
	sort.Slice(resp.Sas, func(i, j int) bool {
		a, b := resp.Sas[i], resp.Sas[j]
		if a.Tunnel != b.Tunnel {
			return a.Tunnel < b.Tunnel
		}
		return a.UniqueId < b.UniqueId
	})
	resp.Sas, resp.Total = pageSAs(resp.Sas, req.GetOffset(), req.GetLimit())
	return resp, nil
}

func nativeLocalInitiator(profile *vpnpb.Ikev2Profile, sa ikev2.SAState) bool {
	if profile == nil {
		return false
	}
	if remote := profile.GetResponder().GetAddress(); remote != "" {
		if sa.RAddr == remote && sa.IAddr != remote {
			return true
		}
		if sa.IAddr == remote && sa.RAddr != remote {
			return false
		}
	}
	if sa.IID == nil {
		return false
	}
	if sa.RID != nil && sa.RID.Type == sa.IID.Type && sa.RID.Value == sa.IID.Value {
		return false
	}
	return profile.GetLocalId().GetType() == sa.IID.Type && profile.GetLocalId().GetValue() == sa.IID.Value
}
func nativeChildDirection(initiator bool, ch ikev2.ChildSAState) (uint32, uint32) {
	if initiator {
		return ch.ISPI, ch.RSPI
	}
	return ch.RSPI, ch.ISPI
}

// nativeProfileForAction rejects arbitrary VPP names and actions outside owned
// profiles. The registry descriptor owns the HMAC key and secret resolver.
func nativeProfileFromKVs(name string, kvs []scheduler.KV) *vpnpb.Ikev2Profile {
	for _, kv := range kvs {
		if p, ok := kv.Value.(*vpnpb.Ikev2Profile); ok && p.Name == name {
			return p
		}
	}
	return nil
}
