// Package autosdl holds the reconciler descriptor of VPP's auto_sdl plugin (WBS D2.4,
// F-rpf-adl-pbr): the automatic source deny list of the VPP host stack — a source that exceeds
// `threshold` hits (TCP SYN floods towards the session layer) gets an SDL deny entry that expires
// after `remove_timeout` seconds. Message names come only from apps/agent/binapi/auto_sdl;
// docs/agent/descriptors/auto_sdl.md is the object ↔ message table.
//
// auto_sdl_config is a VPP-global singleton without a getter: Retrieve returns
// dfkit.ErrRetrieveUnsupported (write-only, D-063), and only the globals owner registers the
// descriptor (D-071; slot agents on the shared host never do). VPP 26.06 specifics
// (plugins/auto_sdl/auto_sdl.c):
//   - the call answers FEATURE_DISABLED unless the session layer's SDL backend is enabled
//     (session_sdl_is_enabled; startup.conf `session { rt-backend sdl }`) — ErrSessionSDLDisabled;
//   - a second enable while enabled is a silent no-op that keeps the OLD threshold and timeout
//     (the callback registration fails first and the handler ignores the error), so Create sends
//     disable, then enable — once per VPP instance: an applied-once record in the owner's
//     BootStore (D-076, keyed by the D-080 boot identity) skips the re-apply of a resync;
//   - disable removes every automatic entry; Delete disables only what this owner enabled on the
//     running VPP instance.
package autosdl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	autosdlapi "ngfw/agent/binapi/auto_sdl"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Name is the descriptor name; the singleton's key is "auto-sdl.config/global".
const Name = "auto-sdl.config"

// ID is the object id of the singleton.
const ID = "global"

// Key is the key of the singleton.
var Key = scheduler.Join(Name, ID)

// VPP's defaults (auto_sdl.api: threshold [default=5], remove_timeout [default=300]).
const (
	DefaultThreshold     = 5
	DefaultRemoveTimeout = 300
)

// ErrSessionSDLDisabled is returned (wrapped) when VPP refuses the call because the session
// layer's SDL backend is off (startup.conf; handover-gated on vrx-a). Host tests skip on it.
var ErrSessionSDLDisabled = errors.New("auto_sdl needs the session layer's SDL backend (startup.conf session { rt-backend sdl })")

// Config is the auto-sdl.config singleton (auto_sdl_config).
type Config struct {
	Enable        bool   `json:"enable"`
	Threshold     uint32 `json:"threshold"`
	RemoveTimeout uint32 `json:"remove_timeout"`
}

// Proto returns the canonical structpb document.
func (c Config) Proto() *structpb.Struct { return dfkit.Encode(c) }

// Validate checks what VPP would misread (0 would deny on the first hit / expire at once).
func (c Config) Validate() error {
	switch {
	case !c.Enable:
		return dfkit.Specf("auto-sdl: only the enabled state is desired (omit the object to disable)")
	case c.Threshold == 0:
		return dfkit.Specf("auto-sdl: threshold must be > 0")
	case c.RemoveTimeout == 0:
		return dfkit.Specf("auto-sdl: remove_timeout must be > 0")
	}
	return nil
}

// Descriptor manages the singleton (see the package doc).
type Descriptor struct {
	client vpp.Client
	boot   dfkit.BootStore
	mu     sync.Mutex
}

var _ scheduler.Descriptor = (*Descriptor)(nil)

// New returns the descriptor. boot must be the owner's persisted store (subsystems.Wiring.BootStore),
// else an agent restart would disable and re-enable (flushing every automatic entry). A nil store
// panics (programming error).
func New(client vpp.Client, boot dfkit.BootStore) *Descriptor {
	if boot == nil {
		panic("autosdl: New needs a BootStore (D-076)")
	}
	return &Descriptor{client: client, boot: boot}
}

// Name implements scheduler.Descriptor.
func (*Descriptor) Name() string { return Name }

// KeyOf implements scheduler.Descriptor.
func (*Descriptor) KeyOf(proto.Message) scheduler.Key { return Key }

// Dependencies implements scheduler.Descriptor: none.
func (*Descriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *Descriptor) config(ctx context.Context, c Config) error {
	_, err := autosdlapi.NewServiceClient(d.client).AutoSdlConfig(ctx, &autosdlapi.AutoSdlConfig{Enable: c.Enable, Threshold: c.Threshold, RemoveTimeout: c.RemoveTimeout})
	switch {
	case err == nil:
		return nil
	case dfkit.IsVPPError(err, api.FEATURE_DISABLED):
		return fmt.Errorf("auto_sdl_config: %w (%v)", ErrSessionSDLDisabled, err)
	default:
		return fmt.Errorf("auto_sdl_config(enable=%v): %w", c.Enable, dfkit.PluginError("auto_sdl", err))
	}
}

// Create implements scheduler.Descriptor: disable, then enable with the desired values, once per
// VPP instance (the record makes a resync's re-apply a no-op).
func (d *Descriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	var c Config
	if err := dfkit.Decode(obj, &c); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	value, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	applied, id, err := dfkit.AppliedThisBoot(ctx, d.client, d.boot, Key, string(value))
	if err != nil {
		return nil, err
	}
	if applied {
		return nil, nil
	}
	// an enable while enabled keeps VPP's old values: start from disabled
	if err := d.config(ctx, Config{}); err != nil {
		return nil, err
	}
	if err := d.config(ctx, c); err != nil {
		return nil, err
	}
	return nil, d.boot.Put(dfkit.BootRecord{Key: string(Key), Identity: id, Value: string(value)})
}

// Update implements scheduler.Descriptor: new values need disable + enable — a recreate.
func (*Descriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete disables auto-SDL when this owner enabled it on the running VPP instance (VPP's default
// is disabled; a restarted VPP has nothing of ours).
func (d *Descriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	started, err := dfkit.StartedThisBoot(ctx, d.client, d.boot, Key)
	if err != nil {
		return err
	}
	if started {
		if err := d.config(ctx, Config{}); err != nil {
			return err
		}
	}
	return d.boot.Delete(string(Key))
}

// Retrieve implements scheduler.Descriptor: VPP has no getter (write-only, D-063).
func (*Descriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(Name)
}

// Register registers the descriptor. Call it only in the designated globals owner's agent (D-071:
// auto_sdl_config is VPP-global); every other agent leaves `services.autoSdl` unapplied.
func Register(r scheduler.Registry, client vpp.Client, boot dfkit.BootStore) {
	r.Register(New(client, boot))
}
