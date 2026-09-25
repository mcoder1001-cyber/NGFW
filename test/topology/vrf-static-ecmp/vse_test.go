package vrfstaticecmp

// F-vrf-static-ecmp topology test on the host (VRX_INTEGRATION=1, root, shared lab lock): the real vrx-agent and vrx-api
// of this tree, the host VPP, the slot's af_packet rig for the ping. It proves the acceptance items end to end:
//
//   - commit VRFs (+ source VRF select) and static routes (weighted ECMP, next hop in another VRF, blackhole) through the
//     API; `vppctl show ip fib table <id> 0.0.0.0/0` shows both weighted paths; the drift view (Retrieve vs running) is empty
//   - a next hop in an undeclared VRF → 400 problem+json with its pointer
//   - the FIB browser (`/state/routes`, the agent's ListRoutes) answers one page
//   - ping from the data plane through the Action bridge reaches the rig peer
//   - restart simulation: stop the agent, delete our routes / svs objects / tables behind its back (binapi, routes before
//     tables), start it → everything is back (time + agent log excerpt)
//   - rollback to the revision before: routes then tables removed, no stray entry (V15: the table ids re-created hold only
//     VPP's defaults)
//
// Optional screenshots (VRX_VSE_SHOTS=<node script>, VRX_VSE_SHOTS_OUT=<dir>): vite preview of apps/web/dist on the slot web
// port and an external headless-browser script (P07a/P07b/P08 approach; nothing installed, nothing committed).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	govpp "go.fd.io/govpp"
	vppapi "go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	svsapi "ngfw/agent/binapi/svs"
)

const apiSocket = "/run/vpp/api.sock"

func TestVrfStaticEcmpTopology(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("F-vrf-static-ecmp topology test: set VRX_INTEGRATION=1 (host VPP, rig, PostgreSQL) — run.sh does")
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
	base := 1000 * s.num
	red, blue := base+21, base+22
	n := s.num
	conn := connectVPP(t)

	// the rig (ping target 10.<n>.2.2 behind host-<p>w0) — the V19 preflight first (D-095): no packet before it passes
	pre, err := run(t, "go", "-C", filepath.Join(s.repo, "apps", "agent"), "run", "./cmd/vrx-vpp-preflight")
	t.Logf("vrx-vpp-preflight: %v\n%s", err, pre)
	if err != nil {
		t.Fatalf("V19 preflight failed: no packet may cross the rig")
	}
	t.Log(mustRun(t, s.lab, "rig", "up", s.prefix))
	t.Cleanup(func() {
		out, err := run(t, s.lab, "rig", "down", s.prefix)
		t.Logf("rig down: %v\n%s", err, out)
	})

	st := newStack(t, s)
	a := st.api
	t.Cleanup(func() { cleanupConfig(t, a) })

	// ---- commit ---------------------------------------------------------------------------------------------
	l1, l2 := fmt.Sprintf("loop%d21", n), fmt.Sprintf("loop%d22", n)
	cfg := map[string]any{
		"interfaces": map[string]any{
			l1: map[string]any{"enabled": true, "vrf": "red", "ipv4": []string{fmt.Sprintf("10.%d.221.1/24", n)}},
			l2: map[string]any{"enabled": true, "ipv4": []string{fmt.Sprintf("10.%d.222.1/24", n)}},
		},
		"vrfs": map[string]any{
			"red":  map[string]any{"id": red, "description": "customer red", "sourceSelect": []any{map[string]any{"prefix": fmt.Sprintf("10.%d.50.0/24", n), "interface": l2}}},
			"blue": map[string]any{"id": blue},
		},
		// canonical order (VRF, then prefix) — the order Retrieve assembles, so the drift view compares equal
		"routing": map[string]any{"static": []any{
			map[string]any{"prefix": fmt.Sprintf("10.%d.70.0/24", n), "vrf": "blue", "blackhole": true},
			map[string]any{"prefix": "0.0.0.0/0", "vrf": "red", "description": "weighted ECMP", "nextHops": []any{
				map[string]any{"address": fmt.Sprintf("10.%d.221.2", n), "weight": 3},
				map[string]any{"address": fmt.Sprintf("10.%d.221.3", n), "weight": 1}}},
			map[string]any{"prefix": fmt.Sprintf("10.%d.60.0/24", n), "vrf": "red", "nextHops": []any{
				map[string]any{"address": fmt.Sprintf("10.%d.2.2", n), "vrf": "default"}}},
		}},
	}
	// baseline revision: the two loopbacks in the default VRF (the rollback target)
	a.patch("/interfaces", map[string]any{
		l1: map[string]any{"enabled": true, "ipv4": []string{fmt.Sprintf("10.%d.221.1/24", n)}},
		l2: map[string]any{"enabled": true, "ipv4": []string{fmt.Sprintf("10.%d.222.1/24", n)}},
	})
	a.commit("vse-baseline")
	rev0 := revision(t, a)
	for _, k := range []string{"vrfs", "interfaces", "routing"} {
		a.patch("/"+k, cfg[k])
	}
	start := time.Now()
	a.commit("vse")
	rev1 := revision(t, a)
	t.Logf("commit rev %d → %d in %v", rev0, rev1, time.Since(start))

	fib := mustRun(t, "vppctl", "show", "ip", "fib", "table", fmt.Sprint(red), "0.0.0.0/0")
	t.Logf("vppctl show ip fib table %d 0.0.0.0/0:\n%s", red, fib)
	if !strings.Contains(fib, "weight=3") || !strings.Contains(fib, "weight=1") {
		t.Errorf("both weighted ECMP paths expected in the FIB")
	}
	t.Logf("vppctl show ip fib table %d 10.%d.60.0/24:\n%s", red, n, mustRun(t, "vppctl", "show", "ip", "fib", "table", fmt.Sprint(red), fmt.Sprintf("10.%d.60.0/24", n)))
	t.Logf("vppctl show ip fib table %d 10.%d.70.0/24:\n%s", blue, n, mustRun(t, "vppctl", "show", "ip", "fib", "table", fmt.Sprint(blue), fmt.Sprintf("10.%d.70.0/24", n)))
	t.Logf("vppctl show svs:\n%s", mustRun(t, "vppctl", "show", "svs"))
	drift := a.must(200, "GET", "/api/v1/state/drift", nil)
	t.Logf("GET /api/v1/state/drift → %s", drift.raw)
	if ch, _ := drift.body["changes"].([]any); len(ch) != 0 {
		t.Errorf("Retrieve differs from running: %v", ch)
	}

	// ---- validation failure --------------------------------------------------------------------------------
	a.patch("/routing", map[string]any{"static": []any{map[string]any{"prefix": fmt.Sprintf("10.%d.61.0/24", n), "vrf": "red",
		"nextHops": []any{map[string]any{"address": fmt.Sprintf("10.%d.2.2", n), "vrf": "nope"}}}}})
	bad := a.call("POST", "/api/v1/config/commit?comment=bad", nil)
	t.Logf("commit with a next hop in an undeclared VRF → %d %s", bad.status, bad.raw)
	if bad.status != 400 || !strings.Contains(bad.raw, `"pointer":"/routing/static/0/nextHops/0/vrf"`) {
		t.Errorf("want 400 with pointer /routing/static/0/nextHops/0/vrf")
	}
	a.must(200, "POST", "/api/v1/config/discard", nil)

	// ---- FIB browser and ping ------------------------------------------------------------------------------
	t0 := time.Now()
	page := a.must(200, "GET", "/api/v1/state/routes?vrf=red&page=1&pageSize=1000", nil)
	t.Logf("GET /api/v1/state/routes?vrf=red&pageSize=1000 → %d routes of %v in %v", len(page.body["items"].([]any)), page.body["total"], time.Since(t0).Round(time.Millisecond))
	ping := a.must(200, "POST", "/api/v1/actions/ping", map[string]any{"target": fmt.Sprintf("10.%d.2.2", n), "count": 5, "intervalMs": 500})
	t.Logf("POST /api/v1/actions/ping 10.%d.2.2 → %s", n, ping.raw)
	if done, _ := ping.body["done"].(map[string]any); done == nil || done["exitCode"] != float64(0) {
		t.Errorf("no ping reply from the rig peer (VPP's ping API under-counts on a busy API: V-new)")
	}
	refused := a.call("POST", "/api/v1/actions/ping", map[string]any{"target": fmt.Sprintf("10.%d.2.2", n), "vrf": "red"})
	t.Logf("ping in VRF red → %d %s", refused.status, refused.raw)
	if refused.status != 400 || !strings.Contains(refused.raw, `"pointer":"/vrf"`) {
		t.Errorf("ping in a non-default VRF: want 400 with pointer /vrf")
	}

	if script, out := os.Getenv("VRX_VSE_SHOTS"), os.Getenv("VRX_VSE_SHOTS_OUT"); script != "" && out != "" {
		screenshots(t, s, st, script, out)
	}

	// ---- restart simulation --------------------------------------------------------------------------------
	st.agent.stop(t)
	svsTable := ownedSvsTable(t, conn, s.prefix)
	loseObjects(t, conn, s.prefix, svsTable, uint32(red), uint32(blue), l2)
	if out := mustRun(t, "vppctl", "show", "ip", "fib", "table", fmt.Sprint(red)); !strings.Contains(out, "No such") && strings.Contains(out, "0.0.0.0/0") && strings.Contains(out, "weight=3") {
		t.Fatalf("routes still there after the simulated loss:\n%s", out)
	}
	logFrom := fileSize(st.agentLog)
	start = time.Now()
	st.startAgent(t)
	back := waitFor(30*time.Second, func() bool {
		out, _ := run(t, "vppctl", "show", "ip", "fib", "table", fmt.Sprint(red), "0.0.0.0/0")
		d := a.call("GET", "/api/v1/state/drift", nil)
		ch, _ := d.body["changes"].([]any)
		return strings.Contains(out, "weight=3") && d.status == 200 && len(ch) == 0
	})
	took := time.Since(start)
	_, raw := readAgentLog(t, st.agentLog, logFrom)
	var excerpt []string
	for _, l := range raw {
		if strings.Contains(l, "reconcile") || strings.Contains(l, "created") || strings.Contains(l, "VPP binary API connected") {
			excerpt = append(excerpt, l)
		}
	}
	if len(excerpt) > 40 {
		excerpt = append(excerpt[:20], append([]string{"…"}, excerpt[len(excerpt)-20:]...)...)
	}
	t.Logf("restart simulation: agent restarted, VRFs + routes + svs back=%v in %v; agent log excerpt:\n%s", back, took.Round(time.Millisecond), strings.Join(excerpt, "\n"))
	if !back {
		t.Errorf("not recreated within 30 s")
	}
	t.Logf("after the restart, vppctl show ip fib table %d 0.0.0.0/0:\n%s", red, mustRun(t, "vppctl", "show", "ip", "fib", "table", fmt.Sprint(red), "0.0.0.0/0"))
	t.Logf("after the restart, vppctl show svs:\n%s", mustRun(t, "vppctl", "show", "svs"))

	// ---- rollback ------------------------------------------------------------------------------------------
	rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d", rev0), nil)
	t.Logf("rollback to rev %d → %s", rev0, rb.raw)
	for _, id := range []int{red, blue, svsTable} {
		t.Logf("after rollback, vppctl show ip fib table %d:\n%s", id, mustRun(t, "vppctl", "show", "ip", "fib", "table", fmt.Sprint(id)))
	}
	ret := a.must(200, "GET", "/api/v1/state/routes?vrf=default&source=API&pageSize=100", nil)
	t.Logf("after rollback, API routes in default: %s", ret.raw)
	for _, id := range []uint32{uint32(red), uint32(blue), uint32(svsTable)} {
		cnt, entries := probeTable(t, conn, id, s.prefix+":v15probe")
		t.Logf("V15 probe: table %d re-created holds %d entries: %s", id, cnt, entries)
		if cnt != 5 {
			t.Errorf("V15: table %d holds %d entries after the rollback (want VPP's 5 defaults)", id, cnt)
		}
	}
}

// revision is the running revision id.
func revision(t *testing.T, a *api) int {
	t.Helper()
	r := a.must(200, "GET", "/api/v1/state/system", nil)
	v, _ := r.body["runningRevision"].(float64)
	return int(v)
}

// cleanupConfig commits an empty interfaces/vrfs/routing so the agent removes every object of this test.
func cleanupConfig(t *testing.T, a *api) {
	t.Helper()
	for _, k := range []string{"routing", "vrfs", "interfaces"} {
		a.call("PUT", "/api/v1/config/"+k, map[string]any{})
	}
	r := a.call("POST", "/api/v1/config/commit?comment=vse-cleanup", nil)
	t.Logf("cleanup commit: %d %s", r.status, r.raw)
}

func connectVPP(t *testing.T) vppapi.Connection {
	t.Helper()
	conn, err := govpp.Connect(apiSocket)
	if err != nil {
		t.Fatalf("govpp connect: %v", err)
	}
	t.Cleanup(conn.Disconnect)
	return conn
}

// ownedSvsTable finds this slot's svs table ("<prefix>:svs:<id>").
func ownedSvsTable(t *testing.T, conn vppapi.Connection, prefix string) int {
	t.Helper()
	ctx := context.Background()
	stream, err := ip.NewServiceClient(conn).IPTableDump(ctx, &ip.IPTableDump{})
	if err != nil {
		t.Fatal(err)
	}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			t.Fatal("no svs table of " + prefix)
		}
		if err != nil {
			t.Fatal(err)
		}
		var id int
		if _, e := fmt.Sscanf(strings.TrimRight(d.Table.Name, "\x00"), prefix+":svs:%d", &id); e == nil {
			return id
		}
	}
}

// loseObjects deletes this test's data-plane objects behind the agent's back (binapi, own objects only): the svs
// enablement, svs entries and svs table, the API routes of red/blue — routes before their tables (V15) — and the
// VRF tables. The loopbacks stay (the interfaces test covers them).
func loseObjects(t *testing.T, conn vppapi.Connection, prefix string, svsTable int, red, blue uint32, l2 string) {
	t.Helper()
	ctx := context.Background()
	cl := ip.NewServiceClient(conn)
	sv := svsapi.NewServiceClient(conn)
	if idx, ok := ifIndex(t, conn, l2); ok {
		for _, af := range []ip_types.AddressFamily{ip_types.ADDRESS_IP4, ip_types.ADDRESS_IP6} {
			if _, err := sv.SvsEnableDisable(ctx, &svsapi.SvsEnableDisable{IsEnable: false, Af: af, TableID: uint32(svsTable), SwIfIndex: idx}); err != nil {
				t.Logf("svs disable %v: %v", af, err)
			}
		}
	}
	for _, tbl := range []uint32{uint32(svsTable), red, blue} {
		for _, v6 := range []bool{false, true} {
			var routes []ip.IPRouteV2
			stream, err := cl.IPRouteV2Dump(ctx, &ip.IPRouteV2Dump{Table: ip.IPTable{TableID: tbl, IsIP6: v6}})
			if err != nil {
				t.Fatal(err)
			}
			for {
				d, err := stream.Recv()
				if err != nil {
					break
				}
				routes = append(routes, d.Route)
			}
			for _, r := range routes {
				switch {
				case tbl == uint32(svsTable) && r.Src != 8 && r.Prefix.Len > 0:
					if _, err := sv.SvsRouteAddDel(ctx, &svsapi.SvsRouteAddDel{IsAdd: false, Prefix: r.Prefix, TableID: tbl}); err != nil {
						t.Logf("svs route del %s: %v", r.Prefix, err)
					}
				case r.Src == 8:
					if _, err := cl.IPRouteAddDel(ctx, &ip.IPRouteAddDel{IsAdd: false, Route: ip.IPRoute{TableID: tbl, Prefix: r.Prefix}}); err != nil {
						t.Logf("route del %d %s: %v", tbl, r.Prefix, err)
					}
				}
			}
		}
	}
	// VRF tables: the interface binding keeps VPP's table alive, the API lock goes (what a lost table looks like)
	for _, tbl := range []uint32{uint32(svsTable), red, blue} {
		for _, v6 := range []bool{false, true} {
			if _, err := cl.IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: false, Table: ip.IPTable{TableID: tbl, IsIP6: v6}}); err != nil {
				t.Logf("table del %d v6=%v: %v", tbl, v6, err)
			}
		}
	}
	t.Logf("simulated loss (binapi): svs on %s, svs table %d, API routes of tables %d/%d, tables %d/%d/%d; vppctl show svs:\n%s",
		l2, svsTable, red, blue, svsTable, red, blue, mustRun(t, "vppctl", "show", "svs"))
}

func ifIndex(t *testing.T, conn vppapi.Connection, name string) (interface_types.InterfaceIndex, bool) {
	t.Helper()
	out, err := run(t, "vppctl", "show", "interface", name)
	if err != nil {
		return 0, false
	}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 && f[0] == name {
			var idx uint32
			if _, e := fmt.Sscanf(f[1], "%d", &idx); e == nil {
				return interface_types.InterfaceIndex(idx), true
			}
		}
	}
	return 0, false
}

// probeTable creates table id (IPv4) under a probe name, returns its entries and deletes it again (V15 check).
func probeTable(t *testing.T, conn vppapi.Connection, id uint32, name string) (int, string) {
	t.Helper()
	ctx := context.Background()
	cl := ip.NewServiceClient(conn)
	if _, err := cl.IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: true, Table: ip.IPTable{TableID: id, Name: name}}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = cl.IPTableAddDel(context.Background(), &ip.IPTableAddDel{IsAdd: false, Table: ip.IPTable{TableID: id}})
	}()
	stream, err := cl.IPRouteV2Dump(ctx, &ip.IPRouteV2Dump{Table: ip.IPTable{TableID: id}})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for {
		d, err := stream.Recv()
		if err != nil {
			break
		}
		names = append(names, fmt.Sprintf("%s(src %d)", d.Route.Prefix, d.Route.Src))
	}
	return len(names), strings.Join(names, " ")
}

// screenshots: vite preview of the production web build on the slot web port, then the external script
// (node <script> <baseUrl> <outDir> <adminPasswordFile>).
func screenshots(t *testing.T, s slot, st *stack, script, out string) {
	t.Helper()
	webPort := os.Getenv("VRX_WEB_PORT")
	if webPort == "" {
		t.Fatal("VRX_WEB_PORT unset (eval \"$(tools/lab env <slot>)\")")
	}
	web := filepath.Join(s.repo, "apps", "web")
	env := append(os.Environ(), "VRX_HTTP_PORT="+s.httpPort, "VRX_WEB_PORT="+webPort)
	pv := start(t, "vite-preview", filepath.Join(s.runDir, "vse", "vite.log"), env, filepath.Join(web, "node_modules", ".bin", "vite"), "preview", web)
	defer pv.stop(t)
	pwFile := filepath.Join(s.runDir, "vse", "admin.pw")
	if err := os.WriteFile(pwFile, []byte(st.adminPW), 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(pwFile)
	if !waitFor(60*time.Second, func() bool {
		_, err := run(t, "curl", "-fsS", "-o", "/dev/null", "http://127.0.0.1:"+webPort+"/")
		return err == nil
	}) {
		t.Fatal("vite preview did not come up")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", script, "http://127.0.0.1:"+webPort, out, pwFile) //nolint:gosec // test-provided script path
	cmd.Env = append(os.Environ(), "VSE_SLOT="+fmt.Sprint(s.num), "VSE_PING="+fmt.Sprintf("10.%d.2.2", s.num))
	res, err := cmd.CombinedOutput()
	t.Logf("screenshots (%s): %v\n%s", out, err, res)
	if err != nil {
		t.Errorf("screenshot script failed")
	}
}
