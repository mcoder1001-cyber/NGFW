package coretest

// F-det44-map-dslite-cnat: stateful model of the pnat plugin (DF-3's descriptors/pnat), ported from DF-3's per-package
// fake and VPP 26.06's pnat_api.c: a binding pool with holes (the lowest free slot is reused), pnat_bindings_get's
// cursor semantics, the flow hash keyed by (sw_if_index, point, match), and the V11 crash conditions — a flow lookup
// or detach while no interface was ever attached is recorded in Crashes (it must stay empty).

import (
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	pnatapi "ngfw/agent/binapi/pnat"
)

// PnatFlow keys the flow hash.
type PnatFlow struct {
	SwIfIndex uint32
	Point     pnatapi.PnatAttachmentPoint
	Match     pnatapi.PnatMatchTuple
}

// Pnat is the pnat plugin model.
type Pnat struct {
	mu sync.Mutex

	Pool    []*pnatapi.PnatBindingsDetails // nil = free slot
	Flows   map[PnatFlow]uint32
	Ifaces  map[uint32]int // sw_if_index → attached flows
	Crashes []string       // calls that would crash VPP 26.06 (V11); must stay empty
}

// Lock guards the exported fields.
func (p *Pnat) Lock() { p.mu.Lock() }

// Unlock releases Lock.
func (p *Pnat) Unlock() { p.mu.Unlock() }

var pnatModels sync.Map

func init() {
	RegisterExtension("pnat", func(v *VPP) { pnatModels.Store(v, v.installPnat()) })
}

// Pnat returns the pnat plugin model of v.
func (v *VPP) Pnat() *Pnat {
	m, ok := pnatModels.Load(v)
	if !ok {
		m, _ = pnatModels.LoadOrStore(v, v.installPnat())
	}
	return m.(*Pnat)
}

func (v *VPP) installPnat() *Pnat {
	p := &Pnat{Flows: map[PnatFlow]uint32{}, Ifaces: map[uint32]int{}}
	one := func(x api.Message) ([]api.Message, error) { return []api.Message{x}, nil }
	v.On("pnat_binding_add_v2", func(x api.Message) ([]api.Message, error) {
		r := x.(*pnatapi.PnatBindingAddV2)
		p.mu.Lock()
		defer p.mu.Unlock()
		d := &pnatapi.PnatBindingsDetails{Match: r.Match, Rewrite: r.Rewrite}
		for i, s := range p.Pool {
			if s == nil {
				p.Pool[i] = d
				return one(&pnatapi.PnatBindingAddV2Reply{BindingIndex: uint32(i)}) //nolint:gosec // model index
			}
		}
		p.Pool = append(p.Pool, d)
		return one(&pnatapi.PnatBindingAddV2Reply{BindingIndex: uint32(len(p.Pool) - 1)}) //nolint:gosec // model index
	})
	v.On("pnat_binding_del", func(x api.Message) ([]api.Message, error) {
		i := x.(*pnatapi.PnatBindingDel).BindingIndex
		p.mu.Lock()
		defer p.mu.Unlock()
		if int(i) >= len(p.Pool) || p.Pool[i] == nil {
			return one(&pnatapi.PnatBindingDelReply{Retval: -1})
		}
		p.Pool[i] = nil
		return one(&pnatapi.PnatBindingDelReply{})
	})
	v.On("pnat_bindings_get", func(x api.Message) ([]api.Message, error) {
		c := int(x.(*pnatapi.PnatBindingsGet).Cursor)
		p.mu.Lock()
		defer p.mu.Unlock()
		var out []api.Message
		for i := c; i < len(p.Pool); i++ {
			if p.Pool[i] != nil {
				d := *p.Pool[i]
				out = append(out, &d)
			}
		}
		rep := &pnatapi.PnatBindingsGetReply{Cursor: ^uint32(0)}
		if len(out) == 0 {
			rep.Retval = int32(api.INVALID_VALUE)
		}
		return append(out, rep), nil
	})
	v.On("pnat_interfaces_get", func(api.Message) ([]api.Message, error) {
		p.mu.Lock()
		defer p.mu.Unlock()
		var out []api.Message
		for sw := uint32(0); sw < 4096; sw++ {
			if p.Ifaces[sw] > 0 {
				out = append(out, &pnatapi.PnatInterfacesDetails{SwIfIndex: interface_types.InterfaceIndex(sw), Enabled: []bool{true, true}})
			}
		}
		return append(out, &pnatapi.PnatInterfacesGetReply{Cursor: ^uint32(0)}), nil
	})
	v.On("pnat_binding_attach", func(x api.Message) ([]api.Message, error) {
		r := x.(*pnatapi.PnatBindingAttach)
		p.mu.Lock()
		defer p.mu.Unlock()
		if int(r.BindingIndex) >= len(p.Pool) || p.Pool[r.BindingIndex] == nil {
			return one(&pnatapi.PnatBindingAttachReply{Retval: -1})
		}
		k := PnatFlow{uint32(r.SwIfIndex), r.Attachment, p.Pool[r.BindingIndex].Match}
		if _, dup := p.Flows[k]; dup {
			return one(&pnatapi.PnatBindingAttachReply{Retval: -3})
		}
		p.Flows[k] = r.BindingIndex
		p.Ifaces[uint32(r.SwIfIndex)]++
		return one(&pnatapi.PnatBindingAttachReply{})
	})
	v.On("pnat_binding_detach", func(x api.Message) ([]api.Message, error) {
		r := x.(*pnatapi.PnatBindingDetach)
		p.mu.Lock()
		defer p.mu.Unlock()
		if len(p.Ifaces) == 0 {
			p.Crashes = append(p.Crashes, "pnat_binding_detach")
			return one(&pnatapi.PnatBindingDetachReply{Retval: -1})
		}
		if int(r.BindingIndex) >= len(p.Pool) || p.Pool[r.BindingIndex] == nil {
			return one(&pnatapi.PnatBindingDetachReply{Retval: -1})
		}
		k := PnatFlow{uint32(r.SwIfIndex), r.Attachment, p.Pool[r.BindingIndex].Match}
		if _, ok := p.Flows[k]; !ok {
			return one(&pnatapi.PnatBindingDetachReply{Retval: -2})
		}
		delete(p.Flows, k)
		if p.Ifaces[uint32(r.SwIfIndex)]--; p.Ifaces[uint32(r.SwIfIndex)] == 0 {
			delete(p.Ifaces, uint32(r.SwIfIndex))
		}
		return one(&pnatapi.PnatBindingDetachReply{})
	})
	v.On("pnat_flow_lookup", func(x api.Message) ([]api.Message, error) {
		r := x.(*pnatapi.PnatFlowLookup)
		p.mu.Lock()
		defer p.mu.Unlock()
		if len(p.Ifaces) == 0 {
			p.Crashes = append(p.Crashes, "pnat_flow_lookup")
			return one(&pnatapi.PnatFlowLookupReply{Retval: -1, BindingIndex: ^uint32(0)})
		}
		if i, ok := p.Flows[PnatFlow{uint32(r.SwIfIndex), r.Attachment, r.Match}]; ok {
			return one(&pnatapi.PnatFlowLookupReply{BindingIndex: i})
		}
		return one(&pnatapi.PnatFlowLookupReply{Retval: -1, BindingIndex: ^uint32(0)})
	})
	return p
}
