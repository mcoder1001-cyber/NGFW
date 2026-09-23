package vppstartup

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"path/filepath"
	"text/template"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/renderers"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var startupTmpl = template.Must(renderers.NewTemplate("startup.conf.tmpl").
	Funcs(template.FuncMap{"pathtok": PathToken}).
	ParseFS(templateFS, "templates/startup.conf.tmpl"))

// DefaultConfPath is VPP's start-up configuration on the appliance.
const DefaultConfPath = "/etc/vpp/startup.conf"

// ErrManagerStep is returned by Apply: writing the live file and restarting VPP is a manager
// step under `flock -x /run/lock/vrx-lab.lock` (docs/agent/renderers/vppstartup.md), never
// something the agent or a commit does.
var ErrManagerStep = errors.New("vppstartup: applying startup.conf restarts VPP and is a manager step (vrx-startupgen + docs/agent/renderers/vppstartup.md)")

// ErrRetrieveUnsupported is returned by Retrieve: VPP has no API that returns its start-up
// configuration; compare files with vrx-startupgen --diff instead.
var ErrRetrieveUnsupported = errors.New("vppstartup: VPP cannot report its start-up configuration")

// Settings are the product constants of the unix/api/statseg sections. They are not part of
// the configuration document; the defaults reproduce the appliance layout (and the current
// hand-written file on vrx-a).
type Settings struct {
	// ConfPath is where the file goes (Files key).
	ConfPath string
	// LogFile is `unix { log … }`.
	LogFile string
	// CLISocket is `unix { cli-listen … }`.
	CLISocket string
	// StatsSocket is `statseg { socket-name … }`.
	StatsSocket string
	// Group owns the API segment and the CLI socket (`gid`).
	Group string
}

// DefaultSettings returns the appliance constants.
func DefaultSettings() Settings {
	return Settings{
		ConfPath:    DefaultConfPath,
		LogFile:     "/var/log/vpp/vpp.log",
		CLISocket:   "/run/vpp/cli.sock",
		StatsSocket: "/run/vpp/stats.sock",
		Group:       "vpp",
	}
}

// PathToken accepts an absolute, clean path made of [A-Za-z0-9_.@/-] only, so it is exactly one
// token of the rendered file.
func PathToken(p string) (string, error) {
	if p == "" || !filepath.IsAbs(p) || filepath.Clean(p) != p || len(p) > 255 {
		return "", fmt.Errorf("%w: path %q must be absolute and clean", renderers.ErrUnsafe, p)
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '@' || c == '/' || c == '-') {
			return "", fmt.Errorf("%w: path %q contains %q", renderers.ErrUnsafe, p, c)
		}
	}
	return p, nil
}

// Generate is the pure generator: document → validated model → file content. msg is anything
// Desired accepts. It performs no I/O.
func Generate(msg proto.Message, host Host, s Settings) ([]byte, *Model, error) {
	dp, ext, err := Desired(msg)
	if err != nil {
		return nil, nil, err
	}
	m, err := BuildModel(dp, ext, host)
	if err != nil {
		return nil, nil, err
	}
	out, err := RenderModel(m, s)
	if err != nil {
		return nil, nil, err
	}
	return out, m, nil
}

// RenderModel renders a validated model.
func RenderModel(m *Model, s Settings) ([]byte, error) {
	if m == nil {
		return nil, fmt.Errorf("%w: nil model", ErrInput)
	}
	return renderers.Execute(startupTmpl, struct {
		M *Model
		S Settings
	}{m, s})
}

// Renderer implements renderers.Renderer for DryRun-style use: Render + Validate only. The
// commit engine may call them to report "restart required" and surface validation errors;
// Apply and Retrieve refuse (see ErrManagerStep, ErrRetrieveUnsupported).
type Renderer struct {
	host     Host
	settings Settings
}

var _ renderers.Renderer = (*Renderer)(nil)

// Option configures a Renderer.
type Option func(*Renderer)

// WithHost sets the host facts validation runs against (default: all unknown).
func WithHost(h Host) Option { return func(r *Renderer) { r.host = h } }

// WithSettings replaces the product constants (tests point ConfPath at a temp dir).
func WithSettings(s Settings) Option { return func(r *Renderer) { r.settings = s } }

// New returns a Renderer with DefaultSettings.
func New(opts ...Option) *Renderer {
	r := &Renderer{settings: DefaultSettings()}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Name implements renderers.Renderer.
func (r *Renderer) Name() string { return "vpp-startup" }

// Render implements renderers.Renderer: one file, mode 0644 root:vpp-readable, deterministic.
func (r *Renderer) Render(_ context.Context, desired proto.Message) (renderers.Files, error) {
	if _, err := PathToken(r.settings.ConfPath); err != nil {
		return nil, err
	}
	out, _, err := Generate(desired, r.host, r.settings)
	if err != nil {
		return nil, err
	}
	return renderers.Files{r.settings.ConfPath: {Mode: 0o644, Content: out}}, nil
}

// Validate implements renderers.Renderer. VPP has no offline checker for startup.conf, so the
// check is structural: exactly the one expected path, balanced braces, no control characters.
func (r *Renderer) Validate(_ context.Context, files renderers.Files) error {
	if err := files.Validate(); err != nil {
		return err
	}
	f, ok := files[r.settings.ConfPath]
	if !ok || len(files) != 1 {
		return fmt.Errorf("%w: expected exactly %s, got %v", renderers.ErrInvalidFiles, r.settings.ConfPath, files.Paths())
	}
	if err := renderers.CheckRendered(f.Content); err != nil {
		return fmt.Errorf("%w: %v", renderers.ErrInvalidFiles, err)
	}
	if _, err := Parse(f.Content); err != nil {
		return fmt.Errorf("%w: %v", renderers.ErrInvalidFiles, err)
	}
	return nil
}

// Apply implements renderers.Renderer and always refuses (ErrManagerStep).
func (r *Renderer) Apply(context.Context, renderers.Files) error { return ErrManagerStep }

// Retrieve implements renderers.Renderer and always refuses (ErrRetrieveUnsupported).
func (r *Renderer) Retrieve(context.Context) (proto.Message, error) {
	return nil, ErrRetrieveUnsupported
}
