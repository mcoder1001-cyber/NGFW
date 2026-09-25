package wireguard

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/wireguard"
	"ngfw/agent/internal/descriptors/vpn"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp"
)

// InterfaceState is the live, read-only state of one of this owner's WireGuard interfaces (the
// F-wireguard WireguardState RPC). No key material: the public key is the only key it carries.
type InterfaceState struct {
	Name      string // wg<instance> (logical name = VPP name, D-069)
	Instance  uint32
	SwIfIndex uint32
	PublicKey string // std base64
	Port      uint32
	SrcIP     string
	AdminUp   bool
	LinkUp    bool
	Peers     []PeerState // sorted by public key
}

// PeerState is the live state of one peer (wireguard_peers_dump — the v1 dump carries no
// preshared key).
type PeerState struct {
	PublicKey           string
	PeerIndex           uint32
	Established         bool
	Dead                bool
	Endpoint            string // current endpoint ("" = none yet)
	EndpointPort        uint32
	PersistentKeepalive uint32
	TableID             uint32
	AllowedIps          []string // canonical, sorted
}

// DumpState reads this owner's WireGuard interfaces and their peers: one sw_interface_dump, one
// wireguard_interface_dump (show_private_key=false) and one wireguard_peers_dump. names (VPP
// names) filters the interfaces; empty = all. Sorted by name.
func DumpState(ctx context.Context, c vpp.Client, owner string, names ...string) ([]InterfaceState, error) {
	tbl, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return nil, err
	}
	svc := wireguard.NewServiceClient(c)
	stream, err := svc.WireguardInterfaceDump(ctx, &wireguard.WireguardInterfaceDump{
		ShowPrivateKey: false, SwIfIndex: interface_types.InterfaceIndex(noInterface),
	})
	if err != nil {
		return nil, fmt.Errorf("wireguard_interface_dump: %w", err)
	}
	bySw := map[uint32]*InterfaceState{}
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("wireguard_interface_dump: %w", err)
		}
		w := det.Interface
		vpn.Zero(w.PrivateKey)
		idx := uint32(w.SwIfIndex)
		name := ItfName(w.UserInstance)
		if id, owned := tbl.OwnedID(idx); !owned || id != name {
			continue
		}
		if len(names) > 0 && !slices.Contains(names, name) {
			continue
		}
		st := &InterfaceState{
			Name: name, Instance: w.UserInstance, SwIfIndex: idx, Port: uint32(w.Port),
			PublicKey: base64.StdEncoding.EncodeToString(w.PublicKey), SrcIP: srcIP(w.SrcIP),
		}
		if d, ok := tbl.Details(idx); ok {
			st.AdminUp = d.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0
			st.LinkUp = d.Flags&interface_types.IF_STATUS_API_FLAG_LINK_UP != 0
		}
		bySw[idx] = st
	}
	if len(bySw) > 0 {
		peers, err := svc.WireguardPeersDump(ctx, &wireguard.WireguardPeersDump{PeerIndex: noInterface})
		if err != nil {
			return nil, fmt.Errorf("wireguard_peers_dump: %w", err)
		}
		for {
			det, err := peers.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("wireguard_peers_dump: %w", err)
			}
			p := det.Peer
			st, ok := bySw[uint32(p.SwIfIndex)]
			if !ok {
				continue // another owner's interface
			}
			ps := PeerState{
				PublicKey: base64.StdEncoding.EncodeToString(p.PublicKey), PeerIndex: p.PeerIndex,
				Established:         p.Flags&wireguard.WIREGUARD_PEER_ESTABLISHED != 0,
				Dead:                p.Flags&wireguard.WIREGUARD_PEER_STATUS_DEAD != 0,
				EndpointPort:        uint32(p.Port),
				PersistentKeepalive: uint32(p.PersistentKeepalive),
				TableID:             p.TableID,
			}
			if !vpn.IsUnspecified(p.Endpoint) {
				ps.Endpoint = vpn.AddressString(p.Endpoint)
			}
			for _, a := range p.AllowedIps {
				ps.AllowedIps = append(ps.AllowedIps, vpn.PrefixString(a))
			}
			slices.Sort(ps.AllowedIps)
			ps.AllowedIps = slices.Compact(ps.AllowedIps)
			st.Peers = append(st.Peers, ps)
		}
	}
	out := make([]InterfaceState, 0, len(bySw))
	for _, st := range bySw {
		sort.Slice(st.Peers, func(i, j int) bool { return st.Peers[i].PublicKey < st.Peers[j].PublicKey })
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
