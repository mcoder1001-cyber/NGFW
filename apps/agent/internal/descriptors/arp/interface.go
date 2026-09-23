package arp

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	arpapi "ngfw/agent/binapi/arp"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// InterfaceName is the descriptor name; keys are "arp.proxy-interface/<interface>".
const InterfaceName = "arp.proxy-interface"

// InterfaceDescriptor enables proxy ARP on interfaces (proxy_arp_intfc_enable_disable).
type InterfaceDescriptor struct {
	client vpp.Client
	owner  string
}

// NewInterface returns the descriptor for the given owner.
func NewInterface(c vpp.Client, owner string) *InterfaceDescriptor {
	return &InterfaceDescriptor{client: c, owner: owner}
}

// InterfaceMeta is the runtime handle.
type InterfaceMeta struct{ SwIfIndex uint32 }

// Name implements scheduler.Descriptor.
func (*InterfaceDescriptor) Name() string { return InterfaceName }

// KeyOf implements scheduler.Descriptor.
func (*InterfaceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(InterfaceName, obj.(*ProxyInterface).GetInterface())
}

// Dependencies implements scheduler.Descriptor.
func (*InterfaceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{df2.InterfaceDep(obj.(*ProxyInterface).GetInterface())}
}

func (d *InterfaceDescriptor) set(ctx context.Context, idx interface_types.InterfaceIndex, enable bool) error {
	if _, err := arpapi.NewServiceClient(d.client).ProxyArpIntfcEnableDisable(ctx, &arpapi.ProxyArpIntfcEnableDisable{SwIfIndex: idx, Enable: enable}); err != nil {
		return fmt.Errorf("proxy_arp_intfc_enable_disable: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *InterfaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	idx, err := ifs.Index(obj.(*ProxyInterface).GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, idx, true); err != nil {
		return nil, err
	}
	return InterfaceMeta{SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor: the interface is the key, nothing else to change.
func (*InterfaceDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *InterfaceDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, ok := meta.(InterfaceMeta)
	if !ok {
		return fmt.Errorf("%s: %w %T", InterfaceName, df2.ErrBadMeta, meta)
	}
	return d.set(ctx, interface_types.InterfaceIndex(m.SwIfIndex), false)
}

// Retrieve lists the enabled interfaces (proxy_arp_intfc_dump) owned by this agent.
func (d *InterfaceDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	stream, err := arpapi.NewServiceClient(d.client).ProxyArpIntfcDump(ctx, &arpapi.ProxyArpIntfcDump{})
	if err != nil {
		return nil, fmt.Errorf("proxy_arp_intfc_dump: %w", err)
	}
	details, err := df2.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("proxy_arp_intfc_dump: %w", err)
	}
	var out []scheduler.KV
	for _, det := range details {
		name, ok := ifs.OwnedName(det.SwIfIndex)
		if !ok {
			continue
		}
		v := &ProxyInterface{Interface: name}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: InterfaceMeta{SwIfIndex: det.SwIfIndex}})
	}
	return out, nil
}
