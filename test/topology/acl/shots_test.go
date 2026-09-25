package acl

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ngfw/agent/binapi/acl_types"
)

// TestACLScreenshots is the evidence run for docs/user/firewall/acl.md: the stack, rig and V19 checks of
// TestACLTopology with a committed list (zone attachment, a foreign ACL first on the port, MACIP on the wan port) and
// ping traffic for the hit counters, plus an UNCOMMITTED list of VRX_ACL_SHOTS_RULES rules (default 100 000, imported
// through the CSV route into the candidate — the editor pages the candidate) for the rule editor at scale; `vite preview`
// of the production web build (apps/web/dist) on the slot web port and an external headless-browser script
// (playwright-core + Chrome-for-Testing from env paths, nothing installed, nothing committed — P07a/P07b/P08).
//
//	VRX_ACL_SHOTS=<node script> VRX_ACL_SHOTS_OUT=<dir> run.sh -run TestACLScreenshots
//
// The script is called as: node <script> <baseUrl> <outDir> <adminPasswordFile> <bigListName>.
func TestACLScreenshots(t *testing.T) {
	script, out := os.Getenv("VRX_ACL_SHOTS"), os.Getenv("VRX_ACL_SHOTS_OUT")
	if os.Getenv("VRX_INTEGRATION") != "1" || script == "" || out == "" {
		t.Skip("screenshot evidence run: set VRX_INTEGRATION=1, VRX_ACL_SHOTS (node script) and VRX_ACL_SHOTS_OUT")
	}
	big := 100_000
	if n, err := strconv.Atoi(os.Getenv("VRX_ACL_SHOTS_RULES")); err == nil && n > 0 {
		big = n
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

	webPort := os.Getenv("VRX_WEB_PORT")
	if webPort == "" {
		t.Fatal("VRX_WEB_PORT unset (eval \"$(tools/lab env <slot>)\")")
	}
	web := filepath.Join(s.repo, "apps", "web")
	env := append(os.Environ(), "VRX_HTTP_PORT="+s.httpPort, "VRX_WEB_PORT="+webPort)
	pv := start(t, "vite-preview", filepath.Join(st.work, "vite.log"), env, filepath.Join(web, "node_modules", ".bin", "vite"), "preview", web)
	t.Cleanup(func() { pv.stop(t) })

	a.patch("/interfaces", map[string]any{
		r.lanIf: map[string]any{"enabled": true, "description": "LAN (rig)", "ipv4": []string{r.lanGW + "/24"}},
		r.wanIf: map[string]any{"enabled": true, "description": "WAN (rig)", "ipv4": []string{r.wanGW + "/24"}},
	})
	a.commit("acl-shots-interfaces")
	idx := waitIfs(t, conn, r.lanIf, r.wanIf)
	v19Guard(t, conn, idx)
	foreign, _ := addACL(t, conn, s.prefix+"-foreign:guard", []acl_types.ACLRule{foreignRule(s.num)})
	setIfaceACLs(t, conn, idx[r.lanIf], 1, foreign)
	t.Cleanup(func() {
		n, acls := ifaceACLs(t, conn, idx[r.lanIf])
		var keep []uint32
		in := n
		for i, x := range acls {
			if x == foreign {
				if i < int(n) {
					in--
				}
				continue
			}
			keep = append(keep, x)
		}
		setIfaceACLs(t, conn, idx[r.lanIf], in, keep...)
		delACL(t, conn, foreign)
	})
	a.patch("/objects", map[string]any{
		"tags":          map[string]any{"prod": map[string]any{"color": "#1e88e5"}},
		"addresses":     map[string]any{"wan-host": map[string]any{"type": "host", "address": r.wanIP}, "lan": map[string]any{"type": "network", "prefix": "10." + strconv.Itoa(s.num) + ".1.0/24"}},
		"addressGroups": map[string]any{"wan-hosts": map[string]any{"members": []string{"wan-host"}}},
		"services":      map[string]any{"echo": map[string]any{"protocol": "icmp", "type": 8}, "https": map[string]any{"protocol": "tcp", "destinationPorts": []string{"443"}}},
		"schedules":     map[string]any{"office-hours": map[string]any{"type": "recurring", "days": []string{"mon", "tue", "wed", "thu", "fri"}, "start": "08:00", "end": "18:00"}},
		"zones":         map[string]any{"lan": map[string]any{"interfaces": []string{r.lanIf}}},
	})
	a.patch("/acl", map[string]any{
		"lists": map[string]any{
			"lan-in": map[string]any{"description": "LAN ingress", "tags": []string{"prod"}, "rules": []any{
				map[string]any{"sequence": 10, "action": "permit", "description": "echo to the wan hosts", "source": map[string]any{"kind": "object", "name": "lan"},
					"destination": map[string]any{"kind": "object", "name": "wan-hosts"}, "service": map[string]any{"kind": "object", "name": "echo"}},
				map[string]any{"sequence": 20, "action": "reflect", "description": "web, stateful", "destination": map[string]any{"kind": "object", "name": "wan-hosts"},
					"service": map[string]any{"kind": "object", "name": "https"}, "schedule": "office-hours"},
				map[string]any{"sequence": 25, "action": "permit", "enabled": false, "description": "maintenance (off)"},
				map[string]any{"sequence": 30, "action": "deny", "ipVersion": "ipv4", "description": "everything else (IPv4)"},
				map[string]any{"sequence": 40, "action": "permit", "ipVersion": "ipv6"},
			}},
		},
		"macip":            map[string]any{"wan-l2": map[string]any{"rules": []any{map[string]any{"sequence": 10, "action": "permit", "sourceMac": r.peerMAC(t), "sourcePrefix": r.wanIP + "/32"}}}},
		"attachments":      []any{map[string]any{"list": "lan-in", "target": map[string]any{"kind": "zone", "zone": "lan"}, "direction": "in", "sequence": 10}},
		"macipAttachments": []any{map[string]any{"list": "wan-l2", "interface": r.wanIf}},
	})
	a.commit("acl-shots")
	if countersFlag(t, conn) {
		st.preflight(t)
		r.peers(t, true)
		for _, dst := range []string{r.wanIP, r.wanIP, r.wanGW} {
			out, rx := r.ping(t, dst, 4)
			t.Logf("ping %s: %d received\n%s", dst, rx, strings.TrimSpace(out))
		}
	} else {
		t.Log("counters flag off: the screen shows counters unavailable")
	}

	// the big list, in the candidate only (the rule editor pages the candidate)
	var csv strings.Builder
	csv.WriteString("sequence,action,source,destination,service,description\n")
	for i := 0; i < big; i++ {
		fmt.Fprintf(&csv, "%d,%s,10.%d.%d.%d/32,172.%d.%d.%d/32,tcp:443,generated rule %d\n", (i+1)*10, map[bool]string{true: "permit", false: "deny"}[i%7 != 3],
			s.num, 100+i/65536, i/256%256, 16+i/65536, i/256%256, i%256, i+1)
	}
	t0 := time.Now()
	imp := a.raw("POST", "/api/v1/actions/acl/import?list=scale&dryRun=false", "text/csv", csv.String())
	t.Logf("CSV import of %d rules into the candidate → %d in %.1f s", big, imp.status, time.Since(t0).Seconds())
	if imp.status != 200 {
		t.Fatalf("import: %s", trunc(imp.raw, 800))
	}

	pwFile := filepath.Join(st.work, "admin.pw")
	if err := os.WriteFile(pwFile, []byte(st.adminPW), 0o600); err != nil {
		t.Fatal(err)
	}
	if !waitFor(30*time.Second, func() bool {
		o, err := run(t, "curl", "-sf", "-o", "/dev/null", "http://127.0.0.1:"+webPort+"/")
		return err == nil && o == ""
	}) {
		t.Fatal("vite preview did not come up")
	}
	outp, err := run(t, "node", script, "http://127.0.0.1:"+webPort, out, pwFile, "scale")
	t.Log("screenshots:\n" + strings.TrimSpace(outp))
	if err != nil {
		t.Fatalf("screenshot script: %v", err)
	}
	r.peers(t, false)
	a.must(200, "POST", "/api/v1/config/discard", nil)
	a.must(200, "PUT", "/api/v1/config/acl", map[string]any{})
	a.must(200, "PUT", "/api/v1/config/objects", map[string]any{})
	for _, n := range []string{r.lanIf, r.wanIf} {
		a.must(200, "DELETE", "/api/v1/config/interfaces/"+n, nil)
	}
	c := a.call("POST", "/api/v1/config/commit?comment=acl-shots-cleanup", nil)
	t.Logf("cleanup commit → %d", c.status)
	if own := ownACLs(t, conn, s.prefix); len(own) != 0 {
		t.Errorf("ACLs left: %v", own)
	}
}
