package ipsec

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// AsyncMode toggles asynchronous crypto (ipsec_set_async_mode). VPP has no getter, so the
// descriptor is write-only (D-063): Retrieve returns vpn.ErrRetrieveUnsupported and the reconciler
// re-applies the desired value on resync (the call is idempotent). Delete leaves VPP as it is:
// absence in the desired state is not "disable". Key ipsec.async-mode/global.
type AsyncMode struct{ cfg Config }

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
	return nil, nil
}

// Update implements scheduler.Descriptor.
func (d *AsyncMode) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: a no-op (see the type doc).
func (*AsyncMode) Delete(context.Context, proto.Message, any) error { return nil }

// Retrieve implements scheduler.Descriptor: VPP has no getter (D-063, write-only).
func (*AsyncMode) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", AsyncModeName, vpn.ErrRetrieveUnsupported)
}
