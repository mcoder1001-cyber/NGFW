package subsystems

// RV-A R1 M3: the keepalived stage (subsystems/keepalived.go) had no test. These exercise the stage's
// own logic on the fake VPP path — no real keepalived daemon — with a RecordingRunner for the
// `keepalived -t` validation and a recording controller for the reload.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/keepalived"
	"ngfw/agent/internal/renderers/rfkit"
	"ngfw/agent/internal/scheduler"
)

// fakeKeepalived is a keepalived controller: it records reloads and answers the JSON-dump signal by
// writing the instances of the live keepalived.conf to DumpDir (what the real daemon does), so Apply's
// convergence check passes without a daemon. It never shells out.
type fakeKeepalived struct {
	paths   keepalived.Paths
	reloads int
}

func (c *fakeKeepalived) Reload(context.Context) error  { c.reloads++; return nil }
func (c *fakeKeepalived) Restart(context.Context) error { return nil }
func (c *fakeKeepalived) Signal(context.Context, syscall.Signal) error {
	conf, _ := os.ReadFile(c.paths.ConfFile) //nolint:gosec // test temp dir
	var items []string
	var name, ifname string
	var vrid, prio int
	flush := func() {
		if name != "" {
			items = append(items, fmt.Sprintf(`{"data":{"iname":%q,"ifp_ifname":%q,"vrid":%d,"base_priority":%d,"effective_priority":%d,"state":2,"version":3},"stats":{}}`, name, ifname, vrid, prio, prio))
		}
		name = ""
	}
	for _, l := range strings.Split(string(conf), "\n") {
		f := strings.Fields(l)
		switch {
		case len(f) >= 2 && f[0] == "vrrp_instance":
			flush()
			name = f[1]
		case len(f) == 2 && f[0] == "interface":
			ifname = f[1]
		case len(f) == 2 && f[0] == "virtual_router_id":
			vrid, _ = strconv.Atoi(f[1])
		case len(f) == 2 && f[0] == "priority":
			prio, _ = strconv.Atoi(f[1])
		}
	}
	flush()
	return os.WriteFile(filepath.Join(c.paths.DumpDir, "keepalived.json"), []byte("["+strings.Join(items, ",")+"]"), 0o600)
}

// newStage builds a KeepalivedStage over a temp dir with rec as the runner (`keepalived -t`) and a fake
// daemon controller.
func newStage(t *testing.T, rec renderers.Runner) (*KeepalivedStage, *keepalived.Renderer, *fakeKeepalived) {
	t.Helper()
	dir := t.TempDir()
	p := keepalived.TestPaths("w7", dir, "")
	p.ConfFile = filepath.Join(dir, "keepalived.conf")
	p.StateDir = filepath.Join(dir, "state")
	p.DumpDir = filepath.Join(dir, "tmp")
	if err := os.MkdirAll(p.DumpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	ctl := &fakeKeepalived{paths: p}
	// No WithInterfaceMapper: the stage passes the value's own mapping to every render (RenderWith), so the
	// renderer's default NoMapper must never be consulted.
	r := keepalived.New(rec, keepalived.WithPaths(p), keepalived.WithController(ctl), keepalived.WithJSONSignal(36),
		keepalived.WithVerifyTimeout(2*time.Second))
	return NewKeepalivedStage(r, filepath.Join(dir, "keepalived-w7.json")), r, ctl
}

// stageValue is the keepalived.config value desired.Vrrp builds: one keepalived instance plus its pair.
func stageValue(t *testing.T, prio int) *vrxv1.DesiredState {
	t.Helper()
	v := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(fmt.Sprintf(`{
	  "interfaces": {"loop7301": {"lcp": {"hostIfName": "lan0"}}},
	  "ha": {"vrrp": {"vi": {"interface": "loop7301", "vrId": 20, "engine": "keepalived", "priority": %d, "addresses": ["10.7.4.1"]}}}
	}`, prio)), v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestKeepalivedStageLifecycleDrift (RV-A R1 M3): an instance through the stage — Create validates with
// `keepalived -t -f <staged>`, writes the config bound to the Linux interface, reloads and records;
// Retrieve round-trips it; a drifted live config is reported absent; Update re-applies a changed value;
// Delete writes the empty rendering and removes the record.
func TestKeepalivedStageLifecycleDrift(t *testing.T) {
	rec := renderers.NewRecordingRunner().Succeed(keepalived.KeepalivedBin, "")
	st, r, ctl := newStage(t, rec)
	ctx := context.Background()
	v1 := stageValue(t, 150)

	if _, err := st.Create(ctx, v1); err != nil {
		t.Fatal(err)
	}
	var checks int
	for _, c := range rec.Calls() {
		if c.Path == keepalived.KeepalivedBin && len(c.Args) >= 3 && c.Args[0] == "-t" && c.Args[1] == "-f" {
			if c.Args[2] == r.Paths().ConfFile {
				t.Fatalf("the checker must run on a staged copy, not the live file: %v", c.Args)
			}
			checks++
		}
	}
	if checks != 1 {
		t.Fatalf("want one `keepalived -t -f <staged>` before the write, calls %+v", rec.Calls())
	}
	conf, err := os.ReadFile(r.Paths().ConfFile)
	if err != nil || !strings.Contains(string(conf), "interface lan0") || !strings.Contains(string(conf), "virtual_router_id 20") ||
		strings.Contains(string(conf), "loop7301") {
		t.Fatalf("keepalived.conf must bind the Linux interface (not the VPP name): %v\n%s", err, conf)
	}
	if ctl.reloads != 1 {
		t.Fatalf("Create reloaded %d times, want 1", ctl.reloads)
	}
	if _, err := os.Stat(st.record); err != nil {
		t.Fatalf("no record after Create: %v", err)
	}
	kvs, err := st.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || kvs[0].Key != desired.KeepalivedKey || !proto.Equal(kvs[0].Value, v1) {
		t.Fatalf("Retrieve after Create: %v %v", kvs, err)
	}

	// drift: the live config no longer equals the record's rendering → reported absent (the resync re-applies)
	if err := os.WriteFile(r.Paths().ConfFile, []byte("! drifted\n"), 0o640); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	if kvs, _ := st.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("drift not detected: %v", kvs)
	}

	// Update re-applies a changed value
	v2 := stageValue(t, 120)
	if _, err := st.Update(ctx, v1, v2, nil); err != nil {
		t.Fatal(err)
	}
	if kvs, _ := st.Retrieve(ctx); len(kvs) != 1 || !proto.Equal(kvs[0].Value, v2) {
		t.Fatalf("Retrieve after Update: %v", kvs)
	}
	if conf, _ := os.ReadFile(r.Paths().ConfFile); !strings.Contains(string(conf), "priority 120") {
		t.Fatalf("Update did not re-render:\n%s", conf)
	}

	// Delete writes the empty rendering and removes the record
	if err := st.Delete(ctx, v2, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(st.record); !os.IsNotExist(err) {
		t.Fatalf("Delete left the record: %v", err)
	}
	if conf, _ := os.ReadFile(r.Paths().ConfFile); strings.Contains(string(conf), "vrrp_instance") {
		t.Fatalf("Delete left an instance:\n%s", conf)
	}
	if kvs, _ := st.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("Retrieve after Delete: %v", kvs)
	}
}

// TestKeepalivedStageCheckerRefuses: a failing `keepalived -t` fails the Create before anything is written —
// no keepalived.conf, no reload, no record.
func TestKeepalivedStageCheckerRefuses(t *testing.T) {
	rec := renderers.NewRecordingRunner().FailWith(keepalived.KeepalivedBin, 1, "(line 3) unknown keyword")
	st, r, ctl := newStage(t, rec)
	if _, err := st.Create(context.Background(), stageValue(t, 150)); err == nil {
		t.Fatal("a refused configuration must fail the Create")
	}
	if _, err := os.Stat(r.Paths().ConfFile); !os.IsNotExist(err) {
		t.Fatalf("a refused configuration was written: %v", err)
	}
	if _, err := os.Stat(st.record); !os.IsNotExist(err) {
		t.Fatalf("a refused configuration was recorded: %v", err)
	}
	if ctl.reloads != 0 {
		t.Fatalf("a refused configuration reloaded keepalived %d times", ctl.reloads)
	}
}

// TestKeepalivedStageMapper: render binds the Linux side of the value's linux-cp pairs (RF-4 NoMapper rule:
// never the VPP name) and the mapping is local to the call (RV-A R4 n1): two values with different pairs
// rendered concurrently each see only their own, and the renderer's own mapper stays NoMapper.
func TestKeepalivedStageMapper(t *testing.T) {
	st, r, _ := newStage(t, renderers.NewRecordingRunner())
	ctx := context.Background()
	conf := func(v *vrxv1.DesiredState) string {
		files, err := st.render(ctx, v)
		if err != nil {
			t.Error(err)
			return ""
		}
		return string(files[r.Paths().ConfFile].Content)
	}
	if c := conf(stageValue(t, 150)); !strings.Contains(c, "interface lan0") || strings.Contains(c, "loop7301") {
		t.Fatalf("keepalived.conf must bind the Linux interface:\n%s", c)
	}
	if _, err := r.Render(ctx, stageValue(t, 150)); err == nil {
		t.Fatal("the renderer's own mapper must stay NoMapper (the stage never mutates it)")
	}
	a, b := stageValue(t, 150), stageValue(t, 150)
	b.Interfaces["loop7301"].Lcp.HostIfName = proto.String("lan1")
	var wg sync.WaitGroup
	for range 8 {
		for _, c := range []struct {
			v           *vrxv1.DesiredState
			want, other string
		}{{a, "interface lan0", "lan1"}, {b, "interface lan1", "lan0"}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if got := conf(c.v); !strings.Contains(got, c.want) || strings.Contains(got, c.other) {
					t.Errorf("mapping leaked between concurrent renders: want %s\n%s", c.want, got)
				}
			}()
		}
	}
	wg.Wait()
}

// TestKeepalivedStageValidator (TD-13, S-keepalived-validator): the stage is a StageDaemon Validator. Validate
// runs `keepalived -t -f <staged copy>` on a temp dir that is gone afterwards, with `dynamic_interfaces` in the
// copy only; it writes no keepalived.conf, no record, reloads nothing; a refusal is the checker's message.
func TestKeepalivedStageValidator(t *testing.T) {
	if got := st0().Stage(); got != scheduler.StageDaemon {
		t.Fatalf("Stage() = %v, want daemon", got)
	}
	var staged string
	var stagedContent []byte
	rec := renderers.NewRecordingRunner().On(keepalived.KeepalivedBin, func(c renderers.Command) (renderers.Output, error) {
		if len(c.Args) < 3 || c.Args[0] != "-t" || c.Args[1] != "-f" {
			return renderers.Output{}, fmt.Errorf("unexpected argv %v", c.Args)
		}
		staged = c.Args[2]
		var err error
		stagedContent, err = os.ReadFile(staged) //nolint:gosec // the staged path the renderer chose
		return renderers.Output{}, err
	})
	st, r, ctl := newStage(t, rec)
	ctx := context.Background()
	v := stageValue(t, 150)
	if err := st.Validate(ctx, desired.KeepalivedKey, v, nil); err != nil {
		t.Fatal(err)
	}
	if staged == "" || staged == r.Paths().ConfFile || !strings.HasPrefix(staged, os.TempDir()+string(os.PathSeparator)) {
		t.Fatalf("the checker must run on a staged copy under the temp dir, got %q", staged)
	}
	if _, err := os.Stat(filepath.Dir(staged)); !os.IsNotExist(err) {
		t.Fatalf("staging dir %s must be removed after Validate: %v", filepath.Dir(staged), err)
	}
	c := string(stagedContent)
	if strings.Count(c, "dynamic_interfaces") != 1 || !strings.Contains(c, "global_defs {\n    dynamic_interfaces\n") ||
		!strings.Contains(c, "interface lan0") {
		t.Fatalf("staged copy must carry dynamic_interfaces once, in global_defs:\n%s", c)
	}
	files, err := st.render(ctx, v)
	if err != nil || strings.Contains(string(files[r.Paths().ConfFile].Content), "dynamic_interfaces") {
		t.Fatalf("the rendering itself must not carry dynamic_interfaces: %v", err)
	}
	if _, err := os.Stat(r.Paths().ConfFile); !os.IsNotExist(err) {
		t.Fatalf("Validate wrote keepalived.conf: %v", err)
	}
	if _, err := os.Stat(st.record); !os.IsNotExist(err) {
		t.Fatalf("Validate wrote the record: %v", err)
	}
	if ctl.reloads != 0 {
		t.Fatalf("Validate reloaded keepalived %d times", ctl.reloads)
	}
	if kvs, err := st.Retrieve(ctx); err != nil || len(kvs) != 0 {
		t.Fatalf("Retrieve after Validate must still be empty: %v %v", kvs, err)
	}

	// refused: the checker's message reaches the finding, wrapped in ErrDaemon
	st2, _, _ := newStage(t, renderers.NewRecordingRunner().FailWith(keepalived.KeepalivedBin, 5, "(keepalived.conf: Line 9) Unknown keyword 'bogus'"))
	err = st2.Validate(ctx, desired.KeepalivedKey, v, nil)
	if !errors.Is(err, keepalived.ErrDaemon) || !strings.Contains(err.Error(), "Unknown keyword 'bogus'") {
		t.Fatalf("refusal must carry the checker's message: %v", err)
	}
	// an unrenderable value (no linux-cp pair) is a finding too, without running the checker
	bad := stageValue(t, 150)
	bad.Interfaces["loop7301"].Lcp = nil
	rec.Reset()
	if err := st.Validate(ctx, desired.KeepalivedKey, bad, nil); err == nil || len(rec.Calls()) != 0 {
		t.Fatalf("an unrenderable value must be refused before the checker: %v %v", err, rec.Calls())
	}
	if err := st.Validate(ctx, desired.KeepalivedKey, &vrxv1.SnmpService{}, nil); err == nil {
		t.Fatal("a foreign value must be refused")
	}
}

func st0() *KeepalivedStage { return &KeepalivedStage{} }

// execRunner runs a fake keepalived binary (a script) in place of KeepalivedBin, with the renderer's argv.
type execRunner struct{ bin string }

func (e execRunner) Run(ctx context.Context, cmd renderers.Command) (renderers.Output, error) {
	if cmd.Path != keepalived.KeepalivedBin {
		return renderers.Output{}, fmt.Errorf("unexpected binary %s", cmd.Path)
	}
	c := exec.CommandContext(ctx, e.bin, cmd.Args...) //nolint:gosec // the test's own script
	var so, se bytes.Buffer
	c.Stdout, c.Stderr = &so, &se
	err := c.Run()
	return renderers.Output{Stdout: so.Bytes(), Stderr: se.Bytes()}, err
}

// TestKeepalivedStageValidatorFakeBinary: the same through a fake keepalived executable — it sees the staged
// file with dynamic_interfaces and the Linux interface, accepts priority 150 and refuses 120 with a message
// the finding carries; the staged file is gone afterwards and nothing was written.
func TestKeepalivedStageValidatorFakeBinary(t *testing.T) {
	dir := t.TempDir()
	seen := filepath.Join(dir, "seen.txt")
	bin := filepath.Join(dir, "keepalived")
	script := `#!/bin/sh
[ "$1" = -t ] && [ "$2" = -f ] || { echo "bad argv: $*" >&2; exit 2; }
[ -f "$3" ] || { echo "missing $3" >&2; exit 2; }
echo "$3" > ` + seen + `
grep -q '^    dynamic_interfaces$' "$3" || { echo "no dynamic_interfaces" >&2; exit 2; }
grep -q '^    interface lan0$' "$3" || { echo "no interface lan0" >&2; exit 2; }
grep -q '^    priority 150$' "$3" && exit 0
echo "($3: Line 12) fake keepalived refuses this priority" >&2
exit 5
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil { //nolint:gosec // executable test script
		t.Fatal(err)
	}
	st, r, ctl := newStage(t, execRunner{bin})
	ctx := context.Background()
	if err := st.Validate(ctx, desired.KeepalivedKey, stageValue(t, 150), nil); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(seen) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	staged := strings.TrimSpace(string(raw))
	if _, err := os.Stat(staged); !os.IsNotExist(err) || staged == r.Paths().ConfFile { //nolint:gosec // the path the fake checker reported
		t.Fatalf("staged copy %s must be gone after Validate: %v", staged, err)
	}
	err = st.Validate(ctx, desired.KeepalivedKey, stageValue(t, 120), nil)
	if !errors.Is(err, keepalived.ErrDaemon) || !strings.Contains(err.Error(), "fake keepalived refuses this priority") ||
		strings.Contains(err.Error(), dir) {
		t.Fatalf("refusal must carry the checker's message with the staging path masked: %v", err)
	}
	if _, err := os.Stat(r.Paths().ConfFile); !os.IsNotExist(err) {
		t.Fatalf("Validate wrote keepalived.conf: %v", err)
	}
	if ctl.reloads != 0 {
		t.Fatalf("Validate reloaded keepalived %d times", ctl.reloads)
	}
}

type stoppedKeepalived struct{ *fakeKeepalived }

func (*stoppedKeepalived) MainPID(context.Context) (int, error) { return 0, nil }

func TestKeepalivedStoppedDaemonFindingBeforeApply(t *testing.T) {
	stage, r, ctl := newStage(t, renderers.NewRecordingRunner().Succeed(keepalived.KeepalivedBin, ""))
	stopped := keepalived.New(renderers.NewRecordingRunner().Succeed(keepalived.KeepalivedBin, ""), keepalived.WithPaths(r.Paths()), keepalived.WithController(&stoppedKeepalived{ctl}))
	stage.r = stopped
	value := stageValue(t, 150)
	err := stage.Validate(context.Background(), desired.KeepalivedKey, value, nil)
	if !errors.Is(err, rfkit.ErrNotRunning) || !strings.Contains(err.Error(), "slot harnesses start it") {
		t.Fatalf("DryRun finding: %v", err)
	}
	if _, err := stage.Create(context.Background(), value); !errors.Is(err, rfkit.ErrNotRunning) || !strings.Contains(err.Error(), "slot harnesses start it") {
		t.Fatalf("Apply finding: %v", err)
	}
	if ctl.reloads != 0 {
		t.Fatal("attempted reload of stopped daemon")
	}
	if _, err := os.Stat(r.Paths().ConfFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrote daemon config before readiness: %v", err)
	}
}
