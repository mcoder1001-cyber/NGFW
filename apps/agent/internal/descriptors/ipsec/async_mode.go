package ipsec

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ipsec"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// AsyncMode toggles asynchronous crypto (ipsec_set_async_mode). VPP has no getter, so Retrieve
// reports the value this process last applied (nothing after a restart, which makes the
// scheduler re-apply the desired value — the call is idempotent). Delete forgets the value
// without touching VPP: absence in the desired state is not "disable". Key ipsec.async-mode/global.
type AsyncMode struct {
	cfg   Config
	mu    sync.Mutex
	known *bool
}

// AsyncModeKey is the singleton's key.
var AsyncModeKey = scheduler.Join(AsyncModeName, "global")

// NewAsyncMode returns the descriptor.
func NewAsyncMode(cfg Config) *AsyncMode { return &AsyncMode{cfg: cfg} }

// Name implements scheduler.Descriptor.
func (*AsyncMode) Name() string { return AsyncModeName }

// KeyOf implements scheduler.Descriptor.
func (*AsyncMode) KeyOf(proto.Message) scheduler.Key { return AsyncModeKey }

// Dependencies implements scheduler.Descriptor (none).
func (*AsyncMode) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor.
func (d *AsyncMode) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.IpsecAsyncMode)
	if !ok {
		return nil, typeErr(AsyncModeName, obj)
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecSetAsyncMode(ctx, &ipsec.IpsecSetAsyncMode{AsyncEnable: o.GetEnabled()}); err != nil {
		return nil, fmt.Errorf("ipsec_set_async_mode: %w", err)
	}
	d.mu.Lock()
	v := o.GetEnabled()
	d.known = &v
	d.mu.Unlock()
	return nil, nil
}

// Update implements scheduler.Descriptor.
func (d *AsyncMode) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor (forgets the cached value only).
func (d *AsyncMode) Delete(context.Context, proto.Message, any) error {
	d.mu.Lock()
	d.known = nil
	d.mu.Unlock()
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *AsyncMode) Retrieve(context.Context) ([]scheduler.KV, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.known == nil {
		return nil, nil
	}
	return []scheduler.KV{{Key: AsyncModeKey, Value: &vpnpb.IpsecAsyncMode{Enabled: *d.known}}}, nil
}
