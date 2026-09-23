package nat44ei

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sort"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// VPP's default session timeouts (nat44_ei.c), restored by TimeoutsSpec Delete.
const (
	DefaultUDPTimeout            = 300
	DefaultTCPEstablishedTimeout = 7440
	DefaultTCPTransitoryTimeout  = 240
	DefaultICMPTimeout           = 60
)

// EnableSpec is the nat44-ei plugin singleton (nat44_ei_plugin_enable_disable): inside /
// outside VRF and the flags VPP accepts. Session limits are startup.conf / CLI settings in
// nat44-ei (not in the API enable message) and therefore not modelled.
type EnableSpec struct {
	InsideVRF          uint32 `json:"inside_vrf"`
	OutsideVRF         uint32 `json:"outside_vrf"`
	StaticMappingOnly  bool   `json:"static_mapping_only"`
	ConnectionTracking bool   `json:"connection_tracking"`
	Out2InDPO          bool   `json:"out2in_dpo"`
}

func enableFromRunning(rc *nat44_ei.Nat44EiShowRunningConfigReply) EnableSpec {
	return EnableSpec{
		InsideVRF: rc.InsideVrf, OutsideVRF: rc.OutsideVrf,
		StaticMappingOnly:  rc.Flags&nat44_ei.NAT44_EI_STATIC_MAPPING_ONLY != 0,
		ConnectionTracking: rc.Flags&nat44_ei.NAT44_EI_CONNECTION_TRACKING != 0,
		Out2InDPO:          rc.Flags&nat44_ei.NAT44_EI_OUT2IN_DPO != 0,
	}
}

// TimeoutsSpec is the session-timeout singleton (nat44_ei_set_timeouts / running config).
type TimeoutsSpec struct {
	UDP            uint32 `json:"udp"`
	TCPEstablished uint32 `json:"tcp_established"`
	TCPTransitory  uint32 `json:"tcp_transitory"`
	ICMP           uint32 `json:"icmp"`
}

// ForwardingSpec is the forwarding singleton (nat44_ei_forwarding_enable_disable); presence
// means enabled (VPP's default, off, is "no object").
type ForwardingSpec struct{}

// DefaultTimeouts are VPP's built-in values; a TimeoutsSpec equal to them is "no object".
var DefaultTimeouts = TimeoutsSpec{UDP: DefaultUDPTimeout, TCPEstablished: DefaultTCPEstablishedTimeout, TCPTransitory: DefaultTCPTransitoryTimeout, ICMP: DefaultICMPTimeout}

// IpfixSpec is the IPFIX logging singleton (nat44_ei_ipfix_enable_disable). The exporter
// itself is DF-8's object; this only flips the plugin's logging.
type IpfixSpec struct {
	DomainID uint32 `json:"domain_id"`
	SrcPort  uint32 `json:"src_port"`
}

// InterfaceFeatureSpec enables nat44-ei on one side of an interface.
type InterfaceFeatureSpec struct {
	Interface string `json:"interface"`
	Side      string `json:"side"`
}

// OutputFeatureSpec puts an interface on the output-feature path
// (nat44_ei_add_del_output_interface). The older per-side
// nat44_ei_interface_add_del_output_feature is a shim whose dump does not report the
// interface back, so it is not used.
type OutputFeatureSpec struct {
	Interface string `json:"interface"`
}

// InterfaceAddressSpec uses the interface's address as pool address.
type InterfaceAddressSpec struct {
	Interface string `json:"interface"`
}

// AddressPoolSpec is a contiguous pool range (nat44_ei_add_del_address_range).
type AddressPoolSpec struct {
	First string `json:"first"`
	Last  string `json:"last"`
	VRF   uint32 `json:"vrf"`
}

// Normalize canonicalises the range.
func (s *AddressPoolSpec) Normalize() {
	s.First, s.Last = natcommon.CanonAddr(s.First), natcommon.CanonAddr(s.Last)
	if s.Last == "" {
		s.Last = s.First
	}
	a, errA := netip.ParseAddr(s.First)
	b, errB := netip.ParseAddr(s.Last)
	if errA == nil && errB == nil && b.Less(a) {
		s.First, s.Last = s.Last, s.First
	}
}

// Endpoint is an ip:port pair; External may name an interface instead.
type Endpoint struct {
	IP        string `json:"ip"`
	Port      uint32 `json:"port"`
	Interface string `json:"interface"`
}

// StaticMappingSpec is one nat44-ei static mapping (nat44_ei_add_del_static_mapping), tagged
// "<owner>:<name>".
type StaticMappingSpec struct {
	Name     string   `json:"name"`
	Local    Endpoint `json:"local"`
	External Endpoint `json:"external"`
	Protocol string   `json:"protocol"`
	VRF      uint32   `json:"vrf"`
	AddrOnly bool     `json:"addr_only"`
}

// Normalize canonicalises the mapping.
func (s *StaticMappingSpec) Normalize() {
	s.Local.IP, s.External.IP = natcommon.CanonAddr(s.Local.IP), natcommon.CanonAddr(s.External.IP)
	s.Protocol = natcommon.CanonProto(s.Protocol)
	if s.AddrOnly {
		s.Local.Port, s.External.Port, s.Protocol = 0, 0, "any"
	}
	if s.External.Interface != "" {
		s.External.IP = ""
	}
	s.Local.Interface = ""
}

// IdentityMappingSpec maps an address (or interface address) to itself.
type IdentityMappingSpec struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	Interface string `json:"interface"`
	Protocol  string `json:"protocol"`
	Port      uint32 `json:"port"`
	VRF       uint32 `json:"vrf"`
	AddrOnly  bool   `json:"addr_only"`
}

// Normalize canonicalises the identity mapping.
func (s *IdentityMappingSpec) Normalize() {
	s.IP = natcommon.CanonAddr(s.IP)
	s.Protocol = natcommon.CanonProto(s.Protocol)
	if s.AddrOnly {
		s.Port, s.Protocol = 0, "any"
	}
	if s.Interface != "" {
		s.IP = ""
	}
}

func port(p uint32) (uint16, error) {
	if p > 65535 {
		return 0, fmt.Errorf("nat44-ei: port %d out of range", p)
	}
	return uint16(p), nil
}

func sideFlag(side string) (nat44_ei.Nat44EiConfigFlags, error) {
	switch side {
	case SideInside:
		return nat44_ei.NAT44_EI_IF_INSIDE, nil
	case SideOutside:
		return nat44_ei.NAT44_EI_IF_OUTSIDE, nil
	}
	return 0, fmt.Errorf("nat44-ei: side must be %q or %q, got %q", SideInside, SideOutside, side)
}

// ---- singletons ---------------------------------------------------------------------------

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
				if enableFromRunning(rc) == s {
					return nil, nil
				}
				return nil, fmt.Errorf("%w: plugin already enabled with %+v", ErrForeignObjects, enableFromRunning(rc))
			}
			req := &nat44_ei.Nat44EiPluginEnableDisable{Enable: true, InsideVrf: s.InsideVRF, OutsideVrf: s.OutsideVRF}
			// sessions/user_sessions are startup-conf/CLI settings in nat44_ei; the API enable
			// carries only VRFs and flags — the defaults are reported back by the running config.
			if s.StaticMappingOnly {
				req.Flags |= nat44_ei.NAT44_EI_STATIC_MAPPING_ONLY
			}
			if s.ConnectionTracking {
				req.Flags |= nat44_ei.NAT44_EI_CONNECTION_TRACKING
			}
			if s.Out2InDPO {
				req.Flags |= nat44_ei.NAT44_EI_OUT2IN_DPO
			}
			if _, err := p.svc.Nat44EiPluginEnableDisable(ctx, req); err != nil && !natcommon.IsAlreadyEnabled(err) {
				return nil, fmt.Errorf("nat44_ei_plugin_enable_disable: %w", err)
			}
			return nil, nil
		},
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
			if _, err := p.svc.Nat44EiPluginEnableDisable(ctx, &nat44_ei.Nat44EiPluginEnableDisable{Enable: false}); err != nil && !natcommon.IsAlreadyDisabled(err) {
				return fmt.Errorf("nat44_ei_plugin_enable_disable: %w", err)
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

func (p *Plugin) setTimeouts(ctx context.Context, s TimeoutsSpec) error {
	if _, err := p.svc.Nat44EiSetTimeouts(ctx, &nat44_ei.Nat44EiSetTimeouts{UDP: s.UDP, TCPEstablished: s.TCPEstablished, TCPTransitory: s.TCPTransitory, ICMP: s.ICMP}); err != nil {
		return fmt.Errorf("nat44_ei_set_timeouts: %w", err)
	}
	return nil
}

func (p *Plugin) newTimeouts() *natcommon.Descriptor[TimeoutsSpec] {
	return natcommon.New(natcommon.Ops[TimeoutsSpec]{
		Name:   NameTimeouts,
		ID:     func(TimeoutsSpec) string { return Singleton },
		Deps:   func(TimeoutsSpec) []scheduler.Dependency { return enableDep() },
		Create: func(ctx context.Context, s TimeoutsSpec) (any, error) { return nil, p.setTimeouts(ctx, s) },
		Update: func(ctx context.Context, _, s TimeoutsSpec, _ any) (any, error) {
			return nil, p.setTimeouts(ctx, s)
		},
		Delete: func(ctx context.Context, _ TimeoutsSpec, _ any) error { return p.setTimeouts(ctx, DefaultTimeouts) },
		Retrieve: func(ctx context.Context) ([]natcommon.Item[TimeoutsSpec], error) {
			rc, enabled, err := p.runningConfig(ctx)
			if err != nil || !enabled {
				return nil, err
			}
			t := TimeoutsSpec{UDP: rc.Timeouts.UDP, TCPEstablished: rc.Timeouts.TCPEstablished, TCPTransitory: rc.Timeouts.TCPTransitory, ICMP: rc.Timeouts.ICMP}
			if t == DefaultTimeouts {
				return nil, nil
			}
			return []natcommon.Item[TimeoutsSpec]{{Spec: t}}, nil
		},
	})
}

func (p *Plugin) setForwarding(ctx context.Context, enabled bool) error {
	if _, err := p.svc.Nat44EiForwardingEnableDisable(ctx, &nat44_ei.Nat44EiForwardingEnableDisable{Enable: enabled}); err != nil {
		return fmt.Errorf("nat44_ei_forwarding_enable_disable: %w", err)
	}
	return nil
}

func (p *Plugin) newForwarding() *natcommon.Descriptor[ForwardingSpec] {
	return natcommon.New(natcommon.Ops[ForwardingSpec]{
		Name:   NameForwarding,
		ID:     func(ForwardingSpec) string { return Singleton },
		Deps:   func(ForwardingSpec) []scheduler.Dependency { return enableDep() },
		Create: func(ctx context.Context, _ ForwardingSpec) (any, error) { return nil, p.setForwarding(ctx, true) },
		Delete: func(ctx context.Context, _ ForwardingSpec, _ any) error { return p.setForwarding(ctx, false) },
		Retrieve: func(ctx context.Context) ([]natcommon.Item[ForwardingSpec], error) {
			rc, enabled, err := p.runningConfig(ctx)
			if err != nil || !enabled || !rc.ForwardingEnabled {
				return nil, err
			}
			return []natcommon.Item[ForwardingSpec]{{Spec: ForwardingSpec{}}}, nil
		},
	})
}

func (p *Plugin) setIpfix(ctx context.Context, s IpfixSpec, enable bool) error {
	sp, err := port(s.SrcPort)
	if err != nil {
		return err
	}
	if _, err := p.svc.Nat44EiIpfixEnableDisable(ctx, &nat44_ei.Nat44EiIpfixEnableDisable{DomainID: s.DomainID, SrcPort: sp, Enable: enable}); err != nil {
		return fmt.Errorf("nat44_ei_ipfix_enable_disable: %w", err)
	}
	return nil
}

// IPFIX: VPP reports only the on/off state; domain id and source port are write-only, so
// Retrieve echoes the cached spec of the last successful call (documented limitation).
func (p *Plugin) newIpfix() *natcommon.Descriptor[IpfixSpec] {
	var last IpfixSpec
	return natcommon.New(natcommon.Ops[IpfixSpec]{
		Name: NameIpfix,
		ID:   func(IpfixSpec) string { return Singleton },
		Deps: func(IpfixSpec) []scheduler.Dependency { return enableDep() },
		Create: func(ctx context.Context, s IpfixSpec) (any, error) {
			if err := p.setIpfix(ctx, s, true); err != nil {
				return nil, err
			}
			last = s
			return nil, nil
		},
		Update: func(ctx context.Context, _, s IpfixSpec, _ any) (any, error) {
			if err := p.setIpfix(ctx, s, true); err != nil {
				return nil, err
			}
			last = s
			return nil, nil
		},
		Delete: func(ctx context.Context, s IpfixSpec, _ any) error {
			if err := p.setIpfix(ctx, s, false); err != nil {
				return err
			}
			last = IpfixSpec{}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[IpfixSpec], error) {
			rc, enabled, err := p.runningConfig(ctx)
			if err != nil || !enabled || !rc.IpfixLoggingEnabled {
				return nil, err
			}
			return []natcommon.Item[IpfixSpec]{{Spec: last}}, nil
		},
	})
}

// ---- interface-bound objects --------------------------------------------------------------

func (p *Plugin) newInterfaceFeature() *natcommon.Descriptor[InterfaceFeatureSpec] {
	return natcommon.New(natcommon.Ops[InterfaceFeatureSpec]{
		Name: NameInterfaceFeature,
		ID:   func(s InterfaceFeatureSpec) string { return s.Interface + "/" + s.Side },
		Deps: func(s InterfaceFeatureSpec) []scheduler.Dependency { return ifDeps(s.Interface) },
		Create: func(ctx context.Context, s InterfaceFeatureSpec) (any, error) {
			flag, err := sideFlag(s.Side)
			if err != nil {
				return nil, err
			}
			idx, err := natcommon.ResolveInterface(ctx, p.client, s.Interface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat44EiInterfaceAddDelFeature(ctx, &nat44_ei.Nat44EiInterfaceAddDelFeature{IsAdd: true, Flags: flag, SwIfIndex: idx}); err != nil {
				return nil, fmt.Errorf("nat44_ei_interface_add_del_feature: %w", err)
			}
			return IfMeta{SwIfIndex: uint32(idx)}, nil
		},
		Delete: func(ctx context.Context, s InterfaceFeatureSpec, meta any) error {
			flag, err := sideFlag(s.Side)
			if err != nil {
				return err
			}
			m, ok := meta.(IfMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameInterfaceFeature, meta)
			}
			if _, err := p.svc.Nat44EiInterfaceAddDelFeature(ctx, &nat44_ei.Nat44EiInterfaceAddDelFeature{IsAdd: false, Flags: flag, SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat44_ei_interface_add_del_feature: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[InterfaceFeatureSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			stream, err := p.svc.Nat44EiInterfaceDump(ctx, &nat44_ei.Nat44EiInterfaceDump{})
			if err != nil {
				return nil, fmt.Errorf("nat44_ei_interface_dump: %w", err)
			}
			var out []natcommon.Item[InterfaceFeatureSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat44_ei_interface_dump: %w", err)
				}
				i, _ := ifaces.ByIndex(uint32(d.SwIfIndex))
				if !p.scope.OwnsInterface(i) {
					continue
				}
				meta := IfMeta{SwIfIndex: uint32(d.SwIfIndex)}
				if d.Flags&nat44_ei.NAT44_EI_IF_INSIDE != 0 {
					out = append(out, natcommon.Item[InterfaceFeatureSpec]{Spec: InterfaceFeatureSpec{Interface: i.Name, Side: SideInside}, Meta: meta})
				}
				if d.Flags&nat44_ei.NAT44_EI_IF_OUTSIDE != 0 {
					out = append(out, natcommon.Item[InterfaceFeatureSpec]{Spec: InterfaceFeatureSpec{Interface: i.Name, Side: SideOutside}, Meta: meta})
				}
			}
		},
	})
}

func (p *Plugin) newOutputFeature() *natcommon.Descriptor[OutputFeatureSpec] {
	return natcommon.New(natcommon.Ops[OutputFeatureSpec]{
		Name: NameOutputFeature,
		ID:   func(s OutputFeatureSpec) string { return s.Interface },
		Deps: func(s OutputFeatureSpec) []scheduler.Dependency { return ifDeps(s.Interface) },
		Create: func(ctx context.Context, s OutputFeatureSpec) (any, error) {
			idx, err := natcommon.ResolveInterface(ctx, p.client, s.Interface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat44EiAddDelOutputInterface(ctx, &nat44_ei.Nat44EiAddDelOutputInterface{IsAdd: true, SwIfIndex: idx}); err != nil {
				return nil, fmt.Errorf("nat44_ei_add_del_output_interface: %w", err)
			}
			return IfMeta{SwIfIndex: uint32(idx)}, nil
		},
		Delete: func(ctx context.Context, _ OutputFeatureSpec, meta any) error {
			m, ok := meta.(IfMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameOutputFeature, meta)
			}
			if _, err := p.svc.Nat44EiAddDelOutputInterface(ctx, &nat44_ei.Nat44EiAddDelOutputInterface{IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat44_ei_add_del_output_interface: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[OutputFeatureSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			var out []natcommon.Item[OutputFeatureSpec]
			cursor := uint32(0)
			for {
				stream, err := p.svc.Nat44EiOutputInterfaceGet(ctx, &nat44_ei.Nat44EiOutputInterfaceGet{Cursor: cursor})
				if err != nil {
					return nil, fmt.Errorf("nat44_ei_output_interface_get: %w", err)
				}
				again := false
				for {
					d, rep, err := stream.Recv()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						if rv, ok := natcommon.Retval(err); ok && rv == -165 && rep != nil { // EAGAIN: continue from cursor
							cursor, again = rep.Cursor, true
							break
						}
						return nil, fmt.Errorf("nat44_ei_output_interface_get: %w", err)
					}
					i, _ := ifaces.ByIndex(uint32(d.SwIfIndex))
					if !p.scope.OwnsInterface(i) {
						continue
					}
					out = append(out, natcommon.Item[OutputFeatureSpec]{Spec: OutputFeatureSpec{Interface: i.Name}, Meta: IfMeta{SwIfIndex: uint32(d.SwIfIndex)}})
				}
				if !again {
					return out, nil
				}
			}
		},
	})
}

func (p *Plugin) newInterfaceAddress() *natcommon.Descriptor[InterfaceAddressSpec] {
	return natcommon.New(natcommon.Ops[InterfaceAddressSpec]{
		Name: NameInterfaceAddress,
		ID:   func(s InterfaceAddressSpec) string { return s.Interface },
		Deps: func(s InterfaceAddressSpec) []scheduler.Dependency { return ifDeps(s.Interface) },
		Create: func(ctx context.Context, s InterfaceAddressSpec) (any, error) {
			idx, err := natcommon.ResolveInterface(ctx, p.client, s.Interface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat44EiAddDelInterfaceAddr(ctx, &nat44_ei.Nat44EiAddDelInterfaceAddr{IsAdd: true, SwIfIndex: idx}); err != nil {
				return nil, fmt.Errorf("nat44_ei_add_del_interface_addr: %w", err)
			}
			return IfMeta{SwIfIndex: uint32(idx)}, nil
		},
		Delete: func(ctx context.Context, _ InterfaceAddressSpec, meta any) error {
			m, ok := meta.(IfMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameInterfaceAddress, meta)
			}
			if _, err := p.svc.Nat44EiAddDelInterfaceAddr(ctx, &nat44_ei.Nat44EiAddDelInterfaceAddr{IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat44_ei_add_del_interface_addr: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[InterfaceAddressSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			stream, err := p.svc.Nat44EiInterfaceAddrDump(ctx, &nat44_ei.Nat44EiInterfaceAddrDump{})
			if err != nil {
				return nil, fmt.Errorf("nat44_ei_interface_addr_dump: %w", err)
			}
			var out []natcommon.Item[InterfaceAddressSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat44_ei_interface_addr_dump: %w", err)
				}
				i, _ := ifaces.ByIndex(uint32(d.SwIfIndex))
				if !p.scope.OwnsInterface(i) {
					continue
				}
				out = append(out, natcommon.Item[InterfaceAddressSpec]{Spec: InterfaceAddressSpec{Interface: i.Name}, Meta: IfMeta{SwIfIndex: uint32(d.SwIfIndex)}})
			}
		},
	})
}

// ---- pools ----------------------------------------------------------------------------------

func (p *Plugin) addDelRange(ctx context.Context, s AddressPoolSpec, add bool) error {
	first, err := natcommon.IP4(s.First)
	if err != nil {
		return err
	}
	last, err := natcommon.IP4(s.Last)
	if err != nil {
		return err
	}
	if _, err := p.svc.Nat44EiAddDelAddressRange(ctx, &nat44_ei.Nat44EiAddDelAddressRange{FirstIPAddress: first, LastIPAddress: last, VrfID: s.VRF, IsAdd: add}); err != nil {
		if !add && natcommon.IsNoSuchEntry(err) {
			return nil
		}
		return fmt.Errorf("nat44_ei_add_del_address_range: %w", err)
	}
	return nil
}

type poolAddr struct {
	addr netip.Addr
	vrf  uint32
}

func mergeRanges(addrs []poolAddr) []AddressPoolSpec {
	sort.Slice(addrs, func(i, j int) bool {
		if addrs[i].vrf != addrs[j].vrf {
			return addrs[i].vrf < addrs[j].vrf
		}
		return addrs[i].addr.Less(addrs[j].addr)
	})
	var out []AddressPoolSpec
	for i := 0; i < len(addrs); {
		j := i
		for j+1 < len(addrs) && addrs[j+1].vrf == addrs[i].vrf && addrs[j+1].addr == addrs[j].addr.Next() {
			j++
		}
		out = append(out, AddressPoolSpec{First: addrs[i].addr.String(), Last: addrs[j].addr.String(), VRF: addrs[i].vrf})
		i = j + 1
	}
	return out
}

func (p *Plugin) newAddressPool() *natcommon.Descriptor[AddressPoolSpec] {
	return natcommon.New(natcommon.Ops[AddressPoolSpec]{
		Name: NameAddressPool,
		ID:   func(s AddressPoolSpec) string { return fmt.Sprintf("%s-%s/%d", s.First, s.Last, s.VRF) },
		Deps: func(s AddressPoolSpec) []scheduler.Dependency {
			deps := enableDep()
			if s.VRF != ^uint32(0) {
				deps = natcommon.WithVRF(deps, s.VRF)
			}
			return deps
		},
		Create: func(ctx context.Context, s AddressPoolSpec) (any, error) { return nil, p.addDelRange(ctx, s, true) },
		Delete: func(ctx context.Context, s AddressPoolSpec, _ any) error { return p.addDelRange(ctx, s, false) },
		Retrieve: func(ctx context.Context) ([]natcommon.Item[AddressPoolSpec], error) {
			stream, err := p.svc.Nat44EiAddressDump(ctx, &nat44_ei.Nat44EiAddressDump{})
			if err != nil {
				return nil, fmt.Errorf("nat44_ei_address_dump: %w", err)
			}
			var ifAddrs map[netip.Addr]bool
			var addrs []poolAddr
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return nil, fmt.Errorf("nat44_ei_address_dump: %w", err)
				}
				a := netip.AddrFrom4(d.IPAddress)
				if !p.scope.OwnsAddr(a) {
					continue
				}
				if ifAddrs == nil {
					if ifAddrs, err = p.interfacePoolAddresses(ctx); err != nil {
						return nil, err
					}
				}
				if !ifAddrs[a] {
					addrs = append(addrs, poolAddr{addr: a, vrf: d.VrfID})
				}
			}
			var out []natcommon.Item[AddressPoolSpec]
			for _, s := range mergeRanges(addrs) {
				out = append(out, natcommon.Item[AddressPoolSpec]{Spec: s})
			}
			return out, nil
		},
	})
}

// interfacePoolAddresses is the set of IPv4 addresses of interfaces registered with
// nat44_ei_add_del_interface_addr — VPP lists them in nat44_ei_address_dump like pool addresses.
func (p *Plugin) interfacePoolAddresses(ctx context.Context) (map[netip.Addr]bool, error) {
	stream, err := p.svc.Nat44EiInterfaceAddrDump(ctx, &nat44_ei.Nat44EiInterfaceAddrDump{})
	if err != nil {
		return nil, fmt.Errorf("nat44_ei_interface_addr_dump: %w", err)
	}
	var idxs []uint32
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("nat44_ei_interface_addr_dump: %w", err)
		}
		idxs = append(idxs, uint32(d.SwIfIndex))
	}
	return natcommon.InterfaceAddresses(ctx, p.client, idxs)
}

// ---- mappings -------------------------------------------------------------------------------

func ip4OrZero(s string) (ip_types.IP4Address, error) {
	if s == "" {
		return ip_types.IP4Address{}, nil
	}
	return natcommon.IP4(s)
}

// MappingMeta remembers the external interface a mapping was created with.
type MappingMeta struct{ ExternalSwIfIndex uint32 }

func (p *Plugin) staticRequest(ctx context.Context, s StaticMappingSpec, add bool, extIdx interface_types.InterfaceIndex) (*nat44_ei.Nat44EiAddDelStaticMapping, error) {
	tag, err := p.scope.Tag(s.Name)
	if err != nil {
		return nil, err
	}
	local, err := natcommon.IP4(s.Local.IP)
	if err != nil {
		return nil, fmt.Errorf("local: %w", err)
	}
	ext, err := ip4OrZero(s.External.IP)
	if err != nil {
		return nil, fmt.Errorf("external: %w", err)
	}
	proto, err := natcommon.ProtoNumber(s.Protocol)
	if err != nil {
		return nil, err
	}
	lp, err := port(s.Local.Port)
	if err != nil {
		return nil, err
	}
	ep, err := port(s.External.Port)
	if err != nil {
		return nil, err
	}
	if s.External.Interface != "" && extIdx == noInterface {
		if extIdx, err = natcommon.ResolveInterface(ctx, p.client, s.External.Interface); err != nil {
			return nil, err
		}
	} else if s.External.Interface == "" {
		extIdx = noInterface
	}
	var flags nat44_ei.Nat44EiConfigFlags
	if s.AddrOnly {
		flags |= nat44_ei.NAT44_EI_ADDR_ONLY_MAPPING
	}
	return &nat44_ei.Nat44EiAddDelStaticMapping{IsAdd: add, Flags: flags, LocalIPAddress: local, ExternalIPAddress: ext, Protocol: proto, LocalPort: lp, ExternalPort: ep, ExternalSwIfIndex: extIdx, VrfID: s.VRF, Tag: tag}, nil
}

func (p *Plugin) newStaticMapping() *natcommon.Descriptor[StaticMappingSpec] {
	return natcommon.New(natcommon.Ops[StaticMappingSpec]{
		Name: NameStaticMapping,
		ID:   func(s StaticMappingSpec) string { return s.Name },
		Deps: func(s StaticMappingSpec) []scheduler.Dependency {
			deps := natcommon.WithVRF(enableDep(), s.VRF)
			if s.External.Interface != "" {
				deps = append(deps, natcommon.InterfaceDep(s.External.Interface))
			}
			return deps
		},
		Create: func(ctx context.Context, s StaticMappingSpec) (any, error) {
			req, err := p.staticRequest(ctx, s, true, noInterface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat44EiAddDelStaticMapping(ctx, req); err != nil {
				return nil, fmt.Errorf("nat44_ei_add_del_static_mapping: %w", err)
			}
			return MappingMeta{ExternalSwIfIndex: uint32(req.ExternalSwIfIndex)}, nil
		},
		Delete: func(ctx context.Context, s StaticMappingSpec, meta any) error {
			m, ok := meta.(MappingMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameStaticMapping, meta)
			}
			req, err := p.staticRequest(ctx, s, false, interface_types.InterfaceIndex(m.ExternalSwIfIndex))
			if err != nil {
				return err
			}
			if _, err := p.svc.Nat44EiAddDelStaticMapping(ctx, req); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat44_ei_add_del_static_mapping: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[StaticMappingSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			stream, err := p.svc.Nat44EiStaticMappingDump(ctx, &nat44_ei.Nat44EiStaticMappingDump{})
			if err != nil {
				return nil, fmt.Errorf("nat44_ei_static_mapping_dump: %w", err)
			}
			var out []natcommon.Item[StaticMappingSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat44_ei_static_mapping_dump: %w", err)
				}
				name, ok := p.scope.ParseTag(d.Tag)
				if !ok {
					continue
				}
				s := StaticMappingSpec{Name: name, Local: Endpoint{IP: natcommon.IP4String(d.LocalIPAddress), Port: uint32(d.LocalPort)},
					External: Endpoint{IP: natcommon.IP4String(d.ExternalIPAddress), Port: uint32(d.ExternalPort)},
					Protocol: natcommon.ProtoName(d.Protocol), VRF: d.VrfID, AddrOnly: d.Flags&nat44_ei.NAT44_EI_ADDR_ONLY_MAPPING != 0}
				if d.ExternalSwIfIndex != noInterface {
					s.External.Interface = ifaces.Name(uint32(d.ExternalSwIfIndex))
				}
				s.Normalize()
				out = append(out, natcommon.Item[StaticMappingSpec]{Spec: s, Meta: MappingMeta{ExternalSwIfIndex: uint32(d.ExternalSwIfIndex)}})
			}
		},
	})
}

func (p *Plugin) identityRequest(ctx context.Context, s IdentityMappingSpec, add bool, idx interface_types.InterfaceIndex) (*nat44_ei.Nat44EiAddDelIdentityMapping, error) {
	tag, err := p.scope.Tag(s.Name)
	if err != nil {
		return nil, err
	}
	addr, err := ip4OrZero(s.IP)
	if err != nil {
		return nil, err
	}
	proto, err := natcommon.ProtoNumber(s.Protocol)
	if err != nil {
		return nil, err
	}
	pt, err := port(s.Port)
	if err != nil {
		return nil, err
	}
	if s.Interface != "" && idx == noInterface {
		if idx, err = natcommon.ResolveInterface(ctx, p.client, s.Interface); err != nil {
			return nil, err
		}
	} else if s.Interface == "" {
		idx = noInterface
	}
	var flags nat44_ei.Nat44EiConfigFlags
	if s.AddrOnly {
		flags |= nat44_ei.NAT44_EI_ADDR_ONLY_MAPPING
	}
	return &nat44_ei.Nat44EiAddDelIdentityMapping{IsAdd: add, Flags: flags, IPAddress: addr, Protocol: proto, Port: pt, SwIfIndex: idx, VrfID: s.VRF, Tag: tag}, nil
}

func (p *Plugin) newIdentityMapping() *natcommon.Descriptor[IdentityMappingSpec] {
	return natcommon.New(natcommon.Ops[IdentityMappingSpec]{
		Name: NameIdentityMapping,
		ID:   func(s IdentityMappingSpec) string { return s.Name },
		Deps: func(s IdentityMappingSpec) []scheduler.Dependency {
			deps := natcommon.WithVRF(enableDep(), s.VRF)
			if s.Interface != "" {
				deps = append(deps, natcommon.InterfaceDep(s.Interface))
			}
			return deps
		},
		Create: func(ctx context.Context, s IdentityMappingSpec) (any, error) {
			req, err := p.identityRequest(ctx, s, true, noInterface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat44EiAddDelIdentityMapping(ctx, req); err != nil {
				return nil, fmt.Errorf("nat44_ei_add_del_identity_mapping: %w", err)
			}
			return IfMeta{SwIfIndex: uint32(req.SwIfIndex)}, nil
		},
		Delete: func(ctx context.Context, s IdentityMappingSpec, meta any) error {
			m, ok := meta.(IfMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameIdentityMapping, meta)
			}
			req, err := p.identityRequest(ctx, s, false, interface_types.InterfaceIndex(m.SwIfIndex))
			if err != nil {
				return err
			}
			if _, err := p.svc.Nat44EiAddDelIdentityMapping(ctx, req); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat44_ei_add_del_identity_mapping: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[IdentityMappingSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			stream, err := p.svc.Nat44EiIdentityMappingDump(ctx, &nat44_ei.Nat44EiIdentityMappingDump{})
			if err != nil {
				return nil, fmt.Errorf("nat44_ei_identity_mapping_dump: %w", err)
			}
			var out []natcommon.Item[IdentityMappingSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat44_ei_identity_mapping_dump: %w", err)
				}
				name, ok := p.scope.ParseTag(d.Tag)
				if !ok {
					continue
				}
				s := IdentityMappingSpec{Name: name, IP: natcommon.IP4String(d.IPAddress), Protocol: natcommon.ProtoName(d.Protocol), Port: uint32(d.Port), VRF: d.VrfID, AddrOnly: d.Flags&nat44_ei.NAT44_EI_ADDR_ONLY_MAPPING != 0}
				if d.SwIfIndex != noInterface {
					s.Interface = ifaces.Name(uint32(d.SwIfIndex))
				}
				s.Normalize()
				out = append(out, natcommon.Item[IdentityMappingSpec]{Spec: s, Meta: IfMeta{SwIfIndex: uint32(d.SwIfIndex)}})
			}
		},
	})
}

// ---- Retrieve-only sessions -----------------------------------------------------------------

// User is one inside host with sessions (nat44_ei_user_dump).
type User struct {
	IP             string `json:"ip"`
	VRF            uint32 `json:"vrf"`
	Sessions       uint32 `json:"sessions"`
	StaticSessions uint32 `json:"static_sessions"`
}

// Session is one translation (nat44_ei_user_session_v2_details).
type Session struct {
	Inside      Endpoint `json:"inside"`
	Outside     Endpoint `json:"outside"`
	ExtHost     Endpoint `json:"ext_host"`
	Protocol    string   `json:"protocol"`
	Static      bool     `json:"static"`
	IdleSeconds uint64   `json:"idle_seconds"`
	TotalBytes  uint64   `json:"total_bytes"`
	TotalPkts   uint32   `json:"total_pkts"`
}

// Users lists the inside hosts that currently have sessions.
func (p *Plugin) Users(ctx context.Context) ([]User, error) {
	stream, err := p.svc.Nat44EiUserDump(ctx, &nat44_ei.Nat44EiUserDump{})
	if err != nil {
		return nil, fmt.Errorf("nat44_ei_user_dump: %w", err)
	}
	var out []User
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("nat44_ei_user_dump: %w", err)
		}
		out = append(out, User{IP: natcommon.IP4String(d.IPAddress), VRF: d.VrfID, Sessions: d.Nsessions, StaticSessions: d.Nstaticsessions})
	}
}

// UserSessions returns one page (offset, limit; 0 = all) of one user's sessions.
func (p *Plugin) UserSessions(ctx context.Context, user User, offset, limit int) ([]Session, error) {
	ip, err := natcommon.IP4(user.IP)
	if err != nil {
		return nil, err
	}
	stream, err := p.svc.Nat44EiUserSessionV2Dump(ctx, &nat44_ei.Nat44EiUserSessionV2Dump{IPAddress: ip, VrfID: user.VRF})
	if err != nil {
		return nil, fmt.Errorf("nat44_ei_user_session_v2_dump: %w", err)
	}
	var out []Session
	for i := 0; ; i++ {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("nat44_ei_user_session_v2_dump: %w", err)
		}
		if i < offset || (limit > 0 && len(out) >= limit) {
			continue
		}
		out = append(out, Session{
			Inside:   Endpoint{IP: natcommon.IP4String(d.InsideIPAddress), Port: uint32(d.InsidePort)},
			Outside:  Endpoint{IP: natcommon.IP4String(d.OutsideIPAddress), Port: uint32(d.OutsidePort)},
			ExtHost:  Endpoint{IP: natcommon.IP4String(d.ExtHostAddress), Port: uint32(d.ExtHostPort)},
			Protocol: natcommon.ProtoName(uint8(d.Protocol)), //nolint:gosec // IP protocol numbers are 8-bit
			Static:   d.Flags&nat44_ei.NAT44_EI_STATIC_MAPPING != 0, IdleSeconds: d.TimeSinceLastHeard, TotalBytes: d.TotalBytes, TotalPkts: d.TotalPkts,
		})
	}
}

// DeleteSession removes one session (nat44_ei_del_session) by its inside endpoint.
func (p *Plugin) DeleteSession(ctx context.Context, inside Endpoint, protocol string, vrf uint32, extHost Endpoint) error {
	addr, err := natcommon.IP4(inside.IP)
	if err != nil {
		return err
	}
	proto, err := natcommon.ProtoNumber(protocol)
	if err != nil {
		return err
	}
	pt, err := port(inside.Port)
	if err != nil {
		return err
	}
	req := &nat44_ei.Nat44EiDelSession{Address: addr, Protocol: proto, Port: pt, VrfID: vrf, Flags: nat44_ei.NAT44_EI_IF_INSIDE}
	if extHost.IP != "" {
		ext, err := natcommon.IP4(extHost.IP)
		if err != nil {
			return err
		}
		ep, err := port(extHost.Port)
		if err != nil {
			return err
		}
		req.ExtHostAddress, req.ExtHostPort = ext, ep
	}
	if _, err := p.svc.Nat44EiDelSession(ctx, req); err != nil {
		return fmt.Errorf("nat44_ei_del_session: %w", err)
	}
	return nil
}
