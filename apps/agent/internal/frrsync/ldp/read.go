package ldp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/frr"
)

// MaxRows bounds decoded state rows, next hops and translated bindings per observation.
const MaxRows = 10000

// MaxOutput bounds each FRR command response to four MiB.
const MaxOutput = 4 << 20

// ShowBindings is the bounded LDP LIB command.
const ShowBindings frr.ShowCommand = "show mpls ldp binding json"

// ShowDiscovery reads detailed link discovery adjacency addresses.
const ShowDiscovery frr.ShowCommand = "show mpls ldp discovery detail json"

// ShowNeighbors reads LDP session states.
const ShowNeighbors frr.ShowCommand = "show mpls ldp neighbor json"

// Observation is a validated daemon snapshot with joined forwarding bindings and read-only state.
type Observation struct {
	Bindings  []Binding
	LIB       []*ngfwv1.LdpBindingState
	Neighbors []*ngfwv1.LdpNeighborState
}

// Read uses FRR ldp_vty_exec.c's LIB and detailed link discovery shapes.
func Read(ctx context.Context, show frr.ShowFunc) (Observation, error) {
	var out Observation
	bounded := func(ctx context.Context, cmd frr.ShowCommand) (json.RawMessage, error) {
		raw, err := show(ctx, cmd)
		if err != nil {
			return nil, err
		}
		if len(raw) > MaxOutput {
			return nil, fmt.Errorf("LDP output too large")
		}
		return raw, nil
	}
	rawRIB, err := bounded(ctx, frr.ShowCommand("show ip route json"))
	if err != nil {
		return out, err
	}
	rib, err := frr.DecodeRIB(rawRIB)
	if err != nil {
		return out, err
	}
	if len(rib) > MaxRows {
		return out, fmt.Errorf("LDP RIB row limit exceeded")
	}
	active := map[string]map[string]bool{}
	ribHops := 0
	for _, route := range rib {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		ribHops += len(route.Nexthops)
		if ribHops > MaxRows {
			return out, fmt.Errorf("LDP RIB hop limit exceeded")
		}
		if !route.Selected {
			continue
		}
		if active[route.Prefix] == nil {
			active[route.Prefix] = map[string]bool{}
		}
		for _, hop := range route.Nexthops {
			if hop.Active {
				active[route.Prefix][hop.InterfaceName+"/"+hop.IP] = true
			}
		}
	}
	var lib struct {
		Bindings []struct {
			AddressFamily, Prefix, NeighborID, LocalLabel, RemoteLabel string
			InUse                                                      int
		}
	}
	var neighbors struct {
		Neighbors []struct{ AddressFamily, NeighborID, State, TransportAddress string }
	}
	var discovery struct {
		Interfaces map[string]struct {
			Adjacencies []struct{ LsrID, SourceAddress string }
		}
	}
	for _, item := range []struct {
		cmd frr.ShowCommand
		dst any
	}{{ShowBindings, &lib}, {ShowNeighbors, &neighbors}, {ShowDiscovery, &discovery}} {
		raw, err := bounded(ctx, item.cmd)
		if err != nil {
			return out, err
		}
		var shape map[string]json.RawMessage
		if err = json.Unmarshal(raw, &shape); err != nil || shape == nil {
			return out, fmt.Errorf("LDP invalid state shape")
		}
		key := "bindings"
		if item.cmd == ShowNeighbors {
			key = "neighbors"
		}
		if item.cmd == ShowDiscovery {
			key = "interfaces"
		}
		// FRR creates binding/neighbor arrays lazily: {} is its genuine empty shape.
		if len(shape) > 0 {
			value, ok := shape[key]
			if !ok || string(value) == "null" {
				return out, fmt.Errorf("LDP missing state field %s", key)
			}
		}
		if err = json.Unmarshal(raw, item.dst); err != nil {
			return out, fmt.Errorf("LDP invalid JSON: %w", err)
		}
	}
	if len(neighbors.Neighbors) > MaxRows || len(lib.Bindings) > MaxRows {
		return out, fmt.Errorf("LDP row limit exceeded")
	}
	type link struct{ name, source string }
	if len(discovery.Interfaces) > MaxRows {
		return out, fmt.Errorf("LDP interface limit exceeded")
	}
	adjacent := map[string]map[string]link{}
	count := 0
	for name, iface := range discovery.Interfaces {
		for _, adj := range iface.Adjacencies {
			count++
			if count > MaxRows {
				return out, fmt.Errorf("LDP adjacency limit exceeded")
			}
			if adjacent[adj.LsrID] == nil {
				adjacent[adj.LsrID] = map[string]link{}
			}
			adjacent[adj.LsrID][name+"/"+adj.SourceAddress] = link{name, adj.SourceAddress}
		}
	}
	for _, n := range neighbors.Neighbors {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if n.AddressFamily != "ipv4" && n.AddressFamily != "ipv6" {
			return out, fmt.Errorf("LDP invalid neighbor address family")
		}
		if n.AddressFamily == "ipv4" {
			out.Neighbors = append(out.Neighbors, &ngfwv1.LdpNeighborState{LsrId: n.NeighborID, Address: n.TransportAddress, State: n.State})
		}
	}
	joined := map[string][]link{}
	work := 0
	for _, b := range lib.Bindings {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if b.AddressFamily != "ipv4" && b.AddressFamily != "ipv6" {
			return out, fmt.Errorf("LDP invalid binding address family")
		}
		if b.AddressFamily != "ipv4" {
			continue
		}
		local, err := label(b.LocalLabel)
		if err != nil {
			return out, err
		}
		remote, err := label(b.RemoteLabel)
		if err != nil && b.InUse != 0 {
			return out, err
		}
		out.LIB = append(out.LIB, &ngfwv1.LdpBindingState{Prefix: b.Prefix, LocalLabel: local, RemoteLabel: remote, Peer: b.NeighborID, InUse: b.InUse != 0})
		if b.InUse == 0 || local == 3 {
			continue
		}
		found := false
		joinKey := b.Prefix + "/" + b.NeighborID
		links, seen := joined[joinKey]
		if !seen {
			for hop := range active[b.Prefix] {
				work++
				if work > MaxRows*10 {
					return out, fmt.Errorf("LDP join work limit exceeded")
				}
				if err := ctx.Err(); err != nil {
					return out, err
				}
				if adj, ok := adjacent[b.NeighborID][hop]; ok {
					links = append(links, adj)
				}
			}
			joined[joinKey] = links
		}
		for _, adj := range links {
			if len(out.Bindings) >= MaxRows {
				return out, fmt.Errorf("LDP translated binding limit exceeded")
			}
			out.Bindings = append(out.Bindings, Binding{FEC: b.Prefix, LocalLabel: local, RemoteLabel: remote, NextHop: adj.source, LinuxInterface: adj.name})
			found = true
		}
		if !found {
			return out, fmt.Errorf("LDP in-use binding has no link adjacency")
		}
	}
	return out, nil
}
func label(s string) (uint32, error) {
	if s == "imp-null" || s == "implicit-null" {
		return 3, nil
	}
	n, err := strconv.ParseUint(s, 10, 20)
	if err != nil {
		return 0, fmt.Errorf("LDP unsupported label")
	}
	return uint32(n), nil
}
