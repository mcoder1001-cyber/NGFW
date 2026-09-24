// Package nat44ei6466nptv6 is F-nat44-ei-64-66-nptv6's topology test: NAT44-EI, NAT64, NAT66 and NPTv6 end to end
// against the REAL host VPP through the af_packet veth/netns rig (path: af_packet, D-010), with the real vrx-agent and
// vrx-api of the slot. The rig is IPv4-only; this test adds its IPv6 inside fd00:<slot hex>::/32 and removes it.
//
//	TestNatEI6466Nptv6
//	  config          interfaces + slot VRF (rev 1) → mode "ei" with a twice-NAT pool, NPTv6 /48 → /56: 400 + pointer →
//	                  EI + NAT66 + NPTv6 (rev 2) → Retrieve(nat) == canonical desired (NPTv6 is write-only: never
//	                  retrieved) → vppctl show nat44 ei / nat66 / npt66
//	  ei-packets      V19 guard + vrx-vpp-preflight → outbound PAT (tcpdump in the wan netns sees the pool address;
//	                  vppctl show nat44 ei sessions and GET /state/nat/ei/sessions) → kill through the API (audited)
//	  nptv6-packets   lan fd00:N:10::2 → wan fd00:N:2::2 over TCP: tcpdump in the wan netns sees a source in the
//	                  external prefix fd00:N:20::/48, the reply is translated back (the client reads the greeting)
//	  restart-ei      agent stopped → the npt66 binding, EI and NAT66 objects deleted via binapi (dependents first) →
//	                  agent started → back within 30 s, exactly one npt66 binding; a second restart without loss
//	                  re-applies the write-only binding and still leaves exactly one
//	  nat64           rev 3: EI removed, the rig interfaces in the slot VRF, NAT64 with the slot /96 in that VRF →
//	                  Retrieve == canonical → v6 client → [fd00:N:64::<v4>]:8000 reaches the IPv4 wan host (tcpdump
//	                  sees the NAT64 pool address), static BIB inbound, vppctl show nat64 session table all, the API
//	  restart-nat64   the same loss/restart for NAT64 + NAT66 + NPTv6
//	  rollback        to rev 1 → Retrieve has no NAT object, VPP holds none of ours, no npt66 binding of the slot, the
//	                  plugins stay enabled (D-071)
//
// Runs only with VRX_INTEGRATION=1, as root, with a slot prefix, under flock -s on the lab lock, the slot's nat44 lock
// (ED and EI are exclusive) and the globals lock shared (D-082); nat44-ei, nat64 and nat66 are fixtures (enabled only
// if off, disabled again only if this test enabled them and they are empty). NRestarts is checked before and after.
// VPP is never restarted; every process is stopped by PID; no packet trace (D-128).
package nat44ei6466nptv6

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	vppapi "go.fd.io/govpp/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

func dialAgent(t *testing.T, sock string) vrxv1.DataplaneClient {
	t.Helper()
	cc, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	return vrxv1.NewDataplaneClient(cc)
}

func retrieveNat(t *testing.T, c vrxv1.DataplaneClient) (*vrxv1.NatConfig, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := c.Retrieve(ctx, &vrxv1.RetrieveRequest{Subsystems: []string{"nat"}})
	if err != nil {
		return nil, err
	}
	return r.GetDesiredState().GetNat(), nil
}

func natJSON(t *testing.T, js string) *vrxv1.NatConfig {
	t.Helper()
	n := &vrxv1.NatConfig{}
	if err := protojson.Unmarshal([]byte(js), n); err != nil {
		t.Fatalf("nat json: %v\n%s", err, js)
	}
	return n
}

// grepLines keeps the lines of a vppctl output that mention any of the needles (the shared VPP holds other slots').
func grepLines(out string, needles ...string) string {
	var keep []string
	for _, l := range strings.Split(out, "\n") {
		for _, n := range needles {
			if strings.Contains(l, n) {
				keep = append(keep, l)
				break
			}
		}
	}
	return strings.Join(keep, "\n")
}

type fixture struct {
	s       slot
	r       rig
	a6      v6
	st      *stack
	a       *api
	c       vrxv1.DataplaneClient
	conn    vppapi.Connection
	sc      scope
	scripts string
	ei      *plugin
	loopIn  string // NAT66 inside / outside: slot loopbacks
	loopOut string
	rev1    int
	canon   *vrxv1.NatConfig
	natCfg  map[string]any
	binding map[string]string // interface → internal prefix of the npt66 binding (for the loss)
}

func (f *fixture) evidence(t *testing.T) {
	t.Helper()
	r, n := f.r, fmt.Sprintf("10.%d.", f.s.num)
	t.Log("vppctl show nat44 ei interfaces (ours):\n" + grepLines(vppctl(t, "show", "nat44", "ei", "interfaces"), r.lanIf, r.wanIf))
	t.Log("vppctl show nat44 ei addresses (ours):\n" + grepLines(vppctl(t, "show", "nat44", "ei", "addresses"), n))
	t.Log("vppctl show nat44 ei static mappings (ours):\n" + grepLines(vppctl(t, "show", "nat44", "ei", "static", "mappings"), n))
	t.Log("vppctl show nat66 interfaces (ours):\n" + grepLines(vppctl(t, "show", "nat66", "interfaces"), f.loopIn, f.loopOut))
	t.Log("vppctl show nat66 static mappings (ours):\n" + grepLines(vppctl(t, "show", "nat66", "static", "mappings"), "fd00:"+hexSlot(f.s)+":"))
	t.Log("vppctl show npt66 bindings (ours):\n" + strings.Join(f.sc.npt66Lines(t), "\n"))
}

func (f *fixture) evidence64(t *testing.T) {
	t.Helper()
	r, n, p6 := f.r, fmt.Sprintf("10.%d.", f.s.num), "fd00:"+hexSlot(f.s)+":"
	t.Log("vppctl show nat64 interfaces (ours):\n" + grepLines(vppctl(t, "show", "nat64", "interfaces"), r.lanIf, r.wanIf))
	t.Log("vppctl show nat64 prefix (ours):\n" + grepLines(vppctl(t, "show", "nat64", "prefix"), p6))
	t.Log("vppctl show nat64 pool (ours):\n" + grepLines(vppctl(t, "show", "nat64", "pool"), n))
	t.Log("vppctl show nat64 bib all (ours, static):\n" + grepLines(vppctl(t, "show", "nat64", "bib", "all"), p6))
}

// retrieveUntil polls Retrieve(nat) until it equals want (or the timeout) and returns how long it took.
func (f *fixture) retrieveUntil(t *testing.T, want *vrxv1.NatConfig, timeout time.Duration) (time.Duration, *vrxv1.NatConfig) {
	t.Helper()
	start := time.Now()
	var got *vrxv1.NatConfig
	waitFor(timeout, func() bool {
		g, err := retrieveNat(t, f.c)
		if err != nil {
			return false
		}
		got = g
		return proto.Equal(g, want)
	})
	return time.Since(start), got
}

func TestNatEI6466Nptv6(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("F-nat44-ei-64-66-nptv6 topology test: set VRX_INTEGRATION=1 (host VPP, rig, PostgreSQL) — run.sh does")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (netns, veth, VPP API socket)")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	slotLock(t, s, "nat44") // ED and EI are mutually exclusive; this slot's NAT tests run one at a time
	sharedFlock(t, globalsLock)
	restarts0 := nRestarts(t)
	t.Logf("systemctl show vpp -p NRestarts (before) = %d", restarts0)
	t.Cleanup(func() {
		n := nRestarts(t)
		t.Logf("systemctl show vpp -p NRestarts (after) = %d", n)
		if n != restarts0 {
			t.Errorf("VPP restarted during the test: NRestarts %d → %d", restarts0, n)
		}
	})
	conn := connectVPP(t)
	if edEnabled(t, conn) {
		t.Skip("nat44-ed is enabled on this VPP by another owner: ED and EI are mutually exclusive (a window is needed, questions file)")
	}
	f := &fixture{s: s, r: newRig(s), a6: newV6(s), conn: conn, binding: map[string]string{}}
	f.ei = ensureEI(t, conn)
	rc := eiRunningConfig(t, conn)
	t.Logf("nat44-ei running config (fixture, was on=%v): inside VRF %d outside VRF %d flags %d forwarding %v timeouts %+v", f.ei.wasOn, rc.insideVrf, rc.outsideV, rc.flags, rc.forwarding, rc.timeouts)
	if rc.insideVrf != 0 || rc.outsideV != 0 || rc.flags != 0 || rc.forwarding {
		t.Skipf("nat44-ei is enabled by another owner with another configuration (%+v): this slot's document cannot require it", rc)
	}
	p64 := ensure64(t, conn)
	p66 := ensure66(t, conn)
	_, _ = p64, p66
	f.sc = scope{s: s, v4: netip.MustParsePrefix(fmt.Sprintf("10.%d.0.0/16", s.num)), v6: netip.MustParsePrefix(fmt.Sprintf("fd00:%s::/32", hexSlot(s)))}
	r, a6 := f.r, f.a6
	f.loopIn, f.loopOut = fmt.Sprintf("loop%d61", s.num), fmt.Sprintf("loop%d62", s.num)

	// ---- rig; its VPP side is handed to the agent (created from the configuration); peers down until the V19 guard
	t.Log(mustRun(t, s.lab, "rig", "up", s.prefix))
	t.Cleanup(func() {
		out, err := run(t, s.lab, "rig", "down", s.prefix)
		t.Logf("rig down: %v\n%s", err, out)
	})
	r.peers(t, false)
	for _, l := range deleteBehindBack(t, conn, r.lanDev, r.wanDev) {
		t.Log("rig VPP side handed to the agent: " + l)
	}
	f.st = newStack(t, s)
	// runs before the stack stops (LIFO): whatever NAT object of the slot a failed step left behind is removed through
	// the binary API, dependents first (D-095 c), so the plugin fixtures can restore their previous state
	t.Cleanup(func() {
		if f.sc.ifIdx == nil {
			return
		}
		left := f.sc.ours(t, conn)
		bind := f.sc.npt66Lines(t)
		if len(left)+len(bind) > 0 {
			t.Logf("cleanup: NAT objects of the slot left by a failed step: %v %v", left, bind)
			for _, l := range f.sc.loss(t, conn, f.binding) {
				t.Log("cleanup: " + l)
			}
		}
	})
	f.a = f.st.api
	f.c = dialAgent(t, s.socket)
	f.scripts = f.st.work
	a := f.a
	tbl := fmt.Sprintf("%s-n64", s.prefix)
	tblID := s.num*1000 + 64

	t.Run("config", func(t *testing.T) {
		a.t = t
		a.patch("/vrfs", map[string]any{tbl: map[string]any{"id": tblID, "description": "NAT64 slot VRF"}})
		a.patch("/interfaces", map[string]any{
			r.lanIf:   map[string]any{"enabled": true, "description": "NAT inside (rig lan)", "ipv4": []string{r.lanGW + "/24"}, "ipv6": []string{a6.lanGW + "/64", a6.nptGW + "/64"}},
			r.wanIf:   map[string]any{"enabled": true, "description": "NAT outside (rig wan)", "ipv4": []string{r.wanGW + "/24"}, "ipv6": []string{a6.wanGW + "/64"}},
			f.loopIn:  map[string]any{"enabled": true, "description": "NAT66 inside", "ipv6": []string{fmt.Sprintf("fd00:%s:66:1::1/64", hexSlot(s))}},
			f.loopOut: map[string]any{"enabled": true, "description": "NAT66 outside", "ipv6": []string{fmt.Sprintf("fd00:%s:66:2::1/64", hexSlot(s))}},
		})
		c := a.commit("nat-ei-rev1-interfaces")
		f.rev1 = int(c["revision"].(map[string]any)["id"].(float64))
		t.Logf("commit interfaces + VRF %s (%d) → %v revision %d", tbl, tblID, c["status"], f.rev1)

		// validation failures → 400 problem+json with the pointer; running untouched
		for _, bad := range []struct {
			patch   map[string]any
			pointer string
		}{
			{map[string]any{"mode": "ei", "inside": []string{r.lanIf}, "pools": []any{map[string]any{"name": "tn", "range": r.addr(2, 120), "twiceNat": true}}}, "/nat/pools/0/twiceNat"},
			{map[string]any{"nptv6": map[string]any{"bindings": []any{map[string]any{"interface": r.wanIf, "internal": a6.nptInternal, "external": strings.Replace(a6.nptExternal, "/48", "/56", 1)}}}}, "/nat/nptv6/bindings/0"},
		} {
			a.patch("/nat", bad.patch)
			rsp := a.call("POST", "/api/v1/config/commit?comment=nat-ei-bad", nil)
			t.Logf("commit %s → %d %s", js(bad.patch), rsp.status, rsp.raw)
			if rsp.status != 400 || !strings.Contains(rsp.raw, `"pointer":"`+bad.pointer) {
				t.Fatalf("want 400 problem+json with a pointer under %s", bad.pointer)
			}
			a.must(200, "POST", "/api/v1/config/discard", nil)
		}

		// rev 2: NAT44-EI (outbound PAT pool + a port forward), NAT66 on the slot loopbacks, NPTv6 on the wan interface
		f.natCfg = map[string]any{
			"mode": "ei", "inside": []string{r.lanIf}, "outside": []string{r.wanIf},
			"pools": []any{map[string]any{"name": "pat", "description": "outbound PAT", "range": r.addr(2, 100) + "-" + r.addr(2, 103)}},
			"staticMappings": []any{map[string]any{"name": "web", "description": "port forward :8080 → lan :80", "protocol": "tcp",
				"local": map[string]any{"ip": r.lanIP, "port": 80}, "external": map[string]any{"ip": r.addr(2, 110), "port": 8080}}},
			"nat66": map[string]any{"enabled": true, "inside": []string{f.loopIn}, "outside": []string{f.loopOut},
				"staticMappings": []any{map[string]any{"description": "server", "local": a6.nat66Local, "external": a6.nat66X}}},
			"nptv6": map[string]any{"bindings": []any{map[string]any{"description": "site", "interface": r.wanIf, "internal": a6.nptInternal, "external": a6.nptExternal}}},
		}
		a.patch("/nat", f.natCfg)
		d := a.must(200, "GET", "/api/v1/config/diff", nil)
		t.Logf("candidate diff: %s", trunc(d.raw, 3000))
		c2 := a.commit("nat-ei-rev2")
		t.Logf("commit EI + NAT66 + NPTv6 → %v revision %v results %s", c2["status"], c2["revision"].(map[string]any)["id"], js(c2["results"]))
		f.binding[r.wanIf] = a6.nptInternal

		f.canon = natJSON(t, fmt.Sprintf(`{"mode": "ei", "inside": [%q], "outside": [%q],
		  "pools": [{"range": "%s-%s", "twiceNat": false}],
		  "staticMappings": [{"name": "web", "protocol": "tcp", "local": {"ip": %q, "port": 80}, "external": {"ip": %q, "port": 8080}, "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false}],
		  "nat66": {"enabled": true, "inside": [%q], "outside": [%q], "staticMappings": [{"local": %q, "external": %q}]}}`,
			r.lanIf, r.wanIf, r.addr(2, 100), r.addr(2, 103), r.lanIP, r.addr(2, 110), f.loopIn, f.loopOut, a6.nat66Local, a6.nat66X))
		got, err := retrieveNat(t, f.c)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Retrieve(nat) = %s", protojson.Format(got))
		if !proto.Equal(got, f.canon) {
			t.Fatalf("Retrieve(nat) != canonical desired:\nwant %s", protojson.Format(f.canon))
		}
		t.Log("NPTv6 is write-only (npt66 has no dump, D-063): Retrieve never reports it; VPP shows it below")
		idx := waitIfs(t, conn, r.lanIf, r.wanIf, f.loopIn, f.loopOut)
		f.sc.ifIdx = map[uint32]string{idx[r.lanIf]: r.lanIf, idx[r.wanIf]: r.wanIf, idx[f.loopIn]: f.loopIn, idx[f.loopOut]: f.loopOut}
		f.evidence(t)
		ours := f.sc.ours(t, conn)
		t.Logf("slot objects in VPP: %d %v", len(ours), ours)
		if len(ours) != 2+4+1+2+1 || len(f.sc.npt66Lines(t)) != 1 {
			t.Fatalf("VPP holds %v + npt66 %v", ours, f.sc.npt66Lines(t))
		}
		for _, l := range v19Guard(t, conn, map[string]uint32{r.lanIf: idx[r.lanIf], r.wanIf: idx[r.wanIf]}) {
			t.Log("V19 guard: " + l)
		}
		if bin := os.Getenv("VRX_PREFLIGHT_BIN"); bin != "" {
			out, err := run(t, bin)
			t.Logf("vrx-vpp-preflight: %v\n%s", err, strings.TrimSpace(out))
			if err != nil {
				t.Fatal("vrx-vpp-preflight did not exit 0 — no packet may cross the rig (D-095)")
			}
		}
		r.peers(t, true)
		r.up6(t, a6)
	})
	if t.Failed() {
		return
	}
	t.Run("ei-packets", func(t *testing.T) { a.t = t; f.eiPackets(t) })
	if t.Failed() {
		return
	}
	t.Run("nptv6-packets", func(t *testing.T) { a.t = t; f.nptPackets(t) })
	if t.Failed() {
		return
	}
	t.Run("restart-ei", func(t *testing.T) { a.t = t; f.restart(t, f.canon) })
	if t.Failed() {
		return
	}
	t.Run("nat64", func(t *testing.T) { a.t = t; f.nat64(t, tbl, uint32(tblID)) }) //nolint:gosec // slot ≤ 11
	if t.Failed() {
		return
	}
	t.Run("restart-nat64", func(t *testing.T) { a.t = t; f.restart(t, f.canon) })
	if t.Failed() {
		return
	}
	t.Run("rollback", func(t *testing.T) {
		a.t = t
		rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=nat-ei-rollback", f.rev1), nil)
		t.Logf("POST /config/rollback/%d → %s", f.rev1, trunc(rb.raw, 2500))
		got, err := retrieveNat(t, f.c)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Retrieve(nat) after rollback = %s (size %d)", protojson.Format(got), proto.Size(got))
		if proto.Size(got) != 0 {
			t.Fatal("Retrieve still reports NAT objects after the rollback")
		}
		left, bind := f.sc.ours(t, conn), f.sc.npt66Lines(t)
		t.Logf("slot objects left in nat44-ei / nat64 / nat66 after rollback: %d %v; npt66 bindings of the slot: %v", len(left), left, bind)
		if len(left)+len(bind) != 0 {
			t.Fatal("rollback left NAT objects in VPP")
		}
		t.Logf("plugins after rollback (D-071: a slot never disables them): nat64 fixture held, nat66 fixture held, nat44-ei enabled=%v", eiRunningConfig(t, conn).enabled)
	})
	t.Run("cleanup-through-api", func(t *testing.T) {
		a.t = t
		r.peers(t, false) // D-101: the agent deletes the af_packet interfaces only with their veths down
		for _, n := range []string{r.lanIf, r.wanIf, f.loopIn, f.loopOut} {
			a.must(200, "DELETE", "/api/v1/config/interfaces/"+n, nil)
		}
		a.must(200, "DELETE", "/api/v1/config/vrfs/"+tbl, nil)
		c := a.commit("nat-ei-cleanup")
		t.Logf("commit (interfaces and VRF deleted) → %v revision %v", c["status"], c["revision"].(map[string]any)["id"])
		if left := f.sc.ours(t, conn); len(left) != 0 {
			t.Errorf("NAT objects of the slot left: %v", left)
		}
	})
}

// eiRow finds the API row of one inside endpoint in the EI session browser.
func (f *fixture) eiRow(t *testing.T, query string) map[string]any {
	t.Helper()
	rsp := f.a.must(200, "GET", "/api/v1/state/nat/ei/sessions?"+query, nil)
	items, _ := rsp.body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("GET /state/nat/ei/sessions?%s → %d items: %s", query, len(items), trunc(rsp.raw, 1500))
	}
	return items[0].(map[string]any)
}

func (f *fixture) eiPackets(t *testing.T) {
	r, a := f.r, f.a
	logs := f.st.work
	server := script(t, f.scripts, "hold_server.py", holdServer)
	client := script(t, f.scripts, "hold_client.py", holdClient)
	if out, err := inNS(t, r.lanNS, "ping", "-n", "-c", "2", "-W", "1", r.wanIP); err != nil {
		t.Logf("warm-up ping (may lose the ARP round): %v\n%s", err, out)
	}
	inNSProc(t, "wan-server-8000", logs, r.wanNS, "python3", server, r.wanIP, "8000")
	time.Sleep(500 * time.Millisecond)
	capW := startCapture(t, "tcpdump-wan-ei", logs, r.wanNS, r.wanPeer, "tcp", "port", "8000")
	cl := inNSProc(t, "lan-client-40001", logs, r.lanNS, "python3", client, r.lanIP, "40001", r.wanIP, "8000")
	time.Sleep(1500 * time.Millisecond)
	lines := capW.lines(t)
	t.Logf("tcpdump -i %s (netns %s) tcp port 8000:\n%s", r.wanPeer, r.wanNS, strings.Join(lines, "\n"))
	var natSrc, natPort string
	for _, l := range lines {
		if src, port := srcOf(l); src != r.wanIP && src != "" {
			if src == r.lanIP {
				t.Fatalf("the wan side saw the lan address %s: not translated", r.lanIP)
			}
			natSrc, natPort = src, port
			break
		}
	}
	pool := netip.MustParsePrefix(r.addr(2, 100) + "/30")
	if natSrc == "" || !pool.Contains(netip.MustParseAddr(natSrc)) {
		t.Fatalf("the wan side did not see a pool address (%s-%s) as source: %q", r.addr(2, 100), r.addr(2, 103), natSrc)
	}
	greet, _ := os.ReadFile(cl.log) //nolint:gosec // our own log
	t.Logf("NAT44-EI outbound PAT: %s:40001 → seen on the wan side as %s:%s (pool %s-%s); the client read %q", r.lanIP, natSrc, natPort, r.addr(2, 100), r.addr(2, 103), strings.TrimSpace(string(greet)))
	if !strings.Contains(string(greet), "vrx-nat-ok") {
		t.Fatal("the client did not read the server's greeting: the reply was not translated back")
	}
	vs := vppctl(t, "show", "nat44", "ei", "sessions", "detail", "filter", "saddr", r.lanIP)
	t.Log("vppctl show nat44 ei sessions detail filter saddr " + r.lanIP + ":\n" + vs)
	if !strings.Contains(vs, r.lanIP) || !strings.Contains(vs, natSrc) {
		t.Fatal("vppctl does not show the EI session")
	}
	row := f.eiRow(t, "inside="+r.lanIP+"&port=40001&protocol=tcp")
	t.Logf("GET /state/nat/ei/sessions?inside=%s&port=40001&protocol=tcp → %s", r.lanIP, js(row))
	if row["outsideAddress"] != natSrc || fmt.Sprint(row["outsidePort"]) != natPort {
		t.Fatalf("API row %v != the translation tcpdump saw (%s:%s)", row, natSrc, natPort)
	}

	// kill through the API (inside endpoint): gone from vppctl; a second kill → 404; both audited
	body := map[string]any{"protocol": "tcp", "insideAddress": r.lanIP, "insidePort": 40001}
	k := a.must(200, "POST", "/api/v1/actions/nat/ei/sessions/kill", body)
	t.Logf("POST /api/v1/actions/nat/ei/sessions/kill %s → %s", js(body), k.raw)
	vs = vppctl(t, "show", "nat44", "ei", "sessions", "detail", "filter", "saddr", r.lanIP)
	t.Log("vppctl show nat44 ei sessions detail filter saddr " + r.lanIP + " (after the kill):\n" + vs)
	if strings.Contains(vs, fmt.Sprintf("%s:%d", r.lanIP, 40001)) || strings.Contains(vs, "i2o "+r.lanIP+" proto TCP port 40001") {
		t.Fatal("the killed session is still in VPP")
	}
	k2 := a.call("POST", "/api/v1/actions/nat/ei/sessions/kill", body)
	t.Logf("second kill → %d %s", k2.status, k2.raw)
	if k2.status != 404 {
		t.Fatal("a second kill must be 404")
	}
	au := a.must(200, "GET", "/api/v1/audit?limit=10", nil)
	items, _ := au.body["items"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if strings.Contains(fmt.Sprint(m["action"]), "/actions/nat/ei/sessions/kill") {
			t.Logf("audit entry: action=%v resource=%v result=%v status=%v user=%v", m["action"], m["resource"], m["result"], m["status"], m["username"])
		}
	}
}

func (f *fixture) nptPackets(t *testing.T) {
	r, a6 := f.r, f.a6
	logs := f.st.work
	server := script(t, f.scripts, "hold_server6.py", holdServer6)
	once := script(t, f.scripts, "connect_once6.py", connectOnce6)
	for _, x := range []struct{ ns, dst string }{{r.lanNS, a6.nptGW}, {r.wanNS, a6.wanGW}} { // hosts start the NDP exchange
		if out, err := inNS(t, x.ns, "ping", "-6", "-n", "-c", "2", "-W", "1", x.dst); err != nil {
			t.Logf("warm-up ping6 %s (may lose the first round): %v\n%s", x.dst, err, out)
		}
	}
	inNSProc(t, "wan-server6-8006", logs, r.wanNS, "python3", server, a6.wanHost, "8006")
	time.Sleep(500 * time.Millisecond)
	capW := startCapture(t, "tcpdump-wan-npt", logs, r.wanNS, r.wanPeer, "ip6", "and", "tcp", "port", "8006")
	out, err := inNS(t, r.lanNS, "python3", once, a6.nptHost, "45001", a6.wanHost, "8006")
	lines := capW.lines(t)
	t.Logf("lan [%s]:45001 → wan [%s]:8006: %v %s", a6.nptHost, a6.wanHost, err, strings.TrimSpace(out))
	t.Logf("tcpdump -i %s (netns %s) ip6 and tcp port 8006:\n%s", r.wanPeer, r.wanNS, strings.Join(lines, "\n"))
	ext := netip.MustParsePrefix(a6.nptExternal)
	in := netip.MustParsePrefix(a6.nptInternal)
	var seen string
	for _, l := range lines {
		src, _ := srcOf6(l)
		if src == "" || src == a6.wanHost {
			continue
		}
		ad := netip.MustParseAddr(src)
		if in.Contains(ad) {
			t.Fatalf("the wan side saw the internal address %s: not translated", src)
		}
		if ext.Contains(ad) {
			seen = src
		}
	}
	if seen == "" {
		t.Fatalf("the wan side saw no source in the external prefix %s", a6.nptExternal)
	}
	t.Logf("NPTv6: %s → seen on the wan side as %s (external prefix %s; the interface id / subnet word carries the RFC 6296 checksum adjustment)", a6.nptHost, seen, a6.nptExternal)
	if err != nil || !strings.Contains(out, "vrx-nat-ok") {
		t.Fatal("the lan host did not read the greeting: the reply was not translated back to the internal prefix")
	}
	t.Log("vppctl show errors (npt66 counters):\n" + grepLines(vppctl(t, "show", "errors"), "npt66"))
}

func (f *fixture) nat64(t *testing.T, tbl string, tblID uint32) {
	r, a6, a := f.r, f.a6, f.a
	// rev 3: EI removed (NAT44 without objects is off), the rig interfaces in the slot VRF, NAT64 with the slot /96 in
	// that VRF (nat64 translates by the inside interface's IPv6 FIB); NAT66 and NPTv6 stay
	bibIn, bibOut := a6.lanClient, r.addr(64, 2)
	a.patch("/interfaces", map[string]any{r.lanIf: map[string]any{"vrf": tbl}, r.wanIf: map[string]any{"vrf": tbl}})
	a.patch("/nat", map[string]any{
		"inside": []string{}, "outside": []string{}, "pools": []any{}, "staticMappings": []any{},
		"nat64": map[string]any{"enabled": true, "inside": []string{r.lanIf}, "outside": []string{r.wanIf},
			"prefixes": []any{map[string]any{"prefix": a6.nat64Prefix, "vrf": tbl}},
			"pools":    []any{map[string]any{"range": r.addr(64, 1) + "-" + r.addr(64, 2), "vrf": tbl}},
			"staticBibs": []any{map[string]any{"description": "web", "protocol": "tcp",
				"inside": map[string]any{"ip": bibIn, "port": 80}, "outside": map[string]any{"ip": bibOut, "port": 8080}, "vrf": tbl}}},
	})
	d := a.must(200, "GET", "/api/v1/config/diff", nil)
	t.Logf("candidate diff: %s", trunc(d.raw, 3000))
	c := a.commit("nat-ei-rev3-nat64")
	t.Logf("commit NAT64 (EI removed, interfaces in VRF %s) → %v revision %v results %s", tbl, c["status"], c["revision"].(map[string]any)["id"], trunc(js(c["results"]), 4000))
	f.ei.release(t) // nat44-ei holds nothing of this slot any more: give it back now (other slots' ED tests wait for it)

	f.canon = natJSON(t, fmt.Sprintf(`{
	  "nat64": {"enabled": true, "inside": [%q], "outside": [%q], "prefixes": [{"prefix": %q, "vrf": %q}],
	    "pools": [{"range": "%s-%s", "vrf": %q}],
	    "staticBibs": [{"protocol": "tcp", "inside": {"ip": %q, "port": 80}, "outside": {"ip": %q, "port": 8080}, "vrf": %q}]},
	  "nat66": {"enabled": true, "inside": [%q], "outside": [%q], "staticMappings": [{"local": %q, "external": %q}]}}`,
		r.lanIf, r.wanIf, a6.nat64Prefix, tbl, r.addr(64, 1), r.addr(64, 2), tbl, bibIn, bibOut, tbl, f.loopIn, f.loopOut, a6.nat66Local, a6.nat66X))
	got, err := retrieveNat(t, f.c)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Retrieve(nat) = %s", protojson.Format(got))
	if !proto.Equal(got, f.canon) {
		t.Fatalf("Retrieve(nat) != canonical desired:\nwant %s", protojson.Format(f.canon))
	}
	f.evidence64(t)
	if left := f.sc.ours(t, f.conn); len(left) != 2+1+2+1+2+1 {
		t.Fatalf("VPP holds %v", left)
	}
	t.Logf("npt66 bindings of the slot (unchanged): %v", f.sc.npt66Lines(t))

	logs := f.st.work
	server := script(t, f.scripts, "hold_server.py", holdServer)
	client6 := script(t, f.scripts, "hold_client6.py", holdClient6)
	server6 := script(t, f.scripts, "hold_server6.py", holdServer6)
	once := script(t, f.scripts, "connect_once.py", connectOnce)
	if out, err := inNS(t, r.lanNS, "ping", "-6", "-n", "-c", "2", "-W", "1", a6.lanGW); err != nil { // the client starts NDP
		t.Logf("warm-up ping6 %s: %v\n%s", a6.lanGW, err, out)
	}
	dst := synth(a6.nat64Prefix, r.wanIP)
	inNSProc(t, "wan-server-8000-n64", logs, r.wanNS, "python3", server, r.wanIP, "8000")
	time.Sleep(500 * time.Millisecond)
	capW := startCapture(t, "tcpdump-wan-n64", logs, r.wanNS, r.wanPeer, "tcp", "port", "8000")
	cl := inNSProc(t, "lan-client6-46001", logs, r.lanNS, "python3", client6, a6.lanClient, "46001", dst, "8000")
	time.Sleep(2 * time.Second)
	lines := capW.lines(t)
	t.Logf("tcpdump -i %s (netns %s) tcp port 8000:\n%s", r.wanPeer, r.wanNS, strings.Join(lines, "\n"))
	pool := netip.MustParsePrefix(r.addr(64, 0) + "/30")
	var natSrc string
	for _, l := range lines {
		if src, _ := srcOf(l); src != "" && src != r.wanIP {
			natSrc = src
			break
		}
	}
	greet, _ := os.ReadFile(cl.log) //nolint:gosec // our own log
	t.Logf("NAT64: [%s]:46001 → [%s]:8000 (slot /96 %s) → seen on the IPv4 wan side from %s; the client read %q", a6.lanClient, dst, a6.nat64Prefix, natSrc, strings.TrimSpace(string(greet)))
	if natSrc == "" || !pool.Contains(netip.MustParseAddr(natSrc)) {
		t.Fatalf("the wan side did not see a NAT64 pool address (%s-%s) as source", r.addr(64, 1), r.addr(64, 2))
	}
	if !strings.Contains(string(greet), "vrx-nat-ok") {
		t.Fatal("the IPv6 client did not read the IPv4 server's greeting: the reply was not translated back")
	}
	st := vppctl(t, "show", "nat64", "session", "table", "all")
	t.Log("vppctl show nat64 session table all (ours):\n" + grepLines(st, a6.lanClient, "fd00:"+hexSlot(f.s)+":"))
	if !strings.Contains(st, a6.lanClient) {
		t.Fatal("vppctl does not show the NAT64 session")
	}
	rsp := a.must(200, "GET", "/api/v1/state/nat/nat64/sessions?protocol=tcp", nil)
	t.Logf("GET /state/nat/nat64/sessions?protocol=tcp → %s", trunc(rsp.raw, 1500))
	items, _ := rsp.body["items"].([]any)
	found := false
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m["client"] == a6.lanClient && fmt.Sprint(m["clientPort"]) == "46001" && m["remote"] == r.wanIP && m["remoteIpv6"] == dst {
			found = true
		}
	}
	if !found {
		t.Fatal("the API does not list the NAT64 session")
	}

	// static BIB inbound: the IPv4 wan host reaches [client]:80 through 10.N.64.2:8080
	inNSProc(t, "lan-server6-80", logs, r.lanNS, "python3", server6, a6.lanClient, "80")
	time.Sleep(500 * time.Millisecond)
	out, err := inNS(t, r.wanNS, "python3", once, r.wanIP, "41064", bibOut, "8080")
	t.Logf("static BIB: wan %s:41064 → %s:8080 (→ [%s]:80): %v %s", r.wanIP, bibOut, bibIn, err, strings.TrimSpace(out))
	if err != nil || !strings.Contains(out, "vrx-nat-ok") {
		t.Fatal("the static BIB did not forward the IPv4 connection to the IPv6 server")
	}
	t.Log("vppctl show nat64 session table all (ours, after the BIB connection):\n" + grepLines(vppctl(t, "show", "nat64", "session", "table", "all"), a6.lanClient))
}

// restart is the restart-safety simulation (FAST MODE): stop the slot's agent, delete its NAT objects via binapi
// (dependents first), start the agent → everything back within 30 s without a config API call, the write-only npt66
// binding exactly once; then a second restart without loss (the binding is re-applied, still exactly once).
func (f *fixture) restart(t *testing.T, want *vrxv1.NatConfig) {
	st := f.st
	st.agent.stop(t)
	for _, l := range f.sc.loss(t, f.conn, f.binding) {
		t.Log("simulated loss: " + l)
	}
	if left, bind := f.sc.ours(t, f.conn), f.sc.npt66Lines(t); len(left)+len(bind) != 0 {
		t.Fatalf("after the loss VPP still holds %v %v", left, bind)
	}
	t.Log("NAT dumps after the loss: no object of the slot; show npt66 bindings: none of the slot")
	from := fileSize(st.agentLog)
	started := time.Now()
	st.startAgent(t)
	took, got := f.retrieveUntil(t, want, 30*time.Second)
	if !proto.Equal(got, want) {
		t.Fatalf("Retrieve(nat) did not come back within 30 s: %s", protojson.Format(got))
	}
	var bind []string
	waitFor(10*time.Second, func() bool { bind = f.sc.npt66Lines(t); return len(bind) == 1 })
	_, raw := readAgentLog(t, st.agentLog, from)
	for _, l := range raw {
		if strings.Contains(l, "reconcile") || strings.Contains(l, "starting") || strings.Contains(l, "resync") || strings.Contains(l, `"created"`) {
			t.Log("agent log: " + trunc(l, 400))
		}
	}
	t.Logf("NAT back in Retrieve %.2fs after the agent start (no config API call; agent up at %s); npt66 bindings of the slot: %v", took.Seconds(), started.Format(time.RFC3339Nano), bind)
	if len(bind) != 1 {
		t.Fatalf("npt66: want exactly one binding of the slot, got %v", bind)
	}
	// second restart, no loss: the write-only binding is re-applied on the resync and must not be duplicated
	st.agent.stop(t)
	st.startAgent(t)
	if took, got := f.retrieveUntil(t, want, 30*time.Second); !proto.Equal(got, want) {
		t.Fatalf("second restart: Retrieve(nat) %s", protojson.Format(got))
	} else {
		t.Logf("second restart (no loss): Retrieve == canonical after %.2fs", took.Seconds())
	}
	time.Sleep(time.Second)
	bind = f.sc.npt66Lines(t)
	t.Logf("vppctl show npt66 bindings (ours) after the resync re-apply: %v", bind)
	if len(bind) != 1 {
		t.Fatalf("npt66 binding duplicated or lost by the re-apply: %v", bind)
	}
}
