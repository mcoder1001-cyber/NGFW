package ipsec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

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

// Directions and VPP's "any protocol" value (IPSEC_POLICY_PROTOCOL_ANY = IP_PROTOCOL_RESERVED).
// ipsec_spd_entry_add_del_v2 stores protocol exactly as sent — unlike the v1 handler it does NOT
// map 0 to "any" (verified in VPP 26.06 ipsec_api.c and on the host: protocol 0 shows as
// IP6_HOP_BY_HOP_OPTIONS). Desired protocol 0 means "any" and is sent as 255; a literal IP
// protocol 0 (HOPOPT) cannot be expressed.
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
	// a policy belongs to its SPD's owner: never add one to an SPD we did not create (D-071)
	if owned, _, err := ownedSpd(ctx, d.cfg, o.GetSpdId()); err != nil {
		return nil, err
	} else if !owned {
		return nil, fmt.Errorf("%s: spd %d: %w", SpdEntryName, o.GetSpdId(), vpn.ErrNotOurs)
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

// Delete implements scheduler.Descriptor. Right before the delete it re-checks that the SPD is
// ours and that the policy is still in it (D-071, D-074): a policy that is gone needs nothing.
func (d *SpdEntry) Delete(ctx context.Context, obj proto.Message, _ any) error {
	o, ok := obj.(*vpnpb.IpsecSpdEntry)
	if !ok {
		return typeErr(SpdEntryName, obj)
	}
	e, err := encodeSpdEntry(o)
	if err != nil {
		return err
	}
	owned, exists, err := ownedSpd(ctx, d.cfg, o.GetSpdId())
	if err != nil {
		return err
	}
	if !exists {
		return nil // the SPD and its policies are gone
	}
	if !owned {
		return fmt.Errorf("%s: spd %d: %w", SpdEntryName, o.GetSpdId(), vpn.ErrNotOurs)
	}
	cur, err := d.dumpSpd(ctx, o.GetSpdId())
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(cur, func(v *vpnpb.IpsecSpdEntry) bool { return d.KeyOf(v) == d.KeyOf(o) }) {
		return nil
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecSpdEntryAddDelV2(ctx, &ipsec.IpsecSpdEntryAddDelV2{IsAdd: false, Entry: e}); err != nil {
		return fmt.Errorf("ipsec_spd_entry_add_del_v2 (del): %w", err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: the policies of every owned SPD.
func (d *SpdEntry) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ids, err := ownedSpdIDs(ctx, d.cfg)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, id := range ids {
		vs, err := d.dumpSpd(ctx, id)
		if err != nil {
			return nil, err
		}
		for _, v := range vs {
			out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v})
		}
	}
	return sortKVs(out), nil
}

// dumpSpd returns the decoded policies of SPD id (ipsec_spd_dump).
func (d *SpdEntry) dumpSpd(ctx context.Context, id uint32) ([]*vpnpb.IpsecSpdEntry, error) {
	stream, err := ipsec.NewServiceClient(d.cfg.Client).IpsecSpdDump(ctx, &ipsec.IpsecSpdDump{SpdID: id, SaID: ^uint32(0)})
	if err != nil {
		return nil, fmt.Errorf("ipsec_spd_dump: %w", err)
	}
	var out []*vpnpb.IpsecSpdEntry
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("ipsec_spd_dump: %w", err)
		}
		if det.Entry.SpdID == id {
			out = append(out, decodeSpdEntry(det.Entry))
		}
	}
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
	if o.GetProtocol() >= protocolAny {
		return e, fmt.Errorf("ipsec: spd entry protocol %d out of range 0 (any)–254", o.GetProtocol())
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
	if e.Protocol == 0 {
		e.Protocol = protocolAny
	}
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
