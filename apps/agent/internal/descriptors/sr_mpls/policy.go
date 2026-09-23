// Package sr_mpls holds the SR-MPLS descriptors (policy, steering, endpoint color).
//
// VPP 26.06 has no SR-MPLS dump, and the MPLS FIB entry a policy installs cannot stand in for
// one: mpls_route_dump does not encode the via-label of recursive MPLS paths, which is the
// first segment of every list. All three descriptors are therefore write-only (Retrieve
// returns df6.ErrRetrieveUnsupported); Create/Delete check presence through the BSID's MPLS
// FIB entry so they stay idempotent. See docs/agent/descriptors/sr_mpls.md.
package sr_mpls //nolint:revive,stylecheck // package name mirrors the VPP plugin / binapi package

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"

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

// ErrNoSuchPolicy: the referenced SR-MPLS policy (BSID) does not exist in VPP.
var ErrNoSuchPolicy = errors.New("no such sr-mpls policy")

// PolicyDescriptor manages SR-MPLS policies.
type PolicyDescriptor struct {
	client vpp.Client
}

// NewPolicy returns the descriptor.
func NewPolicy(c vpp.Client) *PolicyDescriptor { return &PolicyDescriptor{client: c} }

// Name implements scheduler.Descriptor.
func (d *PolicyDescriptor) Name() string { return PolicyName }

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

func (d *PolicyDescriptor) cast(obj proto.Message) (*Policy, error) {
	p, ok := obj.(*Policy)
	if !ok {
		return nil, fmt.Errorf("%s: %w: %T", PolicyName, df6.ErrBadValue, obj)
	}
	if err := validatePolicy(p); err != nil {
		return nil, fmt.Errorf("%s: %w", PolicyName, err)
	}
	return p, nil
}

// PolicyKey is the key of the policy with binding SID bsid.
func PolicyKey(bsid uint32) scheduler.Key { return scheduler.Join(PolicyName, df6.U32(bsid)) }

// KeyOf implements scheduler.Descriptor.
func (d *PolicyDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	p, err := d.cast(obj)
	if err != nil {
		return scheduler.Join(PolicyName, "invalid")
	}
	return PolicyKey(p.GetBsid())
}

// Dependencies implements scheduler.Descriptor: MPLS table 0 (DF-7's mpls-table key); VPP
// installs every SR-MPLS BSID there and refuses policies without it.
func (d *PolicyDescriptor) Dependencies(proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{{Key: df6.MPLSTableKey(0)}}
}

// Create implements scheduler.Descriptor: sr_mpls_policy_add with the first list, then
// sr_mpls_policy_mod (ADD) per further list; a failure deletes the half-built policy.
func (d *PolicyDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	p, err := d.cast(obj)
	if err != nil {
		return nil, err
	}
	svc := srmplsapi.NewServiceClient(d.client)
	first := p.GetSegmentLists()[0]
	if _, err := svc.SrMplsPolicyAdd(ctx, &srmplsapi.SrMplsPolicyAdd{
		Bsid: p.GetBsid(), Weight: first.GetWeight(), IsSpray: p.GetSpray(), Segments: first.GetLabels(),
	}); err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("%s: sr_mpls_policy_add: %w", PolicyName, err))
	}
	for i, sl := range p.GetSegmentLists()[1:] {
		if _, err := svc.SrMplsPolicyMod(ctx, &srmplsapi.SrMplsPolicyMod{
			Bsid: p.GetBsid(), Operation: sr_types.SR_POLICY_OP_API_ADD, Weight: sl.GetWeight(), Segments: sl.GetLabels(),
		}); err != nil {
			_, rerr := svc.SrMplsPolicyDel(ctx, &srmplsapi.SrMplsPolicyDel{Bsid: p.GetBsid()})
			return nil, df6.PluginError(Plugin, fmt.Errorf("%s: sr_mpls_policy_mod (add list %d): %w (rollback: %v)", PolicyName, i+1, err, rerr))
		}
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: every change recreates.
func (d *PolicyDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor; a BSID VPP no longer has is already deleted.
func (d *PolicyDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	p, err := d.cast(obj)
	if err != nil {
		return err
	}
	ok, err := BSIDPresent(ctx, d.client, p.GetBsid())
	if err != nil || !ok {
		return err
	}
	if _, err := srmplsapi.NewServiceClient(d.client).SrMplsPolicyDel(ctx, &srmplsapi.SrMplsPolicyDel{Bsid: p.GetBsid()}); err != nil {
		return df6.PluginError(Plugin, fmt.Errorf("%s: sr_mpls_policy_del: %w", PolicyName, err))
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: VPP has no SR-MPLS dump (see the package doc).
func (d *PolicyDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", PolicyName, df6.ErrRetrieveUnsupported)
}

// BSIDPresent reports whether MPLS table 0 has the end-of-stack local-label entry of bsid,
// which VPP installs for every SR-MPLS policy (and removes with it).
func BSIDPresent(ctx context.Context, c vpp.Client, bsid uint32) (bool, error) {
	stream, err := mpls.NewServiceClient(c).MplsRouteDump(ctx, &mpls.MplsRouteDump{Table: mpls.MplsTable{MtTableID: 0}})
	if err != nil {
		return false, fmt.Errorf("mpls_route_dump: %w", err)
	}
	routes, err := df6.Collect(stream.Recv)
	if err != nil {
		return false, fmt.Errorf("mpls_route_dump: %w", err)
	}
	for _, r := range routes {
		if r.MrRoute.MrLabel == bsid && r.MrRoute.MrEos == 1 {
			return true, nil
		}
	}
	return false, nil
}
