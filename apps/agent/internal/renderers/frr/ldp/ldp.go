// Package ldp renders the existing routing.mpls.ldp contract. Live FRR convergence
// validation is separate from the pure rendering tests.
package ldp

import (
	"fmt"
	"maps"
	"net/netip"
	"regexp"
	"slices"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/frr"
)

// Name identifies the LDP renderer section.
const Name = "ldp"

// ShowNeighbors reads the bounded LDP neighbor state, excluding the LIB.
const ShowNeighbors frr.ShowCommand = "show mpls ldp neighbor json"

var passwordRef = regexp.MustCompile(`^password/[A-Za-z0-9_.-]{1,64}$`)

func init() {
	frr.RegisterSection(Section{})
	frr.RegisterStateReader(frr.StateReader{Key: "ldpNeighbors", Command: ShowNeighbors})
}

// Section implements the FRR protocol section for LDP.
type Section struct{}

// Name implements frr.Section.
func (Section) Name() string { return Name }

// Order implements frr.Section.
func (Section) Order() int { return 600 }

// Render implements frr.Section.
func (Section) Render(rc *frr.RenderContext) ([]string, error) {
	return Render(rc.Desired.GetRouting().GetMpls().GetLdp(), rc)
}

// Context supplies the existing interface mapper and secret resolver.
type Context interface {
	MapInterface(string) (string, bool)
	Secret(string) (string, error)
}

// Render validates CLI tokens before producing any output. It never resolves
// inline passwords: the framework owns reference resolution and redaction.
func Render(c *vrxv1.MplsLdp, rc Context) ([]string, error) {
	if c == nil {
		return nil, nil
	}
	rid, err := ipv4(c.GetRouterId())
	if err != nil {
		return nil, fmt.Errorf("LDP routerId: %w", err)
	}
	transport, err := ipv4(c.GetTransportAddress())
	if err != nil {
		return nil, fmt.Errorf("LDP transportAddress: %w", err)
	}
	var out []string
	if r := c.GetLabelRange(); r != nil {
		if r.GetMin() < 16 || r.GetMax() > 1048575 || r.GetMin() > r.GetMax() {
			return nil, fmt.Errorf("LDP labelRange is outside 16..1048575")
		}
		out = append(out, fmt.Sprintf("mpls label dynamic-block %d %d", r.GetMin(), r.GetMax()))
	}
	out = append(out, "mpls ldp", " router-id "+rid)
	for _, peer := range slices.Sorted(maps.Keys(c.GetNeighbors())) {
		address, err := ipv4(peer)
		if err != nil {
			return nil, fmt.Errorf("LDP neighbor: %w", err)
		}
		ref := c.GetNeighbors()[peer].GetPasswordRef()
		if ref == "" {
			continue
		}
		if !passwordRef.MatchString(ref) {
			return nil, fmt.Errorf("LDP neighbor passwordRef must reference password/<name>")
		}
		password, err := rc.Secret(ref)
		if err != nil {
			return nil, fmt.Errorf("LDP neighbor password resolution failed")
		}
		out = append(out, " neighbor "+address+" password "+password)
	}
	out = append(out, " address-family ipv4", "  discovery transport-address "+transport)
	seen := map[string]bool{}
	for _, logical := range slices.Sorted(slices.Values(c.GetInterfaces())) {
		host, ok := rc.MapInterface(logical)
		if !ok {
			return nil, fmt.Errorf("LDP interface has no Linux mapping")
		}
		if _, err := frr.IfName(host); err != nil {
			return nil, fmt.Errorf("LDP mapped interface is unsafe")
		}
		if seen[host] {
			return nil, fmt.Errorf("LDP interfaces have duplicate Linux mapping")
		}
		seen[host] = true
		out = append(out, "  interface "+host, "  exit")
	}
	out = append(out, " exit-address-family", "exit")
	return out, nil
}

func ipv4(s string) (string, error) {
	a, err := netip.ParseAddr(s)
	if err != nil || !a.Is4() {
		return "", fmt.Errorf("IPv4 address required")
	}
	return a.String(), nil
}
