package unbound

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

// The Unbound renderer inside the agent (F-unbound-chrony-syslog; D-109 d — no renderer stage in the agent core,
// so the renderer is one singleton scheduler descriptor):
//
//	unbound.config/vrx   Value = Input(services.dns) (*vrxv1.DnsService: the resolvers)
//	Create / Update      Render → Validate (unbound-checkconf on a staged copy) → Apply (atomic write → reload_keep_cache
//	                     + convergence check, or a start/restart request: D-079)
//	Delete               the idle configuration (loopback only, no resolver)
//	Retrieve             unbound.conf as written → the embedded input → re-rendered → byte-equal: the input is the
//	                     Value; different: a drift Value (*structpb.Struct) that never equals a desired one, so the
//	                     reconciler re-applies it
//
// A start or restart request (*ActionRequired) is not a transaction failure: the file is written and the request is
// reported by Pending (DnsState) on every read until the daemon runs the new configuration (restart requests are
// persisted in Paths.PendingFile, D-079). The agent never starts or restarts unbound itself; the product's unit (P10)
// or, on a test slot, the slot harness does. A file without an embedded input (Debian's default unbound.conf, an
// idle rendering) is not reported, so the agent never touches a foreign configuration unless resolvers are configured.

// Descriptor name and singleton object id.
const (
	Name     = "unbound.config"
	ObjectID = "vrx"
)

// Key is the key of the singleton object.
var Key = scheduler.Join(Name, ObjectID)

// Descriptor is the scheduler descriptor of the Unbound instance.
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

// Dependencies implements scheduler.Descriptor: none (listen addresses are Linux sockets; unbound binds what exists).
func (d *Descriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor.
func (d *Descriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	in, ok := obj.(*vrxv1.DnsService)
	if !ok {
		return nil, fmt.Errorf("%w: %s value is %T, want *vrx.v1.DnsService", ErrInvalid, Name, obj)
	}
	return nil, d.apply(ctx, in)
}

// Update implements scheduler.Descriptor (reload in place, or a restart request).
func (d *Descriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: back to the idle configuration.
func (d *Descriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	return d.apply(ctx, nil)
}

func (d *Descriptor) apply(ctx context.Context, in *vrxv1.DnsService) error {
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
		d.log.Warn("unbound configuration written; the daemon must act on it", "unit", ar.Unit, "action", ar.Action, "reason", ar.Reason, "conf", d.r.paths.Conf())
		return nil
	case err != nil:
		return err
	}
	d.log.Info("unbound configuration applied", "conf", d.r.paths.Conf(), "resolvers", ResolverNames(in))
	return nil
}

// maxDriftLines bounds the drift list carried in a drift Value.
const maxDriftLines = 20

// Retrieve implements scheduler.Descriptor (see the comment at the top of this file).
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	conf, err := os.ReadFile(d.r.paths.Conf())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("unbound: read %s: %w", d.r.paths.Conf(), err)
	}
	in, ok, err := EmbeddedInput(conf)
	if err != nil {
		return []scheduler.KV{{Key: Key, Value: DriftValue([]string{err.Error()})}}, nil
	}
	if !ok {
		return nil, nil
	}
	want, err := d.r.Render(ctx, in)
	if err != nil {
		return []scheduler.KV{{Key: Key, Value: DriftValue([]string{"re-render: " + err.Error()})}}, nil
	}
	if diffs := LineDiff(want[d.r.paths.Conf()].Content, conf); len(diffs) > 0 {
		return []scheduler.KV{{Key: Key, Value: DriftValue(diffs)}}, nil
	}
	return []scheduler.KV{{Key: Key, Value: in}}, nil
}

// DriftValue is the Value of a configuration that is ours but not the rendering of its own input: a different
// message type, so it never equals a desired Value and the reconciler updates (or deletes) it.
func DriftValue(diffs []string) *structpb.Struct {
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

// LineDiff lists the lines that differ between want and have ("-want" / "+have", in order); nil when equal.
func LineDiff(want, have []byte) []string {
	if bytes.Equal(want, have) {
		return nil
	}
	w, h := strings.Split(string(want), "\n"), strings.Split(string(have), "\n")
	var out []string
	for i := 0; i < len(w) || i < len(h); i++ {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(h) {
			b = h[i]
		}
		if a != b {
			out = append(out, fmt.Sprintf("line %d: -%q +%q", i+1, a, b))
		}
	}
	if len(out) == 0 {
		out = []string{"content differs"}
	}
	return out
}

// PendingAction is a start/restart the daemon still has to perform for the written configuration.
type PendingAction struct {
	Daemon, Unit, Action, Reason string
}

// Pending reports what unbound still has to do for the written configuration: a persisted restart request
// (D-079) whose process has not restarted since, or a start when an active configuration is written and the
// daemon does not run. A restart request the daemon has acted on is cleared here too.
func (d *Descriptor) Pending(ctx context.Context) []PendingAction {
	var out []PendingAction
	running := d.r.running()
	if rec := getPending(d.r.paths.PendingFile); rec != nil {
		pid, err := d.r.daemonPid(ctx)
		switch {
		case running && err == nil && rec.startedAfter(pid):
			clearPending(d.r.paths.PendingFile)
		case running:
			out = append(out, PendingAction{Daemon: "unbound", Unit: "unbound", Action: rec.Action, Reason: rec.Reason})
		}
	}
	if !running {
		if conf, err := os.ReadFile(d.r.paths.Conf()); err == nil && Active(conf) {
			out = append(out, PendingAction{Daemon: "unbound", Unit: "unbound", Action: "start", Reason: "configuration has resolvers but unbound is not running"})
		}
	}
	return out
}

// RecordsNoOwnership declares (TD-11b, dfkit/persist) that the descriptor records no ownership in any claim or boot
// store: its object is ours exactly when the rendered file carries our embedded input (Retrieve), a fact the file
// itself keeps across agent restarts.
func (d *Descriptor) RecordsNoOwnership() {}
