package frr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

// RF-1 review fixes: H2 (convergence), M2 (secrets), M3 (bounded state), L1, D-072.

// H2: frr-reload.py exits 0 but FRR did not take a line → Apply fails and rolls back.
func TestApplyFailsWhenNotConverged(t *testing.T) {
	p := tempPaths(t)
	old := []byte("frr version 10.7.1\n!\nend\n")
	if err := os.WriteFile(p.ConfFile(), old, 0o640); err != nil { //nolint:gosec // same mode as the renderer writes
		t.Fatal(err)
	}
	var modes []string
	rr := renderers.NewRecordingRunner().On(ReloadBin, func(c renderers.Command) (renderers.Output, error) {
		modes = append(modes, c.Args[0])
		if c.Args[0] == "--test" && len(modes) == 2 { // the convergence check right after the reload
			return renderers.Output{Stdout: []byte("Lines To Delete\n===============\nLines To Add\n============\nip route 10.12.230.0/24 bl\n")}, nil
		}
		return renderers.Output{}, nil
	})
	r := New(rr, WithPaths(p), WithSections(), WithInterfaceMapper(IdentityMapper))
	files, _ := r.Render(context.Background(), staticDoc(t))
	err := r.Apply(context.Background(), files)
	if !errors.Is(err, ErrDaemon) || !strings.Contains(err.Error(), "not converged") || !strings.Contains(err.Error(), "ip route 10.12.230.0/24 bl") {
		t.Fatalf("err = %v", err)
	}
	if want := []string{"--reload", "--test", "--reload"}; !slices.Equal(modes, want) {
		t.Fatalf("reload sequence = %v, want %v (reload, check, rollback reload)", modes, want)
	}
	if b, _ := os.ReadFile(p.ConfFile()); string(b) != string(old) {
		t.Error("frr.conf not restored after the convergence failure")
	}
}

func TestApplyConvergedIsSuccess(t *testing.T) {
	p := tempPaths(t)
	rr := renderers.NewRecordingRunner().Succeed(ReloadBin, "Lines To Delete\n===============\nLines To Add\n============\n")
	r := New(rr, WithPaths(p), WithSections(), WithInterfaceMapper(IdentityMapper))
	files, _ := r.Render(context.Background(), staticDoc(t))
	if err := r.Apply(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	if n := len(rr.Calls()); n != 2 {
		t.Fatalf("calls = %d, want reload + convergence check", n)
	}
}

// L2.1: the rollback reload runs even when the caller's context is already done.
func TestRollbackIgnoresCallerCancellation(t *testing.T) {
	p := tempPaths(t)
	if err := os.WriteFile(p.ConfFile(), []byte("end\n"), 0o640); err != nil { //nolint:gosec // test
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var rollbackCtxErr error
	calls := 0
	rr := renderers.NewRecordingRunner().On(ReloadBin, func(c renderers.Command) (renderers.Output, error) {
		calls++
		if calls == 1 {
			cancel() // the engine's deadline expires during the first reload
			out := renderers.Output{ExitCode: 1}
			return out, &renderers.ExitError{Command: c, Output: out}
		}
		return renderers.Output{}, nil
	})
	r := New(&ctxRecorder{Runner: rr, seen: &rollbackCtxErr}, WithPaths(p), WithSections(), WithInterfaceMapper(IdentityMapper))
	files, _ := r.Render(context.Background(), nil)
	if err := r.Apply(ctx, files); err == nil {
		t.Fatal("Apply succeeded")
	}
	if calls != 2 || rollbackCtxErr != nil {
		t.Fatalf("rollback reload: calls=%d ctx err=%v", calls, rollbackCtxErr)
	}
}

// ctxRecorder records the context error of the last call (RecordingRunner refuses done contexts).
type ctxRecorder struct {
	renderers.Runner
	seen *error
}

func (c *ctxRecorder) Run(ctx context.Context, cmd renderers.Command) (renderers.Output, error) {
	*c.seen = ctx.Err()
	return c.Runner.Run(ctx, cmd)
}

// ---------------------------------------------------------------- M2 secrets

const plantedSecret = "VRX_TEST_PSK_RF1" //nolint:gosec // test placeholder (00-CONTEXT fixture convention)

// secretSection is what P12 will do for `neighbor … password`: resolve a D-051 reference.
type secretSection struct{ ref string }

func (secretSection) Name() string { return "vty-password" }
func (secretSection) Order() int   { return 450 }
func (s secretSection) Render(rc *RenderContext) ([]string, error) {
	v, err := rc.Secret(s.ref)
	if err != nil {
		return nil, err
	}
	return []string{"password " + v}, nil
}

func resolverFor(value string) SecretResolver {
	return SecretResolverFunc(func(_ context.Context, ref string) (string, error) {
		if ref != "password/w12vty" {
			return "", fmt.Errorf("unknown secret %s", ref)
		}
		return value, nil
	})
}

func TestSecretNeverReturned(t *testing.T) {
	p := tempPaths(t)
	echo := "line 7: % Unknown command: password " + plantedSecret + "\n"
	rr := renderers.NewRecordingRunner().
		On(VtyshBin, func(c renderers.Command) (renderers.Output, error) {
			if slices.Contains(c.Args, "-C") { // the checker echoes the offending line
				out := renderers.Output{Stdout: []byte(echo), ExitCode: 2}
				return out, &renderers.ExitError{Command: c, Output: out}
			}
			answers := standardAnswers()
			answers[ShowRunningConfig] = rc + "password " + plantedSecret + "\n"
			answers["show bgp summary json"] = `{"note":"` + plantedSecret + `"}`
			return renderers.Output{Stdout: []byte(answers[ShowCommand(c.Args[len(c.Args)-1])])}, nil
		}).
		On(ReloadBin, func(c renderers.Command) (renderers.Output, error) {
			if c.Args[0] == "--test" {
				return renderers.Output{Stdout: []byte("Lines To Delete\n===============\nLines To Add\n============\npassword " + plantedSecret + "\n")}, nil
			}
			out := renderers.Output{Stderr: []byte("vtysh failed on: password " + plantedSecret), ExitCode: 1}
			return out, &renderers.ExitError{Command: c, Output: out}
		})
	r := New(rr, WithPaths(p), WithSections(secretSection{ref: "password/w12vty"}), WithSecretResolver(resolverFor(plantedSecret)),
		WithStateReaders(StateReader{Key: "bgpSummary", Command: "show bgp summary json"}))
	ctx := context.Background()
	files, err := r.Render(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	conf := files[p.ConfFile()]
	if !conf.Secret || !strings.Contains(string(conf.Content), "password "+plantedSecret) {
		t.Fatalf("frr.conf must carry the secret and be marked Secret (secret=%v)", conf.Secret)
	}
	if red := files.Redacted()[p.ConfFile()]; strings.Contains(string(red.Content), plantedSecret) {
		t.Error("Files.Redacted leaks")
	}
	var outputs []string
	if err := r.Validate(ctx, files); err != nil {
		outputs = append(outputs, "validate: "+err.Error())
	} else {
		t.Error("Validate: expected the planted checker error")
	}
	diff, err := r.DryRun(ctx, files)
	if err != nil {
		t.Fatal(err)
	}
	outputs = append(outputs, "dryrun: "+diff)
	if err := r.Apply(ctx, files); err != nil {
		outputs = append(outputs, "apply: "+err.Error())
	} else {
		t.Error("Apply: expected the planted reload error")
	}
	msg, err := r.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(msg)
	outputs = append(outputs, "retrieve: "+string(b))
	raw, err := r.Show(ctx, ShowRunningConfig)
	if err != nil {
		t.Fatal(err)
	}
	outputs = append(outputs, "show: "+string(raw))
	for _, o := range outputs {
		if strings.Contains(o, plantedSecret) {
			t.Errorf("secret leaked in %.300s", o)
		}
		if !strings.Contains(o, "redacted") { // json.Marshal escapes '<' '>'

			t.Errorf("no redaction marker in %.300s", o)
		}
	}
	// frr-reload.py never logs applied lines (it would write the secret to its log file).
	for _, c := range rr.Calls() {
		if c.Path == ReloadBin && (!slices.Contains(c.Args, "critical") || slices.Contains(c.Args, "info")) {
			t.Errorf("frr-reload.py log level must be critical: %q", c.Args)
		}
		if strings.Contains(c.String(), plantedSecret) {
			t.Errorf("secret in argv: %s", c)
		}
	}
}

func TestSecretPatternsWithoutResolvedValue(t *testing.T) {
	// A fresh renderer (e.g. after an agent restart) has resolved nothing yet: the FRR
	// patterns still mask what the daemon prints.
	for in, want := range map[string]string{ //nolint:gosec // fake secrets for the redaction patterns
		" neighbor 10.0.0.1 password k0":                 " neighbor 10.0.0.1 password <redacted>",
		"no neighbor 10.0.0.1 password k0":               "no neighbor 10.0.0.1 password <redacted>",
		" ip ospf authentication-key k1":                 " ip ospf authentication-key <redacted>",
		" ip ospf message-digest-key 1 md5 k2":           " ip ospf message-digest-key 1 md5 <redacted>",
		"  key-string k3":                                "  key-string <redacted>",
		" isis password md5 k4":                          " isis password md5 <redacted>",
		" area-password clear k5":                        " area-password clear <redacted>",
		" ip rip authentication string k6":               " ip rip authentication string <redacted>",
		"password k7":                                    "password <redacted>",
		"enable password k8":                             "enable password <redacted>",
		"ip route 10.0.0.0/8 blackhole":                  "ip route 10.0.0.0/8 blackhole",
		`{"x":"line 3: % Unknown command: password k9"}`: `{"x":"line 3: % Unknown command: password <redacted>"}`,
	} {
		if got := RedactPatterns(in); got != want {
			t.Errorf("RedactPatterns(%q) = %q, want %q", in, got, want)
		}
	}
	RegisterRedaction(regexp.MustCompile(`mpls ldp password (\S+)`))
	t.Cleanup(func() { redactMu.Lock(); redactions = redactions[:len(redactions)-1]; redactMu.Unlock() })
	if got := RedactPatterns("mpls ldp password k10"); got != "mpls ldp password <redacted>" {
		t.Errorf("registered redaction: %q", got)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("RegisterRedaction without a group did not panic")
			}
		}()
		RegisterRedaction(regexp.MustCompile(`password \S+`))
	}()
}

func TestSecretResolutionErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		ref   string
		value string
		opts  []Option
		want  string
	}{
		"bad ref":     {ref: "bgp-peer1", value: "x", want: "must match"},
		"no resolver": {ref: "password/w12vty", want: "no secret resolver"},
		"blank":       {ref: "password/w12vty", value: "a b", want: "not usable"},
		"pipe":        {ref: "password/w12vty", value: "a|b", want: "not usable"},
		"quote":       {ref: "password/w12vty", value: `a"b`, want: "not usable"},
		"newline":     {ref: "password/w12vty", value: "a\nrouter bgp 1", want: "not usable"},
	} {
		opts := []Option{WithSections(secretSection{ref: tc.ref})}
		if tc.value != "" {
			opts = append(opts, WithSecretResolver(resolverFor(tc.value)))
		}
		_, err := New(renderers.NewRecordingRunner(), opts...).Render(context.Background(), nil)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
		if err != nil && tc.value != "" && len(tc.value) > 2 && strings.Contains(err.Error(), tc.value) {
			t.Errorf("%s: error echoes the secret value", name)
		}
	}
}

// ---------------------------------------------------------------- M3 bounded state

func TestShowTruncationIsAnError(t *testing.T) {
	if NewSystemRunner().MaxOutput != MaxShowOutput || MaxShowOutput < 64<<20 {
		t.Error("production runner must carry MaxShowOutput")
	}
	if got := New(&renderers.SystemRunner{MaxOutput: 1234}).outputCap(); got != 1234 {
		t.Errorf("outputCap(SystemRunner) = %d", got)
	}
	big := `{"default":{` + strings.Repeat(" ", 300) + `}}`
	answers := standardAnswers()
	answers[ShowIPStaticAll] = big
	cr := &capRunner{Runner: showRunner(t, answers), limit: 128}
	r := New(cr, WithPaths(TestPaths("w12")))
	if _, err := r.Show(context.Background(), ShowIPStaticAll); !errors.Is(err, ErrTruncated) {
		t.Errorf("output at the bound: err = %v, want ErrTruncated", err)
	}
	if _, err := r.State(context.Background()); !errors.Is(err, ErrTruncated) {
		t.Errorf("State over truncated output: err = %v, want ErrTruncated", err)
	}
	cr.limit = 1 << 20
	if _, err := r.Show(context.Background(), ShowIPStaticAll); err != nil {
		t.Errorf("under the bound: %v", err)
	}
}

// capRunner imitates a bounded runner: output is cut at limit, and it reports the limit.
type capRunner struct {
	renderers.Runner
	limit int
}

func (c *capRunner) OutputLimit() int { return c.limit }

func (c *capRunner) Run(ctx context.Context, cmd renderers.Command) (renderers.Output, error) {
	out, err := c.Runner.Run(ctx, cmd)
	if len(out.Stdout) > c.limit {
		out.Stdout = out.Stdout[:c.limit]
	}
	return out, err
}

func TestStreamRIBShapesAndScale(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"default":{`)
	const n = 12000
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `"10.%d.%d.0/24":[{"prefix":"10.%d.%d.0/24","protocol":"static","vrfName":"default","distance":1,"installed":true,"nexthops":[{"blackhole":true,"active":true}]}]`,
			i/256, i%256, i/256, i%256)
	}
	b.WriteString(`},"w12red":{"10.12.210.0/24":[{"protocol":"static","nexthops":[]}]}}`)
	count, red := 0, 0
	err := StreamRIB(strings.NewReader(b.String()), func(rt RIBRoute) error {
		count++
		if rt.VRFName == "w12red" && rt.Prefix == "10.12.210.0/24" {
			red++
		}
		return nil
	})
	if err != nil || count != n+1 || red != 1 {
		t.Fatalf("StreamRIB: err=%v count=%d red=%d", err, count, red)
	}
	flat := `{"10.0.0.0/8":[{"protocol":"static"}]}`
	count = 0
	if err := StreamRIB(strings.NewReader(flat), func(RIBRoute) error { count++; return nil }); err != nil || count != 1 {
		t.Fatalf("flat: %v %d", err, count)
	}
	for _, bad := range []string{`[]`, `{"a":1}`, `{"a":[{"prefix":1}]}`, `{"a":{"b":{}}}`, `{`} {
		if err := StreamRIB(strings.NewReader(bad), func(RIBRoute) error { return nil }); err == nil {
			t.Errorf("StreamRIB(%q) accepted", bad)
		}
	}
}

func TestStateUsesScopedCommandsOnly(t *testing.T) {
	rr := showRunner(t, standardAnswers())
	r := New(rr, WithPaths(TestPaths("w12")), WithStateReaders())
	if _, err := r.Retrieve(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.NewPoller().Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, c := range rr.Calls() {
		cmd := ShowCommand(c.Args[len(c.Args)-1])
		if cmd == ShowIPRouteAll || cmd == ShowIPv6RouteAll || cmd == ShowIPRoute || cmd == ShowIPv6Route {
			t.Errorf("full RIB dump %q", cmd)
		}
	}
}

// ---------------------------------------------------------------- L1, D-072

func TestProductAllowlistHasNoTrampoline(t *testing.T) {
	for _, b := range Binaries() {
		if base := b[strings.LastIndex(b, "/")+1:]; b == "/usr/bin/ip" || base == "env" || base == "sh" || base == "bash" {
			t.Errorf("production allowlist contains %s (can exec arbitrary binaries)", b)
		}
	}
	for _, b := range NewSystemRunner().Allow.Paths() {
		if b == "/usr/bin/ip" {
			t.Error("NewSystemRunner allows /usr/bin/ip")
		}
	}
}

func TestStaticOwnership(t *testing.T) {
	d := staticDoc(t)
	ds, ext, err := Desired(d)
	if err != nil {
		t.Fatal(err)
	}
	var frrOwned, vppOwned int
	for i, sr := range ds.GetRouting().GetStatic() {
		if StaticOwnedByFRR(i, sr, ext) {
			frrOwned++
		} else {
			vppOwned++
		}
	}
	if frrOwned != 10 || vppOwned != 1 {
		t.Fatalf("frr=%d vpp=%d", frrOwned, vppOwned)
	}
	// Default for proto input without a flag: nothing is FRR's.
	if StaticOwnedByFRR(0, &vrxv1.StaticRoute{Prefix: ptr("10.0.0.0/8")}, nil) {
		t.Error("unflagged route owned by FRR")
	}
	// The hook for the real flag (P03b/P12): registered once.
	selectorMu.Lock()
	prev, prevSet := staticSelector, selectorSet
	selectorMu.Unlock()
	t.Cleanup(func() { selectorMu.Lock(); staticSelector, selectorSet = prev, prevSet; selectorMu.Unlock() })
	RegisterStaticSelector(func(_ int, sr *vrxv1.StaticRoute, _ *Extensions) bool { return sr.GetDistance() == 7 })
	if !StaticOwnedByFRR(0, &vrxv1.StaticRoute{Distance: ptr(uint32(7))}, nil) {
		t.Error("registered selector not used")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("second RegisterStaticSelector did not panic")
			}
		}()
		RegisterStaticSelector(FlaggedStatic)
	}()
}
