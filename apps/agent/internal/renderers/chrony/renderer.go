// Package chrony is the chrony renderer (RF-3, WBS D7.4; services.ntp, D-050): desired
// state → chrony.conf + sources.d/vrx.sources + chrony.keys (text/template + strict escaping)
// → validated with `chronyd -p` → applied with `chronyc reload sources` / `chronyc rekey`
// over the unix command socket, or a typed restart request for any other change → state
// read back with `chronyc -c sources | sourcestats | tracking | serverstats` → change events
// by 1 Hz polling. See README.md.
package chrony

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/renderers"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var tmpl = template.Must(renderers.NewTemplate("chrony").Funcs(template.FuncMap{
	"path":   safePath,
	"host":   safeHost,
	"hexkey": hexKey,
}).ParseFS(templateFS, "templates/*.tmpl"))

const (
	validateTimeout = 30 * time.Second
	controlTimeout  = 15 * time.Second
)

// ErrDaemon is wrapped by errors that come from chrony (checker, chronyc).
var ErrDaemon = errors.New("chrony: daemon error")

// ErrNotRunning is returned when the command socket does not exist.
var ErrNotRunning = errors.New("chrony: daemon not running")

func safePath(s string) (string, error) {
	if !safePathRe.MatchString(s) {
		return "", fmt.Errorf("%w: path %q", renderers.ErrUnsafe, s)
	}
	return s, nil
}

func safeHost(s string) (string, error) {
	if a, err := netip.ParseAddr(s); err == nil {
		return renderers.Addr(a.String())
	}
	if !hostnameRe.MatchString(s) {
		return "", fmt.Errorf("%w: host %q", renderers.ErrUnsafe, s)
	}
	return renderers.Ident(s)
}

var hexKeyRe = regexp.MustCompile(`^[0-9A-F]{2,256}$`)

func hexKey(s string) (string, error) {
	if !hexKeyRe.MatchString(s) {
		return "", fmt.Errorf("%w: key is not upper-case hex", renderers.ErrUnsafe) // never echo the key
	}
	return s, nil
}

// Renderer implements renderers.Renderer for chrony.
type Renderer struct {
	runner  renderers.Runner
	paths   Paths
	secrets SecretResolver
}

var _ renderers.Renderer = (*Renderer)(nil)

// Option configures a Renderer.
type Option func(*Renderer)

// WithPaths overrides ProductPaths (tests: TestPaths(prefix, instance)).
func WithPaths(p Paths) Option { return func(r *Renderer) { r.paths = p } }

// WithSecrets sets the resolver for key references (servers[].keyRef).
func WithSecrets(s SecretResolver) Option { return func(r *Renderer) { r.secrets = s } }

// New returns a chrony renderer running its commands through runner (production:
// NewRunner()).
func New(runner renderers.Runner, opts ...Option) *Renderer {
	r := &Renderer{runner: runner, paths: ProductPaths()}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Name implements renderers.Renderer.
func (r *Renderer) Name() string { return "chrony" }

// Paths returns the paths this renderer uses.
func (r *Renderer) Paths() Paths { return r.paths }

func (r *Renderer) check() error {
	if r.runner == nil {
		return errors.New("chrony: renderer has no runner")
	}
	return r.paths.Validate()
}

// Render implements renderers.Renderer: chrony.conf, sources.d/vrx.sources and chrony.keys
// (Secret, KeyMode). Pure apart from the secret resolver lookup.
func (r *Renderer) Render(_ context.Context, desired proto.Message) (renderers.Files, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	in, err := extract(desired)
	if err != nil {
		return nil, err
	}
	rd, err := r.build(in)
	if err != nil {
		return nil, err
	}
	conf, err := renderers.ExecuteTemplate(tmpl, "chrony.conf.tmpl", rd.conf)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	src, err := renderers.ExecuteTemplate(tmpl, "vrx.sources.tmpl", rd.sources)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	keys, err := renderers.ExecuteTemplate(tmpl, "chrony.keys.tmpl", rd.keys)
	if err != nil {
		return nil, fmt.Errorf("%w: chrony.keys: %w", ErrInvalid, renderers.ErrUnsafe) // no detail: secret file
	}
	files := renderers.Files{
		r.paths.Conf():    {Mode: r.paths.FileMode, Owner: r.paths.FileOwner, Content: conf},
		r.paths.Sources(): {Mode: r.paths.FileMode, Owner: r.paths.FileOwner, Content: src},
		r.paths.Keys():    {Mode: r.paths.KeyMode, Owner: r.paths.KeyOwner, Content: keys, Secret: true},
	}
	return files, files.Validate()
}

func (r *Renderer) checkFiles(files renderers.Files) error {
	if err := files.Validate(); err != nil {
		return err
	}
	own := map[string]bool{r.paths.Conf(): true, r.paths.Sources(): true, r.paths.Keys(): true}
	for p := range files {
		if !own[p] {
			return fmt.Errorf("%w: chrony renderer does not own %s", renderers.ErrInvalidFiles, p)
		}
	}
	if _, ok := files[r.paths.Conf()]; !ok {
		return fmt.Errorf("%w: %s missing", renderers.ErrInvalidFiles, r.paths.Conf())
	}
	if k, ok := files[r.paths.Keys()]; ok && !k.Secret {
		return fmt.Errorf("%w: %s must be marked Secret", renderers.ErrInvalidFiles, r.paths.Keys())
	}
	return nil
}

var chronycErrRe = regexp.MustCompile(`^5[0-9]{2} `)

var keyLineRe = regexp.MustCompile(`^[1-9][0-9]{0,9} SHA256 HEX:[0-9A-F]{2,256}$`)

// Validate implements renderers.Renderer: `chronyd -p -f <staged chrony.conf>` and
// `chronyd -p -f <staged vrx.sources>` (chrony ≥ 4: parse every directive, print, exit — no
// socket, no clock access; the sources file holds server/pool directives that are also valid
// in chrony.conf). chrony.keys has no checker: it is validated structurally here, and its
// content never reaches an error message.
func (r *Renderer) Validate(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	if k, ok := files[r.paths.Keys()]; ok {
		for i, l := range strings.Split(strings.TrimRight(string(k.Content), "\n"), "\n") {
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			if !keyLineRe.MatchString(l) {
				return fmt.Errorf("%w: chrony.keys line %d is malformed", ErrDaemon, i+1)
			}
		}
	}
	st, err := renderers.Stage(files)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	for _, p := range []string{r.paths.Conf(), r.paths.Sources()} {
		if _, ok := files[p]; !ok {
			continue
		}
		out, err := r.runner.Run(ctx, renderers.Command{Path: ChronydBin, Args: []string{"-p", "-f", st.Path(p)}, Timeout: validateTimeout})
		if err != nil {
			return fmt.Errorf("%w: chronyd -p rejected %s: %s", ErrDaemon, p[strings.LastIndex(p, "/")+1:], toolMessage(out, err, st.Dir))
		}
	}
	return nil
}

// ActionRequired is returned by Apply when the files were written but chronyd must be
// (re)started by the caller for them to take effect: chrony reloads only its sources
// (`reload sources`) and keys (`rekey`) at run time; every other chrony.conf directive needs a
// restart. It is not a failure: the files are not rolled back.
type ActionRequired struct {
	Daemon, Unit, Action, Reason string
}

func (e *ActionRequired) Error() string {
	return fmt.Sprintf("chrony: %s needs %s (%s): %s", e.Daemon, e.Action, e.Unit, e.Reason)
}

// NeedsRestart reports the unit and action (shared duck-typed shape with kea and unbound).
func (e *ActionRequired) NeedsRestart() (unit, action string) { return e.Unit, e.Action }

// running probes the command socket (a stale socket file survives a chronyd that was killed).
func (r *Renderer) running() bool {
	conn, err := net.DialTimeout("unixgram", r.paths.Socket(), time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Chronyc runs one fixed chronyc command over the unix command socket.
func (r *Renderer) Chronyc(ctx context.Context, args ...string) ([]byte, error) {
	if !r.running() {
		return nil, fmt.Errorf("%w: %s", ErrNotRunning, r.paths.Socket())
	}
	out, err := r.runner.Run(ctx, renderers.Command{
		Path: ChronycBin, Args: append([]string{"-h", r.paths.Socket()}, args...), Timeout: controlTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: chronyc %s: %s", ErrDaemon, strings.Join(args, " "), toolMessage(out, err, ""))
	}
	return out.Stdout, nil
}

// Apply implements renderers.Renderer: snapshot → atomic write → if chronyd runs:
// chrony.conf unchanged → `chronyc rekey` when the keys changed and `chronyc reload sources`
// when the sources changed; chrony.conf changed → *ActionRequired{Action: "restart"}. If
// chronyd does not run and the service is enabled → *ActionRequired{Action: "start"}. A
// failing chronyc restores the snapshot and repeats the reload with the old files.
func (r *Renderer) Apply(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	changed := map[string]bool{}
	for _, p := range files.Paths() {
		old, err := os.ReadFile(p) //nolint:gosec // own file set
		changed[p] = err != nil || !bytes.Equal(old, files[p].Content)
	}
	snap, err := renderers.TakeSnapshot(files.Paths()...)
	if err != nil {
		return err
	}
	if err := renderers.WriteFiles(files); err != nil {
		return errors.Join(err, snap.Restore())
	}
	enabled := !bytes.Contains(files[r.paths.Conf()].Content, []byte("\n# services.ntp is disabled"))
	if !r.running() {
		if enabled {
			return &ActionRequired{Daemon: "chronyd", Unit: r.paths.Unit, Action: "start", Reason: "services.ntp is enabled but chronyd is not running"}
		}
		return nil
	}
	if changed[r.paths.Conf()] {
		return &ActionRequired{Daemon: "chronyd", Unit: r.paths.Unit, Action: "restart", Reason: "chrony.conf changed (only sources and keys reload at run time)"}
	}
	var cmds [][]string
	if changed[r.paths.Keys()] {
		cmds = append(cmds, []string{"rekey"})
	}
	if changed[r.paths.Sources()] {
		cmds = append(cmds, []string{"reload", "sources"})
	}
	for _, c := range cmds {
		if _, err := r.Chronyc(ctx, c...); err != nil {
			restoreErr := snap.Restore()
			var again []error
			for _, c2 := range cmds {
				if _, e := r.Chronyc(ctx, c2...); e != nil && !errors.Is(e, ErrNotRunning) {
					again = append(again, e)
				}
			}
			return errors.Join(append([]error{err, restoreErr}, again...)...)
		}
	}
	return nil
}

// Retrieve implements renderers.Renderer: the State as a structpb.Struct (the proto has no
// NTP state message yet: RF-3-questions Q2).
func (r *Renderer) Retrieve(ctx context.Context) (proto.Message, error) {
	st, err := r.State(ctx)
	if err != nil {
		return nil, err
	}
	return toStruct(st)
}

func toolMessage(out renderers.Output, err error, stagingDir string) string {
	msg := bytes.TrimSpace(append(append([]byte{}, out.Stderr...), out.Stdout...))
	if stagingDir != "" {
		msg = bytes.ReplaceAll(msg, []byte(stagingDir), nil)
	}
	// chronyd -p echoes the parsed file on stdout; keep the error line(s) only.
	var keep [][]byte
	for _, l := range bytes.Split(msg, []byte("\n")) {
		if bytes.Contains(l, []byte("Fatal")) || bytes.Contains(l, []byte("rror")) || chronycErrRe.Match(l) {
			keep = append(keep, l)
		}
	}
	if len(keep) > 0 {
		msg = bytes.Join(keep, []byte("; "))
	}
	if len(msg) > 1024 {
		msg = append(msg[:1024], "..."...)
	}
	if len(msg) == 0 {
		return err.Error()
	}
	return string(msg)
}
