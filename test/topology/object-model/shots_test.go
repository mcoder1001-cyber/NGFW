package objectmodel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestObjectModelScreenshots is the evidence run for docs/user/firewall/object-model.md: the same stack as
// TestObjectModelTopology (real agent resolving the FQDN objects through the test's DNS responder, real API) plus
// `vite preview` of the production web build (apps/web/dist) on the slot web port, and an external headless-browser
// script (playwright-core + Chrome-for-Testing from env paths, nothing installed, nothing committed — P07a/P07b/P08).
//
//	VRX_OM_SHOTS=<node script> VRX_OM_SHOTS_OUT=<dir> run.sh -run TestObjectModelScreenshots
//
// The script is called as: node <script> <baseUrl> <outDir> <adminPasswordFile>.
func TestObjectModelScreenshots(t *testing.T) {
	script, out := os.Getenv("VRX_OM_SHOTS"), os.Getenv("VRX_OM_SHOTS_OUT")
	if os.Getenv("VRX_INTEGRATION") != "1" || script == "" || out == "" {
		t.Skip("screenshot evidence run: set VRX_INTEGRATION=1, VRX_OM_SHOTS (node script) and VRX_OM_SHOTS_OUT")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	zone := s.prefix + ".test"
	cdn, web := "cdn."+zone, "web."+zone
	dns := startDNS(t)
	dns.set(cdn, "192.0.2.53", "2001:db8::53")
	st := newStack(t, s, dns.addr, 30*time.Second) // web.<zone> is not served: the column shows a failed name
	a := st.api

	webDir := filepath.Join(s.repo, "apps", "web")
	env := append(os.Environ(), "VRX_HTTP_PORT="+s.httpPort, "VRX_WEB_PORT="+s.webPort)
	pv := start(t, "vite-preview", filepath.Join(st.work, "vite.log"), env, filepath.Join(webDir, "node_modules", ".bin", "vite"), "preview", webDir)
	t.Cleanup(func() { pv.stop(t) })

	objects := sampleObjects(s, cdn, web)
	a.patch("/objects", objects)
	a.patch("/acl", sampleACL())
	a.commit("object-model-docs")
	if !waitFor(20*time.Second, func() bool {
		item := fqdnItem(a, "cdn")
		return item != nil && len(strs(item["addresses"])) == 2 && fqdnItem(a, "site")["error"] != ""
	}) {
		t.Fatalf("FQDN state not ready: %v %v", fqdnItem(a, "cdn"), fqdnItem(a, "site"))
	}
	// an uncommitted change for the "pending" mark
	a.patch("/objects/addressGroups/web-servers", map[string]any{"description": "public web tier"})

	pwFile := filepath.Join(st.work, "admin.pw")
	if err := os.WriteFile(pwFile, []byte(st.adminPW), 0o600); err != nil {
		t.Fatal(err)
	}
	if !waitFor(30*time.Second, func() bool {
		o, err := run("curl", "-sf", "-o", "/dev/null", "http://127.0.0.1:"+s.webPort+"/")
		return err == nil && o == ""
	}) {
		t.Fatal("vite preview did not come up")
	}
	outp, err := run("node", script, "http://127.0.0.1:"+s.webPort, out, pwFile)
	t.Log("screenshots:\n" + strings.TrimSpace(outp))
	if err != nil {
		t.Fatalf("screenshot script: %v", err)
	}
	a.must(200, "POST", "/api/v1/config/discard", nil)
	a.must(200, "PUT", "/api/v1/config/objects", map[string]any{})
	a.must(200, "PUT", "/api/v1/config/acl", map[string]any{})
	a.commit("object-model-docs-cleanup")
}
