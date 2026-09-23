// Package rsyslog is the rsyslog export renderer (RF-4, WBS D7.7): management.syslog →
// one RainerScript file (a ruleset + omfwd action per target, facility/severity filters, an
// RFC 5424 template, impstats) → `rsyslogd -N1` → applied by restarting rsyslog (it cannot
// reload its configuration; SIGHUP only reopens files) → convergence and state from impstats
// → 1 Hz events on failures/suspensions. TLS keys are D-051 references resolved at render
// time into their own 0640 files. See README.md.
package rsyslog

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
	"sync"
	"text/template"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rfkit"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// ErrDaemon is wrapped by errors from rsyslog (checker, restart, convergence, missing driver).
var ErrDaemon = errors.New("rsyslog: daemon error")

const (
	checkTimeout  = 30 * time.Second
	verifyTimeout = 10 * time.Second
	maxConfSize   = 1 << 20
	// maxStatsTail is how much of the impstats file Retrieve reads (the newest records).
	maxStatsTail = 256 << 10
	// statsTruncateAt: impstats appends forever; above this size Retrieve truncates the file
	// after reading (rsyslog writes it with O_APPEND, so truncation is safe).
	statsTruncateAt = 8 << 20
)

// Renderer implements renderers.Renderer for rsyslog.
type Renderer struct {
	runner        renderers.Runner
	ctl           rfkit.Controller
	paths         Paths
	resolver      rfkit.SecretResolver
	red           rfkit.Redactor
	verifyTimeout time.Duration
	tmpl          *template.Template
	// statsMu keeps State's truncation of the stats file out of an Apply's convergence window
	// (the check reads the records written after the restart by file offset).
	statsMu sync.Mutex
}

var _ renderers.Renderer = (*Renderer)(nil)

// Option configures a Renderer.
type Option func(*Renderer)

// WithPaths overrides ProductPaths (tests: TestPaths).
func WithPaths(p Paths) Option { return func(r *Renderer) { r.paths = p } }

// WithController replaces `systemctl restart rsyslog` (tests: restart the child).
func WithController(c rfkit.Controller) Option { return func(r *Renderer) { r.ctl = c } }

// WithSecretResolver sets the resolver of the D-051 references (TLS CA/cert/key).
func WithSecretResolver(sr rfkit.SecretResolver) Option { return func(r *Renderer) { r.resolver = sr } }

// WithVerifyTimeout bounds the post-restart convergence check (default 10 s).
func WithVerifyTimeout(d time.Duration) Option { return func(r *Renderer) { r.verifyTimeout = d } }

// New returns an rsyslog renderer (production runner:
// renderers.NewSystemRunner(renderers.NewAllowlist(rsyslog.Binaries()...))).
func New(runner renderers.Runner, opts ...Option) *Renderer {
	r := &Renderer{runner: runner, paths: ProductPaths(), verifyTimeout: verifyTimeout}
	for _, o := range opts {
		o(r)
	}
	if r.ctl == nil {
		r.ctl = &rfkit.SystemdController{Runner: runner, Unit: "rsyslog"}
	}
	r.tmpl = template.Must(renderers.NewTemplate("rsyslog").Funcs(funcs()).ParseFS(templateFS, "templates/*.tmpl"))
	return r
}

// Name implements renderers.Renderer.
func (r *Renderer) Name() string { return "rsyslog" }

// Paths returns the paths this renderer uses.
func (r *Renderer) Paths() Paths { return r.paths }

// Redact masks every secret this renderer resolved.
func (r *Renderer) Redact(s string) string { return r.red.Redact(s) }

func (r *Renderer) check() error {
	if r.runner == nil {
		return errors.New("rsyslog: renderer has no runner")
	}
	return r.paths.Validate()
}

// Quote returns s as a RainerScript string literal: `"` and `\` escaped, control characters
// (and invalid UTF-8) rejected.
func Quote(s string) (string, error) {
	if _, err := renderers.Line(s); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		if s[i] == '"' || s[i] == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	b.WriteByte('"')
	return b.String(), nil
}

var (
	rulesetRe = regexp.MustCompile(`^vrx_export_[0-9]{1,2}_[0-9a-f]{8}$`)
	filterRe  = regexp.MustCompile(`^(?:\*|[a-z0-9]+(?:,[a-z0-9]+)*)\.(?:emerg|alert|crit|err|warning|notice|info|debug)$`)
	hostRe    = regexp.MustCompile(`^[A-Za-z0-9.:-]{1,253}$`)
)

func funcs() template.FuncMap {
	return template.FuncMap{
		"q": Quote,
		"port": func(n uint32) (string, error) {
			if n < 1 || n > 65535 {
				return "", fmt.Errorf("%w: port %d", ErrInput, n)
			}
			return strconv.FormatUint(uint64(n), 10), nil
		},
		"num": func(n uint32) string { return strconv.FormatUint(uint64(n), 10) },
		"host": func(s string) (string, error) {
			if !hostRe.MatchString(s) {
				return "", fmt.Errorf("%w: target %q", ErrInput, s)
			}
			return s, nil
		},
		"tmpl": func(s string) (string, error) {
			if s != TemplateRFC5424 && s != TemplateRFC3164 {
				return "", fmt.Errorf("%w: template %q is not in the fixed set", ErrInput, s)
			}
			return s, nil
		},
		"filter": func(s string) (string, error) {
			if !filterRe.MatchString(s) {
				return "", fmt.Errorf("%w: filter %q", ErrInput, s)
			}
			return s, nil
		},
		"ruleset": func(s string) (string, error) {
			if !rulesetRe.MatchString(s) {
				return "", fmt.Errorf("%w: ruleset %q", ErrInput, s)
			}
			return s, nil
		},
		"join": func(l []string) string { return strings.Join(l, ",") },
	}
}

// Render implements renderers.Renderer: the RainerScript file plus, per TLS target, its CA,
// certificate (0644) and private key (0640, KeyOwner, Secret). No I/O except the resolver.
func (r *Renderer) Render(ctx context.Context, desired proto.Message) (renderers.Files, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	ds, ext, err := rfkit.Decode(desired)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	sec := &rfkit.Secrets{Ctx: ctx, Resolver: r.resolver}
	model, TLSFiles, err := BuildModel(ds, ext, sec, r.paths)
	r.red.Add(sec.Values()...)
	if err != nil {
		return nil, r.red.Error(err)
	}
	content, err := renderers.ExecuteTemplate(r.tmpl, "rsyslog.conf.tmpl", model)
	if err != nil {
		return nil, r.red.Error(err)
	}
	files := renderers.Files{r.paths.ConfFile: {Mode: r.paths.FileMode, Owner: r.paths.FileOwner, Content: content}}
	for _, f := range TLSFiles {
		if f.Secret {
			files[f.Path] = renderers.File{Mode: 0o640, Owner: r.paths.KeyOwner, Content: []byte(f.Content), Secret: true}
		} else {
			files[f.Path] = renderers.File{Mode: 0o644, Owner: r.paths.FileOwner, Content: []byte(f.Content)}
		}
	}
	return files, files.Validate()
}

// checkFiles verifies files is this renderer's file set: the config plus TLS files in TLSDir.
func (r *Renderer) checkFiles(files renderers.Files) error {
	if err := files.Validate(); err != nil {
		return err
	}
	if _, ok := files[r.paths.ConfFile]; !ok {
		return fmt.Errorf("%w: %s missing", renderers.ErrInvalidFiles, r.paths.ConfFile)
	}
	for p := range files {
		if p != r.paths.ConfFile && filepath.Dir(p) != r.paths.TLSDir {
			return fmt.Errorf("%w: rsyslog renderer does not own %s", renderers.ErrInvalidFiles, p)
		}
	}
	return nil
}

// Validate implements renderers.Renderer: `rsyslogd -N1 -f <staged file>` (config validation
// run: syntax, module and action parameters). A file with no action (management.syslog
// empty) is not checked by the daemon — rsyslogd rejects a config without any output
// (error -2103) although the empty export is valid — and is validated structurally only.
// TLS targets additionally need the ossl netstream driver, which `-N1` does not load: its
// presence in ModuleDir is checked here so a commit cannot install an export that could never
// connect.
func (r *Renderer) Validate(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	for _, f := range files {
		if f.Secret {
			r.red.Add(string(f.Content))
		}
	}
	conf := files[r.paths.ConfFile].Content
	if bytes.Contains(conf, []byte(`StreamDriver="`+TLSDriver+`"`)) {
		if _, err := os.Stat(filepath.Join(r.paths.ModuleDir, "lmnsd_"+TLSDriver+".so")); err != nil {
			return fmt.Errorf("%w: TLS export needs the rsyslog %s netstream driver (lmnsd_%s.so, package rsyslog-openssl), not installed in %s",
				ErrDaemon, TLSDriver, TLSDriver, r.paths.ModuleDir)
		}
	}
	if !bytes.Contains(conf, []byte("action(")) {
		return nil
	}
	st, err := renderers.Stage(files)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	out, err := r.runner.Run(ctx, renderers.Command{Path: RsyslogdBin, Args: []string{"-N1", "-f", st.Path(r.paths.ConfFile)}, Timeout: checkTimeout})
	if err != nil {
		out.Stdout, out.Stderr = []byte(r.red.Redact(string(out.Stdout))), []byte(r.red.Redact(string(out.Stderr)))
		return r.red.Error(fmt.Errorf("%w: rsyslogd -N1 rejected the export config: %s", ErrDaemon, toolMessage(out, err, st.Dir)))
	}
	return nil
}

func toolMessage(out renderers.Output, err error, stagingDir string) string {
	var parts []string
	for _, b := range [][]byte{out.Stderr, out.Stdout} {
		for _, l := range strings.Split(string(bytes.ToValidUTF8(b, []byte("?"))), "\n") {
			l = strings.TrimSpace(l)
			// rsyslogd's banner lines carry no information about the error.
			if l == "" || strings.Contains(l, "config validation run") || strings.Contains(l, "End of config validation run") {
				continue
			}
			parts = append(parts, l)
		}
	}
	msg := strings.Join(parts, " | ")
	if msg == "" {
		msg = err.Error()
	}
	if stagingDir != "" {
		msg = strings.ReplaceAll(msg, stagingDir, "<staging>")
	}
	if len(msg) > 1024 {
		msg = msg[:1024] + "..."
	}
	return msg
}

// Apply implements renderers.Renderer: TLS dir → snapshot → atomic write (config 0644, keys
// 0640) → restart rsyslog (it cannot reload a configuration: messages arriving during the
// ~100 ms restart are buffered by the kernel socket and imuxsock, see README) → convergence:
// impstats written after the restart lists every rendered action → on failure restore and
// restart again. TLS files of targets that were removed are deleted after success.
func (r *Renderer) Apply(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	if len(files) > 1 {
		if err := os.MkdirAll(r.paths.TLSDir, 0o755); err != nil { //nolint:gosec // keys inside are 0640
			return err
		}
	}
	for _, f := range files {
		if f.Secret {
			r.red.Add(string(f.Content))
		}
	}
	want := actionNames(files[r.paths.ConfFile].Content)
	var offset int64
	var restarted time.Time
	activate := func(ctx context.Context) error {
		offset = statsSize(r.paths.StatsFile)
		err := r.ctl.Restart(ctx)
		// The previous process has exited when Restart returns (systemctl restart waits for
		// the stop; it writes a last impstats batch while stopping): only records stamped in a
		// later second come from the new configuration.
		restarted = time.Now()
		return err
	}
	verify := func(ctx context.Context) error {
		if len(want) == 0 {
			return nil // no action, no stats: nothing rsyslog could report (README)
		}
		return rfkit.Poll(ctx, r.verifyTimeout, 200*time.Millisecond, func(context.Context) error {
			stats, err := readStatsFrom(r.paths.StatsFile, offset, restarted)
			if err != nil {
				return fmt.Errorf("%w: %w: %v", ErrDaemon, rfkit.ErrNotConverged, err)
			}
			// Exactly the rendered actions, from records written after the restart: an old
			// process still running the previous config reports other names (actionName).
			var got []string
			for n, c := range stats {
				if c.Origin == "core.action" && strings.HasPrefix(n, "vrx_export_") {
					got = append(got, n)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, want) {
				return fmt.Errorf("%w: %w: impstats reports actions %v after the restart, rendered %v", ErrDaemon, rfkit.ErrNotConverged, got, want)
			}
			return nil
		})
	}
	r.statsMu.Lock()
	defer r.statsMu.Unlock()
	if err := rfkit.ApplyFiles(ctx, files, activate, verify); err != nil {
		return r.red.Error(err)
	}
	r.pruneTLS(files)
	return nil
}

func (r *Renderer) pruneTLS(files renderers.Files) {
	ents, err := os.ReadDir(r.paths.TLSDir)
	if err != nil {
		return
	}
	for _, e := range ents {
		p := filepath.Join(r.paths.TLSDir, e.Name())
		if _, keep := files[p]; !keep && e.Type().IsRegular() && strings.HasPrefix(e.Name(), "export-") {
			_ = os.Remove(p)
		}
	}
}

var actionNameRe = regexp.MustCompile(`action\(type="omfwd" name="(vrx_export_[0-9]{1,2}_[0-9a-f]{8})"`)

func actionNames(conf []byte) []string {
	var out []string
	for _, m := range actionNameRe.FindAllSubmatch(conf, -1) {
		out = append(out, string(m[1]))
	}
	slices.Sort(out)
	return out
}

func statsSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
