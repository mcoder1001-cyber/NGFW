package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/bfd"
	"ngfw/agent/internal/descriptors/df7"
	frrbfd "ngfw/agent/internal/renderers/frr/bfd"
	"ngfw/agent/internal/renderers/frr/redistribute"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
	"strings"
)

func (g *server) BfdState(ctx context.Context, req *ngfwv1.BfdStateRequest) (*ngfwv1.BfdStateResponse, error) {
	s := g.svc
	if e := s.checkOwner(req.GetOwner()); e != nil {
		return nil, e
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	if e := s.lock(ctx); e != nil {
		return nil, e
	}
	defer s.unlock()
	out := &ngfwv1.BfdStateResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now())}
	states, e := bfd.Sessions(ctx, s.vpp, s.owner)
	if e != nil {
		return nil, status.Errorf(codes.Internal, "BFD state: %v", e)
	}
	byKey := map[string]string{}
	for _, v := range states {
		byKey[v.Key] = v.State
	}
	kvs, e := s.sched.Retrieve(ctx, scheduler.Only(bfd.NameSession))
	if e != nil {
		return nil, status.Errorf(codes.Internal, "BFD sessions: %v", e)
	}
	for _, kv := range kvs {
		x, e := df7.Decode[bfd.Session](kv.Value)
		if e != nil {
			return nil, status.Error(codes.Internal, "BFD session decode failed")
		}
		session := &ngfwv1.BfdSessionState{Engine: "vpp", Multihop: x.Multihop, Interface: x.Interface, LocalAddress: x.Local, PeerAddress: x.Peer, State: byKey[string(kv.Key)], DesiredMinTxUs: x.DesiredMinTx, RequiredMinRxUs: x.RequiredMinRx, DetectMultiplier: uint32(x.DetectMult)}
		if at := subsystems.BfdLastFlap(s.owner, string(kv.Key)); !at.IsZero() {
			session.LastFlap = timestamppb.New(at)
		}
		out.Sessions = append(out.Sessions, session)
	}
	if rt := subsystems.FRRRuntime(s.owner); rt != nil {
		st, e := rt.State(ctx, []string{frrbfd.Reader}, nil, "")
		if e != nil {
			out.Error = "FRR BFD state unavailable"
		} else {
			out.Error = st.Err

			raw := st.Readers[frrbfd.Reader]
			peers, err := decodeFRRBfdPeers(raw)
			if err != nil {
				out.Error = "FRR BFD response invalid"
			} else {
				out.Sessions = append(out.Sessions, peers...)
			}
		}
	}
	return out, nil
}
func (g *server) RedistributionMatrix(ctx context.Context, req *ngfwv1.RedistributionMatrixRequest) (*ngfwv1.RedistributionMatrixResponse, error) {
	s := g.svc
	if e := s.checkOwner(req.GetOwner()); e != nil {
		return nil, e
	}
	if e := s.lock(ctx); e != nil {
		return nil, e
	}
	defer s.unlock()
	out := &ngfwv1.RedistributionMatrixResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now()), Edges: redistribute.Edges(s.st.desired.GetRouting())}
	if rt := subsystems.FRRRuntime(s.owner); rt != nil {
		st, e := rt.State(ctx, []string{redistribute.Reader}, nil, "")
		if e != nil {
			out.Error = "FRR route summary unavailable"
		} else {
			out.Error = st.Err
			applyRedistributionCounts(out.Edges, st.RIBCounts)
		}
	} else {
		out.Error = "FRR routing subsystem unavailable"
	}
	return out, nil
}

// FRR show bfd peers json reports top-level intervals in milliseconds.
func decodeFRRBfdPeers(raw string) ([]*ngfwv1.BfdSessionState, error) {
	var peers []struct {
		Peer             string `json:"peer"`
		Local            string `json:"local"`
		Interface        string `json:"interface"`
		Status           string `json:"status"`
		Multihop         bool   `json:"multihop"`
		ReceiveInterval  uint32 `json:"receive-interval"`
		TransmitInterval uint32 `json:"transmit-interval"`
		DetectMultiplier uint32 `json:"detect-multiplier"`
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &peers); err != nil {
			return nil, err
		}
	}
	out := make([]*ngfwv1.BfdSessionState, 0, len(peers))
	for _, p := range peers {
		if p.TransmitInterval > ^uint32(0)/1000 || p.ReceiveInterval > ^uint32(0)/1000 {
			return nil, fmt.Errorf("FRR BFD interval exceeds microsecond state range")
		}
		out = append(out, &ngfwv1.BfdSessionState{Engine: "frr", Interface: p.Interface, LocalAddress: p.Local, PeerAddress: p.Peer, State: strings.ToLower(p.Status), Multihop: p.Multihop, DesiredMinTxUs: p.TransmitInterval * 1000, RequiredMinRxUs: p.ReceiveInterval * 1000, DetectMultiplier: p.DetectMultiplier})
	}
	return out, nil
}
func applyRedistributionCounts(edges []*ngfwv1.RedistributionEdge, counts map[string]uint32) {
	for _, edge := range edges {
		family := "ipv4"
		if edge.Target == "ospf6" || edge.Target == "ripng" {
			family = "ipv6"
		}
		if n, ok := counts[family+"/"+edge.Vrf+"/"+edge.Source]; ok {
			count := uint64(n)
			edge.RouteCount = &count
		}
	}
}
