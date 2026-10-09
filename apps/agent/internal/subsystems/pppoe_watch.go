package subsystems

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"net/netip"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/df6"
	desc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/scheduler"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Polling avoids another dependency and shares the agent's lifetime context.
func (rt *PppoeRuntime) watch(ctx context.Context, syncSource SyncFunc) {
	if err := syncSource(ctx); err != nil && ctx.Err() != nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bound, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := rt.poll(bound); err != nil && ctx.Err() == nil {
				rt.log.Warn("PPPoE hook convergence pending", "reason", err.Error())
			}
			cancel()
		}
	}
}
func (rt *PppoeRuntime) watchDesired(*ngfwv1.DesiredState) []scheduler.KV { return nil }

type pppoeObservation struct{}

func (*pppoeObservation) Name() string { return "pppoe.client.observation" }
func (*pppoeObservation) KeyOf(proto.Message) scheduler.Key {
	return scheduler.Join("pppoe.client.observation", "ngfw")
}
func (*pppoeObservation) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (*pppoeObservation) Create(context.Context, proto.Message) (any, error) {
	return nil, errors.New("observation source produces no objects")
}
func (*pppoeObservation) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, errors.New("observation source produces no objects")
}
func (*pppoeObservation) Delete(context.Context, proto.Message, any) error { return nil }
func (*pppoeObservation) Retrieve(context.Context) ([]scheduler.KV, error) { return nil, nil }

func (rt *PppoeRuntime) poll(ctx context.Context) error {
	run := func(ctx context.Context) error {
		rt.mu.Lock()
		defer rt.mu.Unlock()
		if rt.carrierReady == nil {
			rt.carrierReady = map[string]carrierForwarding{}
		}
		if rt.mirrored == nil {
			rt.mirrored = map[string]desc.Mirror{}
		}
		if rt.failures == nil {
			rt.failures = map[string]pppoeFailure{}
		}
		for name, old := range rt.mirrored {
			s, exists := rt.applied[name]
			next, up, err := rt.observe(s)
			if err != nil && exists {
				delete(rt.carrierReady, name)
				return err
			}
			if !exists || !up || !reflect.DeepEqual(old, next) {
				delete(rt.carrierReady, name)
				if err := rt.Mirror(ctx, old, false); err != nil {
					return err
				}
				delete(rt.mirrored, name)
			}
		}
		for name, s := range rt.applied {
			next, up, err := rt.observe(s)
			if err != nil {
				delete(rt.carrierReady, name)
				return err
			}
			if up {
				var forwarding carrierForwarding
				if s.Carrier != nil {
					forwarding, err = rt.prepareCarrierForwarding(ctx, s, next)
					if err != nil {
						if old, ok := rt.mirrored[name]; ok {
							if e := rt.Mirror(ctx, old, false); e != nil {
								return e
							}
							delete(rt.mirrored, name)
						}
						return err
					}
				}
				rt.failures[name] = pppoeFailure{}
				// Reassert on every observation: address addition is dump-idempotent,
				// route addition updates only this session path. Repairs VPP loss.
				// Track before writes: a partial failure must remain withdrawable.
				rt.mirrored[name] = next
				if err := rt.Mirror(ctx, next, true); err != nil {
					return err
				}
				if s.Carrier != nil {
					rt.carrierReady[name] = forwarding
				}
			} else if rt.globalsOwner {
				output, err := rt.runner.Run(ctx, renderers.Command{Path: pppoe.SystemctlBin, Args: []string{"show", carrierUnit(s), "-p", "ExecMainStatus", "-p", "NRestarts"}})
				if err != nil {
					return errors.New("PPPoE unit status unavailable")
				}
				rt.failures[name] = observeExit(rt.failures[name], string(output.Stdout))
			}
		}
		return nil
	}
	if rt.exclusive != nil {
		return rt.exclusive(ctx, run)
	}
	return run(ctx)
}

// observe reads a session's hook state (IPv4 ip-up/ip-down and, when IPv6 is on, ipv6-up/ipv6-down) and returns
// the mirror it implies and whether the session is up (either NCP).
func (rt *PppoeRuntime) observe(s pppoe.Session) (desc.Mirror, bool, error) {
	st, err := rt.sessionRenderer(s).ReadSessionState(s.HostIf, 0, "", s.IPv6Enabled())
	if err != nil {
		return desc.Mirror{}, false, err
	}
	var v6 pppoe.IPv6State
	if s.IPv6Enabled() {
		if v6, err = rt.sessionRenderer(s).ReadIPv6(s.HostIf); err != nil {
			return desc.Mirror{}, false, err
		}
	}
	return mirrorFor(s, st, v6), st.GetPhase() == "up", nil
}

func mirrorFor(s pppoe.Session, st *ngfwv1.PppoeSessionState, v6 pppoe.IPv6State) desc.Mirror {
	m := desc.Mirror{Interface: s.Iface, LocalIPv4: st.GetLocalIpv4(), PeerIPv4: st.GetPeerIpv4(), DefaultRoute: s.DefaultRoute, MSSClamp: s.MSSClamp, MTU: s.MTU}
	if s.IPv6Enabled() && v6.Up {
		if addrs := v6.HostAddrs(); len(addrs) > 0 {
			m.LocalIPv6 = addrs
		}
		if v6.Gateway.IsValid() {
			m.PeerIPv6 = v6.Gateway.String()
		}
	}
	if s.Carrier != nil {
		if m.LocalIPv4 != "" {
			m.PeerIPv4 = netip.MustParsePrefix(s.Carrier.Host4).Addr().String()
		}
		if len(m.LocalIPv6) > 0 {
			m.PeerIPv6 = netip.MustParsePrefix(s.Carrier.Host6).Addr().String()
		}
	}
	return m
}

type pppoeFailure struct {
	count, restarts, code uint32
	message               string
	seen                  bool
}

func observeExit(old pppoeFailure, text string) pppoeFailure {
	values := map[string]uint32{}
	for _, line := range strings.Split(text, "\n") {
		key, raw, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		n, err := strconv.ParseUint(raw, 10, 32)
		if err == nil {
			values[key] = uint32(n)
		}
	}
	code, ok := values["ExecMainStatus"]
	if !ok || code == 0 {
		return old
	}
	restarts := values["NRestarts"]
	if !old.seen || old.code != code || old.restarts != restarts {
		delta := uint32(1)
		if old.seen && restarts > old.restarts {
			delta = restarts - old.restarts
		}
		if ^uint32(0)-old.count < delta {
			old.count = ^uint32(0)
		} else {
			old.count += delta
		}
	}
	old.code = code
	old.restarts = restarts
	old.seen = true
	messages := map[uint32]string{1: "fatal PPP error", 2: "invalid PPP options", 3: "insufficient PPP privileges", 4: "PPP kernel support unavailable", 5: "PPP terminated", 6: "serial port locked", 7: "serial port open failed", 8: "connect script failed", 10: "PPP negotiation failed", 11: "peer authentication failed", 15: "peer did not respond", 16: "modem hangup", 19: "authentication to peer failed"}
	old.message = messages[code]
	if old.message == "" {
		old.message = fmt.Sprintf("pppd exited with status %d", code)
	}
	return old
}
func (rt *PppoeRuntime) runtimeAddresses(ctx context.Context, addrs map[uint32]map[string]bool) (map[uint32]map[string]bool, error) {
	rt.mu.Lock()
	sessions := make([]pppoe.Session, 0, len(rt.applied))
	for _, s := range rt.applied {
		sessions = append(sessions, s)
	}
	rt.mu.Unlock()
	if len(sessions) == 0 {
		return addrs, nil
	}
	ifs, err := df6.DumpInterfaces(ctx, rt.vpp, rt.owner)
	if err != nil {
		return nil, err
	}
	if addrs == nil {
		addrs = map[uint32]map[string]bool{}
	}
	for _, s := range sessions {
		m, up, err := rt.observe(s)
		if err != nil {
			return nil, err
		}
		if !up {
			continue
		}
		idx, err := ifs.Index(s.Iface)
		if err != nil {
			continue
		}
		for _, raw := range append([]string{m.LocalIPv4}, m.LocalIPv6...) {
			p, err := netip.ParsePrefix(raw)
			if err != nil {
				continue
			}
			if addrs[uint32(idx)] == nil {
				addrs[uint32(idx)] = map[string]bool{}
			}
			addrs[uint32(idx)][p.Addr().String()] = true
		}
	}
	return addrs, nil
}

func (*pppoeObservation) RecordsNoOwnership() {}
