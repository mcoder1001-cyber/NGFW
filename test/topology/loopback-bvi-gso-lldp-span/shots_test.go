package lbgs

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestLoopbackBviGsoLldpSpanScreenshots is the evidence run for the LLDP, Port mirroring and nsim pages: the same stack
// as TestLoopbackBviGsoLldpSpanOnHost with the features committed (no packets), plus `vite preview` of the production web
// build (apps/web/dist) on the slot web port, while a headless browser (an external node script: playwright-core +
// Chrome from env paths, nothing installed, nothing committed — the P07a/P07b/P08 approach) takes the screenshots
// against the real endpoints.
//
//	VRX_LBGS_SHOTS=<node script> VRX_LBGS_SHOTS_OUT=<dir> run.sh -run TestLoopbackBviGsoLldpSpanScreenshots
//
// The script is called as: node <script> <baseUrl> <outDir> <adminPasswordFile> <bvi>.
func TestLoopbackBviGsoLldpSpanScreenshots(t *testing.T) {
	script, out := os.Getenv("VRX_LBGS_SHOTS"), os.Getenv("VRX_LBGS_SHOTS_OUT")
	if os.Getenv("VRX_INTEGRATION") != "1" || script == "" || out == "" {
		t.Skip("screenshot evidence run: set VRX_INTEGRATION=1, VRX_LBGS_SHOTS (node script) and VRX_LBGS_SHOTS_OUT")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	restarts0 := nRestarts(t)
	t.Cleanup(func() {
		if n := nRestarts(t); n != restarts0 {
			t.Errorf("VPP restarted during the run: NRestarts %d → %d", restarts0, n)
		}
	})
	n := newNames(s)
	conn := connectVPP(t)
	t.Cleanup(func() { leftovers(t, conn, &n) })
	greFixture(t, conn, n.prefix, n.greInst, n.greSrc(), n.greDst(), 7)
	if name, idx, ok := alignedLoopback(t, conn, n.slot); ok {
		n.lldp, n.lldpIdx = name, idx
	}
	st := newStack(t, s)
	a := st.api

	webPort := os.Getenv("VRX_WEB_PORT")
	if webPort == "" {
		t.Fatal("VRX_WEB_PORT unset (eval \"$(tools/lab env <slot>)\")")
	}
	web := filepath.Join(s.repo, "apps", "web")
	env := append(os.Environ(), "VRX_HTTP_PORT="+s.httpPort, "VRX_WEB_PORT="+webPort)
	pv := start(t, "vite-preview", filepath.Join(s.runDir, "lbgs", "vite.log"), env, filepath.Join(web, "node_modules", ".bin", "vite"), "preview", web)
	t.Cleanup(func() { pv.stop(t) })

	a.patch("/interfaces", map[string]any{n.gre: map[string]any{}})
	c0 := a.commit("lbgs-shots-rev0")
	rev0 := int(c0["revision"].(map[string]any)["id"].(float64))
	if n.lldp != "" {
		tagOwner(t, conn, n.prefix, n.lldp, n.lldpIdx)
	}
	ifs, routing := n.loopbacksDoc()
	a.patch("/interfaces", ifs)
	a.patch("/routing", routing)
	a.commit("lbgs-shots-loopbacks")
	a.patch("/interfaces", n.featuresPatch())
	svc := map[string]any{"nsim": map[string]any{"delayMs": 20, "bandwidthMbps": 100, "dropFraction": 0.001, "outputInterfaces": []string{n.mon}}}
	if n.lldp != "" {
		svc["lldp"] = map[string]any{"enabled": true, "systemName": "vrx-" + n.prefix, "interfaces": []any{map[string]any{"interface": n.lldp, "portDescription": n.prefix + " lab uplink"}}}
	}
	a.patch("/services", svc)
	a.commit("lbgs-shots-features")
	// a pending (uncommitted) change for the "pending" mark on the mirroring page: the ERSPAN session mirrors both ways
	a.patch("/interfaces", map[string]any{n.bvi: map[string]any{"mirror": []any{
		map[string]any{"destination": n.mon, "direction": "both", "level": "device"},
		map[string]any{"destination": n.gre, "direction": "both", "level": "device"},
	}}})

	pwFile := filepath.Join(s.runDir, "lbgs", "admin.pw")
	if err := os.WriteFile(pwFile, []byte(st.adminPW), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(pwFile) })
	if !waitFor(30*time.Second, func() bool {
		o, err := run(t, "curl", "-sf", "-o", "/dev/null", "http://127.0.0.1:"+webPort+"/")
		return err == nil && o == ""
	}) {
		t.Fatal("vite preview did not come up")
	}
	time.Sleep(2 * time.Second)
	outp, err := run(t, "node", script, "http://127.0.0.1:"+webPort, out, pwFile, n.bvi)
	t.Log("screenshots:\n" + strings.TrimSpace(outp))
	if err != nil {
		t.Fatalf("screenshot script: %v", err)
	}
	// leave nothing behind: back to revision 0 (the agent deletes the loopbacks and every dependent first)
	a.must(200, "POST", "/api/v1/config/discard", nil)
	rb := a.must(200, "POST", "/api/v1/config/rollback/"+strconv.Itoa(rev0)+"?comment=lbgs-shots-cleanup", nil)
	t.Logf("rollback to %d → %v", rev0, rb.body["status"])
}
