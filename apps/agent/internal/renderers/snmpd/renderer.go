// Package snmpd is the net-snmp renderer (RF-4, WBS D7.5): services.snmp → snmpd.conf
// (0600: communities and USM passphrases, resolved from D-051 secret references at render
// time) → validated by a daemon parse run → applied with SIGHUP (`systemctl reload snmpd` in
// the product) → state read back over SNMP itself (gosnmp against the local agent; the
// credentials never appear in any argv) → 1 Hz change events. See README.md.
package snmpd

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
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rfkit"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// ErrDaemon is wrapped by errors that come from snmpd (parse run rejected the file, reload or
// convergence failed).
var ErrDaemon = errors.New("snmpd: daemon error")

// Timeouts.
const (
	parseRunTimeout = 15 * time.Second
	verifyTimeout   = 5 * time.Second
	maxConfSize     = 1 << 20
	maxLogSize      = 256 << 10
)

// Renderer implements renderers.Renderer for snmpd.
type Renderer struct {
	runner        renderers.Runner
	ctl           rfkit.Controller
	paths         Paths
	resolver      rfkit.SecretResolver
	querier       Querier
	red           rfkit.Redactor
	verifyTimeout time.Duration
	tmpl          *template.Template
	listeners     func(pid int) ([]string, error)
}

var _ renderers.Renderer = (*Renderer)(nil)

// Option configures a Renderer.
type Option func(*Renderer)

// WithPaths overrides ProductPaths (tests: TestPaths(prefix)).
func WithPaths(p Paths) Option { return func(r *Renderer) { r.paths = p } }

// WithController replaces the product control channel (`systemctl reload snmpd`); tests pass
// an rfkit.ProcessController for the snmpd child they spawned.
func WithController(c rfkit.Controller) Option { return func(r *Renderer) { r.ctl = c } }

// WithSecretResolver sets the resolver of the D-051 references (communities, passphrases).
func WithSecretResolver(sr rfkit.SecretResolver) Option { return func(r *Renderer) { r.resolver = sr } }

// WithQuerier replaces the gosnmp client (unit tests).
func WithQuerier(q Querier) Option { return func(r *Renderer) { r.querier = q } }

// WithListeners replaces the /proc socket reader (unit tests).
func WithListeners(f func(pid int) ([]string, error)) Option {
	return func(r *Renderer) { r.listeners = f }
}

// WithVerifyTimeout bounds the post-reload convergence check (default 5 s).
func WithVerifyTimeout(d time.Duration) Option { return func(r *Renderer) { r.verifyTimeout = d } }

// New returns an snmpd renderer running its commands through runner (production:
// renderers.NewSystemRunner(renderers.NewAllowlist(snmpd.Binaries()...))).
func New(runner renderers.Runner, opts ...Option) *Renderer {
	r := &Renderer{runner: runner, paths: ProductPaths(), querier: GoSNMP{}, verifyTimeout: verifyTimeout, listeners: rfkit.UDPListeners}
	for _, o := range opts {
		o(r)
	}
	if r.ctl == nil {
		r.ctl = &rfkit.SystemdController{Runner: runner, Unit: "snmpd"}
	}
	r.tmpl = template.Must(renderers.NewTemplate("snmpd").Funcs(r.funcs()).ParseFS(templateFS, "templates/*.tmpl"))
	return r
}

// Name implements renderers.Renderer.
func (r *Renderer) Name() string { return "snmpd" }

// Paths returns the paths this renderer uses.
func (r *Renderer) Paths() Paths { return r.paths }

// Redact masks every secret this renderer resolved or read back.
func (r *Renderer) Redact(s string) string { return r.red.Redact(s) }

func (r *Renderer) check() error {
	if r.runner == nil {
		return errors.New("snmpd: renderer has no runner")
	}
	return r.paths.Validate()
}

// funcs are the strict per-daemon template helpers; each returns an error for a bad value.
func (r *Renderer) funcs() template.FuncMap {
	return template.FuncMap{
		"warn": func(s string) (string, error) {
			return renderers.Line(s)
		},
		"tok":  Token,
		"text": Text,
		"oid": func(s string) (string, error) {
			if !oidRe.MatchString(s) || !strings.HasPrefix(s, ".") {
				return "", fmt.Errorf("%w: rendered OID %q is not numeric", ErrInput, s)
			}
			return s, nil
		},
		"hex": func(s string) (string, error) {
			if !engineIDRe.MatchString(s) {
				return "", fmt.Errorf("%w: engine id is not hex", ErrInput)
			}
			return s, nil
		},
		"community": func(s string) (string, error) {
			if err := checkCommunity(s); err != nil {
				return "", fmt.Errorf("%w: %v", ErrInput, err)
			}
			return s, nil
		},
		"pass": func(s string) (string, error) {
			if err := checkPassphrase(s); err != nil {
				return "", fmt.Errorf("%w: %v", ErrInput, err)
			}
			return `"` + s + `"`, nil
		},
		"source": func(s string) (string, error) {
			if s == "default" {
				return s, nil
			}
			return renderers.Network(s)
		},
		"transport": func(s string) (string, error) {
			if !transportRe.MatchString(s) {
				return "", fmt.Errorf("%w: listen transport %q", ErrInput, s)
			}
			return s, nil
		},
		"host": func(s string) (string, error) {
			if !hostRe.MatchString(s) {
				return "", fmt.Errorf("%w: host %q", ErrInput, s)
			}
			return s, nil
		},
		"diskpath": func(s string) (string, error) {
			if !diskPathRe.MatchString(s) || filepath.Clean(s) != s {
				return "", fmt.Errorf("%w: disk path %q", ErrInput, s)
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

var (
	transportRe = regexp.MustCompile(`^(?:udp:[0-9.]{7,15}|udp6:\[[0-9a-f:.]{2,45}\]):[0-9]{1,5}$`)
	hostRe      = regexp.MustCompile(`^(?:[A-Za-z0-9.-]{1,253}|\[[0-9a-f:.]{2,45}\])$`)
)

// Render implements renderers.Renderer: snmpd.conf from services.snmp. desired is a
// *vrxv1.DesiredState or a *structpb.Struct holding the document (decoded into the typed message). No I/O
// except the injected secret resolver; the file is marked Secret whenever it carries one.
func (r *Renderer) Render(ctx context.Context, desired proto.Message) (renderers.Files, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	ds, ext, err := rfkit.Decode(desired)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInput, err)
	}
	sec := &rfkit.Secrets{Ctx: ctx, Resolver: r.resolver}
	model, err := BuildModel(ds, ext, sec, r.paths.AgentXSocket)
	r.red.Add(sec.Values()...)
	if err != nil {
		return nil, r.red.Error(err)
	}
	content, err := renderers.ExecuteTemplate(r.tmpl, "snmpd.conf.tmpl", model)
	if err != nil {
		return nil, r.red.Error(err)
	}
	files := renderers.Files{
		r.paths.ConfFile: {Mode: r.paths.FileMode, Owner: r.paths.FileOwner, Content: content, Secret: true},
	}
	return files, files.Validate()
}

// checkFiles verifies files is exactly this renderer's file set.
func (r *Renderer) checkFiles(files renderers.Files) error {
	if err := files.Validate(); err != nil {
		return err
	}
	if len(files) != 1 {
		return fmt.Errorf("%w: snmpd renderer owns exactly %s", renderers.ErrInvalidFiles, r.paths.ConfFile)
	}
	if _, ok := files[r.paths.ConfFile]; !ok {
		return fmt.Errorf("%w: %s missing", renderers.ErrInvalidFiles, r.paths.ConfFile)
	}
	return nil
}

// Validate implements renderers.Renderer. net-snmp has no offline checker (no -t/-n mode), so
// the renderer does a *parse run*: it stages a check copy of snmpd.conf in which every
// socket-binding or packet-sending directive is redirected into the staging dir
// (agentaddress and agentXSocket → unix sockets there; trap/inform sinks dropped, they are
// validated structurally), starts `snmpd -C -c <copy>` daemonised with its log and pidfile in
// the staging dir, stops that instance by its PID, and rejects the file when the daemon logged
// any warning or error about it ("Unknown token", "line N: Error: …"). Every other directive
// is parsed by the real daemon. It never touches the live paths or the running snmpd.
func (r *Renderer) Validate(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	conf := files[r.paths.ConfFile].Content
	r.red.Add(parseRendered(conf).secrets...)
	st, err := renderers.Stage(files)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	dir := filepath.Join(st.Dir, "check")
	for _, d := range []string{dir, filepath.Join(dir, "persist"), filepath.Join(dir, "mibs")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	checkConf := filepath.Join(dir, "snmpd.conf")
	if err := os.WriteFile(checkConf, CheckCopy(conf, dir), 0o600); err != nil {
		return err
	}
	logFile, pidFile := filepath.Join(dir, "snmpd.log"), filepath.Join(dir, "snmpd.pid")
	args := []string{"-C", "-c", checkConf, "-Lf", logFile, "-p", pidFile, "-m", "", "-M", filepath.Join(dir, "mibs")}
	_, runErr := r.runner.Run(ctx, renderers.Command{Path: SnmpdBin, Args: args, Timeout: parseRunTimeout})
	stopErr := stopCheckInstance(ctx, pidFile, dir)
	logText, _ := rfkit.ReadFileLimit(logFile, maxLogSize)
	msg := strings.ReplaceAll(r.red.Redact(string(bytes.ToValidUTF8(logText, []byte("?")))), st.Dir, "<staging>")
	if runErr != nil {
		return r.red.Error(fmt.Errorf("%w: snmpd parse run failed: %v: %s", ErrDaemon, runErr, oneLine(msg)))
	}
	if problems := parseProblems(msg); len(problems) > 0 {
		return r.red.Error(fmt.Errorf("%w: snmpd rejected snmpd.conf: %s", ErrDaemon, strings.Join(problems, " | ")))
	}
	if stopErr != nil {
		return fmt.Errorf("snmpd: parse run: %w", stopErr)
	}
	return nil
}

// CheckCopy returns the parse-run copy of a rendered snmpd.conf: agentaddress and
// agentXSocket point into dir, trap/inform sinks are replaced by a comment (no packets leave
// the check instance; the line count stays the same) and the persistent store is dir/persist.
func CheckCopy(conf []byte, dir string) []byte {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(string(conf), "\n"), "\n") {
		word, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch strings.ToLower(word) {
		case "agentaddress":
			line = "agentaddress unix:" + filepath.Join(dir, "agent.sock")
		case "agentxsocket":
			line = "agentXSocket unix:" + filepath.Join(dir, "agentx.sock")
		case "trap2sink", "informsink", "trapsess", "trapsink":
			line = "# (parse run) notification sink removed"
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	// Appended so the daemon's line numbers are those of the rendered file.
	b.WriteString("[snmp] persistentDir " + filepath.Join(dir, "persist") + "\n")
	return []byte(b.String())
}

// problemLog matches the forms in which snmpd reports a problem with its configuration (review
// L2: keyed on snmpd's own structured messages, not on loose keywords):
//
//	<file>: line N: Error: …      <file>: line N: Warning: …   (read_config)
//	Warning: Unknown token: …     Error: …                      (token handlers without a line)
//	Error opening specified endpoint …                          (transport)
var problemLog = regexp.MustCompile(`(?i)(?:line [0-9]+: (?:error|warning)\b)|(?:^(?:error|warning)\b[: ])|unknown token|^error opening`)

// benignLog: messages matching problemLog that do not concern the file.
var benignLog = []*regexp.Regexp{
	// A disabled agent grants no access on purpose.
	regexp.MustCompile(`^Warning: no access control information configured\.`),
}

func parseProblems(log string) []string {
	var out []string
	for _, l := range strings.Split(log, "\n") {
		t := strings.TrimSpace(l)
		if t == "" || !problemLog.MatchString(t) {
			continue
		}
		benign := false
		for _, re := range benignLog {
			if re.MatchString(t) {
				benign = true
				break
			}
		}
		if !benign {
			out = append(out, t)
		}
	}
	if len(out) > 8 {
		out = append(out[:8], fmt.Sprintf("(%d more)", len(out)-8))
	}
	return out
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 512 {
		s = s[:512] + "..."
	}
	return s
}

// stopCheckInstance stops the daemonised parse-run snmpd: the PID from its pidfile, verified
// to be snmpd started with a config under dir (never any other process), SIGTERM then SIGKILL.
func stopCheckInstance(ctx context.Context, pidFile, dir string) error {
	var pid int
	// The daemonised child writes the pidfile after the parent has exited.
	_ = rfkit.Poll(ctx, 3*time.Second, 20*time.Millisecond, func(context.Context) error {
		p, err := rfkit.PIDFile(pidFile)()
		pid = p
		return err
	})
	if pid == 0 {
		return nil // never started (the run error says why) or already gone
	}
	ours := func() bool {
		exe, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
		if err != nil || exe != SnmpdBin {
			return false
		}
		cmd, err := rfkit.ReadFileLimit(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"), 4096)
		return err == nil && bytes.Contains(cmd, []byte(dir))
	}
	if !ours() {
		return nil
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	err := rfkit.Poll(context.WithoutCancel(ctx), 3*time.Second, 20*time.Millisecond, func(context.Context) error {
		if ours() {
			return fmt.Errorf("parse-run snmpd pid %d still running", pid)
		}
		return nil
	})
	if err != nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	return nil
}

// startupKeys are the directives snmpd applies only when it starts: SIGHUP re-reads the file
// but never reopens the listening sockets (review H1, reproduced live: after a port change and
// SIGHUP the agent still answers on the old port) nor the AgentX master or the engine id.
var startupKeys = map[string]bool{"agentaddress": true, "agentxsocket": true, "agentxperms": true, "master": true, "exactengineid": true}

func startupDirectives(conf []byte) []string {
	var out []string
	for _, l := range strings.Split(string(conf), "\n") {
		word, _, _ := strings.Cut(strings.TrimSpace(l), " ")
		if startupKeys[strings.ToLower(word)] {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

// Warnings returns the warnings recorded in a rendered snmpd.conf ("# WARNING:" lines: the
// loopback-only default, wildcard listen addresses). They never fail Validate; the commit
// engine shows them.
func Warnings(files renderers.Files, p Paths) []string {
	var out []string
	for _, l := range strings.Split(string(files[p.ConfFile].Content), "\n") {
		if w, ok := strings.CutPrefix(l, "# WARNING: "); ok {
			out = append(out, w)
		}
	}
	return out
}

// mainPID returns snmpd's main PID through the controller (0 when not running or unknown).
func (r *Renderer) mainPID(ctx context.Context) int {
	if m, ok := r.ctl.(rfkit.MainPID); ok {
		if pid, err := m.MainPID(ctx); err == nil {
			return pid
		}
	}
	return 0
}

// listenersMatch proves the listening sockets of the running snmpd are exactly the rendered
// agentaddress set (UDP transports; a disabled agent listens on none).
func (r *Renderer) listenersMatch(ctx context.Context, conf []byte) error {
	pid := r.mainPID(ctx)
	if pid == 0 {
		return fmt.Errorf("%w: %w: snmpd main process unknown", ErrDaemon, rfkit.ErrNotConverged)
	}
	got, err := r.listeners(pid)
	if err != nil {
		return fmt.Errorf("%w: %w: %v", ErrDaemon, rfkit.ErrNotConverged, err)
	}
	want := renderedListeners(conf)
	if !slices.Equal(got, want) {
		return fmt.Errorf("%w: %w: snmpd listens on %v, rendered %v", ErrDaemon, rfkit.ErrNotConverged, got, want)
	}
	return nil
}

func renderedListeners(conf []byte) []string {
	var out []string
	for _, l := range strings.Split(string(conf), "\n") {
		if rest, ok := strings.CutPrefix(l, "agentaddress "); ok {
			for _, t := range strings.Split(rest, ",") {
				if strings.HasPrefix(t, "udp:") || strings.HasPrefix(t, "udp6:") {
					out = append(out, t)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

// Apply implements renderers.Renderer: snapshot → atomic write of snmpd.conf (0600) → then
//   - snmpd not running: nothing to reload; an enabled agent → *rfkit.ActionRequired start;
//   - a startup-only directive changed (agentaddress, AgentX, engine id) → a restart request
//     (*rfkit.ActionRequired restart), persisted in Paths.PendingFile and returned by every
//     Apply until snmpd runs a process started after it (D-079);
//   - otherwise SIGHUP (`systemctl reload snmpd`) → convergence: the sockets snmpd actually
//     holds equal the rendered listen set (/proc/<pid>/fd + net/udp{,6}), and an SNMP GET shows
//     the rendered sysName/sysLocation/sysContact → on any failure restore + reload.
//
// The files stay written when a restart is requested (the restart must read them). Idempotent.
// Not safe for concurrent use: the commit engine serialises commits.
func (r *Renderer) Apply(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	conf := files[r.paths.ConfFile].Content
	want := parseRendered(conf)
	r.red.Add(want.secrets...)
	previous, prevErr := rfkit.ReadFileLimit(r.paths.ConfFile, maxConfSize)
	snap, err := renderers.TakeSnapshot(files.Paths()...)
	if err != nil {
		return err
	}
	if err := renderers.WriteFiles(files); err != nil {
		return errors.Join(err, snap.Restore())
	}
	pid := r.mainPID(ctx)
	if pid == 0 {
		rfkit.ClearPending(r.paths.PendingFile) // the next start reads the new file
		if want.enabled {
			return &rfkit.ActionRequired{Daemon: "snmpd", Unit: "snmpd", Action: "start", Reason: "services.snmp is enabled but snmpd is not running"}
		}
		return nil
	}
	if prevErr != nil || !slices.Equal(startupDirectives(previous), startupDirectives(conf)) {
		reason := "listen addresses, AgentX or engine id changed (snmpd applies them only at startup; SIGHUP keeps the old sockets)"
		if err := rfkit.SetPending(r.paths.PendingFile, "restart", reason, pid); err != nil {
			return fmt.Errorf("snmpd: record pending restart: %w", err)
		}
		return &rfkit.ActionRequired{Daemon: "snmpd", Unit: "snmpd", Action: "restart", Reason: reason}
	}
	if rec := rfkit.GetPending(r.paths.PendingFile); rec != nil {
		if !rec.StartedAfter(pid) {
			return &rfkit.ActionRequired{Daemon: "snmpd", Unit: "snmpd", Action: rec.Action, Reason: rec.Reason + " (still pending: snmpd has not restarted since)"}
		}
		rfkit.ClearPending(r.paths.PendingFile)
	}
	verify := func(ctx context.Context) error {
		return rfkit.Poll(ctx, r.verifyTimeout, 200*time.Millisecond, func(ctx context.Context) error {
			if err := r.listenersMatch(ctx, conf); err != nil {
				return err
			}
			if !want.queryable() {
				return nil // no local credential: the socket set is the proof (README)
			}
			st, err := r.query(ctx, want)
			if err != nil {
				return fmt.Errorf("%w: %w: %v", ErrDaemon, rfkit.ErrNotConverged, err)
			}
			return want.matches(st)
		})
	}
	return r.red.Error(rfkit.Finish(ctx, snap, r.ctl.Reload, verify))
}

// Converged checks that the running snmpd serves the live snmpd.conf (listening sockets and
// sys* values) — for the commit engine after it acted on a restart request.
func (r *Renderer) Converged(ctx context.Context) error {
	conf, err := rfkit.ReadFileLimit(r.paths.ConfFile, maxConfSize)
	if err != nil {
		return err
	}
	if err := r.listenersMatch(ctx, conf); err != nil {
		return r.red.Error(err)
	}
	if rec := rfkit.GetPending(r.paths.PendingFile); rec != nil && rec.StartedAfter(r.mainPID(ctx)) {
		rfkit.ClearPending(r.paths.PendingFile)
	}
	p := parseRendered(conf)
	if !p.queryable() {
		return nil
	}
	st, err := r.query(ctx, p)
	if err != nil {
		return r.red.Error(fmt.Errorf("%w: %w: %v", ErrDaemon, rfkit.ErrNotConverged, err))
	}
	return p.matches(st)
}
