package frr_test

import (
	"context"
	"encoding/json"
	"errors"
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
	"ngfw/agent/internal/vpp/vpptest"
)

// Live regression tests for the RF-1 review findings, against test-scoped FRR 10.7.

type rawSection struct {
	name  string
	lines []string
}

func (s rawSection) Name() string                                { return s.name }
func (rawSection) Order() int                                    { return 450 }
func (s rawSection) Render(*frr.RenderContext) ([]string, error) { return s.lines, nil }

type secretSection struct{ ref string }

func (secretSection) Name() string { return "vty-password" }
func (secretSection) Order() int   { return 460 }
func (s secretSection) Render(rc *frr.RenderContext) ([]string, error) {
	v, err := rc.Secret(s.ref)
	if err != nil {
		return nil, err
	}
	return []string{"password " + v}, nil
}

const plantedSecret = "VRX_TEST_PSK_RF1" //nolint:gosec // test placeholder (00-CONTEXT fixture convention)

func startFRR(t *testing.T) (*frrtest.Harness, string, int) {
	t.Helper()
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix, slot := vpptest.Prefix(t), vpptest.Slot(t)
	etcBefore := etcFRRState(t)
	base := filepath.Dir(frr.TestPaths(prefix).ConfDir)
	t.Cleanup(func() {
		if got := etcFRRState(t); got != etcBefore {
			t.Errorf("/etc/frr changed")
		}
		if left := leftoverDaemons(base); len(left) > 0 {
			t.Errorf("daemons left running: %v", left)
		}
	})
	h := frrtest.Start(t, frrtest.Options{Prefix: prefix, Links: []frrtest.Link{
		{Name: prefix + "f0", Kind: "dummy", CIDR: fmt.Sprintf("10.%d.1.2/24", slot)},
	}})
	return h, prefix, slot
}

func describe(t *testing.T, ifname, desc string) *structpb.Struct {
	return mustDoc(t, map[string]any{"interfaces": map[string]any{ifname: map[string]any{"description": desc}}})
}

// H1: '|' never reaches FRR; the reviewer's verbatim cases still converge on real FRR.
func TestReviewH1DescriptionsLive(t *testing.T) {
	h, prefix, _ := startFRR(t)
	ctx := context.Background()
	r := h.Renderer(frr.WithSections(), frr.WithStateReaders())
	up := prefix + "f0"
	for _, desc := range []string{"a | include b", "a | b", "uplink|ISP-A"} {
		if _, err := r.Render(ctx, describe(t, up, desc)); !errors.Is(err, frr.ErrInput) {
			t.Errorf("description %q: Render err = %v, want ErrInput", desc, err)
		} else {
			t.Logf("H1: description %q rejected: %v", desc, err)
		}
	}
	// The framework backstop: a protocol section cannot emit "| " either.
	rr := h.Renderer(frr.WithSections(rawSection{name: "evil", lines: []string{"interface " + up, " description a | b", "exit"}}))
	if _, err := rr.Render(ctx, nil); !errors.Is(err, renderers.ErrUnsafe) {
		t.Errorf("section line with \"| \": err = %v, want ErrUnsafe", err)
	}
	for _, desc := range []string{`a ! b # c; exit; end`, `x ? y`, `exit-vrf`, `end`, `a\b`, "$(reboot) `id`", `"; rm -rf /`} {
		files, err := r.Render(ctx, describe(t, up, desc))
		if err != nil {
			t.Fatalf("%q: %v", desc, err)
		}
		if err := r.Validate(ctx, files); err != nil {
			t.Fatalf("%q: Validate: %v", desc, err)
		}
		if err := r.Apply(ctx, files); err != nil { // Apply includes the convergence check (H2)
			t.Fatalf("%q: Apply: %v", desc, err)
		}
		raw, err := r.ShowJSON(ctx, frr.ShowInterface) // default VRF: name → state
		if err != nil {
			t.Fatal(err)
		}
		var ifs map[string]struct {
			Description string `json:"description"`
		}
		if err := json.Unmarshal(raw, &ifs); err != nil {
			t.Fatal(err)
		}
		got := ifs[up].Description
		if got != desc {
			t.Errorf("description %q stored as %q", desc, got)
		}
		t.Logf("H1: description %q applied, stored verbatim %q, converged", desc, got)
	}
}

// H2: frr-reload.py exits 0 although FRR stored something else → Apply fails and rolls back.
func TestReviewH2NotConvergedLive(t *testing.T) {
	h, _, slot := startFRR(t)
	ctx := context.Background()
	pfx := fmt.Sprintf("10.%d.230.0/24", slot)
	good := h.Renderer(frr.WithSections(), frr.WithStateReaders())
	base, err := good.Render(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := good.Apply(ctx, base); err != nil {
		t.Fatal(err)
	}
	// "bl" abbreviates `blackhole`: FRR stores `ip route P blackhole`, so the rendered line
	// never matches the running config (M1 makes the framework reject it as an interface
	// name; a raw section can still emit it — exactly the class of silent mismatch H2 catches).
	bad := h.Renderer(frr.WithSections(rawSection{name: "raw", lines: []string{"ip route " + pfx + " bl"}}), frr.WithStateReaders())
	files, err := bad.Render(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = bad.Apply(ctx, files)
	if !errors.Is(err, frr.ErrDaemon) || !strings.Contains(err.Error(), "not converged") {
		t.Fatalf("Apply err = %v, want not converged", err)
	}
	t.Logf("H2: Apply returned: %v", err)
	st, err := good.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, rt := range st.StaticRoutes {
		if rt.Prefix == pfx {
			t.Errorf("rolled back route still in the RIB: %+v", rt)
		}
	}
	for _, l := range st.RunningConfig {
		if strings.Contains(l, pfx) {
			t.Errorf("rolled back line still in running-config: %q", l)
		}
	}
	if diff, err := good.DryRun(ctx, base); err != nil || diff != "" {
		t.Errorf("after rollback the previous config should be in place: diff=%q err=%v", diff, err)
	}
	t.Logf("H2: after rollback no %s in RIB or running-config; previous config converged", pfx)
}

// M2: a planted secret is in frr.conf and in FRR, and nowhere the renderer reports.
func TestReviewM2SecretLive(t *testing.T) {
	h, _, _ := startFRR(t)
	ctx := context.Background()
	resolver := frr.SecretResolverFunc(func(_ context.Context, ref string) (string, error) {
		if ref == "password/w12vty" {
			return plantedSecret, nil
		}
		return "", errors.New("unknown")
	})
	r := h.Renderer(frr.WithSections(secretSection{ref: "password/w12vty"}), frr.WithSecretResolver(resolver), frr.WithStateReaders())
	files, err := r.Render(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	diff, err := r.DryRun(ctx, files)
	if err != nil {
		t.Fatal(err)
	}
	seen = append(seen, "DryRun: "+diff)
	if err := r.Apply(ctx, files); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Ground truth: FRR itself holds the password (raw vtysh through the harness runner).
	rawOut, err := h.Runner.Run(ctx, renderers.Command{Path: frr.VtyshBin, Args: []string{
		"--config_dir", h.Paths.ConfDir, "--vty_socket", h.Paths.RunDir, "-N", h.Paths.Namespace, "-c", "show running-config"}})
	if err != nil || !strings.Contains(string(rawOut.Stdout), "password "+plantedSecret) {
		t.Fatalf("FRR does not hold the planted password (err=%v)", err)
	}
	msg, err := r.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(msg)
	seen = append(seen, "Retrieve: "+string(b))
	show, err := r.Show(ctx, frr.ShowRunningConfig)
	if err != nil {
		t.Fatal(err)
	}
	seen = append(seen, "Show: "+string(show))
	// An error path: the checker echoes the offending line, which carries the secret.
	broken := files.Redacted() // copy with the same paths …
	conf := files[h.Paths.ConfFile()]
	conf.Content = []byte(strings.Replace(string(conf.Content), "password "+plantedSecret, "password "+plantedSecret+" bogus-extra", 1))
	broken[h.Paths.ConfFile()] = conf
	if err := r.Validate(ctx, broken); err != nil {
		seen = append(seen, "Validate error: "+err.Error())
	} else {
		t.Error("Validate accepted the broken line")
	}
	logBytes, _ := os.ReadFile(h.Paths.ReloadLog)
	seen = append(seen, "frr-reload.log: "+string(logBytes))
	for _, s := range seen {
		if strings.Contains(s, plantedSecret) {
			t.Errorf("secret leaked: %.400s", s)
		}
	}
	for _, s := range seen {
		t.Logf("M2: %.300s", strings.ReplaceAll(s, "\n", " ⏎ "))
	}
}

// M3: ≥ 10k FRR static routes — Retrieve and the poller stay bounded and correct.
func TestReviewM3ManyRoutesLive(t *testing.T) {
	h, _, slot := startFRR(t)
	ctx := context.Background()
	const n = 10240
	statics := make([]any, 0, n)
	for i := range n {
		statics = append(statics, map[string]any{"frr": true, "blackhole": true,
			"prefix": fmt.Sprintf("10.%d.%d.%d/32", slot, 100+i/256, i%256)})
	}
	d := mustDoc(t, map[string]any{"routing": map[string]any{"static": statics}})
	r := h.Renderer(frr.WithSections(), frr.WithStateReaders())
	files, err := r.Render(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := r.Apply(ctx, files); err != nil {
		t.Fatalf("Apply %d routes: %v", n, err)
	}
	applyDur := time.Since(start)
	start = time.Now()
	st, err := r.State(ctx)
	if err != nil {
		t.Fatalf("State with %d routes: %v", n, err)
	}
	stateDur := time.Since(start)
	if len(st.StaticRoutes) != n || st.Summary["ipv4/default/static"] != n {
		t.Fatalf("State: %d static routes, summary %v; want %d", len(st.StaticRoutes), st.Summary, n)
	}
	if _, err := r.Retrieve(ctx); err != nil {
		t.Fatal(err)
	}
	p := r.NewPoller()
	start = time.Now()
	if _, err := p.Step(ctx); err != nil {
		t.Fatal(err)
	}
	pollDur := time.Since(start)
	raw, _ := r.ShowJSON(ctx, frr.ShowIPStaticAll)
	sum, _ := r.ShowJSON(ctx, frr.ShowIPSummaryAll)
	t.Logf("M3: %d FRR static routes: Apply %v (incl. convergence check), State %v (%d routes, static json %d bytes, bound %d), poll step %v (summary json %d bytes), summary ipv4/default/static=%d",
		n, applyDur.Round(time.Millisecond), stateDur.Round(time.Millisecond), len(st.StaticRoutes), len(raw), frr.MaxShowOutput,
		pollDur.Round(time.Millisecond), len(sum), st.Summary["ipv4/default/static"])
	// Remove them again (the harness would anyway; this also exercises a 10k-line delete diff).
	empty, _ := r.Render(ctx, nil)
	if err := r.Apply(ctx, empty); err != nil {
		t.Fatalf("Apply (remove %d): %v", n, err)
	}
}
