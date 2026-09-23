package sr

import (
	"context"
	"fmt"

	srapi "ngfw/agent/binapi/sr"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/vpp"
)

// EncapSourceName / EncapHopLimitName are the singleton descriptor names; keys "<name>/global".
const (
	EncapSourceName   = "sr.encap-source"
	EncapHopLimitName = "sr.encap-hop-limit"
)

// DefaultEncapHopLimit is VPP's SRv6 encap hop limit when none is set.
const DefaultEncapHopLimit = 64

// EncapSourceDescriptor sets the global SRv6 encapsulation source (write-only: VPP has no
// getter; Delete resets it to ::).
type EncapSourceDescriptor = df6.SingletonDescriptor[*EncapSource]

// NewEncapSource returns the descriptor.
func NewEncapSource(c vpp.Client) *EncapSourceDescriptor {
	set := func(ctx context.Context, c vpp.Client, addr string) error {
		a, err := df6.IP6Of(addr)
		if err != nil {
			return err
		}
		if _, err := srapi.NewServiceClient(c).SrSetEncapSource(ctx, &srapi.SrSetEncapSource{EncapsSource: a}); err != nil {
			return fmt.Errorf("sr_set_encap_source: %w", err)
		}
		return nil
	}
	return df6.NewSingletonDescriptor(df6.SingletonSpec[*EncapSource]{
		Name:   EncapSourceName,
		Plugin: Plugin,
		Validate: func(e *EncapSource) error {
			_, err := df6.ParseAddr6(e.GetAddress())
			return err
		},
		Set:   func(ctx context.Context, c vpp.Client, e *EncapSource) error { return set(ctx, c, e.GetAddress()) },
		Unset: func(ctx context.Context, c vpp.Client, _ *EncapSource) error { return set(ctx, c, "") },
	}, c)
}

// EncapHopLimitDescriptor sets the global SRv6 encap hop limit (write-only; Delete resets
// it to 64).
type EncapHopLimitDescriptor = df6.SingletonDescriptor[*EncapHopLimit]

// NewEncapHopLimit returns the descriptor.
func NewEncapHopLimit(c vpp.Client) *EncapHopLimitDescriptor {
	set := func(ctx context.Context, c vpp.Client, v uint32) error {
		if _, err := srapi.NewServiceClient(c).SrSetEncapHopLimit(ctx, &srapi.SrSetEncapHopLimit{HopLimit: uint8(v)}); err != nil { //nolint:gosec // validated 1..255
			return fmt.Errorf("sr_set_encap_hop_limit: %w", err)
		}
		return nil
	}
	return df6.NewSingletonDescriptor(df6.SingletonSpec[*EncapHopLimit]{
		Name:   EncapHopLimitName,
		Plugin: Plugin,
		Validate: func(h *EncapHopLimit) error {
			if h.GetHopLimit() == 0 || h.GetHopLimit() > 255 {
				return fmt.Errorf("%w: hop_limit must be 1–255", df6.ErrBadValue)
			}
			return nil
		},
		Set: func(ctx context.Context, c vpp.Client, h *EncapHopLimit) error { return set(ctx, c, h.GetHopLimit()) },
		Unset: func(ctx context.Context, c vpp.Client, _ *EncapHopLimit) error {
			return set(ctx, c, DefaultEncapHopLimit)
		},
	}, c)
}
