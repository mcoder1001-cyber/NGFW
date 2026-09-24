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
	sections []Section             // nil: framework + RegisteredSections() at Render time
	ifLines  []NamedInterfaceLines // nil: registeredInterfaceLines() at Render time (seam S2)
	readers  []StateReader
	resolver SecretResolver
	secrets  secretSet
}

var _ renderers.Renderer = (*Renderer)(nil)

// Option configures a Renderer.
type Option func(*Renderer)

// WithPaths overrides ProductPaths (tests: TestPaths(prefix)).
func WithPaths(p Paths) Option { return func(r *Renderer) { r.paths = p } }

// WithVersion sets the `frr version` line (default DefaultVersion).
func WithVersion(v string) Option { return func(r *Renderer) { r.version = v } }

// WithInterfaceMapper sets the VPP→Linux interface name mapping (default NoMapper: no
// interface has a Linux side until P12 passes the linux-cp mapping; tests use IdentityMapper).
func WithInterfaceMapper(m InterfaceMapper) Option { return func(r *Renderer) { r.mapIf = m } }

// WithSections replaces the globally registered protocol sections by extra (tests, or a
// renderer that must not pick up init()-time registrations). The framework sections are
// always present.
func WithSections(extra ...Section) Option {
	return func(r *Renderer) { r.sections = append([]Section{}, extra...) }
}

// WithSecretResolver sets the resolver sections use through RenderContext.Secret (D-051).
func WithSecretResolver(sr SecretResolver) Option { return func(r *Renderer) { r.resolver = sr } }

// WithStateReaders replaces the globally registered state readers by extra.
func WithStateReaders(extra ...StateReader) Option {
	return func(r *Renderer) { r.readers = append([]StateReader{}, extra...) }
}

// New returns an FRR renderer running its commands through runner (production:
// frr.NewSystemRunner(), which carries the documented output bound MaxShowOutput).
func New(runner renderers.Runner, opts ...Option) *Renderer {
	r := &Renderer{runner: runner, paths: ProductPaths(), version: DefaultVersion, mapIf: NoMapper}
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
// No I/O except the injected secret resolver. desired is a *vrxv1.DesiredState or (D-055
// stand-in) a *structpb.Struct holding the configuration document. When a section resolved a
// secret, frr.conf is marked Secret (Files.Redacted hides it) and the value is remembered so
// every later output of this renderer masks it.
func (r *Renderer) Render(ctx context.Context, desired proto.Message) (renderers.Files, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	ds, ext, err := Desired(desired)
	if err != nil {
		return nil, err
	}
	model, err := BuildModel(ds, ext, r.mapIf)
	if err != nil {
		return nil, err
	}
	rc := &RenderContext{Ctx: ctx, Input: desired, Desired: ds, Ext: ext, mapIf: r.mapIf, resolver: r.resolver, model: model}
	ifLines := r.ifLines
	if ifLines == nil {
		ifLines = registeredInterfaceLines()
	}
	if err := mergeInterfaceLines(model, ifLines, rc); err != nil {
		return nil, redactErr(err, &r.secrets)
	}
	conf, err := assemble(r.allSections(), rc)
	if err != nil {
		return nil, redactErr(err, &r.secrets)
	}
	secrets := rc.secretValues()
	r.secrets.add(secrets...)
	vtysh, err := renderVtyshConf()
	if err != nil {
		return nil, err
	}
	files := renderers.Files{
		r.paths.ConfFile():  {Mode: r.paths.FileMode, Owner: r.paths.FileOwner, Content: conf, Secret: len(secrets) > 0},
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
		return fmt.Errorf("%w: vtysh -C rejected frr.conf: %s", ErrDaemon, r.toolMessage(out, err, st.Dir))
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
		return "", fmt.Errorf("%w: frr-reload.py --test: %s", ErrDaemon, r.toolMessage(out, err, st.Dir))
	}
	return r.secrets.redact(NormalizeDiff(string(out.Stdout))), nil
}

// Apply implements renderers.Renderer: snapshot → atomic write of frr.conf and vtysh.conf →
// `frr-reload.py --reload` (diff applied through vtysh; the daemons keep running) →
// convergence check → on any failure restore the snapshot and reload it.
//
// Convergence (RF-1 review H2): frr-reload.py feeds the added lines to `vtysh -f`, which
// carries on after a line the daemon rejects and exits 0. So after a successful reload Apply
// runs the same `--test` diff against the rendered files; anything left is a failure
// ("not converged") and is rolled back like a failed reload.
//
// Idempotent: applying the running config again is a no-op diff. Not safe for concurrent use
// on one set of paths: the commit engine serialises commits (one Apply at a time).
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
	applyErr := r.reload(ctx)
	if applyErr == nil {
		diff, err := r.DryRun(ctx, files)
		switch {
		case err != nil:
			applyErr = fmt.Errorf("frr: convergence check: %w", err)
		case diff != "":
			applyErr = fmt.Errorf("%w: not converged after frr-reload.py --reload (FRR did not take these lines):\n%s", ErrDaemon, diff)
		default:
			return nil
		}
	}
	// Roll back with a context of its own: the failure may have been the caller's deadline.
	rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), reloadTimeout)
	defer cancel()
	restoreErr := snap.Restore()
	if _, statErr := os.Stat(r.paths.ConfFile()); statErr == nil {
		if rbErr := r.reload(rbCtx); rbErr != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("frr: reloading the previous config: %w", rbErr))
		}
	}
	// With no previous frr.conf (never the case in the product: the package ships one) the
	// snapshot restore removes the file and nothing is reloaded: the daemons keep what was
	// applied until the next successful Apply.
	return errors.Join(applyErr, restoreErr)
}

func (r *Renderer) reload(ctx context.Context) error {
	out, err := r.runner.Run(ctx, renderers.Command{
		Path: ReloadBin, Args: r.paths.reloadArgs("--reload", r.paths.ConfFile()), Timeout: reloadTimeout,
	})
	if err != nil {
		return fmt.Errorf("%w: frr-reload.py --reload: %s", ErrDaemon, r.toolMessage(out, err, ""))
	}
	return nil
}

// toolMessage condenses a failed tool run into one line for the error (stdout first: vtysh
// -C prints "line N: % Unknown command" there), with the staging dir path shortened.
func (r *Renderer) toolMessage(out renderers.Output, err error, stagingDir string) string {
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
	msg = r.secrets.redact(strings.Join(strings.Fields(msg), " "))
	if len(msg) > 1024 {
		msg = msg[:1024] + "..."
	}
	return msg
}

// NormalizeDiff keeps the "Lines To Delete" / "Lines To Add" part of frr-reload.py --test
// output (dropping log noise) and returns "" when both lists are empty. `hostname` lines are
// dropped: FRR 10.7 daemons always report the system hostname and ignore the command (the
// system renderer owns the hostname), so frr-reload.py shows a hostname delta it can never
// apply — it must neither fail the convergence check nor show up in DryRun.
func NormalizeDiff(out string) string {
	lines := strings.Split(strings.ReplaceAll(out, "\r", ""), "\n")
	start := -1
	for i, l := range lines {
		if t := strings.TrimSpace(l); t == "Lines To Delete" || t == "Lines To Add" {
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
		if t == "" || strings.HasPrefix(t, "hostname ") || strings.HasPrefix(t, "no hostname ") {
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
