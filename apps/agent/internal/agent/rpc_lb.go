package agent

// F-lb live-state and action RPCs (docs/contracts/proto.md "F-lb: LbState, LbFlushVip"). Both read VPP's lb tables
// (lb_vip_dump, lb_as_dump) — never a Retrieve source: every lb object is write-only (V20, D-063). One lb walk at a
// time per agent; a second caller waits up to 3 s, then UNAVAILABLE (D-132).

import (
	"context"
	"errors"
	"math/bits"
	"sort"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/lb"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/vpp"
)

// lbWalk admits one lb table walk at a time (D-132); lbWalkWait bounds the wait for it and for the stored desired
// state (a running transaction holds it).
var (
	lbWalk     = make(chan struct{}, 1)
	lbWalkWait = 3 * time.Second
)

// LbState implements the LbState RPC.
func (g *server) LbState(ctx context.Context, req *vrxv1.LbStateRequest) (*vrxv1.LbStateResponse, error) {
	return g.svc.LbState(ctx, req)
}

// LbFlushVip implements the LbFlushVip RPC.
func (g *server) LbFlushVip(ctx context.Context, req *vrxv1.LbFlushVipRequest) (*vrxv1.LbFlushVipResponse, error) {
	return g.svc.LbFlushVip(ctx, req)
}

func lbAcquire(ctx context.Context) (func(), error) {
	t := time.NewTimer(lbWalkWait)
	defer t.Stop()
	select {
	case lbWalk <- struct{}{}:
		return func() { <-lbWalk }, nil
	case <-t.C:
		return nil, status.Error(codes.Unavailable, "another lb table walk is in flight (one at a time, D-132); retry")
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	}
}

// lbStored returns a copy of services.lb of the stored desired state (nil when absent).
func (s *Service) lbStored(ctx context.Context) (*vrxv1.LbService, error) {
	lctx, cancel := context.WithTimeout(ctx, lbWalkWait)
	defer cancel()
	if err := s.lock(lctx); err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		return nil, status.Error(codes.Unavailable, "a transaction is running; retry")
	}
	defer s.unlock()
	l := s.st.desired.GetServices().GetLb()
	if l == nil {
		return nil, nil
	}
	return proto.Clone(l).(*vrxv1.LbService), nil
}

func lbStatusOf(err error) error {
	if errors.Is(err, vpp.ErrDisconnected) {
		return status.Error(codes.Unavailable, err.Error())
	}
	if _, ok := status.FromError(err); ok && status.Code(err) != codes.Unknown {
		return err
	}
	return status.Errorf(codes.Internal, "lb: %v", err)
}

// LbState implements the LbState RPC: one entry per VIP of the stored desired state.
func (s *Service) LbState(ctx context.Context, req *vrxv1.LbStateRequest) (*vrxv1.LbStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	cfg, err := s.lbStored(ctx)
	if err != nil {
		return nil, err
	}
	release, err := lbAcquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	vips, err := lb.DumpVIPs(ctx, s.vpp)
	if err != nil {
		return nil, lbStatusOf(err)
	}
	ases, err := lb.DumpASes(ctx, s.vpp)
	if err != nil {
		return nil, lbStatusOf(err)
	}
	want := map[string]bool{}
	for _, n := range req.GetNames() {
		want[n] = true
	}
	names := make([]string, 0, len(cfg.GetVips()))
	for n := range cfg.GetVips() {
		if len(want) == 0 || want[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	vd := lb.NewVIP(s.vpp, s.owner)
	resp := &vrxv1.LbStateResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now()), TotalVppVips: uint32(len(vips))}
	for _, name := range names {
		v := cfg.GetVips()[name]
		st := &vrxv1.LbVipState{Name: name, Prefix: v.GetPrefix(), Protocol: v.GetProtocol(), Port: v.GetPort()}
		spec, err := desired.LbVIPOf(v)
		if err != nil {
			resp.Vips = append(resp.Vips, st) // never applied (the projection refused it)
			continue
		}
		st.Prefix, st.Protocol = spec.Prefix, spec.Protocol
		if st.Applied, err = vd.Recorded(ctx, string(lb.KeyVIP(spec.VIP))); err != nil {
			return nil, lbStatusOf(err)
		}
		for _, d := range vips {
			if d.Prefix == spec.Prefix && d.Port == spec.Port {
				st.VppEntries++
				st.Encap, st.Dscp, st.TargetPort = d.Encap, uint32(d.DSCP), uint32(bits.ReverseBytes16(d.TargetPort))
			}
		}
		for _, a := range ases {
			if a.Prefix == spec.Prefix && a.Port == spec.Port {
				st.Servers = append(st.Servers, &vrxv1.LbServerState{Address: a.Address, InUse: a.InUse, InUseSince: a.InUseSince})
			}
		}
		sort.SliceStable(st.Servers, func(i, j int) bool {
			if st.Servers[i].GetInUse() != st.Servers[j].GetInUse() {
				return st.Servers[i].GetInUse()
			}
			return st.Servers[i].GetAddress() < st.Servers[j].GetAddress()
		})
		resp.Vips = append(resp.Vips, st)
	}
	return resp, nil
}

// LbFlushVip implements the LbFlushVip RPC: lb_flush_vip for a VIP of the stored desired state that this agent
// created on the running VPP instance and that has an application server in use (lb.FlushVIP).
func (s *Service) LbFlushVip(ctx context.Context, req *vrxv1.LbFlushVipRequest) (*vrxv1.LbFlushVipResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	cfg, err := s.lbStored(ctx)
	if err != nil {
		return nil, err
	}
	v, ok := cfg.GetVips()[req.GetName()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "VIP %q is not in this agent's configuration", req.GetName())
	}
	spec, err := desired.LbVIPOf(v)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "VIP %q was never applied: %v", req.GetName(), err)
	}
	key := lb.KeyVIP(spec.VIP)
	release, err := lbAcquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	ours, err := lb.NewVIP(s.vpp, s.owner).Recorded(ctx, string(key))
	if err != nil {
		return nil, lbStatusOf(err)
	}
	if !ours {
		return nil, status.Errorf(codes.FailedPrecondition, "VIP %q (%s) was not created by this agent on the running VPP instance", req.GetName(), key)
	}
	if err := lb.FlushVIP(ctx, s.vpp, spec.VIP); err != nil {
		if errors.Is(err, lb.ErrFlushNotSafe) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		return nil, lbStatusOf(err)
	}
	s.log.Info("lb VIP flow table flushed", "vip", req.GetName(), "key", string(key))
	return &vrxv1.LbFlushVipResponse{Vip: string(key)}, nil
}
