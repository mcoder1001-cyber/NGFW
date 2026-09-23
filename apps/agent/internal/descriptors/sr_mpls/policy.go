// Package sr_mpls holds the SR-MPLS descriptors (policy, steering, endpoint color).
//
// VPP 26.06 has no SR-MPLS dump, and the MPLS FIB entry a policy installs cannot stand in for
// one: mpls_route_dump does not encode the via-label of recursive MPLS paths, which is the
// first segment of every list. All three descriptors are therefore write-only (Retrieve
// returns df6.ErrRetrieveUnsupported). Ownership is a claim record (D-071); presence is probed
// exactly (the BSID's end-of-stack entry in MPLS table 0 with SR-shaped paths; the steering
// prefix among the FIB_SOURCE_SR routes of its table with an MPLS path), so a re-apply after a
// resync is a no-op while VPP still has the object (D-076). See docs/agent/descriptors/sr_mpls.md.
package sr_mpls //nolint:revive // package name mirrors the VPP plugin / binapi package

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/mpls"
	srmplsapi "ngfw/agent/binapi/sr_mpls"
	"ngfw/agent/binapi/sr_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Plugin is the VPP plugin providing the messages.
const Plugin = "srmpls"

// PolicyName is the descriptor name; keys are "sr-mpls.policy/<bsid>".
const PolicyName = "sr-mpls.policy"

// MPLS label limits.
const (
	MinLabel    = 16
	MaxLabel    = 1<<20 - 1
	MaxSegments = 16
)

// ErrNoSuchPolicy means the referenced SR-MPLS policy (BSID) does not exist in VPP.
var ErrNoSuchPolicy = errors.New("no such sr-mpls policy")

// PolicyDescriptor manages SR-MPLS policies (write-only, ours by claim).
type PolicyDescriptor = df6.KeyedDescriptor[*Policy]

// NewPolicy returns the descriptor.
func NewPolicy(c vpp.Client, owner string, opts ...df6.Option) *PolicyDescriptor {
	return df6.NewKeyedDescriptor(policySpec, c, owner, opts...)
}

func validLabel(l uint32) bool { return l >= MinLabel && l <= MaxLabel }

func validatePolicy(p *Policy) error {
	if !validLabel(p.GetBsid()) {
		return fmt.Errorf("%w: bsid %d outside %d–%d", df6.ErrBadValue, p.GetBsid(), MinLabel, MaxLabel)
	}
	if len(p.GetSegmentLists()) == 0 {
		return fmt.Errorf("%w: at least one segment list", df6.ErrBadValue)
	}
	for i, sl := range p.GetSegmentLists() {
		// An empty list makes VPP read segments[0] of an empty vector.
		if len(sl.GetLabels()) == 0 || len(sl.GetLabels()) > MaxSegments {
			return fmt.Errorf("%w: segment list %d must have 1–%d labels", df6.ErrBadValue, i, MaxSegments)
		}
		for _, l := range sl.GetLabels() {
			if !validLabel(l) {
				return fmt.Errorf("%w: label %d outside %d–%d", df6.ErrBadValue, l, MinLabel, MaxLabel)
			}
		}
		if sl.GetWeight() == 0 || sl.GetWeight() > 255 {
			return fmt.Errorf("%w: segment list %d weight must be 1–255", df6.ErrBadValue, i)
		}
		if i > 0 && compareLists(p.GetSegmentLists()[i-1], sl) >= 0 {
			return fmt.Errorf("%w: segment_lists must be unique and in ascending order", df6.ErrBadValue)
		}
	}
	return nil
}

func compareLists(a, b *SegmentList) int {
	if c := slices.Compare(a.GetLabels(), b.GetLabels()); c != 0 {
		return c
	}
	return int(a.GetWeight()) - int(b.GetWeight())
}

// PolicyKey is the key of the policy with binding SID bsid.
func PolicyKey(bsid uint32) scheduler.Key { return scheduler.Join(PolicyName, df6.U32(bsid)) }

var policySpec = df6.KeyedSpec[*Policy]{
	Name:      PolicyName,
	Plugin:    Plugin,
	WriteOnly: true,
	Canon: func(p *Policy) (*Policy, error) {
		return p, validatePolicy(p)
	},
	ID: func(p *Policy) string { return df6.U32(p.GetBsid()) },
	// MPLS table 0 (DF-7's mpls-table key): VPP installs every SR-MPLS BSID there.
	Deps: func(*Policy) []scheduler.Dependency { return []scheduler.Dependency{{Key: df6.MPLSTableKey(0)}} },
	// Add: sr_mpls_policy_add with the first list, then sr_mpls_policy_mod (ADD) per further
	// list; a failure deletes the half-built policy.
	Add: func(ctx context.Context, c vpp.Client, p *Policy) error {
		svc := srmplsapi.NewServiceClient(c)
		first := p.GetSegmentLists()[0]
		if _, err := svc.SrMplsPolicyAdd(ctx, &srmplsapi.SrMplsPolicyAdd{
			Bsid: p.GetBsid(), Weight: first.GetWeight(), IsSpray: p.GetSpray(), Segments: first.GetLabels(),
		}); err != nil {
			return fmt.Errorf("sr_mpls_policy_add: %w", err)
		}
		for i, sl := range p.GetSegmentLists()[1:] {
			if _, err := svc.SrMplsPolicyMod(ctx, &srmplsapi.SrMplsPolicyMod{
				Bsid: p.GetBsid(), Operation: sr_types.SR_POLICY_OP_API_ADD, Weight: sl.GetWeight(), Segments: sl.GetLabels(),
			}); err != nil {
				_, rerr := svc.SrMplsPolicyDel(ctx, &srmplsapi.SrMplsPolicyDel{Bsid: p.GetBsid()})
				return fmt.Errorf("sr_mpls_policy_mod (add list %d): %w (rollback: %v)", i+1, err, rerr)
			}
		}
		return nil
	},
	// Del also clears an endpoint/color assignment (VPP does so inside sr_mpls_policy_del).
	Del: func(ctx context.Context, c vpp.Client, p *Policy) error {
		if _, err := srmplsapi.NewServiceClient(c).SrMplsPolicyDel(ctx, &srmplsapi.SrMplsPolicyDel{Bsid: p.GetBsid()}); err != nil {
			return fmt.Errorf("sr_mpls_policy_del: %w", err)
		}
		return nil
	},
	List: func(ctx context.Context, c vpp.Client) ([]*Policy, error) {
		bsids, err := srBSIDs(ctx, c)
		if err != nil {
			return nil, err
		}
		out := make([]*Policy, 0, len(bsids))
		for _, b := range bsids {
			out = append(out, &Policy{Bsid: b}) // partial: only the id is readable
		}
		return out, nil
	},
}

// srBSIDs returns the labels of MPLS table 0 whose end-of-stack entry has the shape VPP gives
// an SR-MPLS BSID: every path a recursive MPLS path (proto MPLS, no interface, type normal).
// Ordinary MPLS routes (DF-7: out-labels via an interface / next hop) do not match (review M3).
func srBSIDs(ctx context.Context, c vpp.Client) ([]uint32, error) {
	stream, err := mpls.NewServiceClient(c).MplsRouteDump(ctx, &mpls.MplsRouteDump{Table: mpls.MplsTable{MtTableID: 0}})
	if err != nil {
		return nil, fmt.Errorf("mpls_route_dump: %w", err)
	}
	routes, err := df6.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("mpls_route_dump: %w", err)
	}
	var out []uint32
	for _, r := range routes {
		if r.MrRoute.MrEos != 1 || len(r.MrRoute.MrPaths) == 0 {
			continue
		}
		sr := true
		for _, p := range r.MrRoute.MrPaths {
			if p.Proto != fib_types.FIB_API_PATH_NH_PROTO_MPLS || p.SwIfIndex != df6.NoInterface || p.Type != fib_types.FIB_API_PATH_TYPE_NORMAL {
				sr = false
			}
		}
		if sr {
			out = append(out, r.MrRoute.MrLabel)
		}
	}
	return out, nil
}

// BSIDPresent reports whether VPP has an SR-MPLS-shaped BSID entry for bsid (any owner).
func BSIDPresent(ctx context.Context, c vpp.Client, bsid uint32) (bool, error) {
	bsids, err := srBSIDs(ctx, c)
	if err != nil {
		return false, err
	}
	return slices.Contains(bsids, bsid), nil
}
