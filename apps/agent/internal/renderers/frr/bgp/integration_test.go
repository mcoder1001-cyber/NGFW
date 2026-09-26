package bgp_test

// Live test of the bgp + policy sections against FRR 10.7 (VRX_INTEGRATION=1): two slot-scoped FRR instances (frrtest:
// the VRX side in ns-<prefix>-frr, a peer in ns-<prefix>-p1) joined by a veth pair inside the slot's namespaces — no VPP,
// no root namespace, nothing under /etc/frr. It proves the canonical forms (every Apply runs the framework's convergence
// check), the session with an MD5 password from a fixture resolver, route-map filtering, withdrawal, the neighbour
// poller, the summary parser and the redaction of the password everywhere the renderer answers.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/bgp"
	"ngfw/agent/internal/renderers/frr/frrtest"
	_ "ngfw/agent/internal/renderers/frr/policy" // the policy section (init)
	"ngfw/agent/internal/vpp/vpptest"
)

const testPassword = "VRX_TEST_PSK_P12_1" //nolint:gosec // G101: the test fixture literal (00-CONTEXT: VRX_TEST_PSK_<id>)

func fixtureResolver() frr.SecretResolver {
	return frr.SecretResolverFunc(func(_ context.Context, ref string) (string, error) {
		if ref == "password/p12-peer" {
			return testPassword, nil
		}
		return "", errors.New("unknown fixture secret")
	})
}

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

func apply(t *testing.T, r *frr.Renderer, h *frrtest.Harness, d *structpb.Struct) {
	t.Helper()
	ctx := context.Background()
	files, err := r.Render(ctx, d)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	h.AssertScoped(t, files)
	if err := r.Validate(ctx, files); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := r.Apply(ctx, files); err != nil {
		t.Fatalf("apply: %v\nrendered:\n%s", err, files.Redacted()[r.Paths().ConfFile()].Content)
	}
}

// neighbor returns the VRX side's view of peer.
func neighbor(t *testing.T, r *frr.Renderer, peer string) (bgp.Neighbor, bool) {
	t.Helper()
	insts, err := bgp.Summary(context.Background(), r.ShowJSON)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	for _, in := range insts {
		for _, n := range in.Neighbors {
			if n.Address == peer {
				return n, true
			}
		}
	}
	return bgp.Neighbor{}, false
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

func TestBGPLive(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix := vpptest.Prefix(t)
	slot := vpptest.Slot(t)
	ctx := context.Background()
	daemons := []string{"mgmtd", "zebra", "staticd", "bgpd"}

	vrx := frrtest.Start(t, frrtest.Options{Prefix: prefix, Daemons: daemons})
	peer := frrtest.Start(t, frrtest.Options{Prefix: prefix, Instance: "p1", Daemons: daemons})
	vIf, pIf := prefix+"v0", prefix+"v1"
	vAddr, pAddr := fmt.Sprintf("10.%d.9.1", slot), fmt.Sprintf("10.%d.9.2", slot)
	ip(t, vrx, "link", "add", vIf, "netns", vrx.NetNS, "type", "veth", "peer", "name", pIf, "netns", peer.NetNS)
	ip(t, vrx, "-n", vrx.NetNS, "addr", "add", vAddr+"/24", "dev", vIf)
	ip(t, vrx, "-n", peer.NetNS, "addr", "add", pAddr+"/24", "dev", pIf)
	ip(t, vrx, "-n", vrx.NetNS, "link", "set", vIf, "up")
	ip(t, vrx, "-n", peer.NetNS, "link", "set", pIf, "up")
	t.Logf("vrx %s (pathspace %s), peer %s (pathspace %s), veth %s %s ↔ %s %s", vrx.NetNS, vrx.Paths.Namespace, peer.NetNS, peer.Paths.Namespace, vIf, vAddr, pIf, pAddr)

	rv := vrx.Renderer(frr.WithSecretResolver(fixtureResolver()))
	rp := peer.Renderer(frr.WithSecretResolver(fixtureResolver()))

	// the peer announces 100 /25s: 50 in 10.<N>.64.0/18 (denied later) and 50 in 10.<N>.128.0/18
	var statics, nets []any
	for i := 0; i < 100; i++ {
		third := 64 + i/2
		if i >= 50 {
			third = 128 + (i-50)/2
		}
		p := fmt.Sprintf("10.%d.%d.%d/25", slot, third, (i%2)*128)
		statics = append(statics, map[string]any{"prefix": p, "blackhole": true, "frr": true})
		nets = append(nets, map[string]any{"prefix": p})
	}
	peerDoc := func(announce bool) *structpb.Struct {
		b := map[string]any{"asn": 65081, "routerId": pAddr, "ebgpRequiresPolicy": false,
			"neighbors": map[string]any{vAddr: map[string]any{"remoteAs": 65080, "passwordRef": "password/p12-peer",
				"afi": map[string]any{"ipv4Unicast": map[string]any{"enabled": true}}}}}
		d := map[string]any{"routing": map[string]any{"bgp": b}}
		if announce {
			b["networks"] = nets
			d["routing"].(map[string]any)["static"] = statics
		}
		return doc(t, d)
	}
	vrxDoc := func(denyLow bool) *structpb.Struct {
		inEntries := []any{map[string]any{"seq": 20, "action": "permit", "set": map[string]any{"localPref": 150, "community": []any{"65080:20"}, "communityAdditive": true}}}
		if denyLow {
			inEntries = append([]any{map[string]any{"seq": 10, "action": "deny", "description": "drop the low half", "match": map[string]any{"prefixList": "pl-low"}}}, inEntries...)
		}
		return doc(t, map[string]any{"routing": map[string]any{
			"policy": map[string]any{
				"prefixLists": map[string]any{
					"pl-low":  map[string]any{"description": "peer low half", "family": "ipv4", "rules": []any{map[string]any{"seq": 5, "action": "permit", "prefix": fmt.Sprintf("10.%d.64.0/18", slot), "le": 25}}},
					"pl-v6":   map[string]any{"family": "ipv6", "rules": []any{map[string]any{"seq": 5, "action": "deny", "prefix": "2001:db8::/32", "ge": 48, "le": 64}, map[string]any{"seq": 10, "action": "permit", "prefix": "::/0", "le": 128}}},
					"pl-none": map[string]any{"family": "ipv4", "rules": []any{map[string]any{"seq": 5, "action": "deny", "prefix": "0.0.0.0/0", "le": 32}}},
				},
				"routeMaps": map[string]any{
					"rm-in": map[string]any{"entries": inEntries},
					"rm-out": map[string]any{"entries": []any{
						map[string]any{"seq": 10, "action": "deny", "match": map[string]any{"prefixList": "pl-none"}},
					}},
					"rm-all": map[string]any{"entries": []any{
						map[string]any{"seq": 5, "action": "permit", "match": map[string]any{"community": "65000:100", "asPath": "^65081_", "metric": 7, "tag": 9, "nextHopPrefixList": "pl-low"},
							"set": map[string]any{"asPathPrepend": []any{65080, 65080}, "med": 20, "weight": 100, "tag": 5, "nextHop": vAddr}},
						map[string]any{"seq": 6, "action": "permit", "match": map[string]any{"prefixList": "pl-v6"}, "set": map[string]any{"nextHop": "2001:db8::1"}},
					}},
				},
			},
			"bgp": map[string]any{"asn": 65080, "routerId": vAddr, "gracefulRestart": true,
				"peerGroups": map[string]any{"pg": map[string]any{"remoteAs": 65081, "description": "p12 peers", "keepaliveSec": 3,
					"afi": map[string]any{"ipv4Unicast": map[string]any{"enabled": true, "routeMapIn": "rm-in", "routeMapOut": "rm-out", "softReconfig": true}}}},
				"neighbors": map[string]any{
					pAddr: map[string]any{"peerGroup": "pg", "passwordRef": "password/p12-peer", "updateSource": vAddr, "description": "peer one"},
					fmt.Sprintf("10.%d.9.77", slot): map[string]any{"remoteAs": 65099, "shutdown": true, "ebgpMultihop": 255, "holdTimeSec": 9,
						"afi": map[string]any{"ipv4Unicast": map[string]any{"enabled": true, "prefixListIn": "pl-low", "prefixListOut": "pl-none", "maximumPrefixes": 1000, "nextHopSelf": true, "defaultOriginate": true},
							"ipv6Unicast": map[string]any{"enabled": true, "prefixListIn": "pl-v6", "prefixListOut": "pl-v6"}}},
				},
				"networks":     []any{map[string]any{"prefix": fmt.Sprintf("10.%d.250.0/24", slot), "routeMap": "rm-all"}, map[string]any{"prefix": "2001:db8:8::/48"}},
				"redistribute": map[string]any{"connected": map[string]any{"metric": 10}, "static": map[string]any{"routeMap": "rm-all"}},
			},
		}})
	}

	apply(t, rp, peer, peerDoc(true))
	apply(t, rv, vrx, vrxDoc(false))
	t.Log(waitFor(t, "session Established with 100 prefixes", 60*time.Second, func() (bool, string) {
		n, ok := neighbor(t, rv, pAddr)
		return ok && n.State == "Established" && n.PrefixesReceived == 100, fmt.Sprintf("%+v", n)
	}))
	rc, err := rv.Show(ctx, frr.ShowRunningConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("vrx show running-config (redacted):\n%s", rc)
	if strings.Contains(string(rc), testPassword) {
		t.Fatal("running-config through the renderer shows the password")
	}
	if !strings.Contains(string(rc), "neighbor "+pAddr+" password "+frr.Redacted) {
		t.Errorf("the password line is not in the running config (redacted)")
	}
	st, err := rv.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := st.(*structpb.Struct).MarshalJSON(); strings.Contains(string(b), testPassword) {
		t.Fatal("Retrieve holds the password")
	} else if !strings.Contains(string(b), `"bgpSummary"`) {
		t.Errorf("Retrieve has no bgpSummary reader output")
	}
	dry, err := rv.DryRun(ctx, mustRender(t, rv, vrxDoc(false)))
	if err != nil || dry != "" {
		t.Fatalf("DryRun of the applied config: %q %v (want no diff)", dry, err)
	}

	// route map denies the low half → 50 remain (FRR re-runs the inbound policy on the soft-reconfig copy)
	start := time.Now()
	apply(t, rv, vrx, vrxDoc(true))
	t.Log(waitFor(t, "50 prefixes after the deny entry", 30*time.Second, func() (bool, string) {
		n, _ := neighbor(t, rv, pAddr)
		return n.PrefixesReceived == 50, fmt.Sprintf("pfxRcd=%d", n.PrefixesReceived)
	}), " (apply+converge ", time.Since(start).Round(100*time.Millisecond), ")")

	// withdrawal on the peer → 0 within 5 s
	poller := rv.NewPoller()
	if _, err := poller.Step(ctx); err != nil {
		t.Fatalf("poller baseline: %v", err)
	}
	apply(t, rp, peer, peerDoc(false))
	t.Log(waitFor(t, "0 prefixes after the peer withdrew", 5*time.Second, func() (bool, string) {
		n, _ := neighbor(t, rv, pAddr)
		return n.PrefixesReceived == 0, fmt.Sprintf("pfxRcd=%d", n.PrefixesReceived)
	}))

	// neighbour state change is an event of the bgp-neighbors poller
	apply(t, rp, peer, doc(t, map[string]any{}))
	var seen []string
	t.Log(waitFor(t, "bgp-neighbors event for the peer", 20*time.Second, func() (bool, string) {
		evs, _ := poller.Step(ctx)
		for _, e := range evs {
			seen = append(seen, e.String())
			if e.Poller == bgp.PollerNeighbors && strings.HasSuffix(e.Key, "|"+pAddr) && e.Old == "Established" {
				return true, e.String()
			}
		}
		return false, strings.Join(seen, "; ")
	}))

	// removing BGP from the document removes `router bgp` and the filters
	apply(t, rv, vrx, doc(t, map[string]any{}))
	rc, _ = rv.Show(ctx, frr.ShowRunningConfig)
	for _, gone := range []string{"router bgp", "prefix-list", "route-map", "community-list", "as-path"} {
		if strings.Contains(string(rc), gone) {
			t.Errorf("%q still in the running config after removal:\n%s", gone, rc)
		}
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
