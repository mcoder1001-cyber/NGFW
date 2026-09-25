package afpacket

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/protobuf/proto"

	afpapi "ngfw/agent/binapi/af_packet"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/ifsanitize"
)

// HostInterfaceName is the descriptor name ("af-packet.host-interface").
const HostInterfaceName = iface.HostInterfaceName

// The rings every af_packet interface is created with (D-108, amended by D-113). VPP's defaults
// (af_packet.c:37-48) are a TX ring of 66 KiB frames (2048 × 33, sized for a 64 KB GSO packet) ×
// 1024 in one block, ≈ 66 MiB, and a v3 RX ring of 2048 × 32 × 160 blocks, 10 MiB. Allocating and
// touching them stalled VPP's only thread — every API client — for 40 s to 5.5 min on the lab VM
// (I6; ESXi memory reclaim, 11.6 s measured for the RX ring alone). The rings shrink by frame
// COUNT only: TX 67584 × 16 in one block = 1,081,344 B (264 pages) ≈ 1 MiB, RX 2048 × 8 × 160
// blocks ≈ 2.5 MiB — ≈ 3.5 MiB instead of 76 MiB, the same as the tools/lab rig.
//
// The TX frame SIZE must never shrink below VPP's default (D-113, TD-5 review H1): the TX path
// copies every buffer of a packet into its frame slot with no length check against the frame size
// (af_packet/device.c:561-573; the v2 path the same), behind a 48-byte header. A GSO or jumbo
// frame (the lab veths have TSO/GSO on, the ethernet-mode interface has an L3 MTU of 9000, L2
// paths never check the MTU) would overrun a 2048-byte slot, and an overrun of the last slot
// writes past the ring's mmap: a crash of the shared VPP.
const (
	TxFrameSize      = 2048 * 33 // 67584: VPP's AF_PACKET_DEFAULT_TX_FRAME_SIZE ("GSO packet of 64KB")
	TxFramesPerBlock = 16
	RxFrameSize      = 2048
	RxFramesPerBlock = 8
)

// RollbackTimeout bounds Create's rollback delete (quiesce + af_packet_delete). The rollback runs
// on a context detached from the caller's (context.WithoutCancel): a Create that failed because
// its context expired (an I6 API stall) must still remove the untagged interface, not leave an
// orphan in VPP and a netdev down (TD-5 review L2).
const RollbackTimeout = 30 * time.Second

// ErrEmptyValue is returned for a nil or foreign desired value.
var ErrEmptyValue = errors.New("af_packet: nil or wrong desired value type")

// HostInterfaceDescriptor implements af-packet.host-interface (af_packet_create_v3 /
// af_packet_delete) on an existing Linux netdev. VPP names the interface host-<host_if_name>.
// af_packet_details reports only sw_if_index and host_if_name, so flags, frame sizes and
// queue counts are not modelled (VPP defaults, create-only); mode is decoded from the presence
// of an L2 address (IP-mode interfaces register with the ip hw class and have none).
//
// Every af_packet_delete (Delete, and Create's rollback of an untagged or unclean interface) is
// sent only after the host netdev was brought down (quiesce.go, D-101 / VPP V24).
type HostInterfaceDescriptor struct {
	client vpp.Client
	owner  string
	links  Links                           // the Linux side of the quiesce (netlink)
	settle func(ctx context.Context) error // wait after the link-down (DefaultSettle)
}

// New returns the descriptor for owner.
func New(c vpp.Client, owner string, opts ...Option) *HostInterfaceDescriptor {
	d := &HostInterfaceDescriptor{client: c, owner: owner, links: defaultLinks(), settle: sleep(DefaultSettle)}
	for _, o := range opts {
		o(d)
	}
	return d
}

func (d *HostInterfaceDescriptor) svc() afpapi.RPCService { return afpapi.NewServiceClient(d.client) }

// Name implements scheduler.Descriptor.
func (*HostInterfaceDescriptor) Name() string { return HostInterfaceName }

// KeyOf implements scheduler.Descriptor.
func (*HostInterfaceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(HostInterfaceName, obj.(*HostInterface).GetName())
}

// Dependencies implements scheduler.Descriptor: none in VPP — the Linux netdev is a precondition (the veth rig or a data NIC).
func (*HostInterfaceDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor.
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
	// an untagged or unclean host-interface is removed again (review M3, D-095)
	idx, err := iface.AcquireAndTag(ctx, d.client, d.owner, o.GetName(), func() (uint32, error) {
		rep, err := d.svc().AfPacketCreateV3(ctx, &afpapi.AfPacketCreateV3{
			Mode: mode, UseRandomHwAddr: true, HostIfName: o.GetHostIfName(), NumRxQueues: 1, NumTxQueues: 1,
			TxFrameSize: TxFrameSize, TxFramesPerBlock: TxFramesPerBlock, // D-108/D-113: fewer frames, full-size TX slots (≈ 3.5 MiB, not 76 MiB)
			RxFrameSize: RxFrameSize, RxFramesPerBlock: RxFramesPerBlock,
		})
		if err != nil {
			return 0, fmt.Errorf("af_packet_create_v3: %w", err)
		}
		return uint32(rep.SwIfIndex), nil
	}, func(idx uint32) error {
		// TD-3 re-review L7: the rollback delete quiesces too; a failed quiesce leaves the
		// untagged interface in VPP rather than risk the V24 crash (fail closed). Review L2: on its
		// own bounded context, so a Create whose context expired still rolls back.
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), RollbackTimeout)
		defer cancel()
		err := d.quiescedDelete(rctx, o.GetHostIfName(), nil)
		if errors.Is(err, ErrQuiesce) {
			return fmt.Errorf("untagged orphan host-%s (sw_if_index %d) left in VPP: %w", o.GetHostIfName(), idx, err)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return iface.Meta{SwIfIndex: idx}, nil
}

// Update implements scheduler.Descriptor.
func (*HostInterfaceDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *HostInterfaceDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return err
	}
	o, ok := obj.(*HostInterface)
	if !ok {
		return ErrEmptyValue
	}
	// D-101 / V24: the netdev goes down first, so no frame crosses the interface while its
	// bindings are cleared; then D-095 / review H3: clear every binding while its tables still
	// exist; then af_packet_delete
	return d.quiescedDelete(ctx, o.GetHostIfName(), func() error {
		return ifsanitize.BeforeDelete(ctx, d.client, m.SwIfIndex, o.GetName())
	})
}

// Retrieve implements scheduler.Descriptor.
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
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...Option) {
	r.Register(New(c, owner, opts...))
}
