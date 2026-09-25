package bonding

// Direct VPP access of the test (never of the product API): the fixture taps that stand in for NICs, binary API dumps
// for the evidence and the read-only binding check, the simulated loss, and vppctl `show` commands for the evidence the
// acceptance asks for. Nothing here traces packets (D-128) or unbinds anything by index (D-126).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"

	govpp "go.fd.io/govpp"
	vppapi "go.fd.io/govpp/api"

	"ngfw/agent/binapi/acl"
	bondapi "ngfw/agent/binapi/bond"
	"ngfw/agent/binapi/classify"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/tapv2"
)

const apiSocket = "/run/vpp/api.sock"

const noIndex = ^uint32(0)

func connectVPP(t *testing.T) vppapi.Connection {
	t.Helper()
	conn, err := govpp.Connect(apiSocket)
	if err != nil {
		t.Fatalf("govpp connect: %v", err)
	}
	t.Cleanup(conn.Disconnect)
	return conn
}

func ctx10() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

type vppIf struct {
	idx     uint32
	name    string
	tag     string
	adminUp bool
}

func dumpIfs(t *testing.T, conn vppapi.Connection) map[string]vppIf {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	stream, err := interfaces.NewServiceClient(conn).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(noIndex)})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]vppIf{}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		n := strings.TrimRight(d.InterfaceName, "\x00")
		out[n] = vppIf{idx: uint32(d.SwIfIndex), name: n, tag: strings.TrimRight(d.Tag, "\x00"), adminUp: d.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0}
	}
	return out
}

// fixtureTap is one tap the test creates as a stand-in NIC: untagged (so the agent owns only its memberships and
// attributes, through the persisted ClaimStore — D-075), VPP name tap<id> with the id from the slot's range, host side
// <prefix>bm<i> kept DOWN with IPv6 off, so the kernel sends nothing into VPP.
type fixtureTap struct {
	id      uint32
	name    string // VPP (= logical) name
	hostDev string
}

func fixtureTaps(s slot, n int) []fixtureTap {
	var out []fixtureTap
	for i := 0; i < n; i++ {
		id := uint32(s.num*1000 + i) //nolint:gosec // slot 1–11, i < 10
		out = append(out, fixtureTap{id: id, name: fmt.Sprintf("tap%d", id), hostDev: fmt.Sprintf("%sbm%d", s.prefix, i)})
	}
	return out
}

// createTaps creates the fixture taps (a leftover of an earlier run with the same host name is deleted first) and
// deletes them again in t.Cleanup.
func createTaps(t *testing.T, conn vppapi.Connection, taps []fixtureTap) {
	t.Helper()
	svc := tapv2.NewServiceClient(conn)
	for _, tp := range taps {
		deleteTap(t, conn, tp) // leftover
		ctx, cancel := ctx10()
		r, err := svc.TapCreateV3(ctx, &tapv2.TapCreateV3{
			ID: tp.id, UseRandomMac: true, NumRxQueues: 1, NumTxQueues: 1, TxRingSz: 256, RxRingSz: 256,
			HostIfNameSet: true, HostIfName: tp.hostDev,
		})
		cancel()
		if err != nil {
			t.Fatalf("tap_create_v3 %s (%s): %v", tp.name, tp.hostDev, err)
		}
		t.Logf("fixture tap %s sw_if_index=%d host %s (untagged)", tp.name, r.SwIfIndex, tp.hostDev)
		_, _ = run(t, "sysctl", "-qw", "net.ipv6.conf."+tp.hostDev+".disable_ipv6=1")
		mustRun(t, "ip", "link", "set", tp.hostDev, "down")
	}
	t.Cleanup(func() {
		for _, tp := range taps {
			deleteTap(t, conn, tp)
		}
	})
}

// deleteTap deletes the fixture tap if it exists and is untagged (never another owner's interface).
func deleteTap(t *testing.T, conn vppapi.Connection, tp fixtureTap) {
	t.Helper()
	i, ok := dumpIfs(t, conn)[tp.name]
	if !ok {
		return
	}
	if i.tag != "" {
		t.Fatalf("%s exists with tag %q — not a fixture of this slot, refusing to touch it", tp.name, i.tag)
	}
	ctx, cancel := ctx10()
	defer cancel()
	if _, err := tapv2.NewServiceClient(conn).TapDeleteV2(ctx, &tapv2.TapDeleteV2{SwIfIndex: interface_types.InterfaceIndex(i.idx)}); err != nil {
		t.Errorf("tap_delete_v2 %s: %v", tp.name, err)
		return
	}
	t.Logf("deleted fixture tap %s (sw_if_index %d)", tp.name, i.idx)
}

// bindingCheck is the read-only V19 check of the envelope (D-126: nothing is unbound by index): the fixture taps carry no
// input classify table, no ACL and no SPD binding. The test sends no packets at all; this only proves that the
// members start clean.
func bindingCheck(t *testing.T, conn vppapi.Connection, taps []fixtureTap) []string {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	ifs := dumpIfs(t, conn)
	var ev []string
	spds := map[uint32]uint32{}
	if stream, err := ipsec.NewServiceClient(conn).IpsecSpdInterfaceDump(ctx, &ipsec.IpsecSpdInterfaceDump{}); err == nil {
		for {
			d, err := stream.Recv()
			if err != nil {
				break
			}
			spds[uint32(d.SwIfIndex)] = d.SpdIndex
		}
	}
	for _, tp := range taps {
		idx := ifs[tp.name].idx
		r, err := classify.NewServiceClient(conn).ClassifyTableByInterface(ctx, &classify.ClassifyTableByInterface{SwIfIndex: interface_types.InterfaceIndex(idx)})
		if err != nil {
			t.Fatalf("classify_table_by_interface %s: %v", tp.name, err)
		}
		if r.L2TableID != noIndex || r.IP4TableID != noIndex || r.IP6TableID != noIndex {
			t.Fatalf("V19: %s (sw_if_index %d) carries an input classify binding %d/%d/%d — not touching it (D-126)", tp.name, idx, r.L2TableID, r.IP4TableID, r.IP6TableID)
		}
		al, err := acl.NewServiceClient(conn).ACLInterfaceListDump(ctx, &acl.ACLInterfaceListDump{SwIfIndex: interface_types.InterfaceIndex(idx)})
		if err != nil {
			t.Fatalf("acl_interface_list_dump %s: %v", tp.name, err)
		}
		for {
			d, err := al.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("acl_interface_list_dump %s: %v", tp.name, err)
			}
			if uint32(d.SwIfIndex) == idx && len(d.Acls) > 0 {
				t.Fatalf("V19: %s has %d ACL bindings — not touching them (D-126)", tp.name, len(d.Acls))
			}
		}
		if spd, ok := spds[idx]; ok {
			t.Fatalf("V19: %s is bound to SPD %d", tp.name, spd)
		}
		ev = append(ev, fmt.Sprintf("%s sw_if_index=%d: classify l2/ip4/ip6=~0, no ACL, no SPD", tp.name, idx))
	}
	return ev
}

type bondInfo struct {
	idx     uint32
	name    string
	id      uint32
	mode    bondapi.BondMode
	lb      bondapi.BondLbAlgo
	members map[string]*bondapi.SwMemberInterfaceDetails
}

// bonds dumps every bond whose id is in the slot's range, with its members.
func bonds(t *testing.T, conn vppapi.Connection, s slot) map[string]bondInfo {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	svc := bondapi.NewServiceClient(conn)
	stream, err := svc.SwBondInterfaceDump(ctx, &bondapi.SwBondInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(noIndex)})
	if err != nil {
		t.Fatal(err)
	}
	var ds []*bondapi.SwBondInterfaceDetails
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if d.ID/1000 == uint32(s.num) { //nolint:gosec // slot 1–11
			ds = append(ds, d)
		}
	}
	out := map[string]bondInfo{}
	for _, d := range ds {
		b := bondInfo{idx: uint32(d.SwIfIndex), name: strings.TrimRight(d.InterfaceName, "\x00"), id: d.ID, mode: d.Mode, lb: d.Lb, members: map[string]*bondapi.SwMemberInterfaceDetails{}}
		ms, err := svc.SwMemberInterfaceDump(ctx, &bondapi.SwMemberInterfaceDump{SwIfIndex: d.SwIfIndex})
		if err != nil {
			t.Fatal(err)
		}
		for {
			m, err := ms.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			b.members[strings.TrimRight(m.InterfaceName, "\x00")] = m
		}
		out[b.name] = b
	}
	return out
}

// loseBonds simulates the loss behind the agent's back: members first, then the bond addresses, then the bonds
// (dependents first, D-095 c) — through the binary API, never through the agent.
func loseBonds(t *testing.T, conn vppapi.Connection, s slot) []string {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	svc := bondapi.NewServiceClient(conn)
	var ev []string
	bs := bonds(t, conn, s)
	names := make([]string, 0, len(bs))
	for n := range bs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		for m, d := range bs[n].members {
			if _, err := svc.BondDetachMember(ctx, &bondapi.BondDetachMember{SwIfIndex: d.SwIfIndex}); err != nil {
				t.Fatalf("loss: bond_detach_member %s: %v", m, err)
			}
			ev = append(ev, fmt.Sprintf("bond_detach_member %s (sw_if_index %d) from %s → ok", m, d.SwIfIndex, n))
		}
	}
	for _, n := range names {
		b := bs[n]
		if _, err := interfaces.NewServiceClient(conn).SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{SwIfIndex: interface_types.InterfaceIndex(b.idx), DelAll: true}); err != nil {
			t.Fatalf("loss: sw_interface_add_del_address del_all %s: %v", n, err)
		}
		ev = append(ev, fmt.Sprintf("sw_interface_add_del_address %s del_all → ok", n))
		if _, err := svc.BondDelete(ctx, &bondapi.BondDelete{SwIfIndex: interface_types.InterfaceIndex(b.idx)}); err != nil {
			t.Fatalf("loss: bond_delete %s: %v", n, err)
		}
		ev = append(ev, fmt.Sprintf("bond_delete %s (sw_if_index %d) → ok", n, b.idx))
	}
	return ev
}

// ---- vppctl (evidence only; fixed `show` commands, no user input; never `trace`, D-128) ---------------------------

func vppctl(t *testing.T, args ...string) string {
	t.Helper()
	if len(args) == 0 || args[0] != "show" {
		t.Fatalf("vppctl %v: this test only runs show commands", args)
	}
	for _, a := range args {
		if strings.Contains(a, "trace") {
			t.Fatalf("vppctl %v: trace is forbidden on the shared VPP (D-128)", args)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "vppctl", args...).CombinedOutput() //nolint:gosec // fixed show commands
	if err != nil {
		t.Fatalf("vppctl %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// section returns the lines of a `show bond details` output that belong to bond name.
func section(out, name string) string {
	var b strings.Builder
	in := false
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "BondEthernet") {
			in = strings.HasPrefix(l, name)
		}
		if in {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}
