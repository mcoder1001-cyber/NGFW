// Package bonding is F-bonding's topology test: link aggregation end to end against the REAL host VPP through the real
// agent + API + slot database, with fixture taps as the member NICs (untagged, like DPDK NICs; no rig, no af_packet).
//
//	TestBondingOnHost
//	  baseline     three fixture taps configured as plain interfaces (enabled) — revision A
//	  commit       BondEthernet<N>000: LACP, lb l34, members tap<N>000 + tap<N>001 (passive), address 10.<N>.10.1/24;
//	               BondEthernet<N>001: active-backup, member tap<N>002 with weight 200 — revision B
//	  evidence     vppctl show bond details / show lacp / show int addr; Retrieve == running (/state/drift and the bond
//	               leaves of /state/interfaces); /state/interfaces/bonds (BondState)
//	  validation   tap<N>001 also in BondEthernet<N>001 → commit 400 problem+json, pointer to the second membership
//	  restart      stop the agent → delete memberships, addresses and bonds behind its back via binapi (members first,
//	               D-095 c) → start the agent → bonds, members, weight and address back within 30 s (agent log)
//	  rollback     rollback to revision A → bonds and memberships gone (Retrieve + dumps), the taps plain L3 interfaces
//	  cleanup      taps out of the configuration, fixture taps deleted, no bond of the slot range left in VPP
//
// No packet is sent (LACP has no partner on taps; its evidence is configuration + `show lacp`), nothing is traced
// (D-128), no binding is swept (D-126). Runs only with VRX_INTEGRATION=1, as root, with a slot prefix (w<N>), under
// flock -s on the lab lock; every process it starts is stopped by PID; NRestarts is checked before and after.
package bonding

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	bondapi "ngfw/agent/binapi/bond"
)

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// bondLeaf returns interfaces.<name>.bond of an /state/interfaces item's config (Retrieve) or running view.
func bondLeaf(item map[string]any, view string) map[string]any {
	c, _ := item[view].(map[string]any)
	b, _ := c["bond"].(map[string]any)
	return b
}

func TestBondingOnHost(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("F-bonding topology test: set VRX_INTEGRATION=1 (host VPP, PostgreSQL) — run.sh does")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (VPP API socket, tap host devices)")
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
	if left := bonds(t, conn, s); len(left) > 0 {
		t.Fatalf("bonds of this slot's id range already in VPP (leftover of another run?): %v", left)
	}
	taps := fixtureTaps(s, 3)
	createTaps(t, conn, taps)
	for _, l := range bindingCheck(t, conn, taps) {
		t.Log("binding check (read-only): " + l)
	}
	st := newStack(t, s)
	a := st.api
	n := strconv.Itoa(s.num)
	b0, b1 := fmt.Sprintf("BondEthernet%d", s.num*1000), fmt.Sprintf("BondEthernet%d", s.num*1000+1)
	t0, t1, t2 := taps[0].name, taps[1].name, taps[2].name
	addr := "10." + n + ".10.1/24"
	defer func() { // leave nothing behind even when a step fails: everything out of the configuration
		a.call("POST", "/api/v1/config/discard", nil)
		for _, name := range []string{b0, b1, t0, t1, t2} {
			a.call("DELETE", "/api/v1/config/interfaces/"+name, nil)
		}
		if r := a.call("POST", "/api/v1/config/commit?comment=f-bonding-cleanup", nil); r.status != 200 {
			t.Logf("cleanup commit: %d %s", r.status, r.raw)
		}
		if left := bonds(t, conn, s); len(left) > 0 {
			t.Errorf("bonds left in VPP after cleanup: %v", left)
		}
		t.Log("vppctl show bond (after cleanup):\n" + vppctl(t, "show", "bond"))
	}()

	// ---- baseline: the taps as plain interfaces (revision A) ----
	a.patch("/interfaces", map[string]any{t0: map[string]any{"enabled": true}, t1: map[string]any{"enabled": true}, t2: map[string]any{"enabled": true}})
	revA := a.commit("f-bonding-baseline")["revision"].(map[string]any)["id"].(float64)
	t.Logf("baseline commit → revision %v", revA)

	// ---- the bonds (revision B) ----
	a.patch("/interfaces", map[string]any{
		b0: map[string]any{"enabled": true, "description": "LACP uplink", "ipv4": []string{addr},
			"bond": map[string]any{"mode": "lacp", "loadBalance": "l34", "members": map[string]any{t0: map[string]any{}, t1: map[string]any{"passive": true}}}},
		b1: map[string]any{"enabled": true,
			"bond": map[string]any{"mode": "active-backup", "members": map[string]any{t2: map[string]any{"weight": 200}}}},
	})
	c := a.commit("f-bonding")
	t.Logf("commit → status %v revision %v results %s", c["status"], c["revision"].(map[string]any)["id"], js(c["results"]))

	bs := bonds(t, conn, s)
	lacp, ab := bs[b0], bs[b1]
	if lacp.mode != bondapi.BOND_API_MODE_LACP || lacp.lb != bondapi.BOND_API_LB_ALGO_L34 || len(lacp.members) != 2 || !lacp.members[t1].IsPassive || lacp.members[t0].IsPassive {
		t.Fatalf("VPP %s = %+v", b0, lacp)
	}
	if ab.mode != bondapi.BOND_API_MODE_ACTIVE_BACKUP || ab.lb != bondapi.BOND_API_LB_ALGO_AB || ab.members[t2] == nil || ab.members[t2].Weight != 200 {
		t.Fatalf("VPP %s = %+v", b1, ab)
	}
	details := vppctl(t, "show", "bond", "details")
	t.Log("vppctl show bond details:\n" + section(details, b0) + section(details, b1))
	for _, want := range []string{"mode: lacp", "load balance: l34", t0, t1} {
		if !strings.Contains(section(details, b0), want) {
			t.Errorf("show bond details %s lacks %q", b0, want)
		}
	}
	t.Log("vppctl show lacp:\n" + vppctl(t, "show", "lacp"))
	t.Log("vppctl show interface address " + b0 + ":\n" + vppctl(t, "show", "interface", "address", b0))
	if !strings.Contains(vppctl(t, "show", "interface", "address", b0), strings.TrimSuffix(addr, "/24")) {
		t.Errorf("%s has no %s", b0, addr)
	}
	ifs := dumpIfs(t, conn)
	if ifs[b0].tag != s.prefix+":"+b0 || !ifs[b0].adminUp || ifs[t0].tag != "" || !ifs[t0].adminUp {
		t.Errorf("tags/admin: %s %+v, %s %+v", b0, ifs[b0], t0, ifs[t0])
	}

	// Retrieve == desired: no drift between running and what the agent retrieves, bond leaves compared explicitly
	drift := a.must(200, "GET", "/api/v1/state/drift", nil)
	t.Logf("GET /state/drift → %s", drift.raw)
	if ch, _ := drift.body["changes"].([]any); len(ch) != 0 {
		t.Errorf("Retrieve differs from running: %s", js(ch))
	}
	items, _ := a.ifState()
	for _, name := range []string{b0, b1} {
		ret, run := bondLeaf(items[name], "config"), bondLeaf(items[name], "running")
		t.Logf("%s bond: Retrieve=%s running=%s", name, js(ret), js(run))
		if ret == nil || js(ret) != js(run) {
			t.Errorf("%s: Retrieve bond %s != running %s", name, js(ret), js(run))
		}
	}
	live := a.must(200, "GET", "/api/v1/state/interfaces/bonds", nil)
	t.Logf("GET /state/interfaces/bonds → %s", live.raw)

	// ---- validation: the second membership of tap<N>001 ----
	a.patch("/interfaces/"+b1+"/bond/members", map[string]any{t1: map[string]any{}})
	v := a.call("POST", "/api/v1/config/commit", nil)
	t.Logf("commit with %s in two bonds → %d %s", t1, v.status, v.raw)
	wantPtr := "/interfaces/" + b1 + "/bond/members/" + t1
	if v.status != 400 || !strings.Contains(v.raw, `"pointer":"`+wantPtr+`"`) {
		t.Errorf("duplicate membership: %d %s (want 400 with pointer %s)", v.status, v.raw, wantPtr)
	}
	a.must(200, "POST", "/api/v1/config/discard", nil)

	if shots, out := os.Getenv("VRX_F_BONDING_SHOTS"), os.Getenv("VRX_F_BONDING_SHOTS_OUT"); shots != "" && out != "" {
		screenshots(t, st, shots, out, b0)
	}

	// ---- restart safety: stop the agent, lose everything behind its back, start it ----
	st.agent.stop(t)
	for _, l := range loseBonds(t, conn, s) {
		t.Log("loss: " + l)
	}
	if len(bonds(t, conn, s)) != 0 {
		t.Fatal("loss simulation left bonds")
	}
	logFrom := fileSize(st.agentLog)
	t0start := time.Now()
	st.startAgent(t)
	back := waitFor(30*time.Second, func() bool {
		bs := bonds(t, conn, s)
		b, ok := bs[b0]
		return ok && len(b.members) == 2 && bs[b1].members[t2] != nil && bs[b1].members[t2].Weight == 200 &&
			strings.Contains(vppctl(t, "show", "interface", "address", b0), strings.TrimSuffix(addr, "/24"))
	})
	took := time.Since(t0start)
	lines, raw := readAgentLog(t, st.agentLog, logFrom)
	for i, l := range lines {
		if strings.HasPrefix(l.Msg, "reconcile") || strings.Contains(l.Msg, "VPP boot identity") || strings.Contains(l.Msg, "created") && strings.Contains(raw[i], "bond") {
			t.Log("agent log: " + raw[i])
		}
	}
	if !back {
		t.Fatalf("bonds, members, weight and address not back within 30 s")
	}
	t.Logf("restart: bonds + members + weight + address back %.3f s after the agent start (no config API call)", took.Seconds())
	details = vppctl(t, "show", "bond", "details")
	t.Log("vppctl show bond details (after restart):\n" + section(details, b0) + section(details, b1))
	if d := a.must(200, "GET", "/api/v1/state/drift", nil); len(d.body["changes"].([]any)) != 0 {
		t.Errorf("drift after restart: %s", d.raw)
	}

	// ---- rollback to the baseline: bonds and memberships removed, the taps plain L3 interfaces ----
	rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=f-bonding-rollback", int(revA)), nil)
	t.Logf("POST /config/rollback/%d → %s", int(revA), rb.raw)
	if left := bonds(t, conn, s); len(left) != 0 {
		t.Errorf("bonds left after rollback: %v", left)
	}
	items, _ = a.ifState()
	for _, name := range []string{b0, b1} {
		if it, ok := items[name]; ok {
			t.Errorf("%s still listed after rollback: %s", name, js(it))
		}
	}
	for _, tp := range taps {
		it := items[tp.name]
		t.Logf("Retrieve %s after rollback: %s", tp.name, js(it["config"]))
		if bondLeaf(it, "config") != nil {
			t.Errorf("%s still has a bond leaf", tp.name)
		}
		mode := vppctl(t, "show", "mode", tp.name)
		t.Log("vppctl show mode " + tp.name + ": " + strings.TrimSpace(mode))
		if !strings.Contains(mode, "l3 "+tp.name) {
			t.Errorf("%s is not a plain L3 interface: %s", tp.name, mode)
		}
	}
	t.Log("vppctl show bond (after rollback):\n" + vppctl(t, "show", "bond"))
	if d := a.must(200, "GET", "/api/v1/state/drift", nil); len(d.body["changes"].([]any)) != 0 {
		t.Errorf("drift after rollback: %s", d.raw)
	}
}

// screenshots runs the external headless-browser script (kept outside the repository, like P07/P08's) against a
// `vite preview` of the production web build on the slot web port, with a pending change for the "pending" mark.
func screenshots(t *testing.T, st *stack, script, out, bond string) {
	t.Helper()
	webPort := os.Getenv("VRX_WEB_PORT")
	if webPort == "" {
		t.Fatal("VRX_WEB_PORT unset (eval \"$(tools/lab env <slot>)\")")
	}
	web := filepath.Join(st.s.repo, "apps", "web")
	env := append(os.Environ(), "VRX_HTTP_PORT="+st.s.httpPort, "VRX_WEB_PORT="+webPort)
	pv := start(t, "vite-preview", filepath.Join(st.s.runDir, "f-bonding-vite.log"), env, filepath.Join(web, "node_modules", ".bin", "vite"), "preview", web)
	defer pv.stop(t)
	st.api.patch("/interfaces/"+bond+"/bond", map[string]any{"numaOnly": true}) // pending
	defer st.api.call("POST", "/api/v1/config/discard", nil)
	pwFile := filepath.Join(st.s.runDir, "f-bonding-admin.pw")
	if err := os.WriteFile(pwFile, []byte(st.adminPW), 0o600); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(pwFile) }()
	if !waitFor(30*time.Second, func() bool {
		o, err := run(t, "curl", "-sf", "-o", "/dev/null", "http://127.0.0.1:"+webPort+"/")
		return err == nil && o == ""
	}) {
		t.Fatal("vite preview did not come up")
	}
	o, err := exec.Command("node", script, "http://127.0.0.1:"+webPort, out, pwFile, bond).CombinedOutput() //nolint:gosec // test-only script path from the operator
	t.Log("screenshots:\n" + strings.TrimSpace(string(o)))
	if err != nil {
		t.Fatalf("screenshot script: %v", err)
	}
}
