package coretest

// F-wireguard extension of the model (A6: own file, installed through the extensions seam): the
// WireGuard plugin as far as DF-5's wireguard descriptors and DumpState use it — interfaces (a
// modelled Iface named wg<instance>, public key derived from the private key), peers with VPP-wide
// unique public keys, both peer dumps (the v2 dump carries the preshared key in clear, like VPP), and
// the event registration. Agent tests of other domains retrieve the (empty) wireguard family unchanged.

import (
	"bytes"
	"crypto/ecdh"
	"fmt"
	"sort"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/wireguard"
)

// wgModel is the WireGuard state of one VPP model.
type wgModel struct {
	itfs     map[uint32]wireguard.WireguardInterface // sw_if_index → interface (private key kept, like VPP)
	peers    map[uint32]wireguard.WireguardPeerV2
	nextPeer uint32
	flags    map[uint32]wireguard.WireguardPeerFlags
}

var wgModels = map[*VPP]*wgModel{}

func init() { extensions = append(extensions, installWireguard) }

// SetWireguardPeerFlags sets the status flags VPP reports for a peer (tests of the state RPC).
func (v *VPP) SetWireguardPeerFlags(peerIndex uint32, f wireguard.WireguardPeerFlags) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if m := wgModels[v]; m != nil {
		m.flags[peerIndex] = f
	}
}

// WireguardPeerCount is the number of peers in the model (all owners).
func (v *VPP) WireguardPeerCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(wgModels[v].peers)
}

func installWireguard(v *VPP) {
	m := &wgModel{itfs: map[uint32]wireguard.WireguardInterface{}, peers: map[uint32]wireguard.WireguardPeerV2{}, nextPeer: 1, flags: map[uint32]wireguard.WireguardPeerFlags{}}
	v.mu.Lock()
	wgModels[v] = m
	v.mu.Unlock()
	v.On("wireguard_interface_create", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*wireguard.WireguardInterfaceCreate)
		v.mu.Lock()
		defer v.mu.Unlock()
		for _, w := range m.itfs {
			if w.UserInstance == r.Interface.UserInstance {
				return reply(&wireguard.WireguardInterfaceCreateReply{Retval: RetvalInstanceInUse})
			}
		}
		priv, err := ecdh.X25519().NewPrivateKey(r.Interface.PrivateKey)
		if err != nil || r.GenerateKey {
			return reply(&wireguard.WireguardInterfaceCreateReply{Retval: RetvalNoSuchEntry})
		}
		idx := v.next
		v.next++
		v.Ifaces[idx] = &Iface{Index: idx, Name: fmt.Sprintf("wg%d", r.Interface.UserInstance), DevType: "Wireguard Tunnel", Addrs: map[string]bool{},
			LinkMtu: 9000, Mtu: [4]uint32{9000}, RxMode: interface_types.RX_MODE_API_POLLING}
		w := r.Interface
		w.SwIfIndex = interface_types.InterfaceIndex(idx)
		w.PrivateKey = append([]byte(nil), r.Interface.PrivateKey...)
		w.PublicKey = priv.PublicKey().Bytes()
		m.itfs[idx] = w
		return reply(&wireguard.WireguardInterfaceCreateReply{SwIfIndex: w.SwIfIndex})
	})
	v.On("wireguard_interface_delete", func(msg api.Message) ([]api.Message, error) {
		sw := uint32(msg.(*wireguard.WireguardInterfaceDelete).SwIfIndex)
		v.mu.Lock()
		defer v.mu.Unlock()
		if _, ok := m.itfs[sw]; !ok {
			return reply(&wireguard.WireguardInterfaceDeleteReply{Retval: RetvalInvalidSwIfIndex})
		}
		delete(m.itfs, sw)
		for i, p := range m.peers {
			if uint32(p.SwIfIndex) == sw {
				delete(m.peers, i)
			}
		}
		if i, ok := v.Ifaces[sw]; ok {
			v.dropInterfaceLocked(i)
		}
		return reply(&wireguard.WireguardInterfaceDeleteReply{})
	})
	v.On("wireguard_interface_dump", func(msg api.Message) ([]api.Message, error) {
		show := msg.(*wireguard.WireguardInterfaceDump).ShowPrivateKey
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedU32(m.itfs) {
			d := m.itfs[idx]
			d.PrivateKey = make([]byte, 32)
			if show {
				copy(d.PrivateKey, m.itfs[idx].PrivateKey)
			}
			d.PublicKey = append([]byte(nil), d.PublicKey...)
			out = append(out, &wireguard.WireguardInterfaceDetails{Interface: d})
		}
		return out, nil
	})
	v.On("wireguard_peer_add_v2", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*wireguard.WireguardPeerAddV2)
		v.mu.Lock()
		defer v.mu.Unlock()
		if _, ok := m.itfs[uint32(r.Peer.SwIfIndex)]; !ok || r.Peer.NAllowedIps == 0 {
			return reply(&wireguard.WireguardPeerAddV2Reply{Retval: RetvalInvalidSwIfIndex})
		}
		for _, p := range m.peers {
			if bytes.Equal(p.PublicKey, r.Peer.PublicKey) {
				return reply(&wireguard.WireguardPeerAddV2Reply{Retval: RetvalAlreadyExists})
			}
		}
		p := r.Peer
		p.PresharedKey = make([]byte, 32)
		if r.Peer.PresharedKeySet {
			copy(p.PresharedKey, r.Peer.PresharedKey)
		}
		p.PublicKey = append([]byte(nil), r.Peer.PublicKey...)
		p.AllowedIps = append(p.AllowedIps[:0:0], r.Peer.AllowedIps...)
		p.PeerIndex = m.nextPeer
		m.nextPeer++
		m.peers[p.PeerIndex] = p
		return reply(&wireguard.WireguardPeerAddV2Reply{PeerIndex: p.PeerIndex})
	})
	v.On("wireguard_peer_remove", func(msg api.Message) ([]api.Message, error) {
		i := msg.(*wireguard.WireguardPeerRemove).PeerIndex
		v.mu.Lock()
		defer v.mu.Unlock()
		if _, ok := m.peers[i]; !ok {
			return reply(&wireguard.WireguardPeerRemoveReply{Retval: RetvalNoSuchEntry})
		}
		delete(m.peers, i)
		return reply(&wireguard.WireguardPeerRemoveReply{})
	})
	v.On("wireguard_peers_v2_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedU32(m.peers) {
			p := m.peers[idx]
			p.PresharedKey = append([]byte(nil), p.PresharedKey...)
			p.PresharedKeySet = !bytes.Equal(p.PresharedKey, make([]byte, 32))
			p.Flags = m.flags[idx]
			out = append(out, &wireguard.WireguardPeersV2Details{Peer: p})
		}
		return out, nil
	})
	v.On("wireguard_peers_dump", func(msg api.Message) ([]api.Message, error) {
		want := msg.(*wireguard.WireguardPeersDump).PeerIndex
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedU32(m.peers) {
			if want != ^uint32(0) && want != idx {
				continue
			}
			p := m.peers[idx]
			out = append(out, &wireguard.WireguardPeersDetails{Peer: wireguard.WireguardPeer{
				PeerIndex: p.PeerIndex, PublicKey: p.PublicKey, Port: p.Port, PersistentKeepalive: p.PersistentKeepalive,
				TableID: p.TableID, Endpoint: p.Endpoint, SwIfIndex: p.SwIfIndex, Flags: m.flags[idx],
				NAllowedIps: p.NAllowedIps, AllowedIps: p.AllowedIps,
			}})
		}
		return out, nil
	})
	v.On("want_wireguard_peer_events", func(api.Message) ([]api.Message, error) {
		return reply(&wireguard.WantWireguardPeerEventsReply{})
	})
	v.On("wg_set_async_mode", func(api.Message) ([]api.Message, error) {
		return reply(&wireguard.WgSetAsyncModeReply{})
	})
}

func sortedU32[V any](m map[uint32]V) []uint32 {
	out := make([]uint32, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
