package bridgel2

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBridgeL2Screenshots is the evidence run for the Bridging page: the same stack and rig (no packets) as
// TestBridgeL2OnHost with the L2 model committed, plus `vite preview` of the production web build (apps/web/dist) on
// the slot web port, while a headless browser (an external node script: playwright-core + Chrome from env paths,
// nothing installed, nothing committed — the P07a/P07b/P08 approach) takes the screenshots against the real endpoints.
//
//	VRX_BL2_SHOTS=<node script> VRX_BL2_SHOTS_OUT=<dir> run.sh -run TestBridgeL2Screenshots
//
// The script is called as: node <script> <baseUrl> <outDir> <adminPasswordFile>.
func TestBridgeL2Screenshots(t *testing.T) {
	script, out := os.Getenv("VRX_BL2_SHOTS"), os.Getenv("VRX_BL2_SHOTS_OUT")
	if os.Getenv("VRX_INTEGRATION") != "1" || script == "" || out == "" {
		t.Skip("screenshot evidence run: set VRX_INTEGRATION=1, VRX_BL2_SHOTS (node script) and VRX_BL2_SHOTS_OUT")
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
	t.Log(mustRun(t, s.lab, "rig", "up", s.prefix))
	t.Cleanup(func() {
		o, err := run(t, s.lab, "rig", "down", s.prefix)
		t.Logf("rig down: %v\n%s", err, o)
	})
	n.peersDown(t)
	deleteBehindBack(t, conn, n.lanDev, n.wanDev)
	st := newStack(t, s)
	a := st.api

	webPort := os.Getenv("VRX_WEB_PORT")
	if webPort == "" {
		t.Fatal("VRX_WEB_PORT unset (eval \"$(tools/lab env <slot>)\")")
	}
	web := filepath.Join(s.repo, "apps", "web")
	env := append(os.Environ(), "VRX_HTTP_PORT="+s.httpPort, "VRX_WEB_PORT="+webPort)
	pv := start(t, "vite-preview", filepath.Join(s.runDir, "bl2", "vite.log"), env, filepath.Join(web, "node_modules", ".bin", "vite"), "preview", web)
	t.Cleanup(func() { pv.stop(t) })

	a.patch("/interfaces", n.l3Doc())
	a.commit("bl2-docs-l3")
	ifs, routing := n.l2Patch()
	a.patch("/interfaces", ifs)
	a.patch("/routing", routing)
	a.commit("bl2-docs-l2")
	// a pending (uncommitted) change for the "pending" mark on the list
	a.patch("/routing/l2/bridgeDomains/"+n.bd, map[string]any{"macAgeMin": 10})

	pwFile := filepath.Join(s.runDir, "bl2", "admin.pw")
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
	outp, err := run(t, "node", script, "http://127.0.0.1:"+webPort, out, pwFile)
	t.Log("screenshots:\n" + strings.TrimSpace(outp))
	if err != nil {
		t.Fatalf("screenshot script: %v", err)
	}
	// leave nothing behind: the interfaces (and with them every L2 object) go through the API
	a.must(200, "POST", "/api/v1/config/discard", nil)
	for name := range n.l3Doc() {
		a.must(200, "DELETE", "/api/v1/config/interfaces/"+name, nil)
	}
	a.must(200, "DELETE", "/api/v1/config/routing/l2", nil)
	a.commit("bl2-docs-cleanup")
	if len(ourBDs(t, conn, n.prefix)) != 0 || len(ourDevices(t, conn, n.prefix)) != 0 {
		t.Error("L2 objects left after the cleanup commit")
	}
}
