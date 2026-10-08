package sysident

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/scheduler"
)

// The system-identity renderer inside the agent (F-system-identity; D-109 d: one singleton scheduler descriptor):
//
//	system.identity/ngfw   Value = Input(system) (*ngfwv1.SystemConfig, normalised)
//	Validate              Check: host name, IANA zone with an existing zone file, banners without control
//	                      characters (D-049), IP name servers, search domains, VRF = default (scheduler.Validator)
//	Create / Update       Render → write only the files whose content differs (atomic), re-point /etc/localtime
//	                      when its target differs, sethostname(2) when the kernel name differs (globals owner)
//	Delete                removes the resolver drop-in only (the host keeps its last name, zone and banners)
//	Retrieve              the drop-in's embedded input → re-rendered → every file and the symlink equal: the input
//	                      is the Value; different: a drift Value (→ Update)
//
// systemd-resolved reads its drop-ins at start: a DNS change is logged as a pending `systemctl restart
// systemd-resolved` (the agent never restarts host daemons itself, D-079 / PENDING-agent-privileges).

// Descriptor name and singleton object id.
const (
	Name     = "system.identity"
	ObjectID = "ngfw"
)

// Key is the key of the singleton object.
var Key = scheduler.Join(Name, ObjectID)

// Descriptor is the scheduler descriptor of the system identity.
type Descriptor struct {
	paths       Paths
	log         *slog.Logger
	prepare     func() error
	provisioned func() error
	hostname    func() (string, error)
	sethost     func(string) error
}

var (
	_ scheduler.Descriptor = (*Descriptor)(nil)
	_ scheduler.Validator  = (*Descriptor)(nil)
	_ scheduler.Stager     = (*Descriptor)(nil)
)

// New returns the descriptor rendering into p.
func New(p Paths, log *slog.Logger) *Descriptor {
	if log == nil {
		log = slog.Default()
	}
	d := &Descriptor{
		paths: p, log: log,
		hostname: os.Hostname,
		sethost:  func(h string) error { return syscall.Sethostname([]byte(h)) },
	}
	if p == ProductPaths() {
		d.provisioned = func() error { return verifyProductPaths("/", 0) }
	}
	return d
}

// WithPrepare sets a hook run before every write (creates a slot's directories).
func (d *Descriptor) WithPrepare(f func() error) *Descriptor {
	d.prepare = f
	return d
}

// Paths returns the paths the descriptor renders into.
func (d *Descriptor) Paths() Paths { return d.paths }

// Name implements scheduler.Descriptor.
func (d *Descriptor) Name() string { return Name }

// KeyOf implements scheduler.Descriptor (a singleton).
func (d *Descriptor) KeyOf(proto.Message) scheduler.Key { return Key }

// Dependencies implements scheduler.Descriptor: none.
func (d *Descriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Stage implements scheduler.Stager: host configuration, after the VPP objects of a transaction.
func (d *Descriptor) Stage() scheduler.Stage { return scheduler.StageDaemon }

// Validate implements scheduler.Validator: Check, the finding pointing at the offending leaf. Read only.
func (d *Descriptor) Validate(_ context.Context, _ scheduler.Key, value proto.Message, _ scheduler.ReadOnlyView) error {
	in, ok := value.(*ngfwv1.SystemConfig)
	if !ok {
		return fmt.Errorf("%w: %s value is %T, want *ngfw.v1.SystemConfig", ErrInvalid, Name, value)
	}
	if err := Check(Input(in), d.paths.ZoneinfoDir); err != nil {
		var fe *FieldError
		if errors.As(err, &fe) {
			return scheduler.InvalidAt(fe.Pointer, errors.New(fe.Msg))
		}
		return err
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *Descriptor) Create(_ context.Context, obj proto.Message) (any, error) {
	in, ok := obj.(*ngfwv1.SystemConfig)
	if !ok {
		return nil, fmt.Errorf("%w: %s value is %T, want *ngfw.v1.SystemConfig", ErrInvalid, Name, obj)
	}
	return nil, d.Apply(Input(in))
}

// Update implements scheduler.Descriptor.
func (d *Descriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: the resolver drop-in goes; hostname, zone and banners stay as they are.
func (d *Descriptor) Delete(context.Context, proto.Message, any) error {
	err := os.Remove(d.paths.ResolvedDropIn)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("sysident: %w", err)
	}
	if err == nil {
		d.log.Warn("system resolver drop-in removed; systemd-resolved must be restarted to drop it", "unit", "systemd-resolved", "action", "restart")
	}
	return nil
}

// Changes is what one Apply changed (tests and the log).
type Changes struct {
	Files     []string
	Localtime bool
	Kernel    bool
}

// Apply renders in (normalised) and writes what differs. An unchanged input writes nothing.
func (d *Descriptor) Apply(in *ngfwv1.SystemConfig) error {
	_, err := d.apply(in)
	return err
}

func (d *Descriptor) apply(in *ngfwv1.SystemConfig) (Changes, error) {
	var ch Changes
	if err := d.paths.Validate(); err != nil {
		return ch, err
	}
	if err := Check(in, d.paths.ZoneinfoDir); err != nil {
		return ch, err
	}
	r, err := Render(in, d.paths)
	if err != nil {
		return ch, err
	}
	if d.provisioned != nil {
		if err := d.provisioned(); err != nil {
			return ch, err
		}
	}
	if d.prepare != nil {
		if err := d.prepare(); err != nil {
			return ch, err
		}
	}
	for _, dir := range d.paths.Dirs() {
		if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // /etc-like directories
			return ch, fmt.Errorf("sysident: %w", err)
		}
	}
	changed := renderers.Files{}
	for _, p := range r.Files.Paths() {
		old, err := os.ReadFile(p) //nolint:gosec // own file set
		if err != nil || !bytes.Equal(old, r.Files[p].Content) {
			changed[p] = r.Files[p]
			ch.Files = append(ch.Files, p)
		}
	}
	snap, err := renderers.TakeSnapshot(changed.Paths()...)
	if err != nil {
		return ch, err
	}
	if err := renderers.WriteFiles(changed); err != nil {
		return ch, errors.Join(err, snap.Restore())
	}
	if cur, err := os.Readlink(d.paths.Localtime); err != nil || cur != r.Zonefile {
		if err := symlinkAtomic(r.Zonefile, d.paths.Localtime); err != nil {
			return ch, errors.Join(err, snap.Restore())
		}
		ch.Localtime = true
	}
	if d.paths.SetKernelHostname {
		if cur, err := d.hostname(); err != nil || cur != in.GetHostname() {
			if err := d.sethost(in.GetHostname()); err != nil {
				return ch, fmt.Errorf("sysident: sethostname: %w", err)
			}
			ch.Kernel = true
		}
	}
	if len(ch.Files) == 0 && !ch.Localtime && !ch.Kernel {
		d.log.Debug("system identity unchanged; nothing written")
		return ch, nil
	}
	d.log.Info("system identity applied", "hostname", in.GetHostname(), "timezone", in.GetTimezone(), "files", ch.Files, "localtime", ch.Localtime, "kernel_hostname", ch.Kernel)
	if _, ok := changed[d.paths.ResolvedDropIn]; ok {
		d.log.Warn("system resolver configuration written; systemd-resolved must be restarted to read it", "unit", "systemd-resolved", "action", "restart", "dropin", d.paths.ResolvedDropIn)
	}
	return ch, nil
}

// symlinkAtomic points link at target: a temporary symlink renamed over link.
func symlinkAtomic(target, link string) error {
	tmp := filepath.Join(filepath.Dir(link), "."+filepath.Base(link)+".ngfw-tmp")
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return fmt.Errorf("sysident: symlink %s: %w", link, err)
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("sysident: rename over %s: %w", link, err)
	}
	return nil
}

const maxDriftLines = 20

// Retrieve implements scheduler.Descriptor (see the comment at the top of this file).
func (d *Descriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	src, err := os.ReadFile(d.paths.ResolvedDropIn)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sysident: read %s: %w", d.paths.ResolvedDropIn, err)
	}
	in, ok, err := EmbeddedInput(src)
	if err != nil {
		return []scheduler.KV{{Key: Key, Value: driftValue([]string{err.Error()})}}, nil
	}
	if !ok {
		return nil, nil
	}
	drift := d.Drift(in)
	if len(drift) > 0 {
		return []scheduler.KV{{Key: Key, Value: driftValue(drift)}}, nil
	}
	return []scheduler.KV{{Key: Key, Value: in}}, nil
}

// Drift lists how the host differs from the rendering of in (nil: in sync).
func (d *Descriptor) Drift(in *ngfwv1.SystemConfig) []string {
	if d.provisioned != nil {
		if err := d.provisioned(); err != nil {
			return []string{err.Error()}
		}
	}

	want, err := Render(in, d.paths)
	if err != nil {
		return []string{"re-render: " + err.Error()}
	}
	var drift []string
	for _, p := range want.Files.Paths() {
		have, err := os.ReadFile(p) //nolint:gosec // own file set
		switch {
		case err != nil:
			drift = append(drift, fmt.Sprintf("%s: %v", p, err))
		case !bytes.Equal(have, want.Files[p].Content):
			drift = append(drift, p+" differs from the rendering of its input")
		}
	}
	if cur, err := os.Readlink(d.paths.Localtime); err != nil || cur != want.Zonefile {
		drift = append(drift, fmt.Sprintf("%s does not point at %s", d.paths.Localtime, want.Zonefile))
	}
	if d.paths.SetKernelHostname {
		if cur, err := d.hostname(); err != nil || cur != in.GetHostname() {
			drift = append(drift, fmt.Sprintf("kernel host name is %q, not %q", cur, in.GetHostname()))
		}
	}
	return drift
}

func driftValue(diffs []string) *structpb.Struct {
	if len(diffs) > maxDriftLines {
		diffs = append(diffs[:maxDriftLines:maxDriftLines], fmt.Sprintf("… %d more", len(diffs)-maxDriftLines))
	}
	list := make([]any, len(diffs))
	for i, s := range diffs {
		list[i] = strings.ToValidUTF8(s, "?")
	}
	v, _ := structpb.NewStruct(map[string]any{"drift": list})
	return v
}

// RecordsNoOwnership declares (TD-11b, dfkit/persist) that the descriptor records no ownership in any claim or boot
// store: its object is ours exactly when the rendered drop-in carries our embedded input (Retrieve).
func (d *Descriptor) RecordsNoOwnership() {}
