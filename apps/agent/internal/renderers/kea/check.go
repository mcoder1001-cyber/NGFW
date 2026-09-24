package kea

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"ngfw/agent/internal/renderers"
)

// ErrInvalid is wrapped by every error about the desired state (a value the renderer refuses
// to put into a Kea config). The schema should have caught it first; the renderer checks
// again because it is the last line before a daemon (defence in depth, D-049).
var ErrInvalid = errors.New("kea: invalid desired state")

func invalid(path, format string, a ...any) error {
	return fmt.Errorf("%w: %s: %s", ErrInvalid, path, fmt.Sprintf(format, a...))
}

var (
	// Linux interface name (IFNAMSIZ 15), as used by Kea's interfaces-config.
	ifNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,14}$`)
	// objectName in packages/schema (record keys: server, subnet, reservation names; VRFs).
	objectNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)
	macRe        = regexp.MustCompile(`^[0-9a-fA-F]{2}([:-])(?:[0-9a-fA-F]{2}[:-]){4}[0-9a-fA-F]{2}$`)
	duidRe       = regexp.MustCompile(`^(?:[0-9a-fA-F]{2}:){1,129}[0-9a-fA-F]{2}$`)
	hostnameRe   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
	printableRe  = regexp.MustCompile(`^[ -~]+$`)
	hexDataRe    = regexp.MustCompile(`^0x(?:[0-9a-fA-F]{2})+$`)
)

// Limits on user text (schema: description ≤ 255 characters; option data printable ASCII).
const (
	maxDescription  = 255
	maxOptionData4  = 255
	maxOptionData6  = 1024
	maxHostnameLen  = 253
	maxInterfaceCnt = 64
)

func checkName(path, s string) error {
	if !objectNameRe.MatchString(s) {
		return invalid(path, "name %q must match %s", s, objectNameRe)
	}
	return nil
}

func checkDescription(path, s string) (string, error) {
	if utf8.RuneCountInString(s) > maxDescription {
		return "", invalid(path, "description longer than %d characters", maxDescription)
	}
	if _, err := renderers.Line(s); err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrInvalid, path, err)
	}
	return asciiText(s), nil
}

// asciiText makes validated free text safe for Kea's JSON parser, which is byte-oriented:
// Kea 3.0 re-emits every non-ASCII byte as its own \u00XX escape, so UTF-8 text does not
// round-trip through config-get. Backslash becomes "\\" and every rune outside printable
// ASCII becomes "\uXXXX" / "\UXXXXXXXX" (Go/JSON escape syntax as plain text), so the text
// is pure printable ASCII and DecodeText restores the original exactly.
func asciiText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r >= ' ' && r <= '~':
			b.WriteRune(r)
		case r <= 0xFFFF:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
	}
	return b.String()
}

// DecodeText reverses asciiText (for Retrieve consumers reading user-context descriptions).
func DecodeText(s string) (string, error) {
	return strconv.Unquote(`"` + strings.ReplaceAll(s, `"`, `\"`) + `"`)
}

func checkHostname(path, s string) (string, error) {
	if len(s) > maxHostnameLen || !hostnameRe.MatchString(s) {
		return "", invalid(path, "hostname %q is not an RFC 1123 host name", s)
	}
	return strings.ToLower(s), nil
}

func checkMAC(path, s string) (string, error) {
	if !macRe.MatchString(s) {
		return "", invalid(path, "MAC %q must look like aa:bb:cc:dd:ee:ff", s)
	}
	return strings.ToLower(strings.ReplaceAll(s, "-", ":")), nil
}

func checkDUID(path, s string) (string, error) {
	if !duidRe.MatchString(s) {
		return "", invalid(path, "DUID %q must be colon-separated hex bytes", s)
	}
	return strings.ToLower(s), nil
}

func checkAddr(path, s string, family int) (netip.Addr, error) {
	a, err := netip.ParseAddr(s)
	if err != nil || a.Zone() != "" {
		return netip.Addr{}, invalid(path, "%q is not an IP address", s)
	}
	a = a.Unmap()
	if (family == 4) != a.Is4() {
		return netip.Addr{}, invalid(path, "%s is not an IPv%d address", a, family)
	}
	return a, nil
}

func checkOptionData(path, s string, family int) error {
	limit := maxOptionData4
	if family == 6 {
		limit = maxOptionData6
	}
	if len(s) == 0 || len(s) > limit || !printableRe.MatchString(s) {
		return invalid(path, "option data must be 1..%d printable ASCII characters", limit)
	}
	return nil
}

// checkInterface validates one Linux interface name and the test-scope prefix.
func (r *Renderer) checkInterface(path, name string) error {
	if !ifNameRe.MatchString(name) {
		return invalid(path, "interface %q is not a Linux interface name (map VPP names with WithInterfaceMapper)", name)
	}
	if r.paths.InterfacePrefix != "" && !strings.HasPrefix(name, r.paths.InterfacePrefix) {
		return invalid(path, "interface %q is outside the test scope %q*", name, r.paths.InterfacePrefix)
	}
	return nil
}
