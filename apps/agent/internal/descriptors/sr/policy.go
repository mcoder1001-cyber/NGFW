package sr

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	srapi "ngfw/agent/binapi/sr"
	"ngfw/agent/binapi/sr_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// PolicyName is the descriptor name; keys are "sr.policy/<bsid>".
const PolicyName = "sr.policy"

// MaxSids is the size of srv6_sid_list.sids in the binary API.
const MaxSids = 16

// PolicyDescriptor manages SRv6 policies (attributed by BSID through the Scope).
type PolicyDescriptor struct {
	client vpp.Client
	scope  *df6.Scope
}

// NewPolicy returns the descriptor.
func NewPolicy(c vpp.Client, scope *df6.Scope) *PolicyDescriptor {
	return &PolicyDescriptor{client: c, scope: scope}
}

// Name implements scheduler.Descriptor.
func (d *PolicyDescriptor) Name() string { return PolicyName }

func canonPolicy(p *Policy) (*Policy, error) {
	bsid, err := df6.ParseAddr6(p.GetBsid())
	if err != nil {
		return nil, err
	}
	if _, ok := PolicyType_name[int32(p.GetType())]; !ok {
		return nil, fmt.Errorf("%w: policy type %d", df6.ErrBadValue, p.GetType())
	}
	if len(p.GetSidLists()) == 0 {
		return nil, fmt.Errorf("%w: at least one sid list", df6.ErrBadValue)
	}
	c := proto.Clone(p).(*Policy)
	c.Bsid = bsid.String()
	for i, sl := range c.GetSidLists() {
		if len(sl.GetSids()) == 0 || len(sl.GetSids()) > MaxSids {
			return nil, fmt.Errorf("%w: sid list %d must have 1–%d sids", df6.ErrBadValue, i, MaxSids)
		}
		for j, s := range sl.GetSids() {
			a, err := df6.ParseAddr6(s)
			if err != nil {
				return nil, err
			}
			sl.Sids[j] = a.String()
		}
	}
	switch {
	case p.GetEncap() && p.GetEncapSrc() == "":
		return nil, fmt.Errorf("%w: encap_src is mandatory for encap policies", df6.ErrBadValue)
	case !p.GetEncap() && p.GetEncapSrc() != "":
		return nil, fmt.Errorf("%w: encap_src only for encap policies", df6.ErrBadValue)
	case p.GetEncap():
		src, err := df6.ParseAddr6(p.GetEncapSrc())
		if err != nil {
			return nil, err
		}
		c.EncapSrc = src.String()
	}
	return c, nil
}

func (d *PolicyDescriptor) cast(obj proto.Message) (*Policy, error) {
	p, ok := obj.(*Policy)
	if !ok {
		return nil, fmt.Errorf("%s: %w: %T", PolicyName, df6.ErrBadValue, obj)
	}
	c, err := canonPolicy(p)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", PolicyName, err)
	}
	return c, nil
}

// KeyOf implements scheduler.Descriptor.
func (d *PolicyDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	p, err := d.cast(obj)
	if err != nil {
		return scheduler.Join(PolicyName, "invalid")
	}
	return PolicyKey(p.GetBsid())
}

// PolicyKey is the key of the policy with binding SID bsid (canonical IPv6 string).
func PolicyKey(bsid string) scheduler.Key { return scheduler.Join(PolicyName, bsid) }

// Dependencies implements scheduler.Descriptor: the BSID's table.
func (d *PolicyDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	p, err := d.cast(obj)
	if err != nil {
		return nil
	}
	return df6.VRFDeps(p.GetFibTable())
}

func sidList(sl *SidList) (srapi.Srv6SidList, error) {
	out := srapi.Srv6SidList{NumSids: uint8(len(sl.GetSids())), Weight: sl.GetWeight()} //nolint:gosec // ≤ 16
	for i, s := range sl.GetSids() {
		a, err := df6.IP6Of(s)
		if err != nil {
			return out, err
		}
		out.Sids[i] = a
	}
	return out, nil
}

// Create implements scheduler.Descriptor: sr_policy_add_v2 with the first sid list, then
// sr_policy_mod_v2 (ADD) per further list; a failure deletes the half-built policy.
func (d *PolicyDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	p, err := d.cast(obj)
	if err != nil {
		return nil, err
	}
	// VPP 26.06 looks the BSID table up with fib_table_find and uses ~0 unchecked.
	if err := df6.RequireTable(ctx, d.client, p.GetFibTable(), true); err != nil {
		return nil, fmt.Errorf("%s: %w", PolicyName, err)
	}
	bsid, _ := df6.IP6Of(p.GetBsid())
	src, _ := df6.IP6Of(p.GetEncapSrc())
	first, err := sidList(p.GetSidLists()[0])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", PolicyName, err)
	}
	svc := srapi.NewServiceClient(d.client)
	if _, err := svc.SrPolicyAddV2(ctx, &srapi.SrPolicyAddV2{
		BsidAddr: bsid, Weight: first.Weight, IsEncap: p.GetEncap(), Type: srapi.SrPolicyType(p.GetType()), //nolint:gosec // validated enum
		FibTable: p.GetFibTable(), Sids: first, EncapSrc: src,
	}); err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("%s: sr_policy_add_v2: %w", PolicyName, err))
	}
	for i, sl := range p.GetSidLists()[1:] {
		l, err := sidList(sl)
		if err == nil {
			_, err = svc.SrPolicyModV2(ctx, &srapi.SrPolicyModV2{
				BsidAddr: bsid, SrPolicyIndex: ^uint32(0), FibTable: p.GetFibTable(), Operation: sr_types.SR_POLICY_OP_API_ADD,
				Weight: l.Weight, Sids: l, EncapSrc: src,
			})
		}
		if err != nil {
			_, rerr := svc.SrPolicyDel(ctx, &srapi.SrPolicyDel{BsidAddr: bsid, SrPolicyIndex: ^uint32(0)})
			return nil, df6.PluginError(Plugin, fmt.Errorf("%s: sr_policy_mod_v2 (add sid list %d): %w (rollback: %v)", PolicyName, i+1, err, rerr))
		}
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: every change recreates (dependent steering entries
// are recreated by the scheduler).
func (d *PolicyDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: sr_policy_del by BSID; already gone is success.
func (d *PolicyDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	p, err := d.cast(obj)
	if err != nil {
		return err
	}
	recs, err := d.dump(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, r := range recs {
		if df6.IP6String(r.Bsid) == p.GetBsid() {
			found = true
		}
	}
	if !found {
		return nil
	}
	// VPP deletes a policy even while steering entries point at it, leaving them dangling
	// (their dump then reads a freed pool element); refuse instead.
	steer, err := dumpSteering(ctx, d.client)
	if err != nil {
		return err
	}
	for _, st := range steer {
		if df6.IP6String(st.Bsid) == p.GetBsid() {
			return fmt.Errorf("%s: %w: %s", PolicyName, ErrPolicyInUse, p.GetBsid())
		}
	}
	bsid, _ := df6.IP6Of(p.GetBsid())
	if _, err := srapi.NewServiceClient(d.client).SrPolicyDel(ctx, &srapi.SrPolicyDel{BsidAddr: bsid, SrPolicyIndex: ^uint32(0)}); err != nil {
		return df6.PluginError(Plugin, fmt.Errorf("%s: sr_policy_del: %w", PolicyName, err))
	}
	return nil
}

func (d *PolicyDescriptor) dump(ctx context.Context) ([]*srapi.SrPoliciesV2Details, error) {
	stream, err := srapi.NewServiceClient(d.client).SrPoliciesV2Dump(ctx, &srapi.SrPoliciesV2Dump{})
	if err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("%s: sr_policies_v2_dump: %w", PolicyName, err))
	}
	recs, err := df6.Collect(stream.Recv)
	if err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("%s: sr_policies_v2_dump: %w", PolicyName, err))
	}
	return recs, nil
}

// Retrieve implements scheduler.Descriptor: every policy whose BSID is in scope.
func (d *PolicyDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	recs, err := d.dump(ctx)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, r := range recs {
		bsid := df6.IP6String(r.Bsid)
		if bsid == "" || !d.scope.OwnsAddrString(bsid) {
			continue
		}
		p := &Policy{Bsid: bsid, Type: PolicyType(r.Type), Encap: r.IsEncap, FibTable: r.FibTable}
		if r.IsEncap {
			p.EncapSrc = df6.IP6String(r.EncapSrc)
		}
		for _, sl := range r.SidLists {
			l := &SidList{Weight: sl.Weight}
			for i := 0; i < int(sl.NumSids) && i < MaxSids; i++ {
				l.Sids = append(l.Sids, df6.IP6String(sl.Sids[i]))
			}
			p.SidLists = append(p.SidLists, l)
		}
		out = append(out, scheduler.KV{Key: PolicyKey(bsid), Value: p})
	}
	return out, nil
}

// ErrPolicyInUse means a steering entry still points at the policy (delete the steering first;
// the scheduler's dependency order does so).
var ErrPolicyInUse = errors.New("sr policy still has steering entries")
