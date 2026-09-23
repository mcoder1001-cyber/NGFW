package core

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// LoopbackDevType is sw_interface_details.interface_dev_type of a loopback in VPP 26.06
// (vnet_device_class_t "Loopback", src/vnet/ethernet/interface.c).
const LoopbackDevType = "Loopback"

// LoopbackDescriptor manages loopback interfaces "loop<N>" created with a fixed user instance
// so that the VPP name equals the document name, and tagged "<owner>:<name>".
type LoopbackDescriptor struct{ Env }

var _ scheduler.Descriptor = (*LoopbackDescriptor)(nil)

// Name implements scheduler.Descriptor.
func (*LoopbackDescriptor) Name() string { return LoopbackName }

func asLoopback(obj proto.Message) *Loopback {
	l, _ := obj.(*Loopback)
	if l == nil {
		return &Loopback{}
	}
	return l
}

// KeyOf implements scheduler.Descriptor.
func (*LoopbackDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return LoopbackKey(asLoopback(obj).GetName())
}

// Dependencies implements scheduler.Descriptor.
func (*LoopbackDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor.
func (d *LoopbackDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	l, ok := obj.(*Loopback)
	if !ok {
		return nil, fmt.Errorf("%w %T", ErrBadValue, obj)
	}
	inst, ok := LoopbackInstance(l.GetName())
	if !ok || inst != l.GetInstance() {
		return nil, fmt.Errorf("loopback: name %q does not match instance %d", l.GetName(), l.GetInstance())
	}
	tag, err := vpp.OwnerTag(d.Owner, l.GetName())
	if err != nil {
		return nil, err
	}
	svc := interfaces.NewServiceClient(d.Client)
	rep, err := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst})
	if err != nil {
		return nil, fmt.Errorf("create_loopback_instance %d: %w", inst, err)
	}
	idx := rep.SwIfIndex
	if _, err := svc.SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: idx, Tag: tag}); err != nil {
		// never leave an untagged (unowned, therefore unreachable for us) loopback behind
		_, _ = svc.DeleteLoopback(ctx, &interfaces.DeleteLoopback{SwIfIndex: idx})
		return nil, fmt.Errorf("sw_interface_tag_add_del %s: %w", tag, err)
	}
	return IfMeta{SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor. The value is fully determined by the key, so an update
// only happens when the instance changed — that needs a new interface.
func (*LoopbackDescriptor) Update(_ context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if !proto.Equal(oldObj, newObj) {
		return nil, scheduler.ErrRecreate
	}
	return meta, nil
}

// Delete implements scheduler.Descriptor.
func (d *LoopbackDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(IfMeta)
	if !ok {
		// Meta lost (should not happen: Retrieve fills it) — resolve by tag.
		t, err := dumpInterfaces(ctx, d.Client, d.Owner)
		if err != nil {
			return err
		}
		in, err := t.owned(asLoopback(obj).GetName())
		if err != nil {
			return err
		}
		m = IfMeta{SwIfIndex: in.Index}
	}
	if _, err := interfaces.NewServiceClient(d.Client).DeleteLoopback(ctx, &interfaces.DeleteLoopback{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil {
		return fmt.Errorf("delete_loopback %d: %w", m.SwIfIndex, err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: owned loopbacks (tag "<owner>:loop<N>", dev type
// Loopback).
func (d *LoopbackDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := dumpInterfaces(ctx, d.Client, d.Owner)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, in := range t.all {
		if in.ID == "" || in.DevType != LoopbackDevType {
			continue
		}
		inst, ok := LoopbackInstance(in.ID)
		if !ok {
			continue
		}
		// A loopback renamed behind our back (VPP name != tag) is reported with the instance
		// VPP actually has, so the diff recreates it.
		if vi, ok := LoopbackInstance(in.VPPName); ok {
			inst = vi
		}
		v := &Loopback{Name: in.ID, Instance: inst}
		out = append(out, scheduler.KV{Key: LoopbackKey(in.ID), Value: v, Meta: IfMeta{SwIfIndex: in.Index}})
	}
	return out, nil
}
