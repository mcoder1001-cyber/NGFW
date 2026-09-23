package nat44ed

import (
	"context"
	"fmt"

	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// VPP's built-in session timeouts (nat44_ed.c), restored by TimeoutsSpec Delete.
const (
	DefaultUDPTimeout            = 300
	DefaultTCPEstablishedTimeout = 7440
	DefaultTCPTransitoryTimeout  = 240
	DefaultICMPTimeout           = 60
)

// EnableSpec is the nat44-ed plugin singleton (nat44_ed_plugin_enable_disable). Sessions 0
// means VPP's default (63*1024 per thread) and is reported back as that number, so callers
// should state it explicitly for a stable diff. static-mapping-only and connection-tracking
// are rejected by VPP 26.06 (VNET_API_ERROR_UNSUPPORTED) and therefore not modelled.
type EnableSpec struct {
	Sessions   uint32 `json:"sessions"`
	InsideVRF  uint32 `json:"inside_vrf"`
	OutsideVRF uint32 `json:"outside_vrf"`
	Out2InDPO  bool   `json:"out2in_dpo"`
}

// DefaultSessions is VPP's per-thread session limit when the enable request says 0.
const DefaultSessions = 63 * 1024

// Normalize fills VPP's default so desired and retrieved carriers are proto.Equal.
func (s *EnableSpec) Normalize() {
	if s.Sessions == 0 {
		s.Sessions = DefaultSessions
	}
}

// TimeoutsSpec is the session-timeout singleton (nat_set_timeouts / running config).
type TimeoutsSpec struct {
	UDP            uint32 `json:"udp"`
	TCPEstablished uint32 `json:"tcp_established"`
	TCPTransitory  uint32 `json:"tcp_transitory"`
	ICMP           uint32 `json:"icmp"`
}

// ForwardingSpec is the forwarding singleton (nat44_forwarding_enable_disable). Presence
// means enabled: VPP's default (off) is "no object", so Retrieve reports it only while on.
type ForwardingSpec struct{}

// DefaultTimeouts are VPP's built-in values; a TimeoutsSpec equal to them is "no object".
var DefaultTimeouts = TimeoutsSpec{UDP: DefaultUDPTimeout, TCPEstablished: DefaultTCPEstablishedTimeout, TCPTransitory: DefaultTCPTransitoryTimeout, ICMP: DefaultICMPTimeout}

// eiEnabled reports whether nat44-ei is enabled (ED and EI are mutually exclusive).
func (p *Plugin) eiEnabled(ctx context.Context) (bool, error) {
	rc, err := nat44_ei.NewServiceClient(p.client).Nat44EiShowRunningConfig(ctx, &nat44_ei.Nat44EiShowRunningConfig{})
	if err != nil {
		return false, fmt.Errorf("nat44_ei_show_running_config: %w", err)
	}
	return rc.Sessions != 0, nil
}

func (p *Plugin) enablePlugin(ctx context.Context, s EnableSpec) error {
	if ei, err := p.eiEnabled(ctx); err != nil {
		return err
	} else if ei {
		return ErrOtherVariant
	}
	req := &nat44_ed.Nat44EdPluginEnableDisable{Enable: true, Sessions: s.Sessions, InsideVrf: s.InsideVRF, OutsideVrf: s.OutsideVRF}
	if s.Out2InDPO {
		req.Flags |= nat44_ed.NAT44_IS_OUT2IN_DPO
	}
	if _, err := p.svc.Nat44EdPluginEnableDisable(ctx, req); err != nil && !natcommon.IsAlreadyEnabled(err) {
		return fmt.Errorf("nat44_ed_plugin_enable_disable: %w", err)
	}
	return nil
}

// disablePlugin disables nat44-ed only when it holds no object of any owner (D-071). It
// reports whether it did; a non-empty plugin is skipped (left enabled), never disabled.
func (p *Plugin) disablePlugin(ctx context.Context) (bool, error) {
	empty, err := p.Empty(ctx)
	if err != nil || !empty {
		return false, err
	}
	if _, err := p.svc.Nat44EdPluginEnableDisable(ctx, &nat44_ed.Nat44EdPluginEnableDisable{Enable: false}); err != nil && !natcommon.IsAlreadyDisabled(err) {
		return false, fmt.Errorf("nat44_ed_plugin_enable_disable: %w", err)
	}
	return true, nil
}

func (p *Plugin) readRunning(ctx context.Context) (*nat44_ed.Nat44ShowRunningConfigReply, bool, error) {
	return p.runningConfig(ctx)
}

// newEnable: VPP-global singleton (D-071). Globals owner: enable (refused while nat44-ei is
// on); Update = disable+enable, only while the plugin is empty (ErrNotEmpty otherwise);
// Delete = disable after the all-owner emptiness check, else skipped (plugin stays enabled).
// Other owners: require the plugin enabled with the desired configuration.
func (p *Plugin) newEnable() *natcommon.Descriptor[EnableSpec] {
	return natcommon.Global(p.cfg, natcommon.GlobalOps[EnableSpec]{
		Name: NameEnable, ID: Singleton,
		Deps: func(s EnableSpec) []scheduler.Dependency {
			return natcommon.WithVRF(natcommon.WithVRF(nil, s.InsideVRF), s.OutsideVRF)
		},
		Read: func(ctx context.Context) (natcommon.GlobalState[EnableSpec], error) {
			rc, enabled, err := p.readRunning(ctx)
			if err != nil {
				return natcommon.GlobalState[EnableSpec]{}, err
			}
			return natcommon.GlobalState[EnableSpec]{Value: enableFromRunning(rc), Present: enabled, Observable: true}, nil
		},
		Set: func(ctx context.Context, s EnableSpec) error {
			rc, enabled, err := p.readRunning(ctx)
			if err != nil {
				return err
			}
			if enabled {
				s.Normalize()
				if enableFromRunning(rc) == s {
					return nil
				}
				return fmt.Errorf("%s: already enabled with %+v (desired %+v); a change needs the plugin empty", NameEnable, enableFromRunning(rc), s)
			}
			return p.enablePlugin(ctx, s)
		},
		SetUpdate: func(ctx context.Context, _, n EnableSpec) error {
			done, err := p.disablePlugin(ctx)
			if err != nil {
				return err
			}
			if !done {
				return fmt.Errorf("%s: changing the enable configuration needs a disable: %w", NameEnable, natcommon.ErrNotEmpty)
			}
			return p.enablePlugin(ctx, n)
		},
		Reset: func(ctx context.Context, _ EnableSpec) error {
			_, err := p.disablePlugin(ctx)
			return err
		},
	})
}

func enableFromRunning(rc *nat44_ed.Nat44ShowRunningConfigReply) EnableSpec {
	return EnableSpec{
		Sessions:   rc.Sessions,
		InsideVRF:  rc.InsideVrf,
		OutsideVRF: rc.OutsideVrf,
		Out2InDPO:  rc.Flags&nat44_ed.NAT44_IS_OUT2IN_DPO != 0,
	}
}

func (p *Plugin) setTimeouts(ctx context.Context, s TimeoutsSpec) error {
	if _, err := p.svc.NatSetTimeouts(ctx, &nat44_ed.NatSetTimeouts{UDP: s.UDP, TCPEstablished: s.TCPEstablished, TCPTransitory: s.TCPTransitory, ICMP: s.ICMP}); err != nil {
		return fmt.Errorf("nat_set_timeouts: %w", err)
	}
	return nil
}

// newTimeouts: VPP-global (D-071). Owner: set / reset to VPP defaults, Retrieve reports
// non-default values. Others: require the desired values.
func (p *Plugin) newTimeouts() *natcommon.Descriptor[TimeoutsSpec] {
	return natcommon.Global(p.cfg, natcommon.GlobalOps[TimeoutsSpec]{
		Name: NameTimeouts, ID: Singleton,
		Deps: func(TimeoutsSpec) []scheduler.Dependency { return enableDep() },
		Read: func(ctx context.Context) (natcommon.GlobalState[TimeoutsSpec], error) {
			rc, enabled, err := p.readRunning(ctx)
			if err != nil {
				return natcommon.GlobalState[TimeoutsSpec]{}, err
			}
			t := TimeoutsSpec{UDP: rc.Timeouts.UDP, TCPEstablished: rc.Timeouts.TCPEstablished, TCPTransitory: rc.Timeouts.TCPTransitory, ICMP: rc.Timeouts.ICMP}
			return natcommon.GlobalState[TimeoutsSpec]{Value: t, Present: enabled, Observable: true}, nil
		},
		Absent: func(t TimeoutsSpec) bool { return t == DefaultTimeouts },
		Set:    p.setTimeouts,
		Reset:  func(ctx context.Context, _ TimeoutsSpec) error { return p.setTimeouts(ctx, DefaultTimeouts) },
	})
}

func (p *Plugin) setForwarding(ctx context.Context, enabled bool) error {
	if _, err := p.svc.Nat44ForwardingEnableDisable(ctx, &nat44_ed.Nat44ForwardingEnableDisable{Enable: enabled}); err != nil {
		return fmt.Errorf("nat44_forwarding_enable_disable: %w", err)
	}
	return nil
}

// newForwarding: VPP-global (D-071); presence = forwarding on.
func (p *Plugin) newForwarding() *natcommon.Descriptor[ForwardingSpec] {
	return natcommon.Global(p.cfg, natcommon.GlobalOps[ForwardingSpec]{
		Name: NameForwarding, ID: Singleton,
		Deps: func(ForwardingSpec) []scheduler.Dependency { return enableDep() },
		Read: func(ctx context.Context) (natcommon.GlobalState[ForwardingSpec], error) {
			rc, enabled, err := p.readRunning(ctx)
			if err != nil {
				return natcommon.GlobalState[ForwardingSpec]{}, err
			}
			return natcommon.GlobalState[ForwardingSpec]{Present: enabled && rc.ForwardingEnabled, Observable: true}, nil
		},
		Set:   func(ctx context.Context, _ ForwardingSpec) error { return p.setForwarding(ctx, true) },
		Reset: func(ctx context.Context, _ ForwardingSpec) error { return p.setForwarding(ctx, false) },
	})
}
