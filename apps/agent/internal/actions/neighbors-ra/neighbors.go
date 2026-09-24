// Package neighborsra is the imperative side of F-neighbors-ra (WBS D2.3, D2.6): the live ARP/ND table lister behind
// the ListNeighbors RPC, the ARP flush action and the helpers of the neighbour-event watcher. Everything here reads or
// acts on VPP through generated bindings only (apps/agent/binapi/ip_neighbor, binapi/interface).
//
// Shared-VPP safety (TASK ENVELOPE, docs/contracts/proto.md §11): ip_neighbor_dump, ip_neighbor_flush and
// want_ip_neighbor_events_v2 default to sw_if_index ~0 — every interface of every slot — and VPP's watcher treats 0 as
// "all" too. Nothing here ever sends ~0 or 0: every call names one interface this owner can name (its own tag or
// untagged, iface.Table.Logical), and events of other interfaces are dropped.
package neighborsra

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"go.fd.io/govpp/api"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_neighbor"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/df2"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp"
)

// ErrInvalid wraps request validation failures (gRPC INVALID_ARGUMENT).
var ErrInvalid = errors.New("invalid request")

// Family selects the address families of a listing or flush.
type Family int

// Families.
const (
	Both Family = iota
	IPv4
	IPv6
)

// ParseFamily parses "", "ipv4" or "ipv6".
func ParseFamily(s string) (Family, error) {
	switch s {
	case "":
		return Both, nil
	case "ipv4":
		return IPv4, nil
	case "ipv6":
		return IPv6, nil
	}
	return Both, fmt.Errorf("%w: family %q is not ipv4 or ipv6", ErrInvalid, s)
}

// afs returns the VPP address families of f, IPv4 first.
func (f Family) afs() []ip_types.AddressFamily {
	switch f {
	case IPv4:
		return []ip_types.AddressFamily{ip_types.ADDRESS_IP4}
	case IPv6:
		return []ip_types.AddressFamily{ip_types.ADDRESS_IP6}
	}
	return []ip_types.AddressFamily{ip_types.ADDRESS_IP4, ip_types.ADDRESS_IP6}
}

func familyName(af ip_types.AddressFamily) string {
	if af == ip_types.ADDRESS_IP6 {
		return "ipv6"
	}
	return "ipv4"
}

// Iface is one interface this owner can name, with its FIB tables.
type Iface struct {
	// Name is the logical name (D-069).
	Name string
	// Index is VPP's sw_if_index (never 0: local0 is not nameable).
	Index uint32
	// Table4 and Table6 are the interface's IPv4 and IPv6 FIB table ids.
	Table4, Table6 uint32
}

func (i Iface) table(af ip_types.AddressFamily) uint32 {
	if af == ip_types.ADDRESS_IP6 {
		return i.Table6
	}
	return i.Table4
}

// Nameable returns every interface owner can name (own tag or untagged, never another owner's, never local0), sorted by
// name, with its FIB tables (sw_interface_get_table per family).
func Nameable(ctx context.Context, c vpp.Client, owner string) ([]Iface, error) {
	t, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return nil, err
	}
	svc := ifapi.NewServiceClient(c)
	seen := map[string]bool{}
	var out []Iface
	for _, idx := range t.Indexes() {
		name, ok := t.Logical(idx)
		if !ok || idx == 0 || idx == df2.NoInterface || seen[name] {
			continue // another owner's, local0, or an untagged twin of our own name (ours wins, alias rule)
		}
		seen[name] = true
		it := Iface{Name: name, Index: idx}
		for _, v6 := range []bool{false, true} {
			r, err := svc.SwInterfaceGetTable(ctx, &ifapi.SwInterfaceGetTable{SwIfIndex: interface_types.InterfaceIndex(idx), IsIPv6: v6})
			if err != nil {
				return nil, fmt.Errorf("sw_interface_get_table %s: %w", name, err)
			}
			if v6 {
				it.Table6 = r.VrfID
			} else {
				it.Table4 = r.VrfID
			}
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Entry is one ARP/ND entry.
type Entry struct {
	Interface  string
	Index      uint32
	IP         netip.Addr
	MAC        string
	Family     string // "ipv4" | "ipv6"
	Static     bool
	NoFibEntry bool
	Age        float64
	TableID    uint32
	VRF        string
	raw        ip_neighbor.IPNeighbor
}

// State is "static" or "dynamic".
func (e Entry) State() string {
	if e.Static {
		return "static"
	}
	return "dynamic"
}

// dump dumps one interface and address family (never ~0, never 0).
func dump(ctx context.Context, c vpp.Client, it Iface, af ip_types.AddressFamily) ([]Entry, error) {
	if it.Index == 0 || it.Index == df2.NoInterface {
		return nil, fmt.Errorf("neighbors: refusing ip_neighbor_dump with sw_if_index %d (all interfaces)", it.Index)
	}
	stream, err := ip_neighbor.NewServiceClient(c).IPNeighborDump(ctx, &ip_neighbor.IPNeighborDump{SwIfIndex: interface_types.InterfaceIndex(it.Index), Af: af})
	if err != nil {
		return nil, fmt.Errorf("ip_neighbor_dump %s: %w", it.Name, err)
	}
	details, err := df2.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("ip_neighbor_dump %s: %w", it.Name, err)
	}
	out := make([]Entry, 0, len(details))
	for _, d := range details {
		nb := d.Neighbor
		if uint32(nb.SwIfIndex) != it.Index {
			continue // VPP walks exactly this interface; never report anything else
		}
		e := Entry{
			Interface:  it.Name,
			Index:      it.Index,
			IP:         df2.FromAddress(nb.IPAddress),
			MAC:        df2.MACString(nb.MacAddress),
			Family:     familyName(af),
			Static:     nb.Flags&ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC != 0,
			NoFibEntry: nb.Flags&ip_neighbor.IP_API_NEIGHBOR_FLAG_NO_FIB_ENTRY != 0,
			Age:        d.Age,
			TableID:    it.table(af),
			raw:        nb,
		}
		if e.Static {
			e.Age = 0 // VPP reports an age for static entries too; it means nothing there (proto: 0 for static entries)
		}
		out = append(out, e)
	}
	return out, nil
}

// Query filters, sorts and pages a listing (ListNeighborsRequest).
type Query struct {
	VRF, Interface, Family, State, Search, Sort string
	Descending                                  bool
	Offset, Limit                               uint32
}

// Default and maximum page size.
const (
	DefaultLimit = 100
	MaxLimit     = 1000
)

var sortKeys = map[string]bool{"": true, "interface": true, "ip": true, "mac": true, "age": true, "vrf": true, "state": true}

// Page is one page of a listing.
type Page struct {
	Entries []Entry
	Total   int
}

// List dumps the neighbour table of every nameable interface (or the one named), filters, sorts and pages it.
// vrfName names a FIB table id ("default" for 0).
func List(ctx context.Context, c vpp.Client, owner string, q Query, vrfName func(uint32) string) (Page, error) {
	fam, err := ParseFamily(q.Family)
	if err != nil {
		return Page{}, err
	}
	if q.State != "" && q.State != "static" && q.State != "dynamic" {
		return Page{}, fmt.Errorf("%w: state %q is not static or dynamic", ErrInvalid, q.State)
	}
	if !sortKeys[q.Sort] {
		return Page{}, fmt.Errorf("%w: sort %q is not interface, ip, mac, age, vrf or state", ErrInvalid, q.Sort)
	}
	limit := q.Limit
	switch {
	case limit == 0:
		limit = DefaultLimit
	case limit > MaxLimit:
		return Page{}, fmt.Errorf("%w: limit %d exceeds %d", ErrInvalid, limit, MaxLimit)
	}
	ifs, err := Nameable(ctx, c, owner)
	if err != nil {
		return Page{}, err
	}
	if q.Interface != "" {
		var one []Iface
		for _, it := range ifs {
			if it.Name == q.Interface {
				one = append(one, it)
			}
		}
		ifs = one
	}
	search := strings.ToLower(q.Search)
	var all []Entry
	for _, it := range ifs {
		for _, af := range fam.afs() {
			name := vrfName(it.table(af))
			if q.VRF != "" && name != q.VRF {
				continue
			}
			es, err := dump(ctx, c, it, af)
			if err != nil {
				return Page{}, err
			}
			for _, e := range es {
				e.VRF = name
				if q.State != "" && e.State() != q.State {
					continue
				}
				if search != "" && !strings.Contains(e.IP.String(), search) && !strings.Contains(e.MAC, search) &&
					!strings.Contains(strings.ToLower(e.Interface), search) {
					continue
				}
				all = append(all, e)
			}
		}
	}
	sortEntries(all, q.Sort, q.Descending)
	page := Page{Total: len(all)}
	if int(q.Offset) < len(all) {
		end := min(int(q.Offset)+int(limit), len(all))
		page.Entries = all[q.Offset:end]
	}
	return page, nil
}

// less orders by interface, then IPv4 before IPv6, then address.
func baseLess(a, b Entry) bool {
	if a.Interface != b.Interface {
		return a.Interface < b.Interface
	}
	return a.IP.Less(b.IP)
}

func sortEntries(es []Entry, key string, desc bool) {
	cmp := func(a, b Entry) int {
		switch key {
		case "ip":
			return a.IP.Compare(b.IP)
		case "mac":
			return strings.Compare(a.MAC, b.MAC)
		case "age":
			switch {
			case a.Age < b.Age:
				return -1
			case a.Age > b.Age:
				return 1
			}
		case "vrf":
			return strings.Compare(a.VRF, b.VRF)
		case "state":
			return strings.Compare(a.State(), b.State())
		}
		return strings.Compare(a.Interface, b.Interface)
	}
	sort.SliceStable(es, func(i, j int) bool {
		c := cmp(es[i], es[j])
		if desc {
			c = -c
		}
		if c != 0 {
			return c < 0
		}
		return baseLess(es[i], es[j])
	})
}

// FlushResult is the outcome of Flush.
type FlushResult struct {
	Deleted    int
	Interfaces int
}

// Flush deletes the learned (non-static) ARP/ND entries of the named interfaces, entry by entry
// (ip_neighbor_add_del is_add=0). It never uses ip_neighbor_flush: VPP's ip_neighbor_del_all removes static entries
// too, and those belong to the configuration (Retrieve would report them missing). Every name must be one this owner
// can name (iface.Table.IndexByName: a foreign tag fails with ErrInvalid). line receives one progress line per interface
// and family.
func Flush(ctx context.Context, c vpp.Client, owner string, names []string, fam Family, line func(string)) (FlushResult, error) {
	if len(names) == 0 {
		return FlushResult{}, nil
	}
	t, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return FlushResult{}, err
	}
	var targets []Iface
	for _, n := range names {
		idx, err := t.IndexByName(n)
		if err != nil {
			return FlushResult{}, fmt.Errorf("%w: interface %q: %v", ErrInvalid, n, err)
		}
		if idx == 0 || idx == df2.NoInterface {
			return FlushResult{}, fmt.Errorf("%w: interface %q", ErrInvalid, n)
		}
		targets = append(targets, Iface{Name: n, Index: idx})
	}
	svc := ip_neighbor.NewServiceClient(c)
	var res FlushResult
	for _, it := range targets {
		res.Interfaces++
		for _, af := range fam.afs() {
			es, err := dump(ctx, c, it, af)
			if err != nil {
				return res, err
			}
			n := 0
			for _, e := range es {
				if e.Static {
					continue
				}
				_, err := svc.IPNeighborAddDel(ctx, &ip_neighbor.IPNeighborAddDel{IsAdd: false, Neighbor: e.raw})
				if err != nil && !isNoSuchEntry(err) {
					return res, fmt.Errorf("ip_neighbor_add_del (delete %s on %s): %w", e.IP, it.Name, err)
				}
				if err == nil {
					n++
				}
			}
			res.Deleted += n
			if line != nil {
				line(fmt.Sprintf("%s %s: deleted %d learned entries", it.Name, familyName(af), n))
			}
		}
	}
	return res, nil
}

// isNoSuchEntry: the entry went away between the dump and the delete (aged out meanwhile).
func isNoSuchEntry(err error) bool {
	var e api.VPPApiError
	return errors.As(err, &e) && e == api.NO_SUCH_ENTRY
}
