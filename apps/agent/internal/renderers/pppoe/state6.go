package pppoe

import (
	"bufio"
	"bytes"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// IPv6State is the IPv6 side of a session as the ipv6-up/ipv6-down hook last recorded it ("<hostif>.state6").
// Every value comes from the network through the kernel or dhcpcd, so it is parsed and kept only when valid.
type IPv6State struct {
	// Up is true between IPv6CP up and down.
	Up bool
	// Failure is a fixed helper error code, never daemon output or network text.
	Failure string
	// LinkLocal / PeerLinkLocal are the IPv6CP-negotiated link-local addresses (ours, the ISP's).
	LinkLocal, PeerLinkLocal netip.Addr
	// Addrs are the global unicast addresses the kernel holds on the PPP link (SLAAC, DHCPv6 IA_NA), sorted, with
	// the on-link prefix length the kernel reports.
	Addrs []netip.Prefix
	// Gateway is the IPv6 default router learned from the RA (usually the ISP's link-local); invalid = none.
	Gateway netip.Addr
	// Delegated is the DHCPv6-PD prefix (ipv6 "dhcpv6"); invalid = none.
	Delegated netip.Prefix
	// Since is when IPv6CP came up.
	Since time.Time
}

// ReadIPv6 parses "<hostif>.state6". A missing file is the down state (IPv6 off or never negotiated).
func (r *Renderer) ReadIPv6(hostIf string) (IPv6State, error) {
	var st IPv6State
	b, err := os.ReadFile(filepath.Join(r.paths.StateDir, hostIf+".state6")) //nolint:gosec // StateDir is a fixed product path
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, fmt.Errorf("pppoe: read IPv6 state of %q: %w", hostIf, err)
	}
	if len(b) > maxStateFile {
		b = b[:maxStateFile]
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "phase":
			st.Up = v == "up"
		case "error":
			switch v {
			case "dhcpv6-client-unavailable", "dhcpv6-client-exited", "ipv6-setup-failed", "ipv6-observation-failed", "ipv6-state-write-failed", "ipv6-start-failed", "ipv6-stop-timeout", "ipv6-lock-timeout", "ipv6-process-identity-invalid", "ipv6-process-identity-mismatch", "ipv6-process-control-unavailable":
				st.Failure = v
			}
		case "lllocal":
			if a, err := netip.ParseAddr(v); err == nil && a.Is6() && a.IsLinkLocalUnicast() {
				st.LinkLocal = a
			}
		case "llremote":
			if a, err := netip.ParseAddr(v); err == nil && a.Is6() && a.IsLinkLocalUnicast() {
				st.PeerLinkLocal = a
			}
		case "addr":
			if p, err := netip.ParsePrefix(v); err == nil && globalV6(p.Addr()) {
				st.Addrs = append(st.Addrs, p)
			}
		case "gw":
			if a, err := netip.ParseAddr(v); err == nil && a.Is6() && !a.Is4In6() && (a.IsLinkLocalUnicast() || a.IsGlobalUnicast()) && a.Zone() == "" {
				st.Gateway = a
			}
		case "pd":
			if p, err := netip.ParsePrefix(v); err == nil && globalV6(p.Addr()) && p.Bits() >= 16 && p.Bits() <= 64 {
				st.Delegated = p.Masked()
			}
		case "at":
			if ts, err := time.Parse(time.RFC3339, v); err == nil {
				st.Since = ts
			}
		}
	}
	if !st.Up {
		// Down: nothing negotiated is current any more.
		return IPv6State{Failure: st.Failure}, nil
	}
	sort.Slice(st.Addrs, func(i, j int) bool { return st.Addrs[i].Addr().Less(st.Addrs[j].Addr()) })
	st.Addrs = dedupe(st.Addrs)
	return st, nil
}

// globalV6 is a routable IPv6 unicast address (not link-local, ULA is allowed, no mapped IPv4, no zone).
func globalV6(a netip.Addr) bool {
	return a.Is6() && !a.Is4In6() && a.Zone() == "" && a.IsGlobalUnicast()
}

func dedupe(ps []netip.Prefix) []netip.Prefix {
	out := ps[:0]
	for i, p := range ps {
		if i == 0 || p != ps[i-1] {
			out = append(out, p)
		}
	}
	return out
}

// HostAddrs are the global addresses as /128 host routes — what the agent mirrors into VPP, the way the IPv4
// address is mirrored as /32 (the on-link /64 belongs to the PPP link, not the Ethernet WAN VPP sees).
func (s IPv6State) HostAddrs() []string {
	out := make([]string, 0, len(s.Addrs))
	for _, p := range s.Addrs {
		out = append(out, netip.PrefixFrom(p.Addr(), 128).String())
	}
	return out
}

// Summary is the operator text for PppoeSessionState.ipv6: the addresses (with their on-link length) and the
// delegated prefix, e.g. "2001:db8:1::5/64, delegated 2001:db8:100::/56"; empty when nothing is assigned.
func (s IPv6State) Summary() string {
	parts := make([]string, 0, len(s.Addrs)+1)
	for _, p := range s.Addrs {
		parts = append(parts, p.String())
	}
	if s.Delegated.IsValid() {
		parts = append(parts, "delegated "+s.Delegated.String())
	}
	return strings.Join(parts, ", ")
}
