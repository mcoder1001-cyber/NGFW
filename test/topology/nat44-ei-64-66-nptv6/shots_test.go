package nat44ei6466nptv6

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNatEI6466Screenshots is the evidence run for the EI, NAT64, NAT66 and NPTv6 tabs of the NAT screen (en + fa/RTL):
// the same stack, rig, fixtures and V19 guard as TestNatEI6466Nptv6, the configuration committed through the API, live
// EI and NAT64 sessions, `vite preview` of the production web build (apps/web/dist) on the slot web port, and a
// headless browser — an external node script with playwright-core and a Chrome-for-Testing headless shell from env
// paths (nothing installed, as in P07a/P07b/P08 and F-nat44-ed-sessions).
//
//	VRX_NAT_SHOTS_OUT=<dir> VRX_PLAYWRIGHT_CORE=<pkg dir> VRX_CHROME=<chrome-headless-shell> VRX_CHROME_LIBS=<dir> run.sh -run TestNatEI6466Screenshots
func TestNatEI6466Screenshots(t *testing.T) {
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
	if edEnabled(t, conn) {
		t.Skip("nat44-ed is enabled on this VPP by another owner")
	}
	f := &fixture{s: s, r: newRig(s), a6: newV6(s), conn: conn, binding: map[string]string{}}
	f.ei = ensureEI(t, conn)
	if rc := eiRunningConfig(t, conn); rc.insideVrf != 0 || rc.outsideV != 0 || rc.flags != 0 || rc.forwarding {
		t.Skipf("nat44-ei enabled by another owner with %+v", rc)
	}
	ensure64(t, conn)
	ensure66(t, conn)
	f.sc = scope{s: s, v4: netip.MustParsePrefix(fmt.Sprintf("10.%d.0.0/16", s.num)), v6: netip.MustParsePrefix(fmt.Sprintf("fd00:%s::/32", hexSlot(s)))}
	r, a6 := f.r, f.a6
	f.loopIn, f.loopOut = fmt.Sprintf("loop%d61", s.num), fmt.Sprintf("loop%d62", s.num)
	t.Log(mustRun(t, s.lab, "rig", "up", s.prefix))
	t.Cleanup(func() {
		o, err := run(t, s.lab, "rig", "down", s.prefix)
		t.Logf("rig down: %v\n%s", err, o)
	})
	r.peers(t, false)
	deleteBehindBack(t, conn, r.lanDev, r.wanDev)
	f.st = newStack(t, s)
	t.Cleanup(func() {
		if f.sc.ifIdx == nil {
			return
		}
		if left, bind := f.sc.ours(t, conn), f.sc.npt66Lines(t); len(left)+len(bind) > 0 {
			for _, l := range f.sc.loss(t, conn, f.binding) {
				t.Log("cleanup: " + l)
			}
		}
	})
	f.a, f.c, f.scripts = f.st.api, dialAgent(t, s.socket), f.st.work
	a := f.a
	tbl, tblID := fmt.Sprintf("%s-n64", s.prefix), s.num*1000+64
	t.Cleanup(func() {
		for _, l := range gcSlotVRF(t, conn, uint32(tblID), netip.MustParsePrefix(r.addr(2, 0)+"/24")) { //nolint:gosec // slot ≤ 11
			t.Log("cleanup: " + l)
		}
	})

	webPort := os.Getenv("VRX_WEB_PORT")
	if webPort == "" {
		t.Fatal("VRX_WEB_PORT unset (eval \"$(tools/lab env <slot>)\")")
	}
	web := filepath.Join(s.repo, "apps", "web")
	if _, err := os.Stat(filepath.Join(web, "dist", "index.html")); err != nil {
		t.Fatalf("apps/web/dist missing — run.sh builds it when VRX_NAT_SHOTS_OUT is set: %v", err)
	}
	env := append(os.Environ(), "VRX_HTTP_PORT="+s.httpPort, "VRX_WEB_PORT="+webPort)
	pv := start(t, "vite-preview", filepath.Join(f.st.work, "vite.log"), env, filepath.Join(web, "node_modules", ".bin", "vite"), "preview", web)
	t.Cleanup(func() { pv.stop(t) })

	if tenantVRFHost() { // the NAT64 screenshot needs the slot VRF (opt-in, see tenantVRFHostEnv)
		a.patch("/vrfs", map[string]any{tbl: map[string]any{"id": tblID, "description": "NAT64 slot VRF"}})
	}
	a.patch("/interfaces", map[string]any{
		r.lanIf:   map[string]any{"enabled": true, "description": "NAT inside (rig lan)", "ipv4": []string{r.lanGW + "/24"}, "ipv6": []string{a6.lanGW + "/64", a6.nptGW + "/64"}},
		r.wanIf:   map[string]any{"enabled": true, "description": "NAT outside (rig wan)", "ipv4": []string{r.wanGW + "/24"}, "ipv6": []string{a6.wanGW + "/64"}},
		f.loopIn:  map[string]any{"enabled": true, "description": "NAT66 inside", "ipv6": []string{fmt.Sprintf("fd00:%s:66:1::1/64", hexSlot(s))}},
		f.loopOut: map[string]any{"enabled": true, "description": "NAT66 outside", "ipv6": []string{fmt.Sprintf("fd00:%s:66:2::1/64", hexSlot(s))}},
	})
	rev1 := int(a.commit("nat-ei-docs-interfaces")["revision"].(map[string]any)["id"].(float64))
	a.patch("/nat", map[string]any{
		"mode": "ei", "inside": []string{r.lanIf}, "outside": []string{r.wanIf},
		"pools": []any{map[string]any{"name": "pat", "description": "outbound PAT", "range": r.addr(2, 100) + "-" + r.addr(2, 103)}},
		"staticMappings": []any{map[string]any{"name": "web", "description": "port forward", "protocol": "tcp",
			"local": map[string]any{"ip": r.lanIP, "port": 80}, "external": map[string]any{"ip": r.addr(2, 103), "port": 8080}}},
		"nat66": map[string]any{"enabled": true, "inside": []string{f.loopIn}, "outside": []string{f.loopOut},
			"staticMappings": []any{map[string]any{"description": "server", "local": a6.nat66Local, "external": a6.nat66X}}},
		"nptv6": map[string]any{"bindings": []any{map[string]any{"description": "site", "interface": r.wanIf, "internal": a6.nptInternal, "external": a6.nptExternal}}},
	})
	a.commit("nat-ei-docs")
	f.binding[r.wanIf] = a6.nptInternal
	idx := waitIfs(t, conn, r.lanIf, r.wanIf, f.loopIn, f.loopOut)
	f.sc.ifIdx = map[uint32]string{idx[r.lanIf]: r.lanIf, idx[r.wanIf]: r.wanIf, idx[f.loopIn]: f.loopIn, idx[f.loopOut]: f.loopOut}
	for _, l := range v19Guard(t, conn, map[string]uint32{r.lanIf: idx[r.lanIf], r.wanIf: idx[r.wanIf]}) {
		t.Log("V19 guard: " + l)
	}
	if bin := os.Getenv("VRX_PREFLIGHT_BIN"); bin != "" {
		if o, err := run(t, bin); err != nil {
			t.Fatalf("vrx-vpp-preflight: %v\n%s", err, o)
		}
	}
	r.peers(t, true)
	r.up6(t, a6)

	// EI traffic: a held TCP session and 300 UDP flows
	server := script(t, f.scripts, "hold_server.py", holdServer)
	client := script(t, f.scripts, "hold_client.py", holdClient)
	flows := script(t, f.scripts, "udp_flows.py", udpFlows)
	inNSProc(t, "wan-server-8000", f.scripts, r.wanNS, "python3", server, r.wanIP, "8000")
	time.Sleep(time.Second)
	inNSProc(t, "lan-client-40001", f.scripts, r.lanNS, "python3", client, r.lanIP, "40001", r.wanIP, "8000")
	if o, err := inNS(t, r.lanNS, "python3", flows, r.lanIP, "20000", "300", r.wanIP, "9"); err != nil {
		t.Fatalf("udp flows: %v %s", err, o)
	}
	ei := a.must(200, "GET", "/api/v1/state/nat/ei/sessions?pageSize=5", nil)
	t.Logf("GET /state/nat/ei/sessions?pageSize=5 → %s", trunc(ei.raw, 600))

	pwFile := filepath.Join(f.st.work, "admin.pw")
	if err := os.WriteFile(pwFile, []byte(f.st.adminPW), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(pwFile) })
	if !waitFor(40*time.Second, func() bool {
		o, err := run(t, "curl", "-sf", "-o", "/dev/null", "http://127.0.0.1:"+webPort+"/")
		return err == nil && o == ""
	}) {
		t.Fatal("vite preview did not come up")
	}
	shots := func(tabs string) {
		chromeEnv := append(os.Environ(), "LD_LIBRARY_PATH="+os.Getenv("VRX_CHROME_LIBS"))
		o, err := runEnv(t, chromeEnv, "node", filepath.Join(s.repo, "test", "topology", "nat44-ei-64-66-nptv6", "shots.mjs"), "http://127.0.0.1:"+webPort, out, pwFile, tabs)
		t.Log("screenshots:\n" + strings.TrimSpace(o))
		if err != nil {
			t.Fatalf("screenshot script: %v", err)
		}
	}
	shots("ei,nat66,nptv6")

	// NAT64 with a live session (rev 3 of TestNatEI6466Nptv6, its assertions included) — opt-in: it pins the slot VRF
	// in VPP until VPP restarts (tenantVRFHostEnv)
	if tenantVRFHost() {
		f.nat64(t, tbl, uint32(tblID)) //nolint:gosec // slot ≤ 11
		n64 := a.must(200, "GET", "/api/v1/state/nat/nat64/sessions", nil)
		t.Logf("GET /state/nat/nat64/sessions → %s", trunc(n64.raw, 600))
		shots("nat64")
	} else {
		t.Logf("NAT64 screenshot skipped: set %s=1 (it leaves table %d in VPP until VPP restarts)", tenantVRFHostEnv, tblID)
	}

	// leave nothing behind: rollback, then the interfaces through the API with the veths down (D-101); the slot VRF
	// stays (nat64 FIB locks, V-new) and its IPv4 table is removed in Cleanup
	a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=nat-ei-docs-off", rev1), nil)
	r.peers(t, false)
	for _, n := range []string{r.lanIf, r.wanIf, f.loopIn, f.loopOut} {
		a.must(200, "DELETE", "/api/v1/config/interfaces/"+n, nil)
	}
	a.commit("nat-ei-docs-cleanup")
	if left, bind := f.sc.ours(t, conn), f.sc.npt66Lines(t); len(left)+len(bind) != 0 {
		t.Errorf("left in VPP: %v %v", left, bind)
	}
}
