// Package bridgel2 is F-bridge-l2's host integration check: the L2 model end to end through the real API and agent
// against the REAL host VPP 26.06, on the slot's rig interfaces (af_packet, path recorded: af_packet rig) and loopbacks.
// No packet is sent (the task lists no packet-level test): the veth peers stay down the whole run.
//
//	TestBridgeL2OnHost
//	  validation      a bridge member that is also an L2 cross-connect rx → 400 problem+json, pointer to the second
//	                  membership (/routing/l2/xconnects/<if>)
//	  apply           BD w7-lan (7001, mac-age 5, static MAC) with loop7xx as BVI, host-w7l0 (+MAC filter) and
//	                  host-w7l0.100 (shg 1, pop-1) as members; L2 cross-connect host-w7w0 ⇄ host-w7w0.200 (translate-1-1
//	                  on the sub-interface); L3 cross-connect on a loopback; a time-range MAC filter device — commit →
//	                  running vs Retrieve drift is empty, vppctl show bridge-domain/mode/l3xc/mactime reflect it
//	  restart-safety  stop the agent → delete the L2 objects via binapi (dependents first) → start the agent → all back
//	                  within 30 s with no config API call (agent log timestamps)
//	  rollback        rollback to the L3-only revision → the members are back in L3, no bridge domain, cross-connect,
//	                  l3xc or MAC filter of this slot is left (binapi dumps and Retrieve)
//	  cleanup         interfaces deleted through the API → nothing with the prefix remains
//
// Runs only with VRX_INTEGRATION=1, as root, with a slot prefix (w<N>), under flock -s on the lab lock; NRestarts is
// checked before and after. Build and run: test/topology/bridge-l2/run.sh.
package bridgel2

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type names struct {
	prefix         string
	lanDev, wanDev string // rig veths (host side) → VPP host-<dev>
	lanIf, wanIf   string
	lanSub, wanSub string
	bvi, l3xcIf    string
	bd             string
	bdID           int
	dev            string
	slot           int
}

func newNames(s slot) names {
	p := s.prefix
	return names{
		prefix: p, slot: s.num,
		lanDev: p + "l0", wanDev: p + "w0",
		lanIf: "host-" + p + "l0", wanIf: "host-" + p + "w0",
		lanSub: "host-" + p + "l0.100", wanSub: "host-" + p + "w0.200",
		bvi: "loop" + strconv.Itoa(s.num*100+20), l3xcIf: "loop" + strconv.Itoa(s.num*100+21),
		bd: p + "-lan", bdID: s.num*1000 + 1, dev: p + "-kids", // bridge-domain ids from the slot's table range
	}
}

// peersDown keeps both rig veth pairs down: no frame can enter VPP (V19 rule) and the af_packet deletes are safe (D-101).
func (n names) peersDown(t *testing.T) {
	t.Helper()
	for _, dev := range []string{n.lanDev, n.wanDev} {
		mustRun(t, "ip", "link", "set", dev, "down")
	}
	mustRun(t, "ip", "-n", "ns-"+n.prefix+"-lan", "link", "set", n.prefix+"l1", "down")
	mustRun(t, "ip", "-n", "ns-"+n.prefix+"-wan", "link", "set", n.prefix+"w1", "down")
}

func (n names) l3Doc() map[string]any {
	sub := func(vlan int) map[string]any { return map[string]any{"vlanId": vlan, "enabled": true} }
	return map[string]any{
		n.bvi:    map[string]any{"enabled": true, "ipv4": []string{fmt.Sprintf("10.%d.20.1/24", n.slot)}},
		n.l3xcIf: map[string]any{"enabled": true, "ipv4": []string{fmt.Sprintf("10.%d.21.1/24", n.slot)}},
		n.lanIf:  map[string]any{"enabled": true, "subinterfaces": map[string]any{"100": sub(100)}},
		n.wanIf:  map[string]any{"enabled": true, "subinterfaces": map[string]any{"200": sub(200)}},
	}
}

func (n names) l2Patch() (ifs map[string]any, routing map[string]any) {
	ifs = map[string]any{
		n.bvi:   map[string]any{"l2": map[string]any{"bridgeDomain": n.bd, "bvi": true}},
		n.lanIf: map[string]any{"l2": map[string]any{"bridgeDomain": n.bd, "macFilter": true}, "subinterfaces": map[string]any{"100": map[string]any{"l2": map[string]any{"bridgeDomain": n.bd, "shg": 1, "tagRewrite": map[string]any{"op": "pop-1"}}}}},
		n.wanIf: map[string]any{"subinterfaces": map[string]any{"200": map[string]any{"l2": map[string]any{"tagRewrite": map[string]any{"op": "translate-1-1", "tag1": 300}}}}},
	}
	routing = map[string]any{"l2": map[string]any{
		"bridgeDomains": map[string]any{n.bd: map[string]any{"id": n.bdID, "macAgeMin": 5, "staticMacs": []any{map[string]any{"mac": "02:07:00:00:70:01", "interface": n.lanIf}}}},
		"xconnects":     map[string]any{n.wanIf: map[string]any{"tx": n.wanSub}, n.wanSub: map[string]any{"tx": n.wanIf}},
		"l3xc":          map[string]any{n.l3xcIf: map[string]any{"ipv4Paths": []any{map[string]any{"nextHop": fmt.Sprintf("10.%d.21.254", n.slot), "interface": n.l3xcIf}}}},
		"macFilters": map[string]any{n.dev: map[string]any{"mac": "02:07:00:00:99:01", "action": "allow",
			"ranges": []any{map[string]any{"days": []string{"mon", "tue", "wed", "thu", "fri"}, "start": "16:00", "end": "20:00"}}}},
	}}
	return ifs, routing
}

func TestBridgeL2OnHost(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("F-bridge-l2 topology test: set VRX_INTEGRATION=1 (host VPP, rig, PostgreSQL) — run.sh does")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (netns, veth, VPP API socket)")
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
	n := newNames(s)
	conn := connectVPP(t)

	// rig: veth/netns only; its VPP side goes (veths down, D-101) so that the AGENT creates the host-interfaces
	t.Log(mustRun(t, s.lab, "rig", "up", s.prefix))
	t.Cleanup(func() {
		out, err := run(t, s.lab, "rig", "down", s.prefix)
		t.Logf("rig down: %v\n%s", err, out)
	})
	n.peersDown(t)
	for _, l := range deleteBehindBack(t, conn, n.lanDev, n.wanDev) {
		t.Log("rig VPP side handed to the agent: " + l)
	}

	st := newStack(t, s)
	a := st.api

	// ---- revision 1: the interfaces in L3 (the rollback target)
	a.patch("/interfaces", n.l3Doc())
	c1 := a.commit("bl2-rev1-l3")
	rev1 := int(c1["revision"].(map[string]any)["id"].(float64))
	t.Logf("commit rev1 (L3 only) → %v revision %d", c1["status"], rev1)

	ok := t.Run("validation", func(t *testing.T) {
		a.t = t
		a.patch("/interfaces", map[string]any{n.lanIf: map[string]any{"l2": map[string]any{"bridgeDomain": n.bd}}})
		a.patch("/routing", map[string]any{"l2": map[string]any{"bridgeDomains": map[string]any{n.bd: map[string]any{"id": n.bdID}}, "xconnects": map[string]any{n.lanIf: map[string]any{"tx": n.wanIf}}}})
		r := a.call("POST", "/api/v1/config/commit?comment=bl2-second-membership", nil)
		t.Logf("commit of %s as bridge member AND cross-connect rx → %d content-type problem+json; body %s", n.lanIf, r.status, r.raw)
		want := `"pointer":"/routing/l2/xconnects/` + n.lanIf + `"`
		if r.status != 400 || r.body["tier"] != "semantic" || !strings.Contains(js(r.body["errors"]), want) {
			t.Fatalf("want a 400 semantic problem with %s", want)
		}
		a.must(200, "POST", "/api/v1/config/discard", nil)
	})
	if !ok {
		return
	}

	var bviIdx uint32
	ok = t.Run("apply", func(t *testing.T) {
		a.t = t
		ifs, routing := n.l2Patch()
		a.patch("/interfaces", ifs)
		a.patch("/routing", routing)
		t.Logf("candidate diff: %s", trunc(a.must(200, "GET", "/api/v1/config/diff", nil).raw, 4000))
		c := a.commit("bl2-rev2-l2")
		t.Logf("commit rev2 (L2) → %v revision %v, %d results", c["status"], c["revision"].(map[string]any)["id"], len(c["results"].([]any)))
		assertApplied(t, a, conn, n, s.socket)
		bviIdx = dumpIfs(t, conn)[n.bvi].idx
		t.Logf("BVI %s sw_if_index %d", n.bvi, bviIdx)
	})
	if !ok {
		return
	}

	ok = t.Run("restart-safety", func(t *testing.T) {
		a.t = t
		st.agent.stop(t)
		for _, l := range l2Loss(t, conn, n.prefix, []string{n.lanIf}, []string{n.lanSub, n.wanSub}) {
			t.Log("simulated loss: " + l)
		}
		if len(ourBDs(t, conn, n.prefix)) != 0 || len(ourDevices(t, conn, n.prefix)) != 0 {
			t.Fatal("L2 objects still in VPP after the simulated loss")
		}
		t.Log("vppctl show bridge-domain (after the loss):\n" + vppctl(t, "show", "bridge-domain"))
		t0 := time.Now()
		logFrom := fileSize(st.agentLog)
		st.startAgent(t)
		back := waitFor(30*time.Second, func() bool {
			bds := ourBDs(t, conn, n.prefix)
			b, ok := bds[uint32(n.bdID)] //nolint:gosec // slot id
			ifs := dumpIfs(t, conn)
			return ok && len(b.members) == 3 && len(ourDevices(t, conn, n.prefix)) == 1 && mactimeOn(t, conn, ifs[n.lanIf].idx) &&
				len(xconnectsOf(t, conn, ifs, n.wanIf, n.wanSub)) == 2 && hasL3xc(t, conn, ifs[n.l3xcIf].idx)
		})
		tBack := time.Since(t0)
		lines, raw := readAgentLog(t, st.agentLog, logFrom)
		var tStart, tResync time.Time
		for i, l := range lines {
			switch {
			case l.Msg == "vrx-agent starting" && tStart.IsZero():
				tStart = l.Time
				t.Log("agent log: " + raw[i])
			case l.Msg == "resync finished" && tResync.IsZero():
				tResync = l.Time
				t.Log("agent log: " + trunc(raw[i], 600))
			case strings.Contains(l.Msg, "reconcile") || strings.Contains(l.Msg, "VPP boot identity"):
				t.Log("agent log: " + trunc(raw[i], 600))
			}
		}
		t.Logf("agent started at +0s; bridge domain, 3 members, cross-connects, l3xc and MAC filter back at +%.2fs (no config API call)", tBack.Seconds())
		if !back {
			t.Fatalf("the L2 objects did not come back within 30 s")
		}
		if !tStart.IsZero() && !tResync.IsZero() {
			t.Logf("reconcile after simulated loss: %s → %s = %.3fs (agent log timestamps)", tStart.Format(time.RFC3339Nano), tResync.Format(time.RFC3339Nano), tResync.Sub(tStart).Seconds())
		}
		assertApplied(t, a, conn, n, s.socket)
	})
	if !ok {
		return
	}

	ok = t.Run("rollback", func(t *testing.T) {
		a.t = t
		rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=bl2-rollback", rev1), nil)
		t.Logf("POST /config/rollback/%d → status %v", rev1, rb.body["status"])
		if rb.body["status"] != "applied" {
			t.Fatalf("rollback: %s", rb.raw)
		}
		ifs := dumpIfs(t, conn)
		if bds := ourBDs(t, conn, n.prefix); len(bds) != 0 {
			t.Errorf("bridge domains of %s left: %v", n.prefix, bds)
		}
		if x := xconnectsOf(t, conn, ifs, n.wanIf, n.wanSub, n.lanIf, n.lanSub); len(x) != 0 {
			t.Errorf("cross-connects left: %v", x)
		}
		if hasL3xc(t, conn, ifs[n.l3xcIf].idx) {
			t.Errorf("l3xc on %s left", n.l3xcIf)
		}
		if d := ourDevices(t, conn, n.prefix); len(d) != 0 {
			t.Errorf("mactime devices left: %v", d)
		}
		if mactimeOn(t, conn, ifs[n.lanIf].idx) {
			t.Errorf("mactime still enabled on %s", n.lanIf)
		}
		for _, name := range []string{n.lanSub, n.wanSub} {
			if ifs[name].vtr != 0 {
				t.Errorf("tag rewrite left on %s (vtr_op %d)", name, ifs[name].vtr)
			}
		}
		t.Log("vppctl show mode (after rollback):\n" + vppctl(t, "show", "mode", n.lanIf, n.lanSub, n.wanIf, n.wanSub, n.bvi))
		t.Log("vppctl show bridge-domain (after rollback):\n" + vppctl(t, "show", "bridge-domain"))
		sl2 := a.must(200, "GET", "/api/v1/state/l2/bridge-domains", nil)
		t.Logf("GET /state/l2/bridge-domains (after rollback) → %s", sl2.raw)
		if items, _ := sl2.body["items"].([]any); len(items) != 0 {
			t.Errorf("state still lists bridge domains: %v", items)
		}
		dr := a.must(200, "GET", "/api/v1/state/drift", nil)
		t.Logf("GET /state/drift (after rollback) → %s", trunc(dr.raw, 3000))
		if strings.Contains(dr.raw, "/l2") {
			t.Errorf("drift still mentions L2 after the rollback")
		}
		compareL2(t, a, s.socket)
		if l2, ports := retrieveL2(t, s.socket); l2 != nil || len(ports) != 0 {
			t.Errorf("Retrieve after rollback still reports routing.l2=%s leaves=%s", js(l2), js(ports))
		}
	})
	if !ok {
		return
	}

	t.Run("cleanup-through-api", func(t *testing.T) {
		a.t = t
		for name := range n.l3Doc() {
			a.must(200, "DELETE", "/api/v1/config/interfaces/"+name, nil)
		}
		c := a.commit("bl2-cleanup")
		t.Logf("commit (interfaces deleted) → %v", c["status"])
		for name := range dumpIfs(t, conn) {
			if strings.HasPrefix(name, "host-"+n.prefix) || name == n.bvi || name == n.l3xcIf {
				t.Errorf("%s still in VPP after the delete commit", name)
			}
		}
	})
}

// assertApplied checks the committed L2 model on VPP (binapi dumps + vppctl) and through the API (Retrieve drift and
// the live state route).
func assertApplied(t *testing.T, a *api, conn vppConn, n names, socket string) {
	t.Helper()
	ifs := dumpIfs(t, conn)
	bds := ourBDs(t, conn, n.prefix)
	b, ok := bds[uint32(n.bdID)] //nolint:gosec // slot id
	if !ok {
		t.Fatalf("bridge domain %d not in VPP (ours: %v)", n.bdID, bds)
	}
	if b.tag != n.prefix+":"+strconv.Itoa(n.bdID)+"/"+n.bd || b.macAge != 5 || b.bvi != ifs[n.bvi].idx {
		t.Errorf("bridge domain %d: tag %q mac-age %d bvi %d (want %s idx %d)", n.bdID, b.tag, b.macAge, b.bvi, n.bvi, ifs[n.bvi].idx)
	}
	for name, shg := range map[string]uint8{n.bvi: 0, n.lanIf: 0, n.lanSub: 1} {
		if got, ok := b.members[ifs[name].idx]; !ok || got != shg {
			t.Errorf("member %s: present=%v shg=%d (want %d)", name, ok, got, shg)
		}
	}
	if ifs[n.lanSub].vtr != 3 || ifs[n.wanSub].vtr != 5 {
		t.Errorf("tag rewrite: %s vtr_op %d (want 3 pop-1), %s vtr_op %d (want 5 translate-1-1)", n.lanSub, ifs[n.lanSub].vtr, n.wanSub, ifs[n.wanSub].vtr)
	}
	if x := xconnectsOf(t, conn, ifs, n.wanIf, n.wanSub); x[n.wanIf] != n.wanSub || x[n.wanSub] != n.wanIf {
		t.Errorf("cross-connects %v", x)
	}
	if !hasL3xc(t, conn, ifs[n.l3xcIf].idx) {
		t.Errorf("no l3xc on %s", n.l3xcIf)
	}
	if d := ourDevices(t, conn, n.prefix); d[n.prefix+":"+n.dev] == nil || len(d[n.prefix+":"+n.dev].Ranges) != 5 {
		t.Errorf("mactime device %s:%s: %v", n.prefix, n.dev, d)
	}
	if !mactimeOn(t, conn, ifs[n.lanIf].idx) {
		t.Errorf("mactime not enabled on %s", n.lanIf)
	}
	bdOut := vppctl(t, "show", "bridge-domain", strconv.Itoa(n.bdID), "detail")
	t.Logf("vppctl show bridge-domain %d detail:\n%s", n.bdID, bdOut)
	for _, want := range []string{n.lanIf, n.lanSub, n.bvi, "pop-1"} {
		if !strings.Contains(bdOut, want) {
			t.Errorf("show bridge-domain %d detail lacks %q", n.bdID, want)
		}
	}
	t.Logf("vppctl show l2fib bd_id %d:\n%s", n.bdID, vppctl(t, "show", "l2fib", "bd_id", strconv.Itoa(n.bdID)))
	t.Log("vppctl show mode:\n" + vppctl(t, "show", "mode", n.lanIf, n.lanSub, n.wanIf, n.wanSub, n.bvi))
	t.Log("vppctl show l2patch (l2patch is VPP's other cross-connect feature; the L2 xconnects show in `show mode`):\n" + vppctl(t, "show", "l2patch"))
	l3 := vppctl(t, "show", "l3xc")
	t.Log("vppctl show l3xc:\n" + l3)
	if !strings.Contains(l3, n.l3xcIf) {
		t.Errorf("show l3xc lacks %s", n.l3xcIf)
	}
	mt := vppctl(t, "show", "mactime")
	t.Log("vppctl show mactime:\n" + mt)
	if !strings.Contains(mt, n.prefix+":"+n.dev) {
		t.Errorf("show mactime lacks %s:%s", n.prefix, n.dev)
	}
	t.Log("vppctl show interface features " + n.lanIf + " (device-input):\n" + section(vppctl(t, "show", "interface", "features", n.lanIf), "device-input:"))

	sl2 := a.must(200, "GET", "/api/v1/state/l2/bridge-domains", nil)
	t.Logf("GET /state/l2/bridge-domains → %s", sl2.raw)
	items, _ := sl2.body["items"].([]any)
	if len(items) != 1 || !strings.Contains(sl2.raw, `"bvi":"`+n.bvi+`"`) {
		t.Errorf("state/l2/bridge-domains: %s", sl2.raw)
	}
	macs := a.must(200, "GET", fmt.Sprintf("/api/v1/state/l2/bridge-domains/%d/macs?page=1&pageSize=10", n.bdID), nil)
	t.Logf("GET /state/l2/bridge-domains/%d/macs → %s", n.bdID, macs.raw)
	dr := a.must(200, "GET", "/api/v1/state/drift", nil)
	t.Logf("GET /state/drift (running vs Retrieve) → %s", trunc(dr.raw, 3000))
	compareL2(t, a, socket)
	var drift struct {
		Changes []struct {
			Pointer string `json:"pointer"`
		} `json:"changes"`
	}
	_ = json.Unmarshal([]byte(dr.raw), &drift)
	for _, c := range drift.Changes {
		if strings.Contains(c.Pointer, "/l2") {
			t.Errorf("Retrieve differs from running at %s", c.Pointer)
		}
	}
}

func section(out, head string) string {
	i := strings.Index(out, head)
	if i < 0 {
		return out
	}
	rest := out[i:]
	if j := strings.Index(rest[len(head):], "\n\n"); j >= 0 {
		return rest[:len(head)+j]
	}
	return rest
}

// xconnectsOf returns rx → tx names of the cross-connects whose rx is one of rxs.
func xconnectsOf(t *testing.T, conn vppConn, ifs map[string]vppIf, rxs ...string) map[string]string {
	t.Helper()
	byIdx := map[uint32]string{}
	for n, i := range ifs {
		byIdx[i.idx] = n
	}
	want := map[string]bool{}
	for _, r := range rxs {
		want[r] = true
	}
	out := map[string]string{}
	for rx, tx := range xconnects(t, conn) {
		if want[byIdx[rx]] {
			out[byIdx[rx]] = byIdx[tx]
		}
	}
	return out
}

func hasL3xc(t *testing.T, conn vppConn, idx uint32) bool {
	t.Helper()
	_, ok := l3xcs(t, conn)[idx]
	return ok
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
