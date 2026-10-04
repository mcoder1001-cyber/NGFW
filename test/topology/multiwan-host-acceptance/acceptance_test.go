package acl

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

const mwRouter = "ns-w7-mw-router"
const mwTarget = "198.18.7.1"
const mwData = "198.18.7.2"

func mwIP(t *testing.T, args ...string) string {
	return mustRun(t, "ip", append([]string{"-n", mwRouter}, args...)...)
}
func mwWait(t *testing.T, a *api, active string, timeout time.Duration) {
	t.Helper()
	var last resp
	if !waitFor(timeout, func() bool {
		last = a.call("GET", "/api/v1/state/wan", nil)
		groups, ok := last.body["groups"].([]any)
		if !ok || len(groups) != 1 {
			return false
		}
		g := groups[0].(map[string]any)
		return last.status == 200 && g["active"] == active
	}) {
		t.Fatalf("WAN did not converge to %s: %s", active, last.raw)
	}
	t.Logf("actual installed active=%s state=%s", active, last.raw)
}
func mwPing(t *testing.T, label string, ttl int) {
	t.Helper()
	fib := mustRun(t, "vppctl", "show", "ip", "fib")
	if strings.Contains(fib, mwData+"/32") {
		t.Fatal("fixture route shadows WAN default for data endpoint")
	}
	out, err := run(t, "ip", "netns", "exec", "ns-w7-mw-lan", "ping", "-n", "-c", "3", "-W", "1", "-i", "0.2", mwData)
	tx, rx := pingCounts(out)
	t.Logf("%s: %s", label, out)
	if err != nil || tx != 3 || rx != 3 || strings.Count(out, fmt.Sprintf("ttl=%d ", ttl)) != 3 {
		t.Log(mustRun(t, "vppctl", "show", "ip", "fib"))
		t.Log(mustRun(t, "vppctl", "show", "ip", "neighbors"))
		t.Fatalf("%s: require3/3 got %d/%d err=%v", label, rx, tx, err)
	}
}
func TestMultiWANRealAPI(t *testing.T) {
	conf, err := os.ReadFile("/run/vpp/startup.conf")
	hostMount, _ := os.Readlink("/proc/1/ns/mnt")
	ourMount, _ := os.Readlink("/proc/self/ns/mnt")
	if err != nil || os.Getenv("NGFW_DISPOSABLE_VPP") != "1" || os.Getenv("NGFW_TEST_PREFIX") != "w7" || hostMount == ourMount || !regexp.MustCompile(`(?m)^api-segment \{ prefix fulltest[0-9]+ \}$`).Match(conf) {
		t.Fatal("requires marked disposable VPP, private mount namespace, reserved slot7")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	before := nRestarts(t)
	t.Cleanup(func() {
		if nRestarts(t) != before {
			t.Error("shared VPP restart count changed")
		}
	})
	names := []string{"lan", "wan1", "wan2"}
	for i, name := range names {
		ns := "ns-w7-mw-" + name
		dev := fmt.Sprintf("w7m%d", i)
		peer := fmt.Sprintf("w7p%d", i)
		mustRun(t, "ip", "netns", "add", ns)
		t.Cleanup(func() { mustRun(t, "ip", "netns", "del", ns) })
		mustRun(t, "ip", "link", "add", dev, "type", "veth", "peer", "name", peer)
		mustRun(t, "ip", "link", "set", dev, "netns", mwRouter)
		mustRun(t, "ip", "link", "set", peer, "netns", ns)
		mustRun(t, "ip", "-n", ns, "link", "set", "lo", "up")
		mustRun(t, "ip", "netns", "exec", ns, "sysctl", "-qw", "net.ipv4.conf.all.arp_announce=2")
		mustRun(t, "ip", "-n", ns, "addr", "add", fmt.Sprintf("10.7.%d.2/24", i+1), "dev", peer)
		mustRun(t, "ip", "-n", ns, "link", "set", peer, "up")
		mwIP(t, "link", "set", dev, "up")
		mustRun(t, "ip", "-n", ns, "route", "add", "default", "via", fmt.Sprintf("10.7.%d.1", i+1))
		if i > 0 {
			mustRun(t, "ip", "netns", "exec", ns, "sysctl", "-qw", fmt.Sprintf("net.ipv4.ip_default_ttl=%d", 63+i))
			mustRun(t, "ip", "-n", ns, "addr", "add", mwTarget+"/32", "dev", "lo")
			mustRun(t, "ip", "-n", ns, "addr", "add", mwData+"/32", "dev", "lo")
		}
	}
	mwIP(t, "link", "set", "lo", "up")
	t.Log(mustRun(t, "ip", "netns", "exec", mwRouter, "sysctl", "-w", "net.ipv4.ping_group_range=0 0"))
	st := newStack(t, s)
	a := st.api
	t.Cleanup(func() {
		if raw, err := os.ReadFile(st.agentLog); err == nil {
			t.Logf("agent log: %s", raw)
		}
	})
	conn := connectVPP(t)
	// Fixture LCP addressing is local to the private router netns; no host routes/FRR are changed.
	ifs := map[string]any{}
	for i := 0; i < 3; i++ {
		value := map[string]any{"enabled": true, "ipv4": []string{fmt.Sprintf("10.7.%d.1/24", i+1)}}
		if i > 0 {
			value["lcp"] = map[string]any{"hostIfName": fmt.Sprintf("w7c%d", i), "hostIfType": "tap"}
		}
		ifs[fmt.Sprintf("host-w7m%d", i)] = value
	}
	a.patch("/interfaces", ifs)
	baseline := a.commit("multiwan-baseline")["revision"].(map[string]any)["id"].(float64)
	t.Cleanup(func() {
		for i := 0; i < 3; i++ {
			_, _ = run(t, "ip", "-n", mwRouter, "link", "set", fmt.Sprintf("w7m%d", i), "down")
		}
		a.patch("/routing", map[string]any{"wanGroups": []any{}})
		a.patch("/interfaces", map[string]any{"host-w7m0": nil, "host-w7m1": nil, "host-w7m2": nil})
		a.commit("multiwan-cleanup")
		remaining := dumpIfs(t, conn)
		for name := range remaining {
			if strings.HasPrefix(name, "host-w7m") || strings.HasPrefix(name, "w7c") {
				t.Errorf("VPP residue %s", name)
			}
		}
	})
	waitIfs(t, conn, "host-w7m0", "host-w7m1", "host-w7m2")
	st.preflight(t)
	for i := 1; i < 3; i++ {
		dev := fmt.Sprintf("w7c%d", i)
		mwIP(t, "addr", "replace", fmt.Sprintf("10.7.%d.1/24", i+1), "dev", dev)
		mwIP(t, "link", "set", dev, "up")
		mwIP(t, "route", "add", mwTarget+"/32", "via", fmt.Sprintf("10.7.%d.2", i+1), "dev", dev, "metric", fmt.Sprint(i))
		// Real device-bound ICMP readiness; this is setup, not the acceptance dataplane packet gate.
		out, err := run(t, "ip", "netns", "exec", mwRouter, "ping", "-I", dev, "-c", "2", "-W", "1", mwTarget)
		t.Logf("probe device %s readiness: %s", dev, out)
		if err != nil {
			t.Fatal(err)
		}
	}
	group := map[string]any{"name": "internet", "mode": "failover", "members": []any{
		map[string]any{"interface": "host-w7m1", "nextHop": "gateway", "gateway": "10.7.2.2", "priority": 10},
		map[string]any{"interface": "host-w7m2", "nextHop": "gateway", "gateway": "10.7.3.2", "priority": 20}},
		"monitors": []any{map[string]any{"type": "icmp", "target": mwTarget, "intervalMs": 1000, "timeoutMs": 200, "downAfter": 2, "upAfter": 2}}}
	a.patch("/routing", map[string]any{"wanGroups": []any{group}})
	c := a.commit("multiwan-live")
	t.Logf("real commit=%s", js(c))
	mwWait(t, a, "host-w7m1", 15*time.Second)
	_, _ = run(t, "ip", "netns", "exec", "ns-w7-mw-lan", "ping", "-c", "2", "-W", "1", mwData)
	mwPing(t, "preferred WAN packets", 63)
	failoverStarted := time.Now()
	mustRun(t, "ip", "-n", "ns-w7-mw-wan1", "link", "set", "w7p1", "down")
	mwWait(t, a, "host-w7m2", 4*time.Second)
	t.Logf("failover elapsed=%s", time.Since(failoverStarted))
	mwPing(t, "failover WAN packets", 64)
	restoreStarted := time.Now()
	mustRun(t, "ip", "-n", "ns-w7-mw-wan1", "link", "set", "w7p1", "up")
	mustRun(t, "ip", "-n", "ns-w7-mw-wan1", "route", "replace", "default", "via", "10.7.2.1")
	t.Log(mustRun(t, "ip", "-n", "ns-w7-mw-wan1", "route", "show"))
	t.Log(mwIP(t, "addr", "show", "dev", "w7c1"))
	t.Log(mwIP(t, "route", "show"))
	mwIP(t, "route", "replace", mwTarget+"/32", "via", "10.7.2.2", "dev", "w7c1", "metric", "1")
	capturePath := t.TempDir() + "/restore-capture.txt"
	capture := start(t, "restore capture", capturePath, os.Environ(), "ip", "netns", "exec", "ns-w7-mw-wan1", "timeout", "4", "tcpdump", "-n", "-l", "-i", "w7p1", "icmp or arp")
	time.Sleep(200 * time.Millisecond)
	outRestore, errRestore := run(t, "ip", "netns", "exec", mwRouter, "ping", "-I", "w7c1", "-c", "2", "-W", "1", mwTarget)
	t.Logf("restore direct probe err=%v %s", errRestore, outRestore)
	capture.stop(t)
	captured, _ := os.ReadFile(capturePath)
	t.Logf("restore peer capture: %s", captured)
	t.Log(mustRun(t, "ip", "-n", "ns-w7-mw-wan1", "addr", "show"))
	t.Log(mustRun(t, "vppctl", "show", "interface"))
	t.Log(mustRun(t, "vppctl", "show", "lcp"))
	t.Log(mustRun(t, "vppctl", "show", "ip", "fib"))
	mwWait(t, a, "host-w7m1", 6*time.Second)
	t.Logf("full failback elapsed=%s", time.Since(restoreStarted))
	if time.Since(restoreStarted) > 6*time.Second {
		t.Fatal("full failback exceeded6s")
	}
	mwPing(t, "restored preferred WAN packets", 63)
	st.agent.stop(t)
	st.startAgent(t)
	mwWait(t, a, "host-w7m1", 15*time.Second)
	mwPing(t, "restart packets", 63)
	rollback := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%.0f", baseline), nil)
	if rollback.body["status"] != "applied" {
		t.Fatalf("rollback not applied: %s", rollback.raw)
	}
	var state resp
	if !waitFor(3*time.Second, func() bool {
		state = a.call("GET", "/api/v1/state/wan", nil)
		groups, ok := state.body["groups"].([]any)
		return state.status == 200 && ok && len(groups) == 0 && state.body["agentError"] == nil
	}) {
		t.Fatalf("rollback retained WAN group: %s", state.raw)
	}
	t.Logf("rollback WAN state %s", state.raw)
	out, err := run(t, "ip", "netns", "exec", "ns-w7-mw-lan", "ping", "-c", "3", "-W", "1", mwData)
	tx, rx := pingCounts(out)
	t.Logf("rollback packets: %s", out)
	if err == nil || tx != 3 || rx != 0 {
		t.Fatalf("rollback expected no default and0/3 got%d/%d err%v", rx, tx, err)
	}
	fib := mustRun(t, "vppctl", "show", "ip", "fib")
	t.Log(fib)
	defaultBlock := regexp.MustCompile(`(?s)0\.0\.0\.0/0\r?\n(.*?)0\.0\.0\.0/32`).FindStringSubmatch(fib)
	if len(defaultBlock) != 2 || !strings.Contains(defaultBlock[1], "dpo-drop ip4") || strings.Contains(defaultBlock[1], "ipv4 via") {
		t.Fatal("rollback retained a forwarding default route")
	}
}
