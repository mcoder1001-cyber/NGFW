package nat44ed

import (
	"context"
	"fmt"

	"ngfw/agent/binapi/nat44_ed"
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

// TimeoutsSpec is the session-timeout singleton (nat_set_timeouts / running config).
type TimeoutsSpec struct {
	UDP            uint32 `json:"udp"`
	TCPEstablished uint32 `json:"tcp_established"`
	TCPTransitory  uint32 `json:"tcp_transitory"`
	ICMP           uint32 `json:"icmp"`
}

// ForwardingSpec is the forwarding singleton (nat44_forwarding_enable_disable).
type ForwardingSpec struct {
	Enabled bool `json:"enabled"`
}

func (p *Plugin) newEnable() *natcommon.Descriptor[EnableSpec] {
	return natcommon.New(natcommon.Ops[EnableSpec]{
		Name: NameEnable,
		ID:   func(EnableSpec) string { return Singleton },
		Deps: func(s EnableSpec) []scheduler.Dependency {
			return natcommon.WithVRF(natcommon.WithVRF(nil, s.InsideVRF), s.OutsideVRF)
		},
		Create: func(ctx context.Context, s EnableSpec) (any, error) {
			rc, enabled, err := p.runningConfig(ctx)
			if err != nil {
				return nil, err
			}
			if enabled {
				// Shared VPP: another owner (or an earlier run) enabled the plugin. A compatible
				// configuration is converged; a different one must not be silently replaced.
				if got := enableFromRunning(rc); got == s.withDefaults() {
					return nil, nil
				}
				return nil, fmt.Errorf("%w: plugin already enabled with %+v", ErrForeignObjects, enableFromRunning(rc))
			}
			req := &nat44_ed.Nat44EdPluginEnableDisable{Enable: true, Sessions: s.Sessions, InsideVrf: s.InsideVRF, OutsideVrf: s.OutsideVRF}
			if s.Out2InDPO {
				req.Flags |= nat44_ed.NAT44_IS_OUT2IN_DPO
			}
			if _, err := p.svc.Nat44EdPluginEnableDisable(ctx, req); err != nil && !natcommon.IsAlreadyEnabled(err) {
				return nil, fmt.Errorf("nat44_ed_plugin_enable_disable: %w", err)
			}
			return nil, nil
		},
		// Every field needs disable+enable; refuse while foreign objects would be destroyed.
		Update: func(ctx context.Context, _, _ EnableSpec, _ any) (any, error) {
			foreign, err := p.hasForeignObjects(ctx)
			if err != nil {
				return nil, err
			}
			if foreign {
				return nil, ErrForeignObjects
			}
			return nil, scheduler.ErrRecreate
		},
		Delete: func(ctx context.Context, _ EnableSpec, _ any) error {
			foreign, err := p.hasForeignObjects(ctx)
			if err != nil {
				return err
			}
			if foreign {
				return ErrForeignObjects
			}
			if _, err := p.svc.Nat44EdPluginEnableDisable(ctx, &nat44_ed.Nat44EdPluginEnableDisable{Enable: false}); err != nil && !natcommon.IsAlreadyDisabled(err) {
				return fmt.Errorf("nat44_ed_plugin_enable_disable: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[EnableSpec], error) {
			rc, enabled, err := p.runningConfig(ctx)
			if err != nil || !enabled {
				return nil, err
			}
			return []natcommon.Item[EnableSpec]{{Spec: enableFromRunning(rc)}}, nil
		},
	})
}

// withDefaults fills VPP's defaults the way the running config reports them.
func (s EnableSpec) withDefaults() EnableSpec {
	if s.Sessions == 0 {
		s.Sessions = 63 * 1024
	}
	return s
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

func (p *Plugin) newTimeouts() *natcommon.Descriptor[TimeoutsSpec] {
	return natcommon.New(natcommon.Ops[TimeoutsSpec]{
		Name: NameTimeouts,
		ID:   func(TimeoutsSpec) string { return Singleton },
		Deps: func(TimeoutsSpec) []scheduler.Dependency { return enableDep() },
		Create: func(ctx context.Context, s TimeoutsSpec) (any, error) {
			return nil, p.setTimeouts(ctx, s)
		},
		Update: func(ctx context.Context, _, s TimeoutsSpec, _ any) (any, error) {
			return nil, p.setTimeouts(ctx, s)
		},
		// Delete restores VPP's defaults so a removed object leaves no trace.
		Delete: func(ctx context.Context, _ TimeoutsSpec, _ any) error {
			return p.setTimeouts(ctx, TimeoutsSpec{UDP: DefaultUDPTimeout, TCPEstablished: DefaultTCPEstablishedTimeout, TCPTransitory: DefaultTCPTransitoryTimeout, ICMP: DefaultICMPTimeout})
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[TimeoutsSpec], error) {
			rc, enabled, err := p.runningConfig(ctx)
			if err != nil || !enabled {
				return nil, err
			}
			t := rc.Timeouts
			return []natcommon.Item[TimeoutsSpec]{{Spec: TimeoutsSpec{UDP: t.UDP, TCPEstablished: t.TCPEstablished, TCPTransitory: t.TCPTransitory, ICMP: t.ICMP}}}, nil
		},
	})
}

func (p *Plugin) setForwarding(ctx context.Context, enabled bool) error {
	if _, err := p.svc.Nat44ForwardingEnableDisable(ctx, &nat44_ed.Nat44ForwardingEnableDisable{Enable: enabled}); err != nil {
		return fmt.Errorf("nat44_forwarding_enable_disable: %w", err)
	}
	return nil
}

func (p *Plugin) newForwarding() *natcommon.Descriptor[ForwardingSpec] {
	return natcommon.New(natcommon.Ops[ForwardingSpec]{
		Name:   NameForwarding,
		ID:     func(ForwardingSpec) string { return Singleton },
		Deps:   func(ForwardingSpec) []scheduler.Dependency { return enableDep() },
		Create: func(ctx context.Context, s ForwardingSpec) (any, error) { return nil, p.setForwarding(ctx, s.Enabled) },
		Update: func(ctx context.Context, _, s ForwardingSpec, _ any) (any, error) {
			return nil, p.setForwarding(ctx, s.Enabled)
		},
		Delete: func(ctx context.Context, _ ForwardingSpec, _ any) error { return p.setForwarding(ctx, false) },
		Retrieve: func(ctx context.Context) ([]natcommon.Item[ForwardingSpec], error) {
			rc, enabled, err := p.runningConfig(ctx)
			if err != nil || !enabled {
				return nil, err
			}
			return []natcommon.Item[ForwardingSpec]{{Spec: ForwardingSpec{Enabled: rc.ForwardingEnabled}}}, nil
		},
	})
}
