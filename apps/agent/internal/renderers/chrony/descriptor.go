package chrony

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
	"ngfw/agent/internal/scheduler"
)

// The chrony renderer inside the agent (F-unbound-chrony-syslog; D-109 d: one singleton scheduler descriptor):
//
//	chrony.config/vrx   Value = Input(services.ntp) (*vrxv1.NtpService, only while enabled)
//	Create / Update     Render → Validate (chronyd -p on staged copies) → Apply (atomic write → `chronyc reload
//	                    sources` / `rekey`, or a start/restart request: chrony.conf changes need a restart, D-079)
//	Delete              the disabled rendering (no sources, no server)
//	Retrieve            vrx.sources as written → the embedded input → re-rendered → chrony.conf, vrx.sources and
//	                    chrony.keys byte-equal: the input is the Value; different: a drift Value
//
// A start/restart request is not a transaction failure (reported by Pending, NtpState). The agent never starts or
// restarts chronyd itself.

// Descriptor name and singleton object id.
const (
	Name     = "chrony.config"
	ObjectID = "vrx"
)

// Key is the key of the singleton object.
var Key = scheduler.Join(Name, ObjectID)

// Descriptor is the scheduler descriptor of the chronyd instance.
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
	in, ok := obj.(*vrxv1.NtpService)
	if !ok {
		return nil, fmt.Errorf("%w: %s value is %T, want *vrx.v1.NtpService", ErrInvalid, Name, obj)
	}
	return nil, d.apply(ctx, in)
}

// Update implements scheduler.Descriptor.
func (d *Descriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: the disabled rendering.
func (d *Descriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	return d.apply(ctx, nil)
}

func (d *Descriptor) apply(ctx context.Context, in *vrxv1.NtpService) error {
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
	var ar *ActionRequired
	switch {
	case errors.As(err, &ar):
		d.log.Warn("chrony configuration written; the daemon must act on it", "unit", ar.Unit, "action", ar.Action, "reason", ar.Reason, "conf", d.r.paths.Conf())
		return nil
	case err != nil:
		return err
	}
	d.log.Info("chrony configuration applied", "conf", d.r.paths.Conf(), "enabled", in != nil)
	return nil
}

const maxDriftLines = 20

// Retrieve implements scheduler.Descriptor (see the comment at the top of this file).
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	src, err := os.ReadFile(d.r.paths.Sources())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("chrony: read %s: %w", d.r.paths.Sources(), err)
	}
	in, ok, err := EmbeddedInput(src)
	if err != nil {
		return []scheduler.KV{{Key: Key, Value: driftValue([]string{err.Error()})}}, nil
	}
	if !ok {
		return nil, nil
	}
	want, err := d.r.Render(ctx, in)
	if err != nil {
		return []scheduler.KV{{Key: Key, Value: driftValue([]string{"re-render: " + err.Error()})}}, nil
	}
	var drift []string
	for _, p := range want.Paths() {
		have, err := os.ReadFile(p) //nolint:gosec // own file set
		switch {
		case err != nil:
			drift = append(drift, fmt.Sprintf("%s: %v", p, err))
		case !bytes.Equal(have, want[p].Content):
			// never quote chrony.keys (secret): name the file only
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

// PendingAction is a start/restart chronyd still has to perform for the written configuration.
type PendingAction struct {
	Daemon, Unit, Action, Reason string
}

// Pending reports what chronyd still has to do: a persisted restart request (D-079) whose process has not
// restarted since (cleared here once it has), or a start when an enabled configuration is written and chronyd does
// not run.
func (d *Descriptor) Pending(context.Context) []PendingAction {
	var out []PendingAction
	running := d.r.running()
	if rec := getPending(d.r.paths.PendingFile); rec != nil && running {
		if pid, err := d.r.daemonPid(); err == nil && rec.startedAfter(pid) {
			clearPending(d.r.paths.PendingFile)
		} else {
			out = append(out, PendingAction{Daemon: "chronyd", Unit: d.r.paths.Unit, Action: rec.Action, Reason: rec.Reason})
		}
	}
	if !running {
		if conf, err := os.ReadFile(d.r.paths.Conf()); err == nil && !bytes.Contains(conf, []byte("\n# services.ntp is disabled")) {
			out = append(out, PendingAction{Daemon: "chronyd", Unit: d.r.paths.Unit, Action: "start", Reason: "services.ntp is enabled but chronyd is not running"})
		}
	}
	return out
}

// RecordsNoOwnership declares (TD-11b, dfkit/persist) that the descriptor records no ownership in any claim or boot
// store: its object is ours exactly when the rendered file carries our embedded input (Retrieve), a fact the file
// itself keeps across agent restarts.
func (d *Descriptor) RecordsNoOwnership() {}
