package wireguard

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/wireguard"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// Peer manages WireGuard peers (wireguard_peer_add_v2 / wireguard_peer_remove; dump
// wireguard_peers_v2_dump). VPP has no peer update message, so every change is ErrRecreate.
// The preshared key is a "sha256:<hex>" reference; wireguard_peers_v2_dump returns the key in
// clear, Retrieve hashes it into the reference and zeroes the buffer. The peer's status flags
// (dead / established) are state, not configuration: they are reported through Events, never in
// the Value.
//
// Canonical form (so proto.Equal is a correct diff): allowed_ips masked and sorted as strings
// without duplicates (Create refuses anything else), endpoint canonical text ("" = none).
type Peer struct {
	cfg Config

	mu     sync.Mutex
	byIdx  map[uint32]peerRef // VPP peer index → key, learnt by Create and Retrieve (events)
	events bool               // an Events subscription is active: register new peers for events
	pid    uint32
}

type peerRef struct {
	key       scheduler.Key
	iface     string
	publicKey string
}

// PeerMeta is the runtime handle of a peer.
type PeerMeta struct {
	PeerIndex uint32
	SwIfIndex uint32
}

// NewPeer returns the descriptor.
func NewPeer(cfg Config) *Peer { return &Peer{cfg: cfg, byIdx: map[uint32]peerRef{}} }

// Name implements scheduler.Descriptor.
func (*Peer) Name() string { return PeerName }

// KeyOf implements scheduler.Descriptor: wireguard.peer/<interface>/<public_key> (std base64, may
// contain "/"; Key.ID() is everything after the descriptor name).
func (*Peer) KeyOf(obj proto.Message) scheduler.Key {
	o, _ := obj.(*vpnpb.WireguardPeer)
	return scheduler.Join(PeerName, o.GetInterface(), o.GetPublicKey())
}

// Dependencies implements scheduler.Descriptor: the WireGuard interface and the FIB table
// (Optional) when table_id != 0.
func (*Peer) Dependencies(obj proto.Message) []scheduler.Dependency {
	o, _ := obj.(*vpnpb.WireguardPeer)
	deps := []scheduler.Dependency{{Key: vpn.InterfaceDependency(o.GetInterface())}}
	if o.GetTableId() != 0 {
		deps = append(deps, scheduler.Dependency{Key: vpn.VRFKey(o.GetTableId()), Optional: true})
	}
	return deps
}

// Create implements scheduler.Descriptor.
func (d *Peer) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.WireguardPeer)
	if !ok {
		return nil, typeErr(PeerName, obj)
	}
	peer, err := encodePeer(o)
	if err != nil {
		return nil, err
	}
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client)
	if err != nil {
		return nil, err
	}
	idx, err := tbl.Index(o.GetInterface())
	if err != nil {
		return nil, err
	}
	peer.SwIfIndex = idx
	if ref := o.GetPresharedKey(); ref != "" {
		mat, err := vpn.Resolve(ctx, d.cfg.Secrets, ref)
		if err != nil {
			return nil, fmt.Errorf("wireguard: peer %s preshared_key: %w", d.KeyOf(o), err)
		}
		if len(mat) != vpn.X25519KeyLen {
			vpn.Zero(mat)
			return nil, fmt.Errorf("wireguard: peer %s preshared_key must be %d bytes", d.KeyOf(o), vpn.X25519KeyLen)
		}
		peer.PresharedKey, peer.PresharedKeySet = mat, true
		defer vpn.Zero(mat)
	}
	svc := wireguard.NewServiceClient(d.cfg.Client)
	rep, err := svc.WireguardPeerAddV2(ctx, &wireguard.WireguardPeerAddV2{Peer: peer})
	if err != nil {
		return nil, fmt.Errorf("wireguard_peer_add_v2 (%s): %w", d.KeyOf(o), err)
	}
	d.mu.Lock()
	d.byIdx[rep.PeerIndex] = peerRef{key: d.KeyOf(o), iface: o.GetInterface(), publicKey: o.GetPublicKey()}
	events, pid := d.events, d.pid
	d.mu.Unlock()
	if events {
		// want_wireguard_peer_events registers the client on peers that exist at call time only
		if _, err := svc.WantWireguardPeerEvents(ctx, &wireguard.WantWireguardPeerEvents{
			SwIfIndex: interface_types.InterfaceIndex(noInterface), PeerIndex: rep.PeerIndex, EnableDisable: 1, PID: pid,
		}); err != nil {
			return PeerMeta{PeerIndex: rep.PeerIndex, SwIfIndex: uint32(idx)}, fmt.Errorf("want_wireguard_peer_events (%s): %w", d.KeyOf(o), err)
		}
	}
	return PeerMeta{PeerIndex: rep.PeerIndex, SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor: VPP has no peer update.
func (*Peer) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *Peer) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(PeerMeta)
	if !ok {
		return metaErr(PeerName, meta)
	}
	if _, err := wireguard.NewServiceClient(d.cfg.Client).WireguardPeerRemove(ctx, &wireguard.WireguardPeerRemove{PeerIndex: m.PeerIndex}); err != nil {
		return fmt.Errorf("wireguard_peer_remove (%s): %w", d.KeyOf(obj), err)
	}
	d.mu.Lock()
	delete(d.byIdx, m.PeerIndex)
	d.mu.Unlock()
	return nil
}

// Retrieve implements scheduler.Descriptor: peers of WireGuard interfaces tagged by this owner.
func (d *Peer) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client)
	if err != nil {
		return nil, err
	}
	stream, err := wireguard.NewServiceClient(d.cfg.Client).WireguardPeersV2Dump(ctx, &wireguard.WireguardPeersV2Dump{PeerIndex: noInterface})
	if err != nil {
		return nil, fmt.Errorf("wireguard_peers_v2_dump: %w", err)
	}
	var out []scheduler.KV
	learnt := map[uint32]peerRef{}
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("wireguard_peers_v2_dump: %w", err)
		}
		p := det.Peer
		sw := uint32(p.SwIfIndex)
		if _, owned := tbl.Owned(sw, d.cfg.Owner); !owned {
			vpn.Zero(p.PresharedKey) // another owner's key: never keep it
			continue
		}
		v := decodePeer(&p, tbl.Name(sw)) // hashes and zeroes the preshared key
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: PeerMeta{PeerIndex: p.PeerIndex, SwIfIndex: sw}})
		learnt[p.PeerIndex] = peerRef{key: d.KeyOf(v), iface: v.GetInterface(), publicKey: v.GetPublicKey()}
	}
	d.mu.Lock()
	for i, r := range learnt {
		d.byIdx[i] = r
	}
	d.mu.Unlock()
	sortKVs(out)
	return out, nil
}

// encodePeer validates o and builds the request without interface and preshared key.
func encodePeer(o *vpnpb.WireguardPeer) (wireguard.WireguardPeerV2, error) {
	var p wireguard.WireguardPeerV2
	pub, err := base64.StdEncoding.DecodeString(o.GetPublicKey())
	if err != nil || len(pub) != vpn.X25519KeyLen {
		return p, fmt.Errorf("wireguard: peer public_key %q must be the std base64 of %d bytes", o.GetPublicKey(), vpn.X25519KeyLen)
	}
	if o.GetPort() > 65535 || o.GetPersistentKeepalive() > 65535 {
		return p, errors.New("wireguard: peer port and persistent_keepalive must be ≤ 65535")
	}
	ep := ip_types.Address{Af: ip_types.ADDRESS_IP4}
	if o.GetEndpoint() != "" {
		if ep, err = vpn.ParseAddress(o.GetEndpoint()); err != nil {
			return p, err
		}
	}
	ips := o.GetAllowedIps()
	if len(ips) == 0 || len(ips) > 255 {
		return p, errors.New("wireguard: peer needs 1–255 allowed_ips")
	}
	prefixes := make([]ip_types.Prefix, 0, len(ips))
	for i, s := range ips {
		pfx, err := vpn.ParsePrefix(s)
		if err != nil {
			return p, err
		}
		if vpn.PrefixString(pfx) != s || (i > 0 && ips[i-1] >= s) {
			return p, fmt.Errorf("wireguard: peer allowed_ips must be canonical, sorted and unique (got %q)", ips)
		}
		prefixes = append(prefixes, pfx)
	}
	return wireguard.WireguardPeerV2{
		PublicKey: pub, Port: uint16(o.GetPort()), PersistentKeepalive: uint16(o.GetPersistentKeepalive()), //nolint:gosec // checked
		TableID: o.GetTableId(), Endpoint: ep, NAllowedIps: uint8(len(prefixes)), AllowedIps: prefixes, //nolint:gosec // checked
	}, nil
}

// decodePeer turns a dumped peer into the desired shape; the preshared key buffer is zeroed.
func decodePeer(p *wireguard.WireguardPeerV2, iface string) *vpnpb.WireguardPeer {
	defer vpn.Zero(p.PresharedKey)
	v := &vpnpb.WireguardPeer{
		Interface: iface, PublicKey: base64.StdEncoding.EncodeToString(p.PublicKey),
		Port: uint32(p.Port), PersistentKeepalive: uint32(p.PersistentKeepalive), TableId: p.TableID,
	}
	if !vpn.IsUnspecified(p.Endpoint) {
		v.Endpoint = vpn.AddressString(p.Endpoint)
	}
	for _, a := range p.AllowedIps {
		v.AllowedIps = append(v.AllowedIps, vpn.PrefixString(a))
	}
	slices.Sort(v.AllowedIps)
	v.AllowedIps = slices.Compact(v.AllowedIps)
	if p.PresharedKeySet {
		n := min(len(p.PresharedKey), vpn.X25519KeyLen)
		v.PresharedKey = vpn.Ref(p.PresharedKey[:n])
	}
	return v
}
