// Package smoke proves the P04 development environment end to end against the REAL VPP on this host:
// govpp connects to /run/vpp/api.sock, the generated bindings (apps/agent/binapi) answer show_version == 26.06,
// `tools/lab rig up <prefix>` builds the veth/netns rig on af_packet host-interfaces, a ping crosses VPP from the
// LAN namespace to the WAN namespace, the rx counters of both host-interfaces (stats segment) increase, and
// `rig down` leaves no object carrying the prefix behind.
//
// Runs only with VRX_INTEGRATION=1 (00-CONTEXT: unit runs must not touch VPP), as root, under the shared lab lock
// (docs/lab/shared-host-rules.md §1b). Every object carries VRX_TEST_PREFIX. Path recorded: af_packet (D-010).
package smoke

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	govpp "go.fd.io/govpp"
	"go.fd.io/govpp/adapter/statsclient"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/vpe"
)

const (
	apiSocket   = "/run/vpp/api.sock"
	statsSocket = "/run/vpp/stats.sock"
	labLock     = "/run/lock/vrx-lab.lock"
	wantVersion = "26.06"
	dataPath    = "af_packet"
)

func TestPingThroughVPPOverAfPacketRig(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("integration test: set VRX_INTEGRATION=1 to run against the host VPP (path af_packet)")
	}
	prefix := os.Getenv("VRX_TEST_PREFIX")
	if prefix == "" {
		t.Fatal("VRX_TEST_PREFIX must be set (your slot prefix, e.g. w3): every rig object carries it")
	}
	if os.Geteuid() != 0 {
		t.Fatal("must run as root: netns/veth creation and the VPP sockets (root:vpp) need it")
	}
	lab := filepath.Join(repoRoot(t), "tools", "lab")
	if _, err := os.Stat(lab); err != nil {
		t.Fatalf("tools/lab not found: %v", err)
	}

	sharedLock(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	// 1. govpp + generated bindings: show_version must be the pinned 26.06.
	conn, err := govpp.Connect(apiSocket)
	if err != nil {
		t.Fatalf("govpp connect %s: %v", apiSocket, err)
	}
	t.Cleanup(conn.Disconnect)
	ver, err := vpe.NewServiceClient(conn).ShowVersion(ctx, &vpe.ShowVersion{})
	if err != nil {
		t.Fatalf("show_version: %v", err)
	}
	t.Logf("vpp: program=%s version=%s built=%s", ver.Program, ver.Version, ver.BuildDate)
	if !strings.Contains(ver.Version, wantVersion) {
		t.Fatalf("show_version = %q, want it to contain %q", ver.Version, wantVersion)
	}

	// 2. rig up (cleanup: rig down — idempotent, so the explicit down at the end makes it a no-op).
	up := runLab(t, lab, "rig", "up", prefix)
	t.Cleanup(func() { _, _ = exec.Command(lab, "rig", "down", prefix).CombinedOutput() })
	rig := parseKV(up)
	for _, k := range []string{"path", "lan_ns", "wan_ns", "wan_ip", "vpp_lan_if", "vpp_wan_if", "lan_veth", "wan_veth"} {
		if rig[k] == "" {
			t.Fatalf("rig up output lacks %q:\n%s", k, up)
		}
	}
	if rig["path"] != dataPath {
		t.Fatalf("rig path = %q, want %q", rig["path"], dataPath)
	}
	lanIf, wanIf := rig["vpp_lan_if"], rig["vpp_wan_if"]
	wanIP := strings.SplitN(rig["wan_ip"], "/", 2)[0]

	// 3. both host-interfaces exist in VPP (sw_interface_dump through the generated bindings).
	ifs := dumpInterfaces(ctx, t, conn)
	for _, name := range []string{lanIf, wanIf} {
		if _, ok := ifs[name]; !ok {
			t.Fatalf("VPP has no interface %q after rig up; have %v", name, keys(ifs))
		}
	}
	t.Logf("vpp interfaces: %s=%d %s=%d", lanIf, ifs[lanIf], wanIf, ifs[wanIf])

	// 4. rx counters before, ping LAN ns → WAN ns through VPP, rx counters after.
	before := rxPackets(t, lanIf, wanIf)
	ping := exec.Command("ip", "netns", "exec", rig["lan_ns"], "ping", "-c", "4", "-i", "0.3", "-W", "2", wanIP)
	pout, err := ping.CombinedOutput()
	t.Logf("ping %s → %s through VPP:\n%s", rig["lan_ns"], wanIP, strings.TrimSpace(string(pout)))
	if err != nil {
		t.Fatalf("ping through VPP failed: %v", err)
	}
	after := rxPackets(t, lanIf, wanIf)
	for _, name := range []string{lanIf, wanIf} {
		if after[name] <= before[name] {
			t.Fatalf("rx packets on %s did not increase: before=%d after=%d", name, before[name], after[name])
		}
		t.Logf("rx packets %s: %d → %d", name, before[name], after[name])
	}

	// 5. rig down leaves nothing with our prefix: no netns, no veth, no VPP host-interface.
	runLab(t, lab, "rig", "down", prefix)
	if left := leftovers(ctx, t, conn, prefix); len(left) > 0 {
		t.Fatalf("rig down left prefixed objects behind: %v", left)
	}
	t.Logf("path: %s · prefix: %s · rig down clean", dataPath, prefix)
}

// sharedLock takes flock -s on the lab lock for the duration of the test (integration harnesses share; the manager's
// ci full and any VPP restart after handover take -x).
func sharedLock(t *testing.T) {
	t.Helper()
	f, err := os.OpenFile(labLock, os.O_RDONLY|os.O_CREATE, 0o666)
	if err != nil {
		t.Fatalf("open %s: %v", labLock, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		t.Fatalf("flock -s %s: %v", labLock, err)
	}
	t.Cleanup(func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() })
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "tools", "lab")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root (containing tools/lab) not found above the test directory")
		}
		dir = parent
	}
}

func runLab(t *testing.T, lab string, args ...string) string {
	t.Helper()
	out, err := exec.Command(lab, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("tools/lab %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// parseKV reads the `key: value` lines tools/lab rig prints.
func parseKV(s string) map[string]string {
	kv := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(line, ": ")
		if ok && !strings.HasPrefix(line, " ") {
			kv[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return kv
}

func dumpInterfaces(ctx context.Context, t *testing.T, conn api.Connection) map[string]uint32 {
	t.Helper()
	stream, err := interfaces.NewServiceClient(conn).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{
		SwIfIndex: ^interface_types.InterfaceIndex(0),
	})
	if err != nil {
		t.Fatalf("sw_interface_dump: %v", err)
	}
	out := map[string]uint32{}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("sw_interface_dump recv: %v", err)
		}
		out[d.InterfaceName] = uint32(d.SwIfIndex)
	}
}

// rxPackets reads the interface rx packet counters of the named interfaces from the VPP stats segment.
func rxPackets(t *testing.T, names ...string) map[string]uint64 {
	t.Helper()
	sc, err := core.ConnectStats(statsclient.NewStatsClient(statsSocket))
	if err != nil {
		t.Fatalf("stats connect %s: %v", statsSocket, err)
	}
	defer sc.Disconnect()
	var stats api.InterfaceStats
	if err := sc.GetInterfaceStats(&stats); err != nil {
		t.Fatalf("GetInterfaceStats: %v", err)
	}
	out := map[string]uint64{}
	for _, ic := range stats.Interfaces {
		for _, n := range names {
			if ic.InterfaceName == n {
				out[n] = ic.Rx.Packets
			}
		}
	}
	for _, n := range names {
		if _, ok := out[n]; !ok {
			t.Fatalf("no stats for interface %q", n)
		}
	}
	return out
}

// rigObjects are the exact names `tools/lab rig` creates for one prefix, as anchored patterns (review F1): prefix w1 must
// never match slot 11's ns-w11-lan / w11l0 / host-w11l0. Same patterns as rig_own_netns/links/vpp in tools/lab.
type rigObjects struct{ netns, link, vpp *regexp.Regexp }

func rigObjectsFor(prefix string) rigObjects {
	p := regexp.QuoteMeta(prefix)
	return rigObjects{
		netns: regexp.MustCompile(`^ns-` + p + `-(lan|wan)$`),
		link:  regexp.MustCompile(`^` + p + `[lw][0-9]+$`),
		vpp:   regexp.MustCompile(`^host-` + p + `[lw][0-9]+$`),
	}
}

// leftovers lists every object of THIS prefix that still exists. A failing `ip` is an error, not "nothing left" (review F7).
func leftovers(ctx context.Context, t *testing.T, conn api.Connection, prefix string) []string {
	t.Helper()
	own := rigObjectsFor(prefix)
	var left []string
	out, err := exec.Command("ip", "netns", "list").Output()
	if err != nil {
		t.Fatalf("ip netns list: %v", err)
	}
	for _, l := range strings.Split(string(out), "\n") {
		if f := strings.Fields(l); len(f) > 0 && own.netns.MatchString(f[0]) {
			left = append(left, "netns:"+f[0])
		}
	}
	out, err = exec.Command("ip", "-o", "link", "show").Output()
	if err != nil {
		t.Fatalf("ip -o link show: %v", err)
	}
	for _, l := range strings.Split(string(out), "\n") {
		if f := strings.SplitN(l, ": ", 3); len(f) >= 2 {
			if name := strings.SplitN(f[1], "@", 2)[0]; own.link.MatchString(name) {
				left = append(left, "link:"+name)
			}
		}
	}
	for name := range dumpInterfaces(ctx, t, conn) {
		if own.vpp.MatchString(name) {
			left = append(left, "vpp:"+name)
		}
	}
	return left
}

// TestRigObjectMatchIsAnchored is a pure unit test (no VPP; runs without VRX_INTEGRATION): the leftover check must see
// exactly its own prefix's objects. Regression for review F1 (`rig gc w1` deleted slot 11's host-w11l0/host-w11w0).
func TestRigObjectMatchIsAnchored(t *testing.T) {
	w1 := rigObjectsFor("w1")
	match := map[*regexp.Regexp][]string{
		w1.netns: {"ns-w1-lan", "ns-w1-wan"},
		w1.link:  {"w1l0", "w1l1", "w1w0", "w1w1"},
		w1.vpp:   {"host-w1l0", "host-w1w0"},
	}
	noMatch := map[*regexp.Regexp][]string{
		w1.netns: {"ns-w11-lan", "ns-w12-wan", "ns-w10-lan", "ns-w1-foo", "ns-w1-lan2", "xns-w1-lan", "ns-w1"},
		w1.link:  {"w11l0", "w10w0", "w12l1", "w1lan", "w1", "aw1l0", "w1l0x", "w1x0"},
		w1.vpp:   {"host-w11l0", "host-w12w0", "host-w10l0", "host-w1", "host-w1lan", "host-w1l0x", "local0", "w1l0"},
	}
	for re, names := range match {
		for _, n := range names {
			if !re.MatchString(n) {
				t.Errorf("%s must match %q", re, n)
			}
		}
	}
	for re, names := range noMatch {
		for _, n := range names {
			if re.MatchString(n) {
				t.Errorf("%s must NOT match %q (another slot's or a foreign object)", re, n)
			}
		}
	}
	// a prefix with regexp metacharacters cannot widen the match (prefixes are validated by tools/lab, but be safe)
	if rigObjectsFor("w.").vpp.MatchString("host-w1l0") {
		t.Error("prefix metacharacters must be quoted")
	}
}

func keys(m map[string]uint32) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
