// Package npt66 holds the descriptor of VPP's NPTv6 plugin (binapi/npt66, npt66_plugin.so, RFC 6296 stateless
// IPv6-to-IPv6 prefix translation): one binding per interface, internal prefix <-> external prefix. Object <->
// message table: docs/agent/descriptors/npt66.md.
//
// VPP 26.06 has only npt66_binding_add_del — no dump and no show in the binary API — so the binding is WRITE-ONLY
// (D-063): Retrieve returns ErrRetrieveUnsupported, the reconciler re-applies the desired bindings on every resync and
// deletes one only when it leaves the desired state after this process applied it. The re-apply is safe without a
// D-076 applied-once record because VPP's add is idempotent (npt66.c npt66_binding_add_del): a binding on an interface
// that already has one is updated in place, and the npt66-input / npt66-output features are enabled only when the
// binding is new, so a repeated add never stacks a feature (modelled in coretest/npt66.go, asserted by
// TestBindingResyncIsIdempotent). VPP keeps one binding per interface; the schema's nat.nptv6-valid rejects two.
package npt66

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/npt66"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// NameBinding is the descriptor name.
const NameBinding = "npt66.binding"

// MaxPrefixLen is VPP's limit for both prefixes of a binding (npt66_binding_add_del answers INVALID_VALUE above it).
const MaxPrefixLen = 64

// BindingSpec is one NPTv6 binding on an interface: packets leaving the interface from the internal prefix get the
// external prefix as source (npt66-output on ip6-output), packets arriving for the external prefix get the internal
// prefix as destination (npt66-input on ip6-unicast); the rewrite is checksum-neutral (RFC 6296 §3.2).
type BindingSpec struct {
	Interface string `json:"interface"`
	Internal  string `json:"internal"`
	External  string `json:"external"`
}

// Normalize masks and canonicalises both prefixes.
func (s *BindingSpec) Normalize() {
	s.Internal, s.External = natcommon.CanonPrefix(s.Internal), natcommon.CanonPrefix(s.External)
}

// BindingID is the object id: "<interface>/<internal prefix>" (the task prompt's key npt66.binding/<if>/<internal>).
func BindingID(s BindingSpec) string { return s.Interface + "/" + s.Internal }

// IfMeta is the Meta of a binding: the sw_if_index it was added on.
type IfMeta struct{ SwIfIndex uint32 }

// ErrPrefixLength is returned for a prefix longer than MaxPrefixLen or two prefixes of different lengths.
var ErrPrefixLength = errors.New("npt66: prefixes must have the same length, at most /64")

// Plugin bundles the client, the owner scope and the descriptor.
type Plugin struct {
	client vpp.Client
	scope  natcommon.Scope
	cfg    natcommon.Config
	svc    npt66.RPCService

	Binding *natcommon.Descriptor[BindingSpec]
}

// New constructs the family for client and owner. npt66 has no VPP-global object, so natcommon.WithGlobalsOwner
// changes nothing here; the claim store (natcommon.WithClaims) records the bindings this owner created.
func New(client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := &Plugin{client: client, scope: natcommon.ScopeFor(owner), cfg: natcommon.BuildConfig(opts), svc: npt66.NewServiceClient(client)}
	p.Binding = p.newBinding()
	return p
}

// Descriptors returns the family in registration order.
func (p *Plugin) Descriptors() []scheduler.Descriptor { return []scheduler.Descriptor{p.Binding} }

// Register constructs the family and registers every descriptor.
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := New(client, owner, opts...)
	for _, d := range p.Descriptors() {
		r.Register(d)
	}
	return p
}

// request validates the spec and builds the add/del message for sw_if_index idx.
func request(s BindingSpec, idx interface_types.InterfaceIndex, add bool) (*npt66.Npt66BindingAddDel, error) {
	in, err := netip.ParsePrefix(s.Internal)
	if err != nil {
		return nil, fmt.Errorf("npt66: internal: %w", err)
	}
	ex, err := netip.ParsePrefix(s.External)
	if err != nil {
		return nil, fmt.Errorf("npt66: external: %w", err)
	}
	if !in.Addr().Is6() || !ex.Addr().Is6() || in.Addr().Is4In6() || ex.Addr().Is4In6() {
		return nil, fmt.Errorf("npt66: %s and %s must be IPv6 prefixes", s.Internal, s.External)
	}
	if in.Bits() != ex.Bits() || in.Bits() > MaxPrefixLen {
		return nil, fmt.Errorf("%w (internal /%d, external /%d)", ErrPrefixLength, in.Bits(), ex.Bits())
	}
	ip, err := natcommon.Prefix6(in.Masked().String())
	if err != nil {
		return nil, err
	}
	ep, err := natcommon.Prefix6(ex.Masked().String())
	if err != nil {
		return nil, err
	}
	return &npt66.Npt66BindingAddDel{IsAdd: add, SwIfIndex: idx, Internal: ip, External: ep}, nil
}

// add sends the add (also an in-place update: VPP overwrites the interface's binding).
func (p *Plugin) add(ctx context.Context, s BindingSpec) (IfMeta, error) {
	if !natcommon.PluginLoaded(p.client, &npt66.Npt66BindingAddDel{}) {
		return IfMeta{}, fmt.Errorf("%s: %w (npt66)", NameBinding, natcommon.ErrPluginNotLoaded)
	}
	idx, err := natcommon.ResolveOwned(ctx, p.client, p.scope, s.Interface)
	if err != nil {
		return IfMeta{}, err
	}
	req, err := request(s, idx, true)
	if err != nil {
		return IfMeta{}, err
	}
	if _, err := p.svc.Npt66BindingAddDel(ctx, req); err != nil {
		return IfMeta{}, fmt.Errorf("npt66_binding_add_del: %w", err)
	}
	return IfMeta{SwIfIndex: uint32(idx)}, nil
}

func (p *Plugin) newBinding() *natcommon.Descriptor[BindingSpec] {
	return natcommon.New(natcommon.Ops[BindingSpec]{
		Claims: p.cfg.Claims,
		Name:   NameBinding,
		ID:     BindingID,
		// the interface: its binding is deleted before it (D-095c: an entry whose interface is gone stays in VPP)
		Deps: func(s BindingSpec) []scheduler.Dependency { return []scheduler.Dependency{natcommon.InterfaceDep(s.Interface)} },
		Create: func(ctx context.Context, s BindingSpec) (any, error) {
			m, err := p.add(ctx, s)
			if err != nil {
				return nil, err
			}
			return m, nil
		},
		// same interface and internal prefix, another external prefix: VPP updates the binding in place
		Update: func(ctx context.Context, _, n BindingSpec, _ any) (any, error) {
			m, err := p.add(ctx, n)
			if err != nil {
				return nil, err
			}
			return m, nil
		},
		Delete: func(ctx context.Context, s BindingSpec, meta any) error {
			// VPP deletes the interface's binding by sw_if_index (the prefixes are ignored); the interface is
			// resolved again by name (it still exists: the binding depends on it), the Meta is the fallback
			idx, err := natcommon.ResolveOwned(ctx, p.client, p.scope, s.Interface)
			if err != nil {
				m, ok := meta.(IfMeta)
				if !ok || errors.Is(err, natcommon.ErrForeignInterface) {
					return err
				}
				idx = interface_types.InterfaceIndex(m.SwIfIndex)
			}
			req, err := request(s, idx, false)
			if err != nil {
				return err
			}
			if _, err := p.svc.Npt66BindingAddDel(ctx, req); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("npt66_binding_add_del: %w", err)
			}
			return nil
		},
		// no dump in VPP 26.06 (V-new npt66_binding_dump): write-only (D-063), never echo the desired state
		Retrieve: func(context.Context) ([]natcommon.Item[BindingSpec], error) {
			return nil, natcommon.ErrRetrieveUnsupported
		},
	})
}
