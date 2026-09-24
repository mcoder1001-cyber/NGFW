package wireguard

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/wireguard"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// AsyncMode toggles asynchronous WireGuard crypto (wg_set_async_mode). A VPP-global (D-071):
// registered as setter only for the globals owner. VPP has no getter, so the descriptor is
// write-only (D-063): Retrieve returns vpn.ErrRetrieveUnsupported and the reconciler re-applies the
// desired value on resync — idempotent (D-076: VPP sets or clears the ASYNC op-mode flag, it does
// not toggle). Delete leaves VPP as it is. Key wireguard.async-mode/global. Not exercised on the
// dev host (no worker threads; test slots are never the globals owner).
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
	o, ok := obj.(*vpnpb.WireguardAsyncMode)
	if !ok {
		return nil, typeErr(AsyncModeName, obj)
	}
	if _, err := wireguard.NewServiceClient(d.cfg.Client).WgSetAsyncMode(ctx, &wireguard.WgSetAsyncMode{AsyncEnable: o.GetEnabled()}); err != nil {
		return nil, fmt.Errorf("wg_set_async_mode: %w", err)
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
