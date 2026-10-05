package agent

// P12 topology test on this host (NGFW_INTEGRATION=1, shared lab lock, slot prefix; docs/status/tasks/P12-questions.md Q1):
//
//	NGFW:   the in-process agent (owner = slot prefix, NGFW_FRR_PATHSPACE = slot) → VPP af_packet interfaces host-<p>l0/w0 on
//	       the slot's veth rig, each with a linux-cp pair whose tap lives in ns-<p>-frr, where the slot's FRR (frrtest,
//	       bgpd) runs; FRR puts the VPP addresses on the taps (lcp-addresses, S2) and speaks BGP through the punt path.
//	peers: two frrtest instances (p1 in ns-<p>-lan, p2 in ns-<p>-wan), eBGP, 100 prefixes each (10.<N>.64–163.x/25).
//
// Steps: commit → both sessions Established, 200 BGP routes in FRR → Retrieve == desired → route map denying half →
// 100 → withdraw on a peer → gone within 5 s → agent restart with the pairs deleted behind its back → pairs recreated,
// sessions back without anything else → link down on a VPP interface (observed) → rollback of the whole BGP config →
// sessions down, no BGP route → cleanup (peers down before the agent deletes its af_packet interfaces, D-101/V24).
//
// FIB proof (the VPP side of linux-nl; P12.md "P12-fib-proof"): linux_nl hears only pairs whose netns equals the lcp
// default netns at pair-add time, from the one socket it opens in that netns with the first pair of the whole VPP
// (lcp_nl.c, lcp_interface.c). On the shared VPP (default netns unset, D-071: never changed from a slot) FRR's routes in
// ns-<p>-frr cannot reach VPP, so the VPP checks run only with NGFW_P12_FIB=private: a VPP of this slot's own
// (LAB-vpp-per-slot, NGFW_VPP_API_SOCKET ≠ /run/vpp/api.sock) whose startup.conf has `linux-cp { default netns
// ns-<p>-frr }`. The test refuses that mode on anything else and never changes a VPP-global setting itself.
//
// NGFW_P12_FIB=root restores historical root-mode semantics only inside an owned
// private outer network namespace and disposable VPP. Shared host root execution
// is refused before any lab, VPP or FRR writes. The LCP default namespace remains unset.

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	afpapi "ngfw/agent/binapi/af_packet"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	lcpapi "ngfw/agent/binapi/lcp"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	lcpdesc "ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/frrtest"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// EnvP12FIB = "private" adds the VPP FIB checks on a private VPP whose lcp default netns is ns-<p>-frr; "root" adds
// them inside a private outer netns with the NGFW-side FRR in that netns (row P12-fib-proof); anything else leaves them out.
const EnvP12FIB = "NGFW_P12_FIB"

// EnvP12Topology runs the topology test (test/topology/bgp/run.sh sets it): it takes the slot's rig over (the agent
// recreates its VPP side) and runs three FRR instances, so it is not part of a plain package run.
const EnvP12Topology = "NGFW_P12_TOPOLOGY"

type p12Env struct {
	t       *testing.T
	prefix  string
	slot    int
	repo    string
	raw     vpp.Client
	c       ngfwv1.DataplaneClient
	a       *Agent
	cfg     Config
	fib     bool // linux-nl FIB checks enabled (NGFW_P12_FIB=private|root)
	frrNS   string
	ngfwFRR *frrtest.Harness
	rootFRR *rootFRR // NGFW_P12_FIB=root: the NGFW-side FRR in the root netns (the pairs' taps live there too)
	peerFRR [2]*frrtest.Harness
}

func init() { rootFRRBins["bgpd"] = frrtest.BgpdBin } // rootFRR for NGFW_P12_FIB=root

// ngfwRenderer is the NGFW-side FRR's renderer (root netns or ns-<p>-frr).
func (e *p12Env) ngfwRenderer() *frr.Renderer {
	if e.rootFRR != nil {
		return e.rootFRR.Renderer()
	}
	return e.ngfwFRR.Renderer()
}

func (e *p12Env) ngfwPIDs() map[string]int {
	if e.rootFRR != nil {
		return e.rootFRR.PIDs()
	}
	return e.ngfwFRR.PIDs()
}

// tapCmd runs `ip <args>` where the taps live: the root netns in root mode, ns-<p>-frr otherwise.
func (e *p12Env) tapCmd(args ...string) string {
	if e.rootFRR == nil {
		args = append([]string{"-n", e.frrNS}, args...)
	}
	out, _ := e.cmd("ip", args...)
	return out
}

func (e *p12Env) cmd(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec // G204: fixed test commands (ip, vppctl, tools/lab, go), no user input
	return string(out), err
}

func (e *p12Env) must(name string, args ...string) string {
	e.t.Helper()
	out, err := e.cmd(name, args...)
	if err != nil {
		e.t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}

func repoRootP12(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "tools", "lab")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root (tools/lab) not found")
		}
		dir = parent
	}
}

func nRestartsP12(t *testing.T) int {
	t.Helper()
	out, err := exec.Command("systemctl", "show", "vpp", "-p", "NRestarts").Output()
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "NRestarts=")))
	if err != nil {
		t.Fatalf("NRestarts: %q", out)
	}
	return n
}

// peers sets the rig's veths down/up (D-101/V24: an af_packet interface is deleted only with its veth down).
func (e *p12Env) peers(up bool) {
	e.t.Helper()
	p := e.prefix
	st := "down"
	if up {
		st = "up"
		e.must("ip", "link", "set", p+"l0", "up")
		e.must("ip", "link", "set", p+"w0", "up")
	}
	e.must("ip", "-n", "ns-"+p+"-lan", "link", "set", p+"l1", st)
	e.must("ip", "-n", "ns-"+p+"-wan", "link", "set", p+"w1", st)
	if !up {
		e.must("ip", "link", "set", p+"l0", "down")
		e.must("ip", "link", "set", p+"w0", "down")
	}
}

// handRigToAgent deletes the rig's VPP side (addresses, af_packet; veths down) so the agent creates both
// host-interfaces from the configuration (tagged, its own).
func (e *p12Env) handRigToAgent() {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	names, err := dumpNames(ctx, e.raw)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, nd := range []string{e.prefix + "l0", e.prefix + "w0"} {
		idx, ok := names["host-"+nd]
		if !ok {
			continue
		}
		if _, err := interfaces.NewServiceClient(e.raw).SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{SwIfIndex: interface_types.InterfaceIndex(idx), DelAll: true}); err != nil {
			e.t.Fatalf("del addresses of host-%s: %v", nd, err)
		}
		if _, err := afpapi.NewServiceClient(e.raw).AfPacketDelete(ctx, &afpapi.AfPacketDelete{HostIfName: nd}); err != nil {
			e.t.Fatalf("af_packet_delete %s: %v", nd, err)
		}
		e.t.Logf("rig VPP side handed to the agent: af_packet_delete %s (veth down)", nd)
	}
}

func dumpNames(ctx context.Context, c vpp.Client) (map[string]uint32, error) {
	stream, err := interfaces.NewServiceClient(c).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: ^interface_types.InterfaceIndex(0)})
	if err != nil {
		return nil, err
	}
	out := map[string]uint32{}
	for {
		d, err := stream.Recv()
		if err != nil {
			break
		}
		out[strings.TrimRight(d.InterfaceName, "\x00")] = uint32(d.SwIfIndex)
	}
	return out, nil
}

// peerDoc is a peer's FRR document: eBGP to the NGFW side, 100 /25 blackholes announced (third octet 64–163, the peer's
// half of each /24), or none when withdrawn.
func peerDoc(t *testing.T, slot, n int, ngfwAddr string, announce bool) *structpb.Struct {
	t.Helper()
	b := map[string]any{"asn": 65080 + n, "ebgpRequiresPolicy": false,
		"neighbors": map[string]any{ngfwAddr: map[string]any{"remoteAs": 65080, "keepaliveSec": 3, "holdTimeSec": 9,
			"afi": map[string]any{"ipv4Unicast": map[string]any{"enabled": true}}}}}
	routing := map[string]any{"bgp": b}
	if announce {
		var statics, nets []any
		for k := 0; k < 100; k++ {
			p := fmt.Sprintf("10.%d.%d.%d/25", slot, 64+k, (n-1)*128)
			statics = append(statics, map[string]any{"prefix": p, "blackhole": true, "frr": true})
			nets = append(nets, map[string]any{"prefix": p})
		}
		b["networks"] = nets
		routing["static"] = statics
	}
	s, err := structpb.NewStruct(map[string]any{"routing": routing})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func applyFRR(t *testing.T, h *frrtest.Harness, d *structpb.Struct) {
	t.Helper()
	r := h.Renderer()
	files, err := r.Render(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	h.AssertScoped(t, files)
	if err := r.Apply(context.Background(), files); err != nil {
		t.Fatalf("peer apply: %v", err)
	}
}

// ngfwDoc is the configuration document the agent gets.
func (e *p12Env) ngfwDoc(withBGP, denyHalf, lanUp bool) *ngfwv1.DesiredState {
	e.t.Helper()
	n, p := e.slot, e.prefix
	lcp := func(host string) map[string]any {
		if e.fib { // the default netns (linux-cp { default netns }) — the only netns linux-nl hears (M4)
			return map[string]any{"hostIfName": host, "hostIfType": "tap"}
		}
		return map[string]any{"hostIfName": host, "hostIfType": "tap", "netns": e.frrNS}
	}
	d := map[string]any{
		"interfaces": map[string]any{
			"host-" + p + "l0": map[string]any{"enabled": lanUp, "ipv4": []any{fmt.Sprintf("10.%d.1.1/24", n)}, "lcp": lcp(p + "-l0")},
			"host-" + p + "w0": map[string]any{"enabled": true, "ipv4": []any{fmt.Sprintf("10.%d.2.1/24", n)}, "lcp": lcp(p + "-w0")},
		},
	}
	if withBGP {
		afi := map[string]any{"enabled": true}
		if denyHalf {
			afi["routeMapIn"] = "rm-in"
		}
		d["routing"] = map[string]any{
			"policy": map[string]any{
				// third octets 64–113 = 50 of each peer's 100 prefixes
				"prefixLists": map[string]any{"pl-half": map[string]any{"family": "ipv4", "description": "first half", "rules": []any{
					map[string]any{"seq": 5, "action": "permit", "prefix": fmt.Sprintf("10.%d.64.0/19", n), "le": 25},
					map[string]any{"seq": 10, "action": "permit", "prefix": fmt.Sprintf("10.%d.96.0/20", n), "le": 25},
					map[string]any{"seq": 15, "action": "permit", "prefix": fmt.Sprintf("10.%d.112.0/23", n), "le": 25},
				}}},
				"routeMaps": map[string]any{"rm-in": map[string]any{"entries": []any{
					map[string]any{"seq": 10, "action": "deny", "match": map[string]any{"prefixList": "pl-half"}},
					map[string]any{"seq": 20, "action": "permit"},
				}}},
			},
			"bgp": map[string]any{
				"asn": 65080, "routerId": fmt.Sprintf("10.%d.1.1", n), "ebgpRequiresPolicy": false,
				"peerGroups": map[string]any{"peers": map[string]any{"keepaliveSec": 3, "holdTimeSec": 9,
					"afi": map[string]any{"ipv4Unicast": afi}}},
				"neighbors": map[string]any{
					fmt.Sprintf("10.%d.1.2", n): map[string]any{"remoteAs": 65081, "peerGroup": "peers", "description": "peer one", "updateSource": "host-" + p + "l0"},
					fmt.Sprintf("10.%d.2.2", n): map[string]any{"remoteAs": 65082, "peerGroup": "peers", "description": "peer two"},
				},
			},
		}
	}
	raw, err := structpb.NewStruct(d)
	if err != nil {
		e.t.Fatal(err)
	}
	js, _ := protojson.Marshal(raw)
	ds := &ngfwv1.DesiredState{}
	if err := protojson.Unmarshal(js, ds); err != nil {
		e.t.Fatal(err)
	}
	return ds
}

func (e *p12Env) apply(id string, ds *ngfwv1.DesiredState, subsystems ...string) *ngfwv1.ApplyResponse {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if len(subsystems) == 0 {
		subsystems = []string{"interfaces", "routing"}
	}
	resp, err := e.c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: e.prefix + "-p12-" + id, DesiredState: ds, Subsystems: subsystems})
	if err != nil || resp.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
		e.t.Fatalf("apply %s: %v %s", id, err, protojson.Format(resp))
	}
	e.t.Logf("apply %s: %s", id, protojson.Format(resp.GetSummary()))
	return resp
}

func (e *p12Env) state() *ngfwv1.RoutingStateResponse {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := e.c.RoutingState(ctx, &ngfwv1.RoutingStateRequest{})
	if err != nil {
		e.t.Fatalf("RoutingState: %v", err)
	}
	return st
}

// neighbors returns peer address → (state, prefixes received).
func neighborsOf(st *ngfwv1.RoutingStateResponse) map[string]*ngfwv1.BgpNeighborState {
	out := map[string]*ngfwv1.BgpNeighborState{}
	for _, in := range st.GetBgp() {
		for _, n := range in.GetNeighbors() {
			out[n.GetAddress()] = n
		}
	}
	return out
}

// vppFRRRoutes counts the linux-nl routes of FRR in VPP table 0 (ListRoutes source lcp-rt-dynamic, slot prefixes only).
func (e *p12Env) vppFRRRoutes() int {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := e.c.ListRoutes(ctx, &ngfwv1.ListRoutesRequest{Family: "ipv4", Prefix: fmt.Sprintf("10.%d.0.0/16", e.slot), Source: "lcp-rt-dynamic", Limit: 1})
	if err != nil {
		e.t.Fatalf("ListRoutes: %v", err)
	}
	return int(r.GetTotal())
}

// waitRoutes waits until FRR has want BGP routes (and, with the FIB proof, VPP has them too) and returns how long it took.
func (e *p12Env) waitRoutes(what string, want int, within time.Duration) time.Duration {
	e.t.Helper()
	start := time.Now()
	var last string
	for {
		st := e.state()
		rib := int(st.GetRibCounts()["ipv4/default/bgp"])
		vppN := -1
		if e.fib {
			vppN = e.vppFRRRoutes()
		}
		nb := neighborsOf(st)
		last = fmt.Sprintf("FRR RIB bgp=%d, VPP lcp-rt-dynamic=%d, neighbours=%v", rib, vppN, summarize(nb))
		if rib == want && (!e.fib || vppN == want) {
			d := time.Since(start)
			e.t.Logf("%s: %s after %v", what, last, d.Round(100*time.Millisecond))
			return d
		}
		if time.Since(start) > within {
			e.t.Fatalf("%s: not reached within %v: %s", what, within, last)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func summarize(nb map[string]*ngfwv1.BgpNeighborState) string {
	var parts []string
	for a, n := range nb {
		parts = append(parts, fmt.Sprintf("%s %s rx=%d", a, n.GetState(), n.GetPrefixesReceived()))
	}
	return strings.Join(parts, "; ")
}

func (e *p12Env) waitEstablished(within time.Duration) {
	e.t.Helper()
	start := time.Now()
	for {
		nb := neighborsOf(e.state())
		ok := len(nb) == 2
		for _, n := range nb {
			ok = ok && n.GetState() == "Established"
		}
		if ok {
			e.t.Logf("both sessions Established after %v: %s", time.Since(start).Round(100*time.Millisecond), summarize(nb))
			return
		}
		if time.Since(start) > within {
			e.t.Fatalf("sessions not Established within %v: %s", within, summarize(nb))
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// evidence logs the pairs, the taps in FRR's netns and FRR's own BGP summary.
func (e *p12Env) evidence(when string) {
	e.t.Helper()
	lcp, _ := e.cmd("vppctl", "show", "lcp")
	e.t.Logf("[%s] vppctl show lcp:\n%s", when, lcp)
	e.t.Logf("[%s] ip -br addr show dev %s-l0 / %s-w0 (taps):\n%s%s", when, e.prefix, e.prefix,
		e.tapCmd("-br", "addr", "show", "dev", e.prefix+"-l0"), e.tapCmd("-br", "addr", "show", "dev", e.prefix+"-w0"))
	if out, err := e.ngfwRenderer().Show(context.Background(), frr.ShowCommand("show bgp summary")); err == nil {
		e.t.Logf("[%s] vtysh -N %s -c \"show bgp summary\":\n%s", when, e.prefix, out)
	}
	if e.fib {
		fib, _ := e.cmd("vppctl", "show", "ip", "fib", "summary")
		e.t.Logf("[%s] vppctl show ip fib summary:\n%s", when, fib)
		for _, pfx := range []string{fmt.Sprintf("10.%d.64.0/25", e.slot), fmt.Sprintf("10.%d.163.128/25", e.slot)} {
			one, _ := e.cmd("vppctl", "show", "ip", "fib", pfx)
			e.t.Logf("[%s] vppctl show ip fib %s:\n%s", when, pfx, truncateLines(one, 14))
		}
		if e.rootFRR != nil {
			kr, _ := e.cmd("ip", "-4", "route", "show", "proto", "bgp")
			e.t.Logf("[%s] ip -4 route show proto bgp (root netns): %d routes\n%s", when, countLines(kr), truncateLines(kr, 3))
		}
	}
}

// checkPrivateFIB enforces the preconditions of the FIB checks (NGFW_P12_FIB=private): a VPP that is not the shared
// one, whose linux-cp default netns is FRR's netns.
func (e *p12Env) checkPrivateFIB() {
	e.t.Helper()
	sock := vppSocket()
	if sock == "/run/vpp/api.sock" {
		e.t.Fatalf("%s=private needs a VPP of this slot's own (NGFW_VPP_API_SOCKET), not the shared %s (D-071)", EnvP12FIB, sock)
	}
	cur, err := lcpdesc.NewDefaultNetns(e.raw).Current(context.Background())
	if err != nil {
		e.t.Fatal(err)
	}
	if cur != e.frrNS {
		e.t.Fatalf("%s=private: the lcp default netns is %q, want %q (startup.conf linux-cp { default netns %s })", EnvP12FIB, cur, e.frrNS, e.frrNS)
	}
	e.t.Logf("%s=private: VPP %s, lcp default netns %s — linux_nl hears FRR's netns", EnvP12FIB, sock, cur)
}

func countLines(s string) int {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	return len(strings.Split(strings.TrimSpace(s), "\n"))
}

// checkRootFIB fails closed before anything of NGFW_P12_FIB=root starts (the F-ospf-host preconditions).
func (e *p12Env) checkRootFIB() {
	e.t.Helper()
	lcp, _ := e.cmd("vppctl", "show", "lcp")
	if strings.Contains(lcp, "itf-pair") {
		e.t.Fatalf("%s=root: LCP pairs already exist on this VPP (linux_nl's socket is bound to their netns):\n%s", EnvP12FIB, lcp)
	}
	if !strings.Contains(lcp, "lcp default netns '<unset>'") {
		e.t.Fatalf("%s=root: the lcp default netns is not unset (never changed from a slot, H3):\n%s", EnvP12FIB, lcp)
	}
	kr, _ := e.cmd("ip", "-4", "route", "show")
	for _, l := range strings.Split(kr, "\n") {
		for _, p := range []string{"proto zebra", "proto bgp", "proto ospf", "proto isis", "proto rip", "proto 196", "proto 11 "} {
			if strings.Contains(l, p) {
				e.t.Fatalf("%s=root: an FRR-protocol route is in the root kernel table (zebra would sweep it): %s", EnvP12FIB, l)
			}
		}
	}
	if out, _ := e.cmd("systemctl", "is-active", "frr"); strings.TrimSpace(out) == "active" {
		e.t.Fatalf("%s=root: the system frr unit is active", EnvP12FIB)
	}
	e.t.Logf("%s=root preconditions: VPP %s, no LCP pair, default netns unset, no FRR-protocol kernel route, system frr inactive; show lcp:\n%s", EnvP12FIB, vppSocket(), lcp)
}

// requirePrivateP12Root independently verifies the runner's namespace and private
// socket mapping before any mutation; flags alone cannot authorize shared root.
func requirePrivateP12Root(t *testing.T) {
	t.Helper()
	host := os.Getenv("NGFW_P12_HOST_NETNS")
	current, err := os.Readlink("/proc/self/ns/net")
	initNS, initErr := os.Readlink("/proc/1/ns/net")
	if initErr != nil || current == initNS || err != nil || host == "" || current == host || os.Getenv("NGFW_DISPOSABLE_VPP") != "1" {
		t.Fatal("P12 root FIB requires the private outer namespace runner")
	}
	supplied := os.Getenv("NGFW_P12_PRIVATE_API")
	scratch := filepath.Join(repoRootP12(t), ".scratch")
	rel, err := filepath.Rel(scratch, supplied)
	parts := strings.Split(rel, string(os.PathSeparator))
	if err != nil || len(parts) != 2 || parts[1] != "api.sock" || !strings.HasPrefix(parts[0], "isolated-vpp-") {
		t.Fatal("P12 private API must be inside the owned isolated runtime")
	}
	runtimePID, err := strconv.Atoi(strings.TrimPrefix(parts[0], "isolated-vpp-"))
	if err != nil || runtimePID <= 1 {
		t.Fatal("P12 private API runtime identity invalid")
	}
	owned := filepath.Join(scratch, fmt.Sprintf("isolated-vpp-%d", runtimePID), "api.sock")
	if supplied != owned {
		t.Fatal("P12 private API runtime path is not canonical")
	}
	if owned == "" || owned == "/run/vpp/api.sock" {
		t.Fatal("P12 private API ownership receipt missing")
	}
	a, err := os.Stat(owned)
	if err != nil {
		t.Fatalf("P12 private API: %v", err)
	}
	b, err := os.Stat(vppSocket())
	if err != nil || !os.SameFile(a, b) || a.Mode()&os.ModeSocket == 0 {
		t.Fatal("P12 API is not the owned disposable VPP socket mapping")
	}
	pid := os.Getenv("NGFW_P12_PRIVATE_VPP_PID")
	start := os.Getenv("NGFW_P12_PRIVATE_VPP_START")
	pidNumber, err := strconv.Atoi(pid)
	if err != nil || pidNumber <= 1 || start == "" {
		t.Fatal("P12 owned VPP PID/starttime receipt missing")
	}
	proc := fmt.Sprintf("/proc/%d", pidNumber)
	procFS := os.DirFS(proc)
	ns, err := os.Readlink(filepath.Join(proc, "ns/net"))
	if err != nil || ns != current {
		t.Fatal("P12 VPP is outside the owned private network namespace")
	}
	stat, err := fs.ReadFile(procFS, "stat")
	end := strings.LastIndex(string(stat), ")")
	if err != nil || end < 0 {
		t.Fatal("P12 owned VPP stat unavailable")
	}
	fields := strings.Fields(string(stat)[end+1:])
	if len(fields) < 20 || fields[19] != start {
		t.Fatal("P12 owned VPP PID starttime changed")
	}
	exe, err := os.Readlink(filepath.Join(proc, "exe"))
	if err != nil || filepath.Base(exe) != "vpp" {
		t.Fatal("P12 owned listener is not VPP")
	}
	unix, err := fs.ReadFile(procFS, "net/unix")
	if err != nil {
		t.Fatal(err)
	}
	fds, err := os.ReadDir(filepath.Join(proc, "fd"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"api.sock", "cli.sock", "stats.sock"} {
		private := filepath.Join(filepath.Dir(owned), name)
		mapped := filepath.Join("/run/vpp", name)
		a, err := os.Stat(private)
		b, mappedErr := os.Stat(mapped)
		if err != nil || mappedErr != nil || !os.SameFile(a, b) {
			t.Fatalf("P12 %s is not the private bind mount", name)
		}
		inode := ""
		listeners := 0
		for _, line := range strings.Split(string(unix), "\n") {
			parts := strings.Fields(line)
			if len(parts) == 8 && parts[7] == mapped && parts[3] == "00010000" {
				inode = parts[6]
				listeners++
			}
		}
		owns := false
		for _, fd := range fds {
			target, _ := os.Readlink(filepath.Join(proc, "fd", fd.Name()))
			if inode != "" && target == "socket:["+inode+"]" {
				owns = true
			}
		}
		if listeners != 1 || !owns {
			t.Fatalf("P12 owned private VPP does not hold %s listener", name)
		}
	}
	t.Logf("P12 private root verified: host=%s current=%s owned API=%s", host, current, owned)
}

func TestP12TopologyOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	if os.Getenv(EnvP12Topology) != "1" {
		t.Skipf("P12 topology test: set %s=1 (test/topology/bgp/run.sh) — it takes the slot's rig over and runs 3 FRR instances", EnvP12Topology)
	}
	if os.Getenv(EnvP12FIB) == "root" {
		requirePrivateP12Root(t)
	}
	vpptest.LockLab(t)
	prefix := vpptest.Prefix(t)
	slot := vpptest.Slot(t)
	e := &p12Env{t: t, prefix: prefix, slot: slot, repo: repoRootP12(t), frrNS: "ns-" + prefix + "-frr", fib: os.Getenv(EnvP12FIB) == "private" || os.Getenv(EnvP12FIB) == "root"}
	rootMode := os.Getenv(EnvP12FIB) == "root"
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
	if rootMode {
		e.checkRootFIB()
	}

	// V19 preflight (D-095) before any packet crosses the rig
	pre, err := e.cmd("go", "-C", filepath.Join(e.repo, "apps", "agent"), "run", "./cmd/ngfw-vpp-preflight")
	t.Logf("ngfw-vpp-preflight: %v\n%s", err, pre)
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

	// FRR: the NGFW side in its own netns, the peers in the rig namespaces
	daemons := []string{"mgmtd", "zebra", "staticd", "bgpd"}
	if rootMode {
		e.rootFRR = startRootFRR(t, prefix, daemons)
	} else {
		e.ngfwFRR = frrtest.Start(t, frrtest.Options{Prefix: prefix, Daemons: daemons})
	}
	e.peerFRR[0] = frrtest.Start(t, frrtest.Options{Prefix: prefix, Instance: "p1", NetNS: "ns-" + prefix + "-lan", Daemons: daemons})
	e.peerFRR[1] = frrtest.Start(t, frrtest.Options{Prefix: prefix, Instance: "p2", NetNS: "ns-" + prefix + "-wan", Daemons: daemons})
	ngfwLan, ngfwWan := fmt.Sprintf("10.%d.1.1", slot), fmt.Sprintf("10.%d.2.1", slot)
	applyFRR(t, e.peerFRR[0], peerDoc(t, slot, 1, ngfwLan, true))
	applyFRR(t, e.peerFRR[1], peerDoc(t, slot, 2, ngfwWan, true))

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
		a, err := Start(context.Background(), e.cfg, "p12-it", nil)
		if err != nil {
			t.Fatal(err)
		}
		e.a = a
		e.c = dialAgent(t, e.cfg.Socket)
		waitReady(t, e.c)
	}
	start()
	t.Cleanup(func() {
		// peers down first: the agent deletes its af_packet interfaces (D-101/V24), then the pairs and FRR's config
		e.peers(false)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		// a client of its own: the dialAgent clients are closed by their cleanups (LIFO) before this one runs
		cc, err := grpc.NewClient("unix://"+e.cfg.Socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			c := ngfwv1.NewDataplaneClient(cc)
			resp, aerr := c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: prefix + "-p12-cleanup", Subsystems: []string{"interfaces", "routing"}})
			t.Logf("cleanup apply: %v %s", aerr, protojson.Format(resp.GetSummary()))
			if aerr != nil || resp.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
				t.Errorf("cleanup apply not APPLIED: %v %s", aerr, resp.GetStatus())
			}
			if e.fib {
				if r, err := c.ListRoutes(ctx, &ngfwv1.ListRoutesRequest{Family: "ipv4", Prefix: fmt.Sprintf("10.%d.0.0/16", slot), Source: "lcp-rt-dynamic", Limit: 1}); err == nil {
					t.Logf("cleanup: %d lcp-rt-dynamic routes of 10.%d/16 left in VPP table 0", r.GetTotal(), slot)
					if r.GetTotal() != 0 {
						t.Errorf("cleanup left %d lcp-rt-dynamic routes of this slot in table 0", r.GetTotal())
					}
				}
			}
			if rootMode {
				if kr, _ := e.cmd("ip", "-4", "route", "show", "proto", "bgp"); countLines(kr) != 0 {
					t.Errorf("cleanup left bgp routes in the root kernel table:\n%s", kr)
				}
			}
			_ = cc.Close()
		}
		e.a.Stop()
	})

	// ---- 1. commit: pairs + FRR config; the peers come up after the preflight
	if e.fib {
		if !rootMode {
			e.checkPrivateFIB()
		}
		if n := e.vppFRRRoutes(); n != 0 {
			t.Fatalf("%d linux-nl routes of 10.%d/16 already in VPP table 0 before the test", n, slot)
		}
	} else {
		t.Logf("VPP FIB checks not requested (%s=root inside the private outer namespace, or =private on a VPP of the slot's own: row P12-fib-proof): FRR's RIB is checked", EnvP12FIB)
	}
	full := e.ngfwDoc(true, false, true)
	e.apply("commit", full)
	e.peers(true)
	e.waitEstablished(90 * time.Second)
	e.waitRoutes("200 routes after commit", 200, 60*time.Second)
	e.evidence("after commit")

	// Retrieve == desired for P12's leaves (routing.bgp, routing.policy, interfaces.<n>.lcp)
	got, err := e.c.Retrieve(context.Background(), &ngfwv1.RetrieveRequest{Subsystems: []string{"interfaces", "routing"}})
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got.GetDesiredState().GetRouting().GetBgp(), full.GetRouting().GetBgp()) ||
		!proto.Equal(got.GetDesiredState().GetRouting().GetPolicy(), full.GetRouting().GetPolicy()) {
		t.Errorf("Retrieve routing differs:\n got %s\nwant %s", protojson.Format(got.GetDesiredState().GetRouting()), protojson.Format(full.GetRouting()))
	}
	for name, itf := range full.GetInterfaces() {
		if !proto.Equal(got.GetDesiredState().GetInterfaces()[name].GetLcp(), itf.GetLcp()) {
			t.Errorf("Retrieve %s.lcp = %v, want %v", name, got.GetDesiredState().GetInterfaces()[name].GetLcp(), itf.GetLcp())
		}
	}
	idem := e.apply("idempotent", full)
	if len(idem.GetResults()) != 0 {
		t.Errorf("second apply changed %v", idem.GetResults())
	}

	// ---- 2. route map denying half → 100 remain (frr-reload.py, no daemon restart)
	pids := e.ngfwPIDs()
	e.apply("deny-half", e.ngfwDoc(true, true, true))
	e.waitRoutes("route map denies half", 100, 30*time.Second)
	if after := e.ngfwPIDs(); fmt.Sprint(after) != fmt.Sprint(pids) {
		t.Errorf("FRR daemons restarted on a config edit: %v → %v", pids, after)
	}

	// ---- 3. withdraw on peer 1 → its 50 accepted prefixes are gone within 5 s
	applyFRR(t, e.peerFRR[0], peerDoc(t, slot, 1, ngfwLan, false))
	if d := e.waitRoutes("peer 1 withdrew", 50, 5*time.Second); d > 5*time.Second {
		t.Errorf("withdrawal took %v", d)
	}
	applyFRR(t, e.peerFRR[0], peerDoc(t, slot, 1, ngfwLan, true))
	e.waitRoutes("peer 1 announces again", 100, 30*time.Second)

	// ---- 4. agent restart with the pairs deleted behind its back → recreated, sessions back, no API involved
	e.a.Stop()
	names, err := dumpNames(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"host-" + prefix + "l0", "host-" + prefix + "w0"} {
		if _, err := lcpapi.NewServiceClient(raw).LcpItfPairAddDelV3(context.Background(), &lcpapi.LcpItfPairAddDelV3{IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(names[n])}); err != nil {
			t.Fatalf("simulated loss: delete pair of %s: %v", n, err)
		}
		t.Logf("simulated loss: lcp_itf_pair_add_del_v3 del %s (sw_if_index %d) with the agent stopped", n, names[n])
	}
	restartAt := time.Now()
	start()
	e.waitEstablished(120 * time.Second)
	e.waitRoutes("after agent restart + pair loss", 100, 60*time.Second)
	t.Logf("recovered %v after the agent restart", time.Since(restartAt).Round(100*time.Millisecond))
	if e.fib {
		// a deleted pair flushes no route (lcp_router.c), so the count alone cannot tell a live linux-nl from stale
		// entries: a withdrawal must still reach VPP after the pairs were recreated
		applyFRR(t, e.peerFRR[0], peerDoc(t, slot, 1, ngfwLan, false))
		e.waitRoutes("peer 1 withdrew after the restart", 50, 5*time.Second)
		applyFRR(t, e.peerFRR[0], peerDoc(t, slot, 1, ngfwLan, true))
		e.waitRoutes("peer 1 announces again after the restart", 100, 30*time.Second)
	}
	e.evidence("after restart")

	// ---- 5. link down on a VPP interface: what reaches the Linux pair, when BGP notices
	e.apply("lan-down", e.ngfwDoc(true, true, false))
	down := time.Now()
	var tapState string
	for time.Since(down) < 15*time.Second {
		out := e.tapCmd("-o", "link", "show", prefix+"-l0")
		nb := neighborsOf(e.state())[fmt.Sprintf("10.%d.1.2", slot)]
		if tapState == "" && !strings.Contains(out, "LOWER_UP") {
			tapState = fmt.Sprintf("carrier down after %v", time.Since(down).Round(100*time.Millisecond))
		}
		if nb.GetState() != "Established" {
			t.Logf("link down on host-%sl0: BGP noticed after %v (state %s); tap: %s", prefix, time.Since(down).Round(100*time.Millisecond), nb.GetState(), firstNonEmpty(tapState, "carrier stays up (lcp-sync off: VPP admin state is not copied to Linux, P12-questions Q13)"))
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	e.apply("lan-up", e.ngfwDoc(true, true, true))
	e.waitEstablished(90 * time.Second)

	// ---- 6. rollback of the whole BGP config → sessions torn down, no BGP route (FRR, and VPP with the FIB proof)
	e.apply("rollback", e.ngfwDoc(false, false, true))
	e.waitRoutes("after rollback", 0, 30*time.Second)
	if st := e.state(); len(st.GetBgp()) != 0 {
		t.Errorf("BGP instance left after rollback: %v", st.GetBgp())
	}
	// the pairs stay, and so do the VPP addresses on their Linux side (FRR keeps rendering them without BGP)
	if addrs := e.tapCmd("-br", "addr", "show", prefix+"-l0"); !strings.Contains(addrs, fmt.Sprintf("10.%d.1.1/24", slot)) {
		t.Errorf("tap %s-l0 lost its address with the BGP rollback: %s", prefix, addrs)
	}
	running, _ := e.ngfwRenderer().Show(context.Background(), frr.ShowRunningConfig)
	if strings.Contains(string(running), "router bgp") || strings.Contains(string(running), "route-map") {
		t.Errorf("FRR still runs BGP/policy after rollback:\n%s", running)
	}
	e.evidence("after rollback")
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
