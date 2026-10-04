package rip

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"ngfw/agent/internal/renderers/frr"
	"strconv"
	"strings"
)

// StatusReader identifies the on-demand RIP peer status reader.
const StatusReader = "ripStatus"

// RoutesReader identifies the on-demand RIP route reader.
const RoutesReader = "ripRoutes"

// ShowStatus reads public RIP peer status.
const ShowStatus frr.ShowCommand = "show ip rip status"

// ShowRoutes reads RIP routes across VRFs as JSON.
const ShowRoutes frr.ShowCommand = "show ip route vrf all rip json"

// MaxStatusBytes bounds input accepted by the status parser.
const MaxStatusBytes = 1 << 20

// Peer contains public RIP peer counters and last update metadata.
type Peer struct {
	Address    string `json:"address"`
	BadPackets uint32 `json:"badPackets"`
	BadRoutes  uint32 `json:"badRoutes"`
	Distance   uint32 `json:"distance"`
	LastUpdate string `json:"lastUpdate"`
}

// Status contains the observed RIP protocol and peer table for one VRF.
type Status struct {
	VRF      string `json:"vrf"`
	Protocol string `json:"protocol"`
	Peers    []Peer `json:"peers"`
}

func init() {
	frr.RegisterStateReader(frr.StateReader{Key: StatusReader, Command: ShowStatus, OnDemand: true, Parse: func(raw []byte) (json.RawMessage, error) { return ParseStatus(raw, false) }})
	frr.RegisterStateReader(frr.StateReader{Key: RoutesReader, Command: ShowRoutes, OnDemand: true})
}

// ParseStatus accepts FRR 10 status peer tables, including RIPng's two-line IPv6 rows.
// It projects only public peer counters. Unrecognized/truncated output is never empty-success.
func ParseStatus(raw []byte, ipv6 bool) (json.RawMessage, error) {
	if len(raw) > MaxStatusBytes {
		return nil, fmt.Errorf("status limit")
	}
	protocol := "rip"
	header := `Routing Protocol is "rip"`
	if ipv6 {
		protocol = "ripng"
		header = `Routing Protocol is "RIPng"`
	}
	text := string(raw)
	if !strings.Contains(text, header) || strings.Count(text, header) != 1 || !strings.Contains(text, "Routing Information Sources:") {
		return nil, fmt.Errorf("unrecognized status")
	}
	status := Status{VRF: frr.DefaultVRF, Protocol: protocol, Peers: []Peer{}}
	source := false
	table := false
	pending := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "Routing Information Sources:" {
			source = true
			continue
		}
		if !source {
			continue
		}
		if strings.HasPrefix(line, "Gateway") {
			if !strings.Contains(line, "BadPackets") || !strings.Contains(line, "BadRoutes") || !strings.Contains(line, "Last Update") {
				return nil, fmt.Errorf("peer header")
			}
			table = true
			continue
		}
		if line == "" {
			continue
		}
		if !table {
			return nil, fmt.Errorf("missing peer header")
		}
		fields := strings.Fields(line)
		if pending != "" {
			fields = append([]string{pending}, fields...)
			pending = ""
		}
		address, err := netip.ParseAddr(fields[0])
		if err != nil { // FRR may append a distance summary after the peer table.
			if strings.HasPrefix(line, "Distance:") {
				break
			}
			return nil, fmt.Errorf("invalid peer address")
		}
		if ipv6 != address.Is6() || address.Is4In6() {
			return nil, fmt.Errorf("peer family")
		}
		if ipv6 && len(fields) == 1 {
			pending = fields[0]
			continue
		}
		if len(fields) != 5 || len(fields[4]) > 24 {
			return nil, fmt.Errorf("invalid peer row")
		}
		var counters [3]uint32
		for i := range counters {
			value, err := strconv.ParseUint(fields[i+1], 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid peer counter")
			}
			counters[i] = uint32(value)
		}
		if counters[2] > 255 {
			return nil, fmt.Errorf("invalid peer distance")
		}
		for _, r := range fields[4] {
			if !strings.ContainsRune("0123456789:wdhmsnever", r) {
				return nil, fmt.Errorf("invalid peer age")
			}
		}
		status.Peers = append(status.Peers, Peer{Address: address.String(), BadPackets: counters[0], BadRoutes: counters[1], Distance: counters[2], LastUpdate: fields[4]})
		if len(status.Peers) > 10000 {
			return nil, fmt.Errorf("peer limit")
		}
	}
	if pending != "" || !table {
		return nil, fmt.Errorf("incomplete peer table")
	}
	return json.Marshal(status)
}
