// Package nat44edsessions is F-nat44-ed-sessions' topology test: NAT44-ED end to end against the REAL host VPP
// through the af_packet veth/netns rig (path: af_packet, D-010), with the real vrx-agent and vrx-api of the slot.
//
//	TestNat44EdSessions
//	  config        interfaces (rev 1) → overlapping / adjacent pools rejected 400 + pointer → NAT (rev 2): inside,
//	                outside, a PAT pool, a port forward, a 1:1 mapping → Retrieve(nat) == canonical desired, vppctl
//	  packets       V19 guard + vrx-vpp-preflight → outbound PAT (tcpdump in the wan netns sees the pool address; the
//	                same 5-tuple in vppctl and GET /state/nat/sessions) → port forward wan→external:8080 reaches the lan
//	                host on :80 (trace: nat44-ed-out2in) → 1:1 both directions → ≥ 2 000 sessions: pageSize=100 never
//	                returns more than 100, bounded gRPC messages → summary → kill through the API (gone from vppctl,
//	                audited)
//	  restart       stop the agent → delete the slot's mappings, pools and features via binapi (dependents first)
//	                → start the agent → NAT back within 30 s (agent log); sessions are lost (documented)
//	  rollback      to rev 1 → Retrieve has no NAT object, VPP has none of ours, the plugin stays enabled (D-071)
//
// Runs only with VRX_INTEGRATION=1, as root, with a slot prefix, under flock -s on the lab lock, the slot's nat44
// lock (ED/EI exclusive) and the globals lock shared (D-082); the plugin is a fixture (enabled only if off, disabled
// again only if this test enabled it and nat44-ed is empty). NRestarts is checked before and after. VPP is never
// restarted; every process is stopped by PID.
package nat44edsessions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
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
		t.Fatalf("nat json: %v", err)
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
	st      *stack
	a       *api
	c       vrxv1.DataplaneClient
	natCfg  map[string]any
	canon   *vrxv1.NatConfig
	ifIdx   map[uint32]string
	slotNet netip.Prefix
	scripts string
}

func TestNat44EdSessions(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("F-nat44-ed-sessions topology test: set VRX_INTEGRATION=1 (host VPP, rig, PostgreSQL) — run.sh does")
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
	if eiEnabled(t, conn) {
		t.Skip("nat44-ei is enabled on this VPP by another owner; ED and EI are mutually exclusive")
	}
	wasOn := ensurePlugin(t, conn)
	rc := natRunning(t, conn)
	t.Logf("nat44-ed running config (fixture, was on=%v): sessions/thread %d inside VRF %d outside VRF %d forwarding %v timeouts %+v", wasOn, rc.sessions, rc.insideVrf, rc.outsideV, rc.forwarding, rc.timeouts)
	if rc.insideVrf != 0 || rc.outsideV != 0 {
		t.Skipf("nat44-ed is enabled by another owner with inside/outside VRF %d/%d: this slot's document cannot require it", rc.insideVrf, rc.outsideV)
	}

	f := &fixture{s: s, r: newRig(s), slotNet: netip.MustParsePrefix(fmt.Sprintf("10.%d.0.0/16", s.num))}
	r := f.r
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
	f.a = f.st.api
	f.c = dialAgent(t, s.socket)
	f.scripts = f.st.work
	a := f.a

	var rev1 float64
	t.Run("config", func(t *testing.T) {
		a.t = t
		a.patch("/interfaces", map[string]any{
			r.lanIf: map[string]any{"enabled": true, "description": "NAT inside (rig lan)", "ipv4": []string{r.lanGW + "/24"}},
			r.wanIf: map[string]any{"enabled": true, "description": "NAT outside (rig wan)", "ipv4": []string{r.wanGW + "/24"}},
		})
		c := a.commit("nat-rev1-interfaces")
		rev1 = c["revision"].(map[string]any)["id"].(float64)
		t.Logf("commit interfaces → %v revision %v", c["status"], rev1)

		// validation failures: overlapping and adjacent pools → 400 problem+json with the pointer; running untouched
		for _, second := range []string{r.addr(2, 102) + "-" + r.addr(2, 110), r.addr(2, 104) + "-" + r.addr(2, 110)} {
			a.patch("/nat", map[string]any{"pools": []any{
				map[string]any{"name": "pat", "range": r.addr(2, 100) + "-" + r.addr(2, 103)},
				map[string]any{"name": "more", "range": second},
			}})
			rsp := a.call("POST", "/api/v1/config/commit?comment=nat-bad-pools", nil)
			t.Logf("commit with pools pat=%s-%s more=%s → %d %s", r.addr(2, 100), r.addr(2, 103), second, rsp.status, rsp.raw)
			if rsp.status != 400 || !strings.Contains(rsp.raw, `"pointer":"/nat/pools/1/range"`) {
				t.Fatalf("bad pools: want 400 problem+json with pointer /nat/pools/1/range")
			}
			a.must(200, "POST", "/api/v1/config/discard", nil)
		}

		// NAT44-ED: outbound PAT pool, a port forward, a 1:1 mapping. The slot is not the globals owner (D-071): the
		// plugin's session limit is a requirement, so the document states what the fixture found.
		f.natCfg = map[string]any{
			"mode": "ed", "inside": []string{r.lanIf}, "outside": []string{r.wanIf},
			"pools": []any{map[string]any{"name": "pat", "description": "outbound PAT", "range": r.addr(2, 100) + "-" + r.addr(2, 103)}},
			"staticMappings": []any{
				map[string]any{"name": "web", "description": "port forward :8080 → lan :80", "protocol": "tcp",
					"local": map[string]any{"ip": r.lanIP, "port": 80}, "external": map[string]any{"ip": r.addr(2, 110), "port": 8080}},
				map[string]any{"name": "one2one", "description": "1:1", "local": map[string]any{"ip": r.addr(1, 3)}, "external": map[string]any{"ip": r.addr(2, 111)}},
			},
		}
		if rc.sessions != 63*1024 {
			f.natCfg["sessionLimit"] = rc.sessions
		}
		a.patch("/nat", f.natCfg)
		d := a.must(200, "GET", "/api/v1/config/diff", nil)
		t.Logf("candidate diff: %s", d.raw)
		c2 := a.commit("nat-rev2")
		t.Logf("commit NAT → %v revision %v results %s", c2["status"], c2["revision"].(map[string]any)["id"], js(c2["results"]))

		f.canon = natJSON(t, fmt.Sprintf(`{"mode": "ed", "inside": [%q], "outside": [%q],
		  "pools": [{"range": "%s-%s", "twiceNat": false}],
		  "staticMappings": [
		    {"name": "one2one", "local": {"ip": %q}, "external": {"ip": %q}, "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false},
		    {"name": "web", "protocol": "tcp", "local": {"ip": %q, "port": 80}, "external": {"ip": %q, "port": 8080}, "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false}
		  ]}`, r.lanIf, r.wanIf, r.addr(2, 100), r.addr(2, 103), r.addr(1, 3), r.addr(2, 111), r.lanIP, r.addr(2, 110)))
		got, err := retrieveNat(t, f.c)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Retrieve(nat) = %s", protojson.Format(got))
		if !proto.Equal(got, f.canon) {
			t.Fatalf("Retrieve(nat) != canonical desired:\nwant %s", protojson.Format(f.canon))
		}
		idx := waitIfs(t, conn, r.lanIf, r.wanIf)
		f.ifIdx = map[uint32]string{idx[r.lanIf]: r.lanIf, idx[r.wanIf]: r.wanIf}
		t.Log("vppctl show nat44 interfaces (ours):\n" + grepLines(vppctl(t, "show", "nat44", "interfaces"), r.lanIf, r.wanIf))
		t.Log("vppctl show nat44 addresses (ours):\n" + grepLines(vppctl(t, "show", "nat44", "addresses"), fmt.Sprintf("10.%d.", s.num)))
		t.Log("vppctl show nat44 static mappings (ours):\n" + grepLines(vppctl(t, "show", "nat44", "static", "mappings"), fmt.Sprintf("10.%d.", s.num)))
		if len(ours(t, conn, s.prefix, f.slotNet, f.ifIdx)) != 2+4+2 {
			t.Fatalf("VPP holds %v", ours(t, conn, s.prefix, f.slotNet, f.ifIdx))
		}
		for _, l := range v19Guard(t, conn, idx) {
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
		mustRun(t, "ip", "-n", r.lanNS, "addr", "add", r.addr(1, 3)+"/24", "dev", r.lanPeer) // the 1:1 host
	})
	if t.Failed() {
		return
	}
	t.Run("packets", func(t *testing.T) { a.t = t; f.packets(t) })
	if t.Failed() {
		return
	}
	t.Run("restart-safety", func(t *testing.T) { a.t = t; f.restart(t) })
	if t.Failed() {
		return
	}
	t.Run("rollback", func(t *testing.T) {
		a.t = t
		rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=nat-rollback", int(rev1)), nil)
		t.Logf("POST /config/rollback/%d → %s", int(rev1), trunc(rb.raw, 1500))
		got, err := retrieveNat(t, f.c)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Retrieve(nat) after rollback = %s (size %d)", protojson.Format(got), proto.Size(got))
		if proto.Size(got) != 0 {
			t.Fatal("Retrieve still reports NAT objects after the rollback")
		}
		left := ours(t, conn, s.prefix, f.slotNet, f.ifIdx)
		t.Logf("slot objects left in nat44-ed after rollback: %d %v", len(left), left)
		if len(left) != 0 {
			t.Fatal("rollback left NAT objects in VPP")
		}
		rc := natRunning(t, conn)
		t.Logf("nat44-ed still enabled after rollback (D-071: a slot never disables it): %v (sessions/thread %d)", rc.enabled, rc.sessions)
		if !rc.enabled {
			t.Fatal("the rollback disabled the plugin")
		}
		t.Log("vppctl show nat44 interfaces (ours, after rollback):\n" + grepLines(vppctl(t, "show", "nat44", "interfaces"), r.lanIf, r.wanIf))
	})
	t.Run("cleanup-through-api", func(t *testing.T) {
		a.t = t
		r.peers(t, false) // D-101: the agent deletes the af_packet interfaces only with their veths down
		for _, n := range []string{r.lanIf, r.wanIf} {
			a.must(200, "DELETE", "/api/v1/config/interfaces/"+n, nil)
		}
		c := a.commit("nat-cleanup")
		t.Logf("commit (interfaces deleted) → %v revision %v", c["status"], c["revision"].(map[string]any)["id"])
		if left := ours(t, conn, s.prefix, f.slotNet, f.ifIdx); len(left) != 0 {
			t.Errorf("NAT objects of the slot left: %v", left)
		}
	})
}

// sessionRow finds the API row of one inside endpoint.
func (f *fixture) sessionRow(t *testing.T, query string) map[string]any {
	t.Helper()
	rsp := f.a.must(200, "GET", "/api/v1/state/nat/sessions?"+query, nil)
	items, _ := rsp.body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("GET /state/nat/sessions?%s → %d items: %s", query, len(items), trunc(rsp.raw, 1500))
	}
	return items[0].(map[string]any)
}

func (f *fixture) packets(t *testing.T) {
	r, a := f.r, f.a
	logs := f.st.work
	server := script(t, f.scripts, "hold_server.py", holdServer)
	client := script(t, f.scripts, "hold_client.py", holdClient)
	once := script(t, f.scripts, "connect_once.py", connectOnce)
	flows := script(t, f.scripts, "udp_flows.py", udpFlows)
	// ARP / first-packet warm-up
	if out, err := inNS(t, r.lanNS, "ping", "-n", "-c", "2", "-W", "1", r.wanIP); err != nil {
		t.Logf("warm-up ping (may lose the ARP round): %v\n%s", err, out)
	}

	// ---- outbound PAT: lan 10.N.1.2:40001 → wan 10.N.2.2:8000; the wan side sees a pool address
	inNSProc(t, "wan-server-8000", logs, r.wanNS, "python3", server, r.wanIP, "8000")
	time.Sleep(500 * time.Millisecond)
	capW := startCapture(t, "tcpdump-wan-pat", logs, r.wanNS, r.wanPeer, "tcp", "port", "8000")
	inNSProc(t, "lan-client-40001", logs, r.lanNS, "python3", client, r.lanIP, "40001", r.wanIP, "8000")
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
	pool, _ := netip.ParsePrefix(r.addr(2, 100) + "/30")
	if natSrc == "" || !pool.Contains(netip.MustParseAddr(natSrc)) {
		t.Fatalf("the wan side did not see a pool address (%s-%s) as source: %q", r.addr(2, 100), r.addr(2, 103), natSrc)
	}
	t.Logf("outbound PAT: %s:40001 → seen on the wan side as %s:%s (pool %s-%s)", r.lanIP, natSrc, natPort, r.addr(2, 100), r.addr(2, 103))
	vs := vppctl(t, "show", "nat44", "sessions", "filter", "i2o", "saddr", r.lanIP, "filter", "i2o", "sport", "40001")
	t.Log("vppctl show nat44 sessions filter i2o saddr " + r.lanIP + " filter i2o sport 40001:\n" + vs)
	if !strings.Contains(vs, fmt.Sprintf("i2o %s proto TCP port 40001", r.lanIP)) || !strings.Contains(vs, fmt.Sprintf("o2i %s proto TCP port %s", natSrc, natPort)) || !strings.Contains(vs, fmt.Sprintf("external host %s:8000", r.wanIP)) {
		t.Fatal("vppctl does not show the session with the same 5-tuple")
	}
	row := f.sessionRow(t, "inside="+r.lanIP+"&port=40001&protocol=tcp")
	t.Logf("GET /state/nat/sessions?inside=%s&port=40001&protocol=tcp → %s", r.lanIP, js(row))
	if row["insideAddress"] != r.lanIP || row["insidePort"] != float64(40001) || row["outsideAddress"] != natSrc || fmt.Sprint(row["outsidePort"]) != natPort ||
		row["externalAddress"] != r.wanIP || row["externalPort"] != float64(8000) || row["protocol"] != "tcp" {
		t.Fatal("the session browser row differs from vppctl / tcpdump")
	}

	// ---- port forward: wan 10.N.2.2:41001 → external 10.N.2.110:8080 reaches the lan host on :80
	inNSProc(t, "lan-server-80", logs, r.lanNS, "python3", server, r.lanIP, "80")
	time.Sleep(500 * time.Millisecond)
	vppctl(t, "trace", "add", "af-packet-input", "40")
	capL := startCapture(t, "tcpdump-lan-fwd", logs, r.lanNS, r.lanPeer, "tcp", "port", "80")
	out, err := inNS(t, r.wanNS, "python3", once, r.wanIP, "41001", r.addr(2, 110), "8080")
	t.Logf("wan %s:41001 → %s:8080: %v %s", r.wanIP, r.addr(2, 110), err, strings.TrimSpace(out))
	if err != nil || !strings.Contains(out, "vrx-nat-ok") {
		t.Fatal("the port forward did not reach the lan host")
	}
	lanLines := capL.lines(t)
	t.Logf("tcpdump -i %s (netns %s) tcp port 80:\n%s", r.lanPeer, r.lanNS, strings.Join(lanLines, "\n"))
	if len(lanLines) == 0 || !strings.Contains(lanLines[0], r.wanIP+".41001 > "+r.lanIP+".80") {
		t.Fatalf("the lan host did not receive %s:41001 → %s:80", r.wanIP, r.lanIP)
	}
	time.Sleep(300 * time.Millisecond)
	all := vppctl(t, "show", "trace", "max", "5000")
	tr, ok := natTraceBlock(all, "nat44-ed-out2in", "TCP: "+r.wanIP+" -> "+r.addr(2, 110), "41001")
	if !ok {
		t.Fatalf("no trace block with nat44-ed-out2in for %s:41001 -> %s; buffer starts:\n%s", r.wanIP, r.addr(2, 110), trunc(all, 3000))
	}
	t.Log("vppctl show trace (the port-forwarded SYN):\n" + tr)

	// ---- 1:1 both directions
	capW = startCapture(t, "tcpdump-wan-1to1", logs, r.wanNS, r.wanPeer, "tcp", "port", "8000", "and", "src", "host", r.addr(2, 111))
	out, err = inNS(t, r.lanNS, "python3", once, r.addr(1, 3), "42001", r.wanIP, "8000")
	if err != nil || !strings.Contains(out, "vrx-nat-ok") {
		t.Fatalf("1:1 outbound from %s failed: %v %s", r.addr(1, 3), err, out)
	}
	lines = capW.lines(t)
	t.Logf("1:1 outbound %s:42001 → %s:8000; tcpdump on the wan side:\n%s", r.addr(1, 3), r.wanIP, strings.Join(lines, "\n"))
	if len(lines) == 0 || !strings.Contains(lines[0], r.addr(2, 111)+".42001 >") {
		t.Fatalf("the wan side did not see %s (the 1:1 external address, port kept)", r.addr(2, 111))
	}
	inNSProc(t, "lan-server-9000", logs, r.lanNS, "python3", server, r.addr(1, 3), "9000")
	time.Sleep(500 * time.Millisecond)
	out, err = inNS(t, r.wanNS, "python3", once, r.wanIP, "43001", r.addr(2, 111), "9000")
	t.Logf("1:1 inbound %s:43001 → %s:9000 (lan %s:9000): %v %s", r.wanIP, r.addr(2, 111), r.addr(1, 3), err, strings.TrimSpace(out))
	if err != nil || !strings.Contains(out, "vrx-nat-ok") {
		t.Fatal("1:1 inbound did not reach the lan host")
	}

	// ---- ≥ 2 000 sessions from the lan host to our own wan address (never outside 10.N.0.0/16)
	out, err = inNS(t, r.lanNS, "python3", flows, r.lanIP, "20000", "2100", r.wanIP, "9")
	t.Logf("udp flows: %v %s", err, strings.TrimSpace(out))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	p1 := a.must(200, "GET", "/api/v1/state/nat/sessions?pageSize=100", nil)
	items, _ := p1.body["items"].([]any)
	total, _ := p1.body["total"].(float64)
	t.Logf("GET /state/nat/sessions?pageSize=100 → %d items, total %v, totalUsers %v, truncated %v (%d bytes)", len(items), total, p1.body["totalUsers"], p1.body["truncated"], len(p1.raw))
	if len(items) != 100 || total < 2000 {
		t.Fatalf("paging: %d items total %v", len(items), total)
	}
	for _, pg := range []int{2, 21} {
		pr := a.must(200, "GET", fmt.Sprintf("/api/v1/state/nat/sessions?pageSize=100&page=%d", pg), nil)
		n := len(pr.body["items"].([]any))
		t.Logf("GET /state/nat/sessions?pageSize=100&page=%d → %d items (%d bytes)", pg, n, len(pr.raw))
		if n > 100 {
			t.Fatalf("page %d has %d items", pg, n)
		}
	}
	for _, limit := range []uint32{100, 1000} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		resp, err := f.c.NatSessions(ctx, &vrxv1.NatSessionsRequest{Limit: limit})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("gRPC NatSessions limit=%d → %d sessions, total %d, next %d, message %d bytes (grpc-go default max 4 MiB)", limit, len(resp.GetSessions()), resp.GetTotalSessions(), resp.GetNextOffset(), proto.Size(resp))
		if uint32(len(resp.GetSessions())) > limit || proto.Size(resp) > 400*int(limit) {
			t.Fatalf("NatSessions limit %d: %d sessions, %d bytes", limit, len(resp.GetSessions()), proto.Size(resp))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_, err = f.c.NatSessions(ctx, &vrxv1.NatSessionsRequest{Limit: 1001})
	cancel()
	t.Logf("gRPC NatSessions limit=1001 → %v", err)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal("limit 1001 must be INVALID_ARGUMENT")
	}
	udp := a.must(200, "GET", "/api/v1/state/nat/sessions?protocol=udp&external="+r.wanIP+"&pageSize=5", nil)
	t.Logf("GET /state/nat/sessions?protocol=udp&external=%s&pageSize=5 → total %v, first %s", r.wanIP, udp.body["total"], js(udp.body["items"].([]any)[0]))
	sum := a.must(200, "GET", "/api/v1/state/nat/summary", nil)
	t.Logf("GET /state/nat/summary → %s", sum.raw)
	pools, _ := sum.body["pools"].([]any)
	if len(pools) != 1 || pools[0].(map[string]any)["name"] != "pat" || pools[0].(map[string]any)["sessions"].(float64) < 2000 {
		t.Fatal("summary: pool 'pat' with ≥ 2000 sessions expected")
	}
	vc := vppctl(t, "show", "nat44", "sessions", "filter", "i2o", "saddr", r.lanIP, "filter", "i2o", "proto", "udp")
	t.Logf("vppctl show nat44 sessions filter i2o saddr %s filter i2o proto udp: %d i2o lines", r.lanIP, strings.Count(vc, "  i2o "))

	// ---- kill through the API: gone from vppctl, audited
	body := map[string]any{"protocol": "tcp", "insideAddress": r.lanIP, "insidePort": 40001, "externalAddress": r.wanIP, "externalPort": 8000}
	k := a.must(200, "POST", "/api/v1/actions/nat/sessions/kill", body)
	t.Logf("POST /api/v1/actions/nat/sessions/kill %s → %s", js(body), k.raw)
	vs = vppctl(t, "show", "nat44", "sessions", "filter", "i2o", "saddr", r.lanIP, "filter", "i2o", "sport", "40001")
	t.Log("vppctl show nat44 sessions filter i2o saddr " + r.lanIP + " filter i2o sport 40001 (after the kill):\n" + vs)
	if strings.Contains(vs, "port 40001") {
		t.Fatal("the killed session is still in VPP")
	}
	k2 := a.call("POST", "/api/v1/actions/nat/sessions/kill", body)
	t.Logf("second kill → %d %s", k2.status, k2.raw)
	if k2.status != 404 {
		t.Fatal("a second kill must be 404")
	}
	au := a.must(200, "GET", "/api/v1/audit?limit=5", nil)
	var entries []any
	_ = json.Unmarshal([]byte(au.raw), &struct{ Items *[]any }{&entries})
	for _, e := range entries {
		m := e.(map[string]any)
		if strings.Contains(fmt.Sprint(m["action"]), "/actions/nat/sessions/kill") {
			t.Logf("audit entry: action=%v resource=%v result=%v status=%v user=%v", m["action"], m["resource"], m["result"], m["status"], m["username"])
		}
	}
	if !strings.Contains(au.raw, fmt.Sprintf("nat/sessions/tcp/%s:40001/%s:8000/default", r.lanIP, r.wanIP)) {
		t.Fatalf("no audit entry for the kill: %s", trunc(au.raw, 2000))
	}
}

func (f *fixture) restart(t *testing.T) {
	st, s := f.st, f.s
	vc := connectVPP(t)
	st.agent.stop(t)
	for _, l := range natLoss(t, vc, s.prefix, f.slotNet, f.ifIdx) {
		t.Log("simulated loss: " + l)
	}
	if left := ours(t, vc, s.prefix, f.slotNet, f.ifIdx); len(left) != 0 {
		t.Fatalf("still in VPP after the loss: %v", left)
	}
	t.Log("nat44-ed dumps after the loss: no feature, pool address or mapping of the slot")
	logFrom := fileSize(st.agentLog)
	t0 := time.Now()
	st.startAgent(t)
	f.c = dialAgent(t, s.socket)
	var got *vrxv1.NatConfig
	ok := waitFor(30*time.Second, func() bool {
		n, err := retrieveNat(t, f.c)
		if err != nil {
			return false
		}
		got = n
		return proto.Equal(n, f.canon)
	})
	back := time.Since(t0)
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
		case strings.Contains(raw[i], "nat44-ed") || strings.Contains(l.Msg, "reconcile"):
			t.Log("agent log: " + trunc(raw[i], 400))
		}
	}
	t.Logf("NAT back in Retrieve %.2fs after the agent start (no config API call); reconcile %s → %s = %.3fs (agent log)", back.Seconds(), tStart.Format(time.RFC3339Nano), tResync.Format(time.RFC3339Nano), tResync.Sub(tStart).Seconds())
	if !ok || back > 30*time.Second {
		t.Fatalf("NAT not back within 30 s: %s", protojson.Format(got))
	}
	t.Log("vppctl show nat44 interfaces (ours, after the restart):\n" + grepLines(vppctl(t, "show", "nat44", "interfaces"), f.r.lanIf, f.r.wanIf))
	t.Log("vppctl show nat44 static mappings (ours, after the restart):\n" + grepLines(vppctl(t, "show", "nat44", "static", "mappings"), fmt.Sprintf("10.%d.", s.num)))
	n := len(strings.Split(strings.TrimSpace(vppctl(t, "show", "nat44", "sessions", "filter", "i2o", "saddr", f.r.lanIP)), "\n"))
	t.Logf("sessions of %s after the loss + restart: vppctl prints %d lines (the pools' sessions went with the pool addresses — documented)", f.r.lanIP, n)
}
