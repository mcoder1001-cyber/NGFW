package subsystems

import (
	"context"
	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/descriptors/dfkit"
	iface "ngfw/agent/internal/descriptors/interface"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
	"strings"
)

// nativeAdminGuard closes the direct-agent partial-domain Apply edge: a caller
// that omits vpn must still never admin-up an IKE-owned IPIP after peer loss.
// The live owned profile, rather than the incoming document, is authoritative.
type nativeAdminGuard struct {
	scheduler.Descriptor
	owner    string
	profiles func(context.Context, string) ([]scheduler.KV, error)
}

func (*nativeAdminGuard) RecordsNoOwnership() {}
func (g *nativeAdminGuard) check(ctx context.Context, v proto.Message) error {
	a, ok := v.(*iface.AdminState)
	if !ok {
		return nil
	}
	name := iface.RefID(a.GetInterface())
	if !strings.HasPrefix(name, "ipip") {
		return nil
	}
	lookup := g.profiles
	if lookup == nil {
		lookup = IKEv2Profiles
	}
	kvs, err := lookup(ctx, g.owner)
	if err != nil {
		return err
	}
	for _, kv := range kvs {
		if p, ok := kv.Value.(*vpnpb.Ikev2Profile); ok && p.GetTunnelInterface() == name {
			return dfkit.Specf("native IKE owns protected IPIP admin state; explicit admin-up is refused")
		}
	}
	return nil
}
func (g *nativeAdminGuard) Create(ctx context.Context, v proto.Message) (any, error) {
	if err := g.check(ctx, v); err != nil {
		return nil, err
	}
	return g.Descriptor.Create(ctx, v)
}
func (g *nativeAdminGuard) Update(ctx context.Context, old, v proto.Message, m any) (any, error) {
	if err := g.check(ctx, v); err != nil {
		return nil, err
	}
	return g.Descriptor.Update(ctx, old, v, m)
}

// Retrieve excludes admin state controlled by the live IKE profile. Otherwise
// full reconciliation would see an undesired admin-up key and lower an active SA.
func (g *nativeAdminGuard) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	kvs, err := g.Descriptor.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	lookup := g.profiles
	if lookup == nil {
		lookup = IKEv2Profiles
	}
	profiles, err := lookup(ctx, g.owner)
	if err != nil {
		return nil, err
	}
	bound := make(map[string]bool)
	for _, kv := range profiles {
		if p, ok := kv.Value.(*vpnpb.Ikev2Profile); ok {
			bound[p.GetTunnelInterface()] = true
		}
	}
	out := make([]scheduler.KV, 0, len(kvs))
	for _, kv := range kvs {
		a, ok := kv.Value.(*iface.AdminState)
		if ok && bound[iface.RefID(a.GetInterface())] {
			continue
		}
		out = append(out, kv)
	}
	return out, nil
}

// Normalize preserves canonical interface references through the native safety guard.
func (g *nativeAdminGuard) Normalize(value proto.Message) proto.Message {
	if normalizer, ok := g.Descriptor.(scheduler.Normalizer); ok {
		return normalizer.Normalize(value)
	}
	return value
}
