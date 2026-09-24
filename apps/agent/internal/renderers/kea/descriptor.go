package kea

import (
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

// The Kea renderer inside the agent (F-kea-dhcp-relay, D-109 d): no renderer stage exists in the agent core
// (service.go runs only the scheduler), so each Kea daemon is one singleton scheduler descriptor —
//
//	kea.dhcp4/vrx, kea.dhcp6/vrx   Value = Input(document, family) (*vrxv1.DesiredState: that family's servers
//	                                plus the interface addresses of the DHCPv4 bindings)
//	Create / Update                 RenderFamily → Validate (kea-dhcp<N> -t) → Apply (atomic write + config-set)
//	Delete                          the idle configuration of the family (no interfaces, no subnets)
//	Retrieve                        config-get (daemon running) or the file the daemon loads at start (not
//	                                running) → the embedded input → re-rendered → ConfigDrift against what the
//	                                daemon runs: equal → the input is the Value; different → a drift Value
//	                                (*structpb.Struct) that never equals a desired one, so the reconciler re-applies
//
// A daemon that is not running with an active configuration is not an error: the files are written and the start
// request is reported by Status on every read until the daemon runs (D-079). The object exists only while the
// document has a server of the family; Retrieve ignores configurations this renderer did not write (no embedded
// input), so a foreign /etc/kea file is never reported, deleted or rewritten unless servers are configured.

// Descriptor names (the first key segment) and the singleton object id.
const (
	NameDhcp4 = "kea.dhcp4"
	NameDhcp6 = "kea.dhcp6"
	ObjectID  = "vrx"
)

// DescriptorName returns the descriptor name of family 4 or 6.
func DescriptorName(family int) string {
	if family == 6 {
		return NameDhcp6
	}
	return NameDhcp4
}

// Key returns the key of the singleton object of family 4 or 6.
func Key(family int) scheduler.Key { return scheduler.Join(DescriptorName(family), ObjectID) }

// maxDriftLines bounds the drift list carried in a drift Value.
const maxDriftLines = 20

// Descriptor is the scheduler descriptor of one Kea daemon.
type Descriptor struct {
	r        *Renderer
	family   int
	ifaceKey func(string) scheduler.Key
	log      *slog.Logger
}

var _ scheduler.Descriptor = (*Descriptor)(nil)

// DescriptorOption configures a Descriptor.
type DescriptorOption func(*Descriptor)

// WithDescriptorInterfaceKey sets the key of the interfaces a server names (default "interface/<name>", D-065);
// they are optional dependencies (ordering only: Kea itself tolerates an interface that appears later).
func WithDescriptorInterfaceKey(f func(string) scheduler.Key) DescriptorOption {
	return func(d *Descriptor) {
		if f != nil {
			d.ifaceKey = f
		}
	}
}

// WithLogger sets the logger (start requests, drift).
func WithLogger(l *slog.Logger) DescriptorOption {
	return func(d *Descriptor) {
		if l != nil {
			d.log = l
		}
	}
}

// NewDescriptor returns the descriptor of family 4 or 6 driving r.
func NewDescriptor(r *Renderer, family int, opts ...DescriptorOption) *Descriptor {
	d := &Descriptor{r: r, family: family, log: slog.Default(),
		ifaceKey: func(n string) scheduler.Key { return scheduler.Join("interface", n) }}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Name implements scheduler.Descriptor.
func (d *Descriptor) Name() string { return DescriptorName(d.family) }

// KeyOf implements scheduler.Descriptor (a singleton).
func (d *Descriptor) KeyOf(proto.Message) scheduler.Key { return Key(d.family) }

// Dependencies implements scheduler.Descriptor: the interfaces the servers name (optional).
func (d *Descriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	in, ok := obj.(*vrxv1.DesiredState)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var deps []scheduler.Dependency
	servers := in.GetServices().GetDhcp().GetServers()
	for _, name := range sortedKeys(servers) {
		for _, ifn := range servers[name].GetInterfaces() {
			if !seen[ifn] {
				seen[ifn] = true
				deps = append(deps, scheduler.Dependency{Key: d.ifaceKey(ifn), Optional: true})
			}
		}
	}
	return deps
}

// Create implements scheduler.Descriptor.
func (d *Descriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	in, ok := obj.(*vrxv1.DesiredState)
	if !ok {
		return nil, fmt.Errorf("%w: %s value is %T, want *vrx.v1.DesiredState", ErrInvalid, d.Name(), obj)
	}
	return nil, d.apply(ctx, in)
}

// Update implements scheduler.Descriptor: the new configuration replaces the running one in place (config-set).
func (d *Descriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: the family goes back to the idle configuration.
func (d *Descriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	return d.apply(ctx, nil)
}

func (d *Descriptor) apply(ctx context.Context, in *vrxv1.DesiredState) error {
	files, err := d.r.RenderFamily(in, d.family)
	if err != nil {
		return err
	}
	if err := d.r.Validate(ctx, files); err != nil {
		return err
	}
	err = d.r.Apply(ctx, files)
	var ar *ActionRequired
	if errors.As(err, &ar) {
		// Files written; the daemon loads them when it starts. Status reports the start request on every read
		// until the daemon runs (D-079) — not a transaction failure.
		d.log.Warn("kea configuration written; the daemon must be started", "daemon", ar.Daemon, "unit", ar.Unit, "action", ar.Action, "reason", ar.Reason)
		return nil
	}
	return err
}

// Retrieve implements scheduler.Descriptor (see the package comment of this file).
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	cfg, running, err := d.r.actualConfig(ctx, d.family)
	if err != nil || cfg == nil {
		return nil, err
	}
	in, ok, err := EmbeddedInput(cfg)
	if err != nil {
		d.log.Warn("kea configuration carries an unreadable render input; reported as drift", "daemon", d.Name(), "err", err)
		return []scheduler.KV{{Key: Key(d.family), Value: driftValue(running, []string{err.Error()})}}, nil
	}
	if !ok {
		return nil, nil // idle, or not written by this renderer
	}
	want, err := d.r.RenderFamily(in, d.family)
	if err != nil {
		return []scheduler.KV{{Key: Key(d.family), Value: driftValue(running, []string{"re-render: " + err.Error()})}}, nil
	}
	diffs, err := ConfigDrift(want[d.r.paths.conf(d.family)].Content, cfg)
	if err != nil {
		return nil, err
	}
	if len(diffs) > 0 {
		return []scheduler.KV{{Key: Key(d.family), Value: driftValue(running, diffs)}}, nil
	}
	return []scheduler.KV{{Key: Key(d.family), Value: in}}, nil
}

// driftValue is the Value of a configuration that is ours but not the rendering of its own input: it never equals
// a desired Value (a different message type), so the reconciler updates (or deletes) it.
func driftValue(running bool, diffs []string) *structpb.Struct {
	if len(diffs) > maxDriftLines {
		diffs = append(diffs[:maxDriftLines:maxDriftLines], fmt.Sprintf("… %d more", len(diffs)-maxDriftLines))
	}
	list := make([]any, len(diffs))
	for i, s := range diffs {
		list[i] = s
	}
	v, _ := structpb.NewStruct(map[string]any{"running": running, "drift": list})
	return v
}

// actualConfig is what the daemon of a family runs: config-get when its control socket answers, otherwise the
// file it loads at start (nil when there is none). running reports which one.
func (r *Renderer) actualConfig(ctx context.Context, family int) (cfg []byte, running bool, err error) {
	cfg, err = r.ConfigGet(ctx, family)
	switch {
	case err == nil:
		return cfg, true, nil
	case !errors.Is(err, ErrNotRunning):
		return nil, false, err
	}
	b, rerr := os.ReadFile(r.paths.conf(family)) //nolint:gosec // own config file
	if errors.Is(rerr, os.ErrNotExist) {
		return nil, false, nil
	}
	if rerr != nil {
		return nil, false, fmt.Errorf("kea: read %s: %w", r.paths.conf(family), rerr)
	}
	return b, false, nil
}
