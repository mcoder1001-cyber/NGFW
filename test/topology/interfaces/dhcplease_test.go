package interfaces

// TD-24 host proof (F-kea review Q5, rated H): the agent's interface-ip reconcile must not delete the address VPP's
// DHCPv4 client leased. The real vrx-agent of this tree (owner = prefix, no API/DB needed) against the REAL host VPP:
//
//	netns ns-<p>-dhcp: veth <p>d1 (+ VLAN <p>d1.100, 10.<N>.100.1/24) and a slot-local dnsmasq on <p>d1.100 (never the
//	                   host's dnsmasq/kea units; no router option, so VPP's client installs no default route)
//	VPP:               host-<p>d0 (af_packet on <p>d0, created by the agent) and its VLAN 100 sub-interface
//	                   host-<p>d0.100 in the slot VRF <p>-td24 (table <N>024) with `dhcpClient` and a static IPv6
//
//	TestDHCPLeaseSurvivesResync
//	  bind     apply the document (vrx-agentctl) → VPP's client reaches BOUND (dhcp_client_dump) → the lease is on the
//	           sub-interface (ip_address_dump, vppctl show interface address)
//	  proof    the same document applied again (a commit's reconcile) → the agent restarted twice (each start is a full
//	           resync from its stored desired state): after every step the lease is still installed, the client still
//	           BOUND on the same address, the agent log never names the lease's interface-ip key, and the agent's
//	           Retrieve never reports it (it is state, not configuration)
//	  cleanup  dhcpClient removed (VPP releases the lease) → everything removed (veth down first, D-101) → nothing with
//	           the prefix is left in VPP; dnsmasq and the agent stopped by PID; netns and veth deleted
//
// The names do not collide with the P08 rig (ns-<p>-lan|wan, <p>l*/<p>w*), so the test also runs beside it in
// `tools/ci.sh full`. Runs only with VRX_INTEGRATION=1, as root, with a slot prefix (w<N>), under flock -s on the lab
// lock; NRestarts of vpp is checked before and after. Never a packet trace (D-128), no classify writes at all.
//
//	eval "$(tools/lab env <slot>)"; cd test/topology/interfaces
//	VRX_INTEGRATION=1 ../../../tools/lab lock shared go test -count=1 -v -run TestDHCPLeaseSurvivesResync .

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	vppapi "go.fd.io/govpp/api"

	"ngfw/agent/binapi/dhcp"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
)

const dnsmasqBin = "/usr/sbin/dnsmasq"

type leaseTopo struct {
	s              slot
	ns, dev, peer  string // ns-<p>-dhcp, <p>d0 (root side, VPP af_packet), <p>d1 (netns side)
	vlan           string // <p>d1.100
	parent, sub    string // host-<p>d0, host-<p>d0.100
	vrf            string
	vrfID          uint32
	serverIP       string
	poolLo, poolHi string
	subnet         netip.Prefix
	v6             string
	work           string // /run/vrx-test/<p>/td24 (0700)
	bin, ctl       string // vrx-agent, vrx-agentctl
	agentEnv       []string
	agentLog       string
	agent          *proc
}

func newLeaseTopo(t *testing.T, s slot) *leaseTopo {
	t.Helper()
	p, n := s.prefix, strconv.Itoa(s.num)
	tp := &leaseTopo{
		s: s, ns: "ns-" + p + "-dhcp", dev: p + "d0", peer: p + "d1", vlan: p + "d1.100",
		parent: "host-" + p + "d0", sub: "host-" + p + "d0.100",
		vrf: p + "-td24", vrfID: uint32(s.num*1000 + 24), //nolint:gosec // slot 1–12
		serverIP: "10." + n + ".100.1", poolLo: "10." + n + ".100.50", poolHi: "10." + n + ".100.59",
		subnet: netip.MustParsePrefix("10." + n + ".100.0/24"), v6: "2001:db8:" + n + ":100::1/64",
		work: filepath.Join(s.runDir, "td24"),
	}
	if err := mkdirShared(s.runDir); err != nil {
		t.Fatal(err)
	}
	_ = os.RemoveAll(tp.work)
	if err := os.MkdirAll(tp.work, 0o700); err != nil {
		t.Fatal(err)
	}
	bins := t.TempDir() // /run is noexec
	for _, b := range [][2]string{{"vrx-agent", "./cmd/vrx-agent"}, {"vrx-agentctl", "./cmd/vrx-agentctl"}} {
		out, err := run(t, "go", "build", "-C", filepath.Join(s.repo, "apps", "agent"), "-o", filepath.Join(bins, b[0]), b[1])
		if err != nil {
			t.Fatalf("go build %s: %v\n%s", b[0], err, out)
		}
	}
	tp.bin, tp.ctl = filepath.Join(bins, "vrx-agent"), filepath.Join(bins, "vrx-agentctl")
	tp.agentLog = filepath.Join(tp.work, "agent.log")
	tp.agentEnv = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"),
		"VRX_AGENT_SOCKET=" + s.socket, "VRX_OWNER=" + p, "VRX_GLOBALS_OWNER=0", // D-071: test slots never own globals
		"VRX_AGENT_STATE_DIR=" + filepath.Join(tp.work, "agent-state"), "VRX_METRICS_PORT=" + s.metricsPort,
		"VRX_VPP_TABLE_BASE=" + strconv.Itoa(s.num*1000), "VRX_SOCKET_GROUP=root", "VRX_LOG_LEVEL=info"}
	return tp
}

// doc is the configuration document (protobuf JSON, as the API sends it).
func (tp *leaseTopo) doc(withDHCP bool) map[string]any {
	sub := map[string]any{"vlanId": 100, "enabled": true, "vrf": tp.vrf, "ipv6": []string{tp.v6}}
	if withDHCP {
		sub["dhcpClient"] = map[string]any{"hostname": tp.s.prefix + "-td24", "setBroadcastFlag": true}
	}
	return map[string]any{
		"vrfs":       map[string]any{tp.vrf: map[string]any{"id": tp.vrfID}},
		"interfaces": map[string]any{tp.parent: map[string]any{"enabled": true, "subinterfaces": map[string]any{"100": sub}}},
	}
}

// ctlJSON runs vrx-agentctl (fixed argv) and decodes its protobuf-JSON output.
func (tp *leaseTopo) ctlJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	out, err := run(t, tp.ctl, append([]string{"-s", tp.s.socket}, args...)...)
	if err != nil {
		t.Fatalf("vrx-agentctl %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("vrx-agentctl %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return m
}

// apply sends doc through the agent's Apply RPC and requires APPLIED; it returns the object results.
func (tp *leaseTopo) apply(t *testing.T, name string, doc map[string]any, subsystems ...string) []any {
	t.Helper()
	path := filepath.Join(tp.work, name+".json")
	raw, _ := json.Marshal(doc)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"apply", path, "-txn", "td24-" + name}
	if len(subsystems) > 0 {
		args = append(args, "-subsystems", strings.Join(subsystems, ","))
	}
	r := tp.ctlJSON(t, args...)
	res, _ := r["results"].([]any)
	if r["status"] != "APPLY_STATUS_APPLIED" {
		t.Fatalf("apply %s: %s", name, js(r))
	}
	return res
}

func (tp *leaseTopo) startAgent(t *testing.T) {
	t.Helper()
	from := fileSize(tp.agentLog)
	tp.agent = start(t, "vrx-agent", tp.agentLog, tp.agentEnv, tp.bin)
	if !waitFor(60*time.Second, func() bool {
		if tp.agent.exited() {
			return true
		}
		lines, _ := readAgentLog(t, tp.agentLog, from)
		for _, l := range lines {
			if l.Msg == "resync finished" {
				return true
			}
		}
		return false
	}) || tp.agent.exited() {
		raw, _ := os.ReadFile(tp.agentLog) //nolint:gosec // our own log
		t.Fatalf("vrx-agent did not finish its start-up resync:\n%s", raw)
	}
	lines, raw := readAgentLog(t, tp.agentLog, from)
	for i, l := range lines {
		if strings.Contains(l.Msg, "resync") || strings.Contains(l.Msg, "VPP boot identity") {
			t.Log("agent log: " + trunc(raw[i], 400))
		}
		if l.Msg == "resync finished" && l.Status != "APPLY_STATUS_APPLIED" {
			t.Fatalf("start-up resync did not apply: %s", raw[i])
		}
	}
}

// retrievedSub returns the agent's Retrieve of the VLAN 100 sub-interface (protobuf JSON) — "" when it is absent.
func (tp *leaseTopo) retrievedSub(t *testing.T) string {
	t.Helper()
	var r struct {
		DesiredState struct {
			Interfaces map[string]struct {
				Subinterfaces map[string]json.RawMessage `json:"subinterfaces"`
			} `json:"interfaces"`
		} `json:"desiredState"`
	}
	out, err := run(t, tp.ctl, "-s", tp.s.socket, "retrieve", "-subsystems", "interfaces")
	if err != nil {
		t.Fatalf("vrx-agentctl retrieve: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("vrx-agentctl retrieve: %v\n%s", err, out)
	}
	return string(r.DesiredState.Interfaces[tp.parent].Subinterfaces["100"])
}

// tableExists reports whether VPP has FIB table id (either family; ip_table_dump, read-only).
func tableExists(t *testing.T, conn vppapi.Connection, id uint32) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := ip.NewServiceClient(conn).IPTableDump(ctx, &ip.IPTableDump{})
	if err != nil {
		t.Fatalf("ip_table_dump: %v", err)
	}
	found := false
	for {
		d, err := st.Recv()
		if errors.Is(err, io.EOF) {
			return found
		}
		if err != nil {
			t.Fatalf("ip_table_dump: %v", err)
		}
		found = found || d.Table.TableID == id
	}
}

// netns side: veth <p>d0 ↔ <p>d1 (ns-<p>-dhcp), VLAN 100 on <p>d1 with the server address. Everything stays down until
// the agent created (and sanitized, D-095) the VPP side.
func (tp *leaseTopo) up(t *testing.T) {
	t.Helper()
	if _, err := run(t, "ip", "netns", "exec", tp.ns, "true"); err == nil {
		t.Fatalf("netns %s exists (leftover of an earlier run: ip netns del %s)", tp.ns, tp.ns)
	}
	if _, err := run(t, "ip", "link", "show", tp.dev); err == nil {
		t.Fatalf("veth %s exists (leftover of an earlier run: ip link del %s)", tp.dev, tp.dev)
	}
	t.Cleanup(func() {
		_, _ = run(t, "ip", "netns", "del", tp.ns)
		_, _ = run(t, "ip", "link", "del", tp.dev)
		t.Logf("netns %s and veth %s deleted", tp.ns, tp.dev)
	})
	mustRun(t, "ip", "netns", "add", tp.ns)
	mustRun(t, "ip", "link", "add", tp.dev, "type", "veth", "peer", "name", tp.peer)
	mustRun(t, "ip", "link", "set", tp.peer, "netns", tp.ns)
	_, _ = run(t, "sysctl", "-qw", "net.ipv6.conf."+tp.dev+".disable_ipv6=1") // keep RS/MLD noise out of VPP
	mustRun(t, "ip", "-n", tp.ns, "link", "set", "lo", "up")
	mustRun(t, "ip", "-n", tp.ns, "link", "add", "link", tp.peer, "name", tp.vlan, "type", "vlan", "id", "100") // 802.1q only (D-126)
	for _, d := range []string{tp.peer, tp.vlan} {
		_, _ = run(t, "ip", "netns", "exec", tp.ns, "sysctl", "-qw", "net.ipv6.conf."+d+".disable_ipv6=1")
	}
	_, _ = run(t, "ip", "netns", "exec", tp.ns, "ethtool", "-K", tp.peer, "tx", "off") // valid UDP checksums into af_packet
	mustRun(t, "ip", "-n", tp.ns, "addr", "add", tp.serverIP+"/24", "dev", tp.vlan)
}

func (tp *leaseTopo) links(t *testing.T, up bool) {
	t.Helper()
	st := "down"
	if up {
		st = "up"
	}
	mustRun(t, "ip", "link", "set", tp.dev, st)
	mustRun(t, "ip", "-n", tp.ns, "link", "set", tp.peer, st)
	mustRun(t, "ip", "-n", tp.ns, "link", "set", tp.vlan, st)
}

// dnsmasq: DHCP only (port 0), bound to the VLAN device in the slot's netns, no router/DNS option, own lease and pid
// files; stopped by PID.
func (tp *leaseTopo) dnsmasq(t *testing.T) *proc {
	t.Helper()
	if _, err := os.Stat(dnsmasqBin); err != nil {
		t.Skipf("%s not installed", dnsmasqBin)
	}
	p := start(t, "dnsmasq", filepath.Join(tp.work, "dnsmasq.log"), []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"},
		"/usr/sbin/ip", "netns", "exec", tp.ns, dnsmasqBin, "--keep-in-foreground", "--conf-file=/dev/null", "--port=0",
		"--no-resolv", "--no-hosts", "--bind-interfaces", "--interface="+tp.vlan, "--except-interface=lo",
		"--dhcp-range="+tp.poolLo+","+tp.poolHi+",255.255.255.0,10m", "--dhcp-option=3", "--dhcp-option=6",
		"--dhcp-authoritative", "--dhcp-leasefile="+filepath.Join(tp.work, "dnsmasq.leases"),
		"--pid-file="+filepath.Join(tp.work, "dnsmasq.pid"), "--user=root", "--group=root", "--log-dhcp", "--log-facility=-")
	t.Cleanup(func() { p.stop(t) })
	if !waitFor(10*time.Second, func() bool {
		raw, _ := os.ReadFile(filepath.Join(tp.work, "dnsmasq.log")) //nolint:gosec // our own log
		return p.exited() || strings.Contains(string(raw), "DHCP, IP range")
	}) || p.exited() {
		raw, _ := os.ReadFile(filepath.Join(tp.work, "dnsmasq.log")) //nolint:gosec // our own log
		t.Fatalf("dnsmasq did not start:\n%s", raw)
	}
	return p
}

// dhcpClientOf returns the DHCPv4 client of sw_if_index idx from dhcp_client_dump (read-only).
func dhcpClientOf(t *testing.T, conn vppapi.Connection, idx uint32) (*dhcp.DHCPClientDetails, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := dhcp.NewServiceClient(conn).DHCPClientDump(ctx, &dhcp.DHCPClientDump{})
	if err != nil {
		t.Fatalf("dhcp_client_dump: %v", err)
	}
	var found *dhcp.DHCPClientDetails
	for {
		d, err := st.Recv()
		if errors.Is(err, io.EOF) {
			return found, found != nil
		}
		if err != nil {
			t.Fatalf("dhcp_client_dump: %v", err)
		}
		if uint32(d.Client.SwIfIndex) == idx {
			found = d
		}
	}
}

func leaseOf(d *dhcp.DHCPClientDetails) (string, string) {
	if d == nil {
		return "", "none"
	}
	a := netip.AddrFrom4(d.Lease.HostAddress.Un.GetIP4())
	st := strings.TrimPrefix(d.Lease.State.String(), "DHCP_CLIENT_STATE_API_")
	if a.IsUnspecified() {
		return "", st
	}
	return netip.PrefixFrom(a, int(d.Lease.MaskWidth)).String(), st
}

// addrsOf lists the IPv4 and IPv6 addresses of sw_if_index idx (ip_address_dump, read-only).
func addrsOf(t *testing.T, conn vppapi.Connection, idx uint32) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out []string
	for _, v6 := range []bool{false, true} {
		st, err := ip.NewServiceClient(conn).IPAddressDump(ctx, &ip.IPAddressDump{SwIfIndex: interface_types.InterfaceIndex(idx), IsIPv6: v6})
		if err != nil {
			t.Fatalf("ip_address_dump: %v", err)
		}
		for {
			d, err := st.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("ip_address_dump: %v", err)
			}
			out = append(out, d.Prefix.String())
		}
	}
	return out
}

func TestDHCPLeaseSurvivesResync(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("TD-24 host proof: set VRX_INTEGRATION=1 (host VPP, netns, dnsmasq)")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (netns, veth, VPP API socket)")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	restarts0 := nRestarts(t)
	t.Logf("systemctl show vpp -p NRestarts (before) = %d", restarts0)
	t.Cleanup(func() {
		n := nRestarts(t)
		t.Logf("systemctl show vpp -p NRestarts (after) = %d", n)
		if n != restarts0 {
			t.Errorf("VPP restarted during the test: NRestarts %d → %d", restarts0, n)
		}
	})
	conn := connectVPP(t)
	tp := newLeaseTopo(t, s)
	tp.up(t) // registers the netns/veth deletion first: it runs last
	tp.dnsmasq(t)
	tp.startAgent(t)
	t.Cleanup(func() { tp.agent.stop(t) })
	cleaned := false
	t.Cleanup(func() { // on a failure: take everything out of VPP through the agent (veth down first, D-101)
		if cleaned || tp.agent.exited() {
			return
		}
		_, _ = run(t, "ip", "link", "set", tp.dev, "down")
		path := filepath.Join(tp.work, "cleanup.json")
		_ = os.WriteFile(path, []byte(`{}`), 0o600)
		out, err := run(t, tp.ctl, "-s", s.socket, "apply", path, "-txn", "td24-cleanup-on-failure", "-subsystems", "interfaces,vrfs")
		t.Logf("cleanup on failure: %v\n%s", err, trunc(out, 2000))
	})

	var lease string
	var subIdx uint32
	t.Run("bind", func(t *testing.T) {
		tp.apply(t, "rev1", tp.doc(true))
		ifs := dumpIfs(t, conn)
		sub, ok := ifs[tp.sub]
		if !ok || sub.tag != s.prefix+":"+tp.sub {
			t.Fatalf("%s not created by the agent (tag %q)", tp.sub, sub.tag)
		}
		subIdx = sub.idx
		t.Logf("VPP: %s sw_if_index=%d tag=%q, %s sw_if_index=%d tag=%q", tp.parent, ifs[tp.parent].idx, ifs[tp.parent].tag, tp.sub, sub.idx, sub.tag)
		_, raw := readAgentLog(t, tp.agentLog, 0)
		for _, l := range raw {
			if strings.Contains(l, "interface sanitized") {
				t.Log("agent log (D-095 sanitizer on create): " + trunc(l, 300))
			}
		}
		tp.links(t, true) // the agent created and sanitized both interfaces: packets may flow now
		t0 := time.Now()
		var state string
		if !waitFor(90*time.Second, func() bool {
			d, _ := dhcpClientOf(t, conn, subIdx)
			lease, state = leaseOf(d)
			return state == "BOUND" && lease != ""
		}) {
			raw, _ := os.ReadFile(filepath.Join(tp.work, "dnsmasq.log")) //nolint:gosec // our own log
			t.Fatalf("VPP's DHCP client on %s did not bind within 90 s (state %s):\n%s\nvppctl show dhcp client:\n%s", tp.sub, state, raw, vppctl(t, "show", "dhcp", "client"))
		}
		p := netip.MustParsePrefix(lease)
		if !tp.subnet.Contains(p.Addr()) || p.Bits() != 24 {
			t.Fatalf("lease %s is not from %s", lease, tp.subnet)
		}
		t.Logf("dhcp_client_dump %s: state BOUND, lease %s after %.1fs", tp.sub, lease, time.Since(t0).Seconds())
		dlog, _ := os.ReadFile(filepath.Join(tp.work, "dnsmasq.log")) //nolint:gosec // our own log
		for _, l := range strings.Split(string(dlog), "\n") {
			if strings.Contains(l, "DHCPDISCOVER") || strings.Contains(l, "DHCPOFFER") || strings.Contains(l, "DHCPREQUEST") || strings.Contains(l, "DHCPACK") {
				t.Log("dnsmasq: " + l)
			}
		}
		t.Logf("ip_address_dump %s: %v", tp.sub, addrsOf(t, conn, subIdx))
		t.Log("vppctl show dhcp client:\n" + vppctl(t, "show", "dhcp", "client"))
		t.Log("vppctl show interface address " + tp.sub + ":\n" + vppctl(t, "show", "interface", "address", tp.sub))
	})
	if t.Failed() {
		return
	}

	leaseKey := "interface-ip/" + tp.sub + "/" + lease
	check := func(t *testing.T, step string, logFrom int64) {
		t.Helper()
		addrs := addrsOf(t, conn, subIdx)
		d, _ := dhcpClientOf(t, conn, subIdx)
		got, state := leaseOf(d)
		t.Logf("%s: ip_address_dump %s = %v; dhcp_client_dump state %s lease %s", step, tp.sub, addrs, state, got)
		if !strings.Contains(strings.Join(addrs, " "), lease) || state != "BOUND" || got != lease {
			t.Fatalf("%s: the lease %s is gone (addresses %v, client %s %s)", step, lease, addrs, state, got)
		}
		if !strings.Contains(strings.Join(addrs, " "), tp.v6) {
			t.Fatalf("%s: the static %s is gone: %v", step, tp.v6, addrs)
		}
		_, raw := readAgentLog(t, tp.agentLog, logFrom)
		for _, l := range raw {
			if strings.Contains(l, leaseKey) {
				t.Fatalf("%s: the agent acted on the lease: %s", step, l)
			}
		}
		sub := tp.retrievedSub(t)
		t.Logf("%s: agent Retrieve %s/subinterfaces/100 = %s", step, tp.parent, sub)
		if strings.Contains(sub, lease) || !strings.Contains(sub, tp.v6) || !strings.Contains(sub, `"dhcpClient"`) {
			t.Fatalf("%s: Retrieve %s", step, sub)
		}
	}

	t.Run("proof", func(t *testing.T) {
		from := fileSize(tp.agentLog)
		res := tp.apply(t, "rev1-again", tp.doc(true))
		t.Logf("re-apply of the same document (a commit's reconcile): %d object results %s", len(res), js(res))
		if len(res) != 0 {
			t.Errorf("re-apply changed %s", js(res))
		}
		check(t, "after re-apply", from)
		for i := 1; i <= 2; i++ {
			tp.agent.stop(t)
			from = fileSize(tp.agentLog)
			tp.startAgent(t)
			check(t, fmt.Sprintf("after agent restart %d (start-up resync)", i), from)
		}
		t.Log("vppctl show interface address " + tp.sub + " (after both resyncs):\n" + vppctl(t, "show", "interface", "address", tp.sub))
		t.Log("vppctl show dhcp client (after both resyncs):\n" + vppctl(t, "show", "dhcp", "client"))
	})

	t.Run("cleanup", func(t *testing.T) {
		tp.apply(t, "no-dhcp", tp.doc(false))
		addrs := addrsOf(t, conn, subIdx)
		t.Logf("dhcpClient removed: ip_address_dump %s = %v (VPP released the lease)", tp.sub, addrs)
		if strings.Contains(strings.Join(addrs, " "), lease) {
			t.Errorf("lease %s still on %s after the client was deleted", lease, tp.sub)
		}
		tp.links(t, false) // D-101: the agent deletes the af_packet interface only with its veth down
		tp.apply(t, "empty", map[string]any{}, "interfaces", "vrfs")
		for n := range dumpIfs(t, conn) {
			if strings.HasPrefix(n, tp.parent) {
				t.Errorf("%s still in VPP after the delete", n)
			}
		}
		if tableExists(t, conn, tp.vrfID) {
			t.Errorf("table %d still in VPP after the delete", tp.vrfID)
		}
		t.Logf("after cleanup: no %s* interface and no table %d in VPP", tp.parent, tp.vrfID)
		cleaned = true
	})
}
