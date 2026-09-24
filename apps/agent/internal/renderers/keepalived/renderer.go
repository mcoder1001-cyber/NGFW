// Package keepalived is the keepalived renderer (RF-4, WBS D9.1 — the VRRP path next to VPP's
// native plugin): ha.vrrp instances with engine "keepalived" → keepalived.conf (VRRPv2 PASS
// keys resolved from D-051 references at render time) → `keepalived -t` → applied with
// SIGHUP (`systemctl reload keepalived`) → convergence proven by keepalived's own JSON dump →
// state from the notify helper's state files plus the dump → 1 Hz transition events. See
// README.md.
package keepalived

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"text/template"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rfkit"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// ErrDaemon is wrapped by errors that come from keepalived (checker, reload, convergence).
var ErrDaemon = errors.New("keepalived: daemon error")

const (
	checkTimeout  = 30 * time.Second
	verifyTimeout = 10 * time.Second
)

// Renderer implements renderers.Renderer for keepalived.
type Renderer struct {
	runner        renderers.Runner
	ctl           rfkit.Controller
	paths         Paths
	resolver      rfkit.SecretResolver
	mapper        InterfaceMapper
	checks        map[string]bool
	red           rfkit.Redactor
	verifyTimeout time.Duration
	tmpl          *template.Template

	dumpMu  sync.Mutex // one SIGJSON + read at a time
	jsonSig int        // cached `keepalived --signum=JSON`

	cacheMu    sync.Mutex
	lastDump   []DumpInstance
	lastDumpAt time.Time
}

var _ renderers.Renderer = (*Renderer)(nil)

// Option configures a Renderer.
type Option func(*Renderer)

// WithPaths overrides ProductPaths (tests: TestPaths).
func WithPaths(p Paths) Option { return func(r *Renderer) { r.paths = p } }

// WithController replaces `systemctl reload|kill keepalived` (tests: the child's PID).
func WithController(c rfkit.Controller) Option { return func(r *Renderer) { r.ctl = c } }

// WithSecretResolver sets the resolver of the D-051 references (VRRPv2 PASS keys).
func WithSecretResolver(sr rfkit.SecretResolver) Option { return func(r *Renderer) { r.resolver = sr } }

// WithInterfaceMapper sets the VPP → Linux interface mapping (default NoMapper).
func WithInterfaceMapper(m InterfaceMapper) Option { return func(r *Renderer) { r.mapper = m } }

// WithChecks names the shipped check executables (in Paths.ChecksDir) a vrrp_script may use.
func WithChecks(names ...string) Option {
	return func(r *Renderer) {
		for _, n := range names {
			r.checks[n] = true
		}
	}
}

// WithVerifyTimeout bounds the post-reload convergence check (default 10 s).
func WithVerifyTimeout(d time.Duration) Option { return func(r *Renderer) { r.verifyTimeout = d } }

// WithJSONSignal fixes the SIGJSON number instead of asking `keepalived --signum=JSON`.
func WithJSONSignal(n int) Option { return func(r *Renderer) { r.jsonSig = n } }

// New returns a keepalived renderer (production runner:
// renderers.NewSystemRunner(renderers.NewAllowlist(keepalived.Binaries()...))).
func New(runner renderers.Runner, opts ...Option) *Renderer {
	r := &Renderer{runner: runner, paths: ProductPaths(), mapper: NoMapper, checks: map[string]bool{}, verifyTimeout: verifyTimeout}
	for _, o := range opts {
		o(r)
	}
	if r.ctl == nil {
		r.ctl = &rfkit.SystemdController{Runner: runner, Unit: "keepalived"}
	}
	r.tmpl = template.Must(renderers.NewTemplate("keepalived").Funcs(funcs()).ParseFS(templateFS, "templates/*.tmpl"))
	return r
}

// Name implements renderers.Renderer.
func (r *Renderer) Name() string { return "keepalived" }

// Paths returns the paths this renderer uses.
func (r *Renderer) Paths() Paths { return r.paths }

// Redact masks every secret this renderer resolved or read back.
func (r *Renderer) Redact(s string) string { return r.red.Redact(s) }

func (r *Renderer) check() error {
	if r.runner == nil {
		return errors.New("keepalived: renderer has no runner")
	}
	return r.paths.Validate()
}

var advRe = regexp.MustCompile(`^[0-9]{1,2}(?:\.[0-9]{1,2})?$`)

func funcs() template.FuncMap {
	return template.FuncMap{
		"name": Name,
		"ifname": func(s string) (string, error) {
			if !ifnameRe.MatchString(s) {
				return "", fmt.Errorf("%w: interface %q", ErrInput, s)
			}
			return s, nil
		},
		"adv": func(s string) (string, error) {
			if !advRe.MatchString(s) {
				return "", fmt.Errorf("%w: advert_int %q", ErrInput, s)
			}
			return s, nil
		},
		"pass": func(s string) (string, error) {
			if !authPassRe.MatchString(s) {
				return "", fmt.Errorf("%w: VRRP PASS key not usable", ErrInput)
			}
			return s, nil
		},
		"path": func(s string) (string, error) {
			if !pathRe.MatchString(s) || filepath.Clean(s) != s {
				return "", fmt.Errorf("%w: path %q", ErrInput, s)
			}
			return s, nil
		},
	}
}

// Render implements renderers.Renderer: keepalived.conf for the enabled ha.vrrp instances
// whose engine is "keepalived" (the "vpp" ones belong to the VPP vrrp descriptor, DF-7).
// No I/O except the secret resolver.
func (r *Renderer) Render(ctx context.Context, desired proto.Message) (renderers.Files, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	ds, ext, err := rfkit.Decode(desired)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	sec := &rfkit.Secrets{Ctx: ctx, Resolver: r.resolver}
	model, err := BuildModel(ds, ext, sec, Options{Paths: r.paths, Mapper: r.mapper, Checks: r.checks})
	r.red.Add(sec.Values()...)
	if err != nil {
		return nil, r.red.Error(err)
	}
	content, err := renderers.ExecuteTemplate(r.tmpl, "keepalived.conf.tmpl", model)
	if err != nil {
		return nil, r.red.Error(err)
	}
	files := renderers.Files{
		r.paths.ConfFile: {Mode: r.paths.FileMode, Owner: r.paths.FileOwner, Content: content, Secret: len(sec.Values()) > 0},
	}
	return files, files.Validate()
}

func (r *Renderer) checkFiles(files renderers.Files) error {
	if err := files.Validate(); err != nil {
		return err
	}
	if _, ok := files[r.paths.ConfFile]; !ok || len(files) != 1 {
		return fmt.Errorf("%w: keepalived renderer owns exactly %s", renderers.ErrInvalidFiles, r.paths.ConfFile)
	}
	return nil
}

// Validate implements renderers.Renderer: `keepalived -t -f <staged keepalived.conf>` (plus
// `-s <netns>` when keepalived runs in a namespace: -t checks that the interfaces exist, and
// that the notify helper and check scripts are present and secure). Never the live file.
func (r *Renderer) Validate(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	r.red.Add(parseRendered(files[r.paths.ConfFile].Content).secrets...)
	st, err := renderers.Stage(files)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	args := []string{"-t", "-f", st.Path(r.paths.ConfFile)}
	if r.paths.NetNS != "" {
		args = append(args, "-s", r.paths.NetNS)
	}
	out, err := r.runner.Run(ctx, renderers.Command{Path: KeepalivedBin, Args: args, Timeout: checkTimeout})
	if err != nil {
		out.Stdout, out.Stderr = []byte(r.red.Redact(string(out.Stdout))), []byte(r.red.Redact(string(out.Stderr)))
		return r.red.Error(fmt.Errorf("%w: keepalived -t rejected keepalived.conf: %s", ErrDaemon, toolMessage(out, err, st.Dir)))
	}
	return nil
}

func toolMessage(out renderers.Output, err error, stagingDir string) string {
	var parts []string
	for _, b := range [][]byte{out.Stderr, out.Stdout} {
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

// Apply implements renderers.Renderer: state dir → snapshot → atomic write → SIGHUP reload
// (keepalived re-reads its configuration in place) → convergence: keepalived's JSON dump
// lists exactly the rendered instances with the rendered interface, VRID and base priority →
// on failure restore and reload. After success, state files of instances that no longer
// exist are removed. Not safe for concurrent use: the commit engine serialises commits.
func (r *Renderer) Apply(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	if err := os.MkdirAll(r.paths.StateDir, 0o755); err != nil { //nolint:gosec // state files are not secret; readable by the agent's API side
		return err
	}
	want := parseRendered(files[r.paths.ConfFile].Content)
	r.red.Add(want.secrets...)
	verify := func(ctx context.Context) error {
		if len(want.instances) == 0 {
			return nil // keepalived has no VRRP child to report on (README)
		}
		return rfkit.Poll(ctx, r.verifyTimeout, 250*time.Millisecond, func(ctx context.Context) error {
			dump, err := r.dump(ctx)
			if err != nil {
				return fmt.Errorf("%w: %w: %v", ErrDaemon, rfkit.ErrNotConverged, err)
			}
			return want.matches(dump)
		})
	}
	r.dropDumpCache()
	defer r.dropDumpCache()
	if err := rfkit.ApplyFiles(ctx, files, r.ctl.Reload, verify); err != nil {
		return r.red.Error(err)
	}
	r.pruneStateFiles(want)
	return nil
}

func (r *Renderer) pruneStateFiles(want rendered) {
	ents, err := os.ReadDir(r.paths.StateDir)
	if err != nil {
		return
	}
	for _, e := range ents {
		name, ok := strings.CutSuffix(e.Name(), ".state")
		if ok && e.Type().IsRegular() && !slices.ContainsFunc(want.instances, func(i renderedInstance) bool { return i.name == name }) {
			_ = os.Remove(filepath.Join(r.paths.StateDir, e.Name()))
		}
	}
}
