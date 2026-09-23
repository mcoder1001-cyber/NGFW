// Package kea is the Kea DHCP renderer (RF-3, WBS D7.1): services.dhcp.servers → validated
// kea-dhcp4.conf / kea-dhcp6.conf / kea-ctrl-agent.conf → applied through the daemons' unix
// control sockets (`config-set`, no process spawned, no restart) → state read back with
// `status-get`, `config-get`, `statistic-get-all` and paged `lease4/6-get-page` → change
// events by 1 Hz polling. Kea configs are JSON: they are rendered by marshalling typed Go
// structs with encoding/json, never by text templates. See README.md.
package kea

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/renderers"
)

// Timeouts.
const (
	validateTimeout = 30 * time.Second
	controlTimeout  = 30 * time.Second
)

// ErrDaemon is wrapped by errors that come from Kea (checker rejected a file, config-set
// failed).
var ErrDaemon = errors.New("kea: daemon error")

// InterfaceMapper maps a VPP interface name from the document to the Linux interface Kea
// binds (linux-cp tap / rig veth). The default is the identity (valid Linux names only).
type InterfaceMapper func(vppName string) (string, error)

// IdentityMapper passes names through unchanged.
func IdentityMapper(name string) (string, error) { return name, nil }

// Controller sends one Kea command to the DHCPv4 (family 4) or DHCPv6 (family 6) server.
// Production: SocketController over the unix control sockets; unit tests inject a fake.
type Controller interface {
	Command(ctx context.Context, family int, command string, args any) (Response, error)
}

// SocketController talks to the daemons over Paths.Socket4 / Socket6.
type SocketController struct{ Paths Paths }

// Command implements Controller.
func (s SocketController) Command(ctx context.Context, family int, command string, args any) (Response, error) {
	return Client{Socket: s.Paths.socket(family), Timeout: controlTimeout}.Command(ctx, command, args)
}

// Renderer implements renderers.Renderer for Kea DHCPv4 + DHCPv6 + the control agent.
type Renderer struct {
	runner        renderers.Runner
	paths         Paths
	mapIf         InterfaceMapper
	ctrl          Controller
	agent         *HTTPClient
	leaseCmdsHook string
	hookSet       bool
}

var _ renderers.Renderer = (*Renderer)(nil)

// Option configures a Renderer.
type Option func(*Renderer)

// WithPaths overrides ProductPaths (tests: TestPaths(prefix, slot)).
func WithPaths(p Paths) Option { return func(r *Renderer) { r.paths = p } }

// WithInterfaceMapper sets the VPP→Linux interface name mapping.
func WithInterfaceMapper(m InterfaceMapper) Option { return func(r *Renderer) { r.mapIf = m } }

// WithController replaces the unix-socket controller (unit tests).
func WithController(c Controller) Option { return func(r *Renderer) { r.ctrl = c } }

// WithCtrlAgent overrides the kea-ctrl-agent HTTP client used to reload the agent's own
// configuration (nil disables that step).
func WithCtrlAgent(c *HTTPClient) Option { return func(r *Renderer) { r.agent = c } }

// WithLeaseCmdsHook fixes the lease_cmds hook path ("" renders no hook) instead of
// discovering it under Paths.HooksDir.
func WithLeaseCmdsHook(path string) Option {
	return func(r *Renderer) { r.leaseCmdsHook, r.hookSet = path, true }
}

// New returns a Kea renderer running its checkers through runner (production: NewRunner).
// Unless WithLeaseCmdsHook is given, libdhcp_lease_cmds.so is looked up under
// Paths.HooksDir once, here, so Render stays free of I/O.
func New(runner renderers.Runner, opts ...Option) *Renderer {
	r := &Renderer{runner: runner, paths: ProductPaths(), mapIf: IdentityMapper}
	for _, o := range opts {
		o(r)
	}
	if r.ctrl == nil {
		r.ctrl = SocketController{Paths: r.paths}
	}
	if r.agent == nil {
		r.agent = &HTTPClient{Host: r.paths.CtrlAgentHost, Port: r.paths.CtrlAgentPort, Timeout: controlTimeout}
	}
	if !r.hookSet {
		candidate := filepath.Join(r.paths.HooksDir, LeaseCmdsHook)
		if st, err := os.Stat(candidate); err == nil && st.Mode().IsRegular() {
			r.leaseCmdsHook = candidate
		}
	}
	return r
}

// Name implements renderers.Renderer.
func (r *Renderer) Name() string { return "kea" }

// Paths returns the paths this renderer uses.
func (r *Renderer) Paths() Paths { return r.paths }

func (r *Renderer) check() error {
	if r.runner == nil {
		return errors.New("kea: renderer has no runner")
	}
	return r.paths.Validate()
}

// Render implements renderers.Renderer: kea-dhcp4.conf, kea-dhcp6.conf (both always present;
// a family without enabled servers gets an idle config with no interfaces and no subnets) and
// kea-ctrl-agent.conf. Pure: no I/O.
func (r *Renderer) Render(_ context.Context, desired proto.Message) (renderers.Files, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	in, err := extract(desired)
	if err != nil {
		return nil, err
	}
	c4, err := r.buildFamily(in, 4)
	if err != nil {
		return nil, err
	}
	r.interfaceBindings(in, &c4, 4)
	c6, err := r.buildFamily(in, 6)
	if err != nil {
		return nil, err
	}
	b4, err := marshal(dhcp4Root{Dhcp4: c4})
	if err != nil {
		return nil, err
	}
	b6, err := marshal(dhcp6Root{Dhcp6: c6})
	if err != nil {
		return nil, err
	}
	bca, err := marshal(r.buildCtrlAgent())
	if err != nil {
		return nil, err
	}
	file := func(b []byte) renderers.File {
		return renderers.File{Mode: r.paths.FileMode, Owner: r.paths.FileOwner, Content: b}
	}
	files := renderers.Files{
		r.paths.Dhcp4Conf():     file(b4),
		r.paths.Dhcp6Conf():     file(b6),
		r.paths.CtrlAgentConf(): file(bca),
	}
	return files, files.Validate()
}

func (r *Renderer) checkFiles(files renderers.Files) error {
	if err := files.Validate(); err != nil {
		return err
	}
	own := map[string]bool{r.paths.Dhcp4Conf(): true, r.paths.Dhcp6Conf(): true, r.paths.CtrlAgentConf(): true}
	for p, f := range files {
		if !own[p] {
			return fmt.Errorf("%w: kea renderer does not own %s", renderers.ErrInvalidFiles, p)
		}
		if !json.Valid(f.Content) {
			return fmt.Errorf("%w: %s is not JSON", renderers.ErrInvalidFiles, p)
		}
	}
	return nil
}

// checker returns the binary that validates path.
func (r *Renderer) checker(path string) string {
	switch path {
	case r.paths.Dhcp4Conf():
		return Dhcp4Bin
	case r.paths.Dhcp6Conf():
		return Dhcp6Bin
	default:
		return CtrlAgentBin
	}
}

// Validate implements renderers.Renderer: `kea-dhcp4 -t`, `kea-dhcp6 -t`, `kea-ctrl-agent -t`
// on a staged copy. The checkers parse the full configuration (option definitions and data,
// pools inside subnets, reservations, hook loading) without opening sockets.
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
	for _, p := range files.Paths() {
		bin := r.checker(p)
		out, err := r.runner.Run(ctx, renderers.Command{Path: bin, Args: []string{"-t", st.Path(p)}, Timeout: validateTimeout})
		if err != nil {
			return fmt.Errorf("%w: %s -t rejected %s: %s", ErrDaemon, filepath.Base(bin), filepath.Base(p), toolMessage(out, err, st.Dir))
		}
	}
	return nil
}

// ActionRequired is returned by Apply when the files were written but a daemon must be
// started or restarted by the caller for them to take effect (Kea: the server of a family
// that now has interfaces is not running). The commit engine surfaces it; it is not a
// failure, the files are not rolled back.
type ActionRequired struct {
	Daemon string // "kea-dhcp4"
	Unit   string // systemd unit in the product ("kea-dhcp4-server")
	Action string // "start" or "restart"
	Reason string
}

func (e *ActionRequired) Error() string {
	return fmt.Sprintf("kea: %s must be %sed (%s): %s", e.Daemon, e.Action, e.Unit, e.Reason)
}

// NeedsRestart reports the unit and action (shared duck-typed shape with the chrony renderer).
func (e *ActionRequired) NeedsRestart() (unit, action string) { return e.Unit, e.Action }

// Apply implements renderers.Renderer: snapshot → atomic write → `config-set` with the
// rendered configuration on each running server (the daemon swaps its configuration in
// place; leases survive) → kea-ctrl-agent `config-reload` when its file changed and it runs.
// On a failed config-set the snapshot is restored and the previous configuration is set
// again. A family whose server is not running is skipped when its configuration is idle and
// reported as *ActionRequired otherwise. `config-write` is not used: the file on disk is
// already exactly the rendered configuration.
func (r *Renderer) Apply(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	previous := map[string][]byte{}
	for _, p := range files.Paths() {
		if b, err := os.ReadFile(p); err == nil { //nolint:gosec // own file set
			previous[p] = b
		}
	}
	snap, err := renderers.TakeSnapshot(files.Paths()...)
	if err != nil {
		return err
	}
	if err := renderers.WriteFiles(files); err != nil {
		return errors.Join(err, snap.Restore())
	}

	var applied []int
	var actions []error
	for _, fam := range []int{4, 6} {
		f, ok := files[r.paths.conf(fam)]
		if !ok {
			continue
		}
		_, err := r.ctrl.Command(ctx, fam, "config-set", json.RawMessage(f.Content))
		switch {
		case err == nil:
			applied = append(applied, fam)
		case errors.Is(err, ErrNotRunning):
			if active(f.Content) {
				actions = append(actions, &ActionRequired{
					Daemon: fmt.Sprintf("kea-dhcp%d", fam), Unit: fmt.Sprintf("kea-dhcp%d-server", fam),
					Action: "start", Reason: "configuration has interfaces but the server is not running",
				})
			}
		default:
			applied = append(applied, fam) // the failing daemon may have half-applied: set the old config again
			rbErr := r.rollback(ctx, snap, previous, applied)
			return errors.Join(fmt.Errorf("%w: dhcp%d config-set: %w", ErrDaemon, fam, err), rbErr)
		}
	}
	if f, ok := files[r.paths.CtrlAgentConf()]; ok && r.agent != nil && !bytes.Equal(previous[r.paths.CtrlAgentConf()], f.Content) {
		if _, err := r.agent.Command(ctx, "", "config-reload", nil); err != nil && !errors.Is(err, ErrNotRunning) {
			rbErr := r.rollback(ctx, snap, previous, applied)
			return errors.Join(fmt.Errorf("%w: kea-ctrl-agent config-reload: %w", ErrDaemon, err), rbErr)
		}
	}
	return errors.Join(actions...)
}

// rollback restores the files and sets the previous configuration on the given families.
func (r *Renderer) rollback(ctx context.Context, snap *renderers.Snapshot, previous map[string][]byte, families []int) error {
	errs := []error{snap.Restore()}
	for _, fam := range families {
		old, ok := previous[r.paths.conf(fam)]
		if !ok || !json.Valid(old) {
			continue
		}
		if _, err := r.ctrl.Command(ctx, fam, "config-set", json.RawMessage(old)); err != nil && !errors.Is(err, ErrNotRunning) {
			errs = append(errs, fmt.Errorf("kea: rollback dhcp%d config-set: %w", fam, err))
		}
	}
	return errors.Join(errs...)
}

// active reports whether a rendered Dhcp4/Dhcp6 config binds at least one interface.
func active(content []byte) bool {
	var root map[string]struct {
		InterfacesConfig struct {
			Interfaces []string `json:"interfaces"`
		} `json:"interfaces-config"`
	}
	if err := json.Unmarshal(content, &root); err != nil {
		return true
	}
	for _, v := range root {
		if len(v.InterfacesConfig.Interfaces) > 0 {
			return true
		}
	}
	return false
}

// Retrieve implements renderers.Renderer: a structpb.Struct {"dhcp4": DaemonState,
// "dhcp6": DaemonState} (the proto has no DHCP state message yet: RF-3-questions Q2).
func (r *Renderer) Retrieve(ctx context.Context) (proto.Message, error) {
	st, err := r.State(ctx)
	if err != nil {
		return nil, err
	}
	return toStruct(st)
}

func toStruct(v any) (*structpb.Struct, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("kea: state: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("kea: state: %w", err)
	}
	return structpb.NewStruct(m)
}

// toolMessage condenses checker output for an error, with the staging dir hidden.
func toolMessage(out renderers.Output, err error, stagingDir string) string {
	msg := bytes.TrimSpace(append(append([]byte{}, out.Stdout...), out.Stderr...))
	var lines [][]byte
	for _, l := range bytes.Split(msg, []byte("\n")) {
		if bytes.Contains(l, []byte("ERROR")) || bytes.Contains(l, []byte("Error")) || bytes.Contains(l, []byte("error")) {
			lines = append(lines, l)
		}
	}
	if len(lines) > 0 {
		msg = bytes.Join(lines, []byte("; "))
	}
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
