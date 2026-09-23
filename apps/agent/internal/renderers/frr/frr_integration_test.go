package frr_test

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/frrtest"
	"ngfw/agent/internal/vpp/vpptest"
)

// etcFRRState fingerprints /etc/frr (names, sizes, mtimes, modes) to prove the test never
// touches it.
func etcFRRState(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	entries, err := os.ReadDir("/etc/frr")
	if err != nil {
		return "absent: " + err.Error()
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%s %d %d %v\n", e.Name(), info.Size(), info.ModTime().UnixNano(), info.Mode())
	}
	return b.String()
}

// leftoverDaemons lists processes whose cmdline names base (the pgrep -f check, without
// pgrep).
func leftoverDaemons(base string) []string {
	var out []string
	procs, _ := filepath.Glob("/proc/[0-9]*/cmdline")
	for _, p := range procs {
		b, err := os.ReadFile(p) //nolint:gosec // /proc read
		if err == nil && strings.Contains(string(b), base) {
			out = append(out, p+": "+strings.ReplaceAll(string(b), "\x00", " "))
		}
	}
	return out
}

func mustDoc(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFRRRendererIntegration(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix := vpptest.Prefix(t)
	slot := vpptest.Slot(t)
	table := vpptest.TableBase(t) + 1
	ctx := context.Background()

	etcBefore := etcFRRState(t)
	base := filepath.Dir(frr.TestPaths(prefix).ConfDir)
	// Registered before the harness so it runs after the harness's Stop (Cleanup is LIFO).
	t.Cleanup(func() {
		if got := etcFRRState(t); got != etcBefore {
			t.Errorf("/etc/frr changed:\nbefore:\n%s\nafter:\n%s", etcBefore, got)
		}
		if left := leftoverDaemons(base); len(left) > 0 {
			t.Errorf("daemons left running: %v", left)
		}
		t.Logf("after Stop: no process names %s; /etc/frr unchanged:\n%s", base, etcBefore)
	})

	up := prefix + "f0" // e.g. w12f0: 10.<N>.1.2/24, gateway 10.<N>.1.1 (connected)
	vrf := prefix + "red"
	vrfIf := prefix + "r0"
	h := frrtest.Start(t, frrtest.Options{Prefix: prefix, Links: []frrtest.Link{
		{Name: up, Kind: "dummy", CIDR: fmt.Sprintf("10.%d.1.2/24", slot)},
		{Name: vrf, Kind: "vrf", Table: table},
		{Name: vrfIf, Kind: "dummy", Master: vrf, CIDR: fmt.Sprintf("10.%d.2.2/24", slot)},
	}})
	t.Logf("harness: netns %s, daemons %v, argv zebra: ip netns exec %s %s", h.NetNS, h.PIDs(), h.NetNS, strings.Join(h.DaemonArgs("zebra"), " "))

	host, _ := os.Hostname()
	if _, err := frr.Hostname(host); err != nil {
		host = "" // FRR 10.7 daemons report the system hostname; render it only when valid
	}
	r := h.Renderer(frr.WithSections(), frr.WithStateReaders())
	gw1 := fmt.Sprintf("10.%d.1.1", slot)
	gw9 := fmt.Sprintf("10.%d.1.9", slot)
	pfx200 := fmt.Sprintf("10.%d.200.0/24", slot)
	pfx201 := fmt.Sprintf("10.%d.201.0/24", slot)
	pfx210 := fmt.Sprintf("10.%d.210.0/24", slot)
	desired := func(gw string, withRoutes bool) *structpb.Struct {
		d := map[string]any{
			"system":     map[string]any{"hostname": host},
			"vrfs":       map[string]any{"default": map[string]any{"id": 0}, vrf: map[string]any{"id": table}},
			"interfaces": map[string]any{up: map[string]any{"description": `"; rm -rf /`}},
		}
		if host == "" {
			delete(d, "system")
		}
		if withRoutes {
			d["routing"] = map[string]any{"static": []any{
				map[string]any{"prefix": pfx200, "nextHops": []any{map[string]any{"address": gw}}},
				map[string]any{"prefix": pfx201, "nextHops": []any{map[string]any{"blackhole": true}}, "tag": 100, "distance": 50},
				map[string]any{"prefix": pfx210, "vrf": vrf, "nextHops": []any{map[string]any{"address": fmt.Sprintf("10.%d.2.1", slot)}}},
			}}
		}
		return mustDoc(t, d)
	}
	apply := func(step string, d *structpb.Struct) {
		t.Helper()
		files, err := r.Render(ctx, d)
		if err != nil {
			t.Fatalf("%s: Render: %v", step, err)
		}
		h.AssertScoped(t, files)
		if err := r.Validate(ctx, files); err != nil {
			t.Fatalf("%s: Validate: %v", step, err)
		}
		diff, err := r.DryRun(ctx, files)
		if err != nil {
			t.Fatalf("%s: DryRun: %v", step, err)
		}
		t.Logf("%s: rendered frr.conf:\n%s\n%s: frr-reload.py --test diff:\n%s", step, files[h.Paths.ConfFile()].Content, step, diff)
		if err := r.Apply(ctx, files); err != nil {
			t.Fatalf("%s: Apply: %v", step, err)
		}
		again, err := r.DryRun(ctx, files)
		if err != nil {
			t.Fatalf("%s: DryRun after Apply: %v", step, err)
		}
		if again != "" {
			t.Errorf("%s: not idempotent, diff after Apply:\n%s", step, again)
		}
	}
	statics := func() []frr.RIBRoute {
		t.Helper()
		st, err := r.State(ctx)
		if err != nil {
			t.Fatal(err)
		}
		rs, err := st.StaticRoutes()
		if err != nil {
			t.Fatal(err)
		}
		return rs
	}
	showIPRoute := func(step string) {
		raw, err := r.ShowJSON(ctx, frr.ShowIPRoute)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]json.RawMessage
		_ = json.Unmarshal(raw, &m)
		var b strings.Builder
		for _, k := range slices.Sorted(maps.Keys(m)) {
			if k == pfx200 || k == pfx201 {
				fmt.Fprintf(&b, "%q: %s\n", k, m[k])
			}
		}
		t.Logf("%s: vtysh --command 'show ip route json' (static prefixes):\n%s", step, b.String())
	}

	pids := h.PIDs()
	if len(pids) != 3 {
		t.Fatalf("PIDs = %v", pids)
	}
	poller := r.NewPoller()
	if _, err := poller.Step(ctx); err != nil {
		t.Fatalf("poller baseline: %v", err)
	}

	// 1. two static routes (+ one in the VRF), a hostile description.
	apply("step1", desired(gw1, true))
	showIPRoute("step1")
	got := statics()
	byPrefix := map[string]frr.RIBRoute{}
	for _, rt := range got {
		byPrefix[rt.VRFName+" "+rt.Prefix] = rt
	}
	if rt, ok := byPrefix["default "+pfx200]; !ok || !rt.Installed || rt.Nexthops[0].IP != gw1 {
		t.Errorf("step1: %s via %s missing/not installed: %+v", pfx200, gw1, got)
	}
	if rt, ok := byPrefix["default "+pfx201]; !ok || !rt.Nexthops[0].Blackhole || rt.Distance != 50 || rt.Tag != 100 {
		t.Errorf("step1: blackhole %s missing: %+v", pfx201, got)
	}
	if rt, ok := byPrefix[vrf+" "+pfx210]; !ok || !rt.Installed {
		t.Errorf("step1: %s in vrf %s missing: %+v", pfx210, vrf, got)
	}
	kernel, err := h.Runner.Run(ctx, renderers.Command{Path: frrtest.IPBin, Args: []string{"-n", h.NetNS, "route", "show", pfx200}})
	if err != nil || !strings.Contains(string(kernel.Stdout), "via "+gw1) {
		t.Errorf("step1: kernel route in %s: %q %v", h.NetNS, kernel.Stdout, err)
	}
	t.Logf("step1: ip -n %s route show %s: %s", h.NetNS, pfx200, strings.TrimSpace(string(kernel.Stdout)))
	ifRaw, err := r.ShowJSON(ctx, frr.ShowInterfaceAll)
	if err != nil || !strings.Contains(string(ifRaw), `"description":"\"; rm -rf /"`) {
		t.Errorf("step1: description not stored verbatim: %v", err)
	}
	if ev, err := poller.Step(ctx); err != nil || len(ev) == 0 {
		t.Errorf("step1: no route-count events: %v %v", ev, err)
	} else {
		t.Logf("step1: events %v", ev)
	}

	// 2. change one route: frr-reload applies a diff; the daemons keep their PIDs.
	apply("step2", desired(gw9, true))
	showIPRoute("step2")
	got = statics()
	for _, rt := range got {
		if rt.Prefix == pfx200 && rt.Nexthops[0].IP != gw9 {
			t.Errorf("step2: %s still via %s", pfx200, rt.Nexthops[0].IP)
		}
	}
	if now := h.PIDs(); !maps.Equal(now, pids) {
		t.Errorf("daemon restarted: PIDs before %v, after %v", pids, now)
	} else {
		t.Logf("step2: PIDs unchanged %v (reload, not restart)", now)
	}

	// 3. link event: w<N>f0 down → LINK_DOWN from the poller.
	if _, err := h.Runner.Run(ctx, renderers.Command{Path: frrtest.IPBin, Args: []string{"-n", h.NetNS, "link", "set", up, "down"}}); err != nil {
		t.Fatal(err)
	}
	ev, err := poller.Step(ctx)
	if err != nil {
		raw, _ := r.Show(ctx, frr.ShowInterfaceAll)
		t.Fatalf("%v\n%s", err, raw)
	}
	var linkDown bool
	for _, e := range ev {
		if e.Poller == frr.PollerInterfaces && e.Key == up && e.New == "down" && e.ToProto().GetKind().String() == "EVENT_KIND_LINK_DOWN" {
			linkDown = true
		}
	}
	if !linkDown {
		t.Errorf("step3: no LINK_DOWN for %s in %v", up, ev)
	}
	t.Logf("step3: events after link down %v", ev)
	if _, err := h.Runner.Run(ctx, renderers.Command{Path: frrtest.IPBin, Args: []string{"-n", h.NetNS, "link", "set", up, "up"}}); err != nil {
		t.Fatal(err)
	}

	// 4. remove all routes: Retrieve shows none.
	apply("step4", desired(gw9, false))
	if got := statics(); len(got) != 0 {
		t.Errorf("step4: static routes remain: %+v", got)
	}
	msg, err := r.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rc := msg.(*structpb.Struct).GetFields()["runningConfig"].GetListValue()
	for _, v := range rc.GetValues() {
		if strings.Contains(v.GetStringValue(), "route ") {
			t.Errorf("step4: running-config still has %q", v.GetStringValue())
		}
	}
	t.Logf("step4: Retrieve runningConfig %v", rc.AsSlice())
	if now := h.PIDs(); !maps.Equal(now, pids) {
		t.Errorf("daemon restarted: PIDs before %v, after %v", pids, now)
	} else {
		t.Logf("step4: PIDs unchanged %v", now)
	}
}
