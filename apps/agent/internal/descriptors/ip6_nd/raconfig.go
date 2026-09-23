// Package ip6nd implements the descriptors of VPP's ip6_nd API (router advertisements,
// advertised prefixes, ND proxy) and of the ip6_dad_autoremove plugin (duplicate address
// detection). Messages come from apps/agent/binapi/ip6_nd and binapi/ip6_dad only.
package ip6nd

import (
	"context"
	"fmt"
	"math"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip6_nd"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// RaConfigName is the descriptor name; keys are "ip6-nd.ra-config/<interface>".
const RaConfigName = "ip6-nd.ra-config"

// VPP 26.06 router-advertisement state of a freshly IPv6-enabled interface (ip6_ra.c,
// verified on vrx-a with sw_interface_ip6nd_ra_dump): RAs suppressed, lifetime 600,
// max/min interval 200/150, initial burst 3 × 16 s. An interface in that state is
// "unconfigured": Retrieve omits it and Delete restores it.
const (
	DefaultSuppress        = true
	DefaultRouterLifetime  = 600
	DefaultMaxInterval     = 200
	DefaultMinInterval     = 150
	DefaultInitialCount    = 3
	DefaultInitialInterval = 16
)

// MinIntervalFor is VPP's rule for a minimum left unset next to an explicit maximum:
// 0.75 × max interval.
func MinIntervalFor(maxInterval uint32) uint32 { return maxInterval * 3 / 4 }

// NormalizeRaConfig returns c with zero timers replaced by the VPP defaults — the form
// Retrieve produces. Callers normalise desired objects with it before diffing.
func NormalizeRaConfig(c *RaConfig) *RaConfig {
	n := proto.Clone(c).(*RaConfig)
	if n.MinInterval == 0 {
		if n.MaxInterval == 0 {
			n.MinInterval = DefaultMinInterval
		} else {
			n.MinInterval = MinIntervalFor(n.MaxInterval)
		}
	}
	if n.MaxInterval == 0 {
		n.MaxInterval = DefaultMaxInterval
	}
	if n.InitialCount == 0 {
		n.InitialCount = DefaultInitialCount
	}
	if n.InitialInterval == 0 {
		n.InitialInterval = DefaultInitialInterval
	}
	return n
}

// isDefaultRa reports whether a normalised configuration equals the fresh-interface state.
func isDefaultRa(c *RaConfig) bool {
	return c.GetSuppress() /* DefaultSuppress */ && !c.GetManaged() && !c.GetOther() && !c.GetSuppressLinkLayerOption() &&
		!c.GetSendUnicast() && !c.GetCease() && c.GetRouterLifetime() == DefaultRouterLifetime &&
		c.GetMaxInterval() == DefaultMaxInterval && c.GetMinInterval() == DefaultMinInterval &&
		c.GetInitialCount() == DefaultInitialCount && c.GetInitialInterval() == DefaultInitialInterval
}

// RaConfigDescriptor manages per-interface RA settings (sw_interface_ip6nd_ra_config). The
// API is toggle-style (a zero field means "unchanged"; a set field means "set", or "back to
// default" with is_no), so every apply is three calls: reset every flag and timer except
// suppress to its default (is_no), set the desired flags and timers, then set the suppress
// state on its own (un-suppressing needs is_no, which must not touch the other fields).
type RaConfigDescriptor struct {
	client vpp.Client
	owner  string
	opts   df2.Options
}

// NewRaConfig returns the descriptor for the given owner.
func NewRaConfig(c vpp.Client, owner string, opts ...df2.Option) *RaConfigDescriptor {
	return &RaConfigDescriptor{client: c, owner: owner, opts: df2.BuildOptions(opts...)}
}

// RaMeta is the runtime handle of ra-config and ra-prefix.
type RaMeta struct{ SwIfIndex uint32 }

// Name implements scheduler.Descriptor.
func (*RaConfigDescriptor) Name() string { return RaConfigName }

// KeyOf implements scheduler.Descriptor.
func (*RaConfigDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(RaConfigName, obj.(*RaConfig).GetInterface())
}

// Dependencies implements scheduler.Descriptor. IPv6 must be enabled on the interface (an
// address assigned) before VPP accepts RA settings; P05 core orders that via the interface.
func (*RaConfigDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{df2.InterfaceDep(obj.(*RaConfig).GetInterface())}
}

func b2u(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// resetRa returns every RA setting of idx to the fresh-interface state: flags and timers
// with is_no, then suppress set explicitly (the is_no form would un-suppress).
func (d *RaConfigDescriptor) resetRa(ctx context.Context, idx interface_types.InterfaceIndex) error {
	svc := ip6_nd.NewServiceClient(d.client)
	req := &ip6_nd.SwInterfaceIP6ndRaConfig{
		SwIfIndex: idx, IsNo: true,
		Managed: 1, Other: 1, LlOption: 1, SendUnicast: 1, Cease: 1, DefaultRouter: 1,
		Lifetime: 1, MaxInterval: 1, MinInterval: 1, InitialCount: 1, InitialInterval: 1,
	}
	if _, err := svc.SwInterfaceIP6ndRaConfig(ctx, req); err != nil {
		return fmt.Errorf("sw_interface_ip6nd_ra_config (reset): %w", err)
	}
	return d.setSuppress(ctx, idx, DefaultSuppress)
}

// setSuppress sets only the suppress state: suppress=1 with is_no=0 stops RAs, with is_no=1
// (re)starts them; every other field is zero, i.e. unchanged.
func (d *RaConfigDescriptor) setSuppress(ctx context.Context, idx interface_types.InterfaceIndex, suppress bool) error {
	req := &ip6_nd.SwInterfaceIP6ndRaConfig{SwIfIndex: idx, Suppress: 1, IsNo: !suppress}
	if _, err := ip6_nd.NewServiceClient(d.client).SwInterfaceIP6ndRaConfig(ctx, req); err != nil {
		return fmt.Errorf("sw_interface_ip6nd_ra_config (suppress): %w", err)
	}
	return nil
}

func (d *RaConfigDescriptor) apply(ctx context.Context, c *RaConfig, idx interface_types.InterfaceIndex) error {
	if c.GetRouterLifetime() != 0 && c.GetRouterLifetime() < c.GetMaxInterval() {
		return fmt.Errorf("%s: router_lifetime %d must be 0 or ≥ max_interval %d", RaConfigName, c.GetRouterLifetime(), c.GetMaxInterval())
	}
	if err := d.resetRa(ctx, idx); err != nil {
		return err
	}
	req := &ip6_nd.SwInterfaceIP6ndRaConfig{
		SwIfIndex:       idx,
		Managed:         b2u(c.GetManaged()),
		Other:           b2u(c.GetOther()),
		LlOption:        b2u(c.GetSuppressLinkLayerOption()),
		SendUnicast:     b2u(c.GetSendUnicast()),
		Cease:           b2u(c.GetCease()),
		DefaultRouter:   1,
		Lifetime:        c.GetRouterLifetime(),
		MaxInterval:     c.GetMaxInterval(),
		MinInterval:     c.GetMinInterval(),
		InitialCount:    c.GetInitialCount(),
		InitialInterval: c.GetInitialInterval(),
	}
	if _, err := ip6_nd.NewServiceClient(d.client).SwInterfaceIP6ndRaConfig(ctx, req); err != nil {
		return fmt.Errorf("sw_interface_ip6nd_ra_config: %w", err)
	}
	return d.setSuppress(ctx, idx, c.GetSuppress())
}

// Create implements scheduler.Descriptor.
func (d *RaConfigDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	c := NormalizeRaConfig(obj.(*RaConfig))
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	idx32, untagged, err := ifs.Resolve(c.GetInterface())
	if err != nil {
		return nil, err
	}
	idx := interface_types.InterfaceIndex(idx32)
	if err := d.apply(ctx, c, idx); err != nil {
		return nil, err
	}
	if err := df2.Claim(d.opts.Claims, untagged, d.KeyOf(obj)); err != nil {
		return nil, err
	}
	return RaMeta{SwIfIndex: idx32}, nil
}

// Update re-applies in place; a different interface is a different object.
func (d *RaConfigDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if d.KeyOf(oldObj) != d.KeyOf(newObj) {
		return nil, scheduler.ErrRecreate
	}
	m, ok := meta.(RaMeta)
	if !ok {
		return nil, fmt.Errorf("%s: %w %T", RaConfigName, df2.ErrBadMeta, meta)
	}
	if err := d.apply(ctx, NormalizeRaConfig(newObj.(*RaConfig)), interface_types.InterfaceIndex(m.SwIfIndex)); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete restores the VPP defaults.
func (d *RaConfigDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(RaMeta)
	if !ok {
		return fmt.Errorf("%s: %w %T", RaConfigName, df2.ErrBadMeta, meta)
	}
	if err := d.resetRa(ctx, interface_types.InterfaceIndex(m.SwIfIndex)); err != nil {
		return err
	}
	return df2.Release(d.opts.Claims, d.KeyOf(obj))
}

// dumpRa dumps the RA state of every interface.
func dumpRa(ctx context.Context, c vpp.Client) ([]*ip6_nd.SwInterfaceIP6ndRaDetails, error) {
	stream, err := ip6_nd.NewServiceClient(c).SwInterfaceIP6ndRaDump(ctx, &ip6_nd.SwInterfaceIP6ndRaDump{SwIfIndex: interface_types.InterfaceIndex(df2.NoInterface)})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_ip6nd_ra_dump: %w", err)
	}
	details, err := df2.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("sw_interface_ip6nd_ra_dump: %w", err)
	}
	return details, nil
}

func secs(f float64) uint32 {
	if f < 0 || f > math.MaxUint32 {
		return 0
	}
	return uint32(math.Round(f))
}

func decodeRa(name string, det *ip6_nd.SwInterfaceIP6ndRaDetails) *RaConfig {
	return &RaConfig{
		Interface:               name,
		Suppress:                !det.SendRadv,
		Managed:                 det.AdvManagedFlag,
		Other:                   det.AdvOtherFlag,
		SuppressLinkLayerOption: !det.AdvLinkLayerAddress,
		SendUnicast:             det.SendUnicast,
		Cease:                   det.CeaseRadv,
		RouterLifetime:          uint32(det.AdvRouterLifetime),
		MaxInterval:             secs(det.MaxRadvInterval),
		MinInterval:             secs(det.MinRadvInterval),
		InitialCount:            det.InitialAdvertsCount,
		InitialInterval:         secs(det.InitialAdvertsInterval),
	}
}

// Retrieve returns the RA configuration of every owned interface that differs from the VPP
// defaults (sw_interface_ip6nd_ra_dump).
func (d *RaConfigDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	details, err := dumpRa(ctx, d.client)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, det := range details {
		name, ok := ifs.Name(uint32(det.SwIfIndex))
		if !ok {
			continue
		}
		v := decodeRa(name, det)
		if isDefaultRa(v) {
			continue
		}
		if !ifs.OwnsObject(uint32(det.SwIfIndex), d.KeyOf(v), d.opts.Claims) {
			continue
		}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: RaMeta{SwIfIndex: uint32(det.SwIfIndex)}})
	}
	return out, nil
}
