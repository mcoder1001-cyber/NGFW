package rsyslog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/rfkit"
	"ngfw/agent/internal/scheduler"
)

// The rsyslog export renderer inside the agent (F-unbound-chrony-syslog; D-109 d: one singleton scheduler
// descriptor):
//
//	rsyslog.config/vrx   Value = Input(document) (*vrxv1.ManagementConfig carrying management.syslog only)
//	Create / Update      Render → Validate (rsyslogd -N1 on a staged copy) → Apply (atomic write → restart +
//	                     impstats convergence; with a DeferredController — test slots — a restart request instead)
//	Delete               the empty export (no action)
//	Retrieve             the export file as written → the embedded input → re-rendered → byte-equal (and the TLS
//	                     files): the input is the Value; different: a drift Value
//
// A restart request is not a transaction failure (reported by Pending, SyslogState).

// Descriptor name and singleton object id.
const (
	Name     = "rsyslog.config"
	ObjectID = "vrx"
)

// Key is the key of the singleton object.
var Key = scheduler.Join(Name, ObjectID)

// Descriptor is the scheduler descriptor of the export.
type Descriptor struct {
	r       *Renderer
	log     *slog.Logger
	prepare func() error
}

var _ scheduler.Descriptor = (*Descriptor)(nil)

// NewDescriptor returns the descriptor driving r.
func NewDescriptor(r *Renderer, log *slog.Logger) *Descriptor {
	if log == nil {
		log = slog.Default()
	}
	return &Descriptor{r: r, log: log}
}

// WithPrepare sets a hook run before every write (idempotent: creates the instance's directories). Registration
// itself has no filesystem side effects.
func (d *Descriptor) WithPrepare(f func() error) *Descriptor {
	d.prepare = f
	return d
}

// Renderer returns the renderer this descriptor drives (state RPCs).
func (d *Descriptor) Renderer() *Renderer { return d.r }

// Name implements scheduler.Descriptor.
func (d *Descriptor) Name() string { return Name }

// KeyOf implements scheduler.Descriptor (a singleton).
func (d *Descriptor) KeyOf(proto.Message) scheduler.Key { return Key }

// Dependencies implements scheduler.Descriptor: none.
func (d *Descriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor.
func (d *Descriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	in, ok := obj.(*vrxv1.ManagementConfig)
	if !ok {
		return nil, fmt.Errorf("%w: %s value is %T, want *vrx.v1.ManagementConfig", ErrInput, Name, obj)
	}
	return nil, d.apply(ctx, in)
}

// Update implements scheduler.Descriptor.
func (d *Descriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: the empty export.
func (d *Descriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	return d.apply(ctx, nil)
}

func (d *Descriptor) apply(ctx context.Context, in *vrxv1.ManagementConfig) error {
	if d.prepare != nil {
		if err := d.prepare(); err != nil {
			return err
		}
	}
	var desired proto.Message
	if in != nil {
		desired = in
	}
	files, err := d.r.Render(ctx, desired)
	if err != nil {
		return err
	}
	if err := d.r.Validate(ctx, files); err != nil {
		return err
	}
	err = d.r.Apply(ctx, files)
	var ar *rfkit.ActionRequired
	switch {
	case errors.As(err, &ar):
		d.log.Warn("rsyslog export written; the daemon must act on it", "unit", ar.Unit, "action", ar.Action, "reason", ar.Reason, "conf", d.r.paths.ConfFile)
		return nil
	case err != nil:
		return err
	}
	d.log.Info("rsyslog export applied", "conf", d.r.paths.ConfFile, "targets", len(in.GetSyslog()))
	return nil
}

const maxDriftLines = 20

// Retrieve implements scheduler.Descriptor (see the comment at the top of this file).
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	conf, err := os.ReadFile(d.r.paths.ConfFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("rsyslog: read %s: %w", d.r.paths.ConfFile, err)
	}
	in, ok, err := EmbeddedInput(conf)
	if err != nil {
		return []scheduler.KV{{Key: Key, Value: driftValue([]string{err.Error()})}}, nil
	}
	if !ok {
		return nil, nil
	}
	want, err := d.r.Render(ctx, in)
	if err != nil {
		return []scheduler.KV{{Key: Key, Value: driftValue([]string{"re-render: " + d.r.red.Redact(err.Error())})}}, nil
	}
	var drift []string
	for _, p := range want.Paths() {
		have, err := os.ReadFile(p) //nolint:gosec // own file set
		switch {
		case err != nil:
			drift = append(drift, fmt.Sprintf("%s: %v", p, err))
		case !bytes.Equal(have, want[p].Content):
			drift = append(drift, p+" differs from the rendering of its input")
		}
	}
	if len(drift) > 0 {
		return []scheduler.KV{{Key: Key, Value: driftValue(drift)}}, nil
	}
	return []scheduler.KV{{Key: Key, Value: in}}, nil
}

func driftValue(diffs []string) *structpb.Struct {
	if len(diffs) > maxDriftLines {
		diffs = append(diffs[:maxDriftLines:maxDriftLines], fmt.Sprintf("… %d more", len(diffs)-maxDriftLines))
	}
	list := make([]any, len(diffs))
	for i, s := range diffs {
		list[i] = s
	}
	v, _ := structpb.NewStruct(map[string]any{"drift": list})
	return v
}

// PendingAction is a start/restart rsyslogd still has to perform for the written export.
type PendingAction struct {
	Daemon, Unit, Action, Reason string
}

// Pending reports the restart request a DeferredController recorded and the instance has not acted on, or a start
// when an export with targets is written and the instance does not run. With the product's systemd controller the
// renderer restarts rsyslog itself, so nothing is pending.
func (d *Descriptor) Pending(context.Context) []PendingAction {
	dc, ok := d.r.ctl.(*DeferredController)
	if !ok {
		return nil
	}
	if rec := dc.Pending(); rec != nil {
		return []PendingAction{{Daemon: "rsyslogd", Unit: "rsyslog", Action: rec.Action, Reason: rec.Reason}}
	}
	if dc.PID() == 0 {
		if conf, err := os.ReadFile(d.r.paths.ConfFile); err == nil && bytes.Contains(conf, []byte("action(")) {
			return []PendingAction{{Daemon: "rsyslogd", Unit: "rsyslog", Action: "start", Reason: "the export has targets but the rsyslog instance is not running"}}
		}
	}
	return nil
}
