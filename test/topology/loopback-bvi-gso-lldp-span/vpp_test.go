package lbgs

// Direct VPP access of the test (never of the product API): binary API dumps for the evidence, the ERSPAN GRE fixture
// and the simulated loss; vppctl for what the acceptance asks to see. Only objects of this slot (w<N>) are touched.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	govpp "go.fd.io/govpp"
	vppapi "go.fd.io/govpp/api"

	"ngfw/agent/binapi/feature"
	greapi "ngfw/agent/binapi/gre"
	gsoapi "ngfw/agent/binapi/gso"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	l2api "ngfw/agent/binapi/l2"
	lldpapi "ngfw/agent/binapi/lldp"
	spanapi "ngfw/agent/binapi/span"
	"ngfw/agent/binapi/tunnel_types"
	"ngfw/agent/binapi/vlib"
)

const apiSocket = "/run/vpp/api.sock"

const noIndex = ^uint32(0)

type vppConn = vppapi.Connection

func connectVPP(t *testing.T) vppapi.Connection {
	t.Helper()
	conn, err := govpp.Connect(apiSocket)
	if err != nil {
		t.Fatalf("govpp connect: %v", err)
	}
	t.Cleanup(conn.Disconnect)
	return conn
}

type vppIf struct {
	idx  uint32
	name string
	tag  string
}

func ctx10() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func dumpIfs(t *testing.T, conn vppConn) map[string]vppIf {
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
		out[n] = vppIf{idx: uint32(d.SwIfIndex), name: n, tag: strings.TrimRight(d.Tag, "\x00")}
	}
	return out
}

// ---- mirror / GSO / LLDP state -------------------------------------------------------------------

type spanEntry struct {
	from, to uint32
	state    spanapi.SpanState
	l2       bool
}

func spans(t *testing.T, conn vppConn) []spanEntry {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	var out []spanEntry
	for _, l2 := range []bool{false, true} {
		stream, err := spanapi.NewServiceClient(conn).SwInterfaceSpanDump(ctx, &spanapi.SwInterfaceSpanDump{IsL2: l2})
		if err != nil {
			t.Fatal(err)
		}
		for {
			d, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, spanEntry{uint32(d.SwIfIndexFrom), uint32(d.SwIfIndexTo), d.State, d.IsL2})
		}
	}
	return out
}

// spansFrom returns the sessions whose source is idx, as "<to name>/<device|l2>" → state.
func spansFrom(t *testing.T, conn vppConn, ifs map[string]vppIf, idx uint32) map[string]spanapi.SpanState {
	t.Helper()
	byIdx := map[uint32]string{}
	for n, i := range ifs {
		byIdx[i.idx] = n
	}
	out := map[string]spanapi.SpanState{}
	for _, s := range spans(t, conn) {
		if s.from != idx {
			continue
		}
		level := "device"
		if s.l2 {
			level = "l2"
		}
		name := byIdx[s.to]
		if name == "" {
			name = "#" + strconv.FormatUint(uint64(s.to), 10)
		}
		out[name+"/"+level] = s.state
	}
	return out
}

func gsoOn(t *testing.T, conn vppConn, idx uint32) bool {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	r, err := feature.NewServiceClient(conn).FeatureIsEnabled(ctx, &feature.FeatureIsEnabled{ArcName: "ip4-output", FeatureName: "gso-ip4", SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		t.Fatal(err)
	}
	return r.IsEnabled
}

// lldpOn lists the sw_if_indexes lldp_dump reports (LLDP enabled), cursor-paginated.
func lldpOn(t *testing.T, conn vppConn) map[uint32]bool {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	out := map[uint32]bool{}
	cursor := uint32(0)
	for page := 0; page < 64; page++ {
		stream, err := lldpapi.NewServiceClient(conn).LldpDump(ctx, &lldpapi.LldpDump{Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		var reply *lldpapi.LldpDumpReply
		for {
			d, rep, err := stream.Recv()
			if rep != nil {
				reply = rep
			}
			if d != nil {
				out[uint32(d.SwIfIndex)] = true
			}
			if err == nil {
				continue
			}
			if errors.Is(err, vppapi.EAGAIN) && reply != nil {
				break
			}
			if errors.Is(err, io.EOF) {
				return out
			}
			t.Fatalf("lldp_dump: %v", err)
		}
		cursor = reply.Cursor
	}
	return out
}

// hwIndex reads the hardware interface index of name — TEST-ONLY V20 guard (VPP 26.06 sw_interface_set_lldp uses
// the sw_if_index as a hw_if_index; the binary API exposes no hw index): `show hardware-interfaces brief <name>`
// through cli_inband, a fixed command (df7test.HwIndex does the same).
func hwIndex(t *testing.T, conn vppConn, name string) (uint32, bool) {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	rep, err := vlib.NewServiceClient(conn).CliInband(ctx, &vlib.CliInband{Cmd: "show hardware-interfaces brief " + name})
	if err != nil {
		t.Fatalf("cli_inband: %v", err)
	}
	for _, line := range strings.Split(rep.Reply, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == name {
			if n, err := strconv.ParseUint(f[1], 10, 32); err == nil {
				return uint32(n), true
			}
		}
	}
	return 0, false
}

type bdInfo struct {
	id      uint32
	tag     string
	bvi     uint32
	members map[uint32]bool
}

func bridgeDomain(t *testing.T, conn vppConn, id uint32) (bdInfo, bool) {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	stream, err := l2api.NewServiceClient(conn).BridgeDomainDump(ctx, &l2api.BridgeDomainDump{BdID: id, SwIfIndex: interface_types.InterfaceIndex(noIndex)})
	if err != nil {
		t.Fatal(err)
	}
	var out bdInfo
	found := false
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, found
		}
		if err != nil {
			t.Fatal(err)
		}
		if d.BdID != id {
			continue
		}
		found = true
		out = bdInfo{id: d.BdID, tag: strings.TrimRight(d.BdTag, "\x00"), bvi: uint32(d.BviSwIfIndex), members: map[uint32]bool{}}
		for _, m := range d.SwIfDetails {
			out.members[uint32(m.SwIfIndex)] = true
		}
	}
}

// ---- ERSPAN GRE fixture ------------------------------------------------------------------------------

// greFixture creates the slot-prefixed ERSPAN GRE tunnel gre<inst> exactly as DF-6's gre.tunnel descriptor does —
// gre_tunnel_add_del_v2 (type erspan, p2p) and the owner tag "<owner>:gre<inst>" (df6.TagInterface) — because
// apps/agent/internal/** cannot be imported from this module (Go's internal rule); the descriptor itself is exercised on
// the host by apps/agent/internal/descriptors/span TestERSPANOnHost. The tunnels domain belongs to F-tunnels: the
// configuration only names the tunnel. Deleted in Cleanup.
func greFixture(t *testing.T, conn vppConn, owner string, inst uint32, src, dst string, session uint16) (string, uint32) {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	name := fmt.Sprintf("gre%d", inst)
	if old, ok := dumpIfs(t, conn)[name]; ok {
		if old.tag != owner+":"+name {
			t.Fatalf("%s exists and is not ours (tag %q)", name, old.tag)
		}
		t.Logf("leftover %s from an earlier run: deleting", name)
		delGre(t, conn, old.idx, inst, src, dst, session)
	}
	tun := greTunnel(inst, src, dst, session)
	rep, err := greapi.NewServiceClient(conn).GreTunnelAddDelV2(ctx, &greapi.GreTunnelAddDelV2{IsAdd: true, Tunnel: tun})
	if err != nil {
		t.Fatalf("gre_tunnel_add_del_v2 %s: %v", name, err)
	}
	if _, err := interfaces.NewServiceClient(conn).SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: rep.SwIfIndex, Tag: owner + ":" + name}); err != nil {
		delGre(t, conn, uint32(rep.SwIfIndex), inst, src, dst, session)
		t.Fatalf("tag %s: %v", name, err)
	}
	idx := uint32(rep.SwIfIndex)
	t.Cleanup(func() { delGre(t, conn, idx, inst, src, dst, session) })
	return name, idx
}

func greTunnel(inst uint32, src, dst string, session uint16) greapi.GreTunnelV2 {
	a := func(s string) ip_types.Address {
		p := netip.MustParseAddr(s).As4()
		return ip_types.Address{Af: ip_types.ADDRESS_IP4, Un: ip_types.AddressUnionIP4(p)}
	}
	return greapi.GreTunnelV2{Type: greapi.GRE_API_TUNNEL_TYPE_ERSPAN, Mode: tunnel_types.TUNNEL_API_MODE_P2P, SessionID: session,
		Instance: inst, SwIfIndex: interface_types.InterfaceIndex(noIndex), Src: a(src), Dst: a(dst)}
}

func delGre(t *testing.T, conn vppConn, idx, inst uint32, src, dst string, session uint16) {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	tun := greTunnel(inst, src, dst, session)
	tun.SwIfIndex = interface_types.InterfaceIndex(idx)
	if _, err := greapi.NewServiceClient(conn).GreTunnelAddDelV2(ctx, &greapi.GreTunnelAddDelV2{IsAdd: false, Tunnel: tun}); err != nil {
		t.Errorf("delete gre%d: %v", inst, err)
	} else {
		t.Logf("fixture gre%d deleted", inst)
	}
}

// ---- simulated loss (restart-safety) ---------------------------------------------------------------

// loss deletes, behind the stopped agent's back, the feature's objects and then the loopbacks (D-095c: every
// dependent first — mirror sessions, GSO, LLDP, the BVI membership and the bridge domain, the addresses).
func loss(t *testing.T, conn vppConn, bvi, mon string, bdID uint32) []string {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	ifs := dumpIfs(t, conn)
	b, m := ifs[bvi], ifs[mon]
	var ev []string
	for _, s := range spans(t, conn) {
		if s.from != b.idx && s.from != m.idx {
			continue
		}
		if _, err := spanapi.NewServiceClient(conn).SwInterfaceSpanEnableDisable(ctx, &spanapi.SwInterfaceSpanEnableDisable{
			SwIfIndexFrom: interface_types.InterfaceIndex(s.from), SwIfIndexTo: interface_types.InterfaceIndex(s.to), State: spanapi.SPAN_STATE_API_DISABLED, IsL2: s.l2}); err != nil {
			t.Fatalf("span disable: %v", err)
		}
		ev = append(ev, fmt.Sprintf("sw_interface_span_enable_disable %d→%d l2=%v state=disabled → ok", s.from, s.to, s.l2))
	}
	if gsoOn(t, conn, b.idx) {
		if _, err := gsoapi.NewServiceClient(conn).FeatureGsoEnableDisable(ctx, &gsoapi.FeatureGsoEnableDisable{SwIfIndex: interface_types.InterfaceIndex(b.idx)}); err != nil {
			t.Fatalf("gso disable: %v", err)
		}
		ev = append(ev, fmt.Sprintf("feature_gso_enable_disable %s (%d) enable=false → ok", bvi, b.idx))
	}
	if _, ok := bridgeDomain(t, conn, bdID); ok {
		if _, err := l2api.NewServiceClient(conn).SwInterfaceSetL2Bridge(ctx, &l2api.SwInterfaceSetL2Bridge{RxSwIfIndex: interface_types.InterfaceIndex(b.idx), BdID: bdID, PortType: l2api.L2_API_PORT_TYPE_BVI, Enable: false}); err != nil {
			t.Fatalf("BVI → L3: %v", err)
		}
		if _, err := l2api.NewServiceClient(conn).BridgeDomainAddDelV2(ctx, &l2api.BridgeDomainAddDelV2{BdID: bdID, IsAdd: false}); err != nil {
			t.Fatalf("bridge_domain_add_del_v2 del: %v", err)
		}
		ev = append(ev, fmt.Sprintf("sw_interface_set_l2_bridge %s L3 + bridge_domain_add_del_v2 del %d → ok", bvi, bdID))
	}
	for _, n := range []string{bvi, mon} {
		i := ifs[n]
		if _, err := interfaces.NewServiceClient(conn).SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{SwIfIndex: interface_types.InterfaceIndex(i.idx), DelAll: true}); err != nil {
			t.Fatalf("del addresses %s: %v", n, err)
		}
		if _, err := interfaces.NewServiceClient(conn).DeleteLoopback(ctx, &interfaces.DeleteLoopback{SwIfIndex: interface_types.InterfaceIndex(i.idx)}); err != nil {
			t.Fatalf("delete_loopback %s: %v", n, err)
		}
		ev = append(ev, fmt.Sprintf("delete_loopback %s (sw_if_index %d, tag %q) → ok", n, i.idx, i.tag))
	}
	return ev
}

// ---- vppctl (evidence only; fixed commands, no user input) ----------------------------------------

func vppctl(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "vppctl", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("vppctl %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// alignedLoopback returns an UNTAGGED loopback whose sw_if_index equals its hw_if_index (VPP 26.06
// sw_interface_set_lldp uses the sw index as a hw index, V20). VPP takes both indexes from per-pool free lists (last
// freed first), and other slots' sub-interfaces take software indexes only, so a new loopback's two indexes drift apart
// on the shared host. The walk: create probe loop<slot>8x → (sw s, hw h); aligned → done. Otherwise delete it (s and h
// are on top of their free lists again) and take software indexes only — VLAN sub-interfaces of a holder loopback
// loop<slot>89 — until the one taken is h; give that one back (h is on top of both lists now) and create the probe
// again. Tag the result right before the configuration names it (the agent then adopts it by its owner tag); the
// holder and its sub-interfaces are deleted in Cleanup (untagged: the agent never touches them).
func alignedLoopback(t *testing.T, conn vppConn, slot int) (string, uint32, bool) {
	t.Helper()
	svc := interfaces.NewServiceClient(conn)
	mk := func(inst int) (string, uint32) {
		ctx, cancel := ctx10()
		defer cancel()
		name := fmt.Sprintf("loop%d", inst)
		if old, ok := dumpIfs(t, conn)[name]; ok {
			t.Logf("leftover %s (sw_if_index %d, tag %q): deleting", name, old.idx, old.tag)
			delLoop(t, conn, old.idx)
		}
		rep, err := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: uint32(inst)}) //nolint:gosec // slot range
		if err != nil {
			t.Fatalf("create_loopback_instance %s: %v", name, err)
		}
		return name, uint32(rep.SwIfIndex)
	}
	_, holder := mk(slot*100 + 89)
	t.Cleanup(func() { delLoop(t, conn, holder) }) // registered before the sub-interfaces: runs after them
	vlan := uint32(100)
	for try := 0; try < 6; try++ {
		name, s := mk(slot*100 + 80 + try)
		h, found := hwIndex(t, conn, name)
		t.Logf("LLDP probe %s: sw_if_index %d hw_if_index %d (found %v)", name, s, h, found)
		if found && h == s {
			return name, s, true
		}
		delLoop(t, conn, s)
		if !found {
			continue
		}
		for k := 0; k < 48; k++ {
			ctx, cancel := ctx10()
			sub, err := svc.CreateSubif(ctx, &interfaces.CreateSubif{SwIfIndex: interface_types.InterfaceIndex(holder), SubID: vlan, OuterVlanID: uint16(vlan),
				SubIfFlags: interface_types.SUB_IF_API_FLAG_ONE_TAG | interface_types.SUB_IF_API_FLAG_EXACT_MATCH})
			cancel()
			if err != nil {
				t.Fatalf("create_subif on the holder: %v", err)
			}
			vlan++
			v := uint32(sub.SwIfIndex)
			if v == h {
				delSubif(t, conn, v) // h back on top of the software free list
				break
			}
			t.Cleanup(func() { delSubif(t, conn, v) })
		}
		name, s = mk(slot*100 + 80 + try)
		h, found = hwIndex(t, conn, name)
		t.Logf("LLDP probe %s (after taking software indexes): sw_if_index %d hw_if_index %d (found %v)", name, s, h, found)
		if found && h == s {
			return name, s, true
		}
		delLoop(t, conn, s)
	}
	return "", 0, false
}

func delLoop(t *testing.T, conn vppConn, idx uint32) {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	if _, err := interfaces.NewServiceClient(conn).DeleteLoopback(ctx, &interfaces.DeleteLoopback{SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
		t.Errorf("delete_loopback %d: %v", idx, err)
	}
}

func delSubif(t *testing.T, conn vppConn, idx uint32) {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	if _, err := interfaces.NewServiceClient(conn).DeleteSubif(ctx, &interfaces.DeleteSubif{SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
		t.Errorf("delete_subif %d: %v", idx, err)
	}
}

// tagOwner stamps idx with the owner tag the agent's loopback creator writes ("<owner>:<name>").
func tagOwner(t *testing.T, conn vppConn, owner, name string, idx uint32) {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	if _, err := interfaces.NewServiceClient(conn).SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: interface_types.InterfaceIndex(idx), Tag: owner + ":" + name}); err != nil {
		t.Fatalf("tag %s: %v", name, err)
	}
}

// lldpOff disables LLDP on idx behind the agent's back (its loopback stays: re-created it could lose the index
// alignment LLDP needs, V20).
func lldpOff(t *testing.T, conn vppConn, idx uint32) {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	if _, err := lldpapi.NewServiceClient(conn).SwInterfaceSetLldp(ctx, &lldpapi.SwInterfaceSetLldp{SwIfIndex: interface_types.InterfaceIndex(idx), MgmtOid: make([]byte, 128)}); err != nil {
		t.Fatalf("lldp disable: %v", err)
	}
}
