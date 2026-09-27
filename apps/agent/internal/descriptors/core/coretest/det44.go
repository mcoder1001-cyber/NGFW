package coretest

// F-det44-map-dslite-cnat: stateful models of the det44 and dslite plugins (DF-3's descriptors/det44, this task's
// descriptors/dslite) following VPP 26.06's det44_api.c / dslite_api.c where the agent depends on them: the det44
// enable answers the bare retval 1 when already enabled (a duplicate enable must be tolerated, D-076) and counts its
// enables; a det44 DISABLE is a test failure (V9: it segfaults VPP 26.06); interface and map calls on a disabled
// plugin fail; the dslite AFTR/B4 are plain overwrites, pool addresses are kept one by one.

import (
	"net/netip"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/det44"
	"ngfw/agent/binapi/dslite"
)

// Det44 is the det44 plugin model; tests may read and seed the exported fields under Lock/Unlock.
type Det44 struct {
	v  *VPP
	mu sync.Mutex

	Enabled  bool
	Enables  int // successful + duplicate enable calls
	Disables int // must stay 0 (V9)
	Timeouts det44.Det44GetTimeoutsReply
	Ifaces   map[uint32]*det44.Det44InterfaceDetails
	Maps     []*det44.Det44MapDetails
}

// Lock guards the exported fields.
func (d *Det44) Lock() { d.mu.Lock() }

// Unlock releases Lock.
func (d *Det44) Unlock() { d.mu.Unlock() }

// Dslite is the dslite plugin model.
type Dslite struct {
	v  *VPP
	mu sync.Mutex

	Aftr, B4 dslite.DsliteGetAftrAddrReply
	Pool     map[netip.Addr]bool
	Sets     int
}

// Lock guards the exported fields.
func (d *Dslite) Lock() { d.mu.Lock() }

// Unlock releases Lock.
func (d *Dslite) Unlock() { d.mu.Unlock() }

var det44Models, dsliteModels sync.Map

func init() {
	RegisterExtension("det44-dslite", func(v *VPP) {
		det44Models.Store(v, v.installDet44())
		dsliteModels.Store(v, v.installDslite())
	})
}

// Det44 returns the det44 plugin model of v (starts disabled).
func (v *VPP) Det44() *Det44 {
	m, ok := det44Models.Load(v)
	if !ok {
		m, _ = det44Models.LoadOrStore(v, v.installDet44())
	}
	return m.(*Det44)
}

// Dslite returns the dslite plugin model of v.
func (v *VPP) Dslite() *Dslite {
	m, ok := dsliteModels.Load(v)
	if !ok {
		m, _ = dsliteModels.LoadOrStore(v, v.installDslite())
	}
	return m.(*Dslite)
}

func (v *VPP) installDet44() *Det44 {
	d := &Det44{v: v, Ifaces: map[uint32]*det44.Det44InterfaceDetails{},
		Timeouts: det44.Det44GetTimeoutsReply{UDP: 300, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60}}
	one := func(m api.Message) ([]api.Message, error) { return []api.Message{m}, nil }
	rv := func(e api.VPPApiError) int32 { return int32(e) }
	v.On("det44_plugin_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*det44.Det44PluginEnableDisable)
		d.mu.Lock()
		defer d.mu.Unlock()
		if !r.Enable {
			d.Disables++
			return one(&det44.Det44PluginEnableDisableReply{Retval: rv(api.UNSUPPORTED)}) // V9: would crash VPP 26.06
		}
		d.Enables++
		if d.Enabled {
			return one(&det44.Det44PluginEnableDisableReply{Retval: 1})
		}
		d.Enabled = true
		return one(&det44.Det44PluginEnableDisableReply{})
	})
	v.On("det44_get_timeouts", func(api.Message) ([]api.Message, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		t := d.Timeouts
		return one(&t)
	})
	v.On("det44_set_timeouts", func(m api.Message) ([]api.Message, error) {
		r := m.(*det44.Det44SetTimeouts)
		d.mu.Lock()
		defer d.mu.Unlock()
		d.Timeouts = det44.Det44GetTimeoutsReply{UDP: r.UDP, TCPEstablished: r.TCPEstablished, TCPTransitory: r.TCPTransitory, ICMP: r.ICMP}
		return one(&det44.Det44SetTimeoutsReply{})
	})
	v.On("det44_interface_add_del_feature", func(m api.Message) ([]api.Message, error) {
		r := m.(*det44.Det44InterfaceAddDelFeature)
		idx := uint32(r.SwIfIndex)
		if !v.ifExists(idx) {
			return one(&det44.Det44InterfaceAddDelFeatureReply{Retval: rv(api.INVALID_SW_IF_INDEX)})
		}
		d.mu.Lock()
		defer d.mu.Unlock()
		if !d.Enabled {
			return one(&det44.Det44InterfaceAddDelFeatureReply{Retval: rv(api.UNSUPPORTED)})
		}
		x, ok := d.Ifaces[idx]
		if !ok {
			x = &det44.Det44InterfaceDetails{SwIfIndex: r.SwIfIndex}
			d.Ifaces[idx] = x
		}
		switch {
		case r.IsAdd && r.IsInside:
			x.IsInside = true
		case r.IsAdd:
			x.IsOutside = true
		case r.IsInside:
			x.IsInside = false
		default:
			x.IsOutside = false
		}
		if !x.IsInside && !x.IsOutside {
			delete(d.Ifaces, idx)
		}
		return one(&det44.Det44InterfaceAddDelFeatureReply{})
	})
	v.On("det44_interface_dump", func(api.Message) ([]api.Message, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedIdx(d.Ifaces) {
			c := *d.Ifaces[idx]
			out = append(out, &c)
		}
		return out, nil
	})
	v.On("det44_add_del_map", func(m api.Message) ([]api.Message, error) {
		r := m.(*det44.Det44AddDelMap)
		d.mu.Lock()
		defer d.mu.Unlock()
		if !d.Enabled {
			return one(&det44.Det44AddDelMapReply{Retval: rv(api.UNSUPPORTED)})
		}
		for i, x := range d.Maps {
			if x.InAddr == r.InAddr && x.InPlen == r.InPlen {
				if r.IsAdd {
					return one(&det44.Det44AddDelMapReply{Retval: rv(api.VALUE_EXIST)})
				}
				d.Maps = append(d.Maps[:i], d.Maps[i+1:]...)
				return one(&det44.Det44AddDelMapReply{})
			}
		}
		if !r.IsAdd {
			return one(&det44.Det44AddDelMapReply{Retval: rv(api.NO_SUCH_ENTRY)})
		}
		ratio := uint32(1) << (uint32(r.OutPlen) - uint32(r.InPlen))
		d.Maps = append(d.Maps, &det44.Det44MapDetails{InAddr: r.InAddr, InPlen: r.InPlen, OutAddr: r.OutAddr, OutPlen: r.OutPlen,
			SharingRatio: ratio, PortsPerHost: uint16((65535 - 1023) / ratio)})
		return one(&det44.Det44AddDelMapReply{})
	})
	v.On("det44_map_dump", func(api.Message) ([]api.Message, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		out := make([]api.Message, 0, len(d.Maps))
		for _, x := range d.Maps {
			c := *x
			out = append(out, &c)
		}
		return out, nil
	})
	return d
}

func (v *VPP) installDslite() *Dslite {
	d := &Dslite{v: v, Pool: map[netip.Addr]bool{}}
	one := func(m api.Message) ([]api.Message, error) { return []api.Message{m}, nil }
	v.On("dslite_get_aftr_addr", func(api.Message) ([]api.Message, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		r := d.Aftr
		return one(&r)
	})
	v.On("dslite_set_aftr_addr", func(m api.Message) ([]api.Message, error) {
		r := m.(*dslite.DsliteSetAftrAddr)
		d.mu.Lock()
		defer d.mu.Unlock()
		d.Sets++
		d.Aftr = dslite.DsliteGetAftrAddrReply{IP4Addr: r.IP4Addr, IP6Addr: r.IP6Addr}
		return one(&dslite.DsliteSetAftrAddrReply{})
	})
	v.On("dslite_get_b4_addr", func(api.Message) ([]api.Message, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		return one(&dslite.DsliteGetB4AddrReply{IP4Addr: d.B4.IP4Addr, IP6Addr: d.B4.IP6Addr})
	})
	v.On("dslite_set_b4_addr", func(m api.Message) ([]api.Message, error) {
		r := m.(*dslite.DsliteSetB4Addr)
		d.mu.Lock()
		defer d.mu.Unlock()
		d.Sets++
		d.B4 = dslite.DsliteGetAftrAddrReply{IP4Addr: r.IP4Addr, IP6Addr: r.IP6Addr}
		return one(&dslite.DsliteSetB4AddrReply{})
	})
	v.On("dslite_add_del_pool_addr_range", func(m api.Message) ([]api.Message, error) {
		r := m.(*dslite.DsliteAddDelPoolAddrRange)
		d.mu.Lock()
		defer d.mu.Unlock()
		a, b := netip.AddrFrom4(r.StartAddr), netip.AddrFrom4(r.EndAddr)
		for x := a; !b.Less(x); x = x.Next() {
			if r.IsAdd == d.Pool[x] {
				e := api.VALUE_EXIST
				if !r.IsAdd {
					e = api.NO_SUCH_ENTRY
				}
				return one(&dslite.DsliteAddDelPoolAddrRangeReply{Retval: int32(e)})
			}
		}
		for x := a; !b.Less(x); x = x.Next() {
			if r.IsAdd {
				d.Pool[x] = true
			} else {
				delete(d.Pool, x)
			}
		}
		return one(&dslite.DsliteAddDelPoolAddrRangeReply{})
	})
	v.On("dslite_address_dump", func(api.Message) ([]api.Message, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		var out []api.Message
		for a := range d.Pool {
			out = append(out, &dslite.DsliteAddressDetails{IPAddress: a.As4()})
		}
		return out, nil
	})
	return d
}
