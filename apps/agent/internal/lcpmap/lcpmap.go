// Package lcpmap is the linux-cp glue between the configuration document and FRR (P12):
//
//   - HostName: the Linux name of an interface's linux-cp pair (`interfaces.<n>.lcp.hostIfName`, default the VPP name);
//   - Mapper: the VPP → Linux interface mapping the FRR renderer uses (frr.WithInterfaceMapper), built from the
//     desired state the renderer is about to apply — never from VPP runtime state, so a render is a pure function of
//     its input;
//   - the `lcp-addresses` interface-line producer (frr.RegisterInterfaceLines, seam S2): FRR's zebra puts the VPP
//     interface's addresses on its Linux pair (`interface <tap> / ip address …`), because VPP copies them only at pair
//     creation time with `lcp lcp-sync` off (a VPP global; P12-questions Q5). linux-nl mirrors them back into VPP,
//     where they already exist.
package lcpmap

import (
	"fmt"
	"net/netip"
	"slices"
	"sync"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
)

// LinesName is the name of the interface-line producer.
const LinesName = "lcp-addresses"

func init() { frr.RegisterInterfaceLines(LinesName, AddressLines) }

// HostName returns the Linux name of vppName's pair: lcp.hostIfName, else vppName itself; either must be a valid Linux
// interface name for FRR (frr.IfName: ≤ 15 bytes of [A-Za-z0-9_.-], not an address).
func HostName(vppName string, lcp *vrxv1.InterfaceLcp) (string, error) {
	name := vppName
	if lcp.GetHostIfName() != "" {
		name = lcp.GetHostIfName()
	}
	if _, err := frr.IfName(name); err != nil {
		if lcp.GetHostIfName() == "" {
			return "", fmt.Errorf("%w: interface %q is no valid Linux name: set lcp.hostIfName", renderers.ErrUnsafe, vppName)
		}
		return "", err
	}
	return name, nil
}

// HostType returns lcp.hostIfType ("tap" by default).
func HostType(lcp *vrxv1.InterfaceLcp) string {
	if t := lcp.GetHostIfType(); t != "" {
		return t
	}
	return "tap"
}

// Map is a VPP → Linux name table built from a desired state.
type Map map[string]string

// FromDesired builds the table of every interface with a valid `lcp` leaf (invalid ones are left out: the projection
// reports them).
func FromDesired(ds *vrxv1.DesiredState) Map {
	m := Map{}
	for name, itf := range ds.GetInterfaces() {
		if itf.Lcp == nil {
			continue
		}
		if host, err := HostName(name, itf.GetLcp()); err == nil {
			m[name] = host
		}
	}
	return m
}

// Mapper is the FRR renderer's interface mapper; Set replaces its table before each render (the reconciler serialises
// transactions, and Map is safe for concurrent readers).
type Mapper struct {
	mu sync.RWMutex
	m  Map
}

// Set replaces the table.
func (mp *Mapper) Set(m Map) {
	mp.mu.Lock()
	mp.m = m
	mp.mu.Unlock()
}

// Map implements frr.InterfaceMapper.
func (mp *Mapper) Map(vppName string) (string, bool) {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	n, ok := mp.m[vppName]
	return n, ok
}

// AddressLines is the S2 producer: ` ip address` / ` ipv6 address` of every paired interface, on its Linux name.
// Addresses keep their host bits (an interface address, not a network); link-local IPv6 is the kernel's own.
func AddressLines(rc *frr.RenderContext) (map[string][]string, error) {
	out := map[string][]string{}
	for name, itf := range rc.Desired.GetInterfaces() {
		if itf.Lcp == nil {
			continue
		}
		linux, ok := rc.MapInterface(name)
		if !ok {
			continue
		}
		var lines []string
		for _, fam := range []struct {
			kw    string
			addrs []string
			is6   bool
		}{{"ip", itf.GetIpv4(), false}, {"ipv6", itf.GetIpv6(), true}} {
			addrs := make([]netip.Prefix, 0, len(fam.addrs))
			for _, a := range fam.addrs {
				p, err := netip.ParsePrefix(a)
				if err != nil || p.Addr().Is6() != fam.is6 || p.Addr().Zone() != "" {
					return nil, fmt.Errorf("%w: interfaces.%s: address %q", frr.ErrInput, name, a)
				}
				if fam.is6 && p.Addr().IsLinkLocalUnicast() {
					continue
				}
				addrs = append(addrs, p)
			}
			slices.SortFunc(addrs, func(a, b netip.Prefix) int {
				if c := a.Addr().Compare(b.Addr()); c != 0 {
					return c
				}
				return a.Bits() - b.Bits()
			})
			for _, p := range addrs {
				lines = append(lines, fmt.Sprintf(" %s address %s", fam.kw, p))
			}
		}
		if len(lines) > 0 {
			out[linux] = lines
		}
	}
	return out, nil
}
