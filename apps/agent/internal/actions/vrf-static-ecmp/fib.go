// Package vrfstaticecmp holds the read-only state and the actions of F-vrf-static-ecmp: the paged FIB lister behind the
// ListRoutes RPC (docs/contracts/proto.md §11) and the ping / traceroute actions of the Action RPC. Nothing here mutates
// VPP state.
package vrfstaticecmp

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"go.fd.io/govpp/core"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/memclnt"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/svs"
	"ngfw/agent/internal/vpp"
)

// Page limits of ListRoutes.
const (
	DefaultLimit = 100
	MaxLimit     = 1000
	// MaxWindow bounds offset + limit: the lister keeps that many (slim) entries while it streams the table — about
	// 110 B each with one path, so ≤ ~12 MB per walk. Deeper pages need a narrower filter (prefix, family, source) until
	// a keyset cursor exists (docs/tech-debt.md, V-new (d)).
	MaxWindow = 100_000
	// WalkWait is how long a ListRoutes waits for the FIB walk in progress before it answers ErrFIBBusy.
	WalkWait = 3 * time.Second
	// walkTimeout bounds one walk (both families) once it has started; the walk is not cut short by the caller's
	// cancellation (see ListRoutes).
	walkTimeout = 2 * time.Minute
)

// Errors of the lister: ErrBadRequest → INVALID_ARGUMENT, ErrFIBBusy → UNAVAILABLE.
var (
	ErrBadRequest = errors.New("fib: bad request")
	ErrFIBBusy    = errors.New("a FIB read is in progress (the agent runs one ip_route_v2_dump walk at a time); retry shortly")
)

// walkSem admits one FIB walk per agent process. VPP's ip_route_v2_dump handler is not mp-safe: the binary-API
// dispatcher holds the worker barrier (every worker stopped; on a main-core-only VPP no input is polled either) while it
// walks and sends the whole table (docs/vpp-code-track.md V-new (d)). Concurrent readers would only queue inside VPP and
// stretch that stall, so the agent serialises them and turns a long queue into UNAVAILABLE.
var walkSem = make(chan struct{}, 1)

// acquireWalk takes walkSem, waiting at most WalkWait (or until ctx ends).
func acquireWalk(ctx context.Context) error {
	t := time.NewTimer(WalkWait)
	defer t.Stop()
	select {
	case walkSem <- struct{}{}:
		return nil
	case <-t.C:
		return ErrFIBBusy
	case <-ctx.Done():
		return fmt.Errorf("%w (%v)", ErrFIBBusy, ctx.Err())
	}
}

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

// slimPath is what a page needs of one fib_types.FibPath (~48 B instead of the decoded 252 B).
type slimPath struct {
	nh        netip.Addr // invalid when the path has no (or an all-zero) next-hop address
	swIfIndex uint32
	tableID   uint32
	typ       fib_types.FibPathType
	flags     fib_types.FibPathFlags
	weight    uint8
	pref      uint8
}

// entry is one kept route before it is converted: the window holds these, never the decoded binapi route (review M2).
type entry struct {
	prefix     netip.Prefix
	paths      []slimPath
	statsIndex uint32
	src        uint8
}

// slim copies what convert needs out of a decoded route.
func slim(p netip.Prefix, r *ip.IPRouteV2) entry {
	e := entry{prefix: p, statsIndex: r.StatsIndex, src: r.Src, paths: make([]slimPath, len(r.Paths))}
	for i := range r.Paths {
		fp := &r.Paths[i]
		sp := slimPath{swIfIndex: fp.SwIfIndex, tableID: fp.TableID, typ: fp.Type, flags: fp.Flags, weight: fp.Weight, pref: fp.Preference}
		var a netip.Addr
		switch fp.Proto {
		case fib_types.FIB_API_PATH_NH_PROTO_IP4:
			a = netip.AddrFrom4(fp.Nh.Address.GetIP4())
		case fib_types.FIB_API_PATH_NH_PROTO_IP6:
			a = netip.AddrFrom16(fp.Nh.Address.GetIP6())
		}
		if a.IsValid() && !a.IsUnspecified() {
			sp.nh = a
		}
		e.paths[i] = sp
	}
	return e
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
// number of matches. Memory and the answer are bounded by the window (≤ MaxWindow slim entries), not by the table; VPP's
// cost is not: every call is one full walk of the table under VPP's worker barrier (V-new (d)). Walks are serialised per
// agent (walkSem): a caller that cannot start within WalkWait gets ErrFIBBusy. A started walk is read to its end even if
// ctx is cancelled — VPP finishes the walk anyway, and releasing the slot early would let the next walk queue behind it.
func ListRoutes(ctx context.Context, c vpp.Client, owner string, q Query) (*Page, error) {
	if q.Limit == 0 {
		q.Limit = DefaultLimit
	}
	if q.Limit > MaxLimit {
		return nil, fmt.Errorf("%w: limit %d > %d", ErrBadRequest, q.Limit, MaxLimit)
	}
	if uint64(q.Offset)+uint64(q.Limit) > MaxWindow {
		return nil, fmt.Errorf("%w: offset + limit %d > %d: narrow the listing with prefix, family or source", ErrBadRequest, uint64(q.Offset)+uint64(q.Limit), MaxWindow)
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
	if err := acquireWalk(ctx); err != nil {
		return nil, err
	}
	err = func() error {
		defer func() { <-walkSem }()
		wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), walkTimeout)
		defer cancel()
		for _, v6 := range fams {
			err := DumpRoutes(wctx, c, &ip.IPRouteV2Dump{Src: src, Table: ip.IPTable{TableID: q.Table, IsIP6: v6}}, func(det *ip.IPRouteV2Details) {
				p, err := netip.ParsePrefix(det.Route.Prefix.String())
				if err != nil {
					return
				}
				p = p.Masked()
				if within.IsValid() && (!within.Contains(p.Addr()) || p.Bits() < within.Bits()) {
					return
				}
				total++
				if w.Len() < keep {
					heap.Push(w, slim(p, &det.Route))
				} else if less(p, (*w)[0].prefix) {
					(*w)[0] = slim(p, &det.Route)
					heap.Fix(w, 0)
				}
			})
			if err != nil {
				return fmt.Errorf("ip_route_v2_dump %d: %w", q.Table, err)
			}
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
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
		for _, sp := range e.paths {
			if sp.swIfIndex != ^uint32(0) && ifs == nil {
				if ifs, err = iface.Dump(ctx, c, owner); err != nil {
					return nil, err
				}
			}
		}
		page.Routes = append(page.Routes, convert(e, names, ifs))
	}
	return page, nil
}

// dumpReplyBuffer is the reply buffer of the lister's dump stream. govpp drops a reply its consumer does not take within
// 100 ms once the stream's buffer (default 100) is full — on a loaded host a 100k-entry dump lost ~0.01 % of its entries
// that way (host run 2026-09-24) — so the lister's stream buffers up to this many replies instead.
const dumpReplyBuffer = 1 << 16

// DumpRoutes runs one ip_route_v2_dump (the generated message types, a stream with a large reply buffer) and calls fn for
// every entry until VPP's control-ping reply ends the dump.
func DumpRoutes(ctx context.Context, c vpp.Client, req *ip.IPRouteV2Dump, fn func(*ip.IPRouteV2Details)) error {
	stream, err := c.NewStream(ctx, core.WithReplySize(dumpReplyBuffer))
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	if err := stream.SendMsg(req); err != nil {
		return err
	}
	if err := stream.SendMsg(&memclnt.ControlPing{}); err != nil {
		return err
	}
	for {
		msg, err := stream.RecvMsg()
		if err != nil {
			return err
		}
		switch m := msg.(type) {
		case *ip.IPRouteV2Details:
			fn(m)
		case *memclnt.ControlPingReply:
			return nil
		default:
			return fmt.Errorf("unexpected reply %T", msg)
		}
	}
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
	out := &vrxv1.ListRoutesEntry{Prefix: e.prefix.String(), Source: names[e.src], StatsIndex: e.statsIndex}
	if out.Source == "" {
		out.Source = fmt.Sprintf("source-%d", e.src)
	}
	for _, sp := range e.paths {
		p := &vrxv1.ListRoutesPath{Type: pathTypes[sp.typ], TableId: sp.tableID, Weight: uint32(sp.weight), Preference: uint32(sp.pref)}
		if p.Type == "" {
			p.Type = fmt.Sprintf("type-%d", sp.typ)
		}
		if sp.nh.IsValid() {
			p.NextHop = sp.nh.String()
		}
		if sp.swIfIndex != ^uint32(0) && ifs != nil {
			if n, ok := ifs.Logical(sp.swIfIndex); ok {
				p.Interface = n
			} else {
				p.Interface = ifs.VPPName(sp.swIfIndex)
			}
		}
		if sp.flags&fib_types.FIB_API_PATH_FLAG_RESOLVE_VIA_HOST != 0 {
			p.Flags = append(p.Flags, "resolve-via-host")
		}
		if sp.flags&fib_types.FIB_API_PATH_FLAG_RESOLVE_VIA_ATTACHED != 0 {
			p.Flags = append(p.Flags, "resolve-via-attached")
		}
		if sp.flags&fib_types.FIB_API_PATH_FLAG_POP_PW_CW != 0 {
			p.Flags = append(p.Flags, "pop-pw-cw")
		}
		out.Paths = append(out.Paths, p)
	}
	return out
}
