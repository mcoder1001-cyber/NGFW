package ifsanitize

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.fd.io/govpp/api"

	classifyapi "ngfw/agent/binapi/classify"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	ipapi "ngfw/agent/binapi/ip"
	ipsecapi "ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/vpp"
)

// Finding is one problem the pre-flight found. Fatal findings are crash vectors (a classify
// binding or classify DPO to a table that does not exist: the first packet through it crashes
// VPP in vnet_classify_find_entry — V19, 2026-09-24 04:50:27); the others are reported only.
type Finding struct {
	Fatal     bool
	SwIfIndex uint32 // ~0 when not interface-bound (a FIB entry of no known interface)
	Interface string // "<name> (tag <tag>)", "DELETED (<idx>)" or ""
	What      string
	// Table is the missing classify table the finding is about (NoIndex when none). A table that
	// exists in the second classify_table_ids snapshot was created during the run (another slot):
	// the finding is dropped (TD-3 review M3, TOCTOU).
	Table uint32
}

func (f Finding) String() string {
	sev := "WARN"
	if f.Fatal {
		sev = "FAIL"
	}
	if f.Interface == "" {
		return fmt.Sprintf("%s  %s", sev, f.What)
	}
	return fmt.Sprintf("%s  interface %s: %s", sev, f.Interface, f.What)
}

type ifInfo struct {
	name, tag string
	adminUp   bool
}

// quarantined reports whether the interface is an agent's quarantine holder (ifsanitize.Acquire):
// admin-down, tagged "quarantine:<owner>". Its stale bindings are known and it is never used, so
// the pre-flight reports them as WARN — the same rule as the agent (TD-3 review M4).
func (i ifInfo) quarantined() bool { return IsQuarantineTag(i.tag) && !i.adminUp }

func (i ifInfo) String() string {
	if i.tag == "" {
		return i.name + " (untagged)"
	}
	return fmt.Sprintf("%s (tag %s)", i.name, i.tag)
}

// Preflight inspects the shared VPP for per-interface bindings to classify tables that no
// longer exist, and for IPsec SPD bindings left on deleted or foreign interfaces (D-095 d,
// DF-5 review M3). Binary-API readbacks are used where VPP has them (sw_interface_dump,
// classify_table_ids, classify_table_by_interface, ipsec_spd_interface_dump). The bindings VPP
// has no binary-API readback for — output ACL, policer and flow classify (their dumps are
// broken in 26.06), and the ip classify table, which is only visible as a classify DPO in the
// FIB — are read from VPP's own show commands through cli_inband: this is a read-only CI
// diagnostic, never used by the agent.
//
// Fatal: a binding on an existing interface, or a classify DPO in the FIB, to a missing table.
// Reported: bindings of deleted interfaces to missing tables (dormant until the index is
// reused; the agent's creators clear them, others may not), SPD bindings on deleted interfaces
// (they block the next interface on that index) and on untagged (foreign) interfaces.
func Preflight(ctx context.Context, c vpp.Client) ([]Finding, error) {
	ifs := map[uint32]ifInfo{}
	stream, err := interfaces.NewServiceClient(c).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: ^interface_types.InterfaceIndex(0)})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_dump: %w", err)
	}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("sw_interface_dump: %w", err)
		}
		ifs[uint32(d.SwIfIndex)] = ifInfo{name: strings.TrimRight(d.InterfaceName, "\x00"), tag: strings.TrimRight(d.Tag, "\x00"),
			adminUp: d.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0}
	}
	cl := classifyapi.NewServiceClient(c)
	idsRep, err := cl.ClassifyTableIds(ctx, &classifyapi.ClassifyTableIds{})
	if err != nil {
		return nil, fmt.Errorf("classify_table_ids: %w", err)
	}
	tables := map[uint32]bool{}
	for _, id := range idsRep.Ids {
		tables[id] = true
	}
	var out []Finding
	seen := map[string]bool{}
	add := func(f Finding) {
		if k := f.String(); !seen[k] {
			seen[k] = true
			out = append(out, f)
		}
	}
	// fatal: a binding on an existing interface that is not a quarantine holder
	ifName := func(idx uint32) (string, bool) {
		if in, ok := ifs[idx]; ok {
			return in.String(), !in.quarantined()
		}
		return fmt.Sprintf("DELETED (%d)", idx), false
	}

	// chained tables: a live table whose next_table_index is gone crashes the same chain walk (M2)
	for _, id := range sortedIDs(tables) {
		info, err := cl.ClassifyTableInfo(ctx, &classifyapi.ClassifyTableInfo{TableID: id})
		if err != nil {
			continue // deleted since classify_table_ids
		}
		if info.NextTableIndex != NoIndex && !tables[info.NextTableIndex] {
			add(Finding{Fatal: true, SwIfIndex: NoIndex, Table: info.NextTableIndex,
				What: fmt.Sprintf("classify table %d chains to classify table %d, which does not exist", id, info.NextTableIndex)})
		}
	}

	// input ACL through the binary API, on every existing interface
	idxs := make([]uint32, 0, len(ifs))
	for idx := range ifs {
		idxs = append(idxs, idx)
	}
	sort.Slice(idxs, func(i, j int) bool { return idxs[i] < idxs[j] })
	for _, idx := range idxs {
		rep, err := cl.ClassifyTableByInterface(ctx, &classifyapi.ClassifyTableByInterface{SwIfIndex: interface_types.InterfaceIndex(idx)})
		if isRetval(err, api.INVALID_SW_IF_INDEX) {
			continue // deleted since the dump
		}
		if err != nil {
			return nil, fmt.Errorf("classify_table_by_interface %d: %w", idx, err)
		}
		for _, b := range []struct {
			kind string
			t    uint32
		}{{"ip4", rep.IP4TableID}, {"ip6", rep.IP6TableID}, {"l2", rep.L2TableID}} {
			if b.t != NoIndex && !tables[b.t] {
				name, fatal := ifName(idx)
				add(Finding{Fatal: fatal, SwIfIndex: idx, Interface: name, Table: b.t, What: fmt.Sprintf("input ACL %s bound to classify table %d, which does not exist", b.kind, b.t)})
			}
		}
	}

	// the per-index binding vectors VPP only shows through its CLI (includes deleted indices)
	cli := func(cmd string) (string, error) {
		rep, err := vlib.NewServiceClient(c).CliInband(ctx, &vlib.CliInband{Cmd: cmd})
		if err != nil {
			return "", fmt.Errorf("cli_inband %q: %w", cmd, err)
		}
		return rep.Reply, nil
	}
	for _, src := range []struct{ cmd, what string }{
		{"show inacl type ip4", "input ACL ip4"}, {"show inacl type ip6", "input ACL ip6"}, {"show inacl type l2", "input ACL l2"},
		{"show outacl type ip4", "output ACL ip4"}, {"show outacl type ip6", "output ACL ip6"}, {"show outacl type l2", "output ACL l2"},
		{"show classify policer type ip4", "policer classify ip4"}, {"show classify policer type ip6", "policer classify ip6"}, {"show classify policer type l2", "policer classify l2"},
		{"show classify flow type ip4", "flow classify ip4"}, {"show classify flow type ip6", "flow classify ip6"},
	} {
		text, err := cli(src.cmd)
		if err != nil {
			return nil, err
		}
		for _, b := range parseBindingTable(text) {
			if tables[b.table] {
				continue
			}
			name, live := ifName(b.idx)
			add(Finding{Fatal: live, SwIfIndex: b.idx, Interface: name, Table: b.table, What: fmt.Sprintf("%s bound to classify table %d, which does not exist", src.what, b.table)})
		}
	}

	// the ip classify binding only shows as a classify DPO on the interface's addresses
	addrOwner := addressOwners(ctx, c, idxs)
	for _, cmd := range []string{"show ip fib", "show ip6 fib"} {
		text, err := cli(cmd)
		if err != nil {
			return nil, err
		}
		for _, e := range parseClassifyDPOs(text) {
			if tables[e.table] {
				continue
			}
			f := Finding{Fatal: true, SwIfIndex: NoIndex, Table: e.table, What: fmt.Sprintf("FIB %s %s has a classify DPO to classify table %d, which does not exist (ip classify binding inherited or left behind)", e.vrf, e.prefix, e.table)}
			if idx, ok := addrOwner[e.addr]; ok {
				f.SwIfIndex, f.Interface = idx, ifs[idx].String()
				f.Fatal = !ifs[idx].quarantined()
			}
			add(f)
		}
	}

	// IPsec SPD bindings (DF-5 review M3)
	spds, err := ipsecapi.NewServiceClient(c).IpsecSpdInterfaceDump(ctx, &ipsecapi.IpsecSpdInterfaceDump{})
	if err == nil {
		for {
			d, err := spds.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("ipsec_spd_interface_dump: %w", err)
			}
			idx := uint32(d.SwIfIndex)
			in, live := ifs[idx]
			switch {
			case !live:
				add(Finding{SwIfIndex: idx, Table: NoIndex, Interface: fmt.Sprintf("DELETED (%d)", idx), What: fmt.Sprintf("IPsec SPD (index %d) still bound: the next interface on this index cannot get an SPD", d.SpdIndex)})
			case in.tag == "" && idx != 0:
				add(Finding{SwIfIndex: idx, Table: NoIndex, Interface: in.String(), What: fmt.Sprintf("IPsec SPD (index %d) bound on an untagged (foreign) interface", d.SpdIndex)})
			}
		}
	} else if !unknownMsg(err) {
		return nil, fmt.Errorf("ipsec_spd_interface_dump: %w", err)
	}
	// TOCTOU (review M3): a table created (and bound) by another slot after the first snapshot
	// is not missing — only tables absent from both snapshots count
	again, err := cl.ClassifyTableIds(ctx, &classifyapi.ClassifyTableIds{})
	if err != nil {
		return nil, fmt.Errorf("classify_table_ids (second snapshot): %w", err)
	}
	now := map[uint32]bool{}
	for _, id := range again.Ids {
		now[id] = true
	}
	kept := out[:0]
	for _, f := range out {
		if f.Table != NoIndex && now[f.Table] {
			continue
		}
		kept = append(kept, f)
	}
	out = kept
	sort.SliceStable(out, func(i, j int) bool { return out[i].Fatal && !out[j].Fatal })
	return out, nil
}

func sortedIDs(m map[uint32]bool) []uint32 {
	out := make([]uint32, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

type binding struct{ idx, table uint32 }

var bindingLine = regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s+`)

// parseBindingTable reads the "%10d%20d\t\t<name>" rows of show inacl/outacl/classify
// policer/classify flow.
func parseBindingTable(text string) []binding {
	var out []binding
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		m := bindingLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		idx, err1 := strconv.ParseUint(m[1], 10, 32)
		t, err2 := strconv.ParseUint(m[2], 10, 32)
		if err1 == nil && err2 == nil {
			out = append(out, binding{uint32(idx), uint32(t)})
		}
	}
	return out
}

type classifyDPO struct {
	vrf, prefix string
	addr        netip.Addr
	table       uint32
}

var (
	fibHeader  = regexp.MustCompile(`^(ipv[46]-VRF:\d+)`)
	fibPrefix  = regexp.MustCompile(`^([0-9a-fA-F:.]+/\d+)`)
	classifyRe = regexp.MustCompile(`classify:\[\d+\]:table:(\d+)`)
)

// parseClassifyDPOs finds classify DPOs ("ip4-classify:[i]:table:T", format_classify_dpo) in
// `show ip[6] fib` output and the FIB entry they belong to.
func parseClassifyDPOs(text string) []classifyDPO {
	var out []classifyDPO
	vrf, prefix := "", ""
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if m := fibHeader.FindStringSubmatch(line); m != nil {
			vrf = m[1]
			continue
		}
		if m := fibPrefix.FindStringSubmatch(line); m != nil {
			prefix = m[1]
			continue
		}
		for _, m := range classifyRe.FindAllStringSubmatch(line, -1) {
			t, err := strconv.ParseUint(m[1], 10, 32)
			if err != nil || prefix == "" {
				continue
			}
			var a netip.Addr
			if p, err := netip.ParsePrefix(prefix); err == nil {
				a = p.Addr()
			}
			out = append(out, classifyDPO{vrf: vrf, prefix: prefix, addr: a, table: uint32(t)})
		}
	}
	return out
}

// addressOwners maps every interface address to its interface (best effort).
func addressOwners(ctx context.Context, c vpp.Client, idxs []uint32) map[netip.Addr]uint32 {
	out := map[netip.Addr]uint32{}
	for _, idx := range idxs {
		for _, v6 := range []bool{false, true} {
			s, err := ipAddressDump(ctx, c, idx, v6)
			if err != nil {
				continue
			}
			for _, a := range s {
				out[a] = idx
			}
		}
	}
	return out
}

func ipAddressDump(ctx context.Context, c vpp.Client, idx uint32, v6 bool) ([]netip.Addr, error) {
	s, err := ipapi.NewServiceClient(c).IPAddressDump(ctx, &ipapi.IPAddressDump{SwIfIndex: interface_types.InterfaceIndex(idx), IsIPv6: v6})
	if err != nil {
		return nil, err
	}
	var out []netip.Addr
	for {
		d, err := s.Recv()
		if err != nil {
			return out, nil
		}
		if a, ok := netip.AddrFromSlice(d.Prefix.Address.ToIP()); ok {
			out = append(out, a.Unmap())
		}
	}
}
