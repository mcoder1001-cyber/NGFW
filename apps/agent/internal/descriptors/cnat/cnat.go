// Package cnat holds the descriptors of VPP's CNat plugin (binapi/cnat, cnat_plugin.so, D4.6):
// translations (VIP → backends, load-balanced), the default source-NAT entry, its policy,
// its per-interface policy tables and excluded prefixes, and the per-interface cnat feature.
// Object <-> message table: docs/agent/descriptors/cnat.md.
//
// VPP 26.06 hazards handled here (all verified in src/plugins/cnat):
//   - cnat_set_snat_policy and cnat_snat_policy_add_del_exclude_pfx dereference the default
//     SNAT entry without a NULL check (ASSERT only): calling them before a default entry
//     exists crashes VPP. Every such call is guarded by cnat_get_snat_addresses.
//   - cnat_translation_update with n_paths = 0 underflows vec_validate(paths, n_paths - 1):
//     a translation must have at least one path.
//   - the SNAT policy, policy interfaces and excluded prefixes have no getter: those
//     descriptors are write-only (ErrRetrieveUnsupported, D-063). Translation flags /
//     is_real_ip / flow_hash_config are write-only fields and are not modelled (VPP defaults).
package cnat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"go.fd.io/govpp/api"

	cnatapi "ngfw/agent/binapi/cnat"
	"ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// Descriptor names.
const (
	NameTranslation      = "cnat.translation"
	NameSnatAddresses    = "cnat.snat-addresses"
	NameSnatPolicy       = "cnat.snat-policy"
	NameSnatInterface    = "cnat.snat-interface"
	NameSnatExcludePfx   = "cnat.snat-exclude-prefix"
	NameInterfaceFeature = "cnat.interface-feature"
)

// Singleton is the id of the global singletons.
const Singleton = "global"

// Load-balancing types.
const (
	LBDefault = "default"
	LBMaglev  = "maglev"
)

// SNAT policies (cnat_snat_policies).
const (
	PolicyNone  = "none"
	PolicyIfPfx = "if-pfx"
	PolicyK8s   = "k8s"
	PolicyDNAT  = "dnat"
)

// SNAT policy interface tables (cnat_snat_policy_table).
const (
	TableIncludeV4 = "include-v4"
	TableIncludeV6 = "include-v6"
	TablePod       = "pod"
	TableHost      = "host"
)

var (
	// ErrNoSnatDefault is returned when an object needs the default SNAT entry
	// (cnat.snat-addresses) and VPP has none — calling on would crash VPP 26.06.
	ErrNoSnatDefault = errors.New("cnat: no default SNAT entry (cnat.snat-addresses) on this VPP")
	// ErrNoPaths is returned for a translation without paths (VPP 26.06 underflow).
	ErrNoPaths = errors.New("cnat: a translation needs at least one path")
)

// SnatAddressesKey is the key of the default SNAT entry every snat-* object depends on.
var SnatAddressesKey = scheduler.Join(NameSnatAddresses, Singleton)

// ---- specs --------------------------------------------------------------------------------

// PathSpec is one backend of a translation: the rewritten destination (and optional source)
// endpoint. Empty addresses / zero ports mean "unchanged".
type PathSpec struct {
	Src     string `json:"src"`
	SrcPort uint32 `json:"src_port"`
	Dst     string `json:"dst"`
	DstPort uint32 `json:"dst_port"`
	NoNAT   bool   `json:"no_nat"`
}

// TranslationSpec is one CNat translation (cnat_translation_update): a VIP (address, port,
// protocol) load-balanced over Paths. The translation flags (alloc-port, no-return-session,
// no-client), is_real_ip and flow_hash_config are not returned by cnat_translation_dump, so
// they are not modelled: VPP's defaults are sent (flags 0, is_real_ip 0 = exclusive VIP, as
// the CLI default), DF-3-questions.md Q6.
type TranslationSpec struct {
	VIP    string     `json:"vip"`
	Port   uint32     `json:"port"`
	Proto  string     `json:"proto"`
	LBType string     `json:"lb_type"`
	Paths  []PathSpec `json:"paths"`
}

// Normalize canonicalises addresses, protocol, lb type and sorts the paths.
func (s *TranslationSpec) Normalize() {
	s.VIP = natcommon.CanonAddr(s.VIP)
	s.Proto = natcommon.CanonProto(s.Proto)
	if s.LBType == "" {
		s.LBType = LBDefault
	}
	for i := range s.Paths {
		s.Paths[i].Src, s.Paths[i].Dst = canonOptAddr(s.Paths[i].Src), canonOptAddr(s.Paths[i].Dst)
	}
	if s.Paths == nil {
		s.Paths = []PathSpec{}
	}
	sort.Slice(s.Paths, func(i, j int) bool {
		a, b := s.Paths[i], s.Paths[j]
		if a.Dst != b.Dst {
			return a.Dst < b.Dst
		}
		if a.DstPort != b.DstPort {
			return a.DstPort < b.DstPort
		}
		if a.Src != b.Src {
			return a.Src < b.Src
		}
		return a.SrcPort < b.SrcPort
	})
}

// SnatAddressesSpec is the default SNAT entry (cnat_set_snat_addresses): either IPv4 and/or
// IPv6 addresses, or an interface whose addresses are used.
type SnatAddressesSpec struct {
	IP4       string `json:"ip4"`
	IP6       string `json:"ip6"`
	Interface string `json:"interface"`
}

// Normalize canonicalises the addresses.
func (s *SnatAddressesSpec) Normalize() { s.IP4, s.IP6 = canonOptAddr(s.IP4), canonOptAddr(s.IP6) }

// SnatPolicySpec selects the SNAT policy of the default entry (cnat_set_snat_policy).
type SnatPolicySpec struct {
	Policy string `json:"policy"`
}

// SnatInterfaceSpec adds an interface to one policy table of the default entry
// (cnat_snat_policy_add_del_if).
type SnatInterfaceSpec struct {
	Interface string `json:"interface"`
	Table     string `json:"table"`
}

// SnatExcludePrefixSpec excludes a destination prefix from source NAT
// (cnat_snat_policy_add_del_exclude_pfx).
type SnatExcludePrefixSpec struct {
	Prefix string `json:"prefix"`
}

// Normalize masks the prefix.
func (s *SnatExcludePrefixSpec) Normalize() { s.Prefix = natcommon.CanonPrefix(s.Prefix) }

// InterfaceFeatureSpec enables the cnat feature nodes on an interface
// (feature_cnat_enable_disable).
type InterfaceFeatureSpec struct {
	Interface string `json:"interface"`
}

// TranslationMeta is the translation id VPP assigned.
type TranslationMeta struct{ ID uint32 }

// IfMeta is the Meta of interface-bound objects.
type IfMeta struct{ SwIfIndex uint32 }

// ---- plugin -------------------------------------------------------------------------------

// Plugin bundles the client, the owner scope and the descriptors.
type Plugin struct {
	client vpp.Client
	scope  natcommon.Scope
	cfg    natcommon.Config

	mu          sync.Mutex
	excludeRecs map[string]string // prefix → this process's D-076 record
	snatGen     atomic.Uint64     // default SNAT entry generation (bumped by the owner's Set/Reset)
	svc         cnatapi.RPCService
	feat        feature.RPCService

	Translation      *natcommon.Descriptor[TranslationSpec]
	SnatAddresses    *natcommon.Descriptor[SnatAddressesSpec]
	SnatPolicy       *natcommon.Descriptor[SnatPolicySpec]
	SnatInterface    *natcommon.Descriptor[SnatInterfaceSpec]
	SnatExcludePfx   *natcommon.Descriptor[SnatExcludePrefixSpec]
	InterfaceFeature *natcommon.Descriptor[InterfaceFeatureSpec]
}

// New constructs the family for client and owner.
func New(client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := &Plugin{client: client, scope: natcommon.ScopeFor(owner), cfg: natcommon.BuildConfig(opts), excludeRecs: map[string]string{}, svc: cnatapi.NewServiceClient(client), feat: feature.NewServiceClient(client)}
	p.SnatAddresses = p.newSnatAddresses()
	p.SnatPolicy = p.newSnatPolicy()
	p.SnatInterface = p.newSnatInterface()
	p.SnatExcludePfx = p.newSnatExcludePfx()
	p.InterfaceFeature = p.newInterfaceFeature()
	p.Translation = p.newTranslation()
	return p
}

// Descriptors returns the family in registration order.
func (p *Plugin) Descriptors() []scheduler.Descriptor {
	return []scheduler.Descriptor{p.SnatAddresses, p.SnatPolicy, p.SnatInterface, p.SnatExcludePfx, p.InterfaceFeature, p.Translation}
}

// Register constructs the family and registers every descriptor.
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := New(client, owner, opts...)
	for _, d := range p.Descriptors() {
		r.Register(d)
	}
	return p
}

func canonOptAddr(s string) string {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return s
	}
	if a.IsUnspecified() {
		return ""
	}
	return a.Unmap().String()
}

func u16(v uint32, what string) (uint16, error) {
	if v > 65535 {
		return 0, fmt.Errorf("cnat: %s %d out of range", what, v)
	}
	return uint16(v), nil
}

// endpoint builds an address endpoint (sw_if_index ~0); "" is the unspecified address of the
// VIP's family.
func endpoint(addr string, port uint32, v6 bool, what string) (cnatapi.CnatEndpoint, error) {
	ep := cnatapi.CnatEndpoint{SwIfIndex: ^interface_types.InterfaceIndex(0)}
	pt, err := u16(port, what+" port")
	if err != nil {
		return ep, err
	}
	ep.Port = pt
	if addr == "" {
		if v6 {
			addr = "::"
		} else {
			addr = "0.0.0.0"
		}
	}
	a, err := natcommon.Addr(addr)
	if err != nil {
		return ep, fmt.Errorf("%s: %w", what, err)
	}
	ep.Addr, ep.IfAf = a, a.Af
	return ep, nil
}

func epString(ep cnatapi.CnatEndpoint) string { return canonOptAddr(natcommon.AddrString(ep.Addr)) }

// snatDefault reports the default SNAT entry (cnat_get_snat_addresses); ok is false when
// VPP has none (retval FEATURE_DISABLED).
func (p *Plugin) snatDefault(ctx context.Context) (*cnatapi.CnatGetSnatAddressesReply, bool, error) {
	rep, err := p.svc.CnatGetSnatAddresses(ctx, &cnatapi.CnatGetSnatAddresses{})
	if err != nil {
		if rv, ok := natcommon.Retval(err); ok && rv == api.FEATURE_DISABLED {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("cnat_get_snat_addresses: %w", err)
	}
	return rep, true, nil
}

// ---- translation --------------------------------------------------------------------------

func (p *Plugin) buildTranslation(s TranslationSpec) (*cnatapi.CnatTranslationUpdate, error) {
	if len(s.Paths) == 0 {
		return nil, ErrNoPaths
	}
	vip, err := netip.ParseAddr(s.VIP)
	if err != nil {
		return nil, fmt.Errorf("cnat: vip %q: %w", s.VIP, err)
	}
	v6 := vip.Is6() && !vip.Is4In6()
	proto, err := natcommon.ProtoNumber(s.Proto)
	if err != nil {
		return nil, err
	}
	var lb cnatapi.CnatLbType
	switch s.LBType {
	case LBDefault, "":
		lb = cnatapi.CNAT_LB_TYPE_DEFAULT
	case LBMaglev:
		lb = cnatapi.CNAT_LB_TYPE_MAGLEV
	default:
		return nil, fmt.Errorf("cnat: lb_type must be %q or %q, got %q", LBDefault, LBMaglev, s.LBType)
	}
	vipEP, err := endpoint(s.VIP, s.Port, v6, "vip")
	if err != nil {
		return nil, err
	}
	tr := cnatapi.CnatTranslation{Vip: vipEP, IPProto: ip_types.IPProto(proto), LbType: lb}
	for i, ps := range s.Paths {
		dst, err := endpoint(ps.Dst, ps.DstPort, v6, fmt.Sprintf("paths[%d].dst", i))
		if err != nil {
			return nil, err
		}
		src, err := endpoint(ps.Src, ps.SrcPort, v6, fmt.Sprintf("paths[%d].src", i))
		if err != nil {
			return nil, err
		}
		pt := cnatapi.CnatEndpointTuple{DstEp: dst, SrcEp: src}
		if ps.NoNAT {
			pt.Flags = uint8(cnatapi.CNAT_EPT_NO_NAT)
		}
		tr.Paths = append(tr.Paths, pt)
	}
	tr.NPaths = uint32(len(tr.Paths)) //nolint:gosec // bounded by the desired state
	return &cnatapi.CnatTranslationUpdate{Translation: tr}, nil
}

func (p *Plugin) updateTranslation(ctx context.Context, s TranslationSpec) (any, error) {
	req, err := p.buildTranslation(s)
	if err != nil {
		return nil, err
	}
	rep, err := p.svc.CnatTranslationUpdate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("cnat_translation_update: %w", err)
	}
	return TranslationMeta{ID: rep.ID}, nil
}

func (p *Plugin) newTranslation() *natcommon.Descriptor[TranslationSpec] {
	return natcommon.New(natcommon.Ops[TranslationSpec]{
		Claims: p.cfg.Claims,
		Name:   NameTranslation,
		ID: func(s TranslationSpec) string {
			return s.VIP + "/" + s.Proto + "/" + strconv.FormatUint(uint64(s.Port), 10)
		},
		Deps: func(TranslationSpec) []scheduler.Dependency {
			return []scheduler.Dependency{natcommon.OptionalDep(SnatAddressesKey)}
		},
		Create: p.updateTranslation,
		// cnat_translation_update on the same (vip, port, proto) updates the translation in
		// place (paths, lb type, flags) and keeps its id.
		Update: func(ctx context.Context, _, n TranslationSpec, _ any) (any, error) {
			return p.updateTranslation(ctx, n)
		},
		Delete: func(ctx context.Context, s TranslationSpec, meta any) error {
			m, ok := meta.(TranslationMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameTranslation, meta)
			}
			// D-071 / review finding 3: re-verify that id still is this (vip, port, proto)
			// immediately before deleting by id (VPP restart, id reuse by another owner).
			if same, err := p.translationAt(ctx, m.ID, s); err != nil {
				return err
			} else if !same {
				return nil
			}
			if _, err := p.svc.CnatTranslationDel(ctx, &cnatapi.CnatTranslationDel{ID: m.ID}); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("cnat_translation_del: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[TranslationSpec], error) {
			stream, err := p.svc.CnatTranslationDump(ctx, &cnatapi.CnatTranslationDump{})
			if err != nil {
				return nil, fmt.Errorf("cnat_translation_dump: %w", err)
			}
			var out []natcommon.Item[TranslationSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("cnat_translation_dump: %w", err)
				}
				tr := d.Translation
				vip := epString(tr.Vip)
				if vip == "" {
					continue // interface-resolved VIP, not modelled
				}
				s := TranslationSpec{VIP: vip, Port: uint32(tr.Vip.Port), Proto: natcommon.ProtoName(uint8(tr.IPProto)), LBType: LBDefault}
				if tr.LbType == cnatapi.CNAT_LB_TYPE_MAGLEV {
					s.LBType = LBMaglev
				}
				for _, pt := range tr.Paths {
					s.Paths = append(s.Paths, PathSpec{Src: epString(pt.SrcEp), SrcPort: uint32(pt.SrcEp.Port), Dst: epString(pt.DstEp), DstPort: uint32(pt.DstEp.Port),
						NoNAT: pt.Flags&uint8(cnatapi.CNAT_EPT_NO_NAT) != 0}) // other bits are internal tracker state
				}
				s.Normalize()
				out = append(out, natcommon.Item[TranslationSpec]{Spec: s, Meta: TranslationMeta{ID: tr.ID}, NeedsClaim: p.scope.NeedsClaim(p.scope.OwnsAddrString(vip))})
			}
		},
	})
}

// ---- snat addresses (default entry) -------------------------------------------------------

func (p *Plugin) setSnat(ctx context.Context, s SnatAddressesSpec) error {
	req := &cnatapi.CnatSetSnatAddresses{SwIfIndex: ^interface_types.InterfaceIndex(0)}
	if s.Interface != "" {
		if s.IP4 != "" || s.IP6 != "" {
			return fmt.Errorf("cnat: snat addresses: set either an interface or addresses, not both")
		}
		idx, err := natcommon.ResolveOwned(ctx, p.client, p.scope, s.Interface)
		if err != nil {
			return err
		}
		req.SwIfIndex = idx
	}
	if s.IP4 != "" {
		a, err := natcommon.IP4(s.IP4)
		if err != nil {
			return err
		}
		req.SnatIP4 = a
	}
	if s.IP6 != "" {
		a, err := natcommon.IP6(s.IP6)
		if err != nil {
			return err
		}
		req.SnatIP6 = a
	}
	if _, err := p.svc.CnatSetSnatAddresses(ctx, req); err != nil {
		return fmt.Errorf("cnat_set_snat_addresses: %w", err)
	}
	return nil
}

// translationAt reports whether translation id currently is the (vip, port, proto) of s.
func (p *Plugin) translationAt(ctx context.Context, id uint32, s TranslationSpec) (bool, error) {
	stream, err := p.svc.CnatTranslationDump(ctx, &cnatapi.CnatTranslationDump{})
	if err != nil {
		return false, fmt.Errorf("cnat_translation_dump: %w", err)
	}
	found := false
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return found, nil
		}
		if err != nil {
			return false, fmt.Errorf("cnat_translation_dump: %w", err)
		}
		tr := d.Translation
		if tr.ID == id {
			found = epString(tr.Vip) == s.VIP && uint32(tr.Vip.Port) == s.Port && natcommon.ProtoName(uint8(tr.IPProto)) == s.Proto
		}
	}
}

// lock takes the host-wide cnat lock (review finding 8): exclusive for creating/deleting the
// default SNAT entry, shared for mutations that dereference it (policy, excluded prefixes),
// so the guard check and the dangerous message cannot interleave with another owner's
// delete of the entry.
func (p *Plugin) lock(exclusive bool) (func(), error) {
	return natcommon.HostLock(p.cfg.LockDir, "cnat", exclusive)
}

func (p *Plugin) readSnat(ctx context.Context) (natcommon.GlobalState[SnatAddressesSpec], error) {
	rep, ok, err := p.snatDefault(ctx)
	if err != nil || !ok {
		return natcommon.GlobalState[SnatAddressesSpec]{Observable: true}, err
	}
	s := SnatAddressesSpec{IP4: canonOptAddr(natcommon.IP4String(rep.SnatIP4)), IP6: canonOptAddr(natcommon.IP6String(rep.SnatIP6))}
	// sw_if_index is taken from the IPv6 endpoint: 0 when only an IPv4 address was set
	// (never initialised), ~0 for "no interface".
	if idx := uint32(rep.SwIfIndex); idx != 0 && idx != ^uint32(0) {
		ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
		if err != nil {
			return natcommon.GlobalState[SnatAddressesSpec]{}, err
		}
		s = SnatAddressesSpec{Interface: p.scope.LogicalNameOf(ifaces, idx)} // addresses are resolved from the interface
	}
	return natcommon.GlobalState[SnatAddressesSpec]{Value: s, Present: true, Observable: true}, nil
}

// newSnatAddresses: the cnat default SNAT entry is a VPP-global singleton (D-071). Owner:
// create/replace (Update = recreate: the v1 setter cannot clear one family, and VPP drops
// the policy, interface tables and excluded prefixes with the entry, so dependents are
// recreated) and delete, under the exclusive host-wide cnat lock. Others: require it.
func (p *Plugin) newSnatAddresses() *natcommon.Descriptor[SnatAddressesSpec] {
	return natcommon.Global(p.cfg, natcommon.GlobalOps[SnatAddressesSpec]{
		Name: NameSnatAddresses, ID: Singleton,
		Deps: func(s SnatAddressesSpec) []scheduler.Dependency {
			if s.Interface != "" {
				return []scheduler.Dependency{natcommon.InterfaceDep(s.Interface)}
			}
			return nil
		},
		Read: p.readSnat,
		Set: func(ctx context.Context, s SnatAddressesSpec) error {
			unlock, err := p.lock(true)
			if err != nil {
				return err
			}
			defer unlock()
			p.snatGen.Add(1) // any excluded prefix must be re-applied to the new entry (N2)
			return p.setSnat(ctx, s)
		},
		SetUpdate: func(context.Context, SnatAddressesSpec, SnatAddressesSpec) error { return scheduler.ErrRecreate },
		Reset: func(ctx context.Context, _ SnatAddressesSpec) error {
			unlock, err := p.lock(true)
			if err != nil {
				return err
			}
			defer unlock()
			if _, ok, err := p.snatDefault(ctx); err != nil || !ok {
				return err
			}
			p.snatGen.Add(1)
			// all-zero addresses and no interface = delete the default entry (cnat_set_snat).
			if _, err := p.svc.CnatSetSnatAddresses(ctx, &cnatapi.CnatSetSnatAddresses{SwIfIndex: ^interface_types.InterfaceIndex(0)}); err != nil {
				return fmt.Errorf("cnat_set_snat_addresses (delete): %w", err)
			}
			return nil
		},
	})
}

// ---- snat policy / interfaces / excluded prefixes (no getter: write-only, D-063) ---------

func snatDep() []scheduler.Dependency { return []scheduler.Dependency{natcommon.Dep(SnatAddressesKey)} }

func writeOnly[T any](context.Context) ([]natcommon.Item[T], error) {
	return nil, natcommon.ErrRetrieveUnsupported
}

func policyValue(s string) (cnatapi.CnatSnatPolicies, error) {
	switch s {
	case PolicyNone, "":
		return cnatapi.CNAT_POLICY_NONE, nil
	case PolicyIfPfx:
		return cnatapi.CNAT_POLICY_IF_PFX, nil
	case PolicyK8s:
		return cnatapi.CNAT_POLICY_K8S, nil
	case PolicyDNAT:
		return cnatapi.CNAT_POLICY_DNAT, nil
	}
	return 0, fmt.Errorf("cnat: unknown snat policy %q", s)
}

func (p *Plugin) setPolicy(ctx context.Context, s SnatPolicySpec) error {
	v, err := policyValue(s.Policy)
	if err != nil {
		return err
	}
	unlock, err := p.lock(false)
	if err != nil {
		return err
	}
	defer unlock()
	if _, ok, err := p.snatDefault(ctx); err != nil {
		return err
	} else if !ok {
		return ErrNoSnatDefault
	}
	if _, err := p.svc.CnatSetSnatPolicy(ctx, &cnatapi.CnatSetSnatPolicy{Policy: v}); err != nil {
		return fmt.Errorf("cnat_set_snat_policy: %w", err)
	}
	return nil
}

// newSnatPolicy: the policy of the default SNAT entry is a VPP-global (D-071) with no getter:
// owner sets it (write-only, D-063; Delete sets "none"); a non-owner cannot verify it, so its
// Create succeeds without touching VPP (documented in cnat.md).
func (p *Plugin) newSnatPolicy() *natcommon.Descriptor[SnatPolicySpec] {
	return natcommon.Global(p.cfg, natcommon.GlobalOps[SnatPolicySpec]{
		Name: NameSnatPolicy, ID: Singleton,
		Deps: func(SnatPolicySpec) []scheduler.Dependency { return snatDep() },
		Read: func(context.Context) (natcommon.GlobalState[SnatPolicySpec], error) {
			return natcommon.GlobalState[SnatPolicySpec]{Observable: false}, nil
		},
		WriteOnly: true,
		Set:       p.setPolicy,
		Reset: func(ctx context.Context, _ SnatPolicySpec) error {
			if err := p.setPolicy(ctx, SnatPolicySpec{Policy: PolicyNone}); err != nil && !errors.Is(err, ErrNoSnatDefault) {
				return err
			}
			return nil
		},
	})
}

func tableValue(s string) (cnatapi.CnatSnatPolicyTable, error) {
	switch s {
	case TableIncludeV4:
		return cnatapi.CNAT_POLICY_INCLUDE_V4, nil
	case TableIncludeV6:
		return cnatapi.CNAT_POLICY_INCLUDE_V6, nil
	case TablePod:
		return cnatapi.CNAT_POLICY_POD, nil
	case TableHost:
		return cnatapi.CNAT_POLICY_HOST, nil
	}
	return 0, fmt.Errorf("cnat: unknown snat policy table %q", s)
}

func (p *Plugin) addDelSnatIf(ctx context.Context, s SnatInterfaceSpec, idx interface_types.InterfaceIndex, add bool) error {
	tbl, err := tableValue(s.Table)
	if err != nil {
		return err
	}
	isAdd := uint8(0)
	if add {
		isAdd = 1
	}
	if _, err := p.svc.CnatSnatPolicyAddDelIf(ctx, &cnatapi.CnatSnatPolicyAddDelIf{SwIfIndex: idx, IsAdd: isAdd, Table: tbl}); err != nil {
		if rv, ok := natcommon.Retval(err); ok && rv == api.FEATURE_DISABLED {
			if add {
				return ErrNoSnatDefault
			}
			return nil
		}
		return fmt.Errorf("cnat_snat_policy_add_del_if: %w", err)
	}
	return nil
}

func (p *Plugin) newSnatInterface() *natcommon.Descriptor[SnatInterfaceSpec] {
	return natcommon.New(natcommon.Ops[SnatInterfaceSpec]{
		Claims: p.cfg.Claims,
		Name:   NameSnatInterface,
		ID:     func(s SnatInterfaceSpec) string { return s.Interface + "/" + s.Table },
		Deps: func(s SnatInterfaceSpec) []scheduler.Dependency {
			return append(snatDep(), natcommon.InterfaceDep(s.Interface))
		},
		// Idempotent: the per-table interface bitmap is set, not counted.
		Create: func(ctx context.Context, s SnatInterfaceSpec) (any, error) {
			idx, err := natcommon.ResolveOwned(ctx, p.client, p.scope, s.Interface)
			if err != nil {
				return nil, err
			}
			if err := p.addDelSnatIf(ctx, s, idx, true); err != nil {
				return nil, err
			}
			return IfMeta{SwIfIndex: uint32(idx)}, nil
		},
		// Write-only objects may be deleted without a Meta (after an agent restart): the
		// interface is then resolved by name; a vanished interface has nothing to remove.
		Delete: func(ctx context.Context, s SnatInterfaceSpec, meta any) error {
			m, ok := meta.(IfMeta)
			if !ok {
				idx, err := natcommon.ResolveOwned(ctx, p.client, p.scope, s.Interface)
				if errors.Is(err, natcommon.ErrNoSuchInterface) {
					return nil
				}
				if err != nil {
					return err
				}
				m = IfMeta{SwIfIndex: uint32(idx)}
			}
			return p.addDelSnatIf(ctx, s, interface_types.InterfaceIndex(m.SwIfIndex), false)
		},
		Retrieve: writeOnly[SnatInterfaceSpec],
	})
}

// excludeOnce makes the prefix present exactly once: under the shared host-wide cnat lock it
// checks the default entry (the messages dereference it unchecked, V10) and sends a delete
// followed by an add. VPP's delete of an absent prefix is a no-op and its add bumps a
// per-length refcount, so del+add ends with one instance whatever VPP held before (fresh
// entry, recreated entry, or already present).
func (p *Plugin) excludeOnce(ctx context.Context, s SnatExcludePrefixSpec) error {
	pfx, err := natcommon.Prefix(s.Prefix)
	if err != nil {
		return err
	}
	unlock, err := p.lock(false)
	if err != nil {
		return err
	}
	defer unlock()
	if _, ok, err := p.snatDefault(ctx); err != nil {
		return err
	} else if !ok {
		return ErrNoSnatDefault
	}
	for _, add := range []uint8{0, 1} {
		if _, err := p.svc.CnatSnatPolicyAddDelExcludePfx(ctx, &cnatapi.CnatSnatPolicyAddDelExcludePfx{IsAdd: add, Prefix: pfx}); err != nil {
			return fmt.Errorf("cnat_snat_policy_add_del_exclude_pfx: %w", err)
		}
	}
	return nil
}

func (p *Plugin) addDelExclude(ctx context.Context, s SnatExcludePrefixSpec, add bool) error {
	pfx, err := natcommon.Prefix(s.Prefix)
	if err != nil {
		return err
	}
	unlock, err := p.lock(false)
	if err != nil {
		return err
	}
	defer unlock()
	if _, ok, err := p.snatDefault(ctx); err != nil {
		return err
	} else if !ok {
		if add {
			return ErrNoSnatDefault
		}
		return nil
	}
	isAdd := uint8(0)
	if add {
		isAdd = 1
	}
	if _, err := p.svc.CnatSnatPolicyAddDelExcludePfx(ctx, &cnatapi.CnatSnatPolicyAddDelExcludePfx{IsAdd: isAdd, Prefix: pfx}); err != nil {
		return fmt.Errorf("cnat_snat_policy_add_del_exclude_pfx: %w", err)
	}
	return nil
}

// entryIdentity identifies the VPP instance and the default SNAT entry the excluded prefixes
// live in (re-review N2, D-080): the boot identity triple, the entry's observable fingerprint
// (addresses, interface) and the entry generation this process's globals-owner descriptor
// bumps on every Set/Reset of the entry. VPP has no generation of its own: a recreate by
// another agent with identical addresses is not observable (documented; under D-071 only the
// globals owner — this same agent on a real box — mutates the entry).
func (p *Plugin) entryIdentity(ctx context.Context) (string, error) {
	boot, err := bootid.Current(ctx, p.client)
	if err != nil {
		return "", err
	}
	rep, ok, err := p.snatDefault(ctx)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNoSnatDefault
	}
	return fmt.Sprintf("%s/snat=%v,%v,%d/g%d", boot, rep.SnatIP4, rep.SnatIP6, rep.SwIfIndex, p.snatGen.Load()), nil
}

// newSnatExcludePfx: write-only (no dump, D-063). VPP's add is not idempotent (per-length
// refcount), so (D-076, D-080) a re-apply is skipped while the record "<key>@<entry identity>"
// exists; on a miss (first apply, VPP restart, entry recreated or changed) the prefix is made
// present exactly once with del+add, the record is written and this process's older record
// for the prefix is released (no growth across restarts, re-review I2).
func (p *Plugin) newSnatExcludePfx() *natcommon.Descriptor[SnatExcludePrefixSpec] {
	key := func(s SnatExcludePrefixSpec) string { return string(scheduler.Join(NameSnatExcludePfx, s.Prefix)) }
	return natcommon.New(natcommon.Ops[SnatExcludePrefixSpec]{
		Claims: p.cfg.Claims,
		Name:   NameSnatExcludePfx,
		ID:     func(s SnatExcludePrefixSpec) string { return s.Prefix },
		Deps:   func(SnatExcludePrefixSpec) []scheduler.Dependency { return snatDep() },
		Create: func(ctx context.Context, s SnatExcludePrefixSpec) (any, error) {
			id, err := p.entryIdentity(ctx)
			if err != nil {
				return nil, err
			}
			rec := natcommon.AppliedRecord(key(s), id)
			if p.cfg.Claims.Claimed(rec) {
				return nil, nil // already present in this entry of this VPP process
			}
			if err := p.excludeOnce(ctx, s); err != nil {
				return nil, err
			}
			p.mu.Lock()
			old := p.excludeRecs[s.Prefix]
			p.excludeRecs[s.Prefix] = rec
			p.mu.Unlock()
			if old != "" && old != rec {
				_ = p.cfg.Claims.Release(old)
			}
			return nil, p.cfg.Claims.Claim(rec)
		},
		Delete: func(ctx context.Context, s SnatExcludePrefixSpec, _ any) error {
			if err := p.addDelExclude(ctx, s, false); err != nil {
				return err
			}
			p.mu.Lock()
			old := p.excludeRecs[s.Prefix]
			delete(p.excludeRecs, s.Prefix)
			p.mu.Unlock()
			if old != "" {
				return p.cfg.Claims.Release(old)
			}
			return nil
		},
		Retrieve: writeOnly[SnatExcludePrefixSpec],
	})
}

// ---- interface feature --------------------------------------------------------------------

const (
	featArc  = "ip4-unicast"
	featNode = "cnat-input-ip4"
)

func (p *Plugin) newInterfaceFeature() *natcommon.Descriptor[InterfaceFeatureSpec] {
	return natcommon.New(natcommon.Ops[InterfaceFeatureSpec]{
		Claims: p.cfg.Claims,
		Name:   NameInterfaceFeature,
		ID:     func(s InterfaceFeatureSpec) string { return s.Interface },
		Deps: func(s InterfaceFeatureSpec) []scheduler.Dependency {
			return []scheduler.Dependency{natcommon.InterfaceDep(s.Interface)}
		},
		Create: func(ctx context.Context, s InterfaceFeatureSpec) (any, error) {
			idx, err := natcommon.ResolveOwned(ctx, p.client, p.scope, s.Interface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.FeatureCnatEnableDisable(ctx, &cnatapi.FeatureCnatEnableDisable{SwIfIndex: idx, EnableDisable: true}); err != nil {
				return nil, fmt.Errorf("feature_cnat_enable_disable: %w", err)
			}
			return IfMeta{SwIfIndex: uint32(idx)}, nil
		},
		Delete: func(ctx context.Context, _ InterfaceFeatureSpec, meta any) error {
			m, ok := meta.(IfMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameInterfaceFeature, meta)
			}
			if _, err := p.svc.FeatureCnatEnableDisable(ctx, &cnatapi.FeatureCnatEnableDisable{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex), EnableDisable: false}); err != nil {
				return fmt.Errorf("feature_cnat_enable_disable: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[InterfaceFeatureSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			var out []natcommon.Item[InterfaceFeatureSpec]
			for _, i := range ifaces.All() {
				owned, nc := p.scope.InterfaceOwnership(i)
				if !owned {
					continue
				}
				rep, err := p.feat.FeatureIsEnabled(ctx, &feature.FeatureIsEnabled{SwIfIndex: interface_types.InterfaceIndex(i.SwIfIndex), ArcName: featArc, FeatureName: featNode})
				if err != nil {
					return nil, fmt.Errorf("feature_is_enabled %s/%s: %w", featArc, featNode, err)
				}
				if rep.IsEnabled {
					out = append(out, natcommon.Item[InterfaceFeatureSpec]{Spec: InterfaceFeatureSpec{Interface: p.scope.LogicalName(i)}, Meta: IfMeta{SwIfIndex: i.SwIfIndex}, NeedsClaim: nc})
				}
			}
			return out, nil
		},
	})
}

// ---- state / actions ----------------------------------------------------------------------

// Session is one cnat session (cnat_session_details), Retrieve-only state.
type Session struct {
	Src     string `json:"src"`
	SrcPort uint32 `json:"src_port"`
	Dst     string `json:"dst"`
	DstPort uint32 `json:"dst_port"`
	Proto   string `json:"proto"`
	Flags   uint32 `json:"flags"`
}

// Sessions returns one page (offset, limit; 0 = all) of the cnat sessions.
func (p *Plugin) Sessions(ctx context.Context, offset, limit int) ([]Session, error) {
	stream, err := p.svc.CnatSessionDump(ctx, &cnatapi.CnatSessionDump{})
	if err != nil {
		return nil, fmt.Errorf("cnat_session_dump: %w", err)
	}
	var out []Session
	for i := 0; ; i++ {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("cnat_session_dump: %w", err)
		}
		if i < offset || (limit > 0 && len(out) >= limit) {
			continue
		}
		tp := d.Session.Tuple
		s := Session{Src: natcommon.AddrString(tp.Addr[0]), Dst: natcommon.AddrString(tp.Addr[1]), Proto: natcommon.ProtoName(uint8(tp.IPProto)), Flags: d.Session.Flags}
		if len(tp.Port) == 2 {
			s.SrcPort, s.DstPort = uint32(tp.Port[0]), uint32(tp.Port[1])
		}
		out = append(out, s)
	}
}

// PurgeSessions is the cnat_session_purge action (global: expires stale sessions and
// translations of every owner).
func (p *Plugin) PurgeSessions(ctx context.Context) error {
	if _, err := p.svc.CnatSessionPurge(ctx, &cnatapi.CnatSessionPurge{}); err != nil {
		return fmt.Errorf("cnat_session_purge: %w", err)
	}
	return nil
}
