package isis_test

// Live test of the isis section against FRR 10.7.1 (NGFW_INTEGRATION=1; row F-isis-rip-host, RV-A R7/R1 owed list: "frrtest
// with isisd: golden accepted, DryRun empty, isis neighbor json parser on real output").
//
// This file lives in test/topology/isis-rip/agent-overlay/ (the row's file fence) and is compiled into
// apps/agent/internal/renderers/frr/isis as isis_host_integration_test.go by test/topology/isis-rip/run.sh through
// `go test -overlay` — nothing under apps/ is written. It needs the agent-internal frrtest harness and the parser, which a
// module under test/ cannot import.
//
// Two slot-scoped FRR instances (frrtest: the NGFW side in ns-<prefix>-frr, a peer in ns-<prefix>-p1) joined by a veth pair
// inside the slot's namespaces — no VPP, no root namespace, nothing under /etc/frr:
//
//  1. golden accepted: the golden's own input (fullDoc) rendered with slot interface names is testdata/full.golden with
//     "w8-" → "<prefix>-", FRR 10.7.1 accepts it (vtysh -C + frr-reload), frr-reload DryRun of the applied file is empty
//     and every rendered isis/interface line is in vtysh's running-config;
//  2. live: NGFW (level-1-2, a level-2 point-to-point circuit, a passive dummy, connected redistributed) ↔ peer (level-2,
//     20 blackhole statics redistributed): adjacency Up, the parser reads the real `show isis vrf all neighbor json`, 20
//     IS-IS routes on the NGFW side, NGFW's connected prefix on the peer, Retrieve carries the reader, the isis-adjacencies
//     poller snapshot holds the adjacency, the peer's withdrawal empties the NGFW RIB, the adjacency loss is a poller event;
//  3. removal of routing.isis removes `router isis` and every `isis`/`router isis` interface line.

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
	"ngfw/agent/internal/renderers/frr/isis"
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

// frrDefaultLines are lines the isis section renders whose value is FRR 10.7.1's default, so vtysh's running-config never
// shows them and the renderer's convergence check (frr-reload --test after --reload, RF-1 review H2) reports them as
// "Lines To Add" forever (finding F1 of row F-isis-rip-host: every routing.isis Apply is rolled back).
var frrDefaultLines = map[string]bool{" metric-style wide": true, " is-type level-1-2": true, " isis metric 10": true}

// notConverged returns the indented "Lines To Add" of a "not converged" Apply error (nil when err is something else).
func notConverged(err error) []string {
	if err == nil || !strings.Contains(err.Error(), "not converged") {
		return nil
	}
	_, add, ok := strings.Cut(err.Error(), "Lines To Add")
	if !ok {
		return nil
	}
	var out []string
	for _, l := range strings.Split(add, "\n") {
		if strings.HasPrefix(l, " ") && strings.TrimSpace(l) != "" {
			out = append(out, strings.TrimRight(l, " "))
		}
	}
	return out
}

// stripDefaults returns files with the frrDefaultLines removed from frr.conf (test-only workaround for F1).
func stripDefaults(r *frr.Renderer, files renderers.Files) (renderers.Files, []string) {
	out := renderers.Files{}
	var stripped []string
	for p, f := range files {
		if p == r.Paths().ConfFile() {
			var keep []string
			for _, l := range strings.Split(string(f.Content), "\n") {
				if frrDefaultLines[l] {
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

// hostApplyF1 applies files with r.Apply and records the two FRR 10.7.1 findings of row F-isis-rip-host instead of
// stopping, so the rest of the owed evidence can run on the same FRR:
//   - F1: FRR does not converge only because of FRR-default lines → reported, the render without them is applied
//     (test-only workaround);
//   - F2: frr-reload.py exits 1 because FRR refused a "no isis …" interface line after "no ip router isis" removed the
//     circuit (YANG: mandatory area-tag missing) → the renderer rolled back; reported, frr-reload is re-run with its log
//     on stdout (shows the refused commands), the end state must be converged (DryRun empty) and the files are written
//     as Apply would have.
func hostApplyF1(t *testing.T, r *frr.Renderer, h *frrtest.Harness, files renderers.Files, what string) renderers.Files {
	t.Helper()
	ctx := context.Background()
	h.AssertScoped(t, files)
	if err := r.Validate(ctx, files); err != nil {
		t.Fatalf("%s: validate (vtysh -C): %v", what, err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		err := r.Apply(ctx, files)
		if err == nil {
			t.Logf("%s: Apply converged", what)
			return files
		}
		if missing := notConverged(err); len(missing) > 0 {
			for _, l := range missing {
				if !frrDefaultLines[l] {
					t.Fatalf("%s: FRR did not take %q, which is not an FRR-default line: %v", what, l, err)
				}
			}
			t.Errorf("FINDING F1 (%s): renderer.Apply failed and was rolled back — FRR 10.7.1 took every line but omits FRR-default values from show running-config, so the convergence check never converges; missing only %q:\n%v", what, missing, err)
			var stripped []string
			files, stripped = stripDefaults(r, files)
			t.Logf("%s: test-only workaround F1: applying the same render without the FRR-default lines %q", what, stripped)
			continue
		}
		if strings.Contains(err.Error(), "frr-reload.py --reload") && strings.Contains(err.Error(), "exited 1") {
			refused := reloadDiag(t, r, h, files)
			dry, derr := r.DryRun(ctx, files)
			if derr != nil || dry != "" || len(refused) == 0 {
				t.Fatalf("%s: frr-reload exit 1 and not converged after a re-run (refused %q): diff %q %v; apply error: %v", what, refused, dry, derr, err)
			}
			if werr := renderers.WriteFiles(files); werr != nil {
				t.Fatal(werr)
			}
			t.Errorf("FINDING F2 (%s): renderer.Apply failed and was rolled back — frr-reload.py exited 1 because FRR 10.7.1 refused %d \"no isis …\" interface line(s) after \"no ip router isis\" had removed the circuit (YANG: mandatory area-tag missing); the end state is converged (frr-reload --test empty after a re-run): refused %q", what, len(refused), refused)
			return files
		}
		t.Fatalf("%s: apply: %v", what, err)
	}
	t.Fatalf("%s: still not applied after the F1 workaround", what)
	return nil
}

// reloadDiag re-runs frr-reload.py on the staged render with its log on stdout (the renderer runs it at log level
// critical, so a refused "no …" command is invisible in the Apply error): --test shows the diff, --reload which command
// FRR refused. Test configs carry no secret.
func reloadDiag(t *testing.T, r *frr.Renderer, h *frrtest.Harness, files renderers.Files) []string {
	t.Helper()
	var refused []string
	st, err := renderers.Stage(files)
	if err != nil {
		t.Logf("diag: stage: %v", err)
		return nil
	}
	defer func() { _ = st.Close() }()
	p := r.Paths()
	base := []string{"--bindir", p.BinDir, "--confdir", p.ConfDir, "--rundir", p.SocketDir(), "--vty_socket", p.RunDir, "--pathspace", p.Namespace}
	for _, mode := range [][]string{{"--test"}, {"--reload", "--stdout", "--log-level", "info"}, {"--test"}} {
		args := append(append(append([]string{}, mode...), base...), st.Path(p.ConfFile()))
		out, err := h.Runner.Run(context.Background(), renderers.Command{Path: frr.ReloadBin, Args: args, Timeout: 2 * time.Minute})
		t.Logf("diag: frr-reload.py %s: err=%v\nstdout:\n%s\nstderr:\n%s", strings.Join(mode, " "), err, out.Stdout, out.Stderr)
		if mode[0] != "--reload" {
			continue
		}
		for _, l := range strings.Split(string(out.Stdout)+"\n"+string(out.Stderr), "\n") {
			if !strings.Contains(l, "we failed to remove this command") {
				continue
			}
			if i, j := strings.Index(l, `"`), strings.LastIndex(l, `"`); i >= 0 && j > i {
				refused = append(refused, l[i+1:j])
			}
		}
	}
	return refused
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

func hostCount(routes []string, prefix string) int {
	n := 0
	for _, r := range routes {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

// hostCountIn counts the routes that are one of want (the peer's announced prefixes).
func hostCountIn(routes []string, want map[string]bool) int {
	n := 0
	for _, r := range routes {
		if want[r] {
			n++
		}
	}
	return n
}

// hostLinesIn checks that every rendered line starting with one of the given heads is a line of the running config.
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

func TestISISHostLive(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix := vpptest.Prefix(t)
	slot := vpptest.Slot(t)
	ctx := context.Background()
	daemons := []string{"mgmtd", "zebra", "staticd", "isisd"}

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

	// ---- 1. golden accepted by FRR 10.7.1: render fullDoc with slot interface names, compare with the golden, apply
	goldenMap := func(n string) (string, bool) {
		switch n {
		case "host-w8l0":
			return prefix + "-l0", true
		case "host-w8l1":
			return prefix + "-l1", true
		case "loop0":
			return prefix + "-lo", true
		}
		return "", false
	}
	rg := ngfw.Renderer(frr.WithSections(isis.Section{}), frr.WithInterfaceMapper(goldenMap),
		frr.WithInterfaceLines(frr.NamedInterfaceLines{Name: isis.Name, Fn: isis.InterfaceLines}))
	want, err := os.ReadFile(filepath.Join("testdata", "full.golden"))
	if err != nil {
		t.Fatal(err)
	}
	gfiles, err := rg.Render(ctx, parse(t, fullDoc))
	if err != nil {
		t.Fatal(err)
	}
	grendered := string(gfiles[rg.Paths().ConfFile()].Content)
	if g := strings.ReplaceAll(string(want), "w8-", prefix+"-"); grendered != g {
		t.Fatalf("render of fullDoc with slot names differs from testdata/full.golden (w8- → %s-):\n--- got\n%s\n--- want\n%s", prefix, grendered, g)
	}
	t.Logf("golden: fullDoc rendered with %s-l0/-l1/-lo == testdata/full.golden (w8- → %s-):\n%s", prefix, prefix, grendered)
	gfiles = hostApplyF1(t, rg, ngfw, gfiles, "golden")
	grendered = string(gfiles[rg.Paths().ConfFile()].Content)
	gdry, err := rg.DryRun(ctx, gfiles)
	if err != nil || gdry != "" {
		t.Fatalf("golden: frr-reload DryRun of the applied config: %q %v (want no diff)", gdry, err)
	}
	t.Log("golden: frr-reload --test of the applied golden: no diff (FRR 10.7.1 holds every applied line in canonical form)")
	grc, err := rg.Show(ctx, frr.ShowRunningConfig)
	if err != nil {
		t.Fatal(err)
	}
	n := hostLinesIn(t, grendered, string(grc), "router isis", "interface ", " ip router isis", " ipv6 router isis", " isis ", " is-type", " net ", " metric-style", " redistribute")
	t.Logf("golden: %d rendered isis/interface lines found verbatim in show running-config:\n%s", n, grc)

	// ---- 1b. removal of the whole golden IS-IS configuration (the rollback path of a full routing.isis)
	rv := ngfw.Renderer()
	hostApplyF1(t, rv, ngfw, hostRender(t, rv, hostDoc(t, map[string]any{})), "golden removal")
	if grc, err := rv.Show(ctx, frr.ShowRunningConfig); err == nil && strings.Contains(string(grc), "isis") {
		t.Errorf("golden removal left IS-IS lines:\n%s", grc)
	} else {
		t.Log("golden removal: no isis line left in show running-config")
	}

	// ---- 2. live adjacency
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
		o := map[string]any{"net": fmt.Sprintf("49.%04d.0000.0000.0002.00", slot), "level": "level-2",
			"interfaces": map[string]any{pIf: map[string]any{"networkType": "point-to-point"}}}
		routing := map[string]any{"isis": o}
		if announce {
			o["redistribute"] = map[string]any{"static": map[string]any{}}
			routing["static"] = statics
		}
		return hostDoc(t, map[string]any{"routing": routing})
	}
	ngfwDoc := hostDoc(t, map[string]any{"routing": map[string]any{"isis": map[string]any{
		"net": fmt.Sprintf("49.%04d.0000.0000.0001.00", slot), "level": "level-1-2", "vrf": "default",
		"interfaces": map[string]any{
			vIf:   map[string]any{"metric": 10, "circuitType": "level-2", "networkType": "point-to-point"},
			dummy: map[string]any{"passive": true},
		},
		"redistribute": map[string]any{"connected": map[string]any{"metric": 20}},
	}}})
	pfiles := hostApplyF1(t, rp, peer, hostRender(t, rp, peerDoc(true)), "peer (minimal level-2 IS)")
	files := hostApplyF1(t, rv, ngfw, hostRender(t, rv, ngfwDoc), "ngfw live")
	rendered := string(files[rv.Paths().ConfFile()].Content)
	t.Logf("ngfw rendered %s:\n%s", rv.Paths().ConfFile(), rendered)
	dry, err := rv.DryRun(ctx, files)
	if err != nil || dry != "" {
		t.Fatalf("DryRun of the applied config: %q %v (want no diff)", dry, err)
	}
	t.Log("frr-reload --test of the applied live config: no diff")
	pdry, err := rp.DryRun(ctx, pfiles)
	if err != nil || pdry != "" {
		t.Fatalf("peer DryRun of the applied config: %q %v (want no diff)", pdry, err)
	}
	rc, err := rv.Show(ctx, frr.ShowRunningConfig)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rc), prefix+"-l0") || strings.Contains(string(rc), prefix+"-lo") {
		t.Errorf("the golden's interfaces survived the live apply (frr-reload did not remove them):\n%s", rc)
	}
	n = hostLinesIn(t, rendered, string(rc), "router isis", "interface ", " ip router isis", " ipv6 router isis", " isis ", " is-type", " net ", " metric-style", " redistribute")
	t.Logf("%d rendered isis/interface lines found verbatim in show running-config", n)

	msg, took := hostWait(t, "adjacency Up on the NGFW side (parsed by isis.Neighbors)", 90*time.Second, func() (bool, string) {
		as, err := isis.Neighbors(ctx, rv.ShowJSON)
		if err != nil {
			return false, err.Error()
		}
		for _, a := range as {
			if a.Interface == vIf && a.State == "Up" {
				return true, fmt.Sprintf("%+v", a)
			}
		}
		return false, fmt.Sprintf("%+v", as)
	})
	t.Log(msg)
	_ = took
	rawNbr, err := rv.ShowJSON(ctx, isis.ShowNeighbors)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("real FRR 10.7.1 output of %q (NGFW side):\n%s", isis.ShowNeighbors, rawNbr)
	as, err := isis.ParseNeighbors(rawNbr)
	t.Logf("isis.ParseNeighbors on it: %+v err=%v", as, err)
	if err != nil || len(as) != 1 || as[0].VRF != frr.DefaultVRF || as[0].Area != isis.Tag || as[0].Interface != vIf ||
		as[0].State != "Up" || as[0].SystemID == "" || as[0].Level == "" {
		t.Errorf("neighbour parser on real FRR 10.7.1 output: %+v %v (want one adjacency: vrf %s, area %s, interface %s, state Up, system id and level set)",
			as, err, frr.DefaultVRF, isis.Tag, vIf)
	}
	if txt, err := rv.Show(ctx, frr.ShowCommand("show isis neighbor")); err == nil {
		t.Logf("show isis neighbor (text, NGFW side):\n%s", txt)
	}
	if txt, err := rv.Show(ctx, frr.ShowCommand("show isis neighbor detail")); err == nil {
		t.Logf("show isis neighbor detail (text, NGFW side):\n%s", txt)
	}
	prawNbr, err := rp.ShowJSON(ctx, isis.ShowNeighbors)
	if err == nil {
		pas, perr := isis.ParseNeighbors(prawNbr)
		t.Logf("peer side %q:\n%s\nparsed: %+v err=%v", isis.ShowNeighbors, prawNbr, pas, perr)
	}

	msg, _ = hostWait(t, fmt.Sprintf("%d IS-IS routes on the NGFW side", nPrefixes), 60*time.Second, func() (bool, string) {
		c := hostCountIn(hostRoutes(t, rv, "isis"), announced)
		return c == nPrefixes, fmt.Sprintf("%d of the peer's %d prefixes as isis routes", c, nPrefixes)
	})
	t.Log(msg)
	msg, _ = hostWait(t, "NGFW's connected "+dummyNet+" (metric 20) as an IS-IS route on the peer", 60*time.Second, func() (bool, string) {
		routes := hostRoutes(t, rp, "isis")
		return hostCount(routes, dummyNet) == 1, strings.Join(routes, " ")
	})
	t.Log(msg)
	if one, err := rv.Show(ctx, frr.ShowCommand(fmt.Sprintf("show ip route 10.%d.64.0/24", slot))); err == nil {
		t.Logf("NGFW show ip route 10.%d.64.0/24:\n%s", slot, one)
	}

	// Retrieve carries the reader output; the isis-adjacencies poller holds the adjacency
	st, err := rv.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stJSON, _ := st.(*structpb.Struct).MarshalJSON()
	if !strings.Contains(string(stJSON), `"`+isis.NeighborsReader+`"`) {
		t.Errorf("Retrieve has no %s reader output: %.400s", isis.NeighborsReader, stJSON)
	} else {
		t.Logf("Retrieve carries the %s reader (%d bytes of state)", isis.NeighborsReader, len(stJSON))
	}
	poller := rv.NewPoller()
	if _, err := poller.Step(ctx); err != nil {
		t.Fatalf("poller baseline: %v", err)
	}
	snap, err := isis.PollAdjacencies(ctx, rv.ShowJSON)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("isis-adjacencies poller snapshot: %v", snap)
	found := false
	for k, v := range snap {
		if strings.HasPrefix(k, frr.DefaultVRF+"|"+isis.Tag+"|") && strings.Contains(k, "|"+vIf+"|") && v == "Up" {
			found = true
		}
	}
	if !found {
		t.Errorf("poller snapshot lacks %s|%s|…|%s|…=Up: %v", frr.DefaultVRF, isis.Tag, vIf, snap)
	}

	// ---- the peer withdraws its statics → the NGFW RIB empties
	hostApplyF1(t, rp, peer, hostRender(t, rp, peerDoc(false)), "peer withdraws")
	msg, took = hostWait(t, "0 IS-IS routes of the peer after its withdrawal", 60*time.Second, func() (bool, string) {
		c := hostCountIn(hostRoutes(t, rv, "isis"), announced)
		return c == 0, fmt.Sprintf("%d of the peer's %d prefixes as isis routes", c, nPrefixes)
	})
	t.Log(msg)

	// ---- the peer link goes down → the adjacency loss is an isis-adjacencies event with the old Up state
	hostIP(t, peer, "-n", peer.NetNS, "link", "set", pIf, "down")
	var seen []string
	msg, _ = hostWait(t, "isis-adjacencies event for the lost adjacency", 45*time.Second, func() (bool, string) {
		evs, _ := poller.Step(ctx)
		for _, e := range evs {
			seen = append(seen, e.String())
			if e.Poller == isis.PollerAdjacencies && strings.Contains(e.Key, "|"+vIf+"|") && e.Old == "Up" {
				return true, e.String()
			}
		}
		return false, strings.Join(seen, "; ")
	})
	t.Log(msg)

	// ---- 3. removal (rollback of routing.isis): no `router isis`, no IS-IS interface line, no IS-IS route
	hostApplyF1(t, rv, ngfw, hostRender(t, rv, hostDoc(t, map[string]any{})), "ngfw removal (rollback of routing.isis)")
	rc, _ = rv.Show(ctx, frr.ShowRunningConfig)
	for _, gone := range []string{"router isis", "ip router isis", "ipv6 router isis", " isis "} {
		if strings.Contains(string(rc), gone) {
			t.Errorf("%q still in the running config after removal:\n%s", gone, rc)
		}
	}
	if c := len(hostRoutes(t, rv, "isis")); c != 0 {
		t.Errorf("%d IS-IS routes left after removal", c)
	}
	conf, err := os.ReadFile(rv.Paths().ConfFile())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(conf), "router isis") {
		t.Errorf("the rendered file %s still holds router isis:\n%s", rv.Paths().ConfFile(), conf)
	}
	t.Logf("after removal, rendered %s:\n%s\nshow running-config:\n%s", rv.Paths().ConfFile(), conf, rc)
}

func hostRender(t *testing.T, r *frr.Renderer, d *structpb.Struct) renderers.Files {
	t.Helper()
	f, err := r.Render(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
