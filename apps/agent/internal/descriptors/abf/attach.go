package abf

import (
	"context"
	"fmt"
	"strconv"

	"google.golang.org/protobuf/proto"

	abfapi "ngfw/agent/binapi/abf"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// AttachName is the descriptor name; keys are "abf.attach/<policy_id>/<interface>/<ipv4|ipv6>".
const AttachName = "abf.attach"

// AttachDescriptor manages policy ↔ interface attachments (abf_itf_attach_add_del).
type AttachDescriptor struct {
	client vpp.Client
	owner  string
	ids    *df2.IDRange
}

// NewAttach returns the descriptor; ids scopes the policy ids this agent owns.
func NewAttach(c vpp.Client, owner string, ids *df2.IDRange) *AttachDescriptor {
	return &AttachDescriptor{client: c, owner: owner, ids: ids}
}

// AttachMeta is the runtime handle.
type AttachMeta struct{ SwIfIndex uint32 }

// Name implements scheduler.Descriptor.
func (*AttachDescriptor) Name() string { return AttachName }

func afOf(ipv6 bool) string {
	if ipv6 {
		return "ipv6"
	}
	return "ipv4"
}

// KeyOf implements scheduler.Descriptor.
func (*AttachDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	a := obj.(*Attach)
	return scheduler.Join(AttachName, strconv.FormatUint(uint64(a.GetPolicyId()), 10), a.GetInterface(), afOf(a.GetIpv6()))
}

// Dependencies implements scheduler.Descriptor: the policy and the interface.
func (*AttachDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	a := obj.(*Attach)
	return []scheduler.Dependency{
		{Key: scheduler.Join(PolicyName, strconv.FormatUint(uint64(a.GetPolicyId()), 10))},
		df2.InterfaceDep(a.GetInterface()),
	}
}

func (d *AttachDescriptor) addDel(ctx context.Context, a *Attach, idx interface_types.InterfaceIndex, isAdd bool) error {
	req := &abfapi.AbfItfAttachAddDel{IsAdd: isAdd, Attach: abfapi.AbfItfAttach{PolicyID: a.GetPolicyId(), SwIfIndex: idx, Priority: a.GetPriority(), IsIPv6: a.GetIpv6()}}
	if _, err := abfapi.NewServiceClient(d.client).AbfItfAttachAddDel(ctx, req); err != nil {
		return fmt.Errorf("abf_itf_attach_add_del: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *AttachDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	a := obj.(*Attach)
	if !d.ids.Owns(a.GetPolicyId()) {
		return nil, fmt.Errorf("%s: policy id %d outside this agent's range", AttachName, a.GetPolicyId())
	}
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	idx, err := ifs.Index(a.GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.addDel(ctx, a, idx, true); err != nil {
		return nil, err
	}
	return AttachMeta{SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor: an attachment is immutable (priority included).
func (*AttachDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *AttachDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(AttachMeta)
	if !ok {
		return fmt.Errorf("%s: %w %T", AttachName, df2.ErrBadMeta, meta)
	}
	return d.addDel(ctx, obj.(*Attach), interface_types.InterfaceIndex(m.SwIfIndex), false)
}

// Retrieve dumps every attachment (abf_itf_attach_dump) on owned interfaces with owned
// policy ids.
func (d *AttachDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	stream, err := abfapi.NewServiceClient(d.client).AbfItfAttachDump(ctx, &abfapi.AbfItfAttachDump{})
	if err != nil {
		return nil, fmt.Errorf("abf_itf_attach_dump: %w", err)
	}
	details, err := df2.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("abf_itf_attach_dump: %w", err)
	}
	var out []scheduler.KV
	for _, det := range details {
		at := det.Attach
		if !d.ids.Owns(at.PolicyID) {
			continue
		}
		name, ok := ifs.OwnedName(uint32(at.SwIfIndex))
		if !ok {
			continue
		}
		v := &Attach{PolicyId: at.PolicyID, Interface: name, Priority: at.Priority, Ipv6: at.IsIPv6}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: AttachMeta{SwIfIndex: uint32(at.SwIfIndex)}})
	}
	return out, nil
}

// Register registers the abf descriptors (policy, attach) with r. ids scopes the policy ids
// this agent owns on a shared VPP (nil = all).
func Register(r scheduler.Registry, c vpp.Client, owner string, ids *df2.IDRange) {
	r.Register(NewPolicy(c, owner, ids))
	r.Register(NewAttach(c, owner, ids))
}
