package afpacket

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	afpapi "ngfw/agent/binapi/af_packet"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// HostInterfaceName is the descriptor name ("af_packet.host-interface").
const HostInterfaceName = iface.HostInterfaceName

// ErrEmptyValue is returned for a nil or foreign desired value.
var ErrEmptyValue = errors.New("af_packet: nil or wrong desired value type")

// HostInterfaceDescriptor implements af_packet.host-interface (af_packet_create_v3 /
// af_packet_delete) on an existing Linux netdev. VPP names the interface host-<host_if_name>.
// af_packet_details reports only sw_if_index and host_if_name, so flags, frame sizes and
// queue counts are not modelled (VPP defaults, create-only); mode is decoded from the presence
// of an L2 address (IP-mode interfaces register with the ip hw class and have none).
type HostInterfaceDescriptor struct {
	client vpp.Client
	owner  string
}

// New returns the descriptor for owner.
func New(c vpp.Client, owner string) *HostInterfaceDescriptor { return &HostInterfaceDescriptor{c, owner} }

func (d *HostInterfaceDescriptor) svc() afpapi.RPCService { return afpapi.NewServiceClient(d.client) }

func (*HostInterfaceDescriptor) Name() string { return HostInterfaceName }

func (*HostInterfaceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(HostInterfaceName, obj.(*HostInterface).GetName())
}

// Dependencies: none in VPP — the Linux netdev is a precondition (the veth rig or a data NIC).
func (*HostInterfaceDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *HostInterfaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*HostInterface)
	if !ok {
		return nil, ErrEmptyValue
	}
	if o.GetName() == "" || o.GetHostIfName() == "" || len(o.GetHostIfName()) > 15 {
		return nil, errors.New("af_packet: name and host_if_name (≤ 15 bytes) are mandatory")
	}
	mode := afpapi.AF_PACKET_API_MODE_ETHERNET
	if o.GetMode() == Mode_MODE_IP {
		mode = afpapi.AF_PACKET_API_MODE_IP
	}
	rep, err := d.svc().AfPacketCreateV3(ctx, &afpapi.AfPacketCreateV3{
		Mode: mode, UseRandomHwAddr: true, HostIfName: o.GetHostIfName(), NumRxQueues: 1, NumTxQueues: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("af_packet_create_v3: %w", err)
	}
	idx := uint32(rep.SwIfIndex)
	if err := iface.Tag(ctx, d.client, d.owner, o.GetName(), idx); err != nil {
		return iface.Meta{SwIfIndex: idx}, err
	}
	return iface.Meta{SwIfIndex: idx}, nil
}

func (*HostInterfaceDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

func (d *HostInterfaceDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	if _, err := iface.MetaOf(meta); err != nil {
		return err
	}
	o, ok := obj.(*HostInterface)
	if !ok {
		return ErrEmptyValue
	}
	if _, err := d.svc().AfPacketDelete(ctx, &afpapi.AfPacketDelete{HostIfName: o.GetHostIfName()}); err != nil {
		return fmt.Errorf("af_packet_delete: %w", err)
	}
	return nil
}

func (d *HostInterfaceDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	stream, err := d.svc().AfPacketDump(ctx, &afpapi.AfPacketDump{})
	if err != nil {
		return nil, fmt.Errorf("af_packet_dump: %w", err)
	}
	var out []scheduler.KV
	for {
		row, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("af_packet_dump: %w", err)
		}
		idx := uint32(row.SwIfIndex)
		key, ok := t.KeyFor(idx)
		if !ok || key.Descriptor() != HostInterfaceName {
			continue
		}
		det, _ := t.Details(idx)
		mode := Mode_MODE_ETHERNET
		if iface.FormatMAC(det.L2Address) == "" {
			mode = Mode_MODE_IP
		}
		out = append(out, scheduler.KV{
			Key:   key,
			Value: &HostInterface{Name: key.ID(), HostIfName: row.HostIfName, Mode: mode},
			Meta:  iface.Meta{SwIfIndex: idx},
		})
	}
	return out, nil
}

// Register registers the host-interface descriptor with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) { r.Register(New(c, owner)) }
