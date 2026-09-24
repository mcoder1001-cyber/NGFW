package ipsec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/bits"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/ipsec_types"
	"ngfw/agent/binapi/tunnel_types"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
)

// Sa manages security associations (ipsec_sad_entry_add_v2 / ipsec_sad_entry_del; dump
// ipsec_sa_v5_dump). Keys are secret references (vpn package): Create resolves them, Retrieve
// hashes the material VPP returns into the same references and zeroes it. Any change is
// ErrRecreate (ipsec_sad_entry_update could move the tunnel/UDP ports in place; not used, see
// docs/agent/descriptors/ipsec.md).
type Sa struct{ cfg Config }

// SaMeta is the runtime handle of an SA.
type SaMeta struct {
	SadID     uint32
	StatIndex uint32
}

// Default anti-replay window when the flag is set (VPP: IPSEC_SA_ANTI_REPLAY_WINDOW_SIZE).
const defaultReplayWindow = 64

// NewSa returns the descriptor.
func NewSa(cfg Config) *Sa { return &Sa{cfg: cfg} }

// Name implements scheduler.Descriptor.
func (*Sa) Name() string { return SaName }

// KeyOf implements scheduler.Descriptor: ipsec.sa/<sad_id>.
func (*Sa) KeyOf(obj proto.Message) scheduler.Key {
	o, _ := obj.(*vpnpb.IpsecSa)
	return scheduler.Join(SaName, vpn.Uint(o.GetSadId()))
}

// Dependencies implements scheduler.Descriptor: the outer FIB table of a tunnel SA (Optional).
func (*Sa) Dependencies(obj proto.Message) []scheduler.Dependency {
	o, _ := obj.(*vpnpb.IpsecSa)
	if t := o.GetTunnel(); t != nil && t.GetTableId() != 0 {
		return []scheduler.Dependency{{Key: vpn.VRFKey(t.GetTableId()), Optional: true}}
	}
	return nil
}

// saRecordKey is the ownership record of an SA; the value is "<spi>/<protocol>", so an SA that
// another party created under the same id after a VPP restart never matches.
func saRecordKey(id uint32) string { return string(scheduler.Join(SaName, vpn.Uint(id))) }

func saRecordValue(spi uint32, protocol string) string { return vpn.Uint(spi) + "/" + protocol }

// Create implements scheduler.Descriptor. VPP refuses an existing sad_id
// (ENTRY_ALREADY_EXISTS), so an SA that is not ours is never adopted; the ownership record is
// written only after VPP accepted the add.
func (d *Sa) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.IpsecSa)
	if !ok {
		return nil, typeErr(SaName, obj)
	}
	if err := d.cfg.IDs.Check("sa", o.GetSadId()); err != nil {
		return nil, err
	}
	rec := d.cfg.records()
	bid, err := rec.Identity(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SaName, err)
	}
	entry, err := d.encodeSa(ctx, o)
	if err != nil {
		return nil, err
	}
	defer vpn.Zero(entry.CryptoKey.Data)
	defer vpn.Zero(entry.IntegrityKey.Data)
	key, value := saRecordKey(o.GetSadId()), saRecordValue(o.GetSpi(), o.GetProtocol())
	cur, err := d.dumpDetails(ctx, o.GetSadId())
	if err != nil {
		return nil, err
	}
	for _, sa := range cur {
		if sa.value.GetSadId() != o.GetSadId() {
			continue
		}
		// exists: ours only with our record for this SPI (a retry after a lost reply); a changed
		// SA is planned as Update → ErrRecreate by the scheduler, never here
		if v, ok := rec.Valid(bid, key); ok && v == value && proto.Equal(sa.value, o) {
			return SaMeta{SadID: o.GetSadId(), StatIndex: sa.statIndex}, rec.Put(bid, key, value)
		}
		return nil, fmt.Errorf("%s: sa %d exists: %w", SaName, o.GetSadId(), vpn.ErrNotOurs)
	}
	if err := rec.PutPending(bid, key, value); err != nil { // write-ahead (review M4)
		return nil, fmt.Errorf("%s: record: %w", SaName, err)
	}
	rep, err := ipsec.NewServiceClient(d.cfg.Client).IpsecSadEntryAddV2(ctx, &ipsec.IpsecSadEntryAddV2{Entry: entry})
	if err != nil {
		if after, derr := d.dump(ctx, o.GetSadId()); derr == nil && len(after) == 0 {
			_ = rec.Drop(key)
		}
		return nil, fmt.Errorf("ipsec_sad_entry_add_v2 (sa %d): %w", o.GetSadId(), err)
	}
	if err := rec.Put(bid, key, value); err != nil {
		return nil, fmt.Errorf("%s: record: %w", SaName, err)
	}
	return SaMeta{SadID: o.GetSadId(), StatIndex: rep.StatIndex}, nil
}

// Update implements scheduler.Descriptor: an SA is immutable (a key change is a new SA).
func (*Sa) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor. Right before the delete it re-reads the SA by id and
// re-checks our record against the running VPP instance and the SA's SPI/protocol (D-071, D-074):
// an SA that is gone needs nothing, an SA that is not ours is refused (ErrNotOurs).
func (d *Sa) Delete(ctx context.Context, obj proto.Message, _ any) error {
	o, ok := obj.(*vpnpb.IpsecSa)
	if !ok {
		return typeErr(SaName, obj)
	}
	rec := d.cfg.records()
	key := saRecordKey(o.GetSadId())
	cur, err := d.dump(ctx, o.GetSadId())
	if err != nil {
		return err
	}
	if len(cur) == 0 {
		return rec.Drop(key)
	}
	bid, err := rec.Identity(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", SaName, err)
	}
	if v, ok := rec.Valid(bid, key); !ok || !d.cfg.IDs.Contains(o.GetSadId()) || v != saRecordValue(cur[0].GetSpi(), cur[0].GetProtocol()) {
		return fmt.Errorf("%s: sa %d: %w", SaName, o.GetSadId(), vpn.ErrNotOurs)
	}
	// ipsec_sad_entry_del is an UNLOCK (VPP lock counting): with a policy or a tunnel protection
	// still holding the SA, a (retried) unlock could drop THEIR lock and free the SA under them
	// (review H3). Unlock only when nothing references it any more.
	if used, err := protectionUses(ctx, d.cfg, o.GetSadId()); err != nil {
		return err
	} else if used {
		return fmt.Errorf("%s: sa %d is still used by a tunnel protection; not unlocked", SaName, o.GetSadId())
	}
	if refs, err := policiesUsing(ctx, d.cfg, o.GetSadId()); err != nil {
		return err
	} else if len(refs) > 0 {
		return fmt.Errorf("%s: sa %d is still used by %d SPD policies; not unlocked", SaName, o.GetSadId(), len(refs))
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecSadEntryDel(ctx, &ipsec.IpsecSadEntryDel{ID: o.GetSadId()}); err != nil {
		return fmt.Errorf("ipsec_sad_entry_del (sa %d): %w", o.GetSadId(), err)
	}
	return rec.Drop(key)
}

// Retrieve implements scheduler.Descriptor: every SA in the owned id range whose record matches
// (running VPP instance, SPI, protocol); keys reduced to references.
func (d *Sa) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	all, err := d.dumpDetails(ctx, ^uint32(0))
	if err != nil {
		return nil, err
	}
	rec := d.cfg.records()
	var out []scheduler.KV
	var bid bootid.Identity
	for _, sa := range all {
		v := sa.value
		if !d.cfg.IDs.Contains(v.GetSadId()) {
			continue
		}
		if bid.IsZero() {
			if bid, err = rec.Identity(ctx); err != nil {
				return nil, fmt.Errorf("%s: %w", SaName, err)
			}
		}
		if r, ok := rec.Valid(bid, saRecordKey(v.GetSadId())); !ok || r != saRecordValue(v.GetSpi(), v.GetProtocol()) {
			continue
		}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: SaMeta{SadID: v.GetSadId(), StatIndex: sa.statIndex}})
	}
	return sortKVs(out), nil
}

type dumpedSa struct {
	value     *vpnpb.IpsecSa
	statIndex uint32
}

// dump returns the decoded SA with id (none when absent).
func (d *Sa) dump(ctx context.Context, id uint32) ([]*vpnpb.IpsecSa, error) {
	all, err := d.dumpDetails(ctx, id)
	if err != nil {
		return nil, err
	}
	var out []*vpnpb.IpsecSa
	for _, sa := range all {
		if sa.value.GetSadId() == id {
			out = append(out, sa.value)
		}
	}
	return out, nil
}

// dumpDetails runs ipsec_sa_v5_dump (sa_id ~0 = all). Key material is hashed into references and
// zeroed while decoding.
func (d *Sa) dumpDetails(ctx context.Context, id uint32) ([]dumpedSa, error) {
	return dumpSAs(ctx, d.cfg, id)
}

func dumpSAs(ctx context.Context, cfg Config, id uint32) ([]dumpedSa, error) {
	if cfg.Keys == nil {
		return nil, fmt.Errorf("%s: %w", SaName, vpn.ErrNoKeyer)
	}
	stream, err := ipsec.NewServiceClient(cfg.Client).IpsecSaV5Dump(ctx, &ipsec.IpsecSaV5Dump{SaID: id})
	if err != nil {
		return nil, fmt.Errorf("ipsec_sa_v5_dump: %w", err)
	}
	var out []dumpedSa
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("ipsec_sa_v5_dump: %w", err)
		}
		out = append(out, dumpedSa{value: decodeSa(cfg.Keys, &det.Entry), statIndex: det.StatIndex}) // hashes and zeroes the key material
	}
}

// encodeSa validates o and builds the binapi entry, resolving the key references. The caller
// zeroes the key buffers.
func (d *Sa) encodeSa(ctx context.Context, o *vpnpb.IpsecSa) (ipsec_types.IpsecSadEntryV4, error) {
	var e ipsec_types.IpsecSadEntryV4
	proto, err := protocols.value("protocol", o.GetProtocol())
	if err != nil {
		return e, err
	}
	crypto, err := cryptoAlgs.value("crypto algorithm", o.GetCryptoAlg())
	if err != nil {
		return e, err
	}
	integ, err := integAlgs.value("integrity algorithm", o.GetIntegAlg())
	if err != nil {
		return e, err
	}
	if (crypto == ipsec_types.IPSEC_API_CRYPTO_ALG_NONE) != (o.GetCryptoKey() == "") {
		return e, fmt.Errorf("ipsec: sa %d: crypto_key must be set exactly when crypto_alg is not none", o.GetSadId())
	}
	if (integ == ipsec_types.IPSEC_API_INTEG_ALG_NONE) != (o.GetIntegKey() == "") {
		return e, fmt.Errorf("ipsec: sa %d: integ_key must be set exactly when integ_alg is not none", o.GetSadId())
	}
	for _, ref := range []string{o.GetCryptoKey(), o.GetIntegKey()} {
		if ref == "" {
			continue
		}
		if err := vpn.CheckRef(ref); err != nil {
			return e, fmt.Errorf("ipsec: sa %d: key: %w", o.GetSadId(), err) // never echoes the value (review M1)
		}
	}
	if o.GetUdpEncap() {
		if o.GetUdpSrcPort() == 0 || o.GetUdpDstPort() == 0 || o.GetUdpSrcPort() > 65535 || o.GetUdpDstPort() > 65535 {
			return e, fmt.Errorf("ipsec: sa %d: udp_encap needs explicit udp_src_port/udp_dst_port (1–65535)", o.GetSadId())
		}
	} else if o.GetUdpSrcPort() != 0 || o.GetUdpDstPort() != 0 {
		return e, fmt.Errorf("ipsec: sa %d: udp ports need udp_encap", o.GetSadId())
	}
	w := o.GetAntiReplayWindowSize()
	if o.GetUseAntiReplay() {
		if w < defaultReplayWindow || w&(w-1) != 0 {
			return e, fmt.Errorf("ipsec: sa %d: anti_replay_window_size must be a power of two >= %d", o.GetSadId(), defaultReplayWindow)
		}
	} else if w != 0 {
		return e, fmt.Errorf("ipsec: sa %d: anti_replay_window_size needs use_anti_replay", o.GetSadId())
	}
	var flags ipsec_types.IpsecSadFlags
	set := func(on bool, f ipsec_types.IpsecSadFlags) {
		if on {
			flags |= f
		}
	}
	set(o.GetUseEsn(), ipsec_types.IPSEC_API_SAD_FLAG_USE_ESN)
	set(o.GetUseAntiReplay(), ipsec_types.IPSEC_API_SAD_FLAG_USE_ANTI_REPLAY)
	set(o.GetUdpEncap(), ipsec_types.IPSEC_API_SAD_FLAG_UDP_ENCAP)
	set(o.GetInbound(), ipsec_types.IPSEC_API_SAD_FLAG_IS_INBOUND)
	set(o.GetAsync(), ipsec_types.IPSEC_API_SAD_FLAG_ASYNC)
	if t := o.GetTunnel(); t != nil {
		tun, v6, err := encodeTunnel(t)
		if err != nil {
			return e, fmt.Errorf("ipsec: sa %d: %w", o.GetSadId(), err)
		}
		flags |= ipsec_types.IPSEC_API_SAD_FLAG_IS_TUNNEL
		set(v6, ipsec_types.IPSEC_API_SAD_FLAG_IS_TUNNEL_V6)
		e.Tunnel = tun
	}
	cryptoKey, err := d.key(ctx, o.GetCryptoKey())
	if err != nil {
		return e, fmt.Errorf("ipsec: sa %d crypto_key: %w", o.GetSadId(), err)
	}
	integKey, err := d.key(ctx, o.GetIntegKey())
	if err != nil {
		vpn.Zero(cryptoKey.Data)
		return e, fmt.Errorf("ipsec: sa %d integ_key: %w", o.GetSadId(), err)
	}
	e.SadID, e.Spi, e.Protocol = o.GetSadId(), o.GetSpi(), proto
	e.CryptoAlgorithm, e.CryptoKey = crypto, cryptoKey
	e.IntegrityAlgorithm, e.IntegrityKey = integ, integKey
	e.Flags, e.Salt = flags, o.GetSalt()
	e.UDPSrcPort, e.UDPDstPort = uint16(o.GetUdpSrcPort()), uint16(o.GetUdpDstPort()) //nolint:gosec // checked
	e.AntiReplayWindowSize = w
	return e, nil
}

// key resolves a reference into a binapi Key (≤ 128 bytes).
func (d *Sa) key(ctx context.Context, ref string) (ipsec_types.Key, error) {
	mat, err := vpn.Resolve(ctx, d.cfg.Secrets, d.cfg.Keys, ref)
	if err != nil {
		return ipsec_types.Key{}, err
	}
	if len(mat) > 128 {
		vpn.Zero(mat)
		return ipsec_types.Key{}, errors.New("key longer than 128 bytes")
	}
	return ipsec_types.Key{Length: uint8(len(mat)), Data: mat}, nil //nolint:gosec // checked
}

// encodeTunnel builds the tunnel part of a tunnel-mode SA; v6 reports an IPv6 outer header.
func encodeTunnel(t *vpnpb.IpsecTunnel) (tunnel_types.Tunnel, bool, error) {
	var tun tunnel_types.Tunnel
	src, err := vpn.ParseAddress(t.GetSrc())
	if err != nil {
		return tun, false, err
	}
	dst, err := vpn.ParseAddress(t.GetDst())
	if err != nil {
		return tun, false, err
	}
	if src.Af != dst.Af {
		return tun, false, errors.New("tunnel src and dst address families differ")
	}
	if t.GetDscp() > 63 {
		return tun, false, fmt.Errorf("tunnel dscp %d > 63", t.GetDscp())
	}
	if t.GetHopLimit() > 255 {
		return tun, false, fmt.Errorf("tunnel hop_limit %d > 255", t.GetHopLimit())
	}
	flags, err := encodeEncapFlags(t.GetEncapDecapFlags())
	if err != nil {
		return tun, false, err
	}
	tun = tunnel_types.Tunnel{
		Src: src, Dst: dst, TableID: t.GetTableId(), EncapDecapFlags: flags,
		Mode: tunnel_types.TUNNEL_API_MODE_P2P, Dscp: ip_types.IPDscp(t.GetDscp()), //nolint:gosec // checked
		HopLimit:  uint8(t.GetHopLimit()), //nolint:gosec // checked
		SwIfIndex: interface_types.InterfaceIndex(noInterface),
	}
	return tun, dst.Af == ip_types.ADDRESS_IP6, nil
}

// decodeSa turns a dumped entry into the desired shape. Key material is hashed into references
// and zeroed in place before this function returns.
func decodeSa(k *vpn.Keyer, e *ipsec_types.IpsecSadEntryV4) *vpnpb.IpsecSa {
	defer vpn.Zero(e.CryptoKey.Data)
	defer vpn.Zero(e.IntegrityKey.Data)
	has := func(f ipsec_types.IpsecSadFlags) bool { return e.Flags&f != 0 }
	v := &vpnpb.IpsecSa{
		SadId: e.SadID, Spi: e.Spi, Protocol: protocols.name(e.Protocol),
		CryptoAlg: cryptoAlgs.name(e.CryptoAlgorithm), IntegAlg: integAlgs.name(e.IntegrityAlgorithm),
		UseEsn:        has(ipsec_types.IPSEC_API_SAD_FLAG_USE_ESN),
		UseAntiReplay: has(ipsec_types.IPSEC_API_SAD_FLAG_USE_ANTI_REPLAY),
		UdpEncap:      has(ipsec_types.IPSEC_API_SAD_FLAG_UDP_ENCAP),
		Inbound:       has(ipsec_types.IPSEC_API_SAD_FLAG_IS_INBOUND),
		Async:         has(ipsec_types.IPSEC_API_SAD_FLAG_ASYNC),
		// ipsec_sa_v5_details converts salt by hand and the generated endian pass swaps it again
		// (verified on VPP 26.06: 0x1234 is reported as 0x34120000), so it arrives byte-swapped.
		Salt: bits.ReverseBytes32(e.Salt),
	}
	if e.CryptoAlgorithm != ipsec_types.IPSEC_API_CRYPTO_ALG_NONE {
		v.CryptoKey = keyRef(k, e.CryptoKey)
	}
	if e.IntegrityAlgorithm != ipsec_types.IPSEC_API_INTEG_ALG_NONE {
		v.IntegKey = keyRef(k, e.IntegrityKey)
	}
	if v.UdpEncap {
		v.UdpSrcPort, v.UdpDstPort = uint32(e.UDPSrcPort), uint32(e.UDPDstPort)
	}
	if v.UseAntiReplay {
		v.AntiReplayWindowSize = e.AntiReplayWindowSize
	}
	if has(ipsec_types.IPSEC_API_SAD_FLAG_IS_TUNNEL) || has(ipsec_types.IPSEC_API_SAD_FLAG_IS_TUNNEL_V6) {
		v.Tunnel = &vpnpb.IpsecTunnel{
			Src: vpn.AddressString(e.Tunnel.Src), Dst: vpn.AddressString(e.Tunnel.Dst),
			TableId: e.Tunnel.TableID, Dscp: uint32(e.Tunnel.Dscp), HopLimit: uint32(e.Tunnel.HopLimit),
			EncapDecapFlags: encapFlagNames(e.Tunnel.EncapDecapFlags),
		}
	}
	return v
}

func keyRef(keys *vpn.Keyer, k ipsec_types.Key) string {
	n := int(k.Length)
	if n > len(k.Data) {
		n = len(k.Data)
	}
	return keys.Ref(k.Data[:n])
}
