package hostacl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHostACLScreenshots is the evidence run for docs/user/firewall/host-acl-nftables.md: the stack of
// TestHostACLTopology (real agent rendering into the slot namespace, real API) plus `vite preview` of the production
// web build (apps/web/dist) on the slot web port, and an external headless-browser script (playwright-core +
// Chrome-for-Testing from env paths, nothing installed, nothing committed — the P07a/P07b/P08 approach).
//
//	VRX_HA_SHOTS=<node script> VRX_HA_SHOTS_OUT=<dir> run.sh -run TestHostACLScreenshots
//
// The script is called as: node <script> <baseUrl> <outDir> <adminPasswordFile>.
func TestHostACLScreenshots(t *testing.T) {
	script, out := os.Getenv("VRX_HA_SHOTS"), os.Getenv("VRX_HA_SHOTS_OUT")
	if os.Getenv("VRX_INTEGRATION") != "1" || script == "" || out == "" {
		t.Skip("screenshot evidence run: set VRX_INTEGRATION=1, VRX_HA_SHOTS (node script) and VRX_HA_SHOTS_OUT")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	rootBefore := rootTables(t)
	t.Cleanup(func() {
		if after := rootTables(t); after != rootBefore {
			t.Errorf("the root netns ruleset changed:\n%s\n---\n%s", rootBefore, after)
		}
	})
	ns := newNetns(t, s)
	st := newStack(t, s, ns.host)
	a := st.api

	webDir := filepath.Join(s.repo, "apps", "web")
	env := append(os.Environ(), "VRX_HTTP_PORT="+s.httpPort, "VRX_WEB_PORT="+s.webPort)
	pv := start(t, "vite-preview", filepath.Join(st.work, "vite.log"), env, filepath.Join(webDir, "node_modules", ".bin", "vite"), "preview", webDir)
	t.Cleanup(func() { pv.stop(t) })

	a.patch("/objects", map[string]any{
		"addresses":     map[string]any{"peer": map[string]any{"type": "host", "address": ns.peerAddr}, "noc": map[string]any{"type": "network", "prefix": "10.9.0.0/16"}},
		"addressGroups": map[string]any{"admins": map[string]any{"members": []any{"peer", "noc"}}},
		"services":      map[string]any{"ssh": map[string]any{"protocol": "tcp", "destinationPorts": []any{"22"}}, "https": map[string]any{"protocol": "tcp", "destinationPorts": []any{"443"}}},
		"serviceGroups": map[string]any{"mgmt": map[string]any{"members": []any{"ssh", "https"}}},
	})
	a.patch("/acl", map[string]any{
		"host": map[string]any{
			"local-in": map[string]any{"description": "management plane", "rules": []any{
				map[string]any{"sequence": 10, "action": "accept", "description": "admins: SSH + HTTPS", "source": map[string]any{"kind": "object", "name": "admins"}, "service": map[string]any{"kind": "object", "name": "mgmt"}, "log": true},
				map[string]any{"sequence": 20, "action": "accept", "description": "peer test service", "source": map[string]any{"kind": "object", "name": "peer"}, "service": tcp("2222")},
				map[string]any{"sequence": 30, "action": "drop", "description": "blocked test port", "service": tcp("2323"), "log": true},
				map[string]any{"sequence": 40, "action": "reject", "ipVersion": "ipv6", "service": map[string]any{"kind": "inline", "spec": map[string]any{"protocol": "udp", "destinationPorts": []any{"161-162"}}}},
			}},
			"egress": map[string]any{"description": "from the box", "rules": []any{
				map[string]any{"sequence": 10, "action": "drop", "destination": map[string]any{"kind": "prefix", "prefix": "203.0.113.0/24"}},
			}},
		},
		"hostAttachments": []any{
			map[string]any{"list": "local-in", "chain": "input", "priority": 0, "description": "local-in"},
			map[string]any{"list": "egress", "chain": "output", "priority": 0},
		},
		"hostSettings": map[string]any{"antiLockout": map[string]any{"enabled": true, "sources": []any{"10.9.0.0/16"}, "ports": []any{22, 443}}},
	})
	a.commit("host-acl-docs")
	for _, p := range []int{2222, 2323} {
		ns.listen(t, p)
	}
	for i := 0; i < 3; i++ {
		_ = ns.dial(2222, 2*time.Second)
		_ = ns.dial(2323, 500*time.Millisecond)
	}
	// an uncommitted change for the pending mark
	a.patch("/acl/host/egress", map[string]any{"description": "from the box (pending edit)"})

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
	a.must(200, "PUT", "/api/v1/config/acl", map[string]any{})
	a.must(200, "PUT", "/api/v1/config/objects", map[string]any{})
	a.commit("host-acl-docs-cleanup")
}
