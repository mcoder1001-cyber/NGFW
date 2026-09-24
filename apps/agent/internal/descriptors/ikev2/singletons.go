package ikev2

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ikev2"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// Singleton keys.
var (
	LocalKeyKey      = scheduler.Join(LocalKeyName, "global")
	SleepIntervalKey = scheduler.Join(SleepIntervalName, "global")
	LivenessKey      = scheduler.Join(LivenessName, "global")
)

// ---- ikev2.local-key ------------------------------------------------------------------------

// LocalKey sets the responder's private key file (ikev2_set_local_key), needed by profiles with
// rsa-sig auth. The file is the secret: the agent passes its path and never reads it. A VPP-global
// (D-071): registered as setter only for the globals owner. VPP has no getter, so the descriptor
// is write-only (D-063): Retrieve returns vpn.ErrRetrieveUnsupported and the reconciler re-applies
// the path on resync — idempotent (D-076): VPP frees the loaded key and loads the file again.
// Delete leaves VPP's loaded key (there is no unset).
type LocalKey struct{ cfg Config }

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
	return nil, nil
}

// Update implements scheduler.Descriptor.
func (d *LocalKey) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: a no-op (see the type doc).
func (*LocalKey) Delete(context.Context, proto.Message, any) error { return nil }

// Retrieve implements scheduler.Descriptor: VPP has no getter (D-063, write-only).
func (*LocalKey) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", LocalKeyName, vpn.ErrRetrieveUnsupported)
}

// ---- ikev2.sleep-interval -------------------------------------------------------------------

// SleepInterval sets the IKEv2 process sleep interval (ikev2_plugin_set_sleep_interval; read with
// ikev2_get_sleep_interval). A VPP-global (D-071): the globals owner's setter reports VPP's value
// and never deletes on absence; everybody else requires it through current.
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

// current is the Require getter (D-071).
func (d *SleepInterval) current(ctx context.Context, _ proto.Message) (proto.Message, bool, error) {
	kvs, err := d.Retrieve(ctx)
	if err != nil || len(kvs) == 0 {
		return nil, false, err
	}
	return kvs[0].Value, true, nil
}

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
// no profile name, VPP keeps the values in ikev2_main). A VPP-global (D-071): setter only for the
// globals owner. No getter: write-only (D-063), Retrieve returns vpn.ErrRetrieveUnsupported; the
// re-apply is idempotent (D-076: VPP stores the two values); Delete leaves VPP as it is.
type Liveness struct{ cfg Config }

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
	return nil, nil
}

// Update implements scheduler.Descriptor.
func (d *Liveness) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: a no-op (see the type doc).
func (*Liveness) Delete(context.Context, proto.Message, any) error { return nil }

// Retrieve implements scheduler.Descriptor: VPP has no getter (D-063, write-only).
func (*Liveness) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", LivenessName, vpn.ErrRetrieveUnsupported)
}
