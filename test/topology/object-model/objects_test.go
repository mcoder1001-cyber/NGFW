package objectmodel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestObjectModelTopology is F-object-model's end-to-end run on the slot: the real agent (objects domain, FQDN resolver
// against the test's DNS responder), the real API on the slot database, no VPP object and no rig.
//
//	commit objects + an ACL using them → the agent's Retrieve (vrx-agentctl) equals the committed objects, /state/drift is
//	clean → the FQDN objects resolve → a changed answer is picked up by the 30 s refresh → the agent is stopped, its objects
//	store deleted (simulated loss) and started again: it reloads the FQDN answers without querying and the resync
//	recreates the store → the resolver goes down: last-good answers kept → deleting a referenced object is refused with the
//	rule's pointer → rollback restores the earlier object set (Retrieve + where-used) → everything is deleted again.
func TestObjectModelTopology(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("F-object-model topology test: set VRX_INTEGRATION=1 (real agent + API + slot PostgreSQL) — run.sh does")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	restarts0 := nRestarts(t)
	t.Logf("systemctl show vpp -p NRestarts (before) = %d", restarts0)
	t.Cleanup(func() {
		n := nRestarts(t)
		t.Logf("systemctl show vpp -p NRestarts (after) = %d", n)
		if n != restarts0 {
			t.Errorf("VPP restarted during the test: NRestarts %d → %d", restarts0, n)
		}
	})

	zone := s.prefix + ".test" // the test-only zone of this slot
	cdn, web := "cdn."+zone, "web."+zone
	dns := startDNS(t)
	dns.set(cdn, "192.0.2.53", "2001:db8::53")
	dns.set(web, "198.51.100.80")
	t.Logf("DNS responder %s serving %s", dns.addr, zone)
	const refresh = 30 * time.Second
	st := newStack(t, s, dns.addr, refresh)
	a := st.api

	objects, acl := sampleObjects(s, cdn, web), sampleACL()

	var rev1 float64
	var lastResolved string
	t.Run("commit-retrieve-drift", func(t *testing.T) {
		a.t = t
		a.patch("/objects", objects)
		a.patch("/acl", acl)
		c := a.commit("objects")
		rev1, _ = c["revision"].(map[string]any)["id"].(float64)
		t.Logf("commit: status=%v revision=%v notApplied=%v summary=%v", c["status"], rev1, c["notApplied"], c["summary"])
		running := a.must(200, "GET", "/api/v1/config/objects", nil).body
		got := st.retrieveObjects(t)
		if !reflect.DeepEqual(normalize(got), normalize(running)) {
			t.Fatalf("agent Retrieve != committed objects:\nretrieve %s\nrunning  %s", js(normalize(got)), js(normalize(running)))
		}
		t.Logf("vrx-agentctl retrieve -subsystems objects == running objects (%d kinds, %d address objects)", len(normalize(got)), len(got["addresses"].(map[string]any)))
		d := a.must(200, "GET", "/api/v1/state/drift", nil).body
		for _, ch := range d["changes"].([]any) {
			if p, _ := ch.(map[string]any)["pointer"].(string); strings.HasPrefix(p, "/objects") {
				t.Fatalf("drift in objects: %v", ch)
			}
		}
		t.Logf("/state/drift: subsystems=%v changes=%d (none under /objects)", d["subsystems"], len(d["changes"].([]any)))
	})

	t.Run("fqdn-resolve-and-refresh", func(t *testing.T) {
		a.t = t
		var item map[string]any
		if !waitFor(15*time.Second, func() bool {
			item = fqdnItem(a, "cdn")
			return item != nil && strings.Join(strs(item["addresses"]), " ") == "192.0.2.53 2001:db8::53"
		}) {
			t.Fatalf("cdn not resolved: %v", item)
		}
		lastResolved, _ = item["lastResolved"].(string)
		t.Logf("resolved: %s", js(item))
		if site := fqdnItem(a, "site"); strings.Join(strs(site["addresses"]), " ") != "198.51.100.80" || site["error"] != "" {
			t.Fatalf("site (v4 only): %v", site)
		}
		// a changed answer is picked up by the next refresh (fixed interval, here 30 s)
		dns.set(cdn, "192.0.2.54", "2001:db8::53")
		if !waitFor(refresh+20*time.Second, func() bool {
			item = fqdnItem(a, "cdn")
			return strings.Join(strs(item["addresses"]), " ") == "192.0.2.54 2001:db8::53"
		}) {
			t.Fatalf("refresh not observed: %v", item)
		}
		l1, _ := time.Parse(time.RFC3339Nano, lastResolved)
		l2, _ := time.Parse(time.RFC3339Nano, item["lastResolved"].(string))
		t.Logf("refresh observed: lastResolved %s → %s (%.1f s), addresses %v", lastResolved, item["lastResolved"], l2.Sub(l1).Seconds(), item["addresses"])
		if d := l2.Sub(l1); d < refresh-time.Second || d > refresh+15*time.Second {
			t.Fatalf("refresh interval %v, want ≈ %v", d, refresh)
		}
		lastResolved = item["lastResolved"].(string)
		for _, l := range st.agentLogLines(t, 0, `"fqdn resolved"`) {
			t.Log("agent: " + l)
		}
	})

	t.Run("agent-restart-and-simulated-loss", func(t *testing.T) {
		a.t = t
		before := st.retrieveObjects(t)
		st.agent.stop(t)
		store := filepath.Join(st.stateDir, "objects-"+s.prefix+".json")
		if err := os.Remove(store); err != nil {
			t.Fatalf("simulated loss: %v", err)
		}
		t.Logf("agent stopped; %s deleted (simulated loss of the applied object set); FQDN state kept", store)
		q0, off := dns.count(), fileSize(st.agentLog)
		started := time.Now()
		st.startAgent(t)
		var got map[string]any
		if !waitFor(30*time.Second, func() bool {
			got = st.retrieveObjects(t)
			return reflect.DeepEqual(normalize(got), normalize(before))
		}) {
			t.Fatalf("objects not back within 30 s:\n%s", js(normalize(got)))
		}
		t.Logf("objects back after %.2f s (Retrieve == before the restart)", time.Since(started).Seconds())
		item := fqdnItem(a, "cdn")
		if item["lastResolved"] != lastResolved || strings.Join(strs(item["addresses"]), " ") != "192.0.2.54 2001:db8::53" {
			t.Fatalf("FQDN state not reloaded: %v (want lastResolved %s)", item, lastResolved)
		}
		time.Sleep(3 * time.Second)
		if n := dns.count() - q0; n != 0 {
			t.Fatalf("restart re-queried %d times right away (query storm)", n)
		}
		t.Logf("FQDN state reloaded from the state dir: cdn lastResolved %s unchanged, 0 DNS queries in the first %.1f s after start", lastResolved, time.Since(started).Seconds())
		for _, l := range st.agentLogLines(t, off, `"fqdn state reloaded"`, `"objects domain wired"`, `"reconcile done"`, `"vrx-agent listening"`) {
			t.Log("agent: " + l)
		}
	})

	t.Run("resolver-down-last-good", func(t *testing.T) {
		a.t = t
		off := fileSize(st.agentLog)
		dns.stop()
		var item map[string]any
		if !waitFor(refresh+20*time.Second, func() bool {
			item = fqdnItem(a, "cdn")
			return item["error"] != ""
		}) {
			t.Fatalf("resolver down not noticed: %v", item)
		}
		if strings.Join(strs(item["addresses"]), " ") != "192.0.2.54 2001:db8::53" || item["lastResolved"] != lastResolved {
			t.Fatalf("last-good not kept: %v", item)
		}
		t.Logf("resolver down: %s", js(item))
		var lines []string
		if !waitFor(10*time.Second, func() bool { lines = st.agentLogLines(t, off, "last-good addresses kept"); return len(lines) > 0 }) {
			t.Fatal("no last-good log line")
		}
		for _, l := range lines {
			t.Log("agent: " + l)
		}
		dns.startAgain(t)
	})

	t.Run("delete-referenced-object-refused", func(t *testing.T) {
		a.t = t
		a.must(200, "DELETE", "/api/v1/config/objects/addressGroups/web-servers", nil)
		r := a.must(400, "POST", "/api/v1/config/validate", nil)
		t.Logf("validate after deleting web-servers: %d %s", r.status, r.raw)
		found := false
		for _, e := range r.body["errors"].([]any) {
			if e.(map[string]any)["pointer"] == "/acl/lists/web-in/rules/0/destination/name" {
				found = true
			}
		}
		if !found {
			t.Fatalf("no pointer to the rule: %s", r.raw)
		}
		c := a.must(400, "POST", "/api/v1/config/commit?comment=bad", nil)
		t.Logf("commit: %d type=%v", c.status, c.body["type"])
		a.must(200, "POST", "/api/v1/config/discard", nil)
	})

	t.Run("rollback-restores-object-set", func(t *testing.T) {
		a.t = t
		original := st.retrieveObjects(t)
		usage0 := a.must(200, "GET", "/api/v1/state/objects/usage?name=web-servers", nil).body
		a.must(200, "PUT", "/api/v1/config/acl/lists", map[string]any{"web-in": map[string]any{"rules": []any{
			map[string]any{"sequence": 10, "action": "permit", "destination": map[string]any{"kind": "object", "name": "web1"}},
		}}})
		a.patch("/objects", map[string]any{"addresses": map[string]any{"web2": nil}, "addressGroups": map[string]any{"web-servers": map[string]any{"members": []string{"web1"}}}})
		a.commit("shrink")
		changed := st.retrieveObjects(t)
		if _, ok := changed["addresses"].(map[string]any)["web2"]; ok {
			t.Fatal("web2 still retrieved after the change")
		}
		// only the dmz membership is left: the rule no longer uses the group
		if u := a.must(200, "GET", "/api/v1/state/objects/usage?name=web-servers", nil).body; len(u["usedBy"].([]any)) != 1 ||
			u["usedBy"].([]any)[0].(map[string]any)["pointer"] != "/objects/addressGroups/dmz/members/0" {
			t.Fatalf("usage after the change: %v", u)
		}
		rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=undo", int(rev1)), nil)
		t.Logf("rollback to revision %d: status=%v revision=%v", int(rev1), rb.body["status"], rb.body["revision"].(map[string]any)["id"])
		back := st.retrieveObjects(t)
		if !reflect.DeepEqual(normalize(back), normalize(original)) {
			t.Fatalf("rollback: Retrieve != the original object set:\n%s", js(normalize(back)))
		}
		usage1 := a.must(200, "GET", "/api/v1/state/objects/usage?name=web-servers", nil).body
		if !reflect.DeepEqual(usage1["usedBy"], usage0["usedBy"]) {
			t.Fatalf("where-used not restored: %v vs %v", usage1, usage0)
		}
		t.Logf("after rollback: Retrieve has web2 again (%v), where-used web-servers = %s", back["addresses"].(map[string]any)["web2"], js(usage1["usedBy"]))
	})

	t.Run("cleanup", func(t *testing.T) {
		a.t = t
		a.must(200, "PUT", "/api/v1/config/objects", map[string]any{})
		a.must(200, "PUT", "/api/v1/config/acl", map[string]any{})
		a.commit("cleanup")
		if got := st.retrieveObjects(t); len(normalize(got)) != 0 {
			t.Fatalf("objects left: %s", js(got))
		}
		if items := a.must(200, "GET", "/api/v1/state/objects/fqdn", nil).body["items"].([]any); len(items) != 0 {
			t.Fatalf("fqdn state left: %v", items)
		}
		t.Log("cleanup: objects and acl deleted; Retrieve empty; no FQDN state")
	})
}

func slotNum(s slot) string { return strings.TrimPrefix(s.prefix, "w") }

func fqdnItem(a *api, name string) map[string]any {
	r := a.call("GET", "/api/v1/state/objects/fqdn?name="+name, nil)
	if r.status != 200 {
		return nil
	}
	items, _ := r.body["items"].([]any)
	if len(items) != 1 {
		return nil
	}
	m, _ := items[0].(map[string]any)
	return m
}

func strs(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		out = append(out, fmt.Sprint(x))
	}
	return out
}

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// normalize drops empty arrays and objects (proto3 has no presence for repeated/map fields: the running document carries
// the Zod defaults `tags: []`, Retrieve omits them) and turns JSON numbers into float64 on both sides.
func normalize(v any) map[string]any {
	out, _ := norm(v).(map[string]any)
	if out == nil {
		return map[string]any{}
	}
	return out
}

func norm(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if n := norm(e); n != nil {
				out[k] = n
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		if len(x) == 0 {
			return nil
		}
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = norm(e)
		}
		return out
	default:
		return v
	}
}

// sampleObjects is the object set of the run: every kind, v4 and v6, a range, two FQDN objects (the test's zone), nested
// groups, tags; the zone has no interface (no VPP object in this run).
func sampleObjects(s slot, cdn, web string) map[string]any {
	return map[string]any{
		"tags": map[string]any{"prod": map[string]any{"color": "#1e88e5", "description": "production"}},
		"addresses": map[string]any{
			"web1":  map[string]any{"type": "host", "address": "192.0.2.10", "tags": []string{"prod"}},
			"web2":  map[string]any{"type": "host", "address": "192.0.2.11", "tags": []string{"prod"}},
			"cdn":   map[string]any{"type": "fqdn", "fqdn": cdn, "description": "content delivery"},
			"site":  map[string]any{"type": "fqdn", "fqdn": web},
			"pool":  map[string]any{"type": "range", "start": "10." + slotNum(s) + ".2.1", "end": "10." + slotNum(s) + ".2.20"},
			"lan":   map[string]any{"type": "network", "prefix": "10." + slotNum(s) + ".1.0/24"},
			"web6":  map[string]any{"type": "host", "address": "2001:db8::10"},
			"guest": map[string]any{"type": "network", "prefix": "2001:db8:" + slotNum(s) + "::/64"},
		},
		"addressGroups": map[string]any{
			"web-servers": map[string]any{"members": []string{"web1", "web2", "web6"}, "tags": []string{"prod"}},
			"dmz":         map[string]any{"members": []string{"web-servers", "cdn", "pool"}},
		},
		"services": map[string]any{
			"https": map[string]any{"protocol": "tcp", "destinationPorts": []string{"443"}},
			"dns":   map[string]any{"protocol": "tcp-udp", "destinationPorts": []string{"53"}},
			"ping":  map[string]any{"protocol": "icmp", "type": 8},
		},
		"serviceGroups": map[string]any{"web": map[string]any{"members": []string{"https"}}},
		"schedules": map[string]any{
			"office-hours": map[string]any{"type": "recurring", "days": []string{"mon", "tue", "wed", "thu", "fri"}, "start": "08:00", "end": "18:00"},
			"maintenance":  map[string]any{"type": "once", "start": "2026-10-01T22:00:00+03:30", "end": "2026-10-02T02:00:00+03:30"},
		},
		"zones": map[string]any{"lan": map[string]any{"description": "LAN side (no interface: this run creates no VPP object)"}},
	}
}

// sampleACL is an ACL list that uses the objects (not applied by this agent build — F-acl — but part of the running
// document, so where-used and the reference checks see it).
func sampleACL() map[string]any {
	return map[string]any{
		"lists": map[string]any{"web-in": map[string]any{"rules": []any{
			map[string]any{"sequence": 10, "action": "permit", "destination": map[string]any{"kind": "object", "name": "web-servers"},
				"service": map[string]any{"kind": "object", "name": "web"}, "schedule": "office-hours"},
		}}},
	}
}
