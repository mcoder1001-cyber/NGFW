package wireguard_test

// A stateful fake VPP for the wireguard descriptors. It models what the descriptors rely on: the
// interface dump reports the public key (and the private key only with show_private_key), peer
// public keys are unique VPP-wide, peers_v2_dump returns the preshared key in clear, and
// want_wireguard_peer_events registers the client only on peers that exist at that moment.

import (
	"bytes"
	"crypto/ecdh"
	"fmt"
	"maps"
	"slices"
	"sort"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/wireguard"
	"ngfw/agent/internal/vpp/fake"
)

const (
	rvInvalid = -73 // VNET_API_ERROR_INVALID_VALUE
	rvExists  = -81 // any non-zero works for the descriptor
)

type fakeVPP struct {
	*fake.Client
	nextIf     uint32
	ifaces     map[uint32]*interfaces.SwInterfaceDetails
	wgs        map[uint32]wireguard.WireguardInterface // sw_if_index → interface incl. private key
	nextPeer   uint32
	peers      map[uint32]wireguard.WireguardPeerV2
	registered map[uint32]bool // peer index → event client registered
	async      []bool
}

func newFakeVPP() *fakeVPP {
	v := &fakeVPP{
		Client:     fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})),
		nextIf:     1,
		ifaces:     map[uint32]*interfaces.SwInterfaceDetails{0: {SwIfIndex: 0, InterfaceName: "local0"}},
		wgs:        map[uint32]wireguard.WireguardInterface{},
		nextPeer:   3,
		peers:      map[uint32]wireguard.WireguardPeerV2{},
		registered: map[uint32]bool{},
	}
	v.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(v.ifaces))
		for _, i := range slices.Sorted(maps.Keys(v.ifaces)) {
			out = append(out, v.ifaces[i])
		}
		return out, nil
	})
	v.On("sw_interface_tag_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*interfaces.SwInterfaceTagAddDel)
		i, ok := v.ifaces[uint32(r.SwIfIndex)]
		if !ok {
			return []api.Message{&interfaces.SwInterfaceTagAddDelReply{Retval: rvInvalid}}, nil
		}
		i.Tag = r.Tag
		return []api.Message{&interfaces.SwInterfaceTagAddDelReply{}}, nil
	})
	v.On("wireguard_interface_create", func(req api.Message) ([]api.Message, error) {
		r := req.(*wireguard.WireguardInterfaceCreate)
		for _, w := range v.wgs {
			if w.UserInstance == r.Interface.UserInstance {
				return []api.Message{&wireguard.WireguardInterfaceCreateReply{Retval: rvInvalid}}, nil
			}
		}
		priv, err := ecdh.X25519().NewPrivateKey(r.Interface.PrivateKey)
		if err != nil || r.GenerateKey {
			return []api.Message{&wireguard.WireguardInterfaceCreateReply{Retval: rvInvalid}}, nil
		}
		w := r.Interface
		w.PrivateKey = append([]byte(nil), r.Interface.PrivateKey...) // VPP's copy; the request is zeroed
		w.PublicKey = priv.PublicKey().Bytes()
		idx := v.addIface(fmt.Sprintf("wg%d", w.UserInstance), "")
		w.SwIfIndex = interface_types.InterfaceIndex(idx)
		v.wgs[idx] = w
		return []api.Message{&wireguard.WireguardInterfaceCreateReply{SwIfIndex: w.SwIfIndex}}, nil
	})
	v.On("wireguard_interface_delete", func(req api.Message) ([]api.Message, error) {
		sw := uint32(req.(*wireguard.WireguardInterfaceDelete).SwIfIndex)
		if _, ok := v.wgs[sw]; !ok {
			return []api.Message{&wireguard.WireguardInterfaceDeleteReply{Retval: rvInvalid}}, nil
		}
		delete(v.wgs, sw)
		delete(v.ifaces, sw)
		return []api.Message{&wireguard.WireguardInterfaceDeleteReply{}}, nil
	})
	v.On("wireguard_interface_dump", func(req api.Message) ([]api.Message, error) {
		show := req.(*wireguard.WireguardInterfaceDump).ShowPrivateKey
		var out []api.Message
		for _, w := range v.wgs {
			d := w
			d.PrivateKey = make([]byte, 32)
			if show {
				copy(d.PrivateKey, w.PrivateKey)
			}
			d.PublicKey = append([]byte(nil), w.PublicKey...)
			out = append(out, &wireguard.WireguardInterfaceDetails{Interface: d})
		}
		return out, nil
	})
	v.On("wireguard_peer_add_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*wireguard.WireguardPeerAddV2)
		if _, ok := v.wgs[uint32(r.Peer.SwIfIndex)]; !ok || r.Peer.NAllowedIps == 0 {
			return []api.Message{&wireguard.WireguardPeerAddV2Reply{Retval: rvInvalid}}, nil
		}
		for _, p := range v.peers {
			if bytes.Equal(p.PublicKey, r.Peer.PublicKey) {
				return []api.Message{&wireguard.WireguardPeerAddV2Reply{Retval: rvExists}}, nil
			}
		}
		p := r.Peer
		p.PresharedKey = make([]byte, 32)
		if r.Peer.PresharedKeySet {
			copy(p.PresharedKey, r.Peer.PresharedKey)
		}
		p.PublicKey = append([]byte(nil), r.Peer.PublicKey...)
		p.AllowedIps = append(p.AllowedIps[:0:0], r.Peer.AllowedIps...)
		// VPP keeps allowed ips in the order given; reverse to prove Retrieve sorts
		for i, j := 0, len(p.AllowedIps)-1; i < j; i, j = i+1, j-1 {
			p.AllowedIps[i], p.AllowedIps[j] = p.AllowedIps[j], p.AllowedIps[i]
		}
		p.PeerIndex = v.nextPeer
		v.nextPeer++
		v.peers[p.PeerIndex] = p
		return []api.Message{&wireguard.WireguardPeerAddV2Reply{PeerIndex: p.PeerIndex}}, nil
	})
	v.On("wireguard_peer_remove", func(req api.Message) ([]api.Message, error) {
		i := req.(*wireguard.WireguardPeerRemove).PeerIndex
		if _, ok := v.peers[i]; !ok {
			return []api.Message{&wireguard.WireguardPeerRemoveReply{Retval: rvInvalid}}, nil
		}
		delete(v.peers, i)
		delete(v.registered, i)
		return []api.Message{&wireguard.WireguardPeerRemoveReply{}}, nil
	})
	v.On("wireguard_peers_v2_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, idx := range v.peerIdx() {
			p := v.peers[idx]
			p.PresharedKey = append([]byte(nil), p.PresharedKey...) // in clear, like VPP
			p.PresharedKeySet = !bytes.Equal(p.PresharedKey, make([]byte, 32))
			out = append(out, &wireguard.WireguardPeersV2Details{Peer: p})
		}
		return out, nil
	})
	v.On("wireguard_peers_dump", func(req api.Message) ([]api.Message, error) {
		want := req.(*wireguard.WireguardPeersDump).PeerIndex
		var out []api.Message
		for _, idx := range v.peerIdx() {
			if want != ^uint32(0) && want != idx {
				continue
			}
			p := v.peers[idx]
			out = append(out, &wireguard.WireguardPeersDetails{Peer: wireguard.WireguardPeer{
				PeerIndex: p.PeerIndex, PublicKey: p.PublicKey, Port: p.Port, SwIfIndex: p.SwIfIndex,
				Endpoint: p.Endpoint, NAllowedIps: p.NAllowedIps, AllowedIps: p.AllowedIps,
			}})
		}
		return out, nil
	})
	v.On("want_wireguard_peer_events", func(req api.Message) ([]api.Message, error) {
		r := req.(*wireguard.WantWireguardPeerEvents)
		for idx := range v.peers {
			if r.PeerIndex == ^uint32(0) || r.PeerIndex == idx {
				v.registered[idx] = r.EnableDisable != 0
			}
		}
		return []api.Message{&wireguard.WantWireguardPeerEventsReply{}}, nil
	})
	v.On("wg_set_async_mode", func(req api.Message) ([]api.Message, error) {
		v.async = append(v.async, req.(*wireguard.WgSetAsyncMode).AsyncEnable)
		return []api.Message{&wireguard.WgSetAsyncModeReply{}}, nil
	})
	return v
}

func (v *fakeVPP) addIface(name, tag string) uint32 {
	idx := v.nextIf
	v.nextIf++
	v.ifaces[idx] = &interfaces.SwInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), InterfaceName: name, Tag: tag}
	return idx
}

func (v *fakeVPP) peerIdx() []uint32 {
	out := make([]uint32, 0, len(v.peers))
	for i := range v.peers {
		out = append(out, i)
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}

// emit sends a peer event the way VPP does: only to registered peers.
func (v *fakeVPP) emit(idx uint32, flags wireguard.WireguardPeerFlags) bool {
	if !v.registered[idx] {
		return false
	}
	return v.Emit(&wireguard.WireguardPeerEvent{PeerIndex: idx, Flags: flags}) > 0
}
