package rip_test

// Live test of the rip section against FRR 10.7.1 (NGFW_INTEGRATION=1; row F-isis-rip-host, RV-A R7/R1 owed list: "frrtest
// with ripd: golden accepted, DryRun empty").
//
// This file lives in test/topology/isis-rip/agent-overlay/ (the row's file fence) and is compiled into
// apps/agent/internal/renderers/frr/rip as rip_host_integration_test.go by test/topology/isis-rip/run.sh through
// `go test -overlay` — nothing under apps/ is written.
//
// Two slot-scoped FRR instances (frrtest: NGFW in ns-<prefix>-frr, a peer in ns-<prefix>-p1) joined by a veth pair — no
// VPP, no root namespace, nothing under /etc/frr:
//
//  1. golden accepted: the golden's own input (fullDoc) rendered with slot interface names is testdata/full.golden with
//     "w8-" → "<prefix>-", FRR 10.7.1 takes it (vtysh -C + frr-reload, convergence check), frr-reload DryRun of the applied
//     file is empty and every rendered rip line is in vtysh's running-config;
//  2. live RIPv2: NGFW (default-metric 2, the veth + a passive dummy, connected redistributed) ↔ peer (20 blackhole
//     statics redistributed): the 20 prefixes in the NGFW RIB as rip routes, NGFW's connected prefix on the peer, the
//     peer's withdrawal empties the NGFW RIB (time measured; acceptance ≤ 200 s), `show ip rip status` (FRR 10.7.1 has no
//     JSON form of it — checked);
//  3. removal of routing.rip removes `router rip` (rendered file and running config) and every rip route.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/frrtest"
	"ngfw/agent/internal/renderers/frr/rip"
	"ngfw/agent/internal/vpp/vpptest"
)

func hostDoc(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func hostIP(t *testing.T, h *frrtest.Harness, args ...string) {
	t.Helper()
	out, err := h.Runner.Run(context.Background(), renderers.Command{Path: frrtest.IPBin, Args: args, Timeout: 20 * time.Second})
	if err != nil {
		t.Fatalf("ip %s: %v: %s", strings.Join(args, " "), err, out.Stderr)
	}
}

// hostApplyFiles validates (vtysh -C) and applies (frr-reload + convergence check) files with r on h's FRR.
func hostApplyFiles(t *testing.T, r *frr.Renderer, h *frrtest.Harness, files renderers.Files, what string) renderers.Files {
	t.Helper()
	ctx := context.Background()
	h.AssertScoped(t, files)
	if err := r.Validate(ctx, files); err != nil {
		t.Fatalf("%s: validate (vtysh -C): %v\nrendered:\n%s", what, err, files.Redacted()[r.Paths().ConfFile()].Content)
	}
	if err := r.Apply(ctx, files); err != nil {
		t.Fatalf("%s: apply (frr-reload + convergence check): %v\nrendered:\n%s", what, err, files.Redacted()[r.Paths().ConfFile()].Content)
	}
	t.Logf("%s: Apply converged", what)
	return files
}

func hostRender(t *testing.T, r *frr.Renderer, d any) renderers.Files {
	t.Helper()
	var f renderers.Files
	var err error
	switch v := d.(type) {
	case *structpb.Struct:
		f, err = r.Render(context.Background(), v)
	default:
		f, err = r.Render(context.Background(), parse(t, d.(string)))
	}
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func hostWait(t *testing.T, what string, d time.Duration, cond func() (bool, string)) (string, time.Duration) {
	t.Helper()
	start := time.Now()
	deadline := start.Add(d)
	for {
		ok, state := cond()
		if ok {
			took := time.Since(start)
			return fmt.Sprintf("%s after %v (%s)", what, took.Round(100*time.Millisecond), state), took
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: not reached within %v (last: %s)", what, d, state)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// hostRoutes lists the prefixes of `show ip route <proto> json` (default VRF).
func hostRoutes(t *testing.T, r *frr.Renderer, proto string) []string {
	t.Helper()
	raw, err := r.ShowJSON(context.Background(), frr.ShowCommand("show ip route "+proto+" json"))
	if err != nil {
		t.Fatalf("show ip route %s json: %v", proto, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode routes: %v: %.300s", err, raw)
	}
	out := make([]string, 0, len(m))
	for p := range m {
		out = append(out, p)
	}
	return out
}

func hostCountIn(routes []string, want map[string]bool) int {
	n := 0
	for _, r := range routes {
		if want[r] {
			n++
		}
	}
	return n
}

func hostLinesIn(t *testing.T, rendered, running string, heads ...string) int {
	t.Helper()
	checked := 0
	for _, l := range strings.Split(rendered, "\n") {
		ours := false
		for _, p := range heads {
			ours = ours || strings.HasPrefix(l, p)
		}
		if !ours {
			continue
		}
		checked++
		if !strings.Contains(running, l+"\n") {
			t.Errorf("rendered line %q is not in the running config", l)
		}
	}
	return checked
}

func TestRIPHostLive(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix := vpptest.Prefix(t)
	slot := vpptest.Slot(t)
	ctx := context.Background()
	daemons := []string{"mgmtd", "zebra", "staticd", "ripd"}

	dummy, dummyNet := prefix+"d0", fmt.Sprintf("10.%d.8.0/24", slot)
	ngfw := frrtest.Start(t, frrtest.Options{Prefix: prefix, Daemons: daemons,
		Links: []frrtest.Link{{Name: dummy, Kind: "dummy", CIDR: fmt.Sprintf("10.%d.8.1/24", slot)}}})
	peer := frrtest.Start(t, frrtest.Options{Prefix: prefix, Instance: "p1", Daemons: daemons})
	vIf, pIf := prefix+"v0", prefix+"v1"
	vAddr, pAddr := fmt.Sprintf("10.%d.9.1", slot), fmt.Sprintf("10.%d.9.2", slot)
	hostIP(t, ngfw, "link", "add", vIf, "netns", ngfw.NetNS, "type", "veth", "peer", "name", pIf, "netns", peer.NetNS)
	hostIP(t, ngfw, "-n", ngfw.NetNS, "addr", "add", vAddr+"/24", "dev", vIf)
	hostIP(t, ngfw, "-n", peer.NetNS, "addr", "add", pAddr+"/24", "dev", pIf)
	hostIP(t, ngfw, "-n", ngfw.NetNS, "link", "set", vIf, "up")
	hostIP(t, ngfw, "-n", peer.NetNS, "link", "set", pIf, "up")
	t.Logf("ngfw %s (pathspace %s, pids %v), peer %s (pathspace %s, pids %v), veth %s %s ↔ %s %s, dummy %s %s",
		ngfw.NetNS, ngfw.Paths.Namespace, ngfw.PIDs(), peer.NetNS, peer.Paths.Namespace, peer.PIDs(), vIf, vAddr, pIf, pAddr, dummy, dummyNet)
	if ver, err := ngfw.Renderer().Show(ctx, frr.ShowVersion); err == nil {
		t.Logf("show version: %s", strings.SplitN(string(ver), "\n", 2)[0])
	}

	// ---- 1. golden accepted by FRR 10.7.1
	goldenMap := func(n string) (string, bool) {
		switch n {
		case "host-w8l0":
			return prefix + "-l0", true
		case "loop0":
			return prefix + "-lo", true
		}
		return "", false
	}
	rg := ngfw.Renderer(frr.WithSections(rip.Section{}), frr.WithInterfaceMapper(goldenMap))
	want, err := os.ReadFile(filepath.Join("testdata", "full.golden"))
	if err != nil {
		t.Fatal(err)
	}
	gfiles := hostRender(t, rg, fullDoc)
	grendered := string(gfiles[rg.Paths().ConfFile()].Content)
	if g := strings.ReplaceAll(string(want), "w8-", prefix+"-"); grendered != g {
		t.Fatalf("render of fullDoc with slot names differs from testdata/full.golden (w8- → %s-):\n--- got\n%s\n--- want\n%s", prefix, grendered, g)
	}
	t.Logf("golden: fullDoc rendered with %s-l0/-lo == testdata/full.golden (w8- → %s-):\n%s", prefix, prefix, grendered)
	hostApplyFiles(t, rg, ngfw, gfiles, "golden")
	gdry, err := rg.DryRun(ctx, gfiles)
	if err != nil || gdry != "" {
		t.Fatalf("golden: frr-reload DryRun of the applied config: %q %v (want no diff)", gdry, err)
	}
	t.Log("golden: frr-reload --test of the applied golden: no diff (FRR 10.7.1 holds it in canonical form)")
	grc, err := rg.Show(ctx, frr.ShowRunningConfig)
	if err != nil {
		t.Fatal(err)
	}
	n := hostLinesIn(t, grendered, string(grc), "router rip", " version", " default-metric", " network", " passive-interface", " redistribute")
	t.Logf("golden: %d rendered rip lines found verbatim in show running-config:\n%s", n, grc)

	// ---- 2. live RIPv2 between the two instances (the NGFW side replaces the golden)
	rv := ngfw.Renderer()
	rp := peer.Renderer()
	const nPrefixes = 20
	var statics []any
	announced := map[string]bool{}
	for i := 0; i < nPrefixes; i++ {
		p := fmt.Sprintf("10.%d.%d.0/24", slot, 64+i)
		announced[p] = true
		statics = append(statics, map[string]any{"prefix": p, "blackhole": true, "frr": true})
	}
	peerDoc := func(announce bool) *structpb.Struct {
		o := map[string]any{"interfaces": map[string]any{pIf: map[string]any{}}}
		routing := map[string]any{"rip": o}
		if announce {
			o["redistribute"] = map[string]any{"static": map[string]any{}}
			routing["static"] = statics
		}
		return hostDoc(t, map[string]any{"routing": routing})
	}
	ngfwDoc := hostDoc(t, map[string]any{"routing": map[string]any{"rip": map[string]any{
		"vrf": "default", "defaultMetric": 2,
		"interfaces":   map[string]any{vIf: map[string]any{}, dummy: map[string]any{"passive": true}},
		"redistribute": map[string]any{"connected": map[string]any{"metric": 3}},
	}}})
	pfiles := hostApplyFiles(t, rp, peer, hostRender(t, rp, peerDoc(true)), "peer")
	files := hostApplyFiles(t, rv, ngfw, hostRender(t, rv, ngfwDoc), "ngfw live")
	rendered := string(files[rv.Paths().ConfFile()].Content)
	t.Logf("ngfw rendered %s:\n%s", rv.Paths().ConfFile(), rendered)
	if dry, err := rv.DryRun(ctx, files); err != nil || dry != "" {
		t.Fatalf("DryRun of the applied config: %q %v (want no diff)", dry, err)
	}
	if pdry, err := rp.DryRun(ctx, pfiles); err != nil || pdry != "" {
		t.Fatalf("peer DryRun of the applied config: %q %v (want no diff)", pdry, err)
	}
	t.Log("frr-reload --test of the applied live configs (ngfw, peer): no diff")
	rc, err := rv.Show(ctx, frr.ShowRunningConfig)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rc), "10.8.0.0/16") || strings.Contains(string(rc), prefix+"-lo") {
		t.Errorf("the golden's networks survived the live apply:\n%s", rc)
	}
	n = hostLinesIn(t, rendered, string(rc), "router rip", " version", " default-metric", " network", " passive-interface", " redistribute")
	t.Logf("%d rendered rip lines found verbatim in show running-config", n)

	msg, _ := hostWait(t, fmt.Sprintf("the peer's %d prefixes as rip routes on the NGFW side", nPrefixes), 90*time.Second, func() (bool, string) {
		c := hostCountIn(hostRoutes(t, rv, "rip"), announced)
		return c == nPrefixes, fmt.Sprintf("%d of %d", c, nPrefixes)
	})
	t.Log(msg)
	msg, _ = hostWait(t, "NGFW's connected "+dummyNet+" as a rip route on the peer", 90*time.Second, func() (bool, string) {
		routes := hostRoutes(t, rp, "rip")
		return hostCountIn(routes, map[string]bool{dummyNet: true}) == 1, strings.Join(routes, " ")
	})
	t.Log(msg)
	if one, err := rv.Show(ctx, frr.ShowCommand(fmt.Sprintf("show ip route 10.%d.64.0/24", slot))); err == nil {
		t.Logf("NGFW show ip route 10.%d.64.0/24:\n%s", slot, one)
	}
	if st, err := rv.Show(ctx, frr.ShowCommand("show ip rip status")); err == nil {
		t.Logf("NGFW show ip rip status (text):\n%s", st)
	} else {
		t.Errorf("show ip rip status: %v", err)
	}
	if tbl, err := rv.Show(ctx, frr.ShowCommand("show ip rip")); err == nil {
		t.Logf("NGFW show ip rip (text, first lines):\n%s", firstLines(string(tbl), 12))
	}
	// FRR 10.7.1 has no JSON form of the RIP status/table (the reason rip has no state reader): checked, not assumed
	for _, c := range []string{"show ip rip json", "show ip rip status json"} {
		_, err := rv.ShowJSON(ctx, frr.ShowCommand(c))
		t.Logf("%s → %v", c, err)
		if err == nil {
			t.Logf("NOTE: FRR 10.7.1 answers %q with JSON — a rip state reader is possible", c)
		}
	}

	// the peer withdraws → the 20 prefixes leave the NGFW RIB (acceptance ≤ 200 s)
	hostApplyFiles(t, rp, peer, hostRender(t, rp, peerDoc(false)), "peer withdraws")
	msg, took := hostWait(t, "0 of the peer's prefixes on the NGFW side after the withdrawal", 200*time.Second, func() (bool, string) {
		c := hostCountIn(hostRoutes(t, rv, "rip"), announced)
		return c == 0, fmt.Sprintf("%d of %d", c, nPrefixes)
	})
	t.Logf("%s — acceptance: withdrawn within 200 s: %v", msg, took <= 200*time.Second)

	// ---- 3. removal (rollback of routing.rip): no `router rip` in the rendered file or the running config, no rip route
	hostApplyFiles(t, rv, ngfw, hostRender(t, rv, hostDoc(t, map[string]any{})), "ngfw removal")
	rc, _ = rv.Show(ctx, frr.ShowRunningConfig)
	if strings.Contains(string(rc), "router rip") {
		t.Errorf("router rip still in the running config after removal:\n%s", rc)
	}
	if c := len(hostRoutes(t, rv, "rip")); c != 0 {
		t.Errorf("%d rip routes left after removal", c)
	}
	conf, err := os.ReadFile(rv.Paths().ConfFile())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(conf), "router rip") {
		t.Errorf("the rendered file %s still holds router rip:\n%s", rv.Paths().ConfFile(), conf)
	}
	t.Logf("after removal, rendered %s:\n%s\nshow running-config:\n%s", rv.Paths().ConfFile(), conf, rc)
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n… (%d more lines)", len(lines)-n)
}
