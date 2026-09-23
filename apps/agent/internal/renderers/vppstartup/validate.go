package vppstartup

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"ngfw/agent/internal/renderers"
)

// PCIAddress checks a PCI address "dddd:bb:dd.f" (hex domain:bus:device, function 0-7) and
// returns it in canonical lower-case form. Anything else — short forms, whitespace, braces,
// newlines — is rejected, so the value can only ever be one token of the rendered file.
func PCIAddress(s string) (string, error) {
	if len(s) != 12 || s[4] != ':' || s[7] != ':' || s[10] != '.' {
		return "", fmt.Errorf("%w: PCI address %q is not of the form 0000:0b:00.0", renderers.ErrUnsafe, s)
	}
	for i := 0; i < len(s); i++ {
		if i == 4 || i == 7 || i == 10 {
			continue
		}
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return "", fmt.Errorf("%w: PCI address %q is not of the form 0000:0b:00.0", renderers.ErrUnsafe, s)
		}
	}
	dev, _ := strconv.ParseUint(s[8:10], 16, 8)
	if dev > 0x1f {
		return "", fmt.Errorf("%w: PCI address %q: device %s out of range 00-1f", renderers.ErrUnsafe, s, s[8:10])
	}
	if s[11] > '7' {
		return "", fmt.Errorf("%w: PCI address %q: function %c out of range 0-7", renderers.ErrUnsafe, s, s[11])
	}
	return strings.ToLower(s), nil
}

// MaxLogicalNameLen bounds logical interface names. 15 is Linux IFNAMSIZ-1: linux-cp mirrors the
// VPP interface into the kernel under the same name, so a longer name could not be paired.
const MaxLogicalNameLen = 15

// reservedNames are VPP's own interface names and dpdk-section keywords a NIC must not take.
var reservedNames = []string{"local0", "default", "none", "any", "all"}

// reservedStems are the name stems VPP (or our descriptors) use for interfaces it creates
// ("loop0", "gre1", "host-eth0", "vxlan_tunnel0"): a logical name equal to a stem, or a stem
// followed by a digit, '_' or '-', would collide with, or be mistaken for, one of them.
// ("green" or "loopback" are fine: the stem is followed by a letter.)
var reservedStems = []string{
	"loop", "tap", "tun", "host", "memif", "vxlan", "gre", "ipip", "ipsec", "wg", "bond",
	"pg", "lcp", "bvi", "vhost", "avf", "rdma", "af_xdp", "vmxnet3", "pppoe", "gtpu",
	"geneve", "lisp", "mpls", "sr", "srv6", "pipe", "punt", "span", "local", "virtio", "tapcli",
}

func hasReservedStem(s string) (string, bool) {
	for _, p := range reservedStems {
		rest, ok := strings.CutPrefix(s, p)
		if !ok {
			continue
		}
		if rest == "" || rest[0] >= '0' && rest[0] <= '9' || rest[0] == '_' || rest[0] == '-' {
			return p, true
		}
	}
	return "", false
}

// LogicalName checks a logical interface name for `dpdk { dev <pci> { name <name> } }` (D-069):
// 1..15 characters of lower-case ASCII letters, digits, '_' and '-', starting with a letter and
// not ending with '-' or '_'. No '.' (VPP's sub-interface separator), no '/' or ':' (VPP
// hardware names), no upper case (names are compared case-sensitively in the config but the UI
// and Linux treat them as one), and none of VPP's reserved names or created-interface stems.
func LogicalName(s string) error {
	if s == "" || len(s) > MaxLogicalNameLen {
		return fmt.Errorf("%w: logical name %q: length %d not in 1..%d", renderers.ErrUnsafe, s, len(s), MaxLogicalNameLen)
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case (c >= '0' && c <= '9' || c == '_' || c == '-') && i > 0:
		default:
			return fmt.Errorf("%w: logical name %q: character %q at %d not allowed (want [a-z][a-z0-9_-]*)", renderers.ErrUnsafe, s, c, i)
		}
	}
	if last := s[len(s)-1]; last == '-' || last == '_' {
		return fmt.Errorf("%w: logical name %q must not end with %q", renderers.ErrUnsafe, s, last)
	}
	if slices.Contains(reservedNames, s) {
		return fmt.Errorf("%w: logical name %q is reserved by VPP", renderers.ErrUnsafe, s)
	}
	if p, bad := hasReservedStem(s); bad {
		return fmt.Errorf("%w: logical name %q uses %q, a stem VPP uses for interfaces it creates", renderers.ErrUnsafe, s, p)
	}
	return nil
}

// PluginName checks a plugin file name as it appears in the plugin directory:
// [a-z0-9][a-z0-9_-]*_plugin.so (e.g. linux_cp_plugin.so, vxlan-gpe_plugin.so).
func PluginName(s string) error {
	const suffix = "_plugin.so"
	stem, ok := strings.CutSuffix(s, suffix)
	if !ok || stem == "" || len(s) > 64 {
		return fmt.Errorf("%w: plugin %q is not a plugin file name (<name>_plugin.so)", renderers.ErrUnsafe, s)
	}
	for i := 0; i < len(stem); i++ {
		c := stem[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || (i > 0 && (c == '_' || c == '-'))) {
			return fmt.Errorf("%w: plugin %q: character %q not allowed", renderers.ErrUnsafe, s, c)
		}
	}
	return nil
}

// FormatCPUList renders CPU ids in VPP/kernel range syntax, sorted: [3 2 6 7 8] → "2-3,6-8".
func FormatCPUList(cpus []uint32) string {
	s := slices.Clone(cpus)
	slices.Sort(s)
	s = slices.Compact(s)
	var parts []string
	for i := 0; i < len(s); {
		j := i
		for j+1 < len(s) && s[j+1] == s[j]+1 {
			j++
		}
		if j == i {
			parts = append(parts, strconv.FormatUint(uint64(s[i]), 10))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", s[i], s[j]))
		}
		i = j + 1
	}
	return strings.Join(parts, ",")
}

// ParseCPUList parses kernel range syntax ("0-3,8,10-11"; empty = none). Used for
// /sys/devices/system/cpu/{online,isolated} and the CLI's --isolcpus.
func ParseCPUList(s string) ([]uint32, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out []uint32
	for _, part := range strings.Split(s, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.ParseUint(lo, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("cpu list %q: %v", s, err)
		}
		b := a
		if isRange {
			if b, err = strconv.ParseUint(hi, 10, 16); err != nil {
				return nil, fmt.Errorf("cpu list %q: %v", s, err)
			}
		}
		if b < a || b > 4095 {
			return nil, fmt.Errorf("cpu list %q: bad range %s", s, part)
		}
		for c := a; c <= b; c++ {
			out = append(out, uint32(c))
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}
