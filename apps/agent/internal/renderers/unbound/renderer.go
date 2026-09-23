// Package unbound is the Unbound renderer (RF-3, WBS D7.3): services.dns.resolvers → a
// validated unbound.conf (text/template + strict escaping) → applied with
// `unbound-control reload_keep_cache` over the unix control socket → state read back with
// `unbound-control status | stats_noreset | list_forwards | list_stubs | list_local_zones |
// list_local_data` → change events by 1 Hz polling. See README.md.
package unbound

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/renderers"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var tmpl = template.Must(renderers.NewTemplate("unbound.conf.tmpl").Funcs(template.FuncMap{
	"yesno":    yesno,
	"rr":       rrQuote,
	"sockaddr": sockaddr,
	"fwdaddr":  fwdaddr,
}).ParseFS(templateFS, "templates/*.tmpl"))

const (
	validateTimeout = 30 * time.Second
	controlTimeout  = 30 * time.Second
)

// ErrDaemon is wrapped by errors that come from Unbound (checker, reload, control).
var ErrDaemon = errors.New("unbound: daemon error")

// ErrNotRunning is returned when the control socket does not exist.
var ErrNotRunning = errors.New("unbound: daemon not running")

func yesno(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// rrQuote wraps one resource record in single quotes for local-data. The record was built
// from validated parts (TXT data is \DDD-escaped), so it cannot contain a quote, a bare
// backslash or `include:`; this checks it once more.
func rrQuote(s string) (string, error) {
	if _, err := renderers.Line(s); err != nil {
		return "", err
	}
	if strings.Contains(s, "'") || !backslashOK.MatchString(s) || strings.Contains(strings.ToLower(s), "include:") {
		return "", fmt.Errorf("%w: record %q", renderers.ErrUnsafe, s)
	}
	return "'" + s + "'", nil
}

// backslashOK: backslashes appear only as \DDD escapes (TXT data).
var backslashOK = regexp.MustCompile(`^[^\\]*(?:\\[0-9]{3}[^\\]*)*$`)

var fwdAddrRe = regexp.MustCompile(`^([0-9A-Fa-f:.]+)@([0-9]{1,5})(#[a-z0-9.-]{1,253})?$`)

// sockaddr re-validates "<ip>@<port>".
func sockaddr(s string) (string, error) {
	ip, port, ok := strings.Cut(s, "@")
	a, err := netip.ParseAddr(ip)
	p, perr := strconv.ParseUint(port, 10, 16)
	if !ok || err != nil || a.Zone() != "" || perr != nil || p == 0 {
		return "", fmt.Errorf("%w: listen address %q", renderers.ErrUnsafe, s)
	}
	return a.String() + "@" + strconv.FormatUint(p, 10), nil
}

// fwdaddr re-validates "<ip>@<port>[#<tls name>]".
func fwdaddr(s string) (string, error) {
	m := fwdAddrRe.FindStringSubmatch(s)
	if m == nil {
		return "", fmt.Errorf("%w: forward address %q", renderers.ErrUnsafe, s)
	}
	if _, err := sockaddr(m[1] + "@" + m[2]); err != nil {
		return "", err
	}
	return s, nil
}

// Renderer implements renderers.Renderer for Unbound.
type Renderer struct {
	runner renderers.Runner
	paths  Paths
}

var _ renderers.Renderer = (*Renderer)(nil)

// Option configures a Renderer.
type Option func(*Renderer)

// WithPaths overrides ProductPaths (tests: TestPaths(prefix)).
func WithPaths(p Paths) Option { return func(r *Renderer) { r.paths = p } }

// New returns an Unbound renderer running its commands through runner (production:
// NewRunner()).
func New(runner renderers.Runner, opts ...Option) *Renderer {
	r := &Renderer{runner: runner, paths: ProductPaths()}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Name implements renderers.Renderer.
func (r *Renderer) Name() string { return "unbound" }

// Paths returns the paths this renderer uses.
func (r *Renderer) Paths() Paths { return r.paths }

func (r *Renderer) check() error {
	if r.runner == nil {
		return errors.New("unbound: renderer has no runner")
	}
	return r.paths.Validate()
}

// Render implements renderers.Renderer: the complete unbound.conf. Pure: no I/O.
func (r *Renderer) Render(_ context.Context, desired proto.Message) (renderers.Files, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	in, err := extract(desired)
	if err != nil {
		return nil, err
	}
	data, err := r.build(in)
	if err != nil {
		return nil, err
	}
	conf, err := renderers.Execute(tmpl, data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	files := renderers.Files{r.paths.Conf(): {Mode: r.paths.FileMode, Owner: r.paths.FileOwner, Content: conf}}
	return files, files.Validate()
}

// Active reports whether a rendered unbound.conf serves at least one resolver.
func Active(conf []byte) bool { return !bytes.Contains(conf, []byte("\n# no enabled resolver")) }

func (r *Renderer) checkFiles(files renderers.Files) error {
	if err := files.Validate(); err != nil {
		return err
	}
	for p := range files {
		if p != r.paths.Conf() {
			return fmt.Errorf("%w: unbound renderer does not own %s", renderers.ErrInvalidFiles, p)
		}
	}
	if _, ok := files[r.paths.Conf()]; !ok {
		return fmt.Errorf("%w: %s missing", renderers.ErrInvalidFiles, r.paths.Conf())
	}
	return nil
}

// Validate implements renderers.Renderer: `unbound-checkconf <staged unbound.conf>`. The
// checker parses every directive, the local-data records and the view/forward blocks; it
// also requires the trust-anchor file and directories named in the config to exist.
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
	out, err := r.runner.Run(ctx, renderers.Command{Path: CheckconfBin, Args: []string{st.Path(r.paths.Conf())}, Timeout: validateTimeout})
	if err != nil {
		return fmt.Errorf("%w: unbound-checkconf: %s", ErrDaemon, toolMessage(out, err, st.Dir))
	}
	return nil
}

// ActionRequired is returned by Apply when the file was written but the daemon must be
// started or stopped by the caller (not an error to roll back).
type ActionRequired struct {
	Daemon, Unit, Action, Reason string
}

func (e *ActionRequired) Error() string {
	return fmt.Sprintf("unbound: %s needs %s (%s): %s", e.Daemon, e.Action, e.Unit, e.Reason)
}

// NeedsRestart reports the unit and action (shared duck-typed shape with kea and chrony).
func (e *ActionRequired) NeedsRestart() (unit, action string) { return e.Unit, e.Action }

// Control runs one fixed unbound-control command against the running daemon.
func (r *Renderer) Control(ctx context.Context, args ...string) ([]byte, error) {
	// A stale socket survives an unbound that exited: probe it before running unbound-control.
	conn, err := net.DialTimeout("unix", r.paths.ControlSocket(), time.Second)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrNotRunning, r.paths.ControlSocket(), err)
	}
	_ = conn.Close()
	out, err := r.runner.Run(ctx, renderers.Command{
		Path: ControlBin, Args: append([]string{"-c", r.paths.Conf()}, args...), Timeout: controlTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: unbound-control %s: %s", ErrDaemon, strings.Join(args, " "), toolMessage(out, err, ""))
	}
	return out.Stdout, nil
}

// Apply implements renderers.Renderer: snapshot → atomic write → `unbound-control
// reload_keep_cache` (Unbound ≥ 1.13: re-reads unbound.conf, keeps the cache when the cache
// settings did not change) → on failure restore the snapshot and reload it. A daemon that is
// not running is left alone when the configuration is idle and reported as *ActionRequired
// ("start") otherwise.
func (r *Renderer) Apply(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	snap, err := renderers.TakeSnapshot(files.Paths()...)
	if err != nil {
		return err
	}
	if err := renderers.WriteFiles(files); err != nil {
		return errors.Join(err, snap.Restore())
	}
	_, err = r.Control(ctx, "reload_keep_cache")
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotRunning):
		if Active(files[r.paths.Conf()].Content) {
			return &ActionRequired{Daemon: "unbound", Unit: "unbound", Action: "start", Reason: "configuration has resolvers but unbound is not running"}
		}
		return nil
	}
	restoreErr := snap.Restore()
	_, reloadErr := r.Control(ctx, "reload_keep_cache")
	if errors.Is(reloadErr, ErrNotRunning) {
		reloadErr = nil
	}
	return errors.Join(err, restoreErr, reloadErr)
}

// Retrieve implements renderers.Renderer: the State as a structpb.Struct (the proto has no
// DNS state message yet: RF-3-questions Q2).
func (r *Renderer) Retrieve(ctx context.Context) (proto.Message, error) {
	st, err := r.State(ctx)
	if err != nil {
		return nil, err
	}
	return toStruct(st)
}

func toolMessage(out renderers.Output, err error, stagingDir string) string {
	msg := bytes.TrimSpace(append(append([]byte{}, out.Stdout...), out.Stderr...))
	if stagingDir != "" {
		msg = bytes.ReplaceAll(msg, []byte(stagingDir), nil)
	}
	if len(msg) > 1024 {
		msg = append(msg[:1024], "..."...)
	}
	if len(msg) == 0 {
		return err.Error()
	}
	return string(msg)
}
