package acl

// Direct VPP access for the ACL evidence and the simulated loss — binary API only (never the product API), fixed
// vppctl show commands (never a packet trace, D-128), only objects that carry this slot's prefix.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	vppapi "go.fd.io/govpp/api"

	vppacl "ngfw/agent/binapi/acl"
	"ngfw/agent/binapi/acl_types"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/vlib"
)

type aclEntry struct {
	idx   uint32
	tag   string
	rules int
}

func apiCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

func drainDump[T any](t *testing.T, what string, recv func() (T, error)) []T {
	t.Helper()
	var out []T
	for {
		d, err := recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		out = append(out, d)
	}
}

// aclDump returns every ACL in VPP (all owners) — used only for the tag list, never polled.
func aclDump(t *testing.T, conn vppapi.Connection) []aclEntry {
	t.Helper()
	ctx, cancel := apiCtx()
	defer cancel()
	st, err := vppacl.NewServiceClient(conn).ACLDump(ctx, &vppacl.ACLDump{ACLIndex: noIndex})
	if err != nil {
		t.Fatal(err)
	}
	var out []aclEntry
	for _, d := range drainDump(t, "acl_dump", st.Recv) {
		out = append(out, aclEntry{idx: d.ACLIndex, tag: strings.TrimRight(d.Tag, "\x00"), rules: len(d.R)})
	}
	return out
}

// ownACLs returns the ACLs tagged "<owner>:<name>" as name → entry.
func ownACLs(t *testing.T, conn vppapi.Connection, owner string) map[string]aclEntry {
	t.Helper()
	out := map[string]aclEntry{}
	for _, a := range aclDump(t, conn) {
		if n, ok := strings.CutPrefix(a.tag, owner+":"); ok {
			out[n] = a
		}
	}
	return out
}

func ifaceACLs(t *testing.T, conn vppapi.Connection, swif uint32) (uint8, []uint32) {
	t.Helper()
	ctx, cancel := apiCtx()
	defer cancel()
	st, err := vppacl.NewServiceClient(conn).ACLInterfaceListDump(ctx, &vppacl.ACLInterfaceListDump{SwIfIndex: interface_types.InterfaceIndex(swif)})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range drainDump(t, "acl_interface_list_dump", st.Recv) {
		if uint32(d.SwIfIndex) == swif {
			return d.NInput, d.Acls
		}
	}
	return 0, nil
}

func setIfaceACLs(t *testing.T, conn vppapi.Connection, swif uint32, nInput uint8, acls ...uint32) {
	t.Helper()
	ctx, cancel := apiCtx()
	defer cancel()
	if _, err := vppacl.NewServiceClient(conn).ACLInterfaceSetACLList(ctx, &vppacl.ACLInterfaceSetACLList{
		SwIfIndex: interface_types.InterfaceIndex(swif), NInput: nInput, Acls: acls,
	}); err != nil {
		t.Fatalf("acl_interface_set_acl_list sw_if_index=%d %v: %v", swif, acls, err)
	}
}

func prefixAPI(s string) ip_types.Prefix {
	p := netip.MustParsePrefix(s)
	var out ip_types.Prefix
	out.Len = uint8(p.Bits()) //nolint:gosec // 0..128
	if p.Addr().Is4() {
		out.Address.Af = ip_types.ADDRESS_IP4
		out.Address.Un.SetIP4(ip_types.IP4Address(p.Addr().As4()))
	} else {
		out.Address.Af = ip_types.ADDRESS_IP6
		out.Address.Un.SetIP6(ip_types.IP6Address(p.Addr().As16()))
	}
	return out
}

// addACL creates an ACL with the given tag and rules (acl_add_replace, index ~0) and returns its index and duration.
func addACL(t *testing.T, conn vppapi.Connection, tag string, rules []acl_types.ACLRule) (uint32, time.Duration) {
	t.Helper()
	ctx, cancel := apiCtx()
	defer cancel()
	start := time.Now()
	rep, err := vppacl.NewServiceClient(conn).ACLAddReplace(ctx, &vppacl.ACLAddReplace{ACLIndex: noIndex, Tag: tag, R: rules})
	if err != nil {
		t.Fatalf("acl_add_replace %s (%d rules): %v", tag, len(rules), err)
	}
	return rep.ACLIndex, time.Since(start)
}

func delACL(t *testing.T, conn vppapi.Connection, idx uint32) {
	t.Helper()
	ctx, cancel := apiCtx()
	defer cancel()
	if _, err := vppacl.NewServiceClient(conn).ACLDel(ctx, &vppacl.ACLDel{ACLIndex: idx}); err != nil {
		t.Fatalf("acl_del %d: %v", idx, err)
	}
}

// foreignRule never matches the rig's traffic: deny udp 10.<N>.1.2 → 10.<N>.2.2 port 9 (discard).
func foreignRule(slot int) acl_types.ACLRule {
	return acl_types.ACLRule{
		IsPermit:  acl_types.ACL_ACTION_API_DENY,
		SrcPrefix: prefixAPI(fmt.Sprintf("10.%d.1.2/32", slot)), DstPrefix: prefixAPI(fmt.Sprintf("10.%d.2.2/32", slot)),
		Proto: 17, SrcportOrIcmptypeFirst: 0, SrcportOrIcmptypeLast: 65535, DstportOrIcmpcodeFirst: 9, DstportOrIcmpcodeLast: 9,
	}
}

// scaleRules builds n distinct permit rules (10.<N>.x.y/32 → 172.16-31.x.y/32 tcp 443) for the timing probe.
func scaleRules(slot, n int) []acl_types.ACLRule {
	out := make([]acl_types.ACLRule, n)
	for i := range out {
		out[i] = acl_types.ACLRule{
			IsPermit:  acl_types.ACL_ACTION_API_PERMIT,
			SrcPrefix: prefixAPI(fmt.Sprintf("10.%d.%d.%d/32", slot, 100+i/65536, i/256%256)),
			DstPrefix: prefixAPI(fmt.Sprintf("172.%d.%d.%d/32", 16+i/65536, i/256%256, i%256)),
			Proto:     6, SrcportOrIcmptypeLast: 65535, DstportOrIcmpcodeFirst: 443, DstportOrIcmpcodeLast: 443,
		}
	}
	return out
}

type macipBinding struct {
	swif, acl uint32
}

func macipBindings(t *testing.T, conn vppapi.Connection) []macipBinding {
	t.Helper()
	ctx, cancel := apiCtx()
	defer cancel()
	st, err := vppacl.NewServiceClient(conn).MacipACLInterfaceListDump(ctx, &vppacl.MacipACLInterfaceListDump{SwIfIndex: interface_types.InterfaceIndex(noIndex)})
	if err != nil {
		t.Fatal(err)
	}
	var out []macipBinding
	for _, d := range drainDump(t, "macip_acl_interface_list_dump", st.Recv) {
		for _, a := range d.Acls {
			if a != noIndex {
				out = append(out, macipBinding{swif: uint32(d.SwIfIndex), acl: a})
			}
		}
	}
	return out
}

// macipDump returns every MACIP ACL in VPP (all owners).
func macipDump(t *testing.T, conn vppapi.Connection) []aclEntry {
	t.Helper()
	ctx, cancel := apiCtx()
	defer cancel()
	st, err := vppacl.NewServiceClient(conn).MacipACLDump(ctx, &vppacl.MacipACLDump{ACLIndex: noIndex})
	if err != nil {
		t.Fatal(err)
	}
	var out []aclEntry
	for _, d := range drainDump(t, "macip_acl_dump", st.Recv) {
		out = append(out, aclEntry{idx: d.ACLIndex, tag: strings.TrimRight(d.Tag, "\x00"), rules: len(d.R)})
	}
	return out
}

func ownMacips(t *testing.T, conn vppapi.Connection, owner string) map[string]aclEntry {
	t.Helper()
	out := map[string]aclEntry{}
	for _, m := range macipDump(t, conn) {
		if n, ok := strings.CutPrefix(m.tag, owner+":"); ok {
			out[n] = m
		}
	}
	return out
}

func macipUnbind(t *testing.T, conn vppapi.Connection, swif, acl uint32) {
	t.Helper()
	ctx, cancel := apiCtx()
	defer cancel()
	if _, err := vppacl.NewServiceClient(conn).MacipACLInterfaceAddDel(ctx, &vppacl.MacipACLInterfaceAddDel{IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(swif), ACLIndex: acl}); err != nil {
		t.Fatalf("macip_acl_interface_add_del del %d/%d: %v", swif, acl, err)
	}
}

func macipDel(t *testing.T, conn vppapi.Connection, idx uint32) {
	t.Helper()
	ctx, cancel := apiCtx()
	defer cancel()
	if _, err := vppacl.NewServiceClient(conn).MacipACLDel(ctx, &vppacl.MacipACLDel{ACLIndex: idx}); err != nil {
		t.Fatalf("macip_acl_del %d: %v", idx, err)
	}
}

var countersFlagRe = regexp.MustCompile(`Stats counters enabled for interface ACLs:\s*(\d+)`)

// countersFlag reads the acl plugin's counters flag (read-only CLI, the "mask" qualifier prints no hash tables).
func countersFlag(t *testing.T, conn vppapi.Connection) bool {
	t.Helper()
	ctx, cancel := apiCtx()
	defer cancel()
	rep, err := vlib.NewServiceClient(conn).CliInband(ctx, &vlib.CliInband{Cmd: "show acl-plugin tables mask"})
	if err != nil {
		t.Fatalf("cli_inband: %v", err)
	}
	m := countersFlagRe.FindStringSubmatch(rep.Reply)
	if m == nil {
		t.Fatalf("no counters flag in %q", rep.Reply)
	}
	n, _ := strconv.Atoi(m[1])
	return n != 0
}

// setCounters switches the per-rule counters on or off (acl_stats_intf_counters_enable; VPP 26.06 answers with
// acl_del_reply, so the request goes on a raw stream and either reply is accepted, V7).
func setCounters(t *testing.T, conn vppapi.Connection, on bool) {
	t.Helper()
	ctx, cancel := apiCtx()
	defer cancel()
	st, err := conn.NewStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if err := st.SendMsg(&vppacl.ACLStatsIntfCountersEnable{Enable: on}); err != nil {
		t.Fatalf("acl_stats_intf_counters_enable %v: %v", on, err)
	}
	m, err := st.RecvMsg()
	if err != nil {
		t.Fatalf("acl_stats_intf_counters_enable reply: %v", err)
	}
	switch r := m.(type) {
	case *vppacl.ACLStatsIntfCountersEnableReply:
		if r.Retval != 0 {
			t.Fatalf("acl_stats_intf_counters_enable retval %d", r.Retval)
		}
	case *vppacl.ACLDelReply:
		if r.Retval != 0 {
			t.Fatalf("acl_stats_intf_counters_enable (acl_del_reply) retval %d", r.Retval)
		}
	default:
		t.Fatalf("acl_stats_intf_counters_enable: unexpected reply %T", m)
	}
}

// countersScope makes the test rely on the VPP-wide ACL counters flag under the globals lock (shared-host rules §7,
// D-082) until t ends: with change (opt-in VRX_ACL_STATS_GLOBALS=1) it takes flock -x, saves the current value and
// switches the counters on, then holds flock -s while the test relies on them; at the end it takes flock -x again and
// restores EXACTLY the saved value (off only if it was off), then unlocks. Without change it holds flock -s. It returns
// whether the counters are on for the test.
func countersScope(t *testing.T, conn vppapi.Connection, change bool) bool {
	t.Helper()
	f, err := os.OpenFile(globalsLockPath, os.O_RDONLY|os.O_CREATE, 0o666) //nolint:gosec // the shared globals lock
	if err != nil {
		t.Fatal(err)
	}
	lock := func(how int) {
		if err := syscall.Flock(int(f.Fd()), how); err != nil {
			t.Fatalf("flock %s: %v", globalsLockPath, err)
		}
	}
	if !change {
		lock(syscall.LOCK_SH)
		t.Cleanup(func() { lock(syscall.LOCK_UN); _ = f.Close() })
		return countersFlag(t, conn)
	}
	lock(syscall.LOCK_EX)
	prev := countersFlag(t, conn)
	if !prev {
		setCounters(t, conn, true)
	}
	on := countersFlag(t, conn)
	t.Logf("counters flag: saved %v, now %v (flock -x %s, then -s while the test relies on it)", prev, on, globalsLockPath)
	lock(syscall.LOCK_SH)
	t.Cleanup(func() {
		lock(syscall.LOCK_EX)
		if countersFlag(t, conn) != prev {
			setCounters(t, conn, prev)
		}
		t.Logf("counters flag restored to the saved value %v (now %v)", prev, countersFlag(t, conn))
		lock(syscall.LOCK_UN)
		_ = f.Close()
	})
	return on
}

const globalsLockPath = "/run/lock/vrx-globals.lock"

// showOwnACLs keeps the blocks of `vppctl show acl-plugin acl` whose tag starts with one of the prefixes (the command
// prints every owner's ACLs).
func showOwnACLs(out string, prefixes ...string) string {
	var b strings.Builder
	keep := false
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "acl-index ") {
			keep = false
			for _, p := range prefixes {
				if strings.Contains(l, "tag {"+p) {
					keep = true
				}
			}
		}
		if keep {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}
