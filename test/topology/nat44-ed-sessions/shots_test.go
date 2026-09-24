package nat44edsessions

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNat44EdScreenshots is the evidence run for the NAT screen (en + fa/RTL): the same stack, rig, fixture and V19
// guard as TestNat44EdSessions, the NAT configuration committed through the API, ≥ 2 000 live sessions, `vite preview` of
// the production web build (apps/web/dist) on the slot web port, and a headless browser — an external node script with
// playwright-core and a Chrome-for-Testing headless shell from env paths (nothing installed, as in P07a/P07b/P08).
//
//	VRX_NAT_SHOTS_OUT=<dir> VRX_PLAYWRIGHT_CORE=<pkg dir> VRX_CHROME=<chrome-headless-shell> run.sh -run TestNat44EdScreenshots
//
// shots.mjs is called as: node shots.mjs <baseUrl> <outDir> <adminPasswordFile>.
func TestNat44EdScreenshots(t *testing.T) {
	out := os.Getenv("VRX_NAT_SHOTS_OUT")
	if os.Getenv("VRX_INTEGRATION") != "1" || out == "" || os.Getenv("VRX_PLAYWRIGHT_CORE") == "" || os.Getenv("VRX_CHROME") == "" {
		t.Skip("screenshot evidence run: set VRX_INTEGRATION=1, VRX_NAT_SHOTS_OUT, VRX_PLAYWRIGHT_CORE and VRX_CHROME")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	slotLock(t, s, "nat44")
	sharedFlock(t, globalsLock)
	restarts0 := nRestarts(t)
	t.Cleanup(func() {
		if n := nRestarts(t); n != restarts0 {
			t.Errorf("VPP restarted during the run: NRestarts %d → %d", restarts0, n)
		}
	})
	conn := connectVPP(t)
	if eiEnabled(t, conn) {
		t.Skip("nat44-ei is enabled on this VPP by another owner")
	}
	ensurePlugin(t, conn)
	rc := natRunning(t, conn)
	if rc.insideVrf != 0 || rc.outsideV != 0 || rc.sessions < 2200 {
		t.Skipf("nat44-ed enabled by another owner with VRFs %d/%d, %d sessions/thread", rc.insideVrf, rc.outsideV, rc.sessions)
	}
	r := newRig(s)
	slotNet := netip.MustParsePrefix(fmt.Sprintf("10.%d.0.0/16", s.num))
	t.Log(mustRun(t, s.lab, "rig", "up", s.prefix))
	t.Cleanup(func() {
		o, err := run(t, s.lab, "rig", "down", s.prefix)
		t.Logf("rig down: %v\n%s", err, o)
	})
	r.peers(t, false)
	deleteBehindBack(t, conn, r.lanDev, r.wanDev)
	st := newStack(t, s)
	var ifIdx map[uint32]string
	t.Cleanup(func() {
		if ifIdx == nil {
			return
		}
		if left := ours(t, conn, s.prefix, slotNet, ifIdx); len(left) > 0 {
			for _, l := range natLoss(t, conn, s.prefix, slotNet, ifIdx) {
				t.Log("cleanup: " + l)
			}
		}
	})
	a := st.api

	webPort := os.Getenv("VRX_WEB_PORT")
	if webPort == "" {
		t.Fatal("VRX_WEB_PORT unset (eval \"$(tools/lab env <slot>)\")")
	}
	web := filepath.Join(s.repo, "apps", "web")
	if _, err := os.Stat(filepath.Join(web, "dist", "index.html")); err != nil {
		t.Fatalf("apps/web/dist missing — run.sh builds it when VRX_NAT_SHOTS_OUT is set: %v", err)
	}
	env := append(os.Environ(), "VRX_HTTP_PORT="+s.httpPort, "VRX_WEB_PORT="+webPort)
	pv := start(t, "vite-preview", filepath.Join(st.work, "vite.log"), env, filepath.Join(web, "node_modules", ".bin", "vite"), "preview", web)
	t.Cleanup(func() { pv.stop(t) })

	a.patch("/interfaces", map[string]any{
		r.lanIf: map[string]any{"enabled": true, "description": "NAT inside (rig lan)", "ipv4": []string{r.lanGW + "/24"}},
		r.wanIf: map[string]any{"enabled": true, "description": "NAT outside (rig wan)", "ipv4": []string{r.wanGW + "/24"}},
	})
	rev1 := a.commit("nat-docs-interfaces")["revision"].(map[string]any)["id"].(float64)
	natCfg := map[string]any{
		"inside": []string{r.lanIf}, "outside": []string{r.wanIf},
		"pools": []any{map[string]any{"name": "pat", "description": "outbound PAT", "range": r.addr(2, 100) + "-" + r.addr(2, 103)}},
		"staticMappings": []any{
			map[string]any{"name": "web", "description": "port forward :8080 → lan :80", "protocol": "tcp",
				"local": map[string]any{"ip": r.lanIP, "port": 80}, "external": map[string]any{"ip": r.addr(2, 110), "port": 8080}},
			map[string]any{"name": "one2one", "description": "1:1", "local": map[string]any{"ip": r.addr(1, 3)}, "external": map[string]any{"ip": r.addr(2, 111)}},
		},
		"identityMappings": []any{map[string]any{"ip": r.addr(2, 112), "protocol": "udp", "port": 500}},
	}
	if rc.sessions != 63*1024 {
		natCfg["sessionLimit"] = rc.sessions
	}
	a.patch("/nat", natCfg)
	a.commit("nat-docs")
	idx := waitIfs(t, conn, r.lanIf, r.wanIf)
	ifIdx = map[uint32]string{idx[r.lanIf]: r.lanIf, idx[r.wanIf]: r.wanIf}
	for _, l := range v19Guard(t, conn, idx) {
		t.Log("V19 guard: " + l)
	}
	if bin := os.Getenv("VRX_PREFLIGHT_BIN"); bin != "" {
		if o, err := run(t, bin); err != nil {
			t.Fatalf("vrx-vpp-preflight: %v\n%s", err, o)
		}
	}
	r.peers(t, true)

	// traffic: a held TCP session and 2 100 UDP flows to our own wan address
	server := script(t, st.work, "hold_server.py", holdServer)
	client := script(t, st.work, "hold_client.py", holdClient)
	flows := script(t, st.work, "udp_flows.py", udpFlows)
	inNSProc(t, "wan-server-8000", st.work, r.wanNS, "python3", server, r.wanIP, "8000")
	time.Sleep(time.Second)
	inNSProc(t, "lan-client-40001", st.work, r.lanNS, "python3", client, r.lanIP, "40001", r.wanIP, "8000")
	if o, err := inNS(t, r.lanNS, "python3", flows, r.lanIP, "20000", "2100", r.wanIP, "9"); err != nil {
		t.Fatalf("udp flows: %v %s", err, o)
	}
	sum := a.must(200, "GET", "/api/v1/state/nat/summary", nil)
	t.Logf("GET /state/nat/summary → %s", trunc(sum.raw, 800))
	// a pending (uncommitted) change for the candidate bar
	a.patch("/nat", map[string]any{"forwarding": true})

	pwFile := filepath.Join(st.work, "admin.pw")
	if err := os.WriteFile(pwFile, []byte(st.adminPW), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(pwFile) })
	if !waitFor(40*time.Second, func() bool {
		o, err := run(t, "curl", "-sf", "-o", "/dev/null", "http://127.0.0.1:"+webPort+"/")
		return err == nil && o == ""
	}) {
		t.Fatal("vite preview did not come up")
	}
	chromeEnv := append(os.Environ(), "LD_LIBRARY_PATH="+os.Getenv("VRX_CHROME_LIBS"))
	cmdOut, err := runEnv(t, chromeEnv, "node", filepath.Join(s.repo, "test", "topology", "nat44-ed-sessions", "shots.mjs"), "http://127.0.0.1:"+webPort, out, pwFile)
	t.Log("screenshots:\n" + strings.TrimSpace(cmdOut))
	if err != nil {
		t.Fatalf("screenshot script: %v", err)
	}
	// leave nothing behind: the candidate, the NAT objects (cleanup above), the interfaces through the API with the
	// veths down (D-101)
	a.must(200, "POST", "/api/v1/config/discard", nil)
	a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=nat-docs-off", int(rev1)), nil)
	r.peers(t, false)
	for _, n := range []string{r.lanIf, r.wanIf} {
		a.must(200, "DELETE", "/api/v1/config/interfaces/"+n, nil)
	}
	a.commit("nat-docs-cleanup")
	if left := ours(t, conn, s.prefix, slotNet, ifIdx); len(left) != 0 {
		t.Errorf("left in VPP: %v", left)
	}
}
