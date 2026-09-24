package neighborsra

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ngfw/agent/binapi/ip_neighbor"
)

// TestNeighborsRaScreenshots is the evidence run for docs/status/tasks/F-neighbors-ra.md and docs/user/routing/
// neighbors-ra.md: the same stack as TestNeighborsRaHost (real agent + API + host VPP, prefixed loopbacks, learned entries
// injected with ip_neighbor_add_del flags NONE — no packets), plus `vite preview` of the production web build
// (apps/web/dist) on the slot web port, while a headless browser (an external node script: playwright-core + Chrome
// from env paths, nothing installed, nothing committed — the P07a/P07b/P08 approach) takes the screenshots.
//
//	VRX_NRA_SHOTS=<node script> VRX_NRA_SHOTS_OUT=<dir> run.sh -run TestNeighborsRaScreenshots
//
// The script is called as: node <script> <baseUrl> <outDir> <adminPasswordFile> <lan interface>.
func TestNeighborsRaScreenshots(t *testing.T) {
	script, out := os.Getenv("VRX_NRA_SHOTS"), os.Getenv("VRX_NRA_SHOTS_OUT")
	if os.Getenv("VRX_INTEGRATION") != "1" || script == "" || out == "" {
		t.Skip("screenshot evidence run: set VRX_INTEGRATION=1, VRX_NRA_SHOTS (node script) and VRX_NRA_SHOTS_OUT")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	restarts0 := nRestarts(t)
	t.Cleanup(func() {
		if n := nRestarts(t); n != restarts0 {
			t.Errorf("VPP restarted during the run: NRestarts %d → %d", restarts0, n)
		}
	})
	nm := newNames(s)
	conn := connectVPP(t)
	st := newStack(t, s)
	a := st.api
	t.Cleanup(func() { cleanup(t, a, conn, nm) })

	webPort := os.Getenv("VRX_WEB_PORT")
	if webPort == "" {
		t.Fatal("VRX_WEB_PORT unset (eval \"$(tools/lab env <slot>)\")")
	}
	web := filepath.Join(s.repo, "apps", "web")
	if _, err := os.Stat(filepath.Join(web, "dist", "index.html")); err != nil {
		t.Fatalf("apps/web/dist missing — build the web app first: %v", err)
	}
	env := append(os.Environ(), "VRX_HTTP_PORT="+s.httpPort, "VRX_WEB_PORT="+webPort)
	pv := start(t, "vite-preview", filepath.Join(s.runDir, "nra", "vite.log"), env, filepath.Join(web, "node_modules", ".bin", "vite"), "preview", web)
	t.Cleanup(func() { pv.stop(t) })

	a.patch("", map[string]any{
		"vrfs": map[string]any{nm.vrf: map[string]any{"id": nm.table, "proxyArpRanges": []any{map[string]any{"low": nm.rangeLo, "high": nm.rangeHi}}}},
		"interfaces": map[string]any{
			nm.lo1: map[string]any{"enabled": true, "description": "LAN (RA + SLAAC)", "vrf": nm.vrf, "ipv4": []string{nm.net1 + ".1/24"}, "ipv6": []string{nm.v6a + "::1/64"},
				"proxyArp": true,
				"ipv6Ra": map[string]any{"suppress": false, "other": true, "lifetimeSec": 1800, "maxIntervalSec": 600, "minIntervalSec": 200,
					"prefixes": map[string]any{nm.v6a + "::/64": map[string]any{}}}},
			nm.lo2: map[string]any{"enabled": true, "description": "servers", "ipv4": []string{nm.net2 + ".1/24"}, "ipv6": []string{nm.v6b + "::1/64"}},
		},
		"routing": map[string]any{"neighbors": map[string]any{"static": []any{
			map[string]any{"interface": nm.lo1, "ip": nm.net1 + ".50", "mac": "02:00:00:00:91:50"},
			map[string]any{"interface": nm.lo2, "ip": nm.v6b + "::50", "mac": "02:00:00:00:92:50", "noFibEntry": true},
		}}},
	})
	a.commit("nra-docs")
	i1, i2 := ifIndex(t, conn, nm.lo1), ifIndex(t, conn, nm.lo2)
	for k, e := range []struct {
		idx     uint32
		ip, mac string
	}{
		{i1, nm.net1 + ".20", "02:00:00:00:91:20"}, {i1, nm.net1 + ".21", "02:00:00:00:91:21"}, {i1, nm.net1 + ".22", "02:00:00:00:91:22"},
		{i1, nm.v6a + "::20", "02:00:00:00:91:a0"}, {i1, "fe80::20", "02:00:00:00:91:a0"},
		{i2, nm.net2 + ".30", "02:00:00:00:92:30"}, {i2, nm.net2 + ".31", "02:00:00:00:92:31"}, {i2, nm.v6b + "::31", "02:00:00:00:92:31"},
	} {
		neighbor(t, conn, e.idx, e.ip, e.mac, ip_neighbor.IP_API_NEIGHBOR_FLAG_NONE, true)
		if k%3 == 2 {
			time.Sleep(700 * time.Millisecond) // different ages in the table
		}
	}
	// a pending (uncommitted) change so the pending-change bar shows the diff of a neighbour edit
	a.patch("/routing/neighbors", map[string]any{"static": []any{
		map[string]any{"interface": nm.lo1, "ip": nm.net1 + ".50", "mac": "02:00:00:00:91:50"},
		map[string]any{"interface": nm.lo2, "ip": nm.v6b + "::50", "mac": "02:00:00:00:92:50", "noFibEntry": true},
		map[string]any{"interface": nm.lo1, "ip": nm.net1 + ".60", "mac": "02:00:00:00:91:60"},
	}})
	pwFile := filepath.Join(s.runDir, "nra", "admin.pw")
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
	outp, err := run(t, "node", script, "http://127.0.0.1:"+webPort, out, pwFile, nm.lo1)
	t.Log("screenshots:\n" + strings.TrimSpace(outp))
	if err != nil {
		t.Fatalf("screenshot script: %v", err)
	}
	nbrs := vppctl(t, "show", "ip", "neighbors")
	t.Logf("vppctl show ip neighbors after the UI flush of %s (ours):\n%s", nm.lo1, strings.Join(append(linesWith(nbrs, nm.lo1), linesWith(nbrs, nm.lo2)...), "\n"))
	audit := a.must(200, "GET", "/api/v1/audit?limit=3", nil)
	t.Logf("audit (newest first): %s", trunc(audit.raw, 700))
	if !strings.Contains(audit.raw, `"action":"POST /api/v1/actions/arp-flush"`) {
		t.Fatal("the UI flush was not audited")
	}
	if l := linesWith(nbrs, nm.lo1); len(l) != 1 || !strings.Contains(l[0], nm.net1+".50") {
		t.Fatalf("after the UI flush only the static entry may stay on %s:\n%s", nm.lo1, strings.Join(l, "\n"))
	}
}
