package ipneighbor

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ip_neighbor"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ConfigName is the descriptor name; keys are "ip-neighbor.config/ipv4" and "/ipv6".
const ConfigName = "ip-neighbor.config"

// VPP 26.06 defaults of the neighbour database (ip_neighbor.c), what Delete restores.
const (
	DefaultMaxNumber = 50000
	DefaultMaxAge    = 0
	DefaultRecycle   = false
)

// ConfigDescriptor manages the global per-family neighbour database limits
// (ip_neighbor_config). Both families always exist in VPP, so Retrieve always returns two
// objects; the desired state should therefore always carry both (with defaults), and Delete
// means "back to VPP defaults".
type ConfigDescriptor struct {
	client vpp.Client
}

// NewConfig returns the descriptor.
func NewConfig(c vpp.Client) *ConfigDescriptor { return &ConfigDescriptor{client: c} }

// Name implements scheduler.Descriptor.
func (*ConfigDescriptor) Name() string { return ConfigName }

func afID(af df2.AddressFamily) string {
	if af == df2.AddressFamily_IPV6 {
		return "ipv6"
	}
	return "ipv4"
}

// KeyOf implements scheduler.Descriptor.
func (*ConfigDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(ConfigName, afID(obj.(*Config).GetAf()))
}

// Dependencies implements scheduler.Descriptor: none.
func (*ConfigDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *ConfigDescriptor) set(ctx context.Context, af ip_types.AddressFamily, maxNumber, maxAge uint32, recycle bool) error {
	_, err := ip_neighbor.NewServiceClient(d.client).IPNeighborConfig(ctx, &ip_neighbor.IPNeighborConfig{Af: af, MaxNumber: maxNumber, MaxAge: maxAge, Recycle: recycle})
	if err != nil {
		return fmt.Errorf("ip_neighbor_config: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *ConfigDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	c := obj.(*Config)
	return nil, d.set(ctx, df2.ToIPTypesAF(c.GetAf()), c.GetMaxNumber(), c.GetMaxAge(), c.GetRecycle())
}

// Update implements scheduler.Descriptor: every field changes in place.
func (d *ConfigDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete restores the VPP defaults.
func (d *ConfigDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	return d.set(ctx, df2.ToIPTypesAF(obj.(*Config).GetAf()), DefaultMaxNumber, DefaultMaxAge, DefaultRecycle)
}

// Retrieve reads both families with ip_neighbor_config_get.
func (d *ConfigDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	svc := ip_neighbor.NewServiceClient(d.client)
	out := make([]scheduler.KV, 0, 2)
	for _, af := range []ip_types.AddressFamily{ip_types.ADDRESS_IP4, ip_types.ADDRESS_IP6} {
		rep, err := svc.IPNeighborConfigGet(ctx, &ip_neighbor.IPNeighborConfigGet{Af: af})
		if err != nil {
			return nil, fmt.Errorf("ip_neighbor_config_get: %w", err)
		}
		v := &Config{Af: df2.FromIPTypesAF(af), MaxNumber: rep.MaxNumber, MaxAge: rep.MaxAge, Recycle: rep.Recycle}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v})
	}
	return out, nil
}
