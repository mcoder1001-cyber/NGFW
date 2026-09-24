// Package neighborsra is F-neighbors-ra's host check (FAST MODE DoD 2 + 3): the neighbour / RA / proxy-ARP feature end
// to end against the REAL host VPP, on two prefixed loopbacks (loop<N>01, loop<N>02) and a slot VRF (table <N>001).
// No rig, no packets: learned (dynamic) entries are injected with ip_neighbor_add_del flags NONE, exactly the entry the
// data plane creates after ARP/ND, so the V19 packet-path risk never arises.
//
//	TestNeighborsRaHost
//	  commit          interfaces + VRF (base revision) → a static neighbour outside every connected subnet is refused
//	                  (400 problem+json, pointer) → RA (+ SLAAC prefix), proxy ARP, a proxy-ARP range, two static
//	                  neighbours (and DAD: a warning, the slot agent is not the globals owner) committed
//	  verify          agent Retrieve == the committed leaves; vppctl show ip neighbors / show ip6 interface / show arp proxy
//	  live table      injected learned entries in GET /state/neighbors (filters, paging); ARP flush of one interface
//	                  and of all configured interfaces (static entries stay); a foreign interface is refused
//	  restart-safety  stop the agent → delete every feature object behind its back via binapi → start the agent → all
//	                  back within 30 s without any config API call (agent log timestamps)
//	  rollback        to the base revision → Retrieve has none of the leaves, vppctl shows nothing
//	  cleanup         interfaces and VRF deleted through the API → nothing with the prefix remains
//
// Runs only with VRX_INTEGRATION=1, as root, with a slot prefix (w<N>), under flock -s on the lab lock (only for the run,
// D-094); every process it starts is stopped by PID; the slot database is created and dropped by deploy/dev/pg-test.sh.
// VPP is never restarted (D-012); NRestarts is checked before and after and the test fails if it rises (D-064).
package neighborsra

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	vppapi "go.fd.io/govpp/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"ngfw/agent/binapi/ip_neighbor"
	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// names of this slot's objects
type names struct {
	lo1, lo2   string // loop<N>01 (VRF red), loop<N>02 (default VRF)
	vrf        string
	table      uint32
	net1, net2 string // "10.<N>.1", "10.<N>.2"
	v6a, v6b   string // "2001:db8:<N>:1", "2001:db8:<N>:2"
	rangeLo    string
	rangeHi    string
}

func newNames(s slot) names {
	n := s.num
	return names{
		lo1: fmt.Sprintf("loop%d01", n), lo2: fmt.Sprintf("loop%d02", n),
		vrf: s.prefix + "red", table: uint32(1000*n + 1), //nolint:gosec // G115: slot 1–11
		net1: fmt.Sprintf("10.%d.1", n), net2: fmt.Sprintf("10.%d.2", n),
		v6a: fmt.Sprintf("2001:db8:%d:1", n), v6b: fmt.Sprintf("2001:db8:%d:2", n),
		rangeLo: fmt.Sprintf("10.%d.3.10", n), rangeHi: fmt.Sprintf("10.%d.3.20", n),
	}
}

// retrieve calls the agent's Retrieve (the acceptance's "Retrieve output") and returns protobuf JSON of our objects.
func retrieve(t *testing.T, s slot, nm names) (*vrxv1.DesiredState, string) {
	t.Helper()
	cc, err := grpc.NewClient("unix:"+s.socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer cc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	r, err := vrxv1.NewDataplaneClient(cc).Retrieve(ctx, &vrxv1.RetrieveRequest{Owner: s.prefix, Subsystems: []string{"interfaces", "vrfs", "routing"}})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	ds := r.GetDesiredState()
	mine := &vrxv1.DesiredState{Routing: ds.GetRouting(), Vrfs: map[string]*vrxv1.Vrf{}, Interfaces: map[string]*vrxv1.Interface{}}
	if v, ok := ds.GetVrfs()[nm.vrf]; ok {
		mine.Vrfs[nm.vrf] = v
	}
	for _, n := range []string{nm.lo1, nm.lo2} {
		if v, ok := ds.GetInterfaces()[n]; ok {
			mine.Interfaces[n] = v
		}
	}
	return mine, protojson.MarshalOptions{Multiline: true, Indent: "  "}.Format(mine)
}

func TestNeighborsRaHost(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("F-neighbors-ra host check: set VRX_INTEGRATION=1 (host VPP, PostgreSQL) — run.sh does")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (VPP API socket, vppctl)")
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
	nm := newNames(s)
	conn := connectVPP(t)
	st := newStack(t, s)
	a := st.api
	t.Cleanup(func() { cleanup(t, a, conn, nm) })

	// ---- base revision: the interfaces and the VRF, no feature leaf
	a.patch("", map[string]any{
		"vrfs": map[string]any{nm.vrf: map[string]any{"id": nm.table}},
		"interfaces": map[string]any{
			nm.lo1: map[string]any{"enabled": true, "vrf": nm.vrf, "ipv4": []string{nm.net1 + ".1/24"}, "ipv6": []string{nm.v6a + "::1/64"}},
			nm.lo2: map[string]any{"enabled": true, "ipv4": []string{nm.net2 + ".1/24"}, "ipv6": []string{nm.v6b + "::1/64"}},
		},
	})
	base := revisionOf(t, a.commit("nra-base"))
	t.Logf("base revision %d", base)

	// ---- validation failure: a static neighbour outside every connected subnet
	a.patch("/routing", map[string]any{"neighbors": map[string]any{"static": []any{
		map[string]any{"interface": nm.lo1, "ip": fmt.Sprintf("10.%d.200.5", s.num), "mac": "02:00:00:00:99:05"},
	}}})
	bad := a.must(400, "POST", "/api/v1/config/commit?comment=nra-bad", nil)
	t.Logf("commit with a static neighbour outside every connected subnet → %d %s", bad.status, trunc(bad.raw, 600))
	if !strings.Contains(bad.raw, `"pointer":"/routing/neighbors/static/0/ip"`) || !strings.Contains(bad.raw, "outside every connected subnet") {
		t.Fatalf("no pointer to the static neighbour: %s", bad.raw)
	}
	a.must(200, "POST", "/api/v1/config/discard", nil)

	// ---- the feature
	a.patch("", map[string]any{
		"vrfs": map[string]any{nm.vrf: map[string]any{"proxyArpRanges": []any{map[string]any{"low": nm.rangeLo, "high": nm.rangeHi}}}},
		"interfaces": map[string]any{
			nm.lo1: map[string]any{
				"proxyArp": true,
				"ipv6Ra": map[string]any{"suppress": false, "managed": true, "other": true, "lifetimeSec": 1800, "maxIntervalSec": 600, "minIntervalSec": 200,
					"prefixes": map[string]any{nm.v6a + "::/64": map[string]any{"validSec": 86400, "preferredSec": 14400}}},
			},
		},
		"routing": map[string]any{"neighbors": map[string]any{
			"static": []any{
				map[string]any{"interface": nm.lo1, "ip": nm.net1 + ".50", "mac": "02:00:00:00:91:50"},
				map[string]any{"interface": nm.lo2, "ip": nm.v6b + "::50", "mac": "02:00:00:00:92:50", "noFibEntry": true},
			},
			"dad": map[string]any{"transmits": 1, "delayMs": 1000},
		}},
	})
	res := a.commit("nra-feature")
	t.Logf("commit nra-feature: revision %d, warnings %s", revisionOf(t, res), js(res["warnings"]))
	if !strings.Contains(js(res["warnings"]), "/routing/neighbors/dad") {
		t.Errorf("DAD on a slot agent must be a warning (D-071): %s", js(res["warnings"]))
	}

	// ---- verify: Retrieve == desired, vppctl
	got, gotJS := retrieve(t, s, nm)
	t.Logf("agent Retrieve (our objects):\n%s", gotJS)
	checkApplied(t, got, nm)
	drift := a.must(200, "GET", "/api/v1/state/drift", nil)
	for _, ch := range asList(drift.body["changes"]) {
		p, _ := ch.(map[string]any)["pointer"].(string)
		if strings.Contains(p, nm.lo1) || strings.Contains(p, nm.lo2) || strings.Contains(p, nm.vrf) || strings.HasPrefix(p, "/routing/neighbors") {
			t.Errorf("drift on our leaves: %s", js(ch))
		}
	}
	nbrs := vppctl(t, "show", "ip", "neighbors")
	t.Logf("vppctl show ip neighbors (ours):\n%s", strings.Join(append(linesWith(nbrs, nm.lo1), linesWith(nbrs, nm.lo2)...), "\n"))
	if len(linesWith(nbrs, nm.net1+".50", "S", nm.lo1)) != 1 || len(linesWith(nbrs, nm.v6b+"::50", "S", nm.lo2)) != 1 {
		t.Fatalf("static neighbours missing in show ip neighbors:\n%s", nbrs)
	}
	ip6 := vppctl(t, "show", "ip6", "interface", nm.lo1)
	t.Logf("vppctl show ip6 interface %s:\n%s", nm.lo1, ip6)
	if !strings.Contains(ip6, nm.v6a+"::/64") {
		t.Fatalf("advertised prefix missing in show ip6 interface:\n%s", ip6)
	}
	proxy := vppctl(t, "show", "arp", "proxy")
	t.Logf("vppctl show arp proxy:\n%s", proxy)
	if !strings.Contains(proxy, nm.rangeLo) || !strings.Contains(proxy, nm.lo1) {
		t.Fatalf("proxy ARP range / interface missing:\n%s", proxy)
	}

	// ---- live table + flush (learned entries injected: flags NONE)
	i1, i2 := ifIndex(t, conn, nm.lo1), ifIndex(t, conn, nm.lo2)
	for _, e := range []struct {
		idx     uint32
		ip, mac string
	}{{i1, nm.net1 + ".20", "02:00:00:00:91:20"}, {i1, nm.net1 + ".21", "02:00:00:00:91:21"}, {i1, nm.v6a + "::20", "02:00:00:00:91:22"}, {i2, nm.net2 + ".30", "02:00:00:00:92:30"}} {
		neighbor(t, conn, e.idx, e.ip, e.mac, ip_neighbor.IP_API_NEIGHBOR_FLAG_NONE, true)
	}
	table := a.must(200, "GET", "/api/v1/state/neighbors?interface="+nm.lo1, nil)
	t.Logf("GET /api/v1/state/neighbors?interface=%s → %s", nm.lo1, trunc(table.raw, 1500))
	if n, _ := table.body["total"].(float64); n != 4 {
		t.Fatalf("want 4 entries on %s (1 static + 3 learned): %s", nm.lo1, table.raw)
	}
	page := a.must(200, "GET", "/api/v1/state/neighbors?state=dynamic&family=ipv4&sort=ip&page=2&pageSize=1&interface="+nm.lo1, nil)
	t.Logf("GET …?state=dynamic&family=ipv4&sort=ip&page=2&pageSize=1 → %s", page.raw)
	if items := asList(page.body["items"]); len(items) != 1 || items[0].(map[string]any)["ip"] != nm.net1+".21" {
		t.Fatalf("paging: %s", page.raw)
	}
	fl := a.must(200, "POST", "/api/v1/actions/arp-flush", map[string]any{"interface": nm.lo1})
	t.Logf("POST /api/v1/actions/arp-flush {interface:%s} → %s", nm.lo1, fl.raw)
	if fl.body["deleted"] != float64(3) {
		t.Fatalf("flush deleted %v, want 3", fl.body["deleted"])
	}
	nbrs = vppctl(t, "show", "ip", "neighbors")
	if l := linesWith(nbrs, nm.lo1); len(l) != 1 || !strings.Contains(l[0], nm.net1+".50") {
		t.Fatalf("after the flush only the static entry may stay on %s:\n%s", nm.lo1, strings.Join(l, "\n"))
	}
	t.Logf("vppctl show ip neighbors after the flush (ours):\n%s", strings.Join(append(linesWith(nbrs, nm.lo1), linesWith(nbrs, nm.lo2)...), "\n"))
	all := a.must(200, "POST", "/api/v1/actions/arp-flush", map[string]any{})
	t.Logf("POST /api/v1/actions/arp-flush {} (every configured interface) → %s", all.raw)
	if all.body["deleted"] != float64(1) || all.body["interfaces"] != float64(2) {
		t.Fatalf("flush all: %s", all.raw)
	}
	foreign := a.call("POST", "/api/v1/actions/arp-flush", map[string]any{"interface": "local0"})
	t.Logf("POST /api/v1/actions/arp-flush {interface:local0} → %d %s", foreign.status, foreign.raw)
	if foreign.status != 400 {
		t.Fatalf("a foreign interface must be refused with 400: %d %s", foreign.status, foreign.raw)
	}
	audit := a.must(200, "GET", "/api/v1/audit?limit=5", nil)
	if !strings.Contains(audit.raw, `"action":"POST /api/v1/actions/arp-flush"`) {
		t.Fatalf("flush not audited: %s", audit.raw)
	}
	t.Logf("audit (newest first): %s", trunc(audit.raw, 900))

	// ---- restart safety: agent down, every feature object deleted behind its back, agent up
	st.agent.stop(t)
	neighbor(t, conn, i1, nm.net1+".50", "02:00:00:00:91:50", ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC, false)
	neighbor(t, conn, i2, nm.v6b+"::50", "02:00:00:00:92:50", ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC|ip_neighbor.IP_API_NEIGHBOR_FLAG_NO_FIB_ENTRY, false)
	raPrefixDelete(t, conn, i1, nm.v6a+"::/64")
	raReset(t, conn, i1)
	proxyArpInterface(t, conn, i1, false)
	proxyArpRange(t, conn, nm.table, nm.rangeLo, nm.rangeHi, false)
	gone := vppctl(t, "show", "ip", "neighbors")
	if len(linesWith(gone, nm.lo1)) != 0 || len(linesWith(gone, nm.lo2)) != 0 || strings.Contains(vppctl(t, "show", "arp", "proxy"), nm.rangeLo) {
		t.Fatalf("simulated loss incomplete:\n%s", gone)
	}
	t.Log("simulated loss: static neighbours, RA prefix + RA config, proxy-ARP interface and range deleted via binapi (agent stopped)")
	logFrom := fileSize(st.agentLog)
	t0 := time.Now()
	st.startAgent(t)
	back := waitFor(30*time.Second, func() bool {
		n := vppctl(t, "show", "ip", "neighbors")
		return len(linesWith(n, nm.net1+".50", nm.lo1)) == 1 && len(linesWith(n, nm.v6b+"::50", nm.lo2)) == 1 &&
			strings.Contains(vppctl(t, "show", "ip6", "interface", nm.lo1), nm.v6a+"::/64") &&
			strings.Contains(vppctl(t, "show", "arp", "proxy"), nm.rangeLo)
	})
	took := time.Since(t0)
	lines, raw := readAgentLog(t, st.agentLog, logFrom)
	for i, l := range lines {
		if strings.Contains(l.Msg, "reconcile") || strings.Contains(l.Msg, "resync") || strings.Contains(l.Msg, "listening") {
			t.Logf("agent log: %s", raw[i])
		}
	}
	if !back {
		t.Fatalf("feature objects not back within 30 s")
	}
	t.Logf("restart safety: everything back %.2f s after the agent start (no config API call)", took.Seconds())
	got, _ = retrieve(t, s, nm)
	checkApplied(t, got, nm)

	// ---- rollback to the base revision
	rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=nra-rollback", base), nil)
	t.Logf("POST /api/v1/config/rollback/%d → %s", base, trunc(rb.raw, 400))
	got, gotJS = retrieve(t, s, nm)
	t.Logf("agent Retrieve after the rollback (our objects):\n%s", gotJS)
	for _, leaf := range []string{"ipv6Ra", "proxyArp", "proxyArpRanges", "neighbors"} {
		if strings.Contains(gotJS, `"`+leaf+`"`) {
			t.Fatalf("rollback left %s in Retrieve", leaf)
		}
	}
	nbrs, ip6, proxy = vppctl(t, "show", "ip", "neighbors"), vppctl(t, "show", "ip6", "interface", nm.lo1), vppctl(t, "show", "arp", "proxy")
	t.Logf("after the rollback — show ip neighbors (ours): %q; show arp proxy: %q", append(linesWith(nbrs, nm.lo1), linesWith(nbrs, nm.lo2)...), strings.TrimSpace(proxy))
	t.Logf("after the rollback — show ip6 interface %s:\n%s", nm.lo1, ip6)
	if len(linesWith(nbrs, nm.lo1))+len(linesWith(nbrs, nm.lo2)) != 0 || strings.Contains(ip6, nm.v6a+"::/64") || strings.Contains(proxy, nm.rangeLo) || strings.Contains(proxy, nm.lo1) {
		t.Fatal("rollback left feature objects in VPP")
	}
	_ = got
}

// checkApplied asserts Retrieve carries every committed leaf in canonical form.
func checkApplied(t *testing.T, got *vrxv1.DesiredState, nm names) {
	t.Helper()
	l1 := got.GetInterfaces()[nm.lo1]
	ra := l1.GetIpv6Ra()
	switch {
	case ra == nil || ra.GetSuppress() || !ra.GetManaged() || !ra.GetOther() || ra.GetLifetimeSec() != 1800 || ra.GetMaxIntervalSec() != 600 || ra.GetMinIntervalSec() != 200:
		t.Fatalf("Retrieve ipv6Ra %v", ra)
	case ra.GetPrefixes()[nm.v6a+"::/64"].GetValidSec() != 86400 || ra.GetPrefixes()[nm.v6a+"::/64"].GetPreferredSec() != 14400:
		t.Fatalf("Retrieve RA prefix %v", ra.GetPrefixes())
	case !l1.GetProxyArp():
		t.Fatal("Retrieve proxyArp")
	case len(got.GetVrfs()[nm.vrf].GetProxyArpRanges()) != 1 || got.GetVrfs()[nm.vrf].GetProxyArpRanges()[0].GetLow() != nm.rangeLo:
		t.Fatalf("Retrieve proxyArpRanges %v", got.GetVrfs()[nm.vrf])
	case len(got.GetRouting().GetNeighbors().GetStatic()) != 2:
		t.Fatalf("Retrieve static neighbours %v", got.GetRouting().GetNeighbors())
	case got.GetRouting().GetNeighbors().GetDad() != nil:
		t.Fatal("a slot agent applied DAD (D-071)")
	}
}

// cleanup deletes everything through the API (veths are not involved: loopbacks only) and asserts nothing remains.
func cleanup(t *testing.T, a *api, conn vppapi.Connection, nm names) {
	a.call("POST", "/api/v1/config/discard", nil)
	a.call("PATCH", "/api/v1/config", map[string]any{
		"interfaces": map[string]any{nm.lo1: nil, nm.lo2: nil},
		"vrfs":       map[string]any{nm.vrf: nil},
		"routing":    map[string]any{"neighbors": nil},
	}, "content-type", "application/merge-patch+json")
	r := a.call("POST", "/api/v1/config/commit?comment=nra-cleanup", nil)
	t.Logf("cleanup commit → %d %s", r.status, trunc(r.raw, 300))
	left := ifIndex(t, conn, nm.lo1) + ifIndex(t, conn, nm.lo2)
	ints := vppctl(t, "show", "interface")
	t.Logf("cleanup: %s/%s in VPP: %v; show interface lines: %q; show ip neighbors (ours): %q", nm.lo1, nm.lo2, left != 0,
		append(linesWith(ints, nm.lo1), linesWith(ints, nm.lo2)...), append(linesWith(vppctl(t, "show", "ip", "neighbors"), nm.lo1), linesWith(vppctl(t, "show", "ip", "neighbors"), nm.lo2)...))
	if left != 0 {
		t.Errorf("cleanup left %s/%s in VPP", nm.lo1, nm.lo2)
	}
}

func revisionOf(t *testing.T, body map[string]any) int {
	t.Helper()
	rev, _ := body["revision"].(map[string]any)
	id, ok := rev["id"].(float64)
	if !ok {
		t.Fatalf("no revision in %s", js(body))
	}
	return int(id)
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}
