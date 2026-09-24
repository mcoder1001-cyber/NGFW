// Package coretest is a stateful in-memory model of the VPP objects the P05 core descriptors
// manage (interfaces, tags, FIB tables, interface addresses and table bindings, routes), built on
// internal/vpp/fake. Unit tests of the core descriptors and of the agent (projection, service,
// gRPC) run against it unchanged; integration tests run the same code against the host VPP.
package coretest

import (
	"fmt"
	"net/netip"
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/vpe"
	"ngfw/agent/internal/vpp/fake"
)

// VPP retvals used by the model (vnet/api_errno.h).
const (
	RetvalInvalidSwIfIndex int32 = -2
	RetvalNoSuchFib        int32 = -3
	RetvalInstanceInUse    int32 = -69
	RetvalAddressInUse     int32 = -105
	RetvalNoSuchEntry      int32 = -6
)

// Iface is one modelled interface.
type Iface struct {
	Index   uint32
	Name    string
	DevType string
	Tag     string
	Table4  uint32
	Table6  uint32
	Addrs   map[string]bool // canonical "addr/len"
	// P08 (ifext.go): the attributes DF-1's descriptors read and write.
	AdminUp  bool
	LinkMtu  uint16
	Mtu      [4]uint32
	RxMode   interface_types.RxMode
	L2       [6]uint8
	HostIf   string // af_packet: the Linux netdev
	Sup      uint32 // sub-interface: parent sw_if_index
	SubID    uint32
	SubFlags interface_types.SubIfFlags
	Outer    uint16
	Inner    uint16
	IsSub    bool
}

type tableKey struct {
	id uint32
	v6 bool
}

type routeKey struct {
	table  uint32
	prefix string
}

// VPP is the model. All fields are guarded by mu; tests may inspect them via the helpers.
type VPP struct {
	*fake.Client
	mu     sync.Mutex
	next   uint32
	Ifaces map[uint32]*Iface
	Tables map[tableKey]string // name
	Routes map[routeKey]ip.IPRoute
	// Internal are VPP-generated FIB entries (source id, e.g. 18 recursive-resolution, 4 interface,
	// 15 adjacency); an API route on the same prefix is reported instead (API outranks them).
	Internal map[routeKey]uint8
}

// New returns a model with local0 and the default tables.
func New() *VPP {
	v := &VPP{
		Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})),
		next:   1,
		Ifaces: map[uint32]*Iface{0: {Index: 0, Name: "local0", DevType: "local", Addrs: map[string]bool{}}},
		Tables: map[tableKey]string{{0, false}: "ipv4-VRF:0", {0, true}: "ipv6-VRF:0"},
		Routes:   map[routeKey]ip.IPRoute{},
		Internal: map[routeKey]uint8{},
	}
	v.install()
	v.installIfExt() // P08: DF-1 attributes, af_packet, sub-interfaces, DHCP client dump
	return v
}

func reply(m api.Message) ([]api.Message, error) { return []api.Message{m}, nil }

func (v *VPP) install() {
	v.On("show_version", func(api.Message) ([]api.Message, error) {
		return reply(&vpe.ShowVersionReply{Program: "vpe", Version: "26.06-fake"})
	})
	v.On("want_interface_events", func(api.Message) ([]api.Message, error) {
		return reply(&interfaces.WantInterfaceEventsReply{})
	})
	v.On("create_loopback_instance", func(m api.Message) ([]api.Message, error) {
		req := m.(*interfaces.CreateLoopbackInstance)
		v.mu.Lock()
		defer v.mu.Unlock()
		name := fmt.Sprintf("loop%d", req.UserInstance)
		for _, i := range v.Ifaces {
			if i.Name == name {
				return reply(&interfaces.CreateLoopbackInstanceReply{Retval: RetvalInstanceInUse})
			}
		}
		idx := v.next
		v.next++
		v.Ifaces[idx] = &Iface{Index: idx, Name: name, DevType: "Loopback", Addrs: map[string]bool{}, LinkMtu: 9000, Mtu: [4]uint32{9000}, RxMode: interface_types.RX_MODE_API_POLLING, L2: [6]uint8{0xde, 0xad, 0, 0, 0, uint8(idx)}}
		return reply(&interfaces.CreateLoopbackInstanceReply{SwIfIndex: interface_types.InterfaceIndex(idx)})
	})
	v.On("delete_loopback", func(m api.Message) ([]api.Message, error) {
		req := m.(*interfaces.DeleteLoopback)
		v.mu.Lock()
		defer v.mu.Unlock()
		i, ok := v.Ifaces[uint32(req.SwIfIndex)]
		if !ok || i.DevType != "Loopback" {
			return reply(&interfaces.DeleteLoopbackReply{Retval: RetvalInvalidSwIfIndex})
		}
		v.dropInterfaceLocked(i)
		return reply(&interfaces.DeleteLoopbackReply{})
	})
	v.On("sw_interface_tag_add_del", func(m api.Message) ([]api.Message, error) {
		req := m.(*interfaces.SwInterfaceTagAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		i, ok := v.Ifaces[uint32(req.SwIfIndex)]
		if !ok {
			return reply(&interfaces.SwInterfaceTagAddDelReply{Retval: RetvalInvalidSwIfIndex})
		}
		if req.IsAdd {
			i.Tag = req.Tag
		} else {
			i.Tag = ""
		}
		return reply(&interfaces.SwInterfaceTagAddDelReply{})
	})
	v.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, idx := range v.indexesLocked() {
			i := v.Ifaces[idx]
			out = append(out, detailsOf(i))
		}
		return out, nil
	})
	v.On("sw_interface_set_table", func(m api.Message) ([]api.Message, error) {
		req := m.(*interfaces.SwInterfaceSetTable)
		v.mu.Lock()
		defer v.mu.Unlock()
		i, ok := v.Ifaces[uint32(req.SwIfIndex)]
		if !ok {
			return reply(&interfaces.SwInterfaceSetTableReply{Retval: RetvalInvalidSwIfIndex})
		}
		if _, ok := v.Tables[tableKey{req.VrfID, req.IsIPv6}]; !ok {
			return reply(&interfaces.SwInterfaceSetTableReply{Retval: RetvalNoSuchFib})
		}
		for a := range i.Addrs {
			if netip.MustParsePrefix(a).Addr().Is6() == req.IsIPv6 {
				return reply(&interfaces.SwInterfaceSetTableReply{Retval: -85}) // ADDRESS_FOUND_FOR_INTERFACE
			}
		}
		if req.IsIPv6 {
			i.Table6 = req.VrfID
		} else {
			i.Table4 = req.VrfID
		}
		return reply(&interfaces.SwInterfaceSetTableReply{})
	})
	v.On("sw_interface_get_table", func(m api.Message) ([]api.Message, error) {
		req := m.(*interfaces.SwInterfaceGetTable)
		v.mu.Lock()
		defer v.mu.Unlock()
		i, ok := v.Ifaces[uint32(req.SwIfIndex)]
		if !ok {
			return reply(&interfaces.SwInterfaceGetTableReply{Retval: RetvalInvalidSwIfIndex})
		}
		t := i.Table4
		if req.IsIPv6 {
			t = i.Table6
		}
		return reply(&interfaces.SwInterfaceGetTableReply{VrfID: t})
	})
	v.On("sw_interface_add_del_address", func(m api.Message) ([]api.Message, error) {
		req := m.(*interfaces.SwInterfaceAddDelAddress)
		v.mu.Lock()
		defer v.mu.Unlock()
		i, ok := v.Ifaces[uint32(req.SwIfIndex)]
		if !ok {
			return reply(&interfaces.SwInterfaceAddDelAddressReply{Retval: RetvalInvalidSwIfIndex})
		}
		p := netip.MustParsePrefix(req.Prefix.String())
		a := p.String()
		if !req.IsAdd {
			if !i.Addrs[a] {
				return reply(&interfaces.SwInterfaceAddDelAddressReply{Retval: RetvalNoSuchEntry})
			}
			delete(i.Addrs, a)
			return reply(&interfaces.SwInterfaceAddDelAddressReply{})
		}
		// VPP refuses an overlapping subnet on another interface in the same table.
		for _, o := range v.Ifaces {
			tbl := o.Table4
			mine := i.Table4
			if p.Addr().Is6() {
				tbl, mine = o.Table6, i.Table6
			}
			if tbl != mine {
				continue
			}
			for oa := range o.Addrs {
				op := netip.MustParsePrefix(oa)
				if op.Masked() == p.Masked() && (o != i || oa == a) {
					return reply(&interfaces.SwInterfaceAddDelAddressReply{Retval: RetvalAddressInUse})
				}
			}
		}
		i.Addrs[a] = true
		return reply(&interfaces.SwInterfaceAddDelAddressReply{})
	})
	v.On("ip_address_dump", func(m api.Message) ([]api.Message, error) {
		req := m.(*ip.IPAddressDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		i, ok := v.Ifaces[uint32(req.SwIfIndex)]
		if !ok {
			return nil, nil
		}
		var addrs []string
		for a := range i.Addrs {
			if netip.MustParsePrefix(a).Addr().Is6() == req.IsIPv6 {
				addrs = append(addrs, a)
			}
		}
		sort.Strings(addrs)
		var out []api.Message
		for _, a := range addrs {
			p, _ := ip_types.ParseAddressWithPrefix(a)
			out = append(out, &ip.IPAddressDetails{SwIfIndex: req.SwIfIndex, Prefix: p})
		}
		return out, nil
	})
	v.On("ip_table_add_del", func(m api.Message) ([]api.Message, error) {
		req := m.(*ip.IPTableAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		k := tableKey{req.Table.TableID, req.Table.IsIP6}
		if k.id == 0 {
			return reply(&ip.IPTableAddDelReply{})
		}
		if req.IsAdd {
			if _, ok := v.Tables[k]; !ok {
				v.Tables[k] = req.Table.Name
			}
		} else {
			delete(v.Tables, k)
			for rk, r := range v.Routes {
				if rk.table == k.id && r.Prefix.Address.Af == afOf(k.v6) {
					delete(v.Routes, rk)
				}
			}
		}
		return reply(&ip.IPTableAddDelReply{})
	})
	v.On("ip_table_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		keys := make([]tableKey, 0, len(v.Tables))
		for k := range v.Tables {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].id != keys[j].id {
				return keys[i].id < keys[j].id
			}
			return !keys[i].v6
		})
		var out []api.Message
		for _, k := range keys {
			out = append(out, &ip.IPTableDetails{Table: ip.IPTable{TableID: k.id, IsIP6: k.v6, Name: v.Tables[k]}})
		}
		return out, nil
	})
	v.On("ip_route_add_del", func(m api.Message) ([]api.Message, error) {
		req := m.(*ip.IPRouteAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		p := netip.MustParsePrefix(req.Route.Prefix.String()).Masked()
		if _, ok := v.Tables[tableKey{req.Route.TableID, p.Addr().Is6()}]; !ok {
			return reply(&ip.IPRouteAddDelReply{Retval: RetvalNoSuchFib})
		}
		rk := routeKey{req.Route.TableID, p.String()}
		if !req.IsAdd {
			if _, ok := v.Routes[rk]; !ok {
				return reply(&ip.IPRouteAddDelReply{Retval: RetvalNoSuchEntry})
			}
			delete(v.Routes, rk)
			v.syncRRLocked()
			return reply(&ip.IPRouteAddDelReply{})
		}
		for _, fp := range req.Route.Paths {
			if fp.SwIfIndex != ^uint32(0) {
				if _, ok := v.Ifaces[fp.SwIfIndex]; !ok {
					return reply(&ip.IPRouteAddDelReply{Retval: RetvalInvalidSwIfIndex})
				}
			}
		}
		r := req.Route
		r.Paths = append(r.Paths[:0:0], req.Route.Paths...)
		v.Routes[rk] = r
		v.syncRRLocked()
		return reply(&ip.IPRouteAddDelReply{})
	})
	v.On("ip_route_v2_dump", func(m api.Message) ([]api.Message, error) {
		req := m.(*ip.IPRouteV2Dump)
		v.mu.Lock()
		defer v.mu.Unlock()
		if _, ok := v.Tables[tableKey{req.Table.TableID, req.Table.IsIP6}]; !ok {
			return nil, nil
		}
		seen := map[routeKey]bool{}
		var keys []routeKey
		for rk, r := range v.Routes {
			if rk.table == req.Table.TableID && r.Prefix.Address.Af == afOf(req.Table.IsIP6) {
				keys, seen[rk] = append(keys, rk), true
			}
		}
		for rk := range v.Internal {
			if rk.table == req.Table.TableID && netip.MustParsePrefix(rk.prefix).Addr().Is6() == req.Table.IsIP6 && !seen[rk] {
				keys = append(keys, rk)
			}
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].prefix < keys[j].prefix })
		var out []api.Message
		for _, rk := range keys {
			if r, ok := v.Routes[rk]; ok {
				out = append(out, &ip.IPRouteV2Details{Route: ip.IPRouteV2{TableID: r.TableID, Prefix: r.Prefix, NPaths: r.NPaths, Paths: r.Paths, Src: 8}})
				continue
			}
			pfx, _ := ip_types.ParsePrefix(rk.prefix)
			out = append(out, &ip.IPRouteV2Details{Route: ip.IPRouteV2{TableID: rk.table, Prefix: pfx, NPaths: 1,
				Paths: []fib_types.FibPath{{SwIfIndex: ^uint32(0), Type: fib_types.FIB_API_PATH_TYPE_DROP}}, Src: v.Internal[rk]}})
		}
		return out, nil
	})
	v.On("ip_route_dump", func(m api.Message) ([]api.Message, error) {
		req := m.(*ip.IPRouteDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		if _, ok := v.Tables[tableKey{req.Table.TableID, req.Table.IsIP6}]; !ok {
			return nil, nil
		}
		var keys []routeKey
		for rk, r := range v.Routes {
			if rk.table == req.Table.TableID && r.Prefix.Address.Af == afOf(req.Table.IsIP6) {
				keys = append(keys, rk)
			}
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].prefix < keys[j].prefix })
		var out []api.Message
		for _, rk := range keys {
			out = append(out, &ip.IPRouteDetails{Route: v.Routes[rk]})
		}
		return out, nil
	})
}

// rrSrc is recursive-resolution (fib_source_t 18).
const rrSrc = 18

// syncRRLocked models VPP's recursive-resolution /32 (/128) entries for the next-hop addresses of
// API routes that have no egress interface.
func (v *VPP) syncRRLocked() {
	for rk, src := range v.Internal {
		if src == rrSrc {
			delete(v.Internal, rk)
		}
	}
	for _, r := range v.Routes {
		for _, fp := range r.Paths {
			if fp.Type != fib_types.FIB_API_PATH_TYPE_NORMAL || fp.SwIfIndex != ^uint32(0) {
				continue
			}
			var a netip.Addr
			if fp.Proto == fib_types.FIB_API_PATH_NH_PROTO_IP6 {
				a = netip.AddrFrom16(fp.Nh.Address.GetIP6())
			} else {
				a = netip.AddrFrom4(fp.Nh.Address.GetIP4())
			}
			v.Internal[routeKey{r.TableID, netip.PrefixFrom(a, a.BitLen()).String()}] = rrSrc
		}
	}
}

// AddInternalRoute adds a VPP-generated FIB entry (src: fib_source_t id, e.g. 4 interface,
// 15 adjacency) that no API client owns.
func (v *VPP) AddInternalRoute(table uint32, prefix string, src uint8) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.Internal[routeKey{table, netip.MustParsePrefix(prefix).Masked().String()}] = src
}

func afOf(v6 bool) ip_types.AddressFamily {
	if v6 {
		return ip_types.ADDRESS_IP6
	}
	return ip_types.ADDRESS_IP4
}

func (v *VPP) indexesLocked() []uint32 {
	out := make([]uint32, 0, len(v.Ifaces))
	for idx := range v.Ifaces {
		out = append(out, idx)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// dropInterfaceLocked removes an interface and the routes through it.
func (v *VPP) dropInterfaceLocked(i *Iface) {
	delete(v.Ifaces, i.Index)
	for rk, r := range v.Routes {
		var keep []int
		for n, fp := range r.Paths {
			if fp.SwIfIndex != i.Index {
				keep = append(keep, n)
			}
		}
		if len(keep) == 0 {
			delete(v.Routes, rk)
		}
	}
}

// AddInterface adds a foreign interface (e.g. another owner's, or a physical NIC) to the model.
func (v *VPP) AddInterface(name, devType, tag string) uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()
	idx := v.next
	v.next++
	v.Ifaces[idx] = &Iface{Index: idx, Name: name, DevType: devType, Tag: tag, Addrs: map[string]bool{}}
	return idx
}

// InterfaceByName returns a copy of the modelled interface named name.
func (v *VPP) InterfaceByName(name string) (Iface, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, i := range v.Ifaces {
		if i.Name == name {
			c := *i
			return c, true
		}
	}
	return Iface{}, false
}

// DeleteInterface removes an interface by name behind the agent's back (simulated loss).
func (v *VPP) DeleteInterface(name string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, i := range v.Ifaces {
		if i.Name == name {
			v.dropInterfaceLocked(i)
			return true
		}
	}
	return false
}

// DeleteTable removes a table (one family) behind the agent's back.
func (v *VPP) DeleteTable(id uint32, v6 bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.Tables, tableKey{id, v6})
}

// HasTable reports whether table id/family exists.
func (v *VPP) HasTable(id uint32, v6 bool) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.Tables[tableKey{id, v6}]
	return ok
}

// RouteCount returns the number of modelled routes.
func (v *VPP) RouteCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.Routes)
}

// HasRoute reports whether table/prefix exists.
func (v *VPP) HasRoute(table uint32, prefix string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.Routes[routeKey{table, prefix}]
	return ok
}

// Snapshot returns a comparable description of everything in the model (for "nothing changed"
// assertions).
func (v *VPP) Snapshot() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	var lines []string
	for _, idx := range v.indexesLocked() {
		i := v.Ifaces[idx]
		var addrs []string
		for a := range i.Addrs {
			addrs = append(addrs, a)
		}
		sort.Strings(addrs)
		lines = append(lines, fmt.Sprintf("if %d %s tag=%q t4=%d t6=%d %v", i.Index, i.Name, i.Tag, i.Table4, i.Table6, addrs))
	}
	for k, n := range v.Tables {
		lines = append(lines, fmt.Sprintf("table %d v6=%v %s", k.id, k.v6, n))
	}
	for k, r := range v.Routes {
		lines = append(lines, fmt.Sprintf("route %d %s %+v", k.table, k.prefix, r.Paths))
	}
	sort.Strings(lines)
	return fmt.Sprint(lines)
}

// SetTableName renames a table (e.g. to model a table id taken over by someone else).
func (v *VPP) SetTableName(id uint32, v6 bool, name string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.Tables[tableKey{id, v6}] = name
}
