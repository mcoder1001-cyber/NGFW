// Package strongswan is the strongSwan renderer (RF-2, WBS D6.2): desired IPsec/IKE state
// (vpn.ipsec.proposals, vpn.ipsec.tunnels) → validated swanctl.conf fragments + secrets +
// strongswan.conf → applied through charon's own control channel (VICI load-conn /
// load-shared / load-pool / unload-*) → state read back from VICI list-conns / list-sas /
// stats / version → ike-updown / child-updown / rekey events. The daemon stays a separate
// process; the renderer does not care which kernel plugin charon loads (kernel-netlink on the
// stock package, kernel-vpp in P11). See README.md.
package strongswan

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"text/template"
	"time"

	"embed"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/renderers"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// ErrDaemon is wrapped by errors that come from charon (VICI unreachable, a load rejected,
// not converged after loading).
var ErrDaemon = errors.New("strongswan: daemon error")

// Timeouts for VICI calls and the checker.
const (
	callTimeout     = 15 * time.Second
	validateTimeout = 30 * time.Second
	rollbackTimeout = 60 * time.Second
)

// DaemonConfig is what the renderer puts into strongswan.conf besides the paths.
type DaemonConfig struct {
	// Plugins is the charon plugin load list (load_modular = no). P11 passes the kernel-vpp
	// list; DefaultPlugins is the stock kernel-netlink one. It must contain "vici".
	Plugins []string
	// InstallRoutes lets charon install routes for policies (kernel-netlink table 220).
	InstallRoutes bool
	// Port / PortNATT override the IKE ports (0 = default 500 / 4500).
	Port, PortNATT uint16
	// LogLevel is the filelog default level, 0 or 1 (higher levels can log key material,
	// so the renderer never renders more than 1).
	LogLevel int
	// QuietJournal disables the journal and syslog loggers (test daemons; the product keeps
	// charon-systemd's journal logging).
	QuietJournal bool
}

// DefaultPlugins is the plugin list of the stock Ubuntu strongSwan (kernel-netlink path).
func DefaultPlugins() []string {
	return []string{"random", "nonce", "openssl", "pem", "pkcs1", "pkcs8", "x509", "pubkey", "revocation", "constraints", "kernel-netlink", "socket-default", "vici"}
}

var pluginRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

func (d DaemonConfig) check() error {
	if len(d.Plugins) == 0 || len(d.Plugins) > 64 {
		return fmt.Errorf("strongswan: DaemonConfig.Plugins must list 1..64 plugins")
	}
	hasVici := false
	for _, p := range d.Plugins {
		if !pluginRe.MatchString(p) {
			return fmt.Errorf("strongswan: plugin name %q must match %s", p, pluginRe)
		}
		hasVici = hasVici || p == "vici"
	}
	if !hasVici {
		return errors.New("strongswan: DaemonConfig.Plugins must contain vici (the renderer's control channel)")
	}
	if d.LogLevel < 0 || d.LogLevel > 1 {
		return fmt.Errorf("strongswan: LogLevel %d not in 0..1 (higher levels may log key material)", d.LogLevel)
	}
	return nil
}

// Checker is the optional integration checker of Validate: a scratch charon (never the one
// Apply drives) into which `swanctl --load-all` loads a staged copy of the files. The product
// has none (strongSwan has no offline checker; Validate is structural there).
type Checker struct {
	// ViciSocket is the scratch charon's VICI socket; it must differ from Paths.ViciSocket.
	ViciSocket string
	// Runner runs swanctl (tests: a runner that enters the test mount namespace).
	Runner renderers.Runner
}

// Renderer implements renderers.Renderer for strongSwan.
type Renderer struct {
	paths    Paths
	daemon   DaemonConfig
	resolver SecretResolver
	ifID     IfIDMapper
	dial     Dialer
	checker  *Checker
	log      *slog.Logger
	secrets  secretSet
	now      func() time.Time
	owner    string
	resync   time.Duration
}

var _ renderers.Renderer = (*Renderer)(nil)

// Option configures a Renderer.
type Option func(*Renderer)

// WithPaths sets the paths. Required: there is no implicit product default (RF-2 review M2);
// the product passes ProductPaths(), tests TestPaths(prefix, instance).
func WithPaths(p Paths) Option { return func(r *Renderer) { r.paths = p } }

// WithDaemonConfig sets the strongswan.conf parameters (default: DefaultPlugins).
func WithDaemonConfig(d DaemonConfig) Option { return func(r *Renderer) { r.daemon = d } }

// WithSecretResolver sets the D-051 secret resolver (required for PSK tunnels).
func WithSecretResolver(sr SecretResolver) Option { return func(r *Renderer) { r.resolver = sr } }

// WithIfIDMapper sets the route-based tunnel → if_id mapping (P11).
func WithIfIDMapper(m IfIDMapper) Option { return func(r *Renderer) { r.ifID = m } }

// WithDialer replaces the VICI dialer (unit tests: a fake charon).
func WithDialer(d Dialer) Option { return func(r *Renderer) { r.dial = d } }

// WithChecker enables the swanctl integration check in Validate.
func WithChecker(c Checker) Option { return func(r *Renderer) { r.checker = &c } }

// WithOwnerPrefix limits the renderer to charon objects whose names start with prefix
// (connections, pools, authorities; shared secrets "ike-<prefix>…"): every rendered name must
// carry it, and Apply/State/converge ignore everything else, so several owners (test slots on
// a shared charon) never unload each other's connections. The product uses none (it owns
// charon's VICI configuration, like `swanctl --load-all`).
func WithOwnerPrefix(prefix string) Option { return func(r *Renderer) { r.owner = prefix } }

var ownerRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,15}$`)

// owns reports whether a connection, pool or authority name belongs to this renderer.
func (r *Renderer) owns(name string) bool { return strings.HasPrefix(name, r.owner) }

// ownsShared reports whether a shared-secret id ("ike-<conn>") belongs to this renderer.
func (r *Renderer) ownsShared(id string) bool { return strings.HasPrefix(id, "ike-"+r.owner) }

// WithLogger sets the logger (default: discard). Every line is redacted.
func WithLogger(l *slog.Logger) Option { return func(r *Renderer) { r.log = l } }

// New returns a strongSwan renderer.
func New(opts ...Option) *Renderer {
	r := &Renderer{
		daemon: DaemonConfig{Plugins: DefaultPlugins(), LogLevel: 1},
		dial:   DialVICI,
		log:    slog.New(slog.DiscardHandler),
		now:    time.Now,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Name implements renderers.Renderer.
func (r *Renderer) Name() string { return "strongswan" }

// Paths returns the paths this renderer uses.
func (r *Renderer) Paths() Paths { return r.paths }

func (r *Renderer) check() error {
	if err := r.paths.Validate(); err != nil {
		return err
	}
	if err := r.daemon.check(); err != nil {
		return err
	}
	if r.owner != "" && !ownerRe.MatchString(r.owner) {
		return fmt.Errorf("strongswan: owner prefix %q must match %s", r.owner, ownerRe)
	}
	if r.checker != nil && r.checker.ViciSocket == r.paths.ViciSocket {
		return errors.New("strongswan: the Validate checker must use a scratch charon, not the live VICI socket")
	}
	return nil
}

// ------------------------------------------------------------------------------ templates

var templates = func() *template.Template {
	t := renderers.NewTemplate("strongswan").Funcs(template.FuncMap{
		"name":  SectionName,
		"q":     Quote,
		"tok":   Token,
		"list":  tokenList,
		"words": func(ws []string) string { return strings.Join(ws, " ") },
		"yesno": yesNo,
		"b64":   func(b []byte) string { return base64.StdEncoding.EncodeToString(b) },
		"deref": func(p *uint32) uint32 { return *p },
	})
	return template.Must(t.ParseFS(templateFS, "templates/*.tmpl"))
}()

// tokenList renders a comma list of bare tokens (each validated; no element may be empty or
// contain a comma).
func tokenList(items []string) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("%w: empty list", renderers.ErrUnsafe)
	}
	for _, it := range items {
		if strings.Contains(it, ",") {
			return "", fmt.Errorf("%w: list element %q contains a comma", renderers.ErrUnsafe, clip(it))
		}
		if _, err := Token(it); err != nil {
			return "", err
		}
	}
	return strings.Join(items, ","), nil
}

type confView struct {
	DaemonConfig
	ViciURI string
	LogFile string
}

// RenderModel renders a model (built by BuildModel or by hand, e.g. P11 tests) into the three
// files. Pure: no I/O.
func (r *Renderer) RenderModel(m *Model) (renderers.Files, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	if err := m.normalize(); err != nil {
		return nil, err
	}
	if err := r.checkOwnedModel(m); err != nil {
		return nil, err
	}
	conns, err := renderers.ExecuteTemplate(templates, "vrx.conf.tmpl", m)
	if err != nil {
		return nil, r.secrets.redactErr(err)
	}
	secrets, err := renderers.ExecuteTemplate(templates, "vrx-secrets.conf.tmpl", m)
	if err != nil {
		return nil, r.secrets.redactErr(err)
	}
	conf, err := renderers.ExecuteTemplate(templates, "strongswan.conf.tmpl", confView{
		DaemonConfig: r.daemon, ViciURI: "unix://" + r.paths.ViciSocket, LogFile: r.paths.LogFile,
	})
	if err != nil {
		return nil, err
	}
	files := renderers.Files{
		r.paths.StrongswanConf: {Mode: r.paths.ConfMode, Owner: r.paths.FileOwner, Content: conf},
		r.paths.ConnsFile():    {Mode: r.paths.ConfMode, Owner: r.paths.FileOwner, Content: conns},
		r.paths.SecretsFile():  {Mode: r.paths.SecretMode, Owner: r.paths.FileOwner, Content: secrets, Secret: true},
	}
	return files, files.Validate()
}

// Render implements renderers.Renderer: strongswan.conf, conf.d/vrx.conf and
// conf.d/vrx-secrets.conf (Secret, mode 0600). No I/O except the injected secret resolver.
// Resolved values are remembered so every later output of this renderer masks them.
func (r *Renderer) Render(ctx context.Context, desired proto.Message) (renderers.Files, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	ds, err := Desired(desired)
	if err != nil {
		return nil, err
	}
	resolve := func(ctx context.Context, ref string) ([]byte, error) {
		if r.resolver == nil {
			return nil, ErrNoSecretResolver
		}
		v, err := r.resolver.Resolve(ctx, ref)
		if err != nil {
			return nil, r.secrets.redactErr(err)
		}
		return v, nil
	}
	m, err := BuildModel(ctx, ds, buildOptions{resolve: resolve, ifID: r.ifID})
	if err != nil {
		return nil, r.secrets.redactErr(err)
	}
	return r.RenderModel(m)
}

func (r *Renderer) checkOwnedModel(m *Model) error {
	if r.owner == "" {
		return nil
	}
	var names []string
	for _, c := range m.Conns {
		names = append(names, c.Name)
	}
	for _, p := range m.Pools {
		names = append(names, p.Name)
	}
	for _, a := range m.Authorities {
		names = append(names, a.Name)
	}
	for _, n := range names {
		if !r.owns(n) {
			return fmt.Errorf("%w: %q does not carry the owner prefix %q of this renderer", ErrInput, n, r.owner)
		}
	}
	return nil
}

// checkFiles verifies files is exactly this renderer's file set.
func (r *Renderer) checkFiles(files renderers.Files) error {
	if err := files.Validate(); err != nil {
		return err
	}
	want := []string{r.paths.StrongswanConf, r.paths.ConnsFile(), r.paths.SecretsFile()}
	if len(files) != len(want) {
		return fmt.Errorf("%w: strongswan renderer owns exactly %s", renderers.ErrInvalidFiles, strings.Join(want, ", "))
	}
	for _, p := range want {
		if _, ok := files[p]; !ok {
			return fmt.Errorf("%w: %s missing", renderers.ErrInvalidFiles, p)
		}
	}
	if f := files[r.paths.SecretsFile()]; !f.Secret || f.Mode&0o077 != 0 {
		return fmt.Errorf("%w: %s must be Secret with an owner-only mode", renderers.ErrInvalidFiles, r.paths.SecretsFile())
	}
	return nil
}

// Validate implements renderers.Renderer. strongSwan has no offline checker (swanctl only
// talks to a running charon), so Validate is structural: every file must round-trip through
// the strict settings parser byte for byte, and the trees are checked semantically (known keys
// only, proposal keyword grammar, traffic selectors, identity shapes, every PSK connection has
// its secret). With WithChecker it additionally loads a staged copy into a scratch charon with
// `swanctl --load-all` (start actions rewritten to none there, so the scratch charon never
// initiates). It never touches the live paths or the live charon.
func (r *Renderer) Validate(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	if _, err := r.parseFiles(files); err != nil {
		return r.secrets.redactErr(err)
	}
	if r.checker == nil {
		return nil
	}
	return r.secrets.redactErr(r.runChecker(ctx, files))
}
