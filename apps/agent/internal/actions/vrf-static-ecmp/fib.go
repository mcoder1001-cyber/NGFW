// Package vrfstaticecmp holds the read-only state and the actions of F-vrf-static-ecmp: the paged FIB lister behind the
// ListRoutes RPC (docs/contracts/proto.md §11) and the ping / traceroute actions of the Action RPC. Nothing here mutates
// VPP state.
package vrfstaticecmp

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/ip"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/svs"
	"ngfw/agent/internal/vpp"
)

// Page limits of ListRoutes.
const (
	DefaultLimit = 100
	MaxLimit     = 1000
	// MaxWindow bounds offset + limit: the lister keeps that many entries while it streams the table.
	MaxWindow = 1_000_000
)

// Errors (the RPC maps ErrBadRequest to INVALID_ARGUMENT).
var ErrBadRequest = errors.New("fib: bad request")

// Query is one ListRoutes request with the VRF already resolved to its table.
type Query struct {
	Table  uint32
	Family string // "", "ipv4", "ipv6"
	Prefix string // "" or a CIDR: routes equal to or more specific than it
	Source string // "" or a FIB source name (fib_source_dump)
	Offset uint32
	Limit  uint32
}

// Page is the lister's answer.
type Page struct {
	Routes []*vrxv1.ListRoutesEntry
	Total  uint32
}

// entry is one kept route before it is converted.
type entry struct {
	prefix netip.Prefix
	route  ip.IPRouteV2
}

// less orders routes by family (IPv4 first), address, prefix length.
func less(a, b netip.Prefix) bool {
	if a.Addr().Is4() != b.Addr().Is4() {
		return a.Addr().Is4()
	}
	if c := a.Addr().Compare(b.Addr()); c != 0 {
		return c < 0
	}
	return a.Bits() < b.Bits()
}

// window is a max-heap of the smallest n entries seen so far (the page is its tail after the offset).
type window []entry

func (w window) Len() int           { return len(w) }
func (w window) Less(i, j int) bool { return less(w[j].prefix, w[i].prefix) } // max-heap
func (w window) Swap(i, j int)      { w[i], w[j] = w[j], w[i] }
func (w *window) Push(x any)        { *w = append(*w, x.(entry)) }
func (w *window) Pop() any {
	old := *w
	n := len(old)
	x := old[n-1]
	*w = old[:n-1]
	return x
}

// ListRoutes reads the FIB of q.Table once per family as a stream (ip_route_v2_dump, the source filter pushed down to
// VPP), keeps the entries that pass the filters and only the smallest offset+limit of them, and returns the page plus the
// number of matches. Memory and the answer are bounded by the window, not by the table.
func ListRoutes(ctx context.Context, c vpp.Client, owner string, q Query) (*Page, error) {
	if q.Limit == 0 {
		q.Limit = DefaultLimit
	}
	if q.Limit > MaxLimit {
		return nil, fmt.Errorf("%w: limit %d > %d", ErrBadRequest, q.Limit, MaxLimit)
	}
	if uint64(q.Offset)+uint64(q.Limit) > MaxWindow {
		return nil, fmt.Errorf("%w: offset + limit %d > %d (narrow the filter)", ErrBadRequest, uint64(q.Offset)+uint64(q.Limit), MaxWindow)
	}
	fams := []bool{false, true}
	switch q.Family {
	case "":
	case "ipv4":
		fams = []bool{false}
	case "ipv6":
		fams = []bool{true}
	default:
		return nil, fmt.Errorf("%w: family %q (ipv4, ipv6 or empty)", ErrBadRequest, q.Family)
	}
	var within netip.Prefix
	if q.Prefix != "" {
		p, err := netip.ParsePrefix(strings.TrimSpace(q.Prefix))
		if err != nil {
			return nil, fmt.Errorf("%w: prefix %q: %v", ErrBadRequest, q.Prefix, err)
		}
		within = netip.PrefixFrom(p.Addr().Unmap().WithZone(""), p.Bits()).Masked()
		fams = []bool{within.Addr().Is6()}
		if q.Family != "" && (q.Family == "ipv6") != within.Addr().Is6() {
			return nil, fmt.Errorf("%w: prefix %s is not %s", ErrBadRequest, within, q.Family)
		}
	}
	names, err := svs.Sources(ctx, c)
	if err != nil {
		return nil, err
	}
	var src uint8
	if q.Source != "" {
		found := false
		for id, n := range names {
			if n == q.Source && id != 0 {
				src, found = id, true
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: unknown FIB source %q", ErrBadRequest, q.Source)
		}
	}
	keep := int(q.Offset + q.Limit)
	w := &window{}
	var total uint32
	for _, v6 := range fams {
		stream, err := ip.NewServiceClient(c).IPRouteV2Dump(ctx, &ip.IPRouteV2Dump{Src: src, Table: ip.IPTable{TableID: q.Table, IsIP6: v6}})
		if err != nil {
			return nil, fmt.Errorf("ip_route_v2_dump %d: %w", q.Table, err)
		}
		for {
			det, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("ip_route_v2_dump %d: %w", q.Table, err)
			}
			p, err := netip.ParsePrefix(det.Route.Prefix.String())
			if err != nil {
				continue
			}
			p = p.Masked()
			if within.IsValid() && !(within.Contains(p.Addr()) && p.Bits() >= within.Bits()) {
				continue
			}
			total++
			if w.Len() < keep {
				heap.Push(w, entry{p, det.Route})
			} else if less(p, (*w)[0].prefix) {
				(*w)[0] = entry{p, det.Route}
				heap.Fix(w, 0)
			}
		}
	}
	// the window holds the smallest `keep` entries: sorted ascending, the page is after the offset
	sorted := make([]entry, w.Len())
	for i := len(sorted) - 1; i >= 0; i-- {
		sorted[i] = heap.Pop(w).(entry)
	}
	page := &Page{Total: total}
	if int(q.Offset) >= len(sorted) {
		return page, nil
	}
	sorted = sorted[q.Offset:]
	var ifs *iface.Table
	for _, e := range sorted {
		for _, fp := range e.route.Paths {
			if fp.SwIfIndex != ^uint32(0) && ifs == nil {
				if ifs, err = iface.Dump(ctx, c, owner); err != nil {
					return nil, err
				}
			}
		}
		page.Routes = append(page.Routes, convert(e, names, ifs))
	}
	return page, nil
}

var pathTypes = map[fib_types.FibPathType]string{
	fib_types.FIB_API_PATH_TYPE_NORMAL:        "normal",
	fib_types.FIB_API_PATH_TYPE_LOCAL:         "local",
	fib_types.FIB_API_PATH_TYPE_DROP:          "drop",
	fib_types.FIB_API_PATH_TYPE_UDP_ENCAP:     "udp-encap",
	fib_types.FIB_API_PATH_TYPE_BIER_IMP:      "bier-imp",
	fib_types.FIB_API_PATH_TYPE_ICMP_UNREACH:  "icmp-unreach",
	fib_types.FIB_API_PATH_TYPE_ICMP_PROHIBIT: "icmp-prohibit",
	fib_types.FIB_API_PATH_TYPE_SOURCE_LOOKUP: "source-lookup",
	fib_types.FIB_API_PATH_TYPE_DVR:           "dvr",
	fib_types.FIB_API_PATH_TYPE_INTERFACE_RX:  "interface-rx",
	fib_types.FIB_API_PATH_TYPE_CLASSIFY:      "classify",
}

func convert(e entry, names map[uint8]string, ifs *iface.Table) *vrxv1.ListRoutesEntry {
	out := &vrxv1.ListRoutesEntry{Prefix: e.prefix.String(), Source: names[e.route.Src], StatsIndex: e.route.StatsIndex}
	if out.Source == "" {
		out.Source = fmt.Sprintf("source-%d", e.route.Src)
	}
	for _, fp := range e.route.Paths {
		p := &vrxv1.ListRoutesPath{Type: pathTypes[fp.Type], TableId: fp.TableID, Weight: uint32(fp.Weight), Preference: uint32(fp.Preference)}
		if p.Type == "" {
			p.Type = fmt.Sprintf("type-%d", fp.Type)
		}
		var a netip.Addr
		switch fp.Proto {
		case fib_types.FIB_API_PATH_NH_PROTO_IP4:
			a = netip.AddrFrom4(fp.Nh.Address.GetIP4())
		case fib_types.FIB_API_PATH_NH_PROTO_IP6:
			a = netip.AddrFrom16(fp.Nh.Address.GetIP6())
		}
		if a.IsValid() && !a.IsUnspecified() {
			p.NextHop = a.String()
		}
		if fp.SwIfIndex != ^uint32(0) && ifs != nil {
			if n, ok := ifs.Logical(fp.SwIfIndex); ok {
				p.Interface = n
			} else {
				p.Interface = ifs.VPPName(fp.SwIfIndex)
			}
		}
		if fp.Flags&fib_types.FIB_API_PATH_FLAG_RESOLVE_VIA_HOST != 0 {
			p.Flags = append(p.Flags, "resolve-via-host")
		}
		if fp.Flags&fib_types.FIB_API_PATH_FLAG_RESOLVE_VIA_ATTACHED != 0 {
			p.Flags = append(p.Flags, "resolve-via-attached")
		}
		if fp.Flags&fib_types.FIB_API_PATH_FLAG_POP_PW_CW != 0 {
			p.Flags = append(p.Flags, "pop-pw-cw")
		}
		out.Paths = append(out.Paths, p)
	}
	return out
}
