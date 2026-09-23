// Package mapnat holds the descriptors of VPP's MAP plugin (binapi/map, map_plugin.so; the Go
// package is "mapnat" because "map" is a keyword): MAP-E / MAP-T / LW4o6 domains, per-domain
// PSID rules, the global MAP parameters and the per-interface MAP feature. Object <-> message
// table: docs/agent/descriptors/map.md.
package mapnat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/interface_types"
	maps "ngfw/agent/binapi/map"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameDomain    = "map.domain"
	NameRule      = "map.rule"
	NameParams    = "map.params"
	NameInterface = "map.interface"
)

// Singleton is the id of the parameters singleton.
const Singleton = "global"

// ErrNoDomainIndex is returned when a rule's domain cannot be found on the VPP.
var ErrNoDomainIndex = errors.New("map: domain not found")

// DomainSpec is one MAP domain (map_add_domain). Name is the stable id and goes into VPP's
// tag as "<owner>:<name>". MAP-E vs MAP-T is chosen per interface (InterfaceSpec.Translation)
// in VPP 26.06 — map_add_domain carries no flags.
type DomainSpec struct {
	Name       string `json:"name"`
	IP4Prefix  string `json:"ip4_prefix"`
	IP6Prefix  string `json:"ip6_prefix"`
	IP6Src     string `json:"ip6_src"`
	EABitsLen  uint32 `json:"ea_bits_len"`
	PSIDOffset uint32 `json:"psid_offset"`
	PSIDLength uint32 `json:"psid_length"`
	MTU        uint32 `json:"mtu"`
}

// Normalize canonicalises the prefixes.
func (s *DomainSpec) Normalize() {
	s.IP4Prefix, s.IP6Prefix, s.IP6Src = natcommon.CanonPrefix(s.IP4Prefix), natcommon.CanonPrefix(s.IP6Prefix), natcommon.CanonPrefix(s.IP6Src)
}

// RuleSpec is one PSID → IPv6 destination rule of a domain (map_add_del_rule); LW4o6 is a
// MAP-E domain with rules.
type RuleSpec struct {
	Domain string `json:"domain"`
	PSID   uint32 `json:"psid"`
	IP6Dst string `json:"ip6_dst"`
}

// Normalize canonicalises the destination.
func (s *RuleSpec) Normalize() { s.IP6Dst = natcommon.CanonAddr(s.IP6Dst) }

// ParamsSpec is the global MAP parameter singleton (map_param_set_* / map_param_get). TCP MSS
// (map_param_set_tcp) is write-only in 26.06 (not in map_param_get) and not modelled.
type ParamsSpec struct {
	FragInner          bool   `json:"frag_inner"`
	FragIgnoreDF       bool   `json:"frag_ignore_df"`
	ICMPRelaySrc       string `json:"icmp_relay_src"`
	ICMP6Unreachable   bool   `json:"icmp6_unreachable"`
	SecurityCheck      bool   `json:"security_check"`
	SecurityCheckFrags bool   `json:"security_check_frags"`
	TCCopy             bool   `json:"tc_copy"`
	TCClass            uint32 `json:"tc_class"`
}

// Normalize canonicalises the relay source (empty = 0.0.0.0).
func (s *ParamsSpec) Normalize() {
	if s.ICMPRelaySrc == "" || s.ICMPRelaySrc == "0.0.0.0" {
		s.ICMPRelaySrc = ""
	} else {
		s.ICMPRelaySrc = natcommon.CanonAddr(s.ICMPRelaySrc)
	}
}

// DefaultParams are VPP's defaults (map_param_get on a fresh VPP): security check on, TC copy
// on. A ParamsSpec equal to them is "no object".
var DefaultParams = ParamsSpec{SecurityCheck: true, TCCopy: true}

// InterfaceSpec enables MAP on an interface (map_if_enable_disable); Translation selects
// MAP-T (ip4-map-t / ip6-map-t) instead of MAP-E encapsulation.
type InterfaceSpec struct {
	Interface   string `json:"interface"`
	Translation bool   `json:"translation"`
}

// DomainMeta is the domain index VPP assigned.
type DomainMeta struct{ Index uint32 }

// IfMeta is the Meta of the interface object.
type IfMeta struct{ SwIfIndex uint32 }

// Plugin bundles the client, the owner scope and the MAP descriptors.
type Plugin struct {
	client vpp.Client
	scope  natcommon.Scope
	cfg    natcommon.Config
	svc    maps.RPCService
	feat   feature.RPCService

	Domain    *natcommon.Descriptor[DomainSpec]
	Rule      *natcommon.Descriptor[RuleSpec]
	Params    *natcommon.Descriptor[ParamsSpec]
	Interface *natcommon.Descriptor[InterfaceSpec]
}

// New constructs the family for client and owner.
func New(client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := &Plugin{client: client, scope: natcommon.ScopeFor(owner), cfg: natcommon.BuildConfig(opts), svc: maps.NewServiceClient(client), feat: feature.NewServiceClient(client)}
	p.Params = p.newParams()
	p.Domain = p.newDomain()
	p.Rule = p.newRule()
	p.Interface = p.newInterface()
	return p
}

// Descriptors returns the family in registration order.
func (p *Plugin) Descriptors() []scheduler.Descriptor {
	return []scheduler.Descriptor{p.Params, p.Domain, p.Rule, p.Interface}
}

// Register constructs the family and registers every descriptor.
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := New(client, owner, opts...)
	for _, d := range p.Descriptors() {
		r.Register(d)
	}
	return p
}

func (p *Plugin) claims() natcommon.ClaimStore { return p.cfg.Claims }

// DomainKey is the key of the domain called name.
func DomainKey(name string) scheduler.Key { return scheduler.Join(NameDomain, name) }

// ---- domains --------------------------------------------------------------------------------

type domain struct {
	spec  DomainSpec
	index uint32
}

// domains dumps every domain and keeps the owned ones (tag "<owner>:<name>").
func (p *Plugin) domains(ctx context.Context) ([]domain, error) {
	stream, err := p.svc.MapDomainDump(ctx, &maps.MapDomainDump{})
	if err != nil {
		return nil, fmt.Errorf("map_domain_dump: %w", err)
	}
	var out []domain
	seen := map[string]bool{}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("map_domain_dump: %w", err)
		}
		name, ok := p.scope.ParseTag(d.Tag)
		if !ok {
			continue
		}
		// duplicate tags (a retried Create): the lowest index is canonical, extras surface
		// as "<name>#<index>" so the scheduler deletes them (D-066 pattern).
		if seen[name] {
			name = fmt.Sprintf("%s#%d", name, d.DomainIndex)
		}
		seen[name] = true
		s := DomainSpec{Name: name, IP4Prefix: natcommon.Prefix4String(d.IP4Prefix), IP6Prefix: natcommon.Prefix6String(d.IP6Prefix), IP6Src: natcommon.Prefix6String(d.IP6Src),
			EABitsLen: uint32(d.EaBitsLen), PSIDOffset: uint32(d.PsidOffset), PSIDLength: uint32(d.PsidLength), MTU: uint32(d.Mtu)}
		out = append(out, domain{spec: s, index: d.DomainIndex})
	}
}

// domainAt reports whether VPP domain index idx is currently this owner's domain name.
func (p *Plugin) domainAt(ctx context.Context, idx uint32, name string) (bool, error) {
	ds, err := p.domains(ctx)
	if err != nil {
		return false, err
	}
	for _, d := range ds {
		if d.index == idx {
			return d.spec.Name == name, nil
		}
	}
	return false, nil
}

func (p *Plugin) domainIndex(ctx context.Context, name string) (uint32, error) {
	ds, err := p.domains(ctx)
	if err != nil {
		return 0, err
	}
	for _, d := range ds {
		if d.spec.Name == name {
			return d.index, nil
		}
	}
	return 0, fmt.Errorf("%w: %q", ErrNoDomainIndex, name)
}

func u8(v uint32, what string) (uint8, error) {
	if v > 255 {
		return 0, fmt.Errorf("map: %s %d out of range", what, v)
	}
	return uint8(v), nil
}

func (p *Plugin) newDomain() *natcommon.Descriptor[DomainSpec] {
	return natcommon.New(natcommon.Ops[DomainSpec]{
		Claims: p.claims(),
		Name:   NameDomain,
		ID:     func(s DomainSpec) string { return s.Name },
		Create: func(ctx context.Context, s DomainSpec) (any, error) {
			if strings.Contains(s.Name, "#") {
				return nil, fmt.Errorf("map: domain name %q must not contain '#'", s.Name)
			}
			tag, err := p.scope.Tag(s.Name)
			if err != nil {
				return nil, err
			}
			ip4, err := natcommon.Prefix4(s.IP4Prefix)
			if err != nil {
				return nil, fmt.Errorf("ip4_prefix: %w", err)
			}
			ip6, err := natcommon.Prefix6(s.IP6Prefix)
			if err != nil {
				return nil, fmt.Errorf("ip6_prefix: %w", err)
			}
			src, err := natcommon.Prefix6(s.IP6Src)
			if err != nil {
				return nil, fmt.Errorf("ip6_src: %w", err)
			}
			ea, err := u8(s.EABitsLen, "ea_bits_len")
			if err != nil {
				return nil, err
			}
			po, err := u8(s.PSIDOffset, "psid_offset")
			if err != nil {
				return nil, err
			}
			pl, err := u8(s.PSIDLength, "psid_length")
			if err != nil {
				return nil, err
			}
			if s.MTU > 65535 {
				return nil, fmt.Errorf("map: mtu %d out of range", s.MTU)
			}
			rep, err := p.svc.MapAddDomain(ctx, &maps.MapAddDomain{IP6Prefix: ip6, IP4Prefix: ip4, IP6Src: src, EaBitsLen: ea, PsidOffset: po, PsidLength: pl, Mtu: uint16(s.MTU), Tag: tag})
			if err != nil {
				return nil, fmt.Errorf("map_add_domain: %w", err)
			}
			return DomainMeta{Index: rep.Index}, nil
		},
		Delete: func(ctx context.Context, s DomainSpec, meta any) error {
			m, ok := meta.(DomainMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameDomain, meta)
			}
			// D-071 / review finding 3: re-verify identity immediately before deleting by
			// index — the index must still carry this owner's tag for this domain.
			if ok, err := p.domainAt(ctx, m.Index, s.Name); err != nil {
				return err
			} else if !ok {
				return nil // gone, or the index was reused by another domain: nothing of ours
			}
			if _, err := p.svc.MapDelDomain(ctx, &maps.MapDelDomain{Index: m.Index}); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("map_del_domain: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[DomainSpec], error) {
			ds, err := p.domains(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]natcommon.Item[DomainSpec], 0, len(ds))
			for _, d := range ds {
				out = append(out, natcommon.Item[DomainSpec]{Spec: d.spec, Meta: DomainMeta{Index: d.index}})
			}
			return out, nil
		},
	})
}

// ---- rules ----------------------------------------------------------------------------------

func (p *Plugin) newRule() *natcommon.Descriptor[RuleSpec] {
	return natcommon.New(natcommon.Ops[RuleSpec]{
		Claims: p.claims(),
		Name:   NameRule,
		ID:     func(s RuleSpec) string { return fmt.Sprintf("%s/%d", s.Domain, s.PSID) },
		Deps: func(s RuleSpec) []scheduler.Dependency {
			return []scheduler.Dependency{natcommon.Dep(DomainKey(s.Domain))}
		},
		Create: func(ctx context.Context, s RuleSpec) (any, error) {
			idx, err := p.domainIndex(ctx, s.Domain)
			if err != nil {
				return nil, err
			}
			dst, err := natcommon.IP6(s.IP6Dst)
			if err != nil {
				return nil, err
			}
			if s.PSID > 65535 {
				return nil, fmt.Errorf("map: psid %d out of range", s.PSID)
			}
			if _, err := p.svc.MapAddDelRule(ctx, &maps.MapAddDelRule{Index: idx, IsAdd: true, IP6Dst: dst, Psid: uint16(s.PSID)}); err != nil {
				return nil, fmt.Errorf("map_add_del_rule: %w", err)
			}
			return DomainMeta{Index: idx}, nil
		},
		// Changing the destination of a PSID is a re-add in VPP (same index/psid).
		Update: func(ctx context.Context, _, n RuleSpec, meta any) (any, error) {
			m, ok := meta.(DomainMeta)
			if !ok {
				return nil, fmt.Errorf("%s: unexpected meta %T", NameRule, meta)
			}
			dst, err := natcommon.IP6(n.IP6Dst)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.MapAddDelRule(ctx, &maps.MapAddDelRule{Index: m.Index, IsAdd: true, IP6Dst: dst, Psid: uint16(n.PSID)}); err != nil { //nolint:gosec // validated on Create
				return nil, fmt.Errorf("map_add_del_rule: %w", err)
			}
			return m, nil
		},
		Delete: func(ctx context.Context, s RuleSpec, meta any) error {
			m, ok := meta.(DomainMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameRule, meta)
			}
			if ok, err := p.domainAt(ctx, m.Index, s.Domain); err != nil {
				return err
			} else if !ok {
				return nil // domain gone (its rules with it) or index reused: not ours
			}
			dst, err := natcommon.IP6(s.IP6Dst)
			if err != nil {
				return err
			}
			if _, err := p.svc.MapAddDelRule(ctx, &maps.MapAddDelRule{Index: m.Index, IsAdd: false, IP6Dst: dst, Psid: uint16(s.PSID)}); err != nil && !natcommon.IsNoSuchEntry(err) { //nolint:gosec // validated on Create
				return fmt.Errorf("map_add_del_rule: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[RuleSpec], error) {
			ds, err := p.domains(ctx)
			if err != nil {
				return nil, err
			}
			var out []natcommon.Item[RuleSpec]
			for _, d := range ds {
				stream, err := p.svc.MapRuleDump(ctx, &maps.MapRuleDump{DomainIndex: d.index})
				if err != nil {
					return nil, fmt.Errorf("map_rule_dump: %w", err)
				}
				for {
					r, err := stream.Recv()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						return nil, fmt.Errorf("map_rule_dump: %w", err)
					}
					out = append(out, natcommon.Item[RuleSpec]{Spec: RuleSpec{Domain: d.spec.Name, PSID: uint32(r.Psid), IP6Dst: natcommon.IP6String(r.IP6Dst)}, Meta: DomainMeta{Index: d.index}})
				}
			}
			sort.Slice(out, func(i, j int) bool {
				if out[i].Spec.Domain != out[j].Spec.Domain {
					return out[i].Spec.Domain < out[j].Spec.Domain
				}
				return out[i].Spec.PSID < out[j].Spec.PSID
			})
			return out, nil
		},
	})
}

// ---- params ---------------------------------------------------------------------------------

func (p *Plugin) setParams(ctx context.Context, s ParamsSpec) error {
	if _, err := p.svc.MapParamSetFragmentation(ctx, &maps.MapParamSetFragmentation{Inner: s.FragInner, IgnoreDf: s.FragIgnoreDF}); err != nil {
		return fmt.Errorf("map_param_set_fragmentation: %w", err)
	}
	relay, err := ip4OrZero(s.ICMPRelaySrc)
	if err != nil {
		return err
	}
	if _, err := p.svc.MapParamSetICMP(ctx, &maps.MapParamSetICMP{IP4ErrRelaySrc: relay}); err != nil {
		return fmt.Errorf("map_param_set_icmp: %w", err)
	}
	if _, err := p.svc.MapParamSetICMP6(ctx, &maps.MapParamSetICMP6{EnableUnreachable: s.ICMP6Unreachable}); err != nil {
		return fmt.Errorf("map_param_set_icmp6: %w", err)
	}
	if _, err := p.svc.MapParamSetSecurityCheck(ctx, &maps.MapParamSetSecurityCheck{Enable: s.SecurityCheck, Fragments: s.SecurityCheckFrags}); err != nil {
		return fmt.Errorf("map_param_set_security_check: %w", err)
	}
	tc, err := u8(s.TCClass, "tc_class")
	if err != nil {
		return err
	}
	if _, err := p.svc.MapParamSetTrafficClass(ctx, &maps.MapParamSetTrafficClass{Copy: s.TCCopy, TcClass: tc}); err != nil {
		return fmt.Errorf("map_param_set_traffic_class: %w", err)
	}
	return nil
}

func ip4OrZero(s string) (a [4]uint8, err error) {
	if s == "" {
		return a, nil
	}
	return natcommon.IP4(s)
}

// newParams: VPP-global (D-071). Owner: set / reset to VPP defaults, Retrieve reports
// non-default values. Others: require the desired values (map_param_get).
func (p *Plugin) newParams() *natcommon.Descriptor[ParamsSpec] {
	return natcommon.Global(p.cfg, natcommon.GlobalOps[ParamsSpec]{
		Name: NameParams, ID: Singleton,
		Read: func(ctx context.Context) (natcommon.GlobalState[ParamsSpec], error) {
			rep, err := p.svc.MapParamGet(ctx, &maps.MapParamGet{})
			if err != nil {
				return natcommon.GlobalState[ParamsSpec]{}, fmt.Errorf("map_param_get: %w", err)
			}
			s := ParamsSpec{FragInner: rep.FragInner != 0, FragIgnoreDF: rep.FragIgnoreDf != 0, ICMPRelaySrc: natcommon.IP4String(rep.ICMPIP4ErrRelaySrc),
				ICMP6Unreachable: rep.ICMP6EnableUnreachable, SecurityCheck: rep.SecCheckEnable, SecurityCheckFrags: rep.SecCheckFragments, TCCopy: rep.TcCopy, TCClass: uint32(rep.TcClass)}
			s.Normalize()
			return natcommon.GlobalState[ParamsSpec]{Value: s, Present: true, Observable: true}, nil
		},
		Absent: func(s ParamsSpec) bool { return s == DefaultParams },
		Set:    p.setParams,
		Reset:  func(ctx context.Context, _ ParamsSpec) error { return p.setParams(ctx, DefaultParams) },
	})
}

// ---- interface ----------------------------------------------------------------------------

// MAP has no dump of enabled interfaces; Retrieve asks the feature arc (feature_is_enabled)
// for the MAP nodes on every owned interface. map_if_enable_disable keeps MAP-E (encap) and
// MAP-T (translation) in two independent bitmaps (map_api.c), so an interface may carry
// both: the mode is part of the key.
const (
	arcIP4    = "ip4-unicast"
	featMapE4 = "ip4-map"
	featMapT4 = "ip4-map-t"
)

// Modes of a MAP interface.
const (
	ModeEncap       = "map-e"
	ModeTranslation = "map-t"
)

func mode(translation bool) string {
	if translation {
		return ModeTranslation
	}
	return ModeEncap
}

func (p *Plugin) mapFeature(ctx context.Context, idx uint32, name string) (bool, error) {
	rep, err := p.feat.FeatureIsEnabled(ctx, &feature.FeatureIsEnabled{SwIfIndex: interface_types.InterfaceIndex(idx), ArcName: arcIP4, FeatureName: name})
	if err != nil {
		return false, fmt.Errorf("feature_is_enabled %s/%s: %w", arcIP4, name, err)
	}
	return rep.IsEnabled, nil
}

func (p *Plugin) newInterface() *natcommon.Descriptor[InterfaceSpec] {
	return natcommon.New(natcommon.Ops[InterfaceSpec]{
		Claims: p.claims(),
		Name:   NameInterface,
		ID:     func(s InterfaceSpec) string { return s.Interface + "/" + mode(s.Translation) },
		Deps: func(s InterfaceSpec) []scheduler.Dependency {
			return []scheduler.Dependency{natcommon.InterfaceDep(s.Interface)}
		},
		Create: func(ctx context.Context, s InterfaceSpec) (any, error) {
			idx, err := natcommon.ResolveOwned(ctx, p.client, p.scope, s.Interface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.MapIfEnableDisable(ctx, &maps.MapIfEnableDisable{SwIfIndex: idx, IsEnable: true, IsTranslation: s.Translation}); err != nil {
				return nil, fmt.Errorf("map_if_enable_disable: %w", err)
			}
			return IfMeta{SwIfIndex: uint32(idx)}, nil
		},
		Delete: func(ctx context.Context, s InterfaceSpec, meta any) error {
			m, ok := meta.(IfMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameInterface, meta)
			}
			if _, err := p.svc.MapIfEnableDisable(ctx, &maps.MapIfEnableDisable{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex), IsEnable: false, IsTranslation: s.Translation}); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("map_if_enable_disable: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[InterfaceSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			var out []natcommon.Item[InterfaceSpec]
			for _, i := range ifaces.All() {
				owned, nc := p.scope.InterfaceOwnership(i)
				if !owned {
					continue
				}
				meta := IfMeta{SwIfIndex: i.SwIfIndex}
				for _, m := range []struct {
					feat        string
					translation bool
				}{{featMapE4, false}, {featMapT4, true}} {
					on, err := p.mapFeature(ctx, i.SwIfIndex, m.feat)
					if err != nil {
						return nil, err
					}
					if on {
						out = append(out, natcommon.Item[InterfaceSpec]{Spec: InterfaceSpec{Interface: i.Name, Translation: m.translation}, Meta: meta, NeedsClaim: nc})
					}
				}
			}
			return out, nil
		},
	})
}
