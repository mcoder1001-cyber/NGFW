package ipsec

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/ipsec_types"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// SpdEntry manages SPD policies (ipsec_spd_entry_add_del_v2; dump ipsec_spd_dump per owned SPD).
// VPP identifies a policy by all of its fields, so every field is part of the key and Update is
// always ErrRecreate. Meta is nil: ipsec_spd_details carries no stat index and Delete needs none.
type SpdEntry struct{ cfg Config }

// Directions and the "any protocol" value VPP uses internally (IP_PROTOCOL_RESERVED).
const (
	Inbound     = "inbound"
	Outbound    = "outbound"
	protocolAny = 255
)

// NewSpdEntry returns the descriptor.
func NewSpdEntry(cfg Config) *SpdEntry { return &SpdEntry{cfg: cfg} }

// Name implements scheduler.Descriptor.
func (*SpdEntry) Name() string { return SpdEntryName }

// KeyOf implements scheduler.Descriptor: the canonical form of every field.
func (*SpdEntry) KeyOf(obj proto.Message) scheduler.Key {
	o, _ := obj.(*vpnpb.IpsecSpdEntry)
	return scheduler.Join(SpdEntryName, vpn.Uint(o.GetSpdId()), o.GetDirection(), fmt.Sprint(o.GetPriority()),
		o.GetAction(), vpn.Uint(o.GetSaId()), vpn.Uint(o.GetProtocol()),
		canon(o.GetLocalStart())+"-"+canon(o.GetLocalStop()),
		fmt.Sprintf("%d-%d", o.GetLocalPortStart(), o.GetLocalPortStop()),
		canon(o.GetRemoteStart())+"-"+canon(o.GetRemoteStop()),
		fmt.Sprintf("%d-%d", o.GetRemotePortStart(), o.GetRemotePortStop()))
}

// canon canonicalises an address for a key; an unparsable text stays as typed (Create rejects it).
func canon(s string) string {
	if c, err := vpn.CanonicalAddress(s); err == nil {
		return c
	}
	return s
}

// Dependencies implements scheduler.Descriptor: the SPD, plus the SA for protect policies.
func (*SpdEntry) Dependencies(obj proto.Message) []scheduler.Dependency {
	o, _ := obj.(*vpnpb.IpsecSpdEntry)
	deps := []scheduler.Dependency{{Key: scheduler.Join(SpdName, vpn.Uint(o.GetSpdId()))}}
	if o.GetAction() == actions.name(ipsec_types.IPSEC_API_SPD_ACTION_PROTECT) {
		deps = append(deps, scheduler.Dependency{Key: scheduler.Join(SaName, vpn.Uint(o.GetSaId()))})
	}
	return deps
}

// Create implements scheduler.Descriptor.
func (d *SpdEntry) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.IpsecSpdEntry)
	if !ok {
		return nil, typeErr(SpdEntryName, obj)
	}
	if err := d.cfg.IDs.Check("spd", o.GetSpdId()); err != nil {
		return nil, err
	}
	e, err := encodeSpdEntry(o)
	if err != nil {
		return nil, err
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecSpdEntryAddDelV2(ctx, &ipsec.IpsecSpdEntryAddDelV2{IsAdd: true, Entry: e}); err != nil {
		return nil, fmt.Errorf("ipsec_spd_entry_add_del_v2: %w", err)
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: every field is identity.
func (*SpdEntry) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *SpdEntry) Delete(ctx context.Context, obj proto.Message, _ any) error {
	o, ok := obj.(*vpnpb.IpsecSpdEntry)
	if !ok {
		return typeErr(SpdEntryName, obj)
	}
	e, err := encodeSpdEntry(o)
	if err != nil {
		return err
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecSpdEntryAddDelV2(ctx, &ipsec.IpsecSpdEntryAddDelV2{IsAdd: false, Entry: e}); err != nil {
		return fmt.Errorf("ipsec_spd_entry_add_del_v2: %w", err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: the policies of every owned SPD.
func (d *SpdEntry) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ids, err := ownedSpdIDs(ctx, d.cfg)
	if err != nil {
		return nil, err
	}
	svc := ipsec.NewServiceClient(d.cfg.Client)
	var out []scheduler.KV
	for _, id := range ids {
		stream, err := svc.IpsecSpdDump(ctx, &ipsec.IpsecSpdDump{SpdID: id, SaID: ^uint32(0)})
		if err != nil {
			return nil, fmt.Errorf("ipsec_spd_dump: %w", err)
		}
		for {
			det, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("ipsec_spd_dump: %w", err)
			}
			v := decodeSpdEntry(det.Entry)
			out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v})
		}
	}
	sortKVs(out)
	return out, nil
}

func encodeSpdEntry(o *vpnpb.IpsecSpdEntry) (ipsec_types.IpsecSpdEntryV2, error) {
	var e ipsec_types.IpsecSpdEntryV2
	switch o.GetDirection() {
	case Inbound:
	case Outbound:
		e.IsOutbound = true
	default:
		return e, fmt.Errorf("ipsec: spd entry direction %q (want %s|%s)", o.GetDirection(), Inbound, Outbound)
	}
	action, err := actions.value("spd action", o.GetAction())
	if err != nil {
		return e, err
	}
	if action == ipsec_types.IPSEC_API_SPD_ACTION_PROTECT && o.GetSaId() == 0 {
		return e, errors.New("ipsec: spd entry action protect needs sa_id")
	}
	if o.GetProtocol() > 255 {
		return e, fmt.Errorf("ipsec: spd entry protocol %d > 255", o.GetProtocol())
	}
	for _, p := range []uint32{o.GetLocalPortStart(), o.GetLocalPortStop(), o.GetRemotePortStart(), o.GetRemotePortStop()} {
		if p > 65535 {
			return e, fmt.Errorf("ipsec: spd entry port %d > 65535", p)
		}
	}
	addrs := [4]ip_types.Address{}
	for i, s := range []string{o.GetLocalStart(), o.GetLocalStop(), o.GetRemoteStart(), o.GetRemoteStop()} {
		if addrs[i], err = vpn.ParseAddress(s); err != nil {
			return e, err
		}
	}
	e.SpdID = o.GetSpdId()
	e.Priority = o.GetPriority()
	e.SaID = o.GetSaId()
	e.Policy = action
	e.Protocol = uint8(o.GetProtocol()) //nolint:gosec // checked above
	e.LocalAddressStart, e.LocalAddressStop = addrs[0], addrs[1]
	e.RemoteAddressStart, e.RemoteAddressStop = addrs[2], addrs[3]
	e.LocalPortStart, e.LocalPortStop = uint16(o.GetLocalPortStart()), uint16(o.GetLocalPortStop())     //nolint:gosec // checked
	e.RemotePortStart, e.RemotePortStop = uint16(o.GetRemotePortStart()), uint16(o.GetRemotePortStop()) //nolint:gosec // checked
	return e, nil
}

func decodeSpdEntry(e ipsec_types.IpsecSpdEntry) *vpnpb.IpsecSpdEntry {
	dir := Inbound
	if e.IsOutbound {
		dir = Outbound
	}
	protocol := uint32(e.Protocol)
	if protocol == protocolAny {
		protocol = 0
	}
	saID := e.SaID
	if e.Policy != ipsec_types.IPSEC_API_SPD_ACTION_PROTECT {
		saID = 0
	}
	return &vpnpb.IpsecSpdEntry{
		SpdId: e.SpdID, Priority: e.Priority, Direction: dir, Action: actions.name(e.Policy), SaId: saID,
		Protocol:       protocol,
		LocalStart:     vpn.AddressString(e.LocalAddressStart),
		LocalStop:      vpn.AddressString(e.LocalAddressStop),
		RemoteStart:    vpn.AddressString(e.RemoteAddressStart),
		RemoteStop:     vpn.AddressString(e.RemoteAddressStop),
		LocalPortStart: uint32(e.LocalPortStart), LocalPortStop: uint32(e.LocalPortStop),
		RemotePortStart: uint32(e.RemotePortStart), RemotePortStop: uint32(e.RemotePortStop),
	}
}
