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
// The preshared key is an "hmac:<hex>" reference (keyed, D-096); wireguard_peers_v2_dump returns
// the key in clear, Retrieve fingerprints it into the reference and zeroes the buffer. The peer's status flags
// (dead / established) are state, not configuration: they are reported through Events, never in
// the Value.
//
// Canonical form (so proto.Equal is a correct diff): allowed_ips masked and sorted as strings
// without duplicates (Create refuses anything else), endpoint canonical text ("" = none).
//
// Peers are added only to our own (tagged) WireGuard interfaces, named by their logical name
// (D-069). VPP identifies a peer by its index, which is reused after a peer delete or a VPP
// restart: Delete and the event lookup re-read the peer at that index and check its public key and
// interface before acting on it (D-071); a peer that is gone needs nothing (D-074).
type Peer struct {
	cfg Config

	mu     sync.Mutex
	events bool // an Events subscription is active: register new peers for events
	pid    uint32
}

type peerRef struct {
	key       scheduler.Key
	iface     string
	publicKey string
	owned     bool
}

// PeerMeta is the runtime handle of a peer.
type PeerMeta struct {
	PeerIndex uint32
	SwIfIndex uint32
}

// NewPeer returns the descriptor.
func NewPeer(cfg Config) *Peer { return &Peer{cfg: cfg} }

// Name implements scheduler.Descriptor.
func (*Peer) Name() string { return PeerName }

// RecordsNoOwnership declares the TD-11b ownership protocol: a peer is ours when it sits on one of
// our tagged WireGuard interfaces (the tag VPP carries); nothing is recorded in a store.
func (*Peer) RecordsNoOwnership() {}

// KeyOf implements scheduler.Descriptor: wireguard.peer/<interface>/<public_key> (std base64, may
// contain "/"; Key.ID() is everything after the descriptor name).
func (*Peer) KeyOf(obj proto.Message) scheduler.Key {
	o, _ := obj.(*vpnpb.WireguardPeer)
	return scheduler.Join(PeerName, o.GetInterface(), o.GetPublicKey())
}

// Dependencies implements scheduler.Descriptor: the WireGuard interface by its alias
// interface/wg<N> (D-065; wireguard.interface provides it) and the FIB table (Optional) when
// table_id != 0.
func (*Peer) Dependencies(obj proto.Message) []scheduler.Dependency {
	o, _ := obj.(*vpnpb.WireguardPeer)
	deps := []scheduler.Dependency{{Key: vpn.InterfaceKey(o.GetInterface())}}
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
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client, d.cfg.Owner)
	if err != nil {
		return nil, err
	}
	idx, err := tbl.ResolveOwn(o.GetInterface())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", PeerName, err)
	}
	peer.SwIfIndex = idx
	if ref := o.GetPresharedKey(); ref != "" {
		if d051, ok := unavailable(ref); ok {
			return nil, fmt.Errorf("wireguard: peer %s preshared key %s: %w", d.KeyOf(o), d051, ErrSecretUnavailable)
		}
		if err := vpn.CheckRef(ref); err != nil {
			return nil, fmt.Errorf("wireguard: peer preshared_key: %w", err) // never echoes the value (review M1)
		}
		mat, err := vpn.Resolve(ctx, d.cfg.Secrets, d.cfg.Keys, ref)
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
	events, pid := d.events, d.pid
	d.mu.Unlock()
	if events {
		// want_wireguard_peer_events registers the client on peers that exist at call time only
		if _, err := svc.WantWireguardPeerEvents(ctx, &wireguard.WantWireguardPeerEvents{
			SwIfIndex: interface_types.InterfaceIndex(noInterface), PeerIndex: rep.PeerIndex, EnableDisable: 1, PID: pid,
		}); err != nil {
			// the peer exists in VPP: a partial Create (TD-11b, D-133) — the reconciler journals it with this
			// Meta and the rollback deletes it; a bare error would leave it in VPP, unjournaled
			return PeerMeta{PeerIndex: rep.PeerIndex, SwIfIndex: uint32(idx)}, scheduler.PartialCreate(fmt.Errorf("want_wireguard_peer_events (%s): %w", d.KeyOf(o), err))
		}
	}
	return PeerMeta{PeerIndex: rep.PeerIndex, SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor: VPP has no peer update.
func (*Peer) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor. Right before the delete it re-reads the peer at the
// index and checks that it still is this peer (public key) on our interface (D-071).
func (d *Peer) Delete(ctx context.Context, obj proto.Message, meta any) error {
	o, ok := obj.(*vpnpb.WireguardPeer)
	if !ok {
		return typeErr(PeerName, obj)
	}
	m, ok := meta.(PeerMeta)
	if !ok {
		return metaErr(PeerName, meta)
	}
	ref, found, err := d.peerAt(ctx, m.PeerIndex)
	if err != nil {
		return err
	}
	if !found || ref.publicKey != o.GetPublicKey() || ref.iface != o.GetInterface() {
		return nil // gone (the index may meanwhile hold another peer, which is not touched)
	}
	if !ref.owned {
		return fmt.Errorf("%s: %s: %w", PeerName, d.KeyOf(obj), vpn.ErrNotOurs)
	}
	if _, err := wireguard.NewServiceClient(d.cfg.Client).WireguardPeerRemove(ctx, &wireguard.WireguardPeerRemove{PeerIndex: m.PeerIndex}); err != nil {
		return fmt.Errorf("wireguard_peer_remove (%s): %w", d.KeyOf(obj), err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: peers of WireGuard interfaces tagged by this owner.
func (d *Peer) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if d.cfg.Keys == nil {
		return nil, fmt.Errorf("%s: %w", PeerName, vpn.ErrNoKeyer)
	}
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client, d.cfg.Owner)
	if err != nil {
		return nil, err
	}
	stream, err := wireguard.NewServiceClient(d.cfg.Client).WireguardPeersV2Dump(ctx, &wireguard.WireguardPeersV2Dump{PeerIndex: noInterface})
	if err != nil {
		return nil, fmt.Errorf("wireguard_peers_v2_dump: %w", err)
	}
	var out []scheduler.KV
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
		name, owned := tbl.Owned(sw)
		if !owned {
			vpn.Zero(p.PresharedKey) // another owner's key: never keep it
			continue
		}
		v := decodePeer(d.cfg.Keys, &p, name) // hashes and zeroes the preshared key
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: PeerMeta{PeerIndex: p.PeerIndex, SwIfIndex: sw}})
	}
	return sortKVs(out), nil
}

// peerAt reads the peer at index idx (wireguard_peers_dump, which carries no preshared key) and
// resolves its interface: found=false when there is none; owned when the interface is ours.
func (d *Peer) peerAt(ctx context.Context, idx uint32) (peerRef, bool, error) {
	stream, err := wireguard.NewServiceClient(d.cfg.Client).WireguardPeersDump(ctx, &wireguard.WireguardPeersDump{PeerIndex: idx})
	if err != nil {
		return peerRef{}, false, fmt.Errorf("wireguard_peers_dump: %w", err)
	}
	var found *wireguard.WireguardPeer
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return peerRef{}, false, fmt.Errorf("wireguard_peers_dump: %w", err)
		}
		if det.Peer.PeerIndex == idx {
			p := det.Peer
			found = &p
		}
	}
	if found == nil {
		return peerRef{}, false, nil
	}
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client, d.cfg.Owner)
	if err != nil {
		return peerRef{}, false, err
	}
	name, owned := tbl.Owned(uint32(found.SwIfIndex))
	pub := base64.StdEncoding.EncodeToString(found.PublicKey)
	return peerRef{key: scheduler.Join(PeerName, name, pub), iface: name, publicKey: pub, owned: owned}, true, nil
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
func decodePeer(k *vpn.Keyer, p *wireguard.WireguardPeerV2, iface string) *vpnpb.WireguardPeer {
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
		v.PresharedKey = k.Ref(p.PresharedKey[:n])
	}
	return v
}
