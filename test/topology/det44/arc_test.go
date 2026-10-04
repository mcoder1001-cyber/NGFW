package det44topo

// F-det44-cnat-fix: run 2 of TestDet44MapDsliteCnatOnHost connected 0 of 12 CNAT flows with det44 already removed from
// the document, and `show interface features host-w16l0` listed det44-in2out six times on ip4-unicast. This file
// separates the two suspects on fresh interfaces of the slot and holds the ip4-unicast arc helpers the main test uses
// after every phase.
//
//	TestDet44ArcSplitOnHost
//	  vpp-det44    raw binary API on the rig's own af_packet interfaces, no agent: det44 interface add → del → add →
//	               del, after every step det44_interface_dump + feature_is_enabled + the ip4-unicast arc
//	               (`show interface features`): what VPP 26.06's det44 plugin does to the arc on a delete; then the
//	               config-only repair (feature_enable_disable enable=0 while feature_is_enabled says the node is on).
//	  cnat-fresh   the agent creates fresh interfaces (no det44 history on them) and only the CNAT translation: 12
//	               lan flows to the VIP — suspect (b): does the client side need a cnat interface feature?
//	  det44-churn  the agent adds det44 (inside lan, outside wan) next to the translation, then the document drops
//	               det44: the arc must hold no det44 node afterwards and the VIP must answer 12 of 12 — suspect (a)
//	               through the agent.
//
// Same rules as the main test: NGFW_INTEGRATION=1, root, slot prefix, flock -s on the lab lock and flock -x on the
// globals lock (the det44 plugin enable is a VPP global, V9, opt-in NGFW_FDET44_DET44_HOST=1); no packet trace (D-128),
// processes stopped by PID, NRestarts before and after.

import (
	"fmt"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	vppapi "go.fd.io/govpp/api"
	"google.golang.org/protobuf/encoding/protojson"

	"ngfw/agent/binapi/det44"
	"ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/interface_types"
)

// The det44 graph nodes on the ip4-unicast arc (feature names of src/plugins/nat/det44/det44.c, as `show interface
// features` prints them).
const (
	arcIP4    = "ip4-unicast"
	nodeIn    = "det44-in2out"
	nodeOut   = "det44-out2in"
	maxRepair = 64
)

// arc returns the ip4-unicast feature arc of a VPP interface as `show interface features` prints it, one node per
// line in arc order; VPP's feature config may hold one node several times, so duplicates are kept.
func arc(t *testing.T, ifName string) []string {
	t.Helper()
	var nodes []string
	in := false
	for _, l := range strings.Split(vppctl(t, "show", "interface", "features", ifName), "\n") {
		s := strings.TrimSpace(l)
		switch {
		case s == arcIP4+":":
			in = true
		case in && s == "":
			return nodes
		case in && s != "none configured":
			nodes = append(nodes, s)
		}
	}
	return nodes
}

func countNode(nodes []string, name string) int {
	n := 0
	for _, x := range nodes {
		if x == name {
			n++
		}
	}
	return n
}

func featureOn(t *testing.T, conn vppapi.Connection, idx uint32, node string) bool {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	r, err := feature.NewServiceClient(conn).FeatureIsEnabled(ctx, &feature.FeatureIsEnabled{ArcName: arcIP4, FeatureName: node, SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		t.Fatalf("feature_is_enabled %s/%s sw_if_index=%d: %v", arcIP4, node, idx, err)
	}
	return r.IsEnabled
}

// arcCounts is the number of det44 nodes on the ip4-unicast arc of the lan and wan interface.
type arcCounts struct{ lanIn, lanOut, wanIn, wanOut int }

func (c arcCounts) total() int { return c.lanIn + c.lanOut + c.wanIn + c.wanOut }

// arcState logs, for both rig interfaces, the det44 pool entry (det44_interface_dump), feature_is_enabled of both det44
// nodes and the whole ip4-unicast arc, and returns the det44 node counts.
func (f *fixture) arcState(t *testing.T, label string) arcCounts {
	t.Helper()
	pool := map[uint32]*det44.Det44InterfaceDetails{}
	for _, d := range f.sc.ours(t, f.conn).det44If {
		pool[uint32(d.SwIfIndex)] = d
	}
	var c arcCounts
	var parts []string
	for _, name := range []string{f.r.lanIf, f.r.wanIf} {
		var idx uint32
		found := false
		for i, n := range f.sc.ifIdx {
			if n == name {
				idx, found = i, true
			}
		}
		if !found {
			t.Fatalf("arcState: %s has no sw_if_index in the slot scope", name)
		}
		p := "none"
		if d, ok := pool[idx]; ok {
			p = fmt.Sprintf("in=%v out=%v", d.IsInside, d.IsOutside)
		}
		nodes := arc(t, name)
		in, out := countNode(nodes, nodeIn), countNode(nodes, nodeOut)
		if name == f.r.lanIf {
			c.lanIn, c.lanOut = in, out
		} else {
			c.wanIn, c.wanOut = in, out
		}
		parts = append(parts, fmt.Sprintf("%s(sw_if_index=%d) det44 pool: %s; feature_is_enabled %s=%v %s=%v; %s arc: [%s]",
			name, idx, p, nodeIn, featureOn(t, f.conn, idx, nodeIn), nodeOut, featureOn(t, f.conn, idx, nodeOut), arcIP4, strings.Join(nodes, " ")))
	}
	t.Logf("ARC %s:\n    %s", label, strings.Join(parts, "\n    "))
	return c
}

// mustArc is arcState with the expected det44 node counts: lanIn det44-in2out on the lan interface, wanOut
// det44-out2in on the wan interface, never a node of the other side.
func (f *fixture) mustArc(t *testing.T, label string, lanIn, wanOut int) {
	t.Helper()
	if c := f.arcState(t, label); c != (arcCounts{lanIn: lanIn, wanOut: wanOut}) {
		t.Fatalf("%s: ip4-unicast arcs hold %+v, want %s x%d on %s and %s x%d on %s, nothing else", label, c, nodeIn, lanIn, f.r.lanIf, nodeOut, wanOut, f.r.wanIf)
	}
}

// det44Raw is one det44_interface_add_del_feature call of the test itself (never the agent's).
func det44Raw(t *testing.T, conn vppapi.Connection, idx uint32, inside, add bool) {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	if _, err := det44.NewServiceClient(conn).Det44InterfaceAddDelFeature(ctx, &det44.Det44InterfaceAddDelFeature{IsAdd: add, IsInside: inside, SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
		t.Fatalf("det44_interface_add_del_feature is_add=%v is_inside=%v sw_if_index=%d: %v", add, inside, idx, err)
	}
}

// repairArc is the config-only repair: feature_enable_disable(enable=0) of node while feature_is_enabled reports it
// (VPP removes one instance per disable and ignores a disable of a node that is not on the arc). Returns the number
// of disables sent.
func repairArc(t *testing.T, conn vppapi.Connection, idx uint32, node string) int {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	svc := feature.NewServiceClient(conn)
	for n := 0; n <= maxRepair; n++ {
		if !featureOn(t, conn, idx, node) {
			return n
		}
		if _, err := svc.FeatureEnableDisable(ctx, &feature.FeatureEnableDisable{ArcName: arcIP4, FeatureName: node, SwIfIndex: interface_types.InterfaceIndex(idx), Enable: false}); err != nil {
			t.Fatalf("feature_enable_disable %s/%s sw_if_index=%d enable=0: %v", arcIP4, node, idx, err)
		}
	}
	t.Fatalf("%s still on the %s arc of sw_if_index %d after %d disables", node, arcIP4, idx, maxRepair)
	return 0
}

func (f *fixture) splitDoc(nat string) string {
	return `{"vrfs": {}, "interfaces": ` + f.ifDoc + `, "routing": {"static": []}, "nat": ` + nat + `}`
}

func TestDet44ArcSplitOnHost(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" {
		t.Skip("F-det44-cnat-fix arc split test: set NGFW_INTEGRATION=1 (host VPP, rig) — run.sh does")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (netns, veth, VPP API socket)")
	}
	s := slotFromEnv(t)
	flock(t, labLock, syscallLockSH)
	restarts0 := nRestarts(t, "before the test")
	t.Cleanup(func() {
		if n := nRestarts(t, "after the test"); n != restarts0 {
			t.Errorf("VPP restarted during the test: NRestarts %d → %d", restarts0, n)
		}
	})
	conn := connectVPP(t)
	f := &fixture{s: s, r: newRig(s), conn: conn, vip: s.addr(2, 100), be2: s.addr(2, 3)}
	f.sc = scope{s: s, v4: netip.MustParsePrefix(fmt.Sprintf("10.%d.0.0/16", s.num)), tag: s.prefix + ":"}
	r := f.r
	f.ifDoc = `{"` + r.lanIf + `": {"enabled": true, "description": "rig lan (client side)", "ipv4": ["` + r.lanGW + `/24"]},
	  "` + r.wanIf + `": {"enabled": true, "description": "rig wan (CNAT backends)", "ipv4": ["` + r.wanGW + `/24"]}}`

	winStart := time.Now()
	flock(t, globalsLock, syscallLockEX)
	t.Logf("GLOBALS WINDOW start %s (flock -x %s inside flock -s %s)", winStart.Format(time.RFC3339), globalsLock, labLock)
	t.Cleanup(func() {
		d := time.Since(winStart)
		t.Logf("GLOBALS WINDOW end %s (held %.0fs)", time.Now().Format(time.RFC3339), d.Seconds())
		if d > 10*time.Minute {
			t.Errorf("globals window exceeded 10 min: %s", d)
		}
	})
	if os.Getenv(det44HostEnv) != "1" {
		t.Skipf("%s unset: this test adds det44 interfaces, which needs the det44 plugin enable (V9 window step)", det44HostEnv)
	}
	t.Logf("det44 plugin enable (V9 window step): det44_plugin_enable_disable enable=1 → was already enabled: %v (never disabled)", enableDet44(t, conn))

	t.Log(mustRun(t, s.lab, "rig", "up", s.prefix))
	t.Cleanup(func() {
		out, err := run(t, s.lab, "rig", "down", s.prefix)
		t.Logf("rig down: %v\n%s", err, out)
	})
	r.peers(t, false)

	// ---- (a) in VPP alone: the rig's own interfaces, raw binary API, no agent
	t.Run("vpp-det44", func(t *testing.T) {
		ifs := dumpIfs(t, conn)
		lan, wan := ifs[r.lanIf].idx, ifs[r.wanIf].idx
		f.sc.ifIdx = map[uint32]string{lan: r.lanIf, wan: r.wanIf}
		// a failed step must not leave a det44 pool entry behind on an interface the rig deletes (det44 has no
		// interface-delete hook): delete what is left, then repair the arc — before the rig goes down (LIFO)
		t.Cleanup(func() {
			for _, d := range f.sc.ours(t, conn).det44If {
				det44Raw(t, conn, uint32(d.SwIfIndex), d.IsInside, false)
				t.Logf("cleanup: det44_interface_add_del_feature is_add=0 sw_if_index=%d", d.SwIfIndex)
			}
			for idx := range f.sc.ifIdx {
				repairArc(t, conn, idx, nodeIn)
				repairArc(t, conn, idx, nodeOut)
			}
		})
		if c := f.arcState(t, "fresh rig interfaces (tools/lab rig up)"); c.total() != 0 {
			t.Fatalf("fresh interfaces already carry det44 nodes: %+v", c)
		}
		want := 0
		for i, add := range []bool{true, false, true, false} {
			det44Raw(t, conn, lan, true, add)
			det44Raw(t, conn, wan, false, add)
			want++ // VPP 26.06: the delete enables the node once more instead of disabling it
			c := f.arcState(t, fmt.Sprintf("step %d: det44_interface_add_del_feature is_add=%v (lan inside, wan outside)", i+1, add))
			t.Logf("step %d: %s x%d on %s, %s x%d on %s (an add/del-paired arc would hold %d)", i+1, nodeIn, c.lanIn, r.lanIf, nodeOut, c.wanOut, r.wanIf, map[bool]int{true: 1, false: 0}[add])
			if c.lanIn != want || c.wanOut != want {
				t.Logf("step %d: VPP did not behave as 26.06's det44_interface_add_del (want %d instances): its delete path may be fixed", i+1, want)
				want = c.lanIn
			}
		}
		if left := f.sc.ours(t, conn).det44If; len(left) != 0 {
			t.Fatalf("det44 pool still holds %d of our interfaces after the last delete", len(left))
		}
		nIn, nOut := repairArc(t, conn, lan, nodeIn), repairArc(t, conn, wan, nodeOut)
		t.Logf("config-only repair: feature_enable_disable %s/%s enable=0 x%d on %s, %s/%s enable=0 x%d on %s", arcIP4, nodeIn, nIn, r.lanIf, arcIP4, nodeOut, nOut, r.wanIf)
		if c := f.arcState(t, "after the config-only repair"); c.total() != 0 {
			t.Fatalf("det44 nodes left after the repair: %+v", c)
		}
		nRestarts(t, "after vpp-det44")
	})
	if t.Failed() {
		return
	}

	// ---- the agent takes the rig over: fresh interfaces (the af_packet delete drops every feature of the old ones)
	for _, l := range handToAgent(t, conn, r.lanDev, r.wanDev) {
		t.Log("rig VPP side handed to the agent: " + l)
	}
	f.a = newAgent(t, s)
	f.work = f.a.work
	t.Cleanup(func() { // before the agent stops (LIFO): NAT objects a failed step left behind, via the binary API
		if f.sc.ifIdx == nil {
			return
		}
		if left := f.sc.ours(t, conn); left.count() > 0 {
			t.Logf("cleanup: NAT objects of the slot left by a failed step: %s", left)
			for _, l := range f.sc.lossNoPool(t, conn) { // D-211
				t.Log("cleanup: " + l)
			}
		}
	})
	f.a.mustApply(t, "split-rev1-interfaces", f.splitDoc(`{}`))
	idx := waitIfs(t, conn, r.lanIf, r.wanIf)
	f.sc.ifIdx = map[uint32]string{idx[r.lanIf]: r.lanIf, idx[r.wanIf]: r.wanIf}
	t.Logf("interfaces created by the agent: %s=%d %s=%d", r.lanIf, idx[r.lanIf], r.wanIf, idx[r.wanIf])
	for _, l := range v19Guard(t, conn, idx) {
		t.Log("V19 guard: " + l)
	}
	if bin := os.Getenv("NGFW_PREFLIGHT_BIN"); bin != "" {
		out, err := run(t, bin)
		t.Logf("ngfw-vpp-preflight: %v\n%s", err, strings.TrimSpace(out))
		if err != nil {
			t.Fatal("ngfw-vpp-preflight did not exit 0 — no packet may cross the rig (D-095)")
		}
	}
	r.peers(t, true)
	mustRun(t, "ip", "-n", r.wanNS, "addr", "add", f.be2+"/24", "dev", r.wanPeer)
	t.Cleanup(func() { _, _ = run(t, "ip", "-n", r.wanNS, "addr", "del", f.be2+"/24", "dev", r.wanPeer) })
	server := script(t, f.work, "hold_server.py", holdServer)
	inNSProc(t, "wan-be1-8080", f.work, r.wanNS, "python3", server, r.wanIP, "8080")
	inNSProc(t, "wan-be2-8080", f.work, r.wanNS, "python3", server, f.be2, "8080")
	time.Sleep(500 * time.Millisecond)
	if out, err := inNS(t, r.lanNS, "ping", "-n", "-c", "2", "-W", "1", r.lanGW); err != nil {
		t.Logf("warm-up ping (may lose the ARP round): %v\n%s", err, out)
	}

	cnatOnly := `{"cnat": {"translations": [{"name": "web", "protocol": "tcp", "vip": {"ip": "` + f.vip + `", "port": 80},
	  "backends": [{"ip": "` + r.wanIP + `", "port": 8080}, {"ip": "` + f.be2 + `", "port": 8080}]}]}}`

	// ---- (b) CNAT on interfaces without any det44 history
	t.Run("cnat-fresh", func(t *testing.T) {
		f.a.mustApply(t, "split-rev2-cnat", f.splitDoc(cnatOnly))
		if c := f.arcState(t, "after rev 2 (cnat only, interfaces never had det44)"); c.total() != 0 {
			t.Fatalf("det44 nodes on fresh interfaces: %+v", c)
		}
		t.Log("vppctl show cnat translation (ours):\n" + grepLines(vppctl(t, "show", "cnat", "translation"), "10."+fmt.Sprint(s.num)+"."))
		hits := f.cnatRound(t, "rev 2 (cnat only, no det44 history, no cnat interface feature)", 2)
		t.Logf("suspect (b): CNAT VIP without any cnat interface feature → %v", hits)
		nRestarts(t, "after cnat-fresh")
	})
	if t.Failed() {
		return
	}

	// ---- (a) through the agent: det44 next to the translation, then the document drops det44
	t.Run("det44-churn", func(t *testing.T) {
		det44Doc := `{"det44": {"enabled": true, "inside": ["` + r.lanIf + `"], "outside": ["` + r.wanIf + `"],
		  "mappings": [{"inside": "` + s.addr(1, 0) + `/24", "outside": "` + s.addr(3, 0) + `/30"}]}, ` + strings.TrimPrefix(cnatOnly, "{")
		resp := f.a.mustApply(t, "split-rev3-det44-cnat", f.splitDoc(det44Doc))
		t.Logf("rev 3 results: %d", len(resp.GetResults()))
		if c := f.arcState(t, "after rev 3 (det44 inside lan / outside wan + cnat)"); c.lanIn != 1 || c.wanOut != 1 || c.lanOut != 0 || c.wanIn != 0 {
			t.Fatalf("after the det44 create the arcs hold %+v, want exactly one %s on %s and one %s on %s", c, nodeIn, r.lanIf, nodeOut, r.wanIf)
		}
		resp = f.a.mustApply(t, "split-rev4-cnat-again", f.splitDoc(cnatOnly))
		t.Logf("rev 4 (det44 dropped) results: %s", trunc(protojson.Format(resp), 1500))
		c := f.arcState(t, "after rev 4 (det44 removed from the document, cnat kept)")
		got, err := f.a.retrieveNat(t)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Retrieve(nat) after rev 4 = %s", trunc(protojson.Format(got), 1200))
		if c.total() != 0 { // not fatal: the CNAT round below shows what the leftover nodes do to the VIP
			t.Errorf("det44 removed from the document but the ip4-unicast arcs still hold det44 nodes: %+v", c)
		}
		hits := f.cnatRound(t, "rev 4 (det44 added and removed again, cnat kept)", 3)
		t.Logf("suspect (a): CNAT VIP after a det44 add + delete through the agent → %v", hits)
		nRestarts(t, "after det44-churn")
	})
	if t.Failed() {
		return
	}

	t.Run("cleanup-through-agent", func(t *testing.T) {
		r.peers(t, false) // D-101: the agent deletes the af_packet interfaces only with their veths down
		f.a.mustApplyDomains(t, "split-rev5-cleanup", `{"vrfs": {}, "interfaces": {}, "routing": {"static": []}, "nat": {}}`, "interfaces", "vrfs", "routing", "nat")
		left := f.sc.ours(t, conn)
		ifs := dumpIfs(t, conn)
		_, l := ifs[r.lanIf]
		_, w := ifs[r.wanIf]
		t.Logf("after the cleanup through the agent: NAT objects of the slot %d %s; %s present=%v %s present=%v", left.count(), left, r.lanIf, l, r.wanIf, w)
		if left.count() != 0 || l || w {
			t.Error("the agent left NAT objects or a rig interface in VPP")
		}
		f.sc.ifIdx = nil
	})
}
