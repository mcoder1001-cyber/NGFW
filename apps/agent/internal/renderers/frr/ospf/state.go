package ospf

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

// State commands: bounded by the number of neighbours / OSPF interfaces (never the LSDB or the RIB, RF-1 review M3).
const (
	ShowNeighbors  frr.ShowCommand = "show ip ospf vrf all neighbor json"
	ShowInterfaces frr.ShowCommand = "show ip ospf vrf all interface json"
)

// Retrieve / RoutingState keys of the state commands.
const (
	NeighborsReader  = "ospfNeighbors"
	InterfacesReader = "ospfInterfaces"
)

// PollerNeighbors is the 1 Hz poller of neighbour states: key "<vrf>|<router id>|<interface>", value FRR's state
// ("Full/DR", "2-Way/DROther", …).
const PollerNeighbors = "ospf-neighbors"

// Neighbor is one adjacency as FRR reports it.
type Neighbor struct {
	VRF       string
	RouterID  string
	Address   string
	Interface string
	State     string
	Priority  int
}

// nbr is the part of one neighbour entry the agent reads (FRR 10 field names; older builds used "state").
type nbr struct {
	NbrState  string `json:"nbrState"`
	State     string `json:"state"`
	Address   string `json:"ifaceAddress"`
	Address2  string `json:"address"`
	IfaceName string `json:"ifaceName"`
	Priority  int    `json:"nbrPriority"`
	Priority2 int    `json:"priority"`
}

// ParseNeighbors decodes `show ip ospf vrf all neighbor json` ({<vrf>: {vrfName, neighbors: {<rid>: [entry…]}}}, or
// a single instance's {neighbors: …}) into neighbours sorted by VRF, router id and interface.
func ParseNeighbors(raw json.RawMessage) ([]Neighbor, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "{}" {
		return nil, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("frr: decode %s: %w", ShowNeighbors, err)
	}
	insts := map[string]json.RawMessage{}
	if _, single := top["neighbors"]; single {
		insts[frr.DefaultVRF] = raw
	} else {
		insts = top
	}
	var out []Neighbor
	for _, vrf := range slices.Sorted(maps.Keys(insts)) {
		var inst struct {
			Neighbors map[string]json.RawMessage `json:"neighbors"`
		}
		if err := json.Unmarshal(insts[vrf], &inst); err != nil {
			continue // a scalar key, not an instance
		}
		for rid, v := range inst.Neighbors {
			var entries []nbr
			if err := json.Unmarshal(v, &entries); err != nil {
				var one nbr
				if json.Unmarshal(v, &one) != nil {
					continue
				}
				entries = []nbr{one}
			}
			for _, e := range entries {
				n := Neighbor{VRF: vrf, RouterID: rid, Address: cmpOr(e.Address, e.Address2), State: cmpOr(e.NbrState, e.State),
					Priority: max(e.Priority, e.Priority2)}
				// FRR prints "<ifname>:<local address>"
				n.Interface, _, _ = strings.Cut(e.IfaceName, ":")
				out = append(out, n)
			}
		}
	}
	slices.SortFunc(out, func(a, b Neighbor) int {
		if c := strings.Compare(a.VRF, b.VRF); c != 0 {
			return c
		}
		if c := compareAddr(a.RouterID, b.RouterID); c != 0 {
			return c
		}
		return strings.Compare(a.Interface, b.Interface)
	})
	return out, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func compareAddr(a, b string) int {
	x, errA := netip.ParseAddr(a)
	y, errB := netip.ParseAddr(b)
	if errA != nil || errB != nil {
		return strings.Compare(a, b)
	}
	return x.Compare(y)
}

// Neighbors reads and parses ShowNeighbors through show (frr.Renderer.ShowJSON).
func Neighbors(ctx context.Context, show frr.ShowFunc) ([]Neighbor, error) {
	raw, err := show(ctx, ShowNeighbors)
	if err != nil {
		return nil, err
	}
	return ParseNeighbors(raw)
}

// PollNeighbors is the frr.PollFunc of PollerNeighbors.
func PollNeighbors(ctx context.Context, show frr.ShowFunc) (map[string]string, error) {
	ns, err := Neighbors(ctx, show)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, n := range ns {
		out[n.VRF+"|"+n.RouterID+"|"+n.Interface] = n.State
	}
	return out, nil
}
