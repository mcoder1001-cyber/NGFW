package ospf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"ngfw/agent/internal/renderers/frr"
	"slices"
	"strings"
)

// NeighborsReader6 identifies the bounded OSPFv3 neighbor reader.
const NeighborsReader6 = "ospf6Neighbors"

// InterfacesReader6 identifies the OSPFv3 interface reader.
const InterfacesReader6 = "ospf6Interfaces"

// ShowNeighbors6 is the fixed FRR OSPFv3 neighbor observation command.
const ShowNeighbors6 frr.ShowCommand = "show ipv6 ospf6 vrf all neighbor json"

// ShowInterfaces6 is the fixed FRR OSPFv3 interface observation command.
const ShowInterfaces6 frr.ShowCommand = "show ipv6 ospf6 vrf all interface json"

// PollNeighbors6 reads FRR's v3 neighbor arrays (unlike v2's router-id maps). Invalid observations never
// replace the previous poll snapshot with empty state and synthesize removal events.
func PollNeighbors6(ctx context.Context, show frr.ShowFunc) (map[string]string, error) {
	raw, err := show(ctx, ShowNeighbors6)
	if err != nil {
		return nil, err
	}
	var top map[string]json.RawMessage
	if err = json.Unmarshal(raw, &top); err != nil {
		return nil, err
	}
	if top == nil {
		return nil, fmt.Errorf("OSPFv3 neighbor reader is not an object")
	}
	instances := top
	if _, ok := top["neighbors"]; ok {
		instances = map[string]json.RawMessage{"default": raw}
	}
	out := map[string]string{}
	for vrf, data := range instances {
		var instance map[string]json.RawMessage
		if err = json.Unmarshal(data, &instance); err != nil {
			return nil, fmt.Errorf("OSPFv3 neighbor instance invalid")
		}
		neighbors, ok := instance["neighbors"]
		if !ok || !strings.HasPrefix(strings.TrimSpace(string(neighbors)), "[") {
			return nil, fmt.Errorf("OSPFv3 neighbor list missing or invalid")
		}
		var entries []struct {
			ID        string `json:"neighborId"`
			Interface string `json:"interfaceName"`
			State     string `json:"state"`
		}
		if err = json.Unmarshal(neighbors, &entries); err != nil {
			return nil, fmt.Errorf("OSPFv3 neighbor list invalid")
		}
		for _, n := range entries {
			id, e := netip.ParseAddr(n.ID)
			if e != nil || !id.Is4() {
				return nil, fmt.Errorf("OSPFv3 neighbor router ID invalid")
			}
			if _, e = frr.IfName(n.Interface); e != nil {
				return nil, fmt.Errorf("OSPFv3 neighbor interface invalid")
			}
			if !slices.Contains([]string{"None", "Down", "Attempt", "Init", "Twoway", "2-Way", "ExStart", "ExChange", "Exchange", "Loading", "Full"}, n.State) {
				return nil, fmt.Errorf("OSPFv3 neighbor state invalid")
			}
			out[vrf+"|"+n.ID+"|"+n.Interface] = n.State
		}
	}
	return out, nil
}
