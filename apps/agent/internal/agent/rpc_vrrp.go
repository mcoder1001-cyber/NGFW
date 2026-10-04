package agent

import (
	"context"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/vrrp"
	"ngfw/agent/internal/subsystems"
)

func (g *server) VrrpState(ctx context.Context, req *ngfwv1.VrrpStateRequest) (*ngfwv1.VrrpStateResponse, error) {
	return g.svc.vrrpState(ctx, req)
}

func (s *Service) vrrpState(ctx context.Context, req *ngfwv1.VrrpStateRequest) (*ngfwv1.VrrpStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if err := s.lock(ctx); err != nil {
		return nil, err
	}
	config := proto.Clone(s.st.desired).(*ngfwv1.DesiredState)
	s.unlock()
	out := &ngfwv1.VrrpStateResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now())}
	names := make([]string, 0, len(config.GetHa().GetVrrp()))
	for n := range config.GetHa().GetVrrp() {
		names = append(names, n)
	}
	sort.Strings(names)
	observed := map[string]vrrp.VRState{}
	var vppErr string
	if subsystems.VrrpEnv().VPPEngine {
		if !s.vpp.Connected() {
			vppErr = "VPP disconnected"
		} else {
			rows, err := vrrp.States(ctx, s.vpp, s.owner)
			if err != nil {
				vppErr = err.Error()
			} else {
				for _, row := range rows {
					observed[row.Key] = row
				}
			}
		}
	} else {
		vppErr = "VPP VRRP engine disabled"
	}
	keep, keepErr := subsystems.KeepalivedState(ctx)
	for _, n := range names {
		v := config.GetHa().GetVrrp()[n]
		engine := v.GetEngine()
		if engine == "" {
			engine = "vpp"
		}
		row := &ngfwv1.VrrpRuntime{Name: n, Engine: engine, State: "unknown"}
		if engine == "vpp" {
			row.Error = vppErr
			key := string(vrrp.KeyVR(vrrp.VR{Interface: v.GetInterface(), VRID: uint8(v.GetVrId()), IPv6: v.GetAddressFamily() == "ipv6"}))
			if obs, ok := observed[key]; ok {
				row.State = obs.State
				row.CurrentPriority = uint32(obs.Priority)
				row.MasterAdvertisementIntervalMs = uint32(obs.MasterAdvCS) * 10
			} else if row.Error == "" {
				row.Error = "configured router is absent from runtime"
			}
		} else {
			row.Error = "keepalived runtime unavailable"
			if keepErr != nil {
				row.Error = keepErr.Error()
			}
			if keep != nil {
				for _, obs := range keep.Instances {
					if obs.Name == n {
						row.State = strings.ToLower(obs.State)
						if row.State == "" {
							row.State = "unknown"
						}
						row.Error = keep.DumpError
						if obs.Dump == nil {
							row.State = "unknown"
							if row.Error == "" {
								row.Error = "keepalived instance absent from live dump"
							}
						}
						if obs.Dump != nil {
							row.State = strings.ToLower(obs.Dump.State)
							row.CurrentPriority = uint32(obs.Dump.EffectivePriority)
						}
					}
				}
			}
		}
		out.Routers = append(out.Routers, row)
	}
	if err := ctx.Err(); err != nil {
		return nil, status.Error(codes.DeadlineExceeded, err.Error())
	}
	return out, nil
}

// watchVrrp publishes changes in bounded, owner-scoped observations; failed reads erase the baseline.
func (a *Agent) watchVrrp(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	prev := map[string]string{}
	var events <-chan vrrp.Event
	var stopEvents context.CancelFunc
	defer func() {
		if stopEvents != nil {
			stopEvents()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				events = nil
				if stopEvents != nil {
					stopEvents()
					stopEvents = nil
				}
				continue
			}
			a.svc.events().publish(&ngfwv1.Event{Kind: ngfwv1.EventKind_EVENT_KIND_VRRP_STATE_CHANGED, Interface: proto.String(ev.VR.Interface), Message: ev.Key + ": " + ev.NewState, Attributes: map[string]string{"key": ev.Key, "oldState": ev.OldState, "state": ev.NewState, "engine": "vpp"}})
		case <-ticker.C:
			if events != nil && !a.svc.vpp.Connected() {
				stopEvents()
				stopEvents = nil
				events = nil
			}
			if events == nil && a.svc.vpp.Connected() && subsystems.VrrpEnv().VPPEngine {
				ectx, ecancel := context.WithCancel(ctx)
				stream, err := vrrp.WatchEvents(ectx, a.svc.vpp, a.svc.owner)
				if err != nil {
					ecancel()
				} else {
					events = stream
					stopEvents = ecancel
				}
			}
			q, cancel := context.WithTimeout(ctx, 5*time.Second)
			view, err := a.svc.vrrpState(q, &ngfwv1.VrrpStateRequest{Owner: a.svc.owner})
			cancel()
			if err != nil {
				prev = map[string]string{}
				continue
			}
			next := map[string]string{}
			for _, r := range view.Routers {
				if r.State == "unknown" || r.Error != "" {
					continue
				}
				next[r.Name] = r.State
				if prev[r.Name] != r.State {
					a.svc.events().publish(&ngfwv1.Event{Kind: ngfwv1.EventKind_EVENT_KIND_VRRP_STATE_CHANGED, Message: r.Name + ": " + r.State, Attributes: map[string]string{"name": r.Name, "state": r.State, "engine": r.Engine}})
				}
			}
			prev = next
		}
	}
}
