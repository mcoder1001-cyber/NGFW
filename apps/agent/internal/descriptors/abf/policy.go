package abf

import (
	"context"
	"fmt"
	"strconv"

	"google.golang.org/protobuf/proto"

	abfapi "ngfw/agent/binapi/abf"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// PolicyName is the descriptor name; keys are "abf.policy/<policy_id>".
const PolicyName = "abf.policy"

// PolicyDescriptor manages ABF policies (abf_policy_add_del). Policies carry no tag: they are
// attributed to this agent by policy id (df2.IDRange; nil = every id) and by their ACL being
// one of this owner's.
type PolicyDescriptor struct {
	client vpp.Client
	owner  string
	ids    *df2.IDRange
}

// NewPolicy returns the descriptor; ids scopes the policy ids this agent owns.
func NewPolicy(c vpp.Client, owner string, ids *df2.IDRange) *PolicyDescriptor {
	return &PolicyDescriptor{client: c, owner: owner, ids: ids}
}

// PolicyMeta is the runtime handle: the acl_index the policy was created with.
type PolicyMeta struct{ ACLIndex uint32 }

// NormalizePolicy returns p with canonical paths (df2.NormalizePaths) — the form Retrieve
// produces.
func NormalizePolicy(p *Policy) (*Policy, error) {
	n := proto.Clone(p).(*Policy)
	paths, err := df2.NormalizePaths(n.Paths)
	if err != nil {
		return nil, err
	}
	n.Paths = paths
	return n, nil
}

// Name implements scheduler.Descriptor.
func (*PolicyDescriptor) Name() string { return PolicyName }

// KeyOf implements scheduler.Descriptor.
func (*PolicyDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(PolicyName, strconv.FormatUint(uint64(obj.(*Policy).GetPolicyId()), 10))
}

// Dependencies implements scheduler.Descriptor: the ACL (mandatory) and every next-hop
// interface (optional: ordering only).
func (*PolicyDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	p := obj.(*Policy)
	deps := []scheduler.Dependency{{Key: df2.ACLKey(p.GetAcl())}}
	for _, path := range p.GetPaths() {
		if path.GetInterface() != "" {
			deps = append(deps, scheduler.Dependency{Key: df2.InterfaceKey(path.GetInterface()), Optional: true})
		}
	}
	return deps
}

func (d *PolicyDescriptor) addDel(ctx context.Context, p *Policy, aclIndex uint32, isAdd bool) error {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return err
	}
	paths, err := df2.EncodePaths(p.GetPaths(), ifs)
	if err != nil {
		return err
	}
	if len(paths) > 255 {
		return fmt.Errorf("%s: %d paths exceed 255", PolicyName, len(paths))
	}
	req := &abfapi.AbfPolicyAddDel{IsAdd: isAdd, Policy: abfapi.AbfPolicy{PolicyID: p.GetPolicyId(), ACLIndex: aclIndex, NPaths: uint8(len(paths)), Paths: paths}} //nolint:gosec // checked
	if _, err := abfapi.NewServiceClient(d.client).AbfPolicyAddDel(ctx, req); err != nil {
		return fmt.Errorf("abf_policy_add_del: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *PolicyDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	p, err := NormalizePolicy(obj.(*Policy))
	if err != nil {
		return nil, err
	}
	if !d.ids.Owns(p.GetPolicyId()) {
		return nil, fmt.Errorf("%s: policy id %d outside this agent's range", PolicyName, p.GetPolicyId())
	}
	if len(p.GetPaths()) == 0 {
		return nil, fmt.Errorf("%s: at least one path is required", PolicyName)
	}
	acls, err := DumpACLs(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	aclIndex, err := acls.Index(p.GetAcl())
	if err != nil {
		return nil, err
	}
	if err := d.addDel(ctx, p, aclIndex, true); err != nil {
		return nil, err
	}
	return PolicyMeta{ACLIndex: aclIndex}, nil
}

// Update changes the path list in place. abf_policy_add_del is additive (is_add appends the
// given paths to the policy's list, verified on vrx-a; !is_add removes them), so the new
// paths are added first and the paths no longer desired removed afterwards — the list is
// never empty in between, which would delete the policy. A different ACL needs a recreate
// (VPP rejects changing it).
func (d *PolicyDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, n := oldObj.(*Policy), newObj.(*Policy)
	if o.GetPolicyId() != n.GetPolicyId() || o.GetAcl() != n.GetAcl() {
		return nil, scheduler.ErrRecreate
	}
	m, ok := meta.(PolicyMeta)
	if !ok {
		return nil, fmt.Errorf("%s: %w %T", PolicyName, df2.ErrBadMeta, meta)
	}
	oldP, err := NormalizePolicy(o)
	if err != nil {
		return nil, err
	}
	newP, err := NormalizePolicy(n)
	if err != nil {
		return nil, err
	}
	if len(newP.GetPaths()) == 0 {
		return nil, fmt.Errorf("%s: at least one path is required", PolicyName)
	}
	add, del := pathDiff(oldP.GetPaths(), newP.GetPaths())
	if len(add) > 0 {
		if err := d.addDel(ctx, &Policy{PolicyId: newP.GetPolicyId(), Acl: newP.GetAcl(), Paths: add}, m.ACLIndex, true); err != nil {
			return nil, err
		}
	}
	if len(del) > 0 {
		if err := d.addDel(ctx, &Policy{PolicyId: newP.GetPolicyId(), Acl: newP.GetAcl(), Paths: del}, m.ACLIndex, false); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// pathDiff returns the paths only in newPaths (to add) and only in oldPaths (to remove).
func pathDiff(oldPaths, newPaths []*df2.FibPath) (add, del []*df2.FibPath) {
	contains := func(set []*df2.FibPath, p *df2.FibPath) bool {
		for _, q := range set {
			if proto.Equal(p, q) {
				return true
			}
		}
		return false
	}
	for _, p := range newPaths {
		if !contains(oldPaths, p) {
			add = append(add, p)
		}
	}
	for _, p := range oldPaths {
		if !contains(newPaths, p) {
			del = append(del, p)
		}
	}
	return add, del
}

// Delete removes the policy by removing every path VPP currently holds for it (dumped, not
// taken from the desired object, so a drifted list cannot leave the policy behind).
func (d *PolicyDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(PolicyMeta)
	if !ok {
		return fmt.Errorf("%s: %w %T", PolicyName, df2.ErrBadMeta, meta)
	}
	p, err := NormalizePolicy(obj.(*Policy))
	if err != nil {
		return err
	}
	if actual, err := d.dumpPolicy(ctx, p.GetPolicyId()); err != nil {
		return err
	} else if actual != nil {
		p.Paths = actual
	}
	return d.addDel(ctx, p, m.ACLIndex, false)
}

// dumpPolicy returns the current paths of policy id, or nil when VPP has no such policy.
func (d *PolicyDescriptor) dumpPolicy(ctx context.Context, id uint32) ([]*df2.FibPath, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	stream, err := abfapi.NewServiceClient(d.client).AbfPolicyDump(ctx, &abfapi.AbfPolicyDump{})
	if err != nil {
		return nil, fmt.Errorf("abf_policy_dump: %w", err)
	}
	details, err := df2.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("abf_policy_dump: %w", err)
	}
	for _, det := range details {
		if det.Policy.PolicyID == id {
			return df2.DecodePaths(det.Policy.Paths, ifs), nil
		}
	}
	return nil, nil
}

// Retrieve dumps every policy (abf_policy_dump) and keeps those whose id is owned and whose
// ACL is one of this owner's.
func (d *PolicyDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	acls, err := DumpACLs(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	stream, err := abfapi.NewServiceClient(d.client).AbfPolicyDump(ctx, &abfapi.AbfPolicyDump{})
	if err != nil {
		return nil, fmt.Errorf("abf_policy_dump: %w", err)
	}
	details, err := df2.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("abf_policy_dump: %w", err)
	}
	var out []scheduler.KV
	for _, det := range details {
		pol := det.Policy
		if !d.ids.Owns(pol.PolicyID) {
			continue
		}
		aclName, ok := acls.Name(pol.ACLIndex)
		if !ok {
			continue
		}
		v := &Policy{PolicyId: pol.PolicyID, Acl: aclName, Paths: df2.DecodePaths(pol.Paths, ifs)}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: PolicyMeta{ACLIndex: pol.ACLIndex}})
	}
	return out, nil
}
