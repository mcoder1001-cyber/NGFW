package vlanqinq

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestVlanQinqScreenshots is the evidence run for docs/user/interfaces/vlan-qinq.md: the stack, rig and V19 guard of
// TestVlanQinqTopology, the three sub-interfaces committed on host-<p>w0, one pending (uncommitted) tag change so the
// table shows the stack VPP still has, and `vite preview` of the production web build (apps/web/dist) on the slot web
// port while shots.mjs (headless Chrome, nothing installed) takes the screenshots.
//
//	VRX_QINQ_SHOTS=test/topology/vlan-qinq/shots.mjs VRX_QINQ_SHOTS_OUT=<dir> VRX_PLAYWRIGHT_CORE=<dir> VRX_CHROME=<bin> \
//	  run.sh -run TestVlanQinqScreenshots
func TestVlanQinqScreenshots(t *testing.T) {
	script, out := os.Getenv("VRX_QINQ_SHOTS"), os.Getenv("VRX_QINQ_SHOTS_OUT")
	if os.Getenv("VRX_INTEGRATION") != "1" || script == "" || out == "" {
		t.Skip("screenshot evidence run: set VRX_INTEGRATION=1, VRX_QINQ_SHOTS (node script) and VRX_QINQ_SHOTS_OUT")
	}
	if !filepath.IsAbs(script) {
		wd, _ := os.Getwd()
		script = filepath.Join(wd, filepath.Base(script))
	}
	webPort := os.Getenv("VRX_WEB_PORT")
	if webPort == "" {
		t.Fatal("VRX_WEB_PORT unset (eval \"$(tools/lab env <slot>)\")")
	}
	s, r, conn, st := setup(t)
	a := st.api
	W := r.wanIf
	subs := qinqSubs(s.num)
	names := []string{W}
	cfg := map[string]any{}
	for _, x := range subs {
		names = append(names, x.name(W))
		cfg[x.id] = x.config()
	}

	// vite preview of the production build (preview.proxy = server.proxy → the slot API port)
	web := filepath.Join(s.repo, "apps", "web")
	if _, err := os.Stat(filepath.Join(web, "dist", "index.html")); err != nil {
		t.Fatalf("apps/web/dist missing — run.sh builds it when VRX_QINQ_SHOTS is set: %v", err)
	}
	env := append(os.Environ(), "VRX_HTTP_PORT="+s.httpPort, "VRX_WEB_PORT="+webPort)
	pv := start(t, "vite-preview", filepath.Join(st.work, "vite.log"), env, filepath.Join(web, "node_modules", ".bin", "vite"), "preview", web)
	t.Cleanup(func() { pv.stop(t) })

	a.patch("/interfaces", map[string]any{W: map[string]any{"enabled": true, "description": "WAN (rig): QinQ parent", "ipv4": []string{r.wanGW + "/24"}, "subinterfaces": cfg}})
	a.commit("qinq-docs")
	idx := waitIfs(t, conn, names...)
	for _, l := range v19Guard(t, conn, idx) {
		t.Log("V19 guard: " + l)
	}
	r.peers(t, true) // link up for the status chips; no traffic is sent
	// a pending tag change (inner 30 → 31, not committed): the table shows the stack VPP still has under the new one
	a.patch("/interfaces/"+W+"/subinterfaces/300", map[string]any{"innerVlanId": 31})

	pwFile := filepath.Join(st.work, "admin.pw")
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
	outp, err := run(t, "node", script, "http://127.0.0.1:"+webPort, out, pwFile, W)
	t.Log("screenshots:\n" + strings.TrimSpace(outp))
	if err != nil {
		t.Fatalf("screenshot script: %v", err)
	}
	// leave nothing behind: discard the pending change, delete the parent through the API (the agent quiesces the veth)
	a.must(200, "POST", "/api/v1/config/discard", nil)
	r.peers(t, false)
	a.must(200, "DELETE", "/api/v1/config/interfaces/"+W, nil)
	a.commit("qinq-docs-cleanup")
	for n := range dumpIfs(t, conn) {
		if n == W || strings.HasPrefix(n, W+".") {
			t.Errorf("%s still in VPP after the cleanup commit", n)
		}
	}
}
