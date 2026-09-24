package interfaces

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestInterfacesScreenshots is the evidence run for docs/user/interfaces/basics.md: the same stack, rig and V19 guard as
// TestInterfacesVerticalSlice, plus `vite preview` of the production web build (apps/web/dist) on the slot web port, with
// ping traffic through VPP while a headless browser (an external node script: playwright-core + Chrome from env paths,
// nothing installed, nothing committed — see P07a/P07b) takes the screenshots.
//
//	VRX_P08_SHOTS=<node script> VRX_P08_SHOTS_OUT=<dir> run.sh -run TestInterfacesScreenshots
//
// The script is called as: node <script> <baseUrl> <outDir> <adminPasswordFile>.
func TestInterfacesScreenshots(t *testing.T) {
	script, out := os.Getenv("VRX_P08_SHOTS"), os.Getenv("VRX_P08_SHOTS_OUT")
	if os.Getenv("VRX_INTEGRATION") != "1" || script == "" || out == "" {
		t.Skip("screenshot evidence run: set VRX_INTEGRATION=1, VRX_P08_SHOTS (node script) and VRX_P08_SHOTS_OUT")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	restarts0 := nRestarts(t)
	t.Cleanup(func() {
		if n := nRestarts(t); n != restarts0 {
			t.Errorf("VPP restarted during the run: NRestarts %d → %d", restarts0, n)
		}
	})
	r := newRig(s)
	conn := connectVPP(t)
	t.Log(mustRun(t, s.lab, "rig", "up", s.prefix))
	t.Cleanup(func() {
		out, err := run(t, s.lab, "rig", "down", s.prefix)
		t.Logf("rig down: %v\n%s", err, out)
	})
	r.peers(t, false)
	deleteBehindBack(t, conn, r.lanDev, r.wanDev)
	st := newStack(t, s)
	a := st.api

	// vite preview of the production build (its /api proxy defaults to server.proxy → the slot API port)
	webPort := os.Getenv("VRX_WEB_PORT")
	if webPort == "" {
		t.Fatal("VRX_WEB_PORT unset (eval \"$(tools/lab env <slot>)\")")
	}
	web := filepath.Join(s.repo, "apps", "web")
	env := append(os.Environ(), "VRX_HTTP_PORT="+s.httpPort, "VRX_WEB_PORT="+webPort)
	pv := start(t, "vite-preview", filepath.Join(s.runDir, "p08", "vite.log"), env, filepath.Join(web, "node_modules", ".bin", "vite"), "preview", web)
	t.Cleanup(func() { pv.stop(t) })

	a.patch("/interfaces", map[string]any{
		r.lanIf: map[string]any{"enabled": true, "description": "LAN (rig)", "ipv4": []string{r.lanGW + "/24"}},
		r.wanIf: map[string]any{"enabled": true, "description": "WAN (rig)", "ipv4": []string{r.wanGW + "/24"},
			"subinterfaces": map[string]any{"100": map[string]any{"vlanId": 100, "enabled": true, "ipv4": []string{"10." + itoa(r.slot) + ".100.1/24"}}}},
	})
	a.commit("p08-docs")
	idx := waitIfs(t, conn, r.lanIf, r.wanIf, r.wanIf+".100")
	for _, l := range v19Guard(t, conn, idx) {
		t.Log("V19 guard: " + l)
	}
	r.peers(t, true)
	// a pending (uncommitted) change for the "pending" marks
	a.patch("/interfaces/"+r.lanIf, map[string]any{"mtu": 1400})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { // traffic for the rates and the sparkline
		for ctx.Err() == nil {
			_, _ = exec.CommandContext(ctx, "ip", "netns", "exec", r.lanNS, "ping", "-q", "-c", "50", "-i", "0.05", "-s", "1000", r.wanIP).CombinedOutput() //nolint:gosec // fixed args
		}
	}()
	pwFile := filepath.Join(s.runDir, "p08", "admin.pw")
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
	time.Sleep(3 * time.Second)
	outp, err := run(t, "node", script, "http://127.0.0.1:"+webPort, out, pwFile)
	t.Log("screenshots:\n" + strings.TrimSpace(outp))
	if err != nil {
		t.Fatalf("screenshot script: %v", err)
	}
	cancel()
	// leave nothing behind: delete through the API with the veths down (D-101)
	a.must(200, "POST", "/api/v1/config/discard", nil)
	r.peers(t, false)
	for _, n := range []string{r.lanIf, r.wanIf} {
		a.must(200, "DELETE", "/api/v1/config/interfaces/"+n, nil)
	}
	a.commit("p08-docs-cleanup")
}
