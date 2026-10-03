package agent

// F-isis-rip topology test on this host (VRX_INTEGRATION=1 VRX_ISISRIP_TOPOLOGY=1, shared lab lock, slot prefix; row
// F-isis-rip-host, the RV-A R7/R4/R1 owed lists). Same layout as the F-ospf-host / P12-fib-proof topology tests, whose
// helpers it reuses (p12Env, rootFRR, the rig hand-over, hostConfig, dumpNames, …).
//
// This file lives in test/topology/isis-rip/agent-overlay/ (the row's file fence) and is compiled into
// apps/agent/internal/agent as isisrip_topology_integration_test.go by `test/topology/isis-rip/run.sh topology` through
// `go test -overlay` — nothing under apps/ is written.
//
//	VRX:   the in-process agent (owner = slot prefix, VRX_FRR_PATHSPACE = slot) → VPP af_packet interfaces host-<p>l0/w0 on
//	       the slot's veth rig, each with a linux-cp pair (taps <p>-l0, <p>-w0), mtu 1500; the slot's FRR (mgmtd, zebra,
//	       staticd, ripd, isisd) runs RIPv2 on both taps.
//	peers: two frrtest instances (p1 in ns-<p>-lan, p2 in ns-<p>-wan), RIPv2 on their veth, each redistributing 20
//	       blackhole statics (10.<N>.64–83.0/24 and 10.<N>.128–147.0/24), and IS-IS level-2 on the same veth.
//
// VRX_ISISRIP_FIB=root (default; D-119 M3 "a root-netns zebra is always one slot at a time", D-167): the VRX-side FRR runs
// in the ROOT network namespace with the slot pathspace, the pairs carry no netns, zebra's kernel routes reach VPP table 0
// through linux_nl (the only netns it hears on the shared VPP), the exclusive globals lock is held for the whole run inside
// the shared lab lock, preconditions fail closed (no LCP pair, lcp default netns unset, no FRR-protocol root route, system
// frr inactive). VRX_ISISRIP_FIB=netns keeps FRR in ns-<p>-frr and checks FRR's RIB only.
//
// Steps:
//  1. commit (pairs + routing.rip) → 40 RIP routes in FRR's RIB and as lcp-rt-dynamic in VPP table 0 (`show ip fib`);
//     Retrieve == desired; idempotent second apply; the running config holds `router rip`.
//  2. peer 1 withdraws → 20 within 200 s (acceptance) → announces again → 40.
//  3. IS-IS applies/removes through the production agent, with RIP intact. Read-only OSI-punt state and
//     actual dropped hellos prove the documented disabled-punt limitation; no global enable is called.
//  4. agent restart with both pairs deleted behind its back (tap IPv4 flushed first so linux-cp drops its table-0
//     (*,224.0.0.0/24) Accept — S-ospf-mfib-stale; restart repair is V27) → pairs, addresses and 40 routes back without any API call
//     (≤ 30 s), then a withdraw/announce cycle proves linux-nl hears the recreated pairs.
//  5. rollback of routing.rip → no `router rip` (rendered file + running config), 0 RIP routes in FRR and VPP, Retrieve
//     without routing.rip.
//
// Cleanup: peers down first, then everything of this owner is removed through the agent (routes withdrawn before the
// pairs go, D-101/V24), 0 lcp-rt-dynamic routes of 10.<N>/16 left in table 0, table-0 mfib 224.0.0.0/24 compared with the
// state before the run, agent stopped, FRR instances stopped by PID, rig down.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/binapi/interface_types"
	lcpapi "ngfw/agent/binapi/lcp"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/lcpmap"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/frrtest"
	"ngfw/agent/internal/renderers/frr/isis"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// EnvISISRIPTopology runs this test (test/topology/isis-rip/run.sh topology sets it).
const EnvISISRIPTopology = "VRX_ISISRIP_TOPOLOGY"

// EnvISISRIPFIB = "netns" keeps the VRX-side FRR in ns-<p>-frr (FRR RIB checks only); anything else is root mode.
const EnvISISRIPFIB = "VRX_ISISRIP_FIB"

// EnvPreflightBin is the vrx-vpp-preflight binary run.sh built under the heavy-step semaphore (D-224).
const EnvPreflightBin = "VRX_PREFLIGHT_BIN"

func init() {
	rootFRRBins["ripd"] = frrtest.RipdBin
	rootFRRBins["isisd"] = frrtest.IsisdBin
}

// isisRipFRRDefaults are the isis lines FRR 10.7.1 never shows (finding F1, TestISISHostLive).
var isisRipFRRDefaults = map[string]bool{" metric-style wide": true, " is-type level-1-2": true, " isis metric 10": true}

func isisRipStrip(conf string, files renderers.Files) (renderers.Files, []string) {
	out := renderers.Files{}
	var stripped []string
	for p, f := range files {
		if p == conf {
			var keep []string
			for _, l := range strings.Split(string(f.Content), "\n") {
				if isisRipFRRDefaults[l] {
					stripped = append(stripped, l)
					continue
				}
				keep = append(keep, l)
			}
			f.Content = []byte(strings.Join(keep, "\n"))
		}
		out[p] = f
	}
	return out, stripped
}

type isisRipEnv struct {
	*p12Env
	announced [2]map[string]bool
}

// peerDoc is a peer's FRR document: RIPv2 and IS-IS level-2 on its rig veth, 20 blackhole statics redistributed into RIP
// (or none when withdrawn).
func (e *isisRipEnv) peerDoc(n int, itf string, announce bool) *structpb.Struct {
	e.t.Helper()
	rip := map[string]any{"interfaces": map[string]any{itf: map[string]any{}}}
	is := map[string]any{"net": fmt.Sprintf("49.%04d.0000.0000.%04d.00", e.slot, 10+n), "level": "level-2",
		"interfaces": map[string]any{itf: map[string]any{}}}
	routing := map[string]any{"rip": rip, "isis": is}
	if announce {
		var statics []any
		for _, p := range e.peerPrefixes(n) {
			statics = append(statics, map[string]any{"prefix": p, "blackhole": true, "frr": true})
		}
		rip["redistribute"] = map[string]any{"static": map[string]any{}}
		routing["static"] = statics
	}
	s, err := structpb.NewStruct(map[string]any{"routing": routing})
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (e *isisRipEnv) peerPrefixes(n int) []string {
	base := 64
	if n == 2 {
		base = 128
	}
	out := make([]string, 0, 20)
	for k := 0; k < 20; k++ {
		out = append(out, fmt.Sprintf("10.%d.%d.0/24", e.slot, base+k))
	}
	return out
}

// applyPeer renders d on peer n's FRR, strips the FRR-default isis lines (F1, test scaffolding only) and applies it.
func (e *isisRipEnv) applyPeer(n int, d *structpb.Struct) {
	e.t.Helper()
	h := e.peerFRR[n-1]
	r := h.Renderer()
	files, err := r.Render(context.Background(), d)
	if err != nil {
		e.t.Fatal(err)
	}
	h.AssertScoped(e.t, files)
	files, _ = isisRipStrip(r.Paths().ConfFile(), files)
	if err := r.Validate(context.Background(), files); err != nil {
		e.t.Fatalf("peer %d validate: %v", n, err)
	}
	if err := r.Apply(context.Background(), files); err != nil {
		e.t.Fatalf("peer %d apply: %v", n, err)
	}
}

// doc is the configuration document the agent gets: both host-interfaces with a linux-cp pair, RIPv2 and/or IS-IS on both.
func (e *isisRipEnv) doc(withRIP, withISIS bool) *vrxv1.DesiredState {
	e.t.Helper()
	n, p := e.slot, e.prefix
	lcp := func(host string) map[string]any {
		if e.fib {
			return map[string]any{"hostIfName": host, "hostIfType": "tap"}
		}
		return map[string]any{"hostIfName": host, "hostIfType": "tap", "netns": e.frrNS}
	}
	// mtu 1500 = the peers' veth MTU (the LCP tap otherwise inherits VPP's 9000: IS-IS pads hellos to the MTU, F-ospf-host)
	d := map[string]any{"interfaces": map[string]any{
		"host-" + p + "l0": map[string]any{"enabled": true, "mtu": 1500, "ipv4": []any{fmt.Sprintf("10.%d.1.1/24", n)}, "lcp": lcp(p + "-l0")},
		"host-" + p + "w0": map[string]any{"enabled": true, "mtu": 1500, "ipv4": []any{fmt.Sprintf("10.%d.2.1/24", n)}, "lcp": lcp(p + "-w0")},
	}}
	routing := map[string]any{}
	both := func() map[string]any {
		return map[string]any{"host-" + p + "l0": map[string]any{}, "host-" + p + "w0": map[string]any{}}
	}
	if withRIP {
		routing["rip"] = map[string]any{"interfaces": both()}
	}
	if withISIS {
		// no circuit options: removing `isis network|circuit-type|passive|metric` lines fails on FRR 10.7.1 (finding F2)
		routing["isis"] = map[string]any{"net": fmt.Sprintf("49.%04d.0000.0000.0001.00", n), "level": "level-2", "interfaces": both()}
	}
	if len(routing) > 0 {
		d["routing"] = routing
	}
	raw, err := structpb.NewStruct(d)
	if err != nil {
		e.t.Fatal(err)
	}
	js, _ := protojson.Marshal(raw)
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal(js, ds); err != nil {
		e.t.Fatal(err)
	}
	return ds
}

// vrxDirect is a renderer on the VRX-side FRR with the linux-cp mapping of doc (what the agent's FRR stage builds).
func (e *isisRipEnv) vrxDirect(doc *vrxv1.DesiredState) *frr.Renderer {
	m := &lcpmap.Mapper{}
	m.Set(lcpmap.FromDesired(doc))
	if e.rootFRR != nil {
		return frr.New(e.rootFRR.Runner, frr.WithPaths(e.rootFRR.Paths), frr.WithInterfaceMapper(m.Map))
	}
	return frr.New(e.vrxFRR.Runner, frr.WithPaths(e.vrxFRR.Paths), frr.WithInterfaceMapper(m.Map))
}

func (e *isisRipEnv) waitRIP(what string, want int, within time.Duration) time.Duration {
	e.t.Helper()
	start := time.Now()
	var last string
	for {
		st := e.state()
		rib := int(st.GetRibCounts()["ipv4/default/rip"])
		vppN := -1
		if e.fib {
			vppN = e.vppFRRRoutes()
		}
		last = fmt.Sprintf("FRR RIB rip=%d, VPP lcp-rt-dynamic=%d", rib, vppN)
		if rib == want && (!e.fib || vppN == want) {
			d := time.Since(start)
			e.t.Logf("%s: %s after %v", what, last, d.Round(100*time.Millisecond))
			return d
		}
		if time.Since(start) > within {
			e.evidence("timeout: " + what)
			for n, peer := range e.peerFRR {
				for _, cmd := range []frr.ShowCommand{"show ip route static", "show ip rip status", "show running-config"} {
					out, err := peer.Renderer().Show(context.Background(), cmd)
					e.t.Logf("peer%d %s err=%v:\n%s", n+1, cmd, err, out)
				}
			}
			e.t.Logf("Linux multicast memberships: %s", e.tapCmd("maddr", "show"))
			e.t.Fatalf("%s: not reached within %v: %s", what, within, last)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (e *isisRipEnv) runningConfig() string {
	e.t.Helper()
	out, err := e.vrxRenderer().Show(context.Background(), frr.ShowRunningConfig)
	if err != nil {
		e.t.Fatalf("show running-config: %v", err)
	}
	return string(out)
}

func (e *isisRipEnv) vpp(args ...string) string {
	out, err := e.cmd("timeout", append([]string{"20", "vppctl"}, args...)...)
	if err != nil {
		return fmt.Sprintf("%s(vppctl %s: %v)", out, strings.Join(args, " "), err)
	}
	return out
}

func (e *isisRipEnv) evidence(when string) {
	e.t.Helper()
	for _, args := range [][]string{{"show", "ip", "mfib"}, {"show", "interface", "address"}, {"show", "interface"}} {
		e.t.Logf("[%s] vppctl %v:\n%s", when, args, e.vpp(args...))
	}
	e.t.Logf("[%s] vppctl show lcp:\n%s", when, e.vpp("show", "lcp"))
	e.t.Logf("[%s] ip -br addr show dev %s-l0 / %s-w0 (taps):\n%s%s", when, e.prefix, e.prefix,
		e.tapCmd("-br", "addr", "show", "dev", e.prefix+"-l0"), e.tapCmd("-br", "addr", "show", "dev", e.prefix+"-w0"))
	if out, err := e.vrxRenderer().Show(context.Background(), frr.ShowCommand("show ip rip status")); err == nil {
		e.t.Logf("[%s] vtysh -N %s -c \"show ip rip status\":\n%s", when, e.prefix, out)
	}
	if e.fib {
		kr, _ := e.cmd("ip", "-4", "route", "show", "proto", "rip")
		e.t.Logf("[%s] ip -4 route show proto rip (root netns): %d routes\n%s", when, countLines(kr), truncateLines(kr, 3))
		for _, pfx := range []string{fmt.Sprintf("10.%d.64.0/24", e.slot), fmt.Sprintf("10.%d.147.0/24", e.slot)} {
			e.t.Logf("[%s] vppctl show ip fib %s:\n%s", when, pfx, truncateLines(e.vpp("show", "ip", "fib", pfx), 14))
		}
	}
}

var osiDropRe = regexp.MustCompile(`(?m)^\s*(\d+)\s+osi-input\s+unknown osi protocol`)

// osiDrops is the osi-input "unknown osi protocol" error counter (VPP-global, read-only; no other slot sends OSI frames).
func (e *isisRipEnv) osiDrops() (uint64, string) {
	out := e.vpp("show", "errors")
	var n uint64
	var lines []string
	for _, m := range osiDropRe.FindAllStringSubmatch(out, -1) {
		v, _ := strconv.ParseUint(m[1], 10, 64)
		n += v
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "osi-input") || strings.Contains(l, "llc-input") {
			lines = append(lines, strings.TrimRight(l, " "))
		}
	}
	return n, strings.Join(lines, "\n")
}

var ifRxRe = regexp.MustCompile(`rx packets\s+(\d+)`)

func (e *isisRipEnv) rxPackets(itf string) uint64 {
	m := ifRxRe.FindStringSubmatch(e.vpp("show", "interface", itf))
	if m == nil {
		return 0
	}
	v, _ := strconv.ParseUint(m[1], 10, 64)
	return v
}

// vrxAdjacencies reads the isisNeighbors reader through the agent's RoutingState RPC and parses it.
func (e *isisRipEnv) vrxAdjacencies() ([]isis.Adjacency, string) {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := e.c.RoutingState(ctx, &vrxv1.RoutingStateRequest{Readers: []string{isis.NeighborsReader}})
	if err != nil {
		e.t.Fatalf("RoutingState: %v", err)
	}
	raw := st.GetReaders()[isis.NeighborsReader]
	as, err := isis.ParseNeighbors(json.RawMessage(raw))
	if err != nil {
		e.t.Fatalf("parse %s: %v", isis.NeighborsReader, err)
	}
	return as, raw
}

// mfib224 is table 0's (*,224.0.0.0/24) entry (linux-cp's plugin-low Accept paths) — compared before/after the run.
func (e *isisRipEnv) mfib224() string {
	return e.vpp("show", "ip", "mfib", "224.0.0.0/24")
}

// flushAndDeletePair simulates the loss of a pair: the tap's IPv4 goes first (linux-cp removes its (*,224.0.0.0/24)
// Accept only on the last RTM_DELADDR while the pair exists — V7), then the pair is deleted through the binary API.
func (e *isisRipEnv) flushAndDeletePair(vppName, tap string, idx uint32) {
	e.t.Helper()
	if e.fib {
		out, err := e.cmd("ip", "-4", "addr", "flush", "dev", tap)
		e.t.Logf("simulated loss: ip -4 addr flush dev %s (root netns): %v %s", tap, err, strings.TrimSpace(out))
		time.Sleep(1500 * time.Millisecond)
	}
	if _, err := lcpapi.NewServiceClient(e.raw).LcpItfPairAddDelV3(context.Background(), &lcpapi.LcpItfPairAddDelV3{IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
		e.t.Fatalf("simulated loss: delete pair of %s: %v", vppName, err)
	}
	e.t.Logf("simulated loss: lcp_itf_pair_add_del_v3 del %s (sw_if_index %d) with the agent stopped", vppName, idx)
}

func TestISISRIPTopologyOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	if os.Getenv(EnvISISRIPTopology) != "1" {
		t.Skipf("IS-IS/RIP topology test: set %s=1 (test/topology/isis-rip/run.sh topology) — it takes the slot's rig over and runs 3 FRR instances", EnvISISRIPTopology)
	}
	vpptest.LockLab(t)
	prefix := vpptest.Prefix(t)
	slot := vpptest.Slot(t)
	e := &isisRipEnv{p12Env: &p12Env{t: t, prefix: prefix, slot: slot, repo: repoRootP12(t), frrNS: "ns-" + prefix + "-frr", fib: os.Getenv(EnvISISRIPFIB) != "netns"}}
	if e.fib {
		f, err := os.OpenFile(globalsLockFile, os.O_RDWR|os.O_CREATE, 0o644) //nolint:gosec // shared lock file
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("waiting for the exclusive globals lock %s …", globalsLockFile)
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
			t.Fatal(err)
		}
		t.Logf("globals window open at %s", time.Now().Format(time.RFC3339))
		t.Cleanup(func() {
			t.Logf("globals window closed at %s", time.Now().Format(time.RFC3339))
			_ = f.Close()
		})
	}
	restarts0 := nRestartsP12(t)
	t.Logf("systemctl show vpp -p NRestarts (before) = %d", restarts0)
	t.Cleanup(func() {
		n := nRestartsP12(t)
		t.Logf("systemctl show vpp -p NRestarts (after) = %d", n)
		if n != restarts0 {
			t.Errorf("VPP restarted during the test: NRestarts %d → %d", restarts0, n)
		}
	})
	raw := vpp.Dial(vppSocket(), vpp.ConnOptions{})
	t.Cleanup(raw.Close)
	wctx, wcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer wcancel()
	if err := raw.WaitConnected(wctx); err != nil {
		t.Fatal(err)
	}
	e.raw = raw
	if _, err := lcpapi.NewServiceClient(raw).LcpDefaultNsGet(context.Background(), &lcpapi.LcpDefaultNsGet{}); err != nil {
		t.Skipf("linux_cp is not loaded on this VPP: %v", err)
	}
	if e.fib {
		(&ospfEnv{p12Env: e.p12Env}).checkRootPreconditions()
	}
	mfibBefore := e.mfib224()
	t.Logf("table-0 mfib before the run (vppctl show ip mfib 224.0.0.0/24):\n%s", mfibBefore)

	// V19 preflight (D-095) before any packet crosses the rig
	pf := os.Getenv(EnvPreflightBin)
	if pf == "" {
		t.Fatalf("%s is not set (run.sh builds vrx-vpp-preflight under tools/heavy.sh)", EnvPreflightBin)
	}
	pre, err := e.cmd(pf)
	t.Logf("vrx-vpp-preflight: %v\n%s", err, pre)
	if err != nil {
		t.Fatal("V19 preflight failed: no packet may cross the rig")
	}
	lab := filepath.Join(e.repo, "tools", "lab")
	t.Log(e.must(lab, "rig", "up", prefix))
	t.Cleanup(func() {
		out, err := e.cmd(lab, "rig", "down", prefix)
		t.Logf("rig down: %v\n%s", err, out)
	})
	e.peers(false)
	e.handRigToAgent()

	daemons := []string{"mgmtd", "zebra", "staticd", "ripd", "isisd"}
	if e.fib {
		e.rootFRR = startRootFRR(t, prefix, daemons)
	} else {
		e.vrxFRR = frrtest.Start(t, frrtest.Options{Prefix: prefix, Daemons: daemons})
	}
	e.peerFRR[0] = frrtest.Start(t, frrtest.Options{Prefix: prefix, Instance: "p1", NetNS: "ns-" + prefix + "-lan", Daemons: daemons})
	e.peerFRR[1] = frrtest.Start(t, frrtest.Options{Prefix: prefix, Instance: "p2", NetNS: "ns-" + prefix + "-wan", Daemons: daemons})
	lanIf, wanIf := prefix+"l1", prefix+"w1"
	e.applyPeer(1, e.peerDoc(1, lanIf, true))
	e.applyPeer(2, e.peerDoc(2, wanIf, true))
	t.Logf("peers: p1 %s (RIP + IS-IS L2 on %s, %s…%s), p2 %s (on %s, %s…%s); FRR-default isis lines stripped from the peers' render (F1, scaffolding)",
		e.peerFRR[0].NetNS, lanIf, e.peerPrefixes(1)[0], e.peerPrefixes(1)[19], e.peerFRR[1].NetNS, wanIf, e.peerPrefixes(2)[0], e.peerPrefixes(2)[19])

	// the agent (FRR = the slot instance; table range of the slot)
	t.Setenv(subsystems.EnvFRRPathspace, prefix)
	t.Setenv(subsystems.EnvTableBase, strconv.Itoa(1000*slot))
	e.cfg = hostConfig(t, prefix)
	ids, err := subsystems.ResolveIDScope()
	if err != nil {
		t.Fatal(err)
	}
	e.cfg.IDs = ids
	start := func() {
		a, err := Start(context.Background(), e.cfg, "isisrip-it", nil)
		if err != nil {
			t.Fatal(err)
		}
		e.a = a
		e.c = dialAgent(t, e.cfg.Socket)
		waitReady(t, e.c)
	}
	start()
	t.Cleanup(func() {
		e.peers(false)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cc, err := grpc.NewClient("unix://"+e.cfg.Socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			c := vrxv1.NewDataplaneClient(cc)
			resp, aerr := c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: prefix + "-isisrip-cleanup", Subsystems: []string{"interfaces", "routing"}})
			t.Logf("cleanup apply: %v %s %s", aerr, resp.GetStatus(), protojson.Format(resp.GetSummary()))
			if resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
				t.Errorf("cleanup apply not APPLIED: %v %s", aerr, protojson.Format(resp))
			}
			if e.fib {
				if r, err := c.ListRoutes(ctx, &vrxv1.ListRoutesRequest{Family: "ipv4", Prefix: fmt.Sprintf("10.%d.0.0/16", slot), Source: "lcp-rt-dynamic", Limit: 1}); err == nil {
					t.Logf("cleanup: %d lcp-rt-dynamic routes of 10.%d/16 left in VPP table 0", r.GetTotal(), slot)
					if r.GetTotal() != 0 {
						t.Errorf("cleanup left %d lcp-rt-dynamic routes of this slot in the shared table 0", r.GetTotal())
					}
				}
			}
			_ = cc.Close()
		}
		e.a.Stop()
		after := e.mfib224()
		t.Logf("table-0 mfib after the cleanup (vppctl show ip mfib 224.0.0.0/24):\n%s", after)
		if strings.Count(after, "Accept") > strings.Count(mfibBefore, "Accept") {
			t.Errorf("table-0 (*,224.0.0.0/24) holds more Accept paths than before the run (stale linux-cp Accept, V7)")
		}
		if kr, _ := e.cmd("ip", "-4", "route", "show", "proto", "rip"); strings.TrimSpace(kr) != "" {
			t.Errorf("rip routes left in the root kernel table:\n%s", kr)
		}
	})

	// ---- 1. commit: pairs + routing.rip; the peers come up after the preflight
	if e.fib {
		if n := e.vppFRRRoutes(); n != 0 {
			t.Fatalf("%d linux-nl routes of 10.%d/16 already in VPP table 0 before the test", n, slot)
		}
	} else {
		t.Logf("VPP FIB checks not requested (%s=netns): FRR's RIB is checked", EnvISISRIPFIB)
	}
	ripDoc := e.doc(true, false)
	e.apply("commit-rip", ripDoc)
	e.peers(true)
	e.waitRIP("40 RIP routes after commit", 40, 120*time.Second)
	e.evidence("after commit")

	got, err := e.c.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces", "routing"}})
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got.GetDesiredState().GetRouting().GetRip(), ripDoc.GetRouting().GetRip()) {
		t.Errorf("Retrieve routing.rip differs:\n got %s\nwant %s", protojson.Format(got.GetDesiredState().GetRouting().GetRip()), protojson.Format(ripDoc.GetRouting().GetRip()))
	} else {
		t.Logf("Retrieve routing.rip == desired: %s", protojson.Format(got.GetDesiredState().GetRouting().GetRip()))
	}
	for name, itf := range ripDoc.GetInterfaces() {
		if !proto.Equal(got.GetDesiredState().GetInterfaces()[name].GetLcp(), itf.GetLcp()) {
			t.Errorf("Retrieve %s.lcp = %v, want %v", name, got.GetDesiredState().GetInterfaces()[name].GetLcp(), itf.GetLcp())
		}
	}
	idem := e.apply("idempotent", ripDoc)
	if len(idem.GetResults()) != 0 {
		t.Errorf("second apply changed %v", idem.GetResults())
	}
	rc := e.runningConfig()
	for _, want := range []string{"router rip", " version 2", " network " + prefix + "-l0", " network " + prefix + "-w0"} {
		if !strings.Contains(rc, want+"\n") {
			t.Errorf("running config lacks %q:\n%s", want, rc)
		}
	}
	if strings.Contains(rc, "ens192") {
		t.Fatalf("running config mentions the management NIC:\n%s", rc)
	}

	if os.Getenv("VRX_ISISRIP_RESTART_DIAGNOSTIC") != "1" {
		// ---- 2. peer 1 withdraws → its 20 prefixes leave FRR and VPP (acceptance ≤ 200 s); announces again → 40
		e.applyPeer(1, e.peerDoc(1, lanIf, false))
		if d := e.waitRIP("peer 1 withdrew", 20, 200*time.Second); d > 200*time.Second {
			t.Errorf("withdrawal took %v", d)
		}
		e.applyPeer(1, e.peerDoc(1, lanIf, true))
		e.waitRIP("peer 1 announces again", 40, 120*time.Second)

		// ---- 3. IS-IS without the OSI punt
		osiGet, err := lcpapi.NewServiceClient(raw).LcpOsiProtoGet(context.Background(), &lcpapi.LcpOsiProtoGet{})
		if err != nil || osiGet == nil {
			t.Fatalf("lcp_osi_proto_get: %v", err)
		}
		t.Logf("lcp_osi_proto_get (read-only binary API): retval=%d count=%d osi_protos=%v", osiGet.Retval, osiGet.Count, osiGet.OsiProtos)
		if osiGet.Count != 0 {
			t.Logf("NOTE: an OSI protocol is already enabled on this VPP: %v", osiGet.OsiProtos)
		}
		t.Logf("vppctl show lcp osi-proto: %q", strings.TrimSpace(e.vpp("show", "lcp", "osi-proto")))
		bothDoc := e.doc(true, true)
		ctx := context.Background()
		actx, acancel := context.WithTimeout(ctx, 3*time.Minute)
		resp, aerr := e.c.Apply(actx, &vrxv1.ApplyRequest{TxnId: prefix + "-isisrip-isis", DesiredState: bothDoc, Subsystems: []string{"interfaces", "routing"}})
		acancel()
		t.Logf("commit routing.isis through the agent: %v status=%s message=%q summary=%s", aerr, resp.GetStatus(), resp.GetMessage(), protojson.Format(resp.GetSummary()))
		for _, r := range resp.GetResults() {
			t.Logf("  result: %s %s %s %s", r.GetKey(), r.GetOp(), r.GetCode(), truncateLines(r.GetMessage(), 30))
		}
		if v := resp.GetValidation(); v != nil {
			t.Logf("  validation: %s", protojson.Format(v))
		}
		if aerr != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
			t.Fatalf("IS-IS commit not applied: %v status=%s", aerr, resp.GetStatus())
		}
		e.waitRIP("RIP intact after applied IS-IS commit", 40, 60*time.Second)
		drops0, lines0 := e.osiDrops()
		rx0 := e.rxPackets("host-" + prefix + "l0")
		t0 := time.Now()
		t.Logf("osi-input before: %d\n%s", drops0, lines0)
		var vrxRaw string
		var peerView string
		deadline := time.Now().Add(40 * time.Second)
		for {
			time.Sleep(2 * time.Second)
			var as []isis.Adjacency
			as, vrxRaw = e.vrxAdjacencies()
			pas, perr := isis.ParseNeighbors(func() json.RawMessage {
				r, _ := e.peerFRR[0].Renderer().ShowJSON(ctx, isis.ShowNeighbors)
				return r
			}())
			peerView = fmt.Sprintf("%+v %v", pas, perr)
			init := false
			for _, a := range pas {
				if a.Interface == lanIf && a.State == "Initializing" {
					init = true
				}
			}
			up := false
			for _, a := range as {
				up = up || a.State == "Up"
			}
			if up {
				t.Errorf("an IS-IS adjacency came Up on the VRX side without the OSI punt: %+v", as)
				break
			}
			if init || time.Now().After(deadline) {
				t.Logf("after %v: peer 1 adjacency view %s; VRX adjacencies (agent RoutingState %s, parsed) %+v", time.Since(t0).Round(time.Second), peerView, isis.NeighborsReader, as)
				break
			}
		}
		t.Logf("VRX raw %s:\n%s", isis.NeighborsReader, vrxRaw)
		if out, err := e.peerFRR[0].Renderer().Show(ctx, frr.ShowCommand("show isis neighbor")); err == nil {
			t.Logf("peer 1 show isis neighbor:\n%s", out)
		}
		tapNS := []string{}
		if !e.fib {
			tapNS = []string{"netns", "exec", e.frrNS}
		}
		capture := func(ns []string, itf string) string {
			args := append(append([]string{}, ns...), "timeout", "10", "tcpdump", "-nn", "-e", "-Q", "in", "-c", "3", "-i", itf, "isis")
			var out string
			var err error
			if len(ns) == 0 {
				out, err = e.cmd(args[0], args[1:]...)
			} else {
				out, err = e.cmd("ip", args...)
			}
			return fmt.Sprintf("(exit: %v)\n%s", err, out)
		}
		t.Logf("tcpdump -Q in isis on the peer veth ns-%s-lan/%s (frames VRX → peer):\n%s", prefix, lanIf, capture([]string{"netns", "exec", "ns-" + prefix + "-lan"}, lanIf))
		t.Logf("tcpdump -Q in isis on the VRX tap %s (frames peer → VRX via VPP):\n%s", prefix+"-l0", capture(tapNS, prefix+"-l0"))
		drops1, lines1 := e.osiDrops()
		rx1 := e.rxPackets("host-" + prefix + "l0")
		t.Logf("osi-input after %v: %d (+%d drops 'unknown osi protocol'); host-%sl0 rx packets +%d\n%s", time.Since(t0).Round(time.Second), drops1, drops1-drops0, prefix, rx1-rx0, lines1)
		if drops1 <= drops0 {
			t.Errorf("osi-input 'unknown osi protocol' did not rise while two IS-IS peers sent hellos to the slot's interfaces")
		}
		// Remove IS-IS through the same production agent that applied it. The
		// first operation changes desired configuration; the next must be a no-op.
		e.apply("remove-isis", ripDoc)
		if strings.Contains(e.runningConfig(), "isis") {
			t.Errorf("isis lines left after agent removal:\n%s", e.runningConfig())
		}
		if again := e.apply("rip-after-isis", ripDoc); len(again.GetResults()) != 0 {
			t.Errorf("unchanged apply after agent IS-IS removal changed: %v", again.GetResults())
		}
		e.waitRIP("RIP after the IS-IS window", 40, 60*time.Second)

	}
	// ---- 4. agent restart with both pairs deleted behind its back → recreated, routes back, no API call
	e.a.Stop()
	names, err := dumpNames(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	e.flushAndDeletePair("host-"+prefix+"l0", prefix+"-l0", names["host-"+prefix+"l0"])
	e.flushAndDeletePair("host-"+prefix+"w0", prefix+"-w0", names["host-"+prefix+"w0"])
	t.Logf("table-0 mfib after the simulated loss:\n%s", e.mfib224())
	restartAt := time.Now()
	start()
	e.waitRIP("after agent restart + pair loss", 40, 90*time.Second)
	e.evidence("immediately after restart")
	if os.Getenv("VRX_ISISRIP_RESTART_DIAGNOSTIC") == "1" {
		t.Skip("diagnostic-only restart state capture; full acceptance phases deliberately not run")
	}
	recovered := time.Since(restartAt)
	t.Logf("recovered %v after the agent restart (target ≤ 30 s)", recovered.Round(100*time.Millisecond))
	if recovered > 30*time.Second {
		t.Errorf("recovery took %v, more than 30 s", recovered.Round(time.Second))
	}
	if e.fib {
		// a deleted pair flushes no route (lcp_router.c): only a withdrawal that reaches VPP proves linux-nl hears the
		// recreated pairs
		e.applyPeer(1, e.peerDoc(1, lanIf, false))
		e.waitRIP("peer 1 withdrew after the restart", 20, 200*time.Second)
		e.applyPeer(1, e.peerDoc(1, lanIf, true))
		e.waitRIP("peer 1 announces again after the restart", 40, 120*time.Second)
	}
	e.evidence("after restart")

	// ---- 5. rollback of routing.rip → no `router rip`, no RIP route (FRR, and VPP with the FIB proof)
	e.apply("rollback", e.doc(false, false))
	e.waitRIP("after rollback", 0, 200*time.Second)
	got, err = e.c.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"routing"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetDesiredState().GetRouting().GetRip() != nil {
		t.Errorf("Retrieve still holds routing.rip after the rollback: %s", protojson.Format(got.GetDesiredState().GetRouting()))
	} else {
		t.Log("Retrieve after rollback: no routing.rip")
	}
	rc = e.runningConfig()
	if strings.Contains(rc, "router rip") {
		t.Errorf("FRR still holds router rip after rollback:\n%s", rc)
	}
	conf := e.vrxRenderer().Paths().ConfFile()
	if b, err := os.ReadFile(conf); err != nil || strings.Contains(string(b), "router rip") {
		t.Errorf("rendered %s after rollback: %v\n%s", conf, err, b)
	} else {
		t.Logf("rendered %s after rollback (no router rip):\n%s", conf, b)
	}
	if !strings.Contains(rc, "interface "+prefix+"-l0") {
		t.Errorf("the pair's interface block (addresses) vanished with the RIP rollback:\n%s", rc)
	}
	e.evidence("after rollback")
}
