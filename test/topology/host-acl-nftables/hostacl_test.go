package hostacl

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestHostACLTopology is F-host-acl-nftables' acceptance run on the slot (VRX_INTEGRATION=1, lab lock shared): the real
// agent renders `table inet vrx_<prefix>` into the slot namespace ns-<prefix>-hacl (VRX_HOST_ACL_NETNS), the real API
// drives it through candidate → commit → rollback, and packets come from the veth peer ns-<prefix>-hpeer.
//
//  1. commit → `nft list table` in the namespace shows the rules; /state/drift is clean for acl (Retrieve == running);
//     /state/host-acl reports present + in sync
//  2. an allowed port connects, a dropped port times out and its rule counter increments (/state/host-acl)
//  3. a candidate whose rules would drop the management SSH source (anti-lockout off) → 400 problem+json with a pointer
//  4. commit B, then rollback to A → the kernel table equals A's again (listing and Retrieve, not assumption)
//  5. agent restart simulation: agent stopped, table deleted (simulated loss), agent started → table re-rendered
//     identically within 30 s (log excerpt)
//  6. a foreign table in the namespace and the root netns ruleset (`nft list tables`) are untouched throughout
func TestHostACLTopology(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("topology test: VRX_INTEGRATION=1 and a slot (eval \"$(tools/lab env <N>)\"; run.sh)")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	rootBefore := rootTables(t)
	t.Logf("root netns `nft list tables` before:\n%s", rootBefore)
	t.Cleanup(func() { // registered first → runs last, after the stack and the namespaces are gone
		after := rootTables(t)
		t.Logf("root netns `nft list tables` after:\n%s", after)
		if after != rootBefore {
			t.Errorf("the root netns ruleset changed:\nbefore:\n%s\nafter:\n%s", rootBefore, after)
		}
	})
	restarts := nRestarts(t)
	ns := newNetns(t, s)
	ns.nft(t, "-f", writeTemp(t, "table inet foreign {\n\tchain c {\n\t\ttype filter hook input priority 100; policy accept;\n\t\tcounter\n\t}\n}\n"))
	foreign := ns.listing(t, "foreign")

	st := newStack(t, s, ns.host)
	a := st.api
	table := "vrx_" + s.prefix

	// 1. commit A
	a.patch("/objects", map[string]any{"addresses": map[string]any{"peer": map[string]any{"type": "host", "address": ns.peerAddr}}})
	a.patch("/acl", aclDoc(ns.peerAddr, false))
	revA := revisionOf(t, a.commit("host-acl-a"))
	raw := ns.nft(t, "list", "table", "inet", table)
	t.Logf("`nft list table inet %s` in %s after commit (revision %d):\n%s", table, ns.host, revA, raw)
	listingA := ns.listing(t, table)
	t.Logf("`nft list tables` in %s:\n%s", ns.host, ns.nft(t, "list", "tables"))
	driftClean(t, a)
	state := a.must(200, "GET", "/api/v1/state/host-acl", nil).body
	if state["present"] != true || state["inSync"] != true || state["mode"] != "netns" || state["table"] != table {
		t.Fatalf("state: %v", state)
	}
	t.Logf("retrieve acl (vrx-agentctl): %v", st.retrieveACL(t))

	// 2. packets from the veth peer
	for _, p := range []int{2222, 2323} {
		ns.listen(t, p)
	}
	if err := ns.dial(2222, 3*time.Second); err != nil {
		t.Fatalf("allowed port 2222: %v", err)
	}
	if err := ns.dial(2323, 1500*time.Millisecond); err == nil {
		t.Fatal("port 2323 must be dropped")
	} else {
		t.Logf("port 2323 from %s: %v (dropped)", ns.peerAddr, err)
	}
	var drops, accepts string
	if !waitFor(10*time.Second, func() bool {
		drops, accepts = ruleCounter(a, "local-in", 20), ruleCounter(a, "local-in", 10)
		return drops != "" && drops != "0" && accepts != "0"
	}) {
		t.Fatalf("counters did not increment: drop=%q accept=%q", drops, accepts)
	}
	t.Logf("GET /api/v1/state/host-acl rules[]: sequence 20 (drop 2323) packets=%s, sequence 10 (accept 2222) packets=%s", drops, accepts)

	// 3. anti-lockout: a rule set that would drop the management SSH source → 400 with a pointer
	a.patch("/acl", map[string]any{"hostSettings": map[string]any{"antiLockout": map[string]any{"enabled": false}}})
	rules := aclDoc(ns.peerAddr, false)["host"].(map[string]any)["local-in"].(map[string]any)["rules"].([]any)
	rules = append([]any{map[string]any{"sequence": 5, "action": "drop", "service": tcp("22")}}, rules...)
	a.patch("/acl/host/local-in", map[string]any{"rules": rules})
	bad := a.call("POST", "/api/v1/config/validate", nil)
	t.Logf("validate with anti-lockout off and a drop of tcp/22 → %d %s", bad.status, bad.raw)
	if bad.status != 400 || !strings.Contains(bad.raw, `"pointer":"/acl/host/local-in/rules/0"`) || !strings.Contains(bad.raw, "acl.host-anti-lockout") {
		t.Fatalf("want 400 problem+json at /acl/host/local-in/rules/0")
	}
	if c := a.call("POST", "/api/v1/config/commit?comment=lockout", nil); c.status != 400 {
		t.Fatalf("commit of a lockout: %d %s", c.status, c.raw)
	}
	a.must(200, "POST", "/api/v1/config/discard", nil)
	if ns.listing(t, table) != listingA {
		t.Fatal("a refused commit changed the table")
	}

	// 4. commit B (reject 2424), rollback to A
	a.patch("/acl", aclDoc(ns.peerAddr, true))
	revB := revisionOf(t, a.commit("host-acl-b"))
	listingB := ns.listing(t, table)
	if listingB == listingA || !strings.Contains(listingB, "tcp dport 2424") {
		t.Fatalf("revision B not rendered:\n%s", listingB)
	}
	rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=host-acl-rollback", revA), nil).body
	t.Logf("rollback %d → %d: status %v, revision %v", revB, revA, rb["status"], rb["revision"])
	if rb["status"] != "applied" {
		t.Fatalf("rollback: %v", rb)
	}
	if got := ns.listing(t, table); got != listingA {
		t.Fatalf("after rollback the table differs from revision %d:\n%s\n---\n%s", revA, got, listingA)
	}
	driftClean(t, a)
	t.Logf("rollback: `nft list table inet %s` equals revision %d's (counters blanked); /state/drift clean for acl", table, revA)

	// 5. restart simulation
	st.agent.stop(t)
	ns.nft(t, "delete", "table", "inet", table)
	t.Logf("simulated loss: `nft list tables` in %s with the agent down:\n%s", ns.host, ns.nft(t, "list", "tables"))
	from := fileSize(st.agentLog)
	start := time.Now()
	st.startAgent(t)
	if !waitFor(30*time.Second, func() bool {
		out, err := run("ip", "netns", "exec", ns.host, "nft", "list", "table", "inet", table)
		return err == nil && counterRe.ReplaceAllString(out, "counter") == listingA
	}) {
		t.Fatalf("table not re-rendered within 30 s:\n%s", strings.Join(st.agentLogLines(t, from, "host firewall", "resync", "reconcile"), "\n"))
	}
	t.Logf("restart: table re-rendered identically after %s (diff empty); agent log:\n%s", time.Since(start).Round(time.Millisecond),
		strings.Join(st.agentLogLines(t, from, "host firewall", "resync finished", "reconcile done"), "\n"))
	driftClean(t, a)

	// 6. untouched
	if got := ns.listing(t, "foreign"); got != foreign {
		t.Fatalf("foreign table changed:\n%s\n---\n%s", got, foreign)
	}
	if n := nRestarts(t); n != restarts {
		t.Fatalf("VPP NRestarts %d → %d", restarts, n)
	}
	t.Logf("VPP NRestarts %d → %d", restarts, nRestarts(t))

	// cleanup: remove the host firewall through the API → the table is gone, the foreign one stays
	a.must(200, "PUT", "/api/v1/config/acl", map[string]any{})
	a.commit("host-acl-cleanup")
	if tables := ns.nft(t, "list", "tables"); tables != "table inet foreign\n" {
		t.Fatalf("after removal: %q", tables)
	}
}

func tcp(ports ...string) map[string]any {
	return map[string]any{"kind": "inline", "spec": map[string]any{"protocol": "tcp", "destinationPorts": ports}}
}

// aclDoc is revision A (accept 2222 from the peer object, drop 2323) or B (+ reject 2424).
func aclDoc(peer string, withB bool) map[string]any {
	rules := []any{
		map[string]any{"sequence": 10, "action": "accept", "description": "peer may use 2222", "source": map[string]any{"kind": "object", "name": "peer"}, "service": tcp("2222")},
		map[string]any{"sequence": 20, "action": "drop", "log": true, "service": tcp("2323")},
	}
	if withB {
		rules = append(rules, map[string]any{"sequence": 30, "action": "reject", "service": tcp("2424")})
	}
	return map[string]any{
		"host":            map[string]any{"local-in": map[string]any{"description": "local-in", "rules": rules}},
		"hostAttachments": []any{map[string]any{"list": "local-in", "chain": "input", "priority": 0}},
		"hostSettings":    map[string]any{"antiLockout": map[string]any{"enabled": true, "sources": []any{peer + "/32"}, "ports": []any{22}}},
	}
}

func revisionOf(t *testing.T, body map[string]any) int {
	t.Helper()
	rev, _ := body["revision"].(map[string]any)
	id, ok := rev["id"].(float64)
	if !ok {
		t.Fatalf("commit without a revision id: %v", body)
	}
	return int(id)
}

// driftClean: GET /api/v1/state/drift has no change under /acl (the agent's Retrieve equals the running acl).
func driftClean(t *testing.T, a *api) {
	t.Helper()
	d := a.must(200, "GET", "/api/v1/state/drift", nil)
	for _, c := range d.body["changes"].([]any) {
		if p := fmt.Sprint(c.(map[string]any)["pointer"]); strings.HasPrefix(p, "/acl") {
			t.Fatalf("drift under /acl: %s", d.raw)
		}
	}
	if !strings.Contains(fmt.Sprint(d.body["subsystems"]), "acl") {
		t.Fatalf("acl is not among the drift subsystems: %s", d.raw)
	}
}

// ruleCounter is rules[].packets of list/sequence in /state/host-acl ("" when absent).
func ruleCounter(a *api, list string, seq int) string {
	r := a.call("GET", "/api/v1/state/host-acl", nil)
	items, _ := r.body["rules"].([]any)
	for _, it := range items {
		m := it.(map[string]any)
		if m["list"] == list && fmt.Sprint(m["sequence"]) == fmt.Sprint(seq) {
			return fmt.Sprint(m["packets"])
		}
	}
	return ""
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.nft")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	return f.Name()
}
