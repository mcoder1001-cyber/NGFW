// Package det44 holds the descriptors of VPP's deterministic NAT (CGN) plugin (binapi/det44,
// det44_plugin.so): plugin enable (inside/outside VRF), interfaces, deterministic maps and
// session timeouts, plus Retrieve-only session state and the close-session action helpers.
// Object <-> message table: docs/agent/descriptors/det44.md. Like nat64/nat66, VPP has no
// "is det44 enabled" getter: the enable singleton is write-only (ErrRetrieveUnsupported, D-063).
// The interface descriptor also repairs the ip4-unicast feature arc after VPP 26.06's det44
// delete (see "feature arc repair" below, F-det44-cnat-fix).
package det44

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/det44"
	"ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameEnable    = "det44.enable"
	NameInterface = "det44.interface"
	NameMap       = "det44.map"
	NameTimeouts  = "det44.timeouts"
)

// Singleton is the id of the global singletons.
const Singleton = "global"

// Sides of a det44 interface.
const (
	SideInside  = "inside"
	SideOutside = "outside"
)

// EnableKey is the key every other det44 object depends on.
var EnableKey = scheduler.Join(NameEnable, Singleton)

// DefaultTimeouts are VPP's default det44 timeouts (det44.c).
var DefaultTimeouts = TimeoutsSpec{UDP: 300, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60}

// EnableSpec is the plugin singleton (det44_plugin_enable_disable), write-only: VPP has no
// getter for "enabled" or the VRFs. An enable on an already enabled plugin keeps its VRFs.
type EnableSpec struct {
	InsideVRF  uint32 `json:"inside_vrf"`
	OutsideVRF uint32 `json:"outside_vrf"`
}

// InterfaceSpec puts an interface on the inside or outside of det44
// (det44_interface_add_del_feature).
type InterfaceSpec struct {
	Interface string `json:"interface"`
	Side      string `json:"side"`
}

// MapSpec is one deterministic mapping inside prefix -> outside prefix (det44_add_del_map).
type MapSpec struct {
	Inside  string `json:"inside"`
	Outside string `json:"outside"`
}

// Normalize masks and canonicalises both prefixes.
func (s *MapSpec) Normalize() {
	s.Inside, s.Outside = natcommon.CanonPrefix(s.Inside), natcommon.CanonPrefix(s.Outside)
}

// TimeoutsSpec is the timeout singleton (det44_set_timeouts / det44_get_timeouts); presence =
// non-default values.
type TimeoutsSpec struct {
	UDP            uint32 `json:"udp"`
	TCPEstablished uint32 `json:"tcp_established"`
	TCPTransitory  uint32 `json:"tcp_transitory"`
	ICMP           uint32 `json:"icmp"`
}

// IfMeta is the Meta of interface objects.
type IfMeta struct{ SwIfIndex uint32 }

// Plugin bundles the client, the owner scope and the det44 descriptors.
type Plugin struct {
	client vpp.Client
	scope  natcommon.Scope
	cfg    natcommon.Config
	svc    det44.RPCService
	feat   feature.RPCService

	mu      sync.Mutex
	lastVRF *EnableSpec // VRFs this process enabled det44 with (review finding 6)

	Enable    *natcommon.Descriptor[EnableSpec]
	Interface *natcommon.Descriptor[InterfaceSpec]
	Map       *natcommon.Descriptor[MapSpec]
	Timeouts  *natcommon.Descriptor[TimeoutsSpec]
}

// New constructs the family for client and owner.
func New(client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := &Plugin{client: client, scope: natcommon.ScopeFor(owner), cfg: natcommon.BuildConfig(opts), svc: det44.NewServiceClient(client), feat: feature.NewServiceClient(client)}
	p.Enable = p.newEnable()
	p.Timeouts = p.newTimeouts()
	p.Interface = p.newInterface()
	p.Map = p.newMap()
	return p
}

// Descriptors returns the family in registration order.
func (p *Plugin) Descriptors() []scheduler.Descriptor {
	return []scheduler.Descriptor{p.Enable, p.Timeouts, p.Interface, p.Map}
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

func enableDep() []scheduler.Dependency { return []scheduler.Dependency{natcommon.Dep(EnableKey)} }

func (p *Plugin) ownsMap(in, out netip.Prefix) bool {
	return p.scope.OwnsPrefix(in) || p.scope.OwnsPrefix(out)
}

func mapPrefixes(d *det44.Det44MapDetails) (netip.Prefix, netip.Prefix) {
	return netip.PrefixFrom(netip.AddrFrom4(d.InAddr), int(d.InPlen)).Masked(), netip.PrefixFrom(netip.AddrFrom4(d.OutAddr), int(d.OutPlen)).Masked()
}

// ErrVRFChangeUnsafe is returned when the desired inside/outside VRF differs from the one det44
// was enabled with: changing it needs det44_plugin_enable_disable(disable), which crashes VPP
// 26.06 (see Enable.Delete). The operator changes det44 VRFs with a VPP restart.
var ErrVRFChangeUnsafe = errors.New("det44: changing the det44 VRFs needs a plugin disable, which crashes VPP 26.06; restart VPP instead")

func (p *Plugin) claims() natcommon.ClaimStore { return p.cfg.Claims }

// hasObjects reports whether det44 holds any interface or map (all owners): det44 keeps no
// object while disabled, so this is the only evidence of "enabled".
func (p *Plugin) hasObjects(ctx context.Context) (bool, error) {
	is, err := p.svc.Det44InterfaceDump(ctx, &det44.Det44InterfaceDump{})
	if err != nil {
		return false, fmt.Errorf("det44_interface_dump: %w", err)
	}
	if n, err := natcommon.Count(is.Recv); err != nil || n > 0 {
		return n > 0, err
	}
	ms, err := p.svc.Det44MapDump(ctx, &det44.Det44MapDump{})
	if err != nil {
		return false, fmt.Errorf("det44_map_dump: %w", err)
	}
	n, err := natcommon.Count(ms.Recv)
	return n > 0, err
}

func (p *Plugin) enable(ctx context.Context, s EnableSpec) error {
	_, err := p.svc.Det44PluginEnableDisable(ctx, &det44.Det44PluginEnableDisable{Enable: true, InsideVrf: s.InsideVRF, OutsideVrf: s.OutsideVRF})
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case err == nil:
		v := s
		p.lastVRF = &v
		return nil
	case natcommon.IsAlreadyEnabled(err):
		if p.lastVRF != nil && *p.lastVRF != s {
			return fmt.Errorf("%w (enabled with %+v, desired %+v)", ErrVRFChangeUnsafe, *p.lastVRF, s)
		}
		return nil // enabled before this process: VRFs unverifiable (documented, det44.md)
	}
	return fmt.Errorf("det44_plugin_enable_disable: %w", err)
}

// newEnable: VPP-global (D-071, D-068). Owner: enable; a VRF change is refused
// (ErrVRFChangeUnsafe) because it needs det44_plugin_enable_disable(disable), which crashes
// VPP 26.06 (det44_plugin_disable iterates the interface *pool* as a vector and formats the
// failed delete with unformat_vnet_sw_interface → SIGSEGV; V9). Delete therefore NEVER
// disables — not even for the globals owner: the plugin stays enabled and idle until the next
// VPP restart. Non-owners require it (presence observable only through det44 objects).
func (p *Plugin) newEnable() *natcommon.Descriptor[EnableSpec] {
	return natcommon.Global(p.cfg, natcommon.GlobalOps[EnableSpec]{
		Name: NameEnable, ID: Singleton,
		Deps: func(s EnableSpec) []scheduler.Dependency {
			return natcommon.WithVRF(natcommon.WithVRF(nil, s.InsideVRF), s.OutsideVRF)
		},
		Read: func(ctx context.Context) (natcommon.GlobalState[EnableSpec], error) {
			found, err := p.hasObjects(ctx)
			return natcommon.GlobalState[EnableSpec]{Present: found, Observable: found}, err
		},
		Match:     natcommon.AnyValue[EnableSpec],
		WriteOnly: true,
		Set:       p.enable,
		SetUpdate: func(context.Context, EnableSpec, EnableSpec) error { return ErrVRFChangeUnsafe },
		Reset:     func(context.Context, EnableSpec) error { return nil },
	})
}

func (p *Plugin) setTimeouts(ctx context.Context, s TimeoutsSpec) error {
	if _, err := p.svc.Det44SetTimeouts(ctx, &det44.Det44SetTimeouts{UDP: s.UDP, TCPEstablished: s.TCPEstablished, TCPTransitory: s.TCPTransitory, ICMP: s.ICMP}); err != nil {
		return fmt.Errorf("det44_set_timeouts: %w", err)
	}
	return nil
}

// newTimeouts: VPP-global (D-071).
func (p *Plugin) newTimeouts() *natcommon.Descriptor[TimeoutsSpec] {
	return natcommon.Global(p.cfg, natcommon.GlobalOps[TimeoutsSpec]{
		Name: NameTimeouts, ID: Singleton,
		Deps: func(TimeoutsSpec) []scheduler.Dependency { return enableDep() },
		Read: func(ctx context.Context) (natcommon.GlobalState[TimeoutsSpec], error) {
			rep, err := p.svc.Det44GetTimeouts(ctx, &det44.Det44GetTimeouts{})
			if err != nil {
				return natcommon.GlobalState[TimeoutsSpec]{}, fmt.Errorf("det44_get_timeouts: %w", err)
			}
			t := TimeoutsSpec{UDP: rep.UDP, TCPEstablished: rep.TCPEstablished, TCPTransitory: rep.TCPTransitory, ICMP: rep.ICMP}
			return natcommon.GlobalState[TimeoutsSpec]{Value: t, Present: true, Observable: true}, nil
		},
		Absent: func(t TimeoutsSpec) bool { return t == DefaultTimeouts },
		Set:    p.setTimeouts,
		Reset:  func(ctx context.Context, _ TimeoutsSpec) error { return p.setTimeouts(ctx, DefaultTimeouts) },
	})
}

func isInside(side string) (bool, error) {
	switch side {
	case SideInside:
		return true, nil
	case SideOutside:
		return false, nil
	}
	return false, fmt.Errorf("det44: side must be %q or %q, got %q", SideInside, SideOutside, side)
}

// ---- feature arc repair (F-det44-cnat-fix; config-only fallback for a VPP 26.06 det44 bug) -----
//
// det44_interface_add_del() (src/plugins/nat/det44/det44.c) passes enable=1 to
// vnet_feature_enable_disable on its DELETE path as well: every det44_interface_add_del_feature
// is_add=0 puts one MORE det44-in2out (inside) or det44-out2in (outside) node on the interface's
// ip4-unicast arc instead of removing the one its add put there (add → 1, del → 2, add → 3, …;
// the ip4-sv-reassembly refcount is paired correctly). det44_interface_dump no longer lists the
// interface, but the nodes stay and drop every packet no det44 map matches ("No translation"):
// with det44 removed from the document, a CNAT VIP behind such an interface connected 0 of 12
// flows (F-det44-map-dslite-cnat-host run 2). VPP fix: pass 0 on the delete path (vpp-code-track,
// F-det44-cnat-fix questions Q1). Until then the interface descriptor keeps the arc exact through
// the generic feature API (binapi/feature):
//   - Delete: det44 delete, then feature_enable_disable(enable=0) of every det44 node of a side
//     det44 no longer holds, while feature_is_enabled reports it (VPP removes one instance per
//     disable and ignores the disable of a node that is not on the arc, so this is idempotent and
//     a no-op on a fixed VPP);
//   - Create: never enables twice for one desired object — when det44 already holds the interface
//     on that side nothing is sent; otherwise the nodes a former delete left behind are removed
//     first and det44 adds exactly one;
//   - Retrieve: det44 nodes on an owned interface that det44 does not hold on that side are
//     reported as the leftover object "<interface>#leftover/<side>", which is never desired, so
//     the reconciler deletes it (its Delete is the repair alone).
//
// feature_is_enabled casts its own errors (no such arc or feature node, an index beyond the arc's
// config vector) to is_enabled=true (vnet/feature/feature_api.c), so "both det44 nodes on one
// interface" is also what a VPP without det44 nodes answers — and what the agent's in-memory test
// VPP (coretest) answers for every node it does not model. That answer is not trusted (arcNodes):
// nothing is reported or repaired on such an interface. The descriptor itself never leaves nodes
// of both sides on one interface (Create clears both before its add); only an outside actor's
// churn on both sides of one interface stays unrepaired by this rule.

const (
	arcIP4 = "ip4-unicast"
	// The det44 graph nodes (feature names of det44.c; `show interface features` prints them).
	nodeInside  = "det44-in2out"
	nodeOutside = "det44-out2in"
	// LeftoverSuffix marks the interface name of a leftover object (natcommon.BaseName strips it).
	LeftoverSuffix = "#leftover"
	// maxRepair bounds the disables per node (each det44 delete leaves one instance).
	maxRepair = 256
)

var sides = []string{SideInside, SideOutside}

func featureNode(side string) string {
	if side == SideInside {
		return nodeInside
	}
	return nodeOutside
}

// holds reports whether a det44_interface_details entry holds the interface on side.
func holds(d *det44.Det44InterfaceDetails, side string) bool {
	if d == nil {
		return false
	}
	if side == SideInside {
		return d.IsInside
	}
	return d.IsOutside
}

// poolEntry returns det44's entry of sw_if_index idx (nil when det44 does not hold it).
func (p *Plugin) poolEntry(ctx context.Context, idx uint32) (*det44.Det44InterfaceDetails, error) {
	all, err := p.pool(ctx)
	if err != nil {
		return nil, err
	}
	return all[idx], nil
}

// pool returns every det44 interface entry by sw_if_index (all owners).
func (p *Plugin) pool(ctx context.Context) (map[uint32]*det44.Det44InterfaceDetails, error) {
	stream, err := p.svc.Det44InterfaceDump(ctx, &det44.Det44InterfaceDump{})
	if err != nil {
		return nil, fmt.Errorf("det44_interface_dump: %w", err)
	}
	out := map[uint32]*det44.Det44InterfaceDetails{}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("det44_interface_dump: %w", err)
		}
		if cur, ok := out[uint32(d.SwIfIndex)]; ok { // never two entries in VPP; merge defensively
			cur.IsInside, cur.IsOutside = cur.IsInside || d.IsInside, cur.IsOutside || d.IsOutside
			continue
		}
		out[uint32(d.SwIfIndex)] = d
	}
}

// errGone reports the "no such interface" answer of a feature call: the interface (and with it
// its whole feature arc) is gone, so there is nothing left to repair.
func errGone(err error) bool {
	rv, ok := natcommon.Retval(err)
	return ok && rv == api.INVALID_SW_IF_INDEX
}

// nodeOn is feature_is_enabled(ip4-unicast, node, idx).
func (p *Plugin) nodeOn(ctx context.Context, idx uint32, node string) (bool, error) {
	rep, err := p.feat.FeatureIsEnabled(ctx, &feature.FeatureIsEnabled{ArcName: arcIP4, FeatureName: node, SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		return false, fmt.Errorf("feature_is_enabled %s/%s sw_if_index=%d: %w", arcIP4, node, idx, err)
	}
	return rep.IsEnabled, nil
}

// arcNodes reports which det44 nodes feature_is_enabled finds on the ip4-unicast arc of idx; ok
// is false when the answer cannot be used (both nodes "on": see the arc repair comment above) or
// the interface is gone.
func (p *Plugin) arcNodes(ctx context.Context, idx uint32) (in, out, ok bool, err error) {
	if in, err = p.nodeOn(ctx, idx, nodeInside); err == nil {
		out, err = p.nodeOn(ctx, idx, nodeOutside)
	}
	switch {
	case errGone(err):
		return false, false, false, nil
	case err != nil:
		return false, false, false, err
	}
	return in, out, !(in && out), nil
}

// purge removes every instance of node from the ip4-unicast arc of idx and returns how many
// disables it sent. A gone interface has no arc left: nothing to do.
func (p *Plugin) purge(ctx context.Context, idx uint32, node string) (int, error) {
	for n := 0; n < maxRepair; n++ {
		on, err := p.nodeOn(ctx, idx, node)
		if errGone(err) {
			return n, nil
		}
		if err != nil || !on {
			return n, err
		}
		if _, err := p.feat.FeatureEnableDisable(ctx, &feature.FeatureEnableDisable{ArcName: arcIP4, FeatureName: node, SwIfIndex: interface_types.InterfaceIndex(idx), Enable: false}); err != nil {
			if errGone(err) {
				return n, nil
			}
			return n, fmt.Errorf("feature_enable_disable %s/%s sw_if_index=%d enable=0: %w", arcIP4, node, idx, err)
		}
	}
	return maxRepair, fmt.Errorf("det44: %s still on the %s arc of sw_if_index %d after %d disables", node, arcIP4, idx, maxRepair)
}

// repairArc removes from the ip4-unicast arc of idx the det44 node of every side det44 does not
// hold on idx (the node of a held side is det44's own and stays).
func (p *Plugin) repairArc(ctx context.Context, idx uint32) (int, error) {
	d, err := p.poolEntry(ctx, idx)
	if err != nil {
		return 0, err
	}
	in, out, ok, err := p.arcNodes(ctx, idx)
	if err != nil || !ok {
		return 0, err
	}
	on := map[string]bool{SideInside: in, SideOutside: out}
	total := 0
	for _, side := range sides {
		if holds(d, side) || !on[side] {
			continue
		}
		n, err := p.purge(ctx, idx, featureNode(side))
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (p *Plugin) newInterface() *natcommon.Descriptor[InterfaceSpec] {
	return natcommon.New(natcommon.Ops[InterfaceSpec]{
		Claims: p.claims(),
		Name:   NameInterface,
		ID:     func(s InterfaceSpec) string { return s.Interface + "/" + s.Side },
		Deps: func(s InterfaceSpec) []scheduler.Dependency {
			// a leftover object depends on (is deleted before) the interface it sits on
			return append(enableDep(), natcommon.InterfaceDep(natcommon.BaseName(s.Interface)))
		},
		Create: func(ctx context.Context, s InterfaceSpec) (any, error) {
			inside, err := isInside(s.Side)
			if err != nil {
				return nil, err
			}
			if strings.HasSuffix(s.Interface, LeftoverSuffix) {
				// only a rollback re-creates a leftover (undo of its repair): the repaired arc is the better
				// state, so nothing is sent; the claim the generic Create just recorded for the leftover key
				// is dropped again (Retrieve never needs it)
				if err := p.claims().Release(string(scheduler.Join(NameInterface, s.Interface+"/"+s.Side))); err != nil {
					return nil, fmt.Errorf("%s: release leftover claim: %w", NameInterface, err)
				}
				return IfMeta{}, nil
			}
			idx, err := natcommon.ResolveOwned(ctx, p.client, p.scope, s.Interface)
			if err != nil {
				return nil, err
			}
			meta := IfMeta{SwIfIndex: uint32(idx)}
			d, err := p.poolEntry(ctx, uint32(idx))
			if err != nil {
				return nil, err
			}
			if holds(d, s.Side) {
				return meta, nil // det44 already holds it on this side: never enable twice
			}
			// nodes a former delete left behind (VPP 26.06, see above) go first: det44 then adds exactly one
			if _, err := p.repairArc(ctx, uint32(idx)); err != nil {
				return nil, err
			}
			if _, err := p.svc.Det44InterfaceAddDelFeature(ctx, &det44.Det44InterfaceAddDelFeature{IsAdd: true, IsInside: inside, SwIfIndex: idx}); err != nil {
				return nil, fmt.Errorf("det44_interface_add_del_feature: %w", err)
			}
			return meta, nil
		},
		Delete: func(ctx context.Context, s InterfaceSpec, meta any) error {
			inside, err := isInside(s.Side)
			if err != nil {
				return err
			}
			m, ok := meta.(IfMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameInterface, meta)
			}
			if m.SwIfIndex == 0 && strings.HasSuffix(s.Interface, LeftoverSuffix) {
				return nil // a leftover a rollback "re-created" (no VPP object, no index): nothing to repair
			}
			if !strings.HasSuffix(s.Interface, LeftoverSuffix) {
				if _, err := p.svc.Det44InterfaceAddDelFeature(ctx, &det44.Det44InterfaceAddDelFeature{IsAdd: false, IsInside: inside, SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil && !natcommon.IsNoSuchEntry(err) {
					// VPP answers INVALID_VALUE when det44 does not hold the interface (any more): gone is done
					d, perr := p.poolEntry(ctx, m.SwIfIndex)
					if perr != nil || holds(d, s.Side) {
						return fmt.Errorf("det44_interface_add_del_feature: %w", err)
					}
				}
			}
			if _, err := p.repairArc(ctx, m.SwIfIndex); err != nil {
				return fmt.Errorf("%s: feature arc repair: %w", NameInterface, err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[InterfaceSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			stream, err := p.svc.Det44InterfaceDump(ctx, &det44.Det44InterfaceDump{})
			if err != nil {
				return nil, fmt.Errorf("det44_interface_dump: %w", err)
			}
			var out []natcommon.Item[InterfaceSpec]
			pool := map[uint32]*det44.Det44InterfaceDetails{}
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return nil, fmt.Errorf("det44_interface_dump: %w", err)
				}
				pool[uint32(d.SwIfIndex)] = d
				i, _ := ifaces.ByIndex(uint32(d.SwIfIndex))
				ok, nc := p.scope.InterfaceOwnership(i)
				if !ok {
					continue
				}
				meta := IfMeta{SwIfIndex: uint32(d.SwIfIndex)}
				if d.IsInside {
					out = append(out, natcommon.Item[InterfaceSpec]{Spec: InterfaceSpec{Interface: p.scope.LogicalName(i), Side: SideInside}, Meta: meta, NeedsClaim: nc})
				}
				if d.IsOutside {
					out = append(out, natcommon.Item[InterfaceSpec]{Spec: InterfaceSpec{Interface: p.scope.LogicalName(i), Side: SideOutside}, Meta: meta, NeedsClaim: nc})
				}
			}
			leftovers, err := p.leftovers(ctx, ifaces, pool)
			if err != nil {
				return nil, err
			}
			return append(out, leftovers...), nil
		},
	})
}

// leftovers reports the det44 nodes on owned interfaces that det44 does not hold on that side
// (VPP 26.06's delete leaves them, see above) as "<interface>#leftover/<side>" objects. An
// untagged interface (ownership by claim, D-071) is looked at only where this owner holds the
// claim of the det44 interface object on that side.
func (p *Plugin) leftovers(ctx context.Context, ifaces *natcommon.IfaceTable, pool map[uint32]*det44.Det44InterfaceDetails) ([]natcommon.Item[InterfaceSpec], error) {
	var out []natcommon.Item[InterfaceSpec]
	for _, i := range ifaces.All() {
		ok, nc := p.scope.InterfaceOwnership(i)
		if !ok {
			continue
		}
		name := p.scope.LogicalName(i)
		var look []string
		for _, side := range sides {
			if holds(pool[i.SwIfIndex], side) {
				continue
			}
			if nc && !p.claims().Claimed(string(scheduler.Join(NameInterface, name+"/"+side))) {
				continue
			}
			look = append(look, side)
		}
		if len(look) == 0 {
			continue
		}
		in, outside, ok, err := p.arcNodes(ctx, i.SwIfIndex)
		if err != nil {
			return nil, err
		}
		if !ok { // gone since the interface dump, or an answer that cannot be used
			continue
		}
		on := map[string]bool{SideInside: in, SideOutside: outside}
		for _, side := range look {
			if on[side] {
				out = append(out, natcommon.Item[InterfaceSpec]{Spec: InterfaceSpec{Interface: name + LeftoverSuffix, Side: side}, Meta: IfMeta{SwIfIndex: i.SwIfIndex}})
			}
		}
	}
	return out, nil
}

func (p *Plugin) addDelMap(ctx context.Context, s MapSpec, add bool) error {
	in, err := natcommon.Prefix4(s.Inside)
	if err != nil {
		return fmt.Errorf("inside: %w", err)
	}
	out, err := natcommon.Prefix4(s.Outside)
	if err != nil {
		return fmt.Errorf("outside: %w", err)
	}
	if _, err := p.svc.Det44AddDelMap(ctx, &det44.Det44AddDelMap{IsAdd: add, InAddr: in.Address, InPlen: in.Len, OutAddr: out.Address, OutPlen: out.Len}); err != nil {
		if !add && natcommon.IsNoSuchEntry(err) {
			return nil
		}
		return fmt.Errorf("det44_add_del_map: %w", err)
	}
	return nil
}

func (p *Plugin) newMap() *natcommon.Descriptor[MapSpec] {
	return natcommon.New(natcommon.Ops[MapSpec]{
		Claims: p.claims(),
		Name:   NameMap,
		ID:     func(s MapSpec) string { return s.Inside + "/" + s.Outside },
		Deps:   func(MapSpec) []scheduler.Dependency { return enableDep() },
		Create: func(ctx context.Context, s MapSpec) (any, error) { return nil, p.addDelMap(ctx, s, true) },
		Delete: func(ctx context.Context, s MapSpec, _ any) error { return p.addDelMap(ctx, s, false) },
		Retrieve: func(ctx context.Context) ([]natcommon.Item[MapSpec], error) {
			stream, err := p.svc.Det44MapDump(ctx, &det44.Det44MapDump{})
			if err != nil {
				return nil, fmt.Errorf("det44_map_dump: %w", err)
			}
			var out []natcommon.Item[MapSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("det44_map_dump: %w", err)
				}
				in, o := mapPrefixes(d)
				out = append(out, natcommon.Item[MapSpec]{Spec: MapSpec{Inside: in.String(), Outside: o.String()}, NeedsClaim: p.scope.NeedsClaim(p.ownsMap(in, o))})
			}
		},
	})
}

// Session is one det44 session of a user (det44_session_details), Retrieve-only state.
type Session struct {
	InsidePort   uint32 `json:"inside_port"`
	OutsidePort  uint32 `json:"outside_port"`
	ExternalIP   string `json:"external_ip"`
	ExternalPort uint32 `json:"external_port"`
	State        uint32 `json:"state"`
	Expire       uint32 `json:"expire"`
}

// Sessions returns one page (offset, limit; 0 = all) of the sessions of an inside user.
func (p *Plugin) Sessions(ctx context.Context, userIP string, offset, limit int) ([]Session, error) {
	addr, err := natcommon.IP4(userIP)
	if err != nil {
		return nil, err
	}
	stream, err := p.svc.Det44SessionDump(ctx, &det44.Det44SessionDump{UserAddr: addr})
	if err != nil {
		return nil, fmt.Errorf("det44_session_dump: %w", err)
	}
	var out []Session
	for i := 0; ; i++ {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("det44_session_dump: %w", err)
		}
		if i < offset || (limit > 0 && len(out) >= limit) {
			continue
		}
		out = append(out, Session{InsidePort: uint32(d.InPort), OutsidePort: uint32(d.OutPort), ExternalIP: natcommon.IP4String(d.ExtAddr), ExternalPort: uint32(d.ExtPort), State: uint32(d.State), Expire: d.Expire})
	}
}

// CloseSessionIn closes a session by its inside endpoint (det44_close_session_in).
func (p *Plugin) CloseSessionIn(ctx context.Context, inIP string, inPort uint32, extIP string, extPort uint32) error {
	in, err := natcommon.IP4(inIP)
	if err != nil {
		return err
	}
	ext, err := natcommon.IP4(extIP)
	if err != nil {
		return err
	}
	if inPort > 65535 || extPort > 65535 {
		return fmt.Errorf("det44: port out of range")
	}
	if _, err := p.svc.Det44CloseSessionIn(ctx, &det44.Det44CloseSessionIn{InAddr: in, InPort: uint16(inPort), ExtAddr: ext, ExtPort: uint16(extPort)}); err != nil {
		return fmt.Errorf("det44_close_session_in: %w", err)
	}
	return nil
}

// CloseSessionOut closes a session by its outside endpoint (det44_close_session_out).
func (p *Plugin) CloseSessionOut(ctx context.Context, outIP string, outPort uint32, extIP string, extPort uint32) error {
	out, err := natcommon.IP4(outIP)
	if err != nil {
		return err
	}
	ext, err := natcommon.IP4(extIP)
	if err != nil {
		return err
	}
	if outPort > 65535 || extPort > 65535 {
		return fmt.Errorf("det44: port out of range")
	}
	if _, err := p.svc.Det44CloseSessionOut(ctx, &det44.Det44CloseSessionOut{OutAddr: out, OutPort: uint16(outPort), ExtAddr: ext, ExtPort: uint16(extPort)}); err != nil {
		return fmt.Errorf("det44_close_session_out: %w", err)
	}
	return nil
}
