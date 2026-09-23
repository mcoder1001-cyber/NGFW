package frr

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	"ngfw/agent/internal/renderers"
)

// Strict validators for every user string that reaches frr.conf (D-049; 00-CONTEXT rule 9).
// FRR's configuration is line oriented and a `description` runs to the end of the line, so
// the attacks are an extra line ("x\nrouter bgp 65000"), a comment start ("!"/"#") that
// swallows the line, and characters FRR or frr-reload.py mis-tokenise. Each validator either
// returns the value unchanged or an error wrapping renderers.ErrUnsafe — nothing is silently
// rewritten, so the rendered file always says exactly what the operator configured.

// MaxDescriptionLen bounds interface descriptions (the schema allows more for VPP tags; FRR
// shows 80 comfortably in `show interface`).
const MaxDescriptionLen = 80

var (
	hostnameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,62}$`)
	// ifaceNameRe covers Linux interface names (IFNAMSIZ 15) and VRF device names.
	ifaceNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,15}$`)
)

// Hostname accepts an RFC 1123-style host label run: [A-Za-z0-9][A-Za-z0-9.-]{0,62}.
func Hostname(s string) (string, error) {
	if !hostnameRe.MatchString(s) {
		return "", fmt.Errorf("%w: hostname %q must match %s", renderers.ErrUnsafe, s, hostnameRe)
	}
	return s, nil
}

// IfName accepts a Linux interface name: [A-Za-z0-9_.-]{1,15}, not "." or "..", and not
// something FRR parses as an address (RF-1 review M1: "10.12.1.1" as an interface name
// becomes a gateway in `ip route`).
func IfName(s string) (string, error) {
	if !ifaceNameRe.MatchString(s) || s == "." || s == ".." {
		return "", fmt.Errorf("%w: interface name %q must match %s", renderers.ErrUnsafe, s, ifaceNameRe)
	}
	if _, err := netip.ParseAddr(s); err == nil {
		return "", fmt.Errorf("%w: interface name %q parses as an IP address", renderers.ErrUnsafe, s)
	}
	if _, err := netip.ParsePrefix(s); err == nil {
		return "", fmt.Errorf("%w: interface name %q parses as a prefix", renderers.ErrUnsafe, s)
	}
	return s, nil
}

// routeKeywords are the keywords of FRR 10.7's `ip route` / `ipv6 route` grammar that may
// follow the prefix or next hop. FRR accepts any unique abbreviation ("bl" = blackhole), so a
// next-hop interface name equal to (case-insensitively) or a prefix of any of them would be
// read as the keyword: "Null0"/"blackhole" silently turn the route into a drop.
var routeKeywords = []string{
	"blackhole", "reject", "null0", "tag", "label", "table", "vrf", "nexthop-vrf",
	"onlink", "color", "segments", "bfd", "track", "weight",
}

// RouteIfName is IfName plus the `ip route` keyword rule, for next-hop interfaces.
func RouteIfName(s string) (string, error) {
	if _, err := IfName(s); err != nil {
		return "", err
	}
	l := strings.ToLower(s)
	for _, k := range routeKeywords {
		if strings.HasPrefix(k, l) {
			return "", fmt.Errorf("%w: next-hop interface %q is (an abbreviation of) the ip route keyword %q", renderers.ErrUnsafe, s, k)
		}
	}
	return s, nil
}

// VRFName accepts a VRF name with the same rule as IfName (FRR VRFs are Linux VRF devices).
// "default" is valid but never rendered as a `vrf` block.
func VRFName(s string) (string, error) {
	if !ifaceNameRe.MatchString(s) || s == "." || s == ".." {
		return "", fmt.Errorf("%w: vrf name %q must match %s", renderers.ErrUnsafe, s, ifaceNameRe)
	}
	return s, nil
}

// Description accepts printable ASCII (0x20–0x7e), 1..MaxDescriptionLen bytes, without
// leading, trailing or doubled blanks and not starting with a comment character ('!' or '#'). The
// text is written verbatim after `description `: FRR takes the rest of the line as one LINE
// token, so quotes, semicolons and the like are inert data there (nothing is ever passed to
// a shell), and the line check rules out a second line. '|' is rejected (RF-1 review H1):
// FRR's CLI pipe hook cuts every config line at "| " (inside the daemons and vtysh), so the
// description would be truncated or dropped while vtysh -C and frr-reload.py report success.
// Every LINE-type token a protocol section renders must obey the same rule.
func Description(s string) (string, error) {
	switch {
	case s == "":
		return "", fmt.Errorf("%w: description is empty", renderers.ErrUnsafe)
	case len(s) > MaxDescriptionLen:
		return "", fmt.Errorf("%w: description is %d bytes, max %d", renderers.ErrUnsafe, len(s), MaxDescriptionLen)
	case strings.TrimSpace(s) != s:
		return "", fmt.Errorf("%w: description %q has leading or trailing blanks", renderers.ErrUnsafe, s)
	case s[0] == '!' || s[0] == '#':
		return "", fmt.Errorf("%w: description %q starts with a comment character", renderers.ErrUnsafe, s)
	case strings.Contains(s, "|"):
		return "", fmt.Errorf("%w: description %q contains '|' (FRR's CLI pipe)", renderers.ErrUnsafe, s)
	case strings.Contains(s, "  "):
		// FRR joins the LINE tokens with single blanks; a double blank would never match the
		// running config and frr-reload.py would re-apply it forever.
		return "", fmt.Errorf("%w: description %q contains consecutive blanks", renderers.ErrUnsafe, s)
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return "", fmt.Errorf("%w: description %q contains byte 0x%02x (printable ASCII only)", renderers.ErrUnsafe, s, s[i])
		}
	}
	return s, nil
}

// funcs are the FRR-specific template helpers, added on top of renderers.Funcs.
func funcs() map[string]any {
	return map[string]any{
		"hostname": Hostname,
		"ifname":   IfName,
		"rtifname": RouteIfName,
		"vrfname":  VRFName,
		"desc":     Description,
	}
}
