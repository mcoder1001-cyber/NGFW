// Package det44topo is F-det44-map-dslite-cnat-host's topology test: DET44, MAP-E (LW4o6 rules), DS-Lite and CNAT
// end to end against the REAL host VPP through the af_packet veth/netns rig (path: af_packet, D-010) with the real
// ngfw-agent of the slot driven over gRPC (no API stack: the slot has no Valkey database; the API layer is covered by
// the e2e test of the source task). The rig is IPv4-only; this test adds the MAP-E IPv6 side inside fd00:<slot
// hex>::/32 and removes it.
//
//	TestDet44MapDsliteCnatOnHost
//	  config     interfaces (rev 1) → det44 + dslite + map + cnat (rev 2) → Retrieve(nat) == canonical → empty
//	             re-apply → vppctl show det44 / map / cnat / dslite (ours)
//	  restart    agent stopped → every object of the slot deleted via binapi (dependents first) → agent started →
//	             Retrieve == canonical within 30 s, exactly one MAP domain, one translation, two det44 interfaces;
//	             a second restart without loss changes nothing (write-only objects re-applied, not duplicated)
//	  det44      rev 3 (det44 only on the rig): V19 guard → lan → wan TCP: tcpdump in the wan netns shows the
//	             deterministic outside address and a port inside the block the Det44Lookup RPC returns; sessions RPC
//	  map-cnat   rev 4 (map + cnat, det44 interfaces off the rig): MAP-E BR: lan → shared IPv4 address :PSID port →
//	             tcpdump in the wan netns shows v4-in-v6 (BR source → the rule's CE address), the CE (an ip6tnl in the
//	             wan netns) answers and the client reads the greeting; CNAT: many lan flows → VIP:80 → tcpdump in the
//	             wan netns shows both backends; CnatSessions RPC
//	  rollback   rev 5 (nat {}) → Retrieve has no NAT object, VPP holds none of ours, det44 still enabled (V9)
//
// Runs only with NGFW_INTEGRATION=1, as root, with a slot prefix, under flock -s on the lab lock and flock -x on the
// globals lock (D-167: the det44 enable and the AFTR fixture are VPP globals; the window is the test, ≤ 10 min).
// NGFW_FDET44_DET44_HOST=1 opts in to the det44 plugin enable (irreversible until a VPP restart, V9); without it
// the det44 phases run only when the plugin is already enabled. NRestarts is pasted before and after every phase.
// VPP is never restarted; every process is stopped by PID; no packet trace (D-128).
package det44topo

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	vppapi "go.fd.io/govpp/api"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

const det44HostEnv = "NGFW_FDET44_DET44_HOST"

// dslitePoolEnv=1 opts in to DS-Lite pool addresses. Off by default since run 1 (2026-09-28 19:13): VPP 26.06
// `src/plugins/nat/lib/alloc.c` nat_add_del_ip4_pool_addr frees the bitmaps of pool_addr[1] instead of pool_addr[i]
// (`a = pool->pool_addr + 1`), so deleting a multi-address DS-Lite pool leaves dangling per-thread vectors in the
// stale vector slots; the agent's re-add (restart step) revalidates them and the next delete (the cleanup) is an
// invalid free → os_panic() on the shared VPP. Only in a manager window right before a planned VPP restart.
const dslitePoolEnv = "NGFW_FDET44_DSLITE_POOL"

func poolsOn() bool { return os.Getenv(dslitePoolEnv) == "1" }

// wantPools is the number of DS-Lite pool addresses every document carries (4 with the opt-in, else 0).
func wantPools() int {
	if poolsOn() {
		return 4
	}
	return 0
}

// poolsJSON is the pools member of the dslite part ("" without the opt-in).
func (f *fixture) poolsJSON() string {
	if !poolsOn() {
		return ""
	}
	return `, "pools": [{"range": "` + f.s.addr(5, 1) + `-` + f.s.addr(5, 4) + `"}]`
}

type fixture struct {
	s      slot
	r      rig
	conn   vppapi.Connection
	a      *agent
	sc     scope
	work   string
	aftr   string // "" = omitted from the document
	det44  bool   // det44 phases run
	canon  *ngfwv1.NatConfig
	ifDoc  string // the interfaces part of every document
	v6wan  string // VPP wan IPv6 (fd00:<h>:2::1)
	v6peer string // wan netns IPv6 (fd00:<h>:2::2)
	brAddr string // MAP BR address (fd00:<h>::1)
	cePfx  string // CE prefix routed to the wan netns (fd00:<h>:65::/64)
	ceAddr string // the rule's CE address for PSID 1 (fd00:<h>:65::1)
	shared string // the MAP shared IPv4 address (10.<N>.65.1)
	vip    string // CNAT VIP (10.<N>.2.100)
	be2    string // second backend in the wan netns (10.<N>.2.3)
}

// docs -------------------------------------------------------------------------------------------

func (f *fixture) doc(nat string) string {
	return `{"vrfs": {}, "interfaces": ` + f.ifDoc + `, "routing": {"static": [{"prefix": "` + f.cePfx + `", "vrf": "default", "nextHops": [{"address": "` + f.v6peer + `"}]}]}, "nat": ` + nat + `}`
}

func (f *fixture) det44Part() string {
	return `"det44": {"enabled": true, "inside": ["` + f.r.lanIf + `"], "outside": ["` + f.r.wanIf + `"],
	  "mappings": [{"description": "lan /24 → outside /30 (64 hosts per address)", "inside": "` + f.s.addr(1, 0) + `/24", "outside": "` + f.s.addr(3, 0) + `/30"}]}`
}

func (f *fixture) dslitePart() string {
	a := ""
	if f.aftr != "" {
		a = `, "aftr": {"ipv6": "` + f.aftr + `"}`
	}
	return `"dslite": {"enabled": true` + a + f.poolsJSON() + `}`
}

func (f *fixture) mapPart() string {
	return `"map": {"interfaces": [{"interface": "` + f.r.lanIf + `", "mode": "map-e"}, {"interface": "` + f.r.wanIf + `", "mode": "map-e"}],
	  "domains": [{"name": "lw", "description": "LW4o6 BR: one shared IPv4, 16 CEs × 4032 ports", "mode": "lw4o6", "ipv4Prefix": "` + f.shared + `/32", "ipv6Prefix": "` + f.cePfx + `",
	    "ipv6Source": "` + f.brAddr + `/128", "psidOffset": 6, "psidLength": 4,
	    "rules": [{"psid": 1, "ipv6Destination": "` + f.ceAddr + `"}, {"psid": 2, "ipv6Destination": "` + strings.TrimSuffix(f.ceAddr, "1") + `2"}]}]}`
}

func (f *fixture) cnatPart() string {
	return `"cnat": {"translations": [{"name": "web", "description": "VIP :80 → two backends :8080", "protocol": "tcp", "vip": {"ip": "` + f.vip + `", "port": 80},
	  "backends": [{"ip": "` + f.r.wanIP + `", "port": 8080}, {"ip": "` + f.be2 + `", "port": 8080}]}]}`
}

func (f *fixture) canonical(t *testing.T, withDet44, withMapCnat bool) *ngfwv1.NatConfig {
	t.Helper()
	var parts []string
	if withDet44 {
		parts = append(parts, `"det44": {"enabled": true, "inside": ["`+f.r.lanIf+`"], "outside": ["`+f.r.wanIf+`"], "mappings": [{"inside": "`+f.s.addr(1, 0)+`/24", "outside": "`+f.s.addr(3, 0)+`/30"}]}`)
	}
	// Retrieve observes dslite only through the slot's pool addresses (the AFTR is a global, never attributed to an
	// owner): without the pool opt-in (D-211) the canonical form has no dslite member.
	if poolsOn() {
		parts = append(parts, `"dslite": {"enabled": true`+f.poolsJSON()+`}`)
	}
	if withMapCnat {
		parts = append(parts, `"map": {"interfaces": [{"interface": "`+f.r.lanIf+`", "mode": "map-e"}, {"interface": "`+f.r.wanIf+`", "mode": "map-e"}],
		  "domains": [{"name": "lw", "mode": "lw4o6", "ipv4Prefix": "`+f.shared+`/32", "ipv6Prefix": "`+f.cePfx+`", "ipv6Source": "`+f.brAddr+`/128", "eaBitsLength": 0, "psidOffset": 6, "psidLength": 4,
		    "rules": [{"psid": 1, "ipv6Destination": "`+f.ceAddr+`"}, {"psid": 2, "ipv6Destination": "`+strings.TrimSuffix(f.ceAddr, "1")+`2"}]}]}`,
			`"cnat": {"translations": [{"protocol": "tcp", "vip": {"ip": "`+f.vip+`", "port": 80}, "backends": [{"ip": "`+f.r.wanIP+`", "port": 8080}, {"ip": "`+f.be2+`", "port": 8080}], "lbType": "default"}]}`)
	}
	return natJSON(t, "{"+strings.Join(parts, ",\n")+"}")
}

// evidence -----------------------------------------------------------------------------------------

func (f *fixture) evidence(t *testing.T) {
	t.Helper()
	n := "10." + strconv.Itoa(f.s.num) + "."
	h := "fd00:" + f.s.hex6() + ":"
	t.Log("vppctl show det44 interfaces (ours):\n" + grepLines(vppctl(t, "show", "det44", "interfaces"), f.r.lanIf, f.r.wanIf))
	t.Log("vppctl show det44 mappings (ours):\n" + grepLines(vppctl(t, "show", "det44", "mappings"), n))
	t.Log("vppctl show map domain (ours):\n" + grepLines(vppctl(t, "show", "map", "domain"), f.s.prefix+":", n, h, "psid", "PSID"))
	t.Log("vppctl show cnat translation (ours):\n" + grepLines(vppctl(t, "show", "cnat", "translation"), n))
	t.Log("vppctl show dslite aftr-tunnel-endpoint-address: " + strings.TrimSpace(vppctl(t, "show", "dslite", "aftr-tunnel-endpoint-address")))
	t.Log("vppctl show dslite b4-tunnel-endpoint-address: " + strings.TrimSpace(vppctl(t, "show", "dslite", "b4-tunnel-endpoint-address")))
	t.Log("vppctl show dslite pool (ours):\n" + grepLines(vppctl(t, "show", "dslite", "pool"), n))
}

func (f *fixture) retrieveUntil(t *testing.T, want *ngfwv1.NatConfig, timeout time.Duration) (time.Duration, *ngfwv1.NatConfig) {
	t.Helper()
	start := time.Now()
	var got *ngfwv1.NatConfig
	waitFor(timeout, func() bool {
		g, err := f.a.retrieveNat(t)
		if err != nil {
			return false
		}
		got = g
		return proto.Equal(g, want)
	})
	return time.Since(start), got
}

func (f *fixture) mustRetrieve(t *testing.T, want *ngfwv1.NatConfig, what string) {
	t.Helper()
	got, err := f.a.retrieveNat(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Retrieve(nat) %s = %s", what, protojson.Format(got))
	if !proto.Equal(got, want) {
		t.Fatalf("Retrieve(nat) != canonical desired:\nwant %s", protojson.Format(want))
	}
}

// ---- the test ------------------------------------------------------------------------------------

func TestDet44MapDsliteCnatOnHost(t *testing.T) {
	// D-211 forbids DS-Lite pool deletion on the shared VPP. This historical
	// scenario uses it in restart/cleanup; park execution until a verified VPP
	// fix and reviewed safe harness exist. The pool-free arc split test remains.
	t.Skip("D-211: full DS-Lite acceptance deferred until verified VPP fix and reviewed safe harness")
	if os.Getenv("NGFW_INTEGRATION") != "1" {
		t.Skip("F-det44-map-dslite-cnat-host topology test: set NGFW_INTEGRATION=1 (host VPP, rig) — run.sh does")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (netns, veth, VPP API socket)")
	}
	s := slotFromEnv(t)
	flock(t, labLock, syscallLockSH)
	restarts0 := nRestarts(t, "before the test")
	t.Cleanup(func() {
		n := nRestarts(t, "after the test")
		if n != restarts0 {
			t.Errorf("VPP restarted during the test: NRestarts %d → %d", restarts0, n)
		}
	})
	conn := connectVPP(t)
	h := s.hex6()
	f := &fixture{s: s, r: newRig(s), conn: conn,
		v6wan: "fd00:" + h + ":2::1", v6peer: "fd00:" + h + ":2::2", brAddr: "fd00:" + h + "::1",
		cePfx: "fd00:" + h + ":65::/64", ceAddr: "fd00:" + h + ":65::1", shared: s.addr(65, 1), vip: s.addr(2, 100), be2: s.addr(2, 3)}
	f.sc = scope{s: s, v4: netip.MustParsePrefix(fmt.Sprintf("10.%d.0.0/16", s.num)), tag: s.prefix + ":"}
	r := f.r
	f.ifDoc = `{"` + r.lanIf + `": {"enabled": true, "description": "rig lan (det44 inside, MAP-E IPv4 side)", "ipv4": ["` + r.lanGW + `/24"]},
	  "` + r.wanIf + `": {"enabled": true, "description": "rig wan (det44 outside, MAP-E IPv6 side, CNAT backends)", "ipv4": ["` + r.wanGW + `/24"], "ipv6": ["` + f.v6wan + `/64"]}}`

	// ---- globals window (D-167): exclusive globals lock inside the shared lab lock, ≤ 10 min, recorded
	winStart := time.Now()
	flock(t, globalsLock, syscallLockEX)
	t.Logf("GLOBALS WINDOW start %s (flock -x %s inside flock -s %s)", winStart.Format(time.RFC3339), globalsLock, labLock)
	t.Cleanup(func() {
		d := time.Since(winStart)
		t.Logf("GLOBALS WINDOW end %s (held %.0fs)", time.Now().Format(time.RFC3339), d.Seconds())
		if d > 10*time.Minute {
			t.Errorf("globals window exceeded 10 min: %s", d)
		}
	})
	if os.Getenv(det44HostEnv) == "1" {
		wasOn := enableDet44(t, conn)
		t.Logf("det44 plugin enable (V9 window step, %s=1): det44_plugin_enable_disable enable=1 inside_vrf=0 outside_vrf=0 → was already enabled: %v (never disabled; stays enabled until the next VPP restart)", det44HostEnv, wasOn)
		f.det44 = true
	} else {
		t.Logf("%s unset: the det44 plugin is not enabled by this run; det44 phases run only if the slot's det44 objects apply", det44HostEnv)
		f.det44 = true // the apply below tells: det44_interface_add_del_feature fails on a disabled plugin
	}
	f.aftr = aftrFixture(t, conn, "fd00:"+h+"::aa")
	nRestarts(t, "after the global fixtures")

	// ---- rig; its VPP side is handed to the agent; peers stay down until the V19 guard
	t.Log(mustRun(t, s.lab, "rig", "up", s.prefix))
	t.Cleanup(func() {
		out, err := run(t, s.lab, "rig", "down", s.prefix)
		t.Logf("rig down: %v\n%s", err, out)
	})
	r.peers(t, false)
	for _, l := range handToAgent(t, conn, r.lanDev, r.wanDev) {
		t.Log("rig VPP side handed to the agent: " + l)
	}
	// runs after the agent stopped (registered before newAgent → LIFO): a failed run never reaches rev6, so the
	// slot's CE static route would stay in table 0 (D-216); the harness removes its own route here.
	t.Cleanup(func() { t.Log(dropSlotRoute(t, conn, f.cePfx)) })
	f.a = newAgent(t, s)
	f.work = f.a.work
	// runs before the agent stops (LIFO): NAT objects a failed step left behind are removed via the binary API
	t.Cleanup(func() {
		if f.sc.ifIdx == nil {
			return
		}
		if left := f.sc.ours(t, conn); left.count() > 0 {
			t.Logf("cleanup: NAT objects of the slot left by a failed step: %s", left)
			for _, l := range f.sc.loss(t, conn) {
				t.Log("cleanup: " + l)
			}
		}
	})

	t.Run("config", func(t *testing.T) {
		f.a.mustApply(t, "det44-rev1-interfaces", f.doc(`{}`))
		idx := waitIfs(t, conn, r.lanIf, r.wanIf)
		f.sc.ifIdx = map[uint32]string{idx[r.lanIf]: r.lanIf, idx[r.wanIf]: r.wanIf}
		t.Logf("interfaces created by the agent: %s=%d %s=%d", r.lanIf, idx[r.lanIf], r.wanIf, idx[r.wanIf])

		full := f.doc(`{` + f.det44Part() + `, ` + f.dslitePart() + `, ` + f.mapPart() + `, ` + f.cnatPart() + `}`)
		resp := f.a.apply(t, "det44-rev2-full", full)
		if resp.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
			msg := protojson.Format(resp)
			if os.Getenv(det44HostEnv) != "1" && strings.Contains(msg, "det44") {
				t.Skipf("det44 is not enabled on this VPP and %s is unset (V9 window step not taken): %s", det44HostEnv, trunc(msg, 1500))
			}
			t.Fatalf("Apply rev2 → %s: %s", resp.GetStatus(), msg)
		}
		t.Logf("Apply det44-rev2-full → %s results=%d: %s", resp.GetStatus(), len(resp.GetResults()), trunc(protojson.Format(resp), 2500))
		f.canon = f.canonical(t, true, true)
		f.mustRetrieve(t, f.canon, "after rev 2")
		if again := f.a.mustApply(t, "det44-rev2-again", full); len(again.GetResults()) != 0 {
			t.Fatalf("re-apply of the same document changed %d objects: %s", len(again.GetResults()), protojson.Format(again))
		}
		t.Log("re-apply of the same document: no result (nothing changed)")
		ours := f.sc.ours(t, conn)
		t.Logf("slot objects in VPP after rev 2: %d → %s", ours.count(), ours)
		if len(ours.det44If) != 2 || len(ours.det44Map) != 1 || len(ours.domains) != 1 || len(ours.trs) != 1 || len(ours.pool) != wantPools() {
			t.Fatalf("VPP holds %s", ours)
		}
		f.evidence(t)
		f.mustArc(t, "after rev 2 (det44 inside lan / outside wan)", 1, 1)
		nRestarts(t, "after config")
	})
	if t.Failed() {
		return
	}
	t.Run("restart", func(t *testing.T) { f.restart(t) })
	if t.Failed() {
		return
	}
	t.Run("det44-packets", func(t *testing.T) { f.det44Packets(t) })
	if t.Failed() {
		return
	}
	t.Run("map-cnat-packets", func(t *testing.T) { f.mapCnatPackets(t) })
	if t.Failed() {
		return
	}
	t.Run("rollback", func(t *testing.T) {
		f.a.mustApply(t, "det44-rev5-rollback", f.doc(`{}`))
		got, err := f.a.retrieveNat(t)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Retrieve(nat) after rollback = %s (size %d)", protojson.Format(got), proto.Size(got))
		if proto.Size(got) != 0 {
			t.Fatal("Retrieve still reports NAT objects after the rollback")
		}
		left := f.sc.ours(t, conn)
		t.Logf("slot objects left in det44 / map / cnat / dslite after rollback: %d %s", left.count(), left)
		if left.count() != 0 {
			t.Fatal("rollback left NAT objects in VPP")
		}
		f.mustArc(t, "after the rollback (nat {})", 0, 0)
		f.evidence(t)
		// V9: det44 stays enabled — the enable call answers "already enabled" (bare retval 1) and changes nothing
		t.Logf("det44 after rollback: det44_plugin_enable_disable enable=1 → already enabled: %v (never disabled, V9/D-068)", enableDet44(t, conn))
		t.Logf("dslite AFTR after rollback (a non-owner never resets it, D-071): %s", getAftr(t, conn))
		nRestarts(t, "after rollback")
	})
	t.Run("cleanup-through-agent", func(t *testing.T) {
		r.peers(t, false) // D-101: the agent deletes the af_packet interfaces only with their veths down
		f.a.mustApplyDomains(t, "det44-rev6-cleanup", `{"vrfs": {}, "interfaces": {}, "routing": {"static": []}, "nat": {}}`, "interfaces", "vrfs", "routing", "nat")
		ifs := dumpIfs(t, conn)
		_, l := ifs[r.lanIf]
		_, w := ifs[r.wanIf]
		t.Logf("after the interfaces were deleted through the agent: %s present=%v %s present=%v", r.lanIf, l, r.wanIf, w)
		if l || w {
			t.Error("the agent left a rig interface in VPP")
		}
	})
}

// restart: loss behind the agent's back + agent restart → everything back within 30 s; second restart without loss.
func (f *fixture) restart(t *testing.T) {
	f.a.p.stop(t)
	for _, l := range f.sc.loss(t, f.conn) {
		t.Log("simulated loss: " + l)
	}
	if left := f.sc.ours(t, f.conn); left.count() != 0 {
		t.Fatalf("after the loss VPP still holds %s", left)
	}
	t.Log("dumps after the loss: no det44 / map / cnat / dslite object of the slot")
	if c := f.arcState(t, "after the loss behind the agent's back (raw det44 delete, VPP 26.06 adds a node)"); c.lanIn == 0 || c.wanOut == 0 {
		t.Logf("after the loss the arcs hold %+v: this VPP's det44 delete removed its nodes (fixed delete path?)", c)
	}
	started := f.a.restart(t)
	took, got := f.retrieveUntil(t, f.canon, 30*time.Second)
	if !proto.Equal(got, f.canon) {
		t.Fatalf("Retrieve(nat) did not come back within 30 s: %s", protojson.Format(got))
	}
	ours := f.sc.ours(t, f.conn)
	t.Logf("NAT back in Retrieve %.2fs after the agent start (agent up at %s): %d objects → %s", took.Seconds(), started.Format(time.RFC3339Nano), ours.count(), ours)
	if len(ours.det44If) != 2 || len(ours.det44Map) != 1 || len(ours.domains) != 1 || len(ours.trs) != 1 || len(ours.pool) != wantPools() {
		t.Fatalf("after the restart VPP holds %s", ours)
	}
	f.mustArc(t, "after the agent restart (resync: leftover nodes repaired, one det44 add each)", 1, 1)
	for _, l := range strings.Split(f.a.p.output(), "\n") {
		if strings.Contains(l, "resync") || strings.Contains(l, "reconcile") || strings.Contains(l, `"created"`) || strings.Contains(l, "converged") {
			t.Log("agent log: " + trunc(l, 400))
		}
	}
	// second restart, no loss: the write-only objects (det44 enable, map interface features) are re-applied on the
	// resync and must not duplicate anything
	f.a.restart(t)
	if took, got := f.retrieveUntil(t, f.canon, 30*time.Second); !proto.Equal(got, f.canon) {
		t.Fatalf("second restart: Retrieve(nat) %s", protojson.Format(got))
	} else {
		t.Logf("second restart (no loss): Retrieve == canonical after %.2fs", took.Seconds())
	}
	time.Sleep(time.Second)
	ours = f.sc.ours(t, f.conn)
	t.Logf("slot objects after the resync re-apply: %d → %s", ours.count(), ours)
	if len(ours.domains) != 1 || len(ours.trs) != 1 || len(ours.det44If) != 2 || len(ours.det44Map) != 1 || len(ours.pool) != wantPools() {
		t.Fatalf("objects duplicated or lost by the re-apply: %s", ours)
	}
	f.mustArc(t, "after the second restart (no loss)", 1, 1)
	nRestarts(t, "after restart")
}

// det44Packets: rev 3 keeps det44 (+ dslite) only on the rig, then lan → wan TCP.
func (f *fixture) det44Packets(t *testing.T) {
	r, s := f.r, f.s
	f.a.mustApply(t, "det44-rev3-det44-only", f.doc(`{`+f.det44Part()+`, `+f.dslitePart()+`}`))
	f.mustRetrieve(t, f.canonical(t, true, false), "after rev 3 (det44 + dslite)")
	f.mustArc(t, "after rev 3 (det44 only)", 1, 1)
	idx := waitIfs(t, f.conn, r.lanIf, r.wanIf)
	for _, l := range v19Guard(t, f.conn, map[string]uint32{r.lanIf: idx[r.lanIf], r.wanIf: idx[r.wanIf]}) {
		t.Log("V19 guard: " + l)
	}
	if bin := os.Getenv("NGFW_PREFLIGHT_BIN"); bin != "" {
		out, err := run(t, bin)
		t.Logf("ngfw-vpp-preflight: %v\n%s", err, strings.TrimSpace(out))
		if err != nil {
			t.Fatal("ngfw-vpp-preflight did not exit 0 — no packet may cross the rig (D-095)")
		}
	}
	r.peers(t, true)
	// the wan netns routes the det44 outside block (outside its /24) back through VPP: default via wanGW
	server := script(t, f.work, "hold_server.py", holdServer)
	client := script(t, f.work, "hold_client.py", holdClient)
	if out, err := inNS(t, r.lanNS, "ping", "-n", "-c", "2", "-W", "1", r.wanIP); err != nil {
		t.Logf("warm-up ping (may lose the ARP round): %v\n%s", err, out)
	}
	inNSProc(t, "wan-server-8000", f.work, r.wanNS, "python3", server, r.wanIP, "8000")
	time.Sleep(500 * time.Millisecond)
	capW := startCapture(t, "tcpdump-wan-det44", f.work, r.wanNS, r.wanPeer, "tcp", "port", "8000")
	cl := inNSProc(t, "lan-client-40001", f.work, r.lanNS, "python3", client, r.lanIP, "40001", r.wanIP, "8000")
	time.Sleep(1500 * time.Millisecond)
	lines := capW.lines(t)
	t.Logf("tcpdump -i %s (netns %s) tcp port 8000:\n%s", r.wanPeer, r.wanNS, strings.Join(lines, "\n"))
	greeting := strings.TrimSpace(cl.output())
	t.Logf("lan client (%s:40001 → %s:8000) read: %q", r.lanIP, r.wanIP, greeting)
	var natSrc, natPort string
	for _, l := range lines {
		if src, port, dst, _ := v4Tuple(l); src != "" && dst == r.wanIP {
			if src == r.lanIP {
				t.Fatalf("the wan side saw the lan address %s: not translated", r.lanIP)
			}
			natSrc, natPort = src, port
			break
		}
	}
	if natSrc == "" {
		t.Fatal("no translated SYN seen in the wan netns")
	}
	block := netip.MustParsePrefix(s.addr(3, 0) + "/30")
	if !block.Contains(netip.MustParseAddr(natSrc)) {
		t.Fatalf("translated source %s is not in the det44 outside block %s", natSrc, block)
	}
	// the lookup RPC (det44_forward) must name exactly that address and a port block containing the port seen
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fw, err := f.a.c.Det44Lookup(ctx, &ngfwv1.Det44LookupRequest{Owner: s.prefix, InsideAddress: proto.String(r.lanIP)})
	if err != nil {
		t.Fatalf("Det44Lookup(inside %s): %v", r.lanIP, err)
	}
	t.Logf("Det44Lookup(inside %s) → outside %s ports %d-%d", r.lanIP, fw.GetOutsideAddress(), fw.GetPortLo(), fw.GetPortHi())
	p, _ := strconv.Atoi(natPort)
	if fw.GetOutsideAddress() != natSrc || uint32(p) < fw.GetPortLo() || uint32(p) > fw.GetPortHi() { //nolint:gosec // a port
		t.Fatalf("tcpdump saw %s:%s, the lookup says %s:%d-%d", natSrc, natPort, fw.GetOutsideAddress(), fw.GetPortLo(), fw.GetPortHi())
	}
	rev, err := f.a.c.Det44Lookup(ctx, &ngfwv1.Det44LookupRequest{Owner: s.prefix, OutsideAddress: proto.String(natSrc), OutsidePort: proto.Uint32(uint32(p))}) //nolint:gosec // a port
	if err != nil {
		t.Fatalf("Det44Lookup(outside %s:%d): %v", natSrc, p, err)
	}
	t.Logf("Det44Lookup(outside %s:%d) → inside %s", natSrc, p, rev.GetInsideAddress())
	if rev.GetInsideAddress() != r.lanIP {
		t.Fatalf("reverse lookup returned %s, want %s", rev.GetInsideAddress(), r.lanIP)
	}
	sess, err := f.a.c.Det44Sessions(ctx, &ngfwv1.Det44SessionsRequest{Owner: s.prefix, User: r.lanIP})
	if err != nil {
		t.Fatalf("Det44Sessions(%s): %v", r.lanIP, err)
	}
	t.Logf("Det44Sessions(user %s) → total %d outside %s ports %d-%d first %s", r.lanIP, sess.GetTotalSessions(), sess.GetOutsideAddress(), sess.GetPortLo(), sess.GetPortHi(), trunc(protojson.Format(sess), 600))
	raw := det44Sessions(t, f.conn, r.lanIP)
	t.Logf("det44_session_dump user %s: %d sessions (binary API)", r.lanIP, len(raw))
	if sess.GetTotalSessions() == 0 || len(raw) == 0 || !strings.Contains(greeting, "ok") {
		t.Fatalf("det44 session missing or the client got no greeting (sessions RPC %d, dump %d, greeting %q)", sess.GetTotalSessions(), len(raw), greeting)
	}
	t.Log("vppctl show det44 sessions (ours):\n" + grepLines(vppctl(t, "show", "det44", "sessions"), "10."+strconv.Itoa(s.num)+"."))
	t.Logf("DET44 packet path OK (path: af_packet): %s:40001 → %s:%s → %s:8000; sessions %d", r.lanIP, natSrc, natPort, r.wanIP, sess.GetTotalSessions())
	nRestarts(t, "after det44 packets")
}

// cnatDiag dumps what run 1 (2026-09-28: 0 of 12 VIP flows, no SYN on the wan side) lacked: the VIP's FIB entry, the
// cnat client/session tables, the drop counters and the lan interface's feature arcs.
func (f *fixture) cnatDiag(t *testing.T) {
	t.Helper()
	t.Log("diag: vppctl show ip fib " + f.vip + "/32:\n" + trunc(vppctl(t, "show", "ip", "fib", f.vip+"/32"), 2500))
	t.Log("diag: vppctl show cnat client:\n" + trunc(grepLines(vppctl(t, "show", "cnat", "client"), "10."+strconv.Itoa(f.s.num)+"."), 1500))
	t.Log("diag: vppctl show cnat session verbose 1 ip " + f.r.lanIP + " max 40:\n" + trunc(vppctl(t, "show", "cnat", "session", "verbose", "1", "ip", f.r.lanIP, "max", "40"), 4000))
	t.Log("diag: vppctl show node counters (cnat / det44 / drop / map lines):\n" + trunc(grepLines(vppctl(t, "show", "node", "counters"), "cnat", "det44", "No translation", "drop", "map", "unknown"), 2500))
	t.Log("diag: vppctl show interface features " + f.r.lanIf + ":\n" + trunc(vppctl(t, "show", "interface", "features", f.r.lanIf), 3000))
	t.Log("diag: vppctl show interface features " + f.r.wanIf + ":\n" + trunc(vppctl(t, "show", "interface", "features", f.r.wanIf), 3000))
}

// cnatPorts is the first lan source port of CNAT round `round` (0…4) of this run: 12 ports out of a 60-port block chosen
// by the half-minute (100-minute cycle, below the ephemeral range). VPP 26.06's cnat keeps a closed or failed flow's
// session (and its rewrite towards the old, by now deleted interfaces) until its session scanner reaches it — 100 µs
// per 1 s tick over a multi-million-bucket table, i.e. many minutes — and the data path still matches it: a new flow
// with the same 5-tuple follows the stale rewrite and is lost (F-det44-cnat-fix: a rerun 8 min after a 12/12 round
// on ports 43000+ connected 1 of 12). Every round of every run therefore uses 5-tuples no recent run used.
func cnatPorts(round int) string {
	return strconv.Itoa(20000 + int(time.Now().Unix()/30%200)*60 + round*12)
}

// cnatRound: 12 TCP flows from consecutive lan ports (cnatPorts(round)) to VIP:80; both backends (already listening)
// must be hit and every flow must complete. Diagnostics are dumped before a failure.
func (f *fixture) cnatRound(t *testing.T, label string, round int) map[string]int {
	t.Helper()
	r := f.r
	basePort := cnatPorts(round)
	many := script(t, f.work, "many_clients.py", manyClients)
	capC := startCapture(t, "tcpdump-wan-cnat-"+basePort, f.work, r.wanNS, r.wanPeer, "arp", "or", "tcp", "port", "8080")
	out, err := inNS(t, r.lanNS, "python3", many, r.lanIP, basePort, "12", f.vip, "80")
	t.Logf("%s: lan → VIP %s:80, 12 flows from port %s: %v\n%s", label, f.vip, basePort, err, strings.TrimSpace(out))
	cl2 := capC.lines(t)
	t.Logf("tcpdump -i %s (netns %s) arp or tcp port 8080:\n%s", r.wanPeer, r.wanNS, strings.Join(cl2, "\n"))
	hits := map[string]int{}
	for _, l := range cl2 {
		if src, _, dst, dport := v4Tuple(l); src == r.lanIP && dport == "8080" && strings.Contains(l, "Flags [S]") {
			hits[dst]++
		}
	}
	t.Logf("%s: CNAT VIP %s:80 → backends (SYNs per backend): %v; client lines: %d connected", label, f.vip, hits, strings.Count(out, "ok"))
	if hits[r.wanIP] == 0 || hits[f.be2] == 0 {
		f.cnatDiag(t)
		t.Fatalf("%s: the VIP did not load-balance to both backends: %v", label, hits)
	}
	if !strings.Contains(out, "connected 12 of 12") {
		f.cnatDiag(t)
		t.Fatalf("%s: not every VIP flow completed: %s", label, trunc(out, 800))
	}
	return hits
}

// mapCnatPackets: rev 4a (cnat + dslite: the CNAT path alone, det44 and MAP off the rig interfaces), then rev 4
// (map + cnat + dslite): MAP-E round trip and the CNAT round again with the map-e feature on the same interfaces.
func (f *fixture) mapCnatPackets(t *testing.T) {
	r, s := f.r, f.s
	f.a.mustApply(t, "det44-rev4a-cnat", f.doc(`{`+f.dslitePart()+`, `+f.cnatPart()+`}`))
	got4a, err := f.a.retrieveNat(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Retrieve(nat) after rev 4a (cnat + dslite) = %s", trunc(protojson.Format(got4a), 1500))

	// ---- IPv6 side of the rig (wan netns = the CE): address, routes, an ip6tnl decapsulating the BR's v4-in-v6
	for _, kv := range []string{"accept_ra=0", "autoconf=0", "accept_dad=0", "disable_ipv6=0"} {
		mustRun(t, "ip", "netns", "exec", r.wanNS, "sysctl", "-qw", "net.ipv6.conf."+r.wanPeer+"."+kv)
	}
	mustRun(t, "ip", "-n", r.wanNS, "-6", "addr", "add", f.v6peer+"/64", "dev", r.wanPeer, "nodad")
	mustRun(t, "ip", "-n", r.wanNS, "-6", "addr", "add", f.ceAddr+"/128", "dev", r.wanPeer, "nodad")
	mustRun(t, "ip", "-n", r.wanNS, "-6", "route", "replace", f.brAddr+"/128", "via", f.v6wan)
	tun := s.prefix + "ce"
	mustRun(t, "ip", "-n", r.wanNS, "link", "add", tun, "type", "ip6tnl", "mode", "ipip6", "local", f.ceAddr, "remote", f.brAddr, "encaplimit", "none")
	mustRun(t, "ip", "-n", r.wanNS, "addr", "add", f.shared+"/32", "dev", tun)
	mustRun(t, "ip", "-n", r.wanNS, "link", "set", tun, "up")
	// only the CE's own (shared MAP) address answers the lan through the tunnel (source policy, netns-private table);
	// the CNAT backends in the same netns answer natively on the wan veth. A plain "10.<N>.1.0/24 dev <tun>" route sent
	// the backends' SYN-ACKs into the ip6tnl as well (F-det44-cnat-fix: SYNs reached both backends, 0 of 12 connected).
	ceTable := strconv.Itoa(s.num*1000 + 65)
	mustRun(t, "ip", "-n", r.wanNS, "route", "replace", s.addr(1, 0)+"/24", "dev", tun, "table", ceTable)
	mustRun(t, "ip", "-n", r.wanNS, "rule", "add", "from", f.shared+"/32", "lookup", ceTable, "priority", "1000")
	// CNAT: the second backend is another address of the wan netns
	mustRun(t, "ip", "-n", r.wanNS, "addr", "add", f.be2+"/24", "dev", r.wanPeer)
	t.Cleanup(func() {
		for _, c := range [][]string{
			{"-n", r.wanNS, "addr", "del", f.be2 + "/24", "dev", r.wanPeer},
			{"-n", r.wanNS, "rule", "del", "from", f.shared + "/32", "lookup", ceTable, "priority", "1000"},
			{"-n", r.wanNS, "link", "del", tun},
			{"-n", r.wanNS, "-6", "route", "del", f.brAddr + "/128"},
			{"-n", r.wanNS, "-6", "addr", "del", f.ceAddr + "/128", "dev", r.wanPeer},
			{"-n", r.wanNS, "-6", "addr", "del", f.v6peer + "/64", "dev", r.wanPeer},
		} {
			_, _ = run(t, "ip", c...)
		}
		_, _ = run(t, "ip", "netns", "exec", r.wanNS, "sysctl", "-qw", "net.ipv6.conf."+r.wanPeer+".disable_ipv6=1")
	})
	server := script(t, f.work, "hold_server.py", holdServer)
	client := script(t, f.work, "hold_client.py", holdClient)

	// ---- CNAT alone (rev 4a): two backends in the wan netns, 12 flows from the lan netns
	inNSProc(t, "wan-be1-8080", f.work, r.wanNS, "python3", server, r.wanIP, "8080")
	inNSProc(t, "wan-be2-8080", f.work, r.wanNS, "python3", server, f.be2, "8080")
	time.Sleep(500 * time.Millisecond)
	t.Log("vppctl show cnat translation (ours):\n" + grepLines(vppctl(t, "show", "cnat", "translation"), "10."+strconv.Itoa(s.num)+"."))
	if c := f.arcState(t, "after rev 4a (det44 removed from the document)"); c.total() != 0 {
		t.Errorf("det44 removed from the document but the ip4-unicast arcs still hold det44 nodes: %+v", c)
	}
	hitsA := f.cnatRound(t, "rev 4a (cnat + dslite, no map)", 0)
	nRestarts(t, "after cnat packets (rev 4a)")

	// ---- rev 4: map-e on the rig interfaces + the LW4o6 domain, cnat and dslite unchanged
	f.a.mustApply(t, "det44-rev4-map-cnat", f.doc(`{`+f.dslitePart()+`, `+f.mapPart()+`, `+f.cnatPart()+`}`))
	f.mustRetrieve(t, f.canonical(t, false, true), "after rev 4 (map + cnat + dslite)")
	f.mustArc(t, "after rev 4 (map + cnat, no det44)", 0, 0)
	f.evidence(t)
	if out, err := inNS(t, r.wanNS, "ping", "-6", "-n", "-c", "2", "-W", "1", f.v6wan); err != nil {
		t.Logf("warm-up ping6 to the VPP wan address (may lose the ND round): %v\n%s", err, out)
	}
	t.Log("vppctl show ip6 fib (CE prefix, ours): " + strings.TrimSpace(grepLines(vppctl(t, "show", "ip6", "fib", f.cePfx), f.cePfx, f.v6peer)))

	// ---- MAP-E: lan → shared address, port of PSID 1 (offset 6, length 4: port = a<<10 | psid<<6 | m; 1100 = 1,1,12)
	inNSProc(t, "ce-server-1100", f.work, r.wanNS, "python3", server, f.shared, "1100")
	time.Sleep(500 * time.Millisecond)
	capW := startCapture(t, "tcpdump-wan-map", f.work, r.wanNS, r.wanPeer, "ip6", "and", "not", "icmp6")
	cl := inNSProc(t, "lan-client-map", f.work, r.lanNS, "python3", client, r.lanIP, "40002", f.shared, "1100")
	time.Sleep(2 * time.Second)
	lines := capW.lines(t)
	t.Logf("tcpdump -i %s (netns %s) ip6 and not icmp6:\n%s", r.wanPeer, r.wanNS, strings.Join(lines, "\n"))
	greeting := strings.TrimSpace(cl.output())
	t.Logf("lan client (%s:40002 → %s:1100 through the BR) read: %q", r.lanIP, f.shared, greeting)
	encap, decap := 0, 0
	for _, l := range lines {
		if strings.Contains(l, "IP6 "+f.brAddr+" > "+f.ceAddr) && strings.Contains(l, r.lanIP+".40002 > "+f.shared+".1100") {
			encap++
		}
		if strings.Contains(l, "IP6 "+f.ceAddr+" > "+f.brAddr) && strings.Contains(l, f.shared+".1100 > "+r.lanIP+".40002") {
			decap++
		}
	}
	t.Logf("MAP-E BR: %d packet(s) encapsulated by VPP (%s → %s carrying %s:40002 → %s:1100), %d answered by the CE towards the BR", encap, f.brAddr, f.ceAddr, r.lanIP, f.shared, decap)
	if encap == 0 {
		t.Fatal("no v4-in-v6 packet from the BR seen in the wan netns")
	}
	if !strings.Contains(greeting, "ok") {
		t.Fatalf("the lan client did not read the CE's greeting through the BR: %q", greeting)
	}
	t.Log("vppctl show map domain (ours):\n" + grepLines(vppctl(t, "show", "map", "domain"), s.prefix+":", f.shared, f.brAddr, "psid", "PSID", "rule"))
	t.Log("vppctl show map stats: " + strings.TrimSpace(trunc(vppctl(t, "show", "map", "stats"), 800)))

	// ---- CNAT again with map-e on the same interfaces (rev 4): 12 more flows from port 42000
	hits := f.cnatRound(t, "rev 4 (map + cnat + dslite)", 1)
	t.Logf("CNAT with and without map-e on the rig interfaces: rev 4a %v, rev 4 %v", hitsA, hits)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cs, err := f.a.c.CnatSessions(ctx, &ngfwv1.CnatSessionsRequest{Owner: s.prefix, Limit: 50})
	if err != nil {
		t.Fatalf("CnatSessions: %v", err)
	}
	t.Logf("CnatSessions → total %d truncated %v first: %s", cs.GetTotalSessions(), cs.GetTruncated(), trunc(protojson.Format(cs), 900))
	t.Log("vppctl show cnat translation (ours):\n" + grepLines(vppctl(t, "show", "cnat", "translation"), "10."+strconv.Itoa(s.num)+"."))
	t.Log("vppctl show cnat session verbose 1 ip " + r.lanIP + " max 30:\n" + trunc(vppctl(t, "show", "cnat", "session", "verbose", "1", "ip", r.lanIP, "max", "30"), 3000))
	t.Logf("CNAT packet path OK (path: af_packet): %s → %s:80 → %s:8080 (%d) + %s:8080 (%d)", r.lanIP, f.vip, r.wanIP, hits[r.wanIP], f.be2, hits[f.be2])
	nRestarts(t, "after map/cnat packets")
}
