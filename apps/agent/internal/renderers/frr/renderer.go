// Package frr is the FRR renderer framework (RF-1, WBS D3.2): desired state → validated
// frr.conf + vtysh.conf → applied through FRR's own control channel (frr-reload.py, a diff
// against the running config, never a daemon restart) → state read back from `vtysh … json`
// → change events. It renders the framework sections (globals, vrf, interface description,
// static routes); routing protocols plug in through RegisterSection / RegisterStateReader /
// RegisterPoller from their own packages (P12, F-*). See README.md.
package frr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/renderers"
)

// Timeouts for the FRR tools (frr-reload.py spawns one vtysh per daemon).
const (
	validateTimeout = 30 * time.Second
	reloadTimeout   = 120 * time.Second
)

// ErrDaemon is wrapped by errors that come from FRR (checker rejected the file, reload
// failed, daemon not reachable).
var ErrDaemon = errors.New("frr: daemon error")

// Renderer implements renderers.Renderer for FRR.
type Renderer struct {
	runner   renderers.Runner
	paths    Paths
	version  string
	mapIf    InterfaceMapper
	sections []Section // nil: framework + RegisteredSections() at Render time
	readers  []StateReader
}

var _ renderers.Renderer = (*Renderer)(nil)

// Option configures a Renderer.
type Option func(*Renderer)

// WithPaths overrides ProductPaths (tests: TestPaths(prefix)).
func WithPaths(p Paths) Option { return func(r *Renderer) { r.paths = p } }

// WithVersion sets the `frr version` line (default DefaultVersion).
func WithVersion(v string) Option { return func(r *Renderer) { r.version = v } }

// WithInterfaceMapper sets the VPP→Linux interface name mapping (default IdentityMapper;
// P12 passes the linux-cp mapping).
func WithInterfaceMapper(m InterfaceMapper) Option { return func(r *Renderer) { r.mapIf = m } }

// WithSections replaces the globally registered protocol sections by extra (tests, or a
// renderer that must not pick up init()-time registrations). The framework sections are
// always present.
func WithSections(extra ...Section) Option {
	return func(r *Renderer) { r.sections = append([]Section{}, extra...) }
}

// WithStateReaders replaces the globally registered state readers by extra.
func WithStateReaders(extra ...StateReader) Option {
	return func(r *Renderer) { r.readers = append([]StateReader{}, extra...) }
}

// New returns an FRR renderer running its commands through runner (production:
// renderers.NewSystemRunner(renderers.NewAllowlist(frr.Binaries()...))).
func New(runner renderers.Runner, opts ...Option) *Renderer {
	r := &Renderer{runner: runner, paths: ProductPaths(), version: DefaultVersion, mapIf: IdentityMapper}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Name implements renderers.Renderer.
func (r *Renderer) Name() string { return "frr" }

// Paths returns the paths this renderer uses.
func (r *Renderer) Paths() Paths { return r.paths }

func (r *Renderer) allSections() []Section {
	extra := r.sections
	if extra == nil {
		extra = RegisteredSections()
	}
	return append(frameworkSections(r.version, r.mapIf), extra...)
}

func (r *Renderer) check() error {
	if r.runner == nil {
		return errors.New("frr: renderer has no runner")
	}
	if err := r.paths.Validate(); err != nil {
		return err
	}
	if !versionRe.MatchString(r.version) {
		return fmt.Errorf("frr: version %q must match %s", r.version, versionRe)
	}
	return nil
}

// Render implements renderers.Renderer: the complete frr.conf (all sections) and vtysh.conf.
// Pure: no I/O. desired is a *vrxv1.DesiredState or (D-055 stand-in) a *structpb.Struct
// holding the configuration document.
func (r *Renderer) Render(_ context.Context, desired proto.Message) (renderers.Files, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	conf, err := assemble(r.allSections(), desired)
	if err != nil {
		return nil, err
	}
	vtysh, err := renderVtyshConf()
	if err != nil {
		return nil, err
	}
	files := renderers.Files{
		r.paths.ConfFile():  {Mode: r.paths.FileMode, Owner: r.paths.FileOwner, Content: conf},
		r.paths.VtyshConf(): {Mode: r.paths.FileMode, Owner: r.paths.FileOwner, Content: vtysh},
	}
	return files, files.Validate()
}

// checkFiles verifies files is exactly this renderer's file set.
func (r *Renderer) checkFiles(files renderers.Files) error {
	if err := files.Validate(); err != nil {
		return err
	}
	want := map[string]bool{r.paths.ConfFile(): true, r.paths.VtyshConf(): true}
	for p := range files {
		if !want[p] {
			return fmt.Errorf("%w: frr renderer does not own %s", renderers.ErrInvalidFiles, p)
		}
	}
	if _, ok := files[r.paths.ConfFile()]; !ok {
		return fmt.Errorf("%w: %s missing", renderers.ErrInvalidFiles, r.paths.ConfFile())
	}
	return nil
}

// Validate implements renderers.Renderer: `vtysh -C -f <staged frr.conf>` against a staged
// copy (config dir included, so the staged vtysh.conf is read, never the live one). The
// checker parses every line against vtysh's full command tree (all daemons are compiled in),
// so it needs no running daemon. It cannot check semantic conflicts that only the daemon
// sees at commit time (e.g. a static route in a VRF the kernel does not have: accepted,
// reported by staticd as "not installed"); DryRun and Apply surface those.
func (r *Renderer) Validate(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	st, err := renderers.Stage(files)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	args := append(r.paths.vtyshArgs(st.Path(r.paths.ConfDir)), "-C", "-f", st.Path(r.paths.ConfFile()))
	out, err := r.runner.Run(ctx, renderers.Command{Path: VtyshBin, Args: args, Timeout: validateTimeout})
	if err != nil {
		return fmt.Errorf("%w: vtysh -C rejected frr.conf: %s", ErrDaemon, toolMessage(out, err, st.Dir))
	}
	return nil
}

// DryRun returns frr-reload.py's view of what Apply would change (`--test` against the
// running daemons: "Lines To Delete" / "Lines To Add"), for the commit engine's DryRun. It
// reads the running config only and never changes it. The daemons must be running.
func (r *Renderer) DryRun(ctx context.Context, files renderers.Files) (string, error) {
	if err := r.check(); err != nil {
		return "", err
	}
	if err := r.checkFiles(files); err != nil {
		return "", err
	}
	st, err := renderers.Stage(files)
	if err != nil {
		return "", err
	}
	defer func() { _ = st.Close() }()
	out, err := r.runner.Run(ctx, renderers.Command{
		Path: ReloadBin, Args: r.paths.reloadArgs("--test", st.Path(r.paths.ConfFile())), Timeout: reloadTimeout,
	})
	if err != nil {
		return "", fmt.Errorf("%w: frr-reload.py --test: %s", ErrDaemon, toolMessage(out, err, st.Dir))
	}
	return NormalizeDiff(string(out.Stdout)), nil
}

// Apply implements renderers.Renderer: snapshot → atomic write of frr.conf and vtysh.conf →
// `frr-reload.py --reload` (diff applied through vtysh; the daemons keep running) → on any
// failure restore the snapshot and reload it. Idempotent: applying the running config again
// is a no-op diff.
func (r *Renderer) Apply(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	if info, err := os.Stat(r.paths.ConfSubdir()); err != nil || !info.IsDir() {
		return fmt.Errorf("frr: config directory %s is missing (packaging/harness creates it): %v", r.paths.ConfSubdir(), err)
	}
	snap, err := renderers.TakeSnapshot(files.Paths()...)
	if err != nil {
		return err
	}
	if err := renderers.WriteFiles(files); err != nil {
		return errors.Join(err, snap.Restore())
	}
	reload := renderers.Command{
		Path: ReloadBin, Args: r.paths.reloadArgs("--reload", r.paths.ConfFile()), Timeout: reloadTimeout,
	}
	out, err := r.runner.Run(ctx, reload)
	if err == nil {
		return nil
	}
	applyErr := fmt.Errorf("%w: frr-reload.py --reload: %s", ErrDaemon, toolMessage(out, err, ""))
	restoreErr := snap.Restore()
	if _, statErr := os.Stat(r.paths.ConfFile()); statErr == nil {
		if _, rbErr := r.runner.Run(ctx, reload); rbErr != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("frr: reloading the previous config: %w", rbErr))
		}
	}
	return errors.Join(applyErr, restoreErr)
}

// toolMessage condenses a failed tool run into one line for the error (stdout first: vtysh
// -C prints "line N: % Unknown command" there), with the staging dir path shortened.
func toolMessage(out renderers.Output, err error, stagingDir string) string {
	var parts []string
	for _, b := range [][]byte{out.Stdout, out.Stderr} {
		if s := strings.TrimSpace(string(bytes.ToValidUTF8(b, []byte("?")))); s != "" {
			parts = append(parts, s)
		}
	}
	msg := strings.Join(parts, " | ")
	if msg == "" {
		msg = err.Error()
	}
	if stagingDir != "" {
		msg = strings.ReplaceAll(msg, stagingDir, "<staging>")
	}
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > 1024 {
		msg = msg[:1024] + "..."
	}
	return msg
}

// NormalizeDiff keeps the "Lines To Delete" / "Lines To Add" part of frr-reload.py --test
// output (dropping log noise) and returns "" when both lists are empty.
func NormalizeDiff(out string) string {
	lines := strings.Split(strings.ReplaceAll(out, "\r", ""), "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "Lines To Delete" {
			start = i
			break
		}
	}
	if start < 0 {
		return strings.TrimSpace(out)
	}
	var kept []string
	content := false
	for _, l := range lines[start:] {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		kept = append(kept, strings.TrimRight(l, " "))
		if t != "Lines To Delete" && t != "Lines To Add" && strings.Trim(t, "=") != "" {
			content = true
		}
	}
	if !content {
		return ""
	}
	return strings.Join(kept, "\n") + "\n"
}
