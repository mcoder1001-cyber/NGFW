package coretest

// F-nat44-ei-64-66-nptv6: a model of VPP 26.06's npt66 plugin (npt66.c npt66_binding_add_del), the only message the
// plugin has. It models what makes the descriptor write-only and what makes its re-apply safe:
//   - one binding per interface; an add on an interface that already has one overwrites it in place and does NOT
//     enable the npt66-input / npt66-output features again (FeatureEnables counts the enables per interface);
//   - prefixes longer than /64 → INVALID_VALUE, an unknown sw_if_index → INVALID_SW_IF_INDEX;
//   - a delete finds the binding by sw_if_index only (the prefixes are ignored), NO_SUCH_ENTRY when there is none;
//   - deleting the interface leaves its binding behind (VPP has no interface-delete hook in npt66): Stale reports
//     such bindings, so tests prove the binding is deleted before its interface (D-095c).

import (
	"net/netip"
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/npt66"
)

// NPT66Binding is one modelled binding (prefixes masked as VPP stores them).
type NPT66Binding struct {
	SwIfIndex          uint32
	Internal, External netip.Prefix
}

// NPT66 is the npt66 plugin model; tests may read and seed the exported fields under Lock/Unlock.
type NPT66 struct {
	v  *VPP
	mu sync.Mutex

	// Bindings by sw_if_index (VPP: interface_by_sw_if_index → bindings pool).
	Bindings map[uint32]NPT66Binding
	// FeatureEnables is the number of times the npt66 features were enabled on each interface minus the disables:
	// 1 while a binding exists, whatever the number of adds.
	FeatureEnables map[uint32]int
	// Adds and Dels count the messages (successful or not).
	Adds, Dels int
}

// Lock guards the exported fields while a test seeds or inspects them.
func (n *NPT66) Lock() { n.mu.Lock() }

// Unlock releases Lock.
func (n *NPT66) Unlock() { n.mu.Unlock() }

// Count is the number of bindings.
func (n *NPT66) Count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.Bindings)
}

// Binding returns the binding of an interface (by name).
func (n *NPT66) Binding(ifName string) (NPT66Binding, bool) {
	i, ok := n.v.InterfaceByName(ifName)
	if !ok {
		return NPT66Binding{}, false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	b, ok := n.Bindings[i.Index]
	return b, ok
}

// Stale returns the sw_if_indexes of bindings whose interface no longer exists (sorted).
func (n *NPT66) Stale() []uint32 {
	n.mu.Lock()
	idxs := make([]uint32, 0, len(n.Bindings))
	for idx := range n.Bindings {
		idxs = append(idxs, idx)
	}
	n.mu.Unlock()
	n.v.mu.Lock()
	defer n.v.mu.Unlock()
	var out []uint32
	for _, idx := range idxs {
		if _, ok := n.v.Ifaces[idx]; !ok {
			out = append(out, idx)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}

// DeleteBinding removes an interface's binding behind the agent's back (simulated loss); false when it had none.
func (n *NPT66) DeleteBinding(ifName string) bool {
	i, ok := n.v.InterfaceByName(ifName)
	if !ok {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.Bindings[i.Index]; !ok {
		return false
	}
	delete(n.Bindings, i.Index)
	n.FeatureEnables[i.Index]--
	return true
}

var npt66Models sync.Map

func init() {
	extensions = append(extensions, func(v *VPP) { npt66Models.Store(v, v.installNPT66()) })
}

// NPT66 returns the npt66 plugin model of v.
func (v *VPP) NPT66() *NPT66 {
	m, ok := npt66Models.Load(v)
	if !ok {
		m, _ = npt66Models.LoadOrStore(v, v.installNPT66())
	}
	return m.(*NPT66)
}

func (v *VPP) installNPT66() *NPT66 {
	n := &NPT66{v: v, Bindings: map[uint32]NPT66Binding{}, FeatureEnables: map[uint32]int{}}
	v.On("npt66_binding_add_del", func(m api.Message) ([]api.Message, error) {
		r := m.(*npt66.Npt66BindingAddDel)
		idx := uint32(r.SwIfIndex)
		v.mu.Lock()
		_, exists := v.Ifaces[idx]
		v.mu.Unlock()
		n.mu.Lock()
		defer n.mu.Unlock()
		rep := &npt66.Npt66BindingAddDelReply{}
		if r.IsAdd {
			n.Adds++
		} else {
			n.Dels++
		}
		switch {
		case !exists:
			rep.Retval = int32(api.INVALID_SW_IF_INDEX)
		case r.IsAdd && (r.Internal.Len > 64 || r.External.Len > 64):
			rep.Retval = int32(api.INVALID_VALUE)
		case r.IsAdd:
			if _, had := n.Bindings[idx]; !had {
				n.FeatureEnables[idx]++ // configure_feature only for a new binding
			}
			in := netip.PrefixFrom(netip.AddrFrom16(r.Internal.Address), int(r.Internal.Len)).Masked()
			ex := netip.PrefixFrom(netip.AddrFrom16(r.External.Address), int(r.External.Len)).Masked()
			n.Bindings[idx] = NPT66Binding{SwIfIndex: idx, Internal: in, External: ex}
		default:
			if _, had := n.Bindings[idx]; !had {
				rep.Retval = int32(api.NO_SUCH_ENTRY)
				break
			}
			delete(n.Bindings, idx)
			n.FeatureEnables[idx]--
		}
		return []api.Message{rep}, nil
	})
	return n
}
