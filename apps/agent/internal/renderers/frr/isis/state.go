package isis

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"ngfw/agent/internal/renderers/frr"
)

// ShowNeighbors is bounded by the number of adjacencies (the LSDB is not read periodically, RF-1 review M3).
const ShowNeighbors frr.ShowCommand = "show isis vrf all neighbor json"

// NeighborsReader is the Retrieve / RoutingState key of ShowNeighbors.
const NeighborsReader = "isisNeighbors"

// PollerAdjacencies is the 1 Hz poller of adjacency states: key "<vrf>|<area tag>|<system id>|<interface>|<level>",
// value FRR's state ("Up", "Initializing", "Down").
const PollerAdjacencies = "isis-adjacencies"

// Adjacency is one IS-IS adjacency as FRR reports it.
type Adjacency struct {
	VRF       string
	Area      string
	SystemID  string
	Interface string
	Level     string
	State     string
}

// adj is one circuit entry (FRR 10 hyphenated field names; FRR 8/9 builds used other spellings, both read).
type adj struct {
	Adj       string          `json:"adj"`
	SystemID  string          `json:"system-id"`
	Interface string          `json:"interface"`
	Level     json.RawMessage `json:"level"`
	State     string          `json:"state"`
	State2    string          `json:"adj-state"`
}

type area struct {
	Area     string `json:"area"`
	Circuits []adj  `json:"circuits"`
}

// ParseNeighbors decodes `show isis vrf all neighbor json`: {"vrfs":[{"vrf":…, "areas":[…]}]} or a single instance's
// {"areas":[{"area":tag,"circuits":[…]}]}. Entries without an adjacency (a circuit with no neighbour) are skipped.
func ParseNeighbors(raw json.RawMessage) ([]Adjacency, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "{}" {
		return nil, nil
	}
	var shape map[string]json.RawMessage
	if json.Unmarshal(raw, &shape) != nil || shape == nil {
		return nil, fmt.Errorf("frr: malformed IS-IS neighbors")
	}
	hasArrays := false
	for _, key := range []string{"areas", "vrfs"} {
		if value, ok := shape[key]; ok {
			var rows []json.RawMessage
			if string(value) == "null" || json.Unmarshal(value, &rows) != nil {
				return nil, fmt.Errorf("frr: malformed IS-IS %s", key)
			}
			hasArrays = true
			if key == "vrfs" {
				for _, row := range rows {
					var vrf map[string]json.RawMessage
					var areas []json.RawMessage
					if json.Unmarshal(row, &vrf) != nil || vrf["areas"] == nil || string(vrf["areas"]) == "null" || json.Unmarshal(vrf["areas"], &areas) != nil {
						return nil, fmt.Errorf("frr: malformed IS-IS VRF areas")
					}
				}
			}
		}
	}
	if !hasArrays {
		return nil, fmt.Errorf("frr: missing IS-IS neighbor shape")
	}
	var top struct {
		Vrfs []struct {
			Vrf   string `json:"vrf"`
			Name  string `json:"vrf_name"`
			Areas []area `json:"areas"`
		} `json:"vrfs"`
		Areas []area `json:"areas"`
	}
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("frr: decode %s: %w", ShowNeighbors, err)
	}
	var out []Adjacency
	var malformed bool
	add := func(vrf string, areas []area) {
		if vrf == "" {
			vrf = frr.DefaultVRF
		}
		for _, a := range areas {
			if a.Circuits == nil {
				malformed = true
				continue
			}
			for _, c := range a.Circuits {
				sid := c.Adj
				if sid == "" {
					sid = c.SystemID
				}
				if sid == "" {
					continue
				}
				st := c.State
				if st == "" {
					st = c.State2
				}
				if c.Interface == "" || st == "" || levelText(c.Level) == "" {
					malformed = true
					continue
				}
				out = append(out, Adjacency{VRF: vrf, Area: a.Area, SystemID: sid, Interface: c.Interface, Level: levelText(c.Level), State: st})
			}
		}
	}
	add("", top.Areas)
	for _, v := range top.Vrfs {
		name := v.Vrf
		if name == "" {
			name = v.Name
		}
		add(name, v.Areas)
	}
	if malformed {
		return nil, fmt.Errorf("frr: malformed IS-IS adjacency row")
	}
	slices.SortFunc(out, func(a, b Adjacency) int {
		return strings.Compare(a.key(), b.key())
	})
	return out, nil
}

// levelText accepts FRR's level as a number (1, 2) or a string ("L1", "Level-2", "1").
func levelText(r json.RawMessage) string {
	var n int
	if json.Unmarshal(r, &n) == nil {
		return strconv.Itoa(n)
	}
	var s string
	if json.Unmarshal(r, &s) == nil {
		return s
	}
	return ""
}

func (a Adjacency) key() string {
	return a.VRF + "|" + a.Area + "|" + a.SystemID + "|" + a.Interface + "|" + a.Level
}

// Neighbors reads and parses ShowNeighbors through show (frr.Renderer.ShowJSON).
func Neighbors(ctx context.Context, show frr.ShowFunc) ([]Adjacency, error) {
	raw, err := show(ctx, ShowNeighbors)
	if err != nil {
		return nil, err
	}
	return ParseNeighbors(raw)
}

// PollAdjacencies is the frr.PollFunc of PollerAdjacencies.
func PollAdjacencies(ctx context.Context, show frr.ShowFunc) (map[string]string, error) {
	as, err := Neighbors(ctx, show)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, a := range as {
		out[a.key()] = a.State
	}
	return out, nil
}
