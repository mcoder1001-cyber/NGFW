package tapv2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	tapapi "ngfw/agent/binapi/tapv2"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// TapName is the descriptor name ("tapv2.tap").
const TapName = iface.TapName

// ErrEmptyValue is returned for a nil or foreign desired value.
var ErrEmptyValue = errors.New("tapv2: nil or wrong desired value type")

// TapDescriptor implements tapv2.tap (tap_create_v3 / tap_delete_v2). A tap is immutable in VPP →
// Update is a recreate. The owner tag is passed in tap_create_v3.tag.
type TapDescriptor struct {
	client vpp.Client
	owner  string
}

// New returns the descriptor for owner.
func New(c vpp.Client, owner string) *TapDescriptor { return &TapDescriptor{c, owner} }

func (d *TapDescriptor) svc() tapapi.RPCService { return tapapi.NewServiceClient(d.client) }

func (*TapDescriptor) Name() string { return TapName }

func (*TapDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(TapName, obj.(*Tap).GetName())
}

func (*TapDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func ip4Prefix(s string) (ip_types.IP4AddressWithPrefix, bool, error) {
	if s == "" {
		return ip_types.IP4AddressWithPrefix{}, false, nil
	}
	p, err := netip.ParsePrefix(s)
	if err != nil || !p.Addr().Is4() {
		return ip_types.IP4AddressWithPrefix{}, false, fmt.Errorf("tapv2: invalid IPv4 prefix %q", s)
	}
	return ip_types.IP4AddressWithPrefix{Address: ip_types.IP4Address(p.Addr().As4()), Len: uint8(p.Bits())}, true, nil //nolint:gosec // 0-32
}

func ip6Prefix(s string) (ip_types.IP6AddressWithPrefix, bool, error) {
	if s == "" {
		return ip_types.IP6AddressWithPrefix{}, false, nil
	}
	p, err := netip.ParsePrefix(s)
	if err != nil || !p.Addr().Is6() || p.Addr().Is4In6() {
		return ip_types.IP6AddressWithPrefix{}, false, fmt.Errorf("tapv2: invalid IPv6 prefix %q", s)
	}
	return ip_types.IP6AddressWithPrefix{Address: ip_types.IP6Address(p.Addr().As16()), Len: uint8(p.Bits())}, true, nil //nolint:gosec // 0-128
}

func formatIP4Prefix(p ip_types.IP4AddressWithPrefix) string {
	if p.Len == 0 && p.Address == (ip_types.IP4Address{}) {
		return ""
	}
	return netip.PrefixFrom(netip.AddrFrom4(p.Address), int(p.Len)).String()
}

func formatIP6Prefix(p ip_types.IP6AddressWithPrefix) string {
	if p.Len == 0 && p.Address == (ip_types.IP6Address{}) {
		return ""
	}
	return netip.PrefixFrom(netip.AddrFrom16(p.Address), int(p.Len)).String()
}

func (d *TapDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Tap)
	if !ok {
		return nil, ErrEmptyValue
	}
	if o.GetName() == "" || o.GetId() == ^uint32(0) {
		return nil, errors.New("tapv2: name and id are mandatory")
	}
	if o.GetRxRingSize() == 0 || o.GetRxRingSize() > 32768 || o.GetTxRingSize() == 0 || o.GetTxRingSize() > 32768 {
		return nil, fmt.Errorf("tapv2: ring sizes must be 1..32768 (VPP default 256), got %d/%d", o.GetRxRingSize(), o.GetTxRingSize())
	}
	tag, err := vpp.OwnerTag(d.owner, o.GetName())
	if err != nil {
		return nil, err
	}
	req := &tapapi.TapCreateV3{
		ID: o.GetId(), UseRandomMac: true, NumRxQueues: 1, NumTxQueues: 1,
		RxRingSz: uint16(o.GetRxRingSize()), TxRingSz: uint16(o.GetTxRingSize()), //nolint:gosec // range-checked
		Tag: tag,
	}
	if o.GetHostIfName() != "" {
		if len(o.GetHostIfName()) > 15 {
			return nil, fmt.Errorf("tapv2: host_if_name %q exceeds IFNAMSIZ", o.GetHostIfName())
		}
		req.HostIfNameSet, req.HostIfName = true, o.GetHostIfName()
	}
	if o.GetHostNamespace() != "" {
		req.HostNamespaceSet, req.HostNamespace = true, o.GetHostNamespace()
	}
	if o.GetHostBridge() != "" {
		req.HostBridgeSet, req.HostBridge = true, o.GetHostBridge()
	}
	if p, set, err := ip4Prefix(o.GetHostIp4Prefix()); err != nil {
		return nil, err
	} else if set {
		req.HostIP4PrefixSet, req.HostIP4Prefix = true, p
	}
	if p, set, err := ip6Prefix(o.GetHostIp6Prefix()); err != nil {
		return nil, err
	} else if set {
		req.HostIP6PrefixSet, req.HostIP6Prefix = true, p
	}
	if o.GetHostMtu() != 0 {
		req.HostMtuSet, req.HostMtuSize = true, o.GetHostMtu()
	}
	if o.GetGso() {
		req.TapFlags |= tapapi.TAP_API_FLAG_GSO
	}
	if o.GetCsumOffload() {
		req.TapFlags |= tapapi.TAP_API_FLAG_CSUM_OFFLOAD
	}
	rep, err := d.svc().TapCreateV3(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("tap_create_v3: %w", err)
	}
	return iface.Meta{SwIfIndex: uint32(rep.SwIfIndex)}, nil
}

func (*TapDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

func (d *TapDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return err
	}
	if _, err := d.svc().TapDeleteV2(ctx, &tapapi.TapDeleteV2{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil {
		return fmt.Errorf("tap_delete_v2: %w", err)
	}
	return nil
}

// Decode converts a dump row into the model (exported for tests).
func Decode(name string, t *tapapi.SwInterfaceTapV2Details) *Tap {
	return &Tap{
		Name: name, Id: t.ID, HostIfName: t.HostIfName, HostNamespace: t.HostNamespace, HostBridge: t.HostBridge,
		HostIp4Prefix: formatIP4Prefix(t.HostIP4Prefix), HostIp6Prefix: formatIP6Prefix(t.HostIP6Prefix),
		HostMtu: t.HostMtuSize, RxRingSize: uint32(t.RxRingSz), TxRingSize: uint32(t.TxRingSz),
		Gso: t.TapFlags&tapapi.TAP_API_FLAG_GSO != 0, CsumOffload: t.TapFlags&tapapi.TAP_API_FLAG_CSUM_OFFLOAD != 0,
	}
}

func (d *TapDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	stream, err := d.svc().SwInterfaceTapV2Dump(ctx, &tapapi.SwInterfaceTapV2Dump{SwIfIndex: interface_types.InterfaceIndex(iface.AllInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_tap_v2_dump: %w", err)
	}
	var out []scheduler.KV
	for {
		row, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("sw_interface_tap_v2_dump: %w", err)
		}
		key, ok := t.KeyFor(row.SwIfIndex)
		if !ok || key.Descriptor() != TapName {
			continue
		}
		out = append(out, scheduler.KV{Key: key, Value: Decode(key.ID(), row), Meta: iface.Meta{SwIfIndex: row.SwIfIndex}})
	}
	return out, nil
}

// Register registers the tap descriptor with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) { r.Register(New(c, owner)) }
