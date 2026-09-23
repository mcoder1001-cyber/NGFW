package ikev2

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ikev2"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// Singleton keys.
var (
	LocalKeyKey      = scheduler.Join(LocalKeyName, "global")
	SleepIntervalKey = scheduler.Join(SleepIntervalName, "global")
	LivenessKey      = scheduler.Join(LivenessName, "global")
)

// lastApplied is the "Retrieve reports what this process applied" cache of a singleton VPP
// offers no getter for. After an agent restart it is empty, so the scheduler re-applies the
// desired value once — the setters are idempotent.
type lastApplied[T proto.Message] struct {
	mu sync.Mutex
	v  T
	ok bool
}

func (l *lastApplied[T]) set(v T) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.v, l.ok = proto.Clone(v).(T), true
}

func (l *lastApplied[T]) forget() {
	l.mu.Lock()
	defer l.mu.Unlock()
	var zero T
	l.v, l.ok = zero, false
}

func (l *lastApplied[T]) kvs(key scheduler.Key) []scheduler.KV {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.ok {
		return nil
	}
	return []scheduler.KV{{Key: key, Value: proto.Clone(l.v)}}
}

// ---- ikev2.local-key ------------------------------------------------------------------------

// LocalKey sets the responder's private key file (ikev2_set_local_key), needed by profiles with
// rsa-sig auth. The file is the secret: the agent passes its path and never reads it. VPP has no
// getter; Retrieve reports the last path this process applied, Delete forgets it (VPP keeps the
// loaded key — there is no "unset").
type LocalKey struct {
	cfg  Config
	last lastApplied[*vpnpb.Ikev2LocalKey]
}

// NewLocalKey returns the descriptor.
func NewLocalKey(cfg Config) *LocalKey { return &LocalKey{cfg: cfg} }

// Name implements scheduler.Descriptor.
func (*LocalKey) Name() string { return LocalKeyName }

// KeyOf implements scheduler.Descriptor.
func (*LocalKey) KeyOf(proto.Message) scheduler.Key { return LocalKeyKey }

// Dependencies implements scheduler.Descriptor (none).
func (*LocalKey) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor.
func (d *LocalKey) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.Ikev2LocalKey)
	if !ok {
		return nil, typeErr(LocalKeyName, obj)
	}
	if o.GetKeyFile() == "" || len(o.GetKeyFile()) > 255 || strings.ContainsRune(o.GetKeyFile(), 0) {
		return nil, errors.New("ikev2: local key_file must be a path of 1–255 bytes")
	}
	if _, err := ikev2.NewServiceClient(d.cfg.Client).Ikev2SetLocalKey(ctx, &ikev2.Ikev2SetLocalKey{KeyFile: o.GetKeyFile()}); err != nil {
		return nil, fmt.Errorf("ikev2_set_local_key (%s): %w", o.GetKeyFile(), err)
	}
	d.last.set(o)
	return nil, nil
}

// Update implements scheduler.Descriptor.
func (d *LocalKey) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor (forgets the cached value only).
func (d *LocalKey) Delete(context.Context, proto.Message, any) error {
	d.last.forget()
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *LocalKey) Retrieve(context.Context) ([]scheduler.KV, error) {
	return d.last.kvs(LocalKeyKey), nil
}

// ---- ikev2.sleep-interval -------------------------------------------------------------------

// SleepInterval sets the IKEv2 process sleep interval (ikev2_plugin_set_sleep_interval; read with
// ikev2_get_sleep_interval). Plugin-wide: Retrieve always reports VPP's value, Delete leaves it.
type SleepInterval struct{ cfg Config }

// NewSleepInterval returns the descriptor.
func NewSleepInterval(cfg Config) *SleepInterval { return &SleepInterval{cfg: cfg} }

// Name implements scheduler.Descriptor.
func (*SleepInterval) Name() string { return SleepIntervalName }

// KeyOf implements scheduler.Descriptor.
func (*SleepInterval) KeyOf(proto.Message) scheduler.Key { return SleepIntervalKey }

// Dependencies implements scheduler.Descriptor (none).
func (*SleepInterval) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor.
func (d *SleepInterval) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.Ikev2SleepInterval)
	if !ok {
		return nil, typeErr(SleepIntervalName, obj)
	}
	if o.GetSeconds() <= 0 {
		return nil, errors.New("ikev2: sleep interval must be > 0 seconds")
	}
	if _, err := ikev2.NewServiceClient(d.cfg.Client).Ikev2PluginSetSleepInterval(ctx, &ikev2.Ikev2PluginSetSleepInterval{Timeout: o.GetSeconds()}); err != nil {
		return nil, fmt.Errorf("ikev2_plugin_set_sleep_interval: %w", err)
	}
	return nil, nil
}

// Update implements scheduler.Descriptor.
func (d *SleepInterval) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: a plugin-wide setting cannot be absent.
func (*SleepInterval) Delete(context.Context, proto.Message, any) error { return nil }

// Retrieve implements scheduler.Descriptor.
func (d *SleepInterval) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	rep, err := ikev2.NewServiceClient(d.cfg.Client).Ikev2GetSleepInterval(ctx, &ikev2.Ikev2GetSleepInterval{})
	if err != nil {
		return nil, fmt.Errorf("ikev2_get_sleep_interval: %w", err)
	}
	return []scheduler.KV{{Key: SleepIntervalKey, Value: &vpnpb.Ikev2SleepInterval{Seconds: rep.SleepInterval}}}, nil
}

// ---- ikev2.liveness -------------------------------------------------------------------------

// Liveness sets the plugin-wide dead-peer detection (ikev2_profile_set_liveness — the message has
// no profile name, VPP keeps the values in ikev2_main). No getter: Retrieve reports the last value
// this process applied, Delete forgets it.
type Liveness struct {
	cfg  Config
	last lastApplied[*vpnpb.Ikev2Liveness]
}

// NewLiveness returns the descriptor.
func NewLiveness(cfg Config) *Liveness { return &Liveness{cfg: cfg} }

// Name implements scheduler.Descriptor.
func (*Liveness) Name() string { return LivenessName }

// KeyOf implements scheduler.Descriptor.
func (*Liveness) KeyOf(proto.Message) scheduler.Key { return LivenessKey }

// Dependencies implements scheduler.Descriptor (none).
func (*Liveness) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor.
func (d *Liveness) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.Ikev2Liveness)
	if !ok {
		return nil, typeErr(LivenessName, obj)
	}
	if o.GetPeriod() == 0 || o.GetMaxRetries() == 0 {
		return nil, errors.New("ikev2: liveness period and max_retries must be > 0")
	}
	if _, err := ikev2.NewServiceClient(d.cfg.Client).Ikev2ProfileSetLiveness(ctx, &ikev2.Ikev2ProfileSetLiveness{
		Period: o.GetPeriod(), MaxRetries: o.GetMaxRetries(),
	}); err != nil {
		return nil, fmt.Errorf("ikev2_profile_set_liveness: %w", err)
	}
	d.last.set(o)
	return nil, nil
}

// Update implements scheduler.Descriptor.
func (d *Liveness) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor (forgets the cached value only).
func (d *Liveness) Delete(context.Context, proto.Message, any) error {
	d.last.forget()
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *Liveness) Retrieve(context.Context) ([]scheduler.KV, error) {
	return d.last.kvs(LivenessKey), nil
}
