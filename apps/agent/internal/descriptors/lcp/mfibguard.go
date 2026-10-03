package lcp

// S-ospf-mfib-stale (F-ospf-host Q4): linux-cp adds an Accept path for the phy to the IPv4 link-local
// multicast specials ((*,224.0.0.0/24) …, mfib source plugin-low) when the host interface gets its
// first IPv4 address (lcp_router_link_addr_add_del), and removes it only when it hears the last IPv4
// address go while the pair still exists. Its pair-delete callback removes the IPv6 paths only
// (lcp_router.c lcp_lcp_router_interface_del_cb), so deleting a pair whose host interface still has
// an address leaves the Accept path on the phy's sw_if_index — inherited by whatever interface
// reuses that index. ip_mroute_add_del cannot remove it: it works on mfib source API only
// (ip_api.c), and the path belongs to source plugin-low.
//
// The guard, configuration-only (no VPP code): before the pair is deleted, flush the host
// interface's IPv4 addresses in the agent's own network namespace (a pair in the VPP/default
// namespace — the only one linux_nl listens to, F-ospf-host Q6), wait until linux-cp has removed
// the Accept path, then delete the pair. What is still left afterwards is logged; it is recorded in
// docs/vpp-code-track.md as the stale linux-cp Accept (the lcp_router.c row, no V-number).

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"slices"
	"strings"
	"syscall"
	"time"

	"go.fd.io/govpp/api"
	"golang.org/x/sys/unix"

	"ngfw/agent/binapi/fib_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/lcp"
	"ngfw/agent/binapi/mfib_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/vpp"
)

// HostAddrFlusher removes every IPv4 address of a Linux interface in the agent's network namespace
// and returns how many it removed (0 and no error when the interface does not exist).
// The interface is matched by the pair's Linux ifindex (vif_index) and must still carry the pair's
// host name, so a renamed or reused name is never flushed.
type HostAddrFlusher interface {
	FlushIPv4(ifindex int, name string) (int, error)
}

// WithHostAddrFlusher replaces the netlink flusher (tests).
func WithHostAddrFlusher(f HostAddrFlusher) Option { return func(o *options) { o.flusher = f } }

// WithMfibWait sets how long Delete waits for linux-cp to drop the Accept path (default 2s).
func WithMfibWait(d time.Duration) Option { return func(o *options) { o.mfibWait = d } }

// acceptPrefix is the IPv4 link-local multicast special linux-cp installs (lcp_router.c ip4_specials).
var acceptPrefix = [4]byte{224, 0, 0, 0}

// StaleAccept reports whether table 0's (*,224.0.0.0/24) has an Accept path on swIfIndex.
func StaleAccept(ctx context.Context, c vpp.Client, swIfIndex uint32) (bool, error) {
	return acceptIn(ctx, c, 0, swIfIndex)
}

// acceptIn reports whether table's (*,224.0.0.0/24) has an Accept path on swIfIndex.
func acceptIn(ctx context.Context, c vpp.Client, table, swIfIndex uint32) (bool, error) {
	paths, _, err := mroute224(ctx, c, table)
	if err != nil {
		return false, err
	}
	for _, path := range paths {
		if (swIfIndex == ^uint32(0) || uint32(path.Path.SwIfIndex) == swIfIndex) && path.ItfFlags&mfib_types.MFIB_API_ITF_FLAG_ACCEPT != 0 {
			return true, nil
		}
	}
	return false, nil
}

// mroute224 returns the paths ip_mroute_dump reports for table's (*,224.0.0.0/24) and whether the
// entry exists. VPP encodes the paths of the entry's best source only (mfib_entry_encode): the API
// source's while it has any (API beats plugin-low), else linux-cp's plugin-low paths.
func mroute224(ctx context.Context, c vpp.Client, table uint32) ([]mfib_types.MfibPath, bool, error) {
	st, err := ip.NewServiceClient(c).IPMrouteDump(ctx, &ip.IPMrouteDump{Table: ip.IPTable{TableID: table}})
	if err != nil {
		return nil, false, err
	}
	var paths []mfib_types.MfibPath
	found := false
	for {
		d, err := st.Recv()
		if errors.Is(err, io.EOF) {
			return paths, found, nil
		}
		if err != nil {
			return nil, false, err
		}
		p := d.Route.Prefix
		if p.Af != ip_types.ADDRESS_IP4 || p.GrpAddressLength != 24 || [4]byte(p.GrpAddress.GetIP4()) != acceptPrefix {
			continue
		}
		found = true
		paths = append(paths, d.Route.Paths...)
	}
}

// dropAccept runs before the pair delete of a default-namespace pair.
func (d *ItfPairDescriptor) dropAccept(ctx context.Context, phy uint32, vif int, hostIf string) {
	log := slog.Default().With("descriptor", NameItfPair, "sw_if_index", phy, "host_if", hostIf)
	stale, err := StaleAccept(ctx, d.client, phy)
	if err != nil || !stale {
		if err != nil {
			log.Warn("lcp: ip_mroute_dump failed; mfib Accept guard skipped", "err", err)
		}
		return
	}
	n, err := d.o.flusher.FlushIPv4(vif, hostIf)
	if err != nil {
		log.Warn("lcp: flushing host IPv4 addresses failed; linux-cp may leave (*,224.0.0.0/24) Accept", "err", err)
		return
	}
	deadline := time.Now().Add(d.o.mfibWait)
	for {
		stale, err = StaleAccept(ctx, d.client, phy)
		if err == nil && !stale {
			log.Info("lcp: host IPv4 addresses flushed, linux-cp removed (*,224.0.0.0/24) Accept", "flushed", n)
			return
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			log.Warn("lcp: (*,224.0.0.0/24) Accept still on the phy before pair delete (stale linux-cp Accept, docs/vpp-code-track.md)", "flushed", n, "err", err)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// netlinkFlusher is the HostAddrFlusher of the agent's own network namespace (RTM_DELADDR).
type netlinkFlusher struct{}

func (netlinkFlusher) FlushIPv4(ifindex int, name string) (int, error) {
	ifc, err := net.InterfaceByIndex(ifindex)
	if err != nil || ifc.Name != name {
		return 0, nil // gone, or the index now belongs to another interface: nothing of ours to flush
	}
	addrs, err := ifc.Addrs()
	if err != nil {
		return 0, fmt.Errorf("addresses of %s: %w", name, err)
	}
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return 0, fmt.Errorf("netlink socket: %w", err)
	}
	defer unix.Close(fd)
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return 0, fmt.Errorf("netlink bind: %w", err)
	}
	n := 0
	for i, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.To4() == nil {
			continue
		}
		plen, _ := ipn.Mask.Size()
		if err := addrReq(fd, unix.RTM_DELADDR, 0, uint32(i+1), ifc.Index, ipn.IP.To4(), plen); err != nil {
			return n, fmt.Errorf("RTM_DELADDR %s on %s: %w", ipn, name, err)
		}
		n++
	}
	return n, nil
}

func addrReq(fd int, typ, flags uint16, seq uint32, ifindex int, ip4 net.IP, plen int) error {
	const hdr, ifa, rta = unix.SizeofNlMsghdr, unix.SizeofIfAddrmsg, 8 // rtattr 4 + IPv4 4
	b := make([]byte, hdr+ifa+rta)
	ne := binary.NativeEndian
	ne.PutUint32(b[0:], uint32(len(b)))
	ne.PutUint16(b[4:], typ)
	ne.PutUint16(b[6:], unix.NLM_F_REQUEST|unix.NLM_F_ACK|flags)
	ne.PutUint32(b[8:], seq)
	b[hdr] = unix.AF_INET
	b[hdr+1] = byte(plen)
	ne.PutUint32(b[hdr+4:], uint32(ifindex))
	ne.PutUint16(b[hdr+ifa:], 8)
	ne.PutUint16(b[hdr+ifa+2:], unix.IFA_LOCAL)
	copy(b[hdr+ifa+4:], ip4)
	if err := unix.Sendto(fd, b, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return err
	}
	rb := make([]byte, 4096)
	n, _, err := unix.Recvfrom(fd, rb, 0)
	if err != nil {
		return err
	}
	msgs, err := syscall.ParseNetlinkMessage(rb[:n])
	if err != nil {
		return err
	}
	for _, m := range msgs {
		if m.Header.Type == unix.NLMSG_ERROR && len(m.Data) >= 4 {
			if e := int32(ne.Uint32(m.Data)); e != 0 && unix.Errno(-e) != unix.EADDRNOTAVAIL {
				return unix.Errno(-e)
			}
		}
	}
	return nil
}

// ---- S-lcp-netns-224-accept -----------------------------------------------------------------
//
// linux_nl listens only in the lcp default namespace (F-ospf-host Q6): a pair whose host interface
// lives in any other namespace never gets linux-cp's (*,224.0.0.0/24) Accept, so OSPF/VRRP/etc.
// hellos arriving on the phy are dropped by the mfib before reaching the tap. The pair descriptor
// installs that Accept itself with ip_mroute_add_del (mfib source API, the same path linux-cp
// builds: proto IP4, zero next hop, the phy, weight 1, ACCEPT) in the phy's IPv4 table, and removes
// it with the pair.
//
// mfib forwards with the paths of the entry's best source only (mfib_entry.c
// mfib_entry_recalculate_forwarding: no inheritance), and API beats plugin-low. An API source with
// only Accept paths would therefore replace the table's local-receive Forward path (plugin-low)
// by a drop. The API source carries its own local-receive Forward path (FIB_API_PATH_TYPE_LOCAL),
// removed when its last Accept path goes.
//
// D-217 (option b, Q1): the API source would also shadow linux-cp's plugin-low Accepts of
// default-namespace pairs in the same table, so an IPv4 table never mixes the two kinds: Create
// refuses a pair outside the lcp default namespace in a table holding a default-namespace pair
// (any owner, from lcp_itf_pair_get + sw_interface_get_table), and the reverse. The check and the
// add are not atomic across agents (two agents creating opposite kinds at the same instant can
// both pass); recorded in the task's questions file.
//
// TD-lcp-leftover-local-path (Q2, arbiter finding 2): the API source's shared local path can be
// left in a table with no API Accept (kept on a dump error, or after a race), and that lone API
// source shadows the plugin-low Accept linux-cp gives a later default-namespace pair there. The pair
// dump does not see it, so a default-namespace pair's Create reads the entry first
// (clearLeftoverLocal). Fix round 1 (review finding 1, option (a)): an Accept there on the phy of a
// pair outside the lcp default namespace whose phy is now in another table is deleted first, not
// refused — it serves no packet, and the dump cannot tell the API source's from a stale linux-cp
// Accept on a reused sw_if_index (vpp-code-track, lcp_router.c row).

// DriftAPIAcceptMissing is ItfPair.Drift for a pair outside the lcp default namespace without the
// agent's (*,224.0.0.0/24) Accept on its phy.
const DriftAPIAcceptMissing = "api-accept-missing"

// outsideDefault reports whether a pair's namespace ns lies outside the lcp default namespace def.
func outsideDefault(ns, def string) bool {
	ns = strings.TrimRight(ns, "\x00")
	return ns != "" && ns != def
}

// refuseMixedTable returns an error when phy's IPv4 table already holds a pair of the other kind
// (default namespace vs. not). A pair already on phy (a re-apply; Create then adopts or refuses
// it) is not re-judged. For a new default-namespace pair it then clears a leftover API local path
// in the table's (*,224.0.0.0/24), or refuses when the API source still serves a pair outside the
// lcp default namespace (clearLeftoverLocal).
func (d *ItfPairDescriptor) refuseMixedTable(ctx context.Context, phy uint32, s ItfPair, def string, outside bool) error {
	pairs, err := Pairs(ctx, d.client)
	if err != nil {
		return err
	}
	for _, p := range pairs {
		if uint32(p.PhySwIfIndex) == phy {
			return nil
		}
	}
	table := ^uint32(0)
	for _, p := range pairs {
		other := uint32(p.PhySwIfIndex)
		if other == phy || outsideDefault(p.Netns, def) == outside {
			continue
		}
		if table == ^uint32(0) {
			if table, err = phyTable(ctx, d.client, phy); err != nil {
				return err
			}
		}
		ot, err := phyTable(ctx, d.client, other)
		if err != nil {
			return err
		}
		if ot == table {
			return mixedTableError(s, outside, table, "the pair", p)
		}
	}
	if outside {
		return nil
	}
	if table == ^uint32(0) {
		if table, err = phyTable(ctx, d.client, phy); err != nil {
			return err
		}
	}
	return d.clearLeftoverLocal(ctx, table, s, def, pairs)
}

// mixedTableError is the D-217 refusal of s (outside the lcp default namespace or not) in table,
// which already holds pair p of the other kind; holds says how ("the pair", or the pair's API Accept).
func mixedTableError(s ItfPair, outside bool, table uint32, holds string, p *lcp.LcpItfPairDetails) error {
	kind, okind := "outside the lcp default namespace", "in the lcp default namespace"
	if !outside {
		kind, okind = okind, kind
	}
	return fmt.Errorf("lcp pair %s ↔ %s (netns %q) refused: it is %s, but IPv4 table %d already holds %s "+
		"of sw_if_index %d (%s, netns %q) %s; one table must not mix both kinds, the agent's API (*,224.0.0.0/24) "+
		"Accept would shadow linux-cp's (D-217, S-lcp-netns-224-accept Q1) — put the pair in another VRF",
		s.Interface, s.HostIfName, s.Netns, kind, table, holds, uint32(p.PhySwIfIndex),
		strings.TrimRight(p.HostIfName, "\x00"), strings.TrimRight(p.Netns, "\x00"), okind)
}

// view224 is what a (*,224.0.0.0/24) dump of an IPv4 table shows: a local (receive) path, the
// number of Accept paths and of any other paths, and the Accepts on the phy of a pair outside the
// lcp default namespace:
//   - netns: the first such pair whose phy is in the table — the API source serves it there
//     (linux_nl never hears such a pair, so linux-cp gives it no Accept);
//   - moved: the phys of such pairs that are in another table now. The dump cannot tell the API
//     source's Accept (the pair's phy moved to another VRF) from linux-cp's stale plugin-low Accept
//     on a reused sw_if_index (vpp-code-track, lcp_router.c row: it survives its pair, and VPP hands
//     out freed indexes first). Either way it serves no packet: mfib looks a packet up in its input
//     interface's own table (mfib_forward.c).
type view224 struct {
	local          bool
	accepts, other int
	netns          *lcp.LcpItfPairDetails
	moved          []uint32
}

// loneLocal reports whether the entry shows a local path and nothing else.
func (v view224) loneLocal() bool { return v.local && v.accepts == 0 && v.other == 0 }

// judge224 sorts table's (*,224.0.0.0/24) paths against pairs; the phy of every pair outside the
// lcp default namespace that has an Accept there is looked up with sw_interface_get_table.
func (d *ItfPairDescriptor) judge224(ctx context.Context, table uint32, paths []mfib_types.MfibPath,
	pairs []*lcp.LcpItfPairDetails, def string) (view224, error) {
	byPhy := make(map[uint32]*lcp.LcpItfPairDetails, len(pairs))
	for _, p := range pairs {
		byPhy[uint32(p.PhySwIfIndex)] = p
	}
	var v view224
	for _, path := range paths {
		switch {
		case path.ItfFlags&mfib_types.MFIB_API_ITF_FLAG_ACCEPT != 0:
			v.accepts++
			p, ok := byPhy[uint32(path.Path.SwIfIndex)]
			if !ok || !outsideDefault(p.Netns, def) {
				continue
			}
			pt, err := phyTable(ctx, d.client, uint32(p.PhySwIfIndex))
			if err != nil {
				return v, err
			}
			if pt != table {
				v.moved = append(v.moved, uint32(p.PhySwIfIndex))
			} else if v.netns == nil {
				v.netns = p
			}
		case path.Path.Type == fib_types.FIB_API_PATH_TYPE_LOCAL:
			v.local = true
		default:
			v.other++
		}
	}
	return v, nil
}

// hasAccept reports whether a dump shows any Accept path.
func hasAccept(paths []mfib_types.MfibPath) bool {
	return slices.ContainsFunc(paths, func(p mfib_types.MfibPath) bool {
		return p.ItfFlags&mfib_types.MFIB_API_ITF_FLAG_ACCEPT != 0
	})
}

// samePaths reports whether two dumps of an entry show the same paths, in any order.
func samePaths(a, b []mfib_types.MfibPath) bool {
	if len(a) != len(b) {
		return false
	}
	n := make(map[mfib_types.MfibPath]int, len(a))
	for _, p := range a {
		n[p]++
	}
	for _, p := range b {
		if n[p]--; n[p] < 0 {
			return false
		}
	}
	return true
}

// logDelete logs an API delete sent to table's (*,224.0.0.0/24) by what the re-read shows (review
// NIT 4): Info (done) when the entry changed, Debug (ignored) when it looks the same — VPP ignored
// the delete because the path is another source's, or (no forwarding difference) the API source's
// lone local path gave way to an identical-looking linux-cp one.
func logDelete(ctx context.Context, log *slog.Logger, before, after []mfib_types.MfibPath, found bool, done, ignored string, args ...any) {
	if found && samePaths(before, after) {
		log.DebugContext(ctx, ignored, args...)
		return
	}
	log.InfoContext(ctx, done, append(args, "entry_gone", !found)...)
}

// apiAcceptHolds is mixedTableError's "holds" for a refusal judged from the (*,224.0.0.0/24) entry.
const apiAcceptHolds = "the API (*,224.0.0.0/24) Accept of the pair"

// clearLeftoverLocal runs before a default-namespace pair is added to table. VPP dumps the paths of
// the entry's best source only, so the entry is judged by what it shows (judge224):
//   - an Accept on the phy of a pair outside the lcp default namespace that is in table: the API
//     source serves that pair and would shadow the new pair's plugin-low Accept → refused (D-217);
//   - an Accept on the phy of such a pair that is in another table now (review finding 1, option
//     (a)): ip_mroute_add_del deletes that Accept from the API source. For a moved pair's API Accept
//     this removes it; for linux-cp's stale Accept VPP ignores it (no API source: mfib_entry_path_remove
//     returns at once; a path the API source does not hold is skipped: fib_path_list_paths_remove).
//     Then the entry and the pairs are read again, in that order — a pair whose Accept the entry
//     shows was added before that Accept, so a concurrent Create cannot slip between — and judged
//     again (a pair outside the lcp default namespace in table → refused);
//   - any other Accept (a default-namespace pair's, or one without a pair: a stale linux-cp Accept or
//     an API Accept whose pair is gone — the dump cannot tell them apart), or any other path: left
//     alone, never refused;
//   - a local path and nothing else: the leftover → ip_mroute_add_del deletes the API local path.
//     The dump cannot tell it from another source's lone local path (linux-cp's router table, CLI);
//     for those the delete is a no-op (mfib_entry_path_remove: the entry has no API source).
//
// After the local delete the entry is read again: an Accept of a pair outside the lcp default
// namespace in table that appeared meanwhile (another agent's Create, the race of Q2) gets the local
// path back and the pair is refused. When that re-read fails, the local path is given back too and
// the error returned (review finding 2): a racing pair's Accept left without it would never be
// repaired (Retrieve checks the Accept only), while a needless re-add is a leftover the retried
// Create clears again.
func (d *ItfPairDescriptor) clearLeftoverLocal(ctx context.Context, table uint32, s ItfPair, def string, pairs []*lcp.LcpItfPairDetails) error {
	paths, found, err := mroute224(ctx, d.client, table)
	if err != nil {
		return fmt.Errorf("ip_mroute_dump(table %d): %w", table, err)
	}
	if !found {
		return nil
	}
	v, err := d.judge224(ctx, table, paths, pairs, def)
	if err != nil {
		return err
	}
	if v.netns != nil {
		return mixedTableError(s, false, table, apiAcceptHolds, v.netns)
	}
	log := slog.Default().With("descriptor", NameItfPair, "interface", s.Interface, "table", table)
	svc := ip.NewServiceClient(d.client)
	if len(v.moved) > 0 {
		moved := make([]mfib_types.MfibPath, 0, len(v.moved))
		for _, phy := range v.moved {
			moved = append(moved, acceptPath(phy))
		}
		if _, err := svc.IPMrouteAddDel(ctx, &ip.IPMrouteAddDel{
			IsAdd: false, IsMultipath: true, Route: acceptRoute(table, moved...)}); err != nil {
			return fmt.Errorf("ip_mroute_add_del(del (*,224.0.0.0/24) accept sw_if_index %v table %d): %w", v.moved, table, err)
		}
		before := paths
		if paths, found, err = mroute224(ctx, d.client, table); err != nil {
			return fmt.Errorf("ip_mroute_dump(table %d) after the moved pairs' Accept delete: %w", table, err)
		}
		logDelete(ctx, log, before, paths, found,
			"lcp: deleted the API (*,224.0.0.0/24) Accept of pairs outside the lcp default namespace whose phy is "+
				"in another table (it serves no packet) before adding a default-namespace pair",
			"lcp: sent the API (*,224.0.0.0/24) Accept delete for pairs outside the lcp default namespace whose phy "+
				"is in another table; the entry is unchanged (a stale linux-cp Accept on a reused sw_if_index: VPP ignored it)",
			"sw_if_index", v.moved)
		if !found {
			return nil
		}
		if pairs, err = Pairs(ctx, d.client); err != nil {
			return err
		}
		if v, err = d.judge224(ctx, table, paths, pairs, def); err != nil {
			return err
		}
		if v.netns != nil {
			return mixedTableError(s, false, table, apiAcceptHolds, v.netns)
		}
	}
	if !v.loneLocal() {
		return nil
	}
	if _, err := svc.IPMrouteAddDel(ctx, &ip.IPMrouteAddDel{
		IsAdd: false, IsMultipath: true, Route: acceptRoute(table, localPath)}); err != nil {
		return fmt.Errorf("ip_mroute_add_del(del leftover (*,224.0.0.0/24) local table %d): %w", table, err)
	}
	before := paths
	if paths, found, err = mroute224(ctx, d.client, table); err != nil {
		return d.restoreLocal(ctx, table, fmt.Errorf("ip_mroute_dump(table %d) after the leftover delete: %w", table, err))
	}
	if found && hasAccept(paths) {
		if pairs, err = Pairs(ctx, d.client); err != nil {
			return d.restoreLocal(ctx, table, err)
		}
		if v, err = d.judge224(ctx, table, paths, pairs, def); err != nil {
			return d.restoreLocal(ctx, table, err)
		}
		if v.netns != nil {
			refused := mixedTableError(s, false, table, apiAcceptHolds, v.netns)
			if v.local {
				return refused
			}
			return d.restoreLocal(ctx, table, refused)
		}
	}
	logDelete(ctx, log, before, paths, found,
		"lcp: (*,224.0.0.0/24) held a local path and no Accept: deleted the API local path before adding a "+
			"default-namespace pair (a leftover of S-lcp-netns-224-accept Q2)",
		"lcp: (*,224.0.0.0/24) held a lone local path: sent the API local delete before adding a default-namespace "+
			"pair; the entry is unchanged (the path is another source's, e.g. linux-cp's router table: VPP ignored it)")
	return nil
}

// restoreLocal gives table's (*,224.0.0.0/24) the API source's shared local path back after a local
// delete that could not be checked, or that raced a pair outside the lcp default namespace (review
// finding 2), and returns cause joined with any re-add error. An identical path is not duplicated.
func (d *ItfPairDescriptor) restoreLocal(ctx context.Context, table uint32, cause error) error {
	if _, err := ip.NewServiceClient(d.client).IPMrouteAddDel(ctx, &ip.IPMrouteAddDel{
		IsAdd: true, IsMultipath: true, Route: acceptRoute(table, localPath)}); err != nil {
		return errors.Join(cause, fmt.Errorf("ip_mroute_add_del(re-add (*,224.0.0.0/24) local table %d): %w", table, err))
	}
	return cause
}

// rollback deletes a pair Create just made but could not finish (not claimed yet).
func (d *ItfPairDescriptor) rollback(ctx context.Context, phy uint32, s ItfPair) error {
	_, err := lcp.NewServiceClient(d.client).LcpItfPairAddDelV3(ctx, &lcp.LcpItfPairAddDelV3{
		IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(phy)})
	if err != nil && !dfkit.IsVPPError(err, api.INVALID_SW_IF_INDEX, api.NO_SUCH_ENTRY, api.INVALID_VALUE) {
		return fmt.Errorf("rollback lcp_itf_pair_add_del_v3(del %s): %w", s.Interface, err)
	}
	return nil
}

// phyAccept reports whether phy's IPv4 table's (*,224.0.0.0/24) has an Accept path on phy.
func (d *ItfPairDescriptor) phyAccept(ctx context.Context, phy uint32) (bool, error) {
	table, err := phyTable(ctx, d.client, phy)
	if err != nil {
		return false, err
	}
	return acceptIn(ctx, d.client, table, phy)
}

// installedAPIAccept decides whether Delete removes the API Accept: the Create/Retrieve flag when
// meta carries one, else (no meta) a pair outside VPP's own namespace whose phy shows an Accept.
// Neither depends on today's default netns (review M3). With D-217 an Accept on a non-root-netns
// pair's phy in an unmixed table is ours; removing an absent API path is a no-op for VPP.
func (d *ItfPairDescriptor) installedAPIAccept(ctx context.Context, meta any, phy uint32, ns string) (bool, error) {
	if m, ok := meta.(PairMeta); ok && m.PhySwIfIndex == phy {
		return m.APIAccept, nil
	}
	if ns == "" {
		return false, nil
	}
	return d.phyAccept(ctx, phy)
}

func acceptPath(phy uint32) mfib_types.MfibPath {
	return mfib_types.MfibPath{ItfFlags: mfib_types.MFIB_API_ITF_FLAG_ACCEPT,
		Path: fib_types.FibPath{SwIfIndex: phy, Weight: 1, Proto: fib_types.FIB_API_PATH_NH_PROTO_IP4}}
}

// localPath is the Forward-to-local-receive path of the table's (*,224.0.0.0/24) default entry.
var localPath = mfib_types.MfibPath{ItfFlags: mfib_types.MFIB_API_ITF_FLAG_FORWARD,
	Path: fib_types.FibPath{SwIfIndex: ^uint32(0), Weight: 1, Type: fib_types.FIB_API_PATH_TYPE_LOCAL,
		Proto: fib_types.FIB_API_PATH_NH_PROTO_IP4}}

func acceptRoute(table uint32, paths ...mfib_types.MfibPath) ip.IPMroute {
	return ip.IPMroute{
		TableID: table,
		Prefix: ip_types.Mprefix{Af: ip_types.ADDRESS_IP4, GrpAddressLength: 24,
			GrpAddress: ip_types.AddressUnionIP4(ip_types.IP4Address(acceptPrefix))},
		Paths: paths,
	}
}

// anyAccept reports whether table's (*,224.0.0.0/24) forwards with any Accept path.
func anyAccept(ctx context.Context, c vpp.Client, table uint32) (bool, error) {
	return acceptIn(ctx, c, table, ^uint32(0))
}

func phyTable(ctx context.Context, c vpp.Client, phy uint32) (uint32, error) {
	r, err := interfaces.NewServiceClient(c).SwInterfaceGetTable(ctx, &interfaces.SwInterfaceGetTable{
		SwIfIndex: interface_types.InterfaceIndex(phy)})
	if err != nil {
		return 0, fmt.Errorf("sw_interface_get_table(%d): %w", phy, err)
	}
	return r.VrfID, nil
}

// setAPIAccept adds or removes the agent's API-sourced (*,224.0.0.0/24) Accept path on phy.
func (d *ItfPairDescriptor) setAPIAccept(ctx context.Context, phy uint32, add bool) error {
	table, err := phyTable(ctx, d.client, phy)
	if err != nil {
		return err
	}
	svc := ip.NewServiceClient(d.client)
	paths := []mfib_types.MfibPath{acceptPath(phy)}
	if add {
		paths = append(paths, localPath) // an existing identical path is kept, not duplicated
	}
	if _, err := svc.IPMrouteAddDel(ctx, &ip.IPMrouteAddDel{
		IsAdd: add, IsMultipath: true, Route: acceptRoute(table, paths...)}); err != nil {
		return fmt.Errorf("ip_mroute_add_del(add=%v (*,224.0.0.0/24) accept sw_if_index %d table %d): %w", add, phy, table, err)
	}
	if add {
		return nil
	}
	// Q2: the API source keeps its shared local path while any (any owner's) Accept is left in
	// it; on a dump error it is kept (harmless: Forward-local alone forwards like the default).
	left, err := anyAccept(ctx, d.client, table)
	if err != nil || left {
		return nil
	}
	if _, err := svc.IPMrouteAddDel(ctx, &ip.IPMrouteAddDel{
		IsAdd: false, IsMultipath: true, Route: acceptRoute(table, localPath)}); err != nil {
		return fmt.Errorf("ip_mroute_add_del(del (*,224.0.0.0/24) local table %d): %w", table, err)
	}
	// another agent may have added its Accept between the dump and the delete: give it back the
	// local path (its own add re-adds it too; an identical path is not duplicated). When the re-read
	// fails it is given back as well and the error returned (review finding 2, as in
	// clearLeftoverLocal); the retried Delete removes it again.
	again, err := anyAccept(ctx, d.client, table)
	if err != nil {
		return d.restoreLocal(ctx, table, fmt.Errorf("ip_mroute_dump(table %d) after the local delete: %w", table, err))
	}
	if again {
		return d.restoreLocal(ctx, table, nil)
	}
	return nil
}
