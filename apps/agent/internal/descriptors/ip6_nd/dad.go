package ip6nd

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ip6_dad"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// DadName is the descriptor name; the single key is "ip6-nd.dad/global".
const DadName = "ip6-nd.dad"

// DadPlugin names the plugin the task prompt associated with DAD. On VPP 26.06 the ip6_dad
// API (ip6_dad_enable_disable / ip6_dad_dump) is served by vnet itself and works on vrx-a
// although ip6_dad_autoremove is not loaded (docs/lab/host-vrx-a.md); should a build lack
// the messages, every method returns df2.ErrPluginNotLoaded and the integration test skips
// (skip-unless-plugin-loaded).
const DadPlugin = "ip6_dad_autoremove"

// Defaults of ip6_dad_enable_disable.
const (
	DefaultDadTransmits       = 1
	DefaultDadRetransmitDelay = 1.0
)

// NormalizeDad returns d with zero fields replaced by the plugin defaults.
func NormalizeDad(d *Dad) *Dad {
	n := proto.Clone(d).(*Dad)
	if n.Transmits == 0 {
		n.Transmits = DefaultDadTransmits
	}
	if n.RetransmitDelay == 0 {
		n.RetransmitDelay = DefaultDadRetransmitDelay
	}
	return n
}

// DadDescriptor manages the global DAD switch (ip6_dad_enable_disable): the object exists
// while DAD is enabled.
type DadDescriptor struct {
	client vpp.Client
}

// NewDad returns the descriptor.
func NewDad(c vpp.Client) *DadDescriptor { return &DadDescriptor{client: c} }

// Name implements scheduler.Descriptor.
func (*DadDescriptor) Name() string { return DadName }

// KeyOf implements scheduler.Descriptor.
func (*DadDescriptor) KeyOf(proto.Message) scheduler.Key { return scheduler.Join(DadName, "global") }

// Dependencies implements scheduler.Descriptor: none.
func (*DadDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *DadDescriptor) set(ctx context.Context, enable bool, transmits uint32, delay float64) error {
	if transmits > 255 {
		return fmt.Errorf("%s: transmits %d exceeds 255", DadName, transmits)
	}
	req := &ip6_dad.IP6DadEnableDisable{Enable: enable, DadTransmits: uint8(transmits), DadRetransmitDelay: delay} //nolint:gosec // checked
	if _, err := ip6_dad.NewServiceClient(d.client).IP6DadEnableDisable(ctx, req); err != nil {
		return fmt.Errorf("ip6_dad_enable_disable: %w", df2.PluginError(DadPlugin, err))
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *DadDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	c := NormalizeDad(obj.(*Dad))
	return nil, d.set(ctx, true, c.GetTransmits(), c.GetRetransmitDelay())
}

// Update implements scheduler.Descriptor: everything changes in place.
func (d *DadDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete disables DAD.
func (d *DadDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	return d.set(ctx, false, DefaultDadTransmits, DefaultDadRetransmitDelay)
}

// Retrieve reads the global state (ip6_dad_dump); disabled means absent.
func (d *DadDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	stream, err := ip6_dad.NewServiceClient(d.client).IP6DadDump(ctx, &ip6_dad.IP6DadDump{})
	if err != nil {
		return nil, fmt.Errorf("ip6_dad_dump: %w", df2.PluginError(DadPlugin, err))
	}
	details, err := df2.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("ip6_dad_dump: %w", df2.PluginError(DadPlugin, err))
	}
	var out []scheduler.KV
	for _, det := range details {
		if !det.Enabled {
			continue
		}
		v := &Dad{Transmits: uint32(det.DadTransmits), RetransmitDelay: det.DadRetransmitDelay}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v})
	}
	return out, nil
}
