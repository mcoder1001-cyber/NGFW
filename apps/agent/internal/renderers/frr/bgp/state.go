package bgp

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strings"

	"ngfw/agent/internal/renderers/frr"
)

// ShowSummary is the one BGP state command: every instance's per-family neighbour table. It is bounded by the number of
// neighbours (never the RIB, RF-1 review M3).
const ShowSummary frr.ShowCommand = "show bgp vrf all summary json"

// SummaryReader is the Retrieve / RoutingState key of ShowSummary.
const SummaryReader = "bgpSummary"

// PollerNeighbors is the 1 Hz poller of neighbour states: key "<vrf>|<peer>", value the FRR state name.
const PollerNeighbors = "bgp-neighbors"

// Instance is one `router bgp` instance as FRR reports it.
type Instance struct {
	VRF       string
	ASN       uint32
	RouterID  string
	Neighbors []Neighbor
}

// Neighbor is one neighbour's session state, summed over the families it is active in.
type Neighbor struct {
	Address          string
	RemoteAS         uint32
	State            string
	UptimeSec        uint64
	PrefixesReceived uint32
	PrefixesSent     uint32
	Flaps            uint32
	Established      uint32
	Description      string
	MessagesReceived uint64
	MessagesSent     uint64
	AFIs             []NeighborAFI
}

// NeighborAFI is one family's prefix counters of a neighbour.
type NeighborAFI struct {
	AFI              string
	PrefixesReceived uint32
	PrefixesSent     uint32
}

// summaryPeer is the part of one `peers.<addr>` entry the agent reads (FRR 10.7 field names).
type summaryPeer struct {
	RemoteAs               uint32 `json:"remoteAs"`
	State                  string `json:"state"`
	PeerState              string `json:"peerState"`
	PeerUptimeMsec         uint64 `json:"peerUptimeMsec"`
	PfxRcd                 uint32 `json:"pfxRcd"`
	PfxSnt                 uint32 `json:"pfxSnt"`
	ConnectionsEstablished uint32 `json:"connectionsEstablished"`
	ConnectionsDropped     uint32 `json:"connectionsDropped"`
	Desc                   string `json:"desc"`
	MsgRcvd                uint64 `json:"msgRcvd"`
	MsgSent                uint64 `json:"msgSent"`
}

type summaryAF struct {
	RouterID string                 `json:"routerId"`
	AS       uint32                 `json:"as"`
	VRFName  string                 `json:"vrfName"`
	Peers    map[string]summaryPeer `json:"peers"`
}

// ParseSummary decodes `show bgp vrf all summary json` (vrf → family → {routerId, as, peers}) into instances sorted by
// VRF, neighbours sorted by address. A family without peers still names its instance.
func ParseSummary(raw json.RawMessage) ([]Instance, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "{}" {
		return nil, nil
	}
	var byVRF map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &byVRF); err != nil {
		return nil, fmt.Errorf("frr: decode %s: %w", ShowSummary, err)
	}
	var out []Instance
	for _, vrf := range slices.Sorted(maps.Keys(byVRF)) {
		inst := Instance{VRF: vrf}
		peers := map[string]*Neighbor{}
		for _, fam := range slices.Sorted(maps.Keys(byVRF[vrf])) {
			var af summaryAF
			if err := json.Unmarshal(byVRF[vrf][fam], &af); err != nil || (af.AS == 0 && af.Peers == nil) {
				continue // not a family object (FRR adds scalar keys in some versions)
			}
			if inst.ASN == 0 {
				inst.ASN, inst.RouterID = af.AS, af.RouterID
			}
			for addr, p := range af.Peers {
				n := peers[addr]
				if n == nil {
					n = &Neighbor{Address: addr, RemoteAS: p.RemoteAs, State: p.State, Description: p.Desc,
						Flaps: p.ConnectionsDropped, Established: p.ConnectionsEstablished,
						MessagesReceived: p.MsgRcvd, MessagesSent: p.MsgSent}
					if p.State == "Established" {
						n.UptimeSec = p.PeerUptimeMsec / 1000
					}
					if p.PeerState == "Admin" && p.State == "Idle" {
						n.State = "Idle (Admin)"
					}
					peers[addr] = n
				}
				n.PrefixesReceived += p.PfxRcd
				n.PrefixesSent += p.PfxSnt
				n.AFIs = append(n.AFIs, NeighborAFI{AFI: fam, PrefixesReceived: p.PfxRcd, PrefixesSent: p.PfxSnt})
			}
		}
		for _, n := range peers {
			inst.Neighbors = append(inst.Neighbors, *n)
		}
		slices.SortFunc(inst.Neighbors, func(a, b Neighbor) int { return compareAddr(a.Address, b.Address) })
		out = append(out, inst)
	}
	return out, nil
}

func compareAddr(a, b string) int {
	x, errA := netip.ParseAddr(a)
	y, errB := netip.ParseAddr(b)
	if errA != nil || errB != nil {
		return strings.Compare(a, b)
	}
	return x.Compare(y)
}

// Summary reads and parses ShowSummary through show (frr.Renderer.ShowJSON).
func Summary(ctx context.Context, show frr.ShowFunc) ([]Instance, error) {
	raw, err := show(ctx, ShowSummary)
	if err != nil {
		return nil, err
	}
	return ParseSummary(raw)
}

// PollNeighbors is the frr.PollFunc of PollerNeighbors: "<vrf>|<peer>" → state.
func PollNeighbors(ctx context.Context, show frr.ShowFunc) (map[string]string, error) {
	insts, err := Summary(ctx, show)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, in := range insts {
		for _, n := range in.Neighbors {
			out[in.VRF+"|"+n.Address] = n.State
		}
	}
	return out, nil
}

// SplitNeighborKey splits a PollerNeighbors key into VRF and peer.
func SplitNeighborKey(key string) (vrf, peer string) {
	vrf, peer, ok := strings.Cut(key, "|")
	if !ok {
		return "", key
	}
	return vrf, peer
}
