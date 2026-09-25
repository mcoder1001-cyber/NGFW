// Package lbgs is F-loopback-bvi-gso-lldp-span's host integration check: loopbacks (one as a bridge BVI), GSO, port
// mirroring (SPAN, and ERSPAN to a slot-prefixed GRE erspan tunnel), LLDP and nsim end to end through the real API and
// agent against the REAL host VPP 26.06, on this slot's loopbacks only. No packet is sent (the task lists no
// packet-level test).
//
//	TestLoopbackBviGsoLldpSpanOnHost
//	  validation      a mirror session whose destination is its source → 400 problem+json, pointer
//	                  /interfaces/<if>/mirror/0/destination
//	  loopbacks       rev A: loop<slot>75 (BVI of bridge domain <slot>750) and loop<slot>76, the ERSPAN fixture named
//	  features        rev B: GSO on the BVI, mirror BVI → loop76 (device, both) and BVI → gre<slot>78 (device, rx =
//	                  ERSPAN), LLDP on the BVI (only when its sw_if_index equals its hw_if_index, V20), nsim (a slot agent
//	                  reports it unsupported, D-071) → vppctl show interface span / show lldp / show interface /
//	                  show bridge-domain detail / show interface features; Retrieve == running for gso and mirror;
//	                  /state/lldp/neighbors, /state/interfaces
//	  restart-safety  stop the agent → delete mirror, GSO, LLDP, the BVI membership and bridge domain, then the loopbacks
//	                  via binapi (D-095c) → start the agent → everything back within 30 s with no config API call
//	  rollback        rollback to rev A → no mirror, GSO or LLDP left on the loopbacks (binapi + vppctl); rollback to the
//	                  first revision → the loopbacks are gone
//	  cleanup         the fixture is deleted; nothing with the slot prefix remains (dump)
//
// Runs only with VRX_INTEGRATION=1, as root, with a slot prefix (w<N>), under flock -s on the lab lock; NRestarts is
// checked before and after. Build and run: test/topology/loopback-bvi-gso-lldp-span/run.sh.
package lbgs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	spanapi "ngfw/agent/binapi/span"
	vrxv1 "ngfw/agent/gen/vrx/v1"
)

type names struct {
	prefix   string
	slot     int
	bvi, mon string
	gre      string
	greInst  uint32
	bd       string
	bdID     uint32
	lldp     string // an index-aligned loopback for LLDP (V20), "" when none was found
	lldpIdx  uint32
}

func newNames(s slot) names {
	return names{
		prefix: s.prefix, slot: s.num,
		bvi: "loop" + strconv.Itoa(s.num*100+75), mon: "loop" + strconv.Itoa(s.num*100+76),
		greInst: uint32(s.num*100 + 78), gre: "gre" + strconv.Itoa(s.num*100+78), //nolint:gosec // slot 1–12
		bd: s.prefix + "-lan", bdID: uint32(s.num*1000 + 750), //nolint:gosec // the slot's id range
	}
}

func (n names) greSrc() string { return fmt.Sprintf("10.%d.78.1", n.slot) }
func (n names) greDst() string { return fmt.Sprintf("10.%d.78.2", n.slot) }

// loopbacksDoc is revision A: the loopbacks, the bridge domain with the BVI, the fixture tunnel named (not created).
func (n names) loopbacksDoc() (ifs map[string]any, routing map[string]any) {
	ifs = map[string]any{
		n.bvi: map[string]any{"enabled": true, "ipv4": []string{fmt.Sprintf("10.%d.75.1/24", n.slot)}, "l2": map[string]any{"bridgeDomain": n.bd, "bvi": true}},
		n.mon: map[string]any{"enabled": true},
		n.gre: map[string]any{},
	}
	if n.lldp != "" {
		ifs[n.lldp] = map[string]any{"enabled": true}
	}
	routing = map[string]any{"l2": map[string]any{"bridgeDomains": map[string]any{n.bd: map[string]any{"id": n.bdID}}}}
	return ifs, routing
}

func (n names) featuresPatch() map[string]any {
	return map[string]any{
		n.bvi: map[string]any{"gso": true, "mirror": []any{
			map[string]any{"destination": n.mon, "direction": "both", "level": "device"},
			map[string]any{"destination": n.gre, "direction": "rx", "level": "device"},
		}},
	}
}

func TestLoopbackBviGsoLldpSpanOnHost(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("F-loopback-bvi-gso-lldp-span topology test: set VRX_INTEGRATION=1 (host VPP, PostgreSQL) — run.sh does")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (VPP API socket)")
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
	t.Cleanup(func() { leftovers(t, conn, &n) }) // runs last: after the fixture and the stack are gone
	greName, greIdx := greFixture(t, conn, n.prefix, n.greInst, n.greSrc(), n.greDst(), 7)
	t.Logf("ERSPAN fixture %s (sw_if_index %d, tag %s:%s): gre_tunnel_add_del_v2 type=erspan p2p %s → %s session 7", greName, greIdx, n.prefix, greName, n.greSrc(), n.greDst())
	lldpName, lldpIdx, lldpOK := alignedLoopback(t, conn, n.slot)
	if lldpOK {
		n.lldp, n.lldpIdx = lldpName, lldpIdx
		t.Logf("LLDP interface: %s (sw_if_index %d == hw_if_index): created untagged, tagged %s:%s right before rev A names it (the agent adopts it by its tag)", n.lldp, n.lldpIdx, n.prefix, n.lldp)
	} else {
		t.Log("no index-aligned loopback within the slot's probes: LLDP is skipped (VPP 26.06 V20 would enable it on another interface)")
	}

	st := newStack(t, s)
	a := st.api
	// revision 0: only the fixture tunnel named (the rollback target of the loopbacks)
	a.patch("/interfaces", map[string]any{n.gre: map[string]any{}})
	c0 := a.commit("lbgs-rev0-fixture-named")
	first := int(c0["revision"].(map[string]any)["id"].(float64))
	t.Logf("commit rev 0 (the ERSPAN fixture %s named, nothing else) → %v revision %d", n.gre, c0["status"], first)

	ok := t.Run("validation", func(t *testing.T) {
		a.t = t
		a.patch("/interfaces", map[string]any{n.bvi: map[string]any{"enabled": true, "mirror": []any{map[string]any{"destination": n.bvi}}}})
		r := a.call("POST", "/api/v1/config/commit?comment=lbgs-mirror-to-itself", nil)
		t.Logf("commit of a mirror %s → %s → %d; body %s", n.bvi, n.bvi, r.status, r.raw)
		want := `"pointer":"/interfaces/` + n.bvi + `/mirror/0/destination"`
		if r.status != 400 || r.body["tier"] != "semantic" || !strings.Contains(js(r.body["errors"]), want) {
			t.Fatalf("want a 400 semantic problem with %s", want)
		}
		a.must(200, "POST", "/api/v1/config/discard", nil)
	})
	if !ok {
		return
	}

	var revA int
	ok = t.Run("loopbacks", func(t *testing.T) {
		a.t = t
		if lldpOK {
			tagOwner(t, conn, n.prefix, n.lldp, n.lldpIdx)
		}
		ifs, routing := n.loopbacksDoc()
		a.patch("/interfaces", ifs)
		a.patch("/routing", routing)
		c := a.commit("lbgs-revA-loopbacks")
		revA = int(c["revision"].(map[string]any)["id"].(float64))
		t.Logf("commit rev A (loopbacks + BVI) → %v revision %d", c["status"], revA)
		v := dumpIfs(t, conn)
		b := v[n.bvi]
		if b.tag != n.prefix+":"+n.bvi {
			t.Fatalf("%s not created by the agent (tag %q)", n.bvi, b.tag)
		}
		if lldpOK && v[n.lldp].idx != n.lldpIdx {
			t.Fatalf("%s was re-created (sw_if_index %d, not the aligned %d)", n.lldp, v[n.lldp].idx, n.lldpIdx)
		}
		t.Log("vppctl show interface:\n" + vppctl(t, "show", "interface", n.bvi, n.mon))
	})
	if !ok {
		return
	}

	ok = t.Run("features", func(t *testing.T) {
		a.t = t
		a.patch("/interfaces", n.featuresPatch())
		svc := map[string]any{"nsim": map[string]any{"delayMs": 20, "bandwidthMbps": 100, "outputInterfaces": []string{n.mon}}}
		if lldpOK {
			svc["lldp"] = map[string]any{"enabled": true, "interfaces": []any{map[string]any{"interface": n.lldp, "portDescription": n.prefix + " lab uplink"}}}
		}
		a.patch("/services", svc)
		t.Logf("candidate diff: %s", trunc(a.must(200, "GET", "/api/v1/config/diff", nil).raw, 3000))
		c := a.commit("lbgs-revB-features")
		t.Logf("commit rev B → %v revision %v, %d results", c["status"], c["revision"].(map[string]any)["id"], len(c["results"].([]any)))
		for _, r := range c["results"].([]any) {
			m := r.(map[string]any)
			if k, _ := m["key"].(string); strings.HasPrefix(k, "gso.") || strings.HasPrefix(k, "span.") || strings.HasPrefix(k, "lldp.") {
				t.Logf("  result %v %v %v", m["op"], k, m["code"])
			}
		}
		assertFeatures(t, a, conn, n, s.socket, lldpOK)
	})
	if !ok {
		return
	}

	ok = t.Run("restart-safety", func(t *testing.T) {
		a.t = t
		st.agent.stop(t)
		for _, l := range loss(t, conn, n.bvi, n.mon, n.bdID) {
			t.Log("simulated loss: " + l)
		}
		if lldpOK {
			lldpOff(t, conn, n.lldpIdx)
			t.Logf("simulated loss: sw_interface_set_lldp %s (%d) enable=false → ok (the aligned loopback itself stays: V20)", n.lldp, n.lldpIdx)
		}
		ifs := dumpIfs(t, conn)
		if _, ok := ifs[n.bvi]; ok {
			t.Fatal("loopback still in VPP after the simulated loss")
		}
		t.Log("vppctl show interface span (after the loss):\n" + vppctl(t, "show", "interface", "span"))
		t0 := time.Now()
		logFrom := fileSize(st.agentLog)
		st.startAgent(t)
		back := waitFor(30*time.Second, func() bool {
			v := dumpIfs(t, conn)
			b, okB := v[n.bvi]
			_, okM := v[n.mon]
			if !okB || !okM {
				return false
			}
			bd, okBD := bridgeDomain(t, conn, n.bdID)
			sp := spansFrom(t, conn, v, b.idx)
			return okBD && bd.bvi == b.idx && len(sp) == 2 && gsoOn(t, conn, b.idx) && (!lldpOK || lldpOn(t, conn)[n.lldpIdx])
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
				t.Log("agent log: " + trunc(raw[i], 800))
			case strings.Contains(l.Msg, "reconcile") || strings.Contains(l.Msg, "globals owner"):
				t.Log("agent log: " + trunc(raw[i], 800))
			}
		}
		t.Logf("agent started at +0s; loopbacks, BVI, mirror sessions, GSO and LLDP back at +%.2fs (no config API call)", tBack.Seconds())
		if !back {
			t.Fatalf("the objects did not come back within 30 s")
		}
		if !tStart.IsZero() && !tResync.IsZero() {
			t.Logf("reconcile after simulated loss: %s → %s = %.3fs (agent log timestamps)", tStart.Format(time.RFC3339Nano), tResync.Format(time.RFC3339Nano), tResync.Sub(tStart).Seconds())
		}
		assertFeatures(t, a, conn, n, s.socket, lldpOK)
	})
	if !ok {
		return
	}

	ok = t.Run("rollback", func(t *testing.T) {
		a.t = t
		rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=lbgs-rollback-features", revA), nil)
		t.Logf("POST /config/rollback/%d (rev A: loopbacks only) → status %v", revA, rb.body["status"])
		if rb.body["status"] != "applied" {
			t.Fatalf("rollback: %s", rb.raw)
		}
		v := dumpIfs(t, conn)
		b, ok := v[n.bvi]
		if !ok {
			t.Fatalf("%s must stay after the rollback to rev A", n.bvi)
		}
		if sp := spansFrom(t, conn, v, b.idx); len(sp) != 0 {
			t.Errorf("mirror sessions of %s left: %v", n.bvi, sp)
		}
		if gsoOn(t, conn, b.idx) {
			t.Errorf("GSO still on %s (one delete must clear the single enable)", n.bvi)
		}
		if lldpOK && lldpOn(t, conn)[n.lldpIdx] {
			t.Errorf("LLDP still on %s", n.lldp)
		}
		t.Logf("after the rollback to rev A: sw_interface_span_dump from %s: none; feature_is_enabled(ip4-output, gso-ip4, %d) = false; lldp_dump lists %s: %v", n.bvi, b.idx, n.lldp, lldpOK && lldpOn(t, conn)[n.lldpIdx])
		t.Log("vppctl show interface span (after rollback):\n" + vppctl(t, "show", "interface", "span"))
		t.Log("vppctl show lldp (after rollback):\n" + vppctl(t, "show", "lldp"))
		t.Log("vppctl show interface features " + n.bvi + " (ip4-output, after rollback):\n" + section(vppctl(t, "show", "interface", "features", n.bvi), "ip4-output:"))
		ds := retrieve(t, s.socket)
		itf := ds.GetInterfaces()[n.bvi]
		if itf.GetGso() || len(itf.GetMirror()) > 0 {
			t.Errorf("Retrieve after rollback still reports gso=%v mirror=%v", itf.GetGso(), itf.GetMirror())
		}
		t.Logf("Retrieve after rollback: %s gso=%v mirror=%v", n.bvi, itf.Gso, itf.GetMirror())

		rb = a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=lbgs-rollback-all", first), nil)
		t.Logf("POST /config/rollback/%d (first revision) → status %v", first, rb.body["status"])
		v = dumpIfs(t, conn)
		for _, name := range []string{n.bvi, n.mon, n.lldp} {
			if _, ok := v[name]; ok && name != "" {
				t.Errorf("%s still in VPP after the rollback", name)
			}
		}
		if _, ok := bridgeDomain(t, conn, n.bdID); ok {
			t.Errorf("bridge domain %d left", n.bdID)
		}
		t.Log("vppctl show interface (after the rollback to the first revision):\n" + vppctl(t, "show", "interface"))
	})
	_ = ok
}

// assertFeatures checks the committed features on VPP (binapi dumps + vppctl) and through the API and Retrieve.
func assertFeatures(t *testing.T, a *api, conn vppConn, n names, socket string, lldpOK bool) {
	t.Helper()
	v := dumpIfs(t, conn)
	b, m := v[n.bvi], v[n.mon]
	sp := spansFrom(t, conn, v, b.idx)
	if sp[n.mon+"/device"] != spanapi.SPAN_STATE_API_RX_TX || sp[n.gre+"/device"] != spanapi.SPAN_STATE_API_RX || len(sp) != 2 {
		t.Errorf("mirror sessions of %s: %v", n.bvi, sp)
	}
	if !gsoOn(t, conn, b.idx) {
		t.Errorf("GSO not on %s", n.bvi)
	}
	if lldpOK && !lldpOn(t, conn)[n.lldpIdx] {
		t.Errorf("LLDP not on %s", n.lldp)
	}
	bd, ok := bridgeDomain(t, conn, n.bdID)
	if !ok || bd.bvi != b.idx {
		t.Errorf("bridge domain %d: found %v bvi %d (want %s = %d)", n.bdID, ok, bd.bvi, n.bvi, b.idx)
	}
	_ = m
	spOut := vppctl(t, "show", "interface", "span")
	t.Log("vppctl show interface span:\n" + spOut)
	for _, want := range []string{n.bvi, n.mon, n.gre} {
		if !strings.Contains(spOut, want) {
			t.Errorf("show interface span lacks %s", want)
		}
	}
	lOut := vppctl(t, "show", "lldp")
	t.Log("vppctl show lldp:\n" + lOut)
	if lldpOK && !strings.Contains(lOut, n.lldp) {
		t.Errorf("show lldp lacks %s", n.lldp)
	}
	t.Log("vppctl show interface " + n.bvi + ":\n" + vppctl(t, "show", "interface", n.bvi))
	bdOut := vppctl(t, "show", "bridge-domain", strconv.FormatUint(uint64(n.bdID), 10), "detail")
	t.Logf("vppctl show bridge-domain %d detail:\n%s", n.bdID, bdOut)
	if !strings.Contains(bdOut, n.bvi) || !strings.Contains(bdOut, "BVI") {
		t.Errorf("show bridge-domain %d detail lacks the BVI %s", n.bdID, n.bvi)
	}
	t.Log("vppctl show interface features " + n.bvi + " (ip4-output):\n" + section(vppctl(t, "show", "interface", "features", n.bvi), "ip4-output:"))
	t.Log("vppctl show nsim (VPP-global; a slot agent does not apply services.nsim):\n" + vppctl(t, "show", "nsim"))

	// Retrieve (agent) == running for the retrievable leaves: gso and mirror
	ds := retrieve(t, socket)
	got := ds.GetInterfaces()[n.bvi]
	run := a.must(200, "GET", "/api/v1/config/interfaces/"+n.bvi, nil)
	gotJS, _ := protojson.Marshal(got)
	var gotMap map[string]any
	_ = json.Unmarshal(gotJS, &gotMap)
	if !reflect.DeepEqual(norm(gotMap["gso"]), norm(run.body["gso"])) || !reflect.DeepEqual(norm(gotMap["mirror"]), norm(run.body["mirror"])) {
		t.Errorf("Retrieve != running: gso %v/%v mirror %v/%v", gotMap["gso"], run.body["gso"], gotMap["mirror"], run.body["mirror"])
	} else {
		t.Logf("Retrieve == running for %s: gso=%v mirror=%s", n.bvi, gotMap["gso"], js(gotMap["mirror"]))
	}
	st, _ := a.ifState()
	t.Logf("GET /state/interfaces item %s config.gso=%v config.mirror=%s", n.bvi, st[n.bvi]["config"].(map[string]any)["gso"], js(st[n.bvi]["config"].(map[string]any)["mirror"]))
	nb := a.must(200, "GET", "/api/v1/state/lldp/neighbors", nil)
	t.Logf("GET /state/lldp/neighbors → %s", trunc(nb.raw, 1500))
	if lldpOK && !strings.Contains(nb.raw, `"interface":"`+n.lldp+`"`) {
		t.Errorf("/state/lldp/neighbors lacks %s", n.lldp)
	}
	dr := a.must(200, "GET", "/api/v1/state/drift", nil)
	t.Logf("GET /state/drift → %s", trunc(dr.raw, 3000))
}

// retrieve asks the slot's agent for Retrieve over its unix socket.
func retrieve(t *testing.T, socket string) *vrxv1.DesiredState {
	t.Helper()
	cc, err := grpc.NewClient("unix:"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer cc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := vrxv1.NewDataplaneClient(cc).Retrieve(ctx, &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces", "services"}})
	if err != nil {
		t.Fatalf("agent Retrieve: %v", err)
	}
	return r.GetDesiredState()
}

// leftovers lists (and fails on) anything of this slot the test touched that is still in VPP: the loopbacks, the LLDP
// probes and their holder (tagged or not), the fixture tunnel, the bridge domain.
func leftovers(t *testing.T, conn vppConn, n *names) {
	t.Helper()
	mine := map[string]bool{n.bvi: true, n.mon: true, n.gre: true}
	for i := 80; i <= 89; i++ {
		mine[fmt.Sprintf("loop%d", n.slot*100+i)] = true
	}
	var left []string
	for name, i := range dumpIfs(t, conn) {
		base, _, _ := strings.Cut(name, ".")
		if mine[base] {
			left = append(left, fmt.Sprintf("%s (%d, %q)", name, i.idx, i.tag))
		}
	}
	if _, ok := bridgeDomain(t, conn, n.bdID); ok {
		left = append(left, fmt.Sprintf("bridge domain %d", n.bdID))
	}
	var sp []string
	for _, e := range spans(t, conn) {
		sp = append(sp, fmt.Sprintf("%d→%d", e.from, e.to))
	}
	t.Logf("leftover check (%s %s %s, loop%d80–loop%d89 and sub-interfaces, bridge domain %d): %v; all mirror sessions in VPP: %v", n.bvi, n.mon, n.gre, n.slot, n.slot, n.bdID, left, sp)
	if len(left) > 0 {
		t.Errorf("left in VPP: %v", left)
	}
}

func norm(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
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
