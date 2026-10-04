package acl

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

// This file is staged beside the existing real ACL topology harness by run.py.
// Every assertion crosses the real API, real agent and disposable VPP.
func securityStack(t *testing.T) (*stack, rig, float64) {
	if os.Getenv("NGFW_DISPOSABLE_VPP") != "1" || os.Getenv("NGFW_TEST_PREFIX") != "w8" {
		t.Fatal("requires disposable VPP and reserved slot8")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	before := nRestarts(t)
	t.Cleanup(func() {
		if nRestarts(t) != before {
			t.Error("shared VPP restart count changed")
		}
	})
	r := newRig(s)
	conn := connectVPP(t)
	t.Log(mustRun(t, s.lab, "rig", "up", s.prefix))
	t.Cleanup(func() {
		out, err := run(t, s.lab, "rig", "down", s.prefix)
		t.Logf("rig cleanup %v %s", err, out)
		if err != nil {
			t.Error(err)
		}
	})
	r.peers(t, false)
	for _, line := range deleteBehindBack(t, conn, r.lanDev, r.wanDev) {
		t.Log(line)
	}
	hostNS := "ns-w8-host"
	peerNS := "ns-w8-hostpeer"
	for _, ns := range []string{hostNS, peerNS} {
		mustRun(t, "ip", "netns", "add", ns)
		namespace := ns
		t.Cleanup(func() { mustRun(t, "ip", "netns", "del", namespace) })
	}
	mustRun(t, "ip", "link", "add", "w8host0", "type", "veth", "peer", "name", "w8host1")
	mustRun(t, "ip", "link", "set", "w8host0", "netns", hostNS)
	mustRun(t, "ip", "link", "set", "w8host1", "netns", peerNS)
	for _, v := range []struct{ ns, dev, addr string }{{hostNS, "w8host0", "10.8.4.1/24"}, {peerNS, "w8host1", "10.8.4.2/24"}} {
		mustRun(t, "ip", "-n", v.ns, "addr", "add", v.addr, "dev", v.dev)
		mustRun(t, "ip", "-n", v.ns, "link", "set", v.dev, "up")
		mustRun(t, "ip", "-n", v.ns, "link", "set", "lo", "up")
	}
	t.Setenv("NGFW_HOST_ACL_NETNS", hostNS)
	st := newStack(t, s)
	a := st.api
	a.patch("/interfaces", map[string]any{r.lanIf: map[string]any{"enabled": true, "ipv4": []string{r.lanGW + "/24"}}, r.wanIf: map[string]any{"enabled": true, "ipv4": []string{r.wanGW + "/24"}}})
	rev := a.commit("security-baseline")["revision"].(map[string]any)["id"].(float64)
	t.Cleanup(func() { apiCleanup(t, a, r) })
	waitIfs(t, conn, r.lanIf, r.wanIf)
	st.preflight(t)
	r.peers(t, true)
	_, _ = run(t, "ip", "netns", "exec", r.lanNS, "ping", "-n", "-c", "2", "-W", "1", r.wanIP) // bounded ARP warmup, not acceptance
	securityPing(t, r.lanNS, r.wanIP, 3, true, "baseline")
	return st, r, rev
}
func securityPing(t *testing.T, ns, dst string, n int, pass bool, label string) {
	out, _ := run(t, "ip", "netns", "exec", ns, "ping", "-n", "-c", strconv.Itoa(n), "-W", "1", "-i", "0.2", dst)
	tx, rx := pingCounts(out)
	t.Logf("%s tx=%d rx=%d\n%s", label, tx, rx, out)
	if tx != n || (pass && rx != n) || (!pass && rx != 0) {
		t.Fatalf("%s expected pass=%v tx=%d, got tx=%d rx=%d", label, pass, n, tx, rx)
	}
}
func securityRollback(t *testing.T, st *stack, r rig, rev float64) {
	rr := st.api.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%.0f", rev), nil)
	t.Logf("rollback status %v", rr.body["status"])
	conn := connectVPP(t)
	if own := ownACLs(t, conn, st.s.prefix); len(own) != 0 {
		t.Fatalf("owned ACL residue %v", own)
	}
	securityPing(t, r.lanNS, r.wanIP, 3, true, "rollback restores forwarding")
}
func TestRuleExpiryRealAPI(t *testing.T) {
	st, r, rev := securityStack(t)
	a := st.api
	expires := time.Now().Add(12 * time.Second).UTC().Format(time.RFC3339Nano)
	a.patch("/acl", map[string]any{"lists": map[string]any{"temporary": map[string]any{"rules": []any{
		map[string]any{"sequence": 10, "action": "permit", "ipVersion": "ipv4", "expiresAt": expires, "owner": "netops", "ticket": "slot8-acceptance"}, map[string]any{"sequence": 20, "action": "deny", "ipVersion": "ipv4"},
	}}}, "attachments": []any{map[string]any{"list": "temporary", "target": map[string]any{"kind": "interface", "interface": r.lanIf}, "direction": "in", "sequence": 1}}})
	c := a.commit("expiry-live")
	t.Logf("commit status %v revision %v results %s", c["status"], c["revision"], js(c["results"]))
	committed := a.must(200, "GET", "/api/v1/config", nil).raw
	conn := connectVPP(t)
	initial := ownACLs(t, conn, st.s.prefix)["temporary"]
	if initial.rules != 2 {
		t.Fatalf("expected2 live rules, got %v", initial)
	}
	securityPing(t, r.lanNS, r.wanIP, 3, true, "before expiry")
	if !waitFor(25*time.Second, func() bool { return ownACLs(t, conn, st.s.prefix)["temporary"].rules == 1 }) {
		t.Fatal("expiry did not remove permit from VPP")
	}
	t.Log("expiry removed permit without a second commit: " + vppctl(t, "show", "acl-plugin", "acl"))
	securityPing(t, r.lanNS, r.wanIP, 3, false, "after expiry")
	if got := a.must(200, "GET", "/api/v1/config", nil).raw; got != committed {
		t.Fatal("running config changed across expiry")
	}
	securityRestart(t, st, r)
	if !waitFor(15*time.Second, func() bool { return ownACLs(t, conn, st.s.prefix)["temporary"].rules == 1 }) {
		t.Fatal("agent restart reinstalled expired permit")
	}
	securityPing(t, r.lanNS, r.wanIP, 3, false, "agent restart retains expiry")
	t.Log("Retrieve retains metadata: " + js(st.retrieveACL(t)))
	securityRollback(t, st, r, rev)
	securityPing(t, "ns-w8-hostpeer", "10.8.4.1", 3, true, "rollback restores host local-in")
}
func TestGlobalBlockingRealAPI(t *testing.T) {
	st, r, rev := securityStack(t)
	a := st.api
	securityPing(t, "ns-w8-hostpeer", "10.8.4.1", 3, true, "host local-in baseline")
	block := map[string]any{"enabled": true, "source": map[string]any{"kind": "upload"}, "allInterfaces": false, "interfaces": []string{r.lanIf}, "direction": "both", "protectHost": true, "entries": []string{r.lanIP + "/32", "10.8.4.2/32"}}
	a.patch("/acl", map[string]any{"globalBlocking": map[string]any{"lists": map[string]any{"bad": block}}})
	c := a.commit("global-block-live")
	t.Logf("commit status %v results %s", c["status"], js(c["results"]))
	conn := connectVPP(t)
	t.Log(vppctl(t, "show", "acl-plugin", "acl"))
	t.Log(vppctl(t, "show", "acl-plugin", "interface"))
	if len(ownACLs(t, conn, st.s.prefix)) == 0 {
		t.Fatal("no real global-block ACLs")
	}
	securityPing(t, "ns-w8-hostpeer", "10.8.4.1", 3, false, "host local-in listed source blocked")
	t.Log("isolated host nft readback: " + mustRun(t, "ip", "netns", "exec", "ns-w8-host", "nft", "-j", "list", "ruleset"))
	securityPing(t, r.lanNS, r.wanIP, 3, false, "listed source forward blocked")
	securityPing(t, r.wanNS, r.lanIP, 3, false, "listed endpoint reverse blocked")
	alt := "10.8.1.3"
	mustRun(t, "ip", "-n", r.lanNS, "addr", "add", alt+"/24", "dev", r.lanPeer)
	_, _ = run(t, "ip", "netns", "exec", r.lanNS, "ping", "-I", alt, "-n", "-c", "2", "-W", "1", r.wanIP)
	out, _ := run(t, "ip", "netns", "exec", r.lanNS, "ping", "-I", alt, "-n", "-c", "3", "-W", "1", r.wanIP)
	tx, rx := pingCounts(out)
	t.Log(out)
	if tx != 3 || rx != 3 {
		t.Fatal("unlisted source must pass")
	}
	securityRestart(t, st, r)
	securityPing(t, r.lanNS, r.wanIP, 3, false, "agent restart retains blocking")
	// Select only the WAN interface, then test traffic terminating on LAN's VPP
	// address: the listed source must now pass through the unselected LAN input.
	a.patch("/acl/globalBlocking/lists/bad/interfaces", []string{r.wanIf})
	a.commit("selection-change")
	securityPing(t, r.lanNS, r.lanGW, 3, true, "unselected LAN permits listed source")
	t.Log("Retrieve: " + js(st.retrieveACL(t)))

	securityRollback(t, st, r, rev)
	securityPing(t, "ns-w8-hostpeer", "10.8.4.1", 3, true, "rollback restores host local-in")
}

func securityRestart(t *testing.T, st *stack, r rig) {
	st.agent.stop(t)
	conn := connectVPP(t)
	idx := waitIfs(t, conn, r.lanIf, r.wanIf)
	// Simulated dataplane loss proves persisted replay rather than simply retaining
	// old VPP state. Only this disposable VPP and this slot's owned ACLs are touched.
	for _, i := range idx {
		setIfaceACLs(t, conn, i, 0)
	}
	for _, a := range ownACLs(t, conn, st.s.prefix) {
		delACL(t, conn, a.idx)
	}
	if len(ownACLs(t, conn, st.s.prefix)) != 0 {
		t.Fatal("simulated loss did not remove owned ACLs")
	}
	st.startAgent(t)
	if !waitFor(25*time.Second, func() bool { return len(ownACLs(t, conn, st.s.prefix)) > 0 }) {
		t.Fatal("agent restart did not rebuild removed ACLs")
	}
	t.Log("actual agent restart recreated removed owned ACLs from persisted desired state")
}
