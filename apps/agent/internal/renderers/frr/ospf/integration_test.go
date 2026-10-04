package ospf_test

// Live test of the ospf section against FRR 10.7 (NGFW_INTEGRATION=1; row F-ospf-host, RV-A R1/R7 owed list): two
// slot-scoped FRR instances (frrtest: the NGFW side in ns-<prefix>-frr, a peer in ns-<prefix>-p1) joined by a veth pair
// inside the slot's namespaces — no VPP, no root namespace, nothing under /etc/frr. It proves that the rendered lines
// (every construct of testdata/full.golden that ospfd alone can hold) are accepted by FRR 10.7.1 in the canonical order
// (frr-reload DryRun empty after Apply), that vtysh's running-config matches the render, that the state readers decode
// the real `show ip ospf vrf all neighbor|interface json`, that the adjacency reaches Full, that 50 redistributed
// prefixes arrive in the RIB and leave within 10 s of the withdrawal, that the ospf-neighbors poller reports the
// adjacency loss, and that removing routing.ospf removes `router ospf` and every `ip ospf` line.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/frrtest"
	"ngfw/agent/internal/renderers/frr/ospf"
	"ngfw/agent/internal/vpp/vpptest"
)

func doc(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func ip(t *testing.T, h *frrtest.Harness, args ...string) {
	t.Helper()
	out, err := h.Runner.Run(context.Background(), renderers.Command{Path: frrtest.IPBin, Args: args, Timeout: 20 * time.Second})
	if err != nil {
		t.Fatalf("ip %s: %v: %s", strings.Join(args, " "), err, out.Stderr)
	}
}

func mustRender(t *testing.T, r *frr.Renderer, d *structpb.Struct) renderers.Files {
	t.Helper()
	f, err := r.Render(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// apply renders, validates and applies d on h's FRR and returns the rendered files.
func apply(t *testing.T, r *frr.Renderer, h *frrtest.Harness, d *structpb.Struct) renderers.Files {
	t.Helper()
	ctx := context.Background()
	files := mustRender(t, r, d)
	h.AssertScoped(t, files)
	if err := r.Validate(ctx, files); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := r.Apply(ctx, files); err != nil {
		t.Fatalf("apply: %v\nrendered:\n%s", err, files.Redacted()[r.Paths().ConfFile()].Content)
	}
	return files
}

func waitFor(t *testing.T, what string, d time.Duration, cond func() (bool, string)) string {
	t.Helper()
	deadline := time.Now().Add(d)
	start := time.Now()
	for {
		ok, state := cond()
		if ok {
			return fmt.Sprintf("%s after %v (%s)", what, time.Since(start).Round(100*time.Millisecond), state)
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: not reached within %v (last: %s)", what, d, state)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// ospfRoutes lists the OSPF routes of the default VRF (`show ip route ospf json`: prefix → paths).
func ospfRoutes(t *testing.T, r *frr.Renderer) []string {
	t.Helper()
	raw, err := r.ShowJSON(context.Background(), frr.ShowCommand("show ip route ospf json"))
	if err != nil {
		t.Fatalf("show ip route ospf json: %v", err)
	}
	var m map[string]json.RawMessage
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("decode routes: %v: %s", err, raw)
		}
	}
	out := make([]string, 0, len(m))
	for p := range m {
		out = append(out, p)
	}
	return out
}

func countPrefixed(routes []string, prefix string) int {
	n := 0
	for _, r := range routes {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

func TestOSPFLive(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix := vpptest.Prefix(t)
	slot := vpptest.Slot(t)
	ctx := context.Background()
	daemons := []string{"mgmtd", "zebra", "staticd", "ospfd"}

	// the NGFW side carries a dummy link (passive stub-area interface, redistributed as connected)
	dummy, dummyNet := prefix+"d0", fmt.Sprintf("10.%d.8.0/24", slot)
	ngfw := frrtest.Start(t, frrtest.Options{Prefix: prefix, Daemons: daemons,
		Links: []frrtest.Link{{Name: dummy, Kind: "dummy", CIDR: fmt.Sprintf("10.%d.8.1/24", slot)}}})
	peer := frrtest.Start(t, frrtest.Options{Prefix: prefix, Instance: "p1", Daemons: daemons})
	vIf, pIf := prefix+"v0", prefix+"v1"
	vAddr, pAddr := fmt.Sprintf("10.%d.9.1", slot), fmt.Sprintf("10.%d.9.2", slot)
	ip(t, ngfw, "link", "add", vIf, "netns", ngfw.NetNS, "type", "veth", "peer", "name", pIf, "netns", peer.NetNS)
	ip(t, ngfw, "-n", ngfw.NetNS, "addr", "add", vAddr+"/24", "dev", vIf)
	ip(t, ngfw, "-n", peer.NetNS, "addr", "add", pAddr+"/24", "dev", pIf)
	ip(t, ngfw, "-n", ngfw.NetNS, "link", "set", vIf, "up")
	ip(t, ngfw, "-n", peer.NetNS, "link", "set", pIf, "up")
	t.Logf("ngfw %s (pathspace %s), peer %s (pathspace %s), veth %s %s ↔ %s %s, dummy %s %s",
		ngfw.NetNS, ngfw.Paths.Namespace, peer.NetNS, peer.Paths.Namespace, vIf, vAddr, pIf, pAddr, dummy, dummyNet)

	rv := ngfw.Renderer()
	rp := peer.Renderer()

	// the peer redistributes 50 blackhole statics (10.<N>.64–113.0/24) into area 0
	const nPrefixes = 50
	var statics []any
	for i := 0; i < nPrefixes; i++ {
		statics = append(statics, map[string]any{"prefix": fmt.Sprintf("10.%d.%d.0/24", slot, 64+i), "blackhole": true, "frr": true})
	}
	peerDoc := func(announce bool) *structpb.Struct {
		o := map[string]any{"routerId": pAddr,
			"areas":      map[string]any{"0": map[string]any{}},
			"interfaces": map[string]any{pIf: map[string]any{"area": "0", "networkType": "point-to-point", "helloIntervalSec": 1, "deadIntervalSec": 4}}}
		d := map[string]any{"routing": map[string]any{"ospf": o}}
		if announce {
			o["redistribute"] = map[string]any{"static": map[string]any{}}
			d["routing"].(map[string]any)["static"] = statics
		}
		return doc(t, d)
	}
	// the NGFW side: every construct of testdata/full.golden that ospfd alone can hold (no bgp route map)
	ngfwDoc := doc(t, map[string]any{"routing": map[string]any{"ospf": map[string]any{
		"routerId": vAddr, "vrf": "default", "defaultInformationOriginate": "always",
		"areas": map[string]any{"0.0.0.0": map[string]any{}, "51": map[string]any{"type": "stub", "noSummary": true}, "0.0.0.7": map[string]any{"type": "nssa"}},
		"interfaces": map[string]any{
			vIf:   map[string]any{"area": "0", "cost": 10, "networkType": "point-to-point", "helloIntervalSec": 1, "deadIntervalSec": 4, "priority": 0},
			dummy: map[string]any{"area": "0.0.0.51", "passive": true},
		},
		"redistribute": map[string]any{"connected": map[string]any{"metric": 20}},
	}}})

	apply(t, rp, peer, peerDoc(true))
	files := apply(t, rv, ngfw, ngfwDoc)
	rendered := string(files[rv.Paths().ConfFile()].Content)
	t.Logf("ngfw rendered %s:\n%s", rv.Paths().ConfFile(), rendered)

	// ---- 1. golden accepted by FRR 10.7.1 in canonical order: frr-reload DryRun of the applied config is empty
	dry, err := rv.DryRun(ctx, mustRender(t, rv, ngfwDoc))
	if err != nil || dry != "" {
		t.Fatalf("DryRun of the applied config: %q %v (want no diff)", dry, err)
	}
	t.Log("frr-reload --test of the applied config: no diff (canonical form, FRR 10.7.1)")
	pdry, err := rp.DryRun(ctx, mustRender(t, rp, peerDoc(true)))
	if err != nil || pdry != "" {
		t.Fatalf("peer DryRun of the applied config: %q %v (want no diff)", pdry, err)
	}

	// ---- 2. vtysh running-config matches the render (every rendered ospf line is in the running config)
	rc, err := rv.Show(ctx, frr.ShowRunningConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ngfw show running-config:\n%s", rc)
	checked := 0
	for _, l := range strings.Split(rendered, "\n") {
		ours := false
		for _, p := range []string{"router ospf", "interface ", " ip ospf", " ospf ", " area ", " redistribute", " default-information"} {
			ours = ours || strings.HasPrefix(l, p)
		}
		if !ours {
			continue
		}
		checked++
		if !strings.Contains(string(rc), l+"\n") {
			t.Errorf("rendered line %q is not in the running config", l)
		}
	}
	t.Logf("%d rendered ospf/interface lines checked against the running config", checked)

	// ---- 3. adjacency Full, 50 prefixes + connected + default in the RIBs
	t.Log(waitFor(t, "adjacency Full on the NGFW side", 60*time.Second, func() (bool, string) {
		ns, err := ospf.Neighbors(ctx, rv.ShowJSON)
		if err != nil {
			return false, err.Error()
		}
		for _, n := range ns {
			if n.RouterID == pAddr && strings.HasPrefix(n.State, "Full") {
				return true, fmt.Sprintf("%+v", n)
			}
		}
		return false, fmt.Sprintf("%+v", ns)
	}))
	ns, err := ospf.Neighbors(ctx, rv.ShowJSON)
	if err != nil || len(ns) != 1 || ns[0].VRF != frr.DefaultVRF || ns[0].Interface != vIf || ns[0].Address != pAddr {
		t.Errorf("neighbour parser on real FRR 10.7 output: %+v %v (want vrf default, interface %s, address %s)", ns, err, vIf, pAddr)
	}
	rawNbr, _ := rv.ShowJSON(ctx, ospf.ShowNeighbors)
	t.Logf("%s:\n%s", ospf.ShowNeighbors, rawNbr)
	t.Log(waitFor(t, fmt.Sprintf("%d OSPF routes on the NGFW side", nPrefixes), 30*time.Second, func() (bool, string) {
		routes := ospfRoutes(t, rv)
		n := countPrefixed(routes, fmt.Sprintf("10.%d.", slot))
		return n == nPrefixes, fmt.Sprintf("%d routes of 10.%d/16", n, slot)
	}))
	t.Log(waitFor(t, "connected (metric 20) and default-information originate on the peer", 30*time.Second, func() (bool, string) {
		routes := ospfRoutes(t, rp)
		return countPrefixed(routes, dummyNet) == 1 && countPrefixed(routes, "0.0.0.0/0") == 1, strings.Join(routes, " ")
	}))
	rawIf, err := rv.ShowJSON(ctx, ospf.ShowInterfaces)
	if err != nil {
		t.Fatal(err)
	}
	var ifs map[string]json.RawMessage
	if err := json.Unmarshal(rawIf, &ifs); err != nil {
		t.Fatalf("decode %s: %v", ospf.ShowInterfaces, err)
	}
	t.Logf("%s (keys %v):\n%s", ospf.ShowInterfaces, keys(ifs), truncate(string(rawIf), 1500))

	// ---- 4. Retrieve carries both state readers with the real output; the ospf-neighbors poller sees the adjacency
	st, err := rv.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stJSON, _ := st.(*structpb.Struct).MarshalJSON()
	for _, key := range []string{ospf.NeighborsReader, ospf.InterfacesReader} {
		if !strings.Contains(string(stJSON), `"`+key+`"`) {
			t.Errorf("Retrieve has no %s reader output", key)
		}
	}
	if !strings.Contains(string(stJSON), pAddr) {
		t.Errorf("Retrieve's %s does not mention the peer %s", ospf.NeighborsReader, pAddr)
	}
	poller := rv.NewPoller()
	if _, err := poller.Step(ctx); err != nil {
		t.Fatalf("poller baseline: %v", err)
	}
	snap, err := ospf.PollNeighbors(ctx, rv.ShowJSON)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ospf-neighbors poller snapshot: %v", snap)
	if v, ok := snap[frr.DefaultVRF+"|"+pAddr+"|"+vIf]; !ok || !strings.HasPrefix(v, "Full") {
		t.Errorf("poller snapshot lacks %s|%s|%s=Full…: %v", frr.DefaultVRF, pAddr, vIf, snap)
	}

	// ---- 5. the peer withdraws → the 50 prefixes leave the NGFW RIB within 10 s
	apply(t, rp, peer, peerDoc(false))
	t.Log(waitFor(t, "0 redistributed prefixes after the peer withdrew", 10*time.Second, func() (bool, string) {
		n := countPrefixed(ospfRoutes(t, rv), fmt.Sprintf("10.%d.", slot))
		return n == 0, fmt.Sprintf("%d routes of 10.%d/16", n, slot)
	}))

	// ---- 6. the peer drops OSPF → an ospf-neighbors event with the old Full state
	apply(t, rp, peer, doc(t, map[string]any{}))
	var seen []string
	t.Log(waitFor(t, "ospf-neighbors event for the peer", 20*time.Second, func() (bool, string) {
		evs, _ := poller.Step(ctx)
		for _, e := range evs {
			seen = append(seen, e.String())
			if e.Poller == ospf.PollerNeighbors && strings.Contains(e.Key, "|"+pAddr+"|") && strings.HasPrefix(e.Old, "Full") {
				return true, e.String()
			}
		}
		return false, strings.Join(seen, "; ")
	}))

	// ---- 7. rollback: removing routing.ospf removes `router ospf` and every `ip ospf` line
	apply(t, rv, ngfw, doc(t, map[string]any{}))
	rc, err = rv.Show(ctx, frr.ShowRunningConfig)
	if err != nil {
		t.Fatalf("running config after removal: %v", err)
	}
	for _, gone := range []string{"router ospf", "ip ospf"} {
		if strings.Contains(string(rc), gone) {
			t.Errorf("%q still in the running config after removal:\n%s", gone, rc)
		}
	}
	if n := len(ospfRoutes(t, rv)); n != 0 {
		t.Errorf("%d OSPF routes left after removal", n)
	}
	t.Logf("after removal, show running-config:\n%s", rc)
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
