package vppstartup

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// HostSources says where ReadHost finds the facts. Paths under Root are the kernel's (/sys,
// /proc); tests point Root at a fake tree.
type HostSources struct {
	// Root prefixes /sys and /proc ("" = "/").
	Root string
	// PluginDir is VPP's plugin directory (absolute, not under Root).
	PluginDir string
	// CurrentConf is the current start-up file whose plugin switches are kept for plugins the
	// document does not mention ("" = there is none: no switches). A missing file is an error.
	CurrentConf string
	// MgmtIfaces are extra kernel interfaces to protect besides the default-route interface(s).
	MgmtIfaces []string
	// MgmtPCI are extra management PCI addresses (e.g. a NIC reached through a non-default route).
	MgmtPCI []string
	// ControlPorts are the local TCP ports of control connections (sshd, the agent's API) whose
	// established sessions mark the NIC they arrive on as management (nil = DefaultControlPorts).
	ControlPorts []uint16
}

// DefaultControlPorts are the control ports used when HostSources.ControlPorts is nil: sshd.
var DefaultControlPorts = []uint16{22}

// DefaultPluginDir is where the vpp-plugin-* packages install plugins on Ubuntu.
const DefaultPluginDir = "/usr/lib/x86_64-linux-gnu/vpp_plugins"

// ReadHost collects the Host facts of a running system: online/isolated CPUs, NUMA nodes,
// hugepages, on-disk plugins, the current plugin switches and the management NIC(s) (see
// managementNICs). It never guesses: a fact it cannot read is an error; callers run Host.Check (as
// BuildModel does) after applying any overrides.
func ReadHost(src HostSources) (Host, error) {
	root := src.Root
	if root == "" {
		root = "/"
	}
	at := func(p string) string { return filepath.Join(root, p) }
	var h Host

	online, err := os.ReadFile(at("sys/devices/system/cpu/online")) //nolint:gosec // fixed sysfs path under the configured root
	if err != nil {
		return h, fmt.Errorf("%w: %v", ErrHost, err)
	}
	if h.OnlineCPUs, err = ParseCPUList(string(online)); err != nil {
		return h, fmt.Errorf("%w: online CPUs: %v", ErrHost, err)
	}
	if b, err := os.ReadFile(at("sys/devices/system/cpu/isolated")); err == nil { //nolint:gosec // fixed sysfs path
		if h.IsolCPUs, err = ParseCPUList(string(b)); err != nil {
			return h, fmt.Errorf("%w: isolated CPUs: %v", ErrHost, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return h, fmt.Errorf("%w: %v", ErrHost, err)
	}
	nodes, _ := filepath.Glob(at("sys/devices/system/node/node[0-9]*"))
	h.NUMANodes = max(len(nodes), 1)                // no node directory = a non-NUMA kernel: one node
	meminfo, err := os.ReadFile(at("proc/meminfo")) //nolint:gosec // fixed procfs path
	if err != nil {
		return h, fmt.Errorf("%w: %v", ErrHost, err)
	}
	h.HugepageBytes = HugepagesFromMeminfo(string(meminfo))

	dir := src.PluginDir
	if dir == "" {
		dir = DefaultPluginDir
	}
	plugins, _ := filepath.Glob(filepath.Join(dir, "*.so"))
	for _, p := range plugins {
		h.Plugins = append(h.Plugins, filepath.Base(p))
	}
	if len(h.Plugins) == 0 {
		return h, fmt.Errorf("%w: no plugins found in %s", ErrHost, dir)
	}

	h.CurrentPlugins = map[string]bool{}
	if src.CurrentConf != "" {
		b, err := os.ReadFile(src.CurrentConf) //nolint:gosec // operator-supplied path of the current start-up file, read only
		if err != nil {
			return h, fmt.Errorf("%w: current start-up file: %v", ErrHost, err)
		}
		if h.CurrentPlugins, err = PluginSwitches(b); err != nil {
			return h, fmt.Errorf("%w: current start-up file %s: %v", ErrHost, src.CurrentConf, err)
		}
	}

	mgmt, notes, err := managementNICs(at, src)
	if err != nil {
		return h, err
	}
	h.ManagementNotes = notes
	for p := range mgmt {
		h.ManagementPCI = append(h.ManagementPCI, p)
	}
	slices.Sort(h.ManagementPCI)
	return h, nil
}

// managementNICs returns the PCI addresses of every kernel NIC the host is managed through:
//
//   - every interface carrying an IPv4 or IPv6 default route (/proc/net/route, /proc/net/ipv6_route);
//   - every interface the route lookup picks for the peer of an established control connection
//     (/proc/net/tcp{,6}, local port in src.ControlPorts — sshd by default), i.e. also a management
//     subnet that is only directly connected;
//   - src.MgmtIfaces and src.MgmtPCI.
//
// Interfaces are resolved via /sys/class/net/<if>/device. A tun/tap interface without a device
// (a linux-cp tap: VPP-owned, not a kernel NIC) and unreachable/blackhole routes (iface "*") are
// skipped with a note — they must not stop rendering on a running router; the other rules still
// apply. Any other interface without a PCI device (bond, VLAN, bridge) is an error: name the NIC
// with --mgmt-pci.
func managementNICs(at func(string) string, src HostSources) (map[string]bool, []string, error) {
	why := map[string]string{} // interface → reason
	add := func(ifname, reason string) {
		if ifname == "" || ifname == "*" || ifname == "lo" {
			return
		}
		if _, seen := why[ifname]; !seen {
			why[ifname] = reason
		}
	}
	v4, err := readRoutes4(at)
	if err != nil {
		return nil, nil, err
	}
	v6, err := readRoutes6(at)
	if err != nil {
		return nil, nil, err
	}
	for _, r := range append(slices.Clone(v4), v6...) {
		if r.prefix.Bits() == 0 {
			add(r.iface, "default route")
		}
	}
	ports := src.ControlPorts
	if ports == nil {
		ports = DefaultControlPorts
	}
	peers, err := controlPeers(at, ports)
	if err != nil {
		return nil, nil, err
	}
	for _, peer := range peers {
		table := v4
		if peer.Is6() {
			table = v6
		}
		if r, ok := lookup(table, peer); ok {
			add(r.iface, "control connection from "+peer.String())
		}
	}
	for _, i := range src.MgmtIfaces {
		add(i, "--mgmt-if")
	}

	mgmt := map[string]bool{}
	var notes []string
	var unresolved []string
	names := slices.Sorted(maps.Keys(why))
	for _, ifname := range names {
		pcis, vppOwned, err := ifacePCIs(at, ifname, 0)
		switch {
		case err != nil:
			unresolved = append(unresolved, fmt.Sprintf("%s (%s): %v", ifname, why[ifname], err))
		case vppOwned:
			notes = append(notes, fmt.Sprintf("%s (%s) is a tun/tap interface (linux-cp, VPP-owned), not a kernel NIC: not protected", ifname, why[ifname]))
		default:
			for _, pci := range pcis {
				mgmt[pci] = true
			}
			notes = append(notes, fmt.Sprintf("%s → %s (%s)", ifname, strings.Join(pcis, ","), why[ifname]))
		}
	}
	// an interface we cannot resolve to PCI NICs is only acceptable when the operator named the
	// management NIC(s) explicitly
	if len(unresolved) > 0 && len(src.MgmtPCI) == 0 {
		return nil, nil, fmt.Errorf("%w: %s", ErrHost, strings.Join(unresolved, "; "))
	}
	for _, u := range unresolved {
		notes = append(notes, "unresolved "+u+" — covered by --mgmt-pci")
	}
	for _, p := range src.MgmtPCI {
		pci, err := PCIAddress(p)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: --mgmt-pci: %v", ErrHost, err)
		}
		mgmt[pci] = true
		notes = append(notes, pci+" (--mgmt-pci)")
	}
	return mgmt, notes, nil
}

type route struct {
	prefix netip.Prefix
	iface  string
	metric uint32
}

// lookup is a longest-prefix match (lowest metric on ties).
func lookup(table []route, addr netip.Addr) (route, bool) {
	best, found := route{}, false
	for _, r := range table {
		if r.iface == "*" || !r.prefix.Contains(addr) {
			continue
		}
		if !found || r.prefix.Bits() > best.prefix.Bits() || (r.prefix.Bits() == best.prefix.Bits() && r.metric < best.metric) {
			best, found = r, true
		}
	}
	return best, found
}

// readRoutes4 parses /proc/net/route (little-endian hex destination/mask), usable (RTF_UP) routes
// only; the "*" interface (unreachable/blackhole) is kept so callers can skip it explicitly.
func readRoutes4(at func(string) string) ([]route, error) {
	b, err := os.ReadFile(at("proc/net/route")) //nolint:gosec // fixed procfs path
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHost, err)
	}
	var out []route
	for _, line := range strings.Split(string(b), "\n")[1:] {
		f := strings.Fields(line)
		// Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT
		if len(f) < 8 {
			continue
		}
		dst, ok1 := hexLE4(f[1])
		mask, ok2 := hexLE4(f[7])
		flags, err1 := strconv.ParseUint(f[3], 16, 16)
		metric, err2 := strconv.ParseUint(f[6], 10, 32)
		if !ok1 || !ok2 || err1 != nil || err2 != nil || flags&0x1 == 0 {
			continue
		}
		bits := 0
		for _, by := range mask.As4() {
			for ; by != 0; by <<= 1 {
				bits++
			}
		}
		out = append(out, route{prefix: netip.PrefixFrom(dst, bits).Masked(), iface: f[0], metric: uint32(metric)})
	}
	return out, nil
}

// readRoutes6 parses /proc/net/ipv6_route (dest destlen src srclen nexthop metric refcnt use flags
// iface), skipping reject routes and routes on "lo".
func readRoutes6(at func(string) string) ([]route, error) {
	b, err := os.ReadFile(at("proc/net/ipv6_route")) //nolint:gosec // fixed procfs path
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHost, err)
	}
	var out []route
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) != 10 || f[9] == "lo" {
			continue
		}
		dst, ok := hex16(f[0])
		plen, err1 := strconv.ParseUint(f[1], 16, 8)
		metric, err2 := strconv.ParseUint(f[5], 16, 32)
		flags, err3 := strconv.ParseUint(f[8], 16, 32)
		if !ok || err1 != nil || err2 != nil || err3 != nil || plen > 128 || flags&0x0200 != 0 { // RTF_REJECT
			continue
		}
		out = append(out, route{prefix: netip.PrefixFrom(dst, int(plen)).Masked(), iface: f[9], metric: uint32(metric)})
	}
	return out, nil
}

// controlPeers returns the remote addresses of ESTABLISHED TCP connections whose local port is
// one of ports (/proc/net/tcp and /proc/net/tcp6); loopback peers are ignored.
func controlPeers(at func(string) string, ports []uint16) ([]netip.Addr, error) {
	seen := map[netip.Addr]bool{}
	for _, file := range []string{"proc/net/tcp", "proc/net/tcp6"} {
		b, err := os.ReadFile(at(file)) //nolint:gosec // fixed procfs path
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrHost, err)
		}
		for _, line := range strings.Split(string(b), "\n")[1:] {
			f := strings.Fields(line)
			// sl local_address rem_address st ...
			if len(f) < 4 || f[3] != "01" {
				continue
			}
			_, lport, ok1 := hexSockAddr(f[1])
			raddr, _, ok2 := hexSockAddr(f[2])
			if !ok1 || !ok2 || !slices.Contains(ports, lport) {
				continue
			}
			raddr = raddr.Unmap()
			if raddr.IsLoopback() || raddr.IsUnspecified() {
				continue
			}
			seen[raddr] = true
		}
	}
	return slices.SortedFunc(maps.Keys(seen), func(a, b netip.Addr) int { return a.Compare(b) }), nil
}

// hexSockAddr parses "0100007F:0016" (IPv4, little-endian) or the 32-hex-digit IPv6 form
// (four little-endian 32-bit words) with its hex port.
func hexSockAddr(s string) (netip.Addr, uint16, bool) {
	host, port, ok := strings.Cut(s, ":")
	if !ok {
		return netip.Addr{}, 0, false
	}
	p, err := strconv.ParseUint(port, 16, 16)
	if err != nil {
		return netip.Addr{}, 0, false
	}
	switch len(host) {
	case 8:
		a, ok := hexLE4(host)
		return a, uint16(p), ok
	case 32:
		var b [16]byte
		for w := 0; w < 4; w++ {
			v, err := strconv.ParseUint(host[w*8:w*8+8], 16, 32)
			if err != nil {
				return netip.Addr{}, 0, false
			}
			b[w*4], b[w*4+1], b[w*4+2], b[w*4+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24) //nolint:gosec // v < 2^32, truncation to bytes intended
		}
		return netip.AddrFrom16(b), uint16(p), true
	}
	return netip.Addr{}, 0, false
}

// hexLE4 parses an IPv4 address written as a little-endian 32-bit hex number ("0100007F" = 127.0.0.1).
func hexLE4(s string) (netip.Addr, bool) {
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil || len(s) != 8 {
		return netip.Addr{}, false
	}
	return netip.AddrFrom4([4]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}), true //nolint:gosec // v < 2^32, truncation to bytes intended
}

// hex16 parses 32 hex digits in network order (the /proc/net/ipv6_route form).
func hex16(s string) (netip.Addr, bool) {
	if len(s) != 32 {
		return netip.Addr{}, false
	}
	var b [16]byte
	for i := range 16 {
		v, err := strconv.ParseUint(s[2*i:2*i+2], 16, 8)
		if err != nil {
			return netip.Addr{}, false
		}
		b[i] = byte(v)
	}
	return netip.AddrFrom16(b), true
}

// ifacePCIs resolves an interface to the PCI NIC(s) underneath: /sys/class/net/<if>/device, or —
// for a bond, VLAN or other stacked device — the lower_* links recursively. vppOwned reports a
// tun/tap interface without a device (a linux-cp tap created by VPP).
func ifacePCIs(at func(string) string, ifname string, depth int) (pcis []string, vppOwned bool, err error) {
	if ifname == "" || ifname == "." || ifname == ".." || strings.ContainsAny(ifname, "/\x00") || len(ifname) > 15 {
		return nil, false, fmt.Errorf("bad interface name %q", ifname)
	}
	if depth > 4 {
		return nil, false, fmt.Errorf("interface stack too deep")
	}
	dir := at(filepath.Join("sys/class/net", ifname))
	if target, err := os.Readlink(filepath.Join(dir, "device")); err == nil {
		pci, err := PCIAddress(filepath.Base(target))
		if err != nil {
			return nil, false, err
		}
		return []string{pci}, false, nil
	}
	if _, err := os.Stat(filepath.Join(dir, "tun_flags")); err == nil {
		return nil, true, nil
	}
	lowers, _ := filepath.Glob(filepath.Join(dir, "lower_*"))
	for _, l := range lowers {
		sub, owned, err := ifacePCIs(at, strings.TrimPrefix(filepath.Base(l), "lower_"), depth+1)
		if err != nil {
			return nil, false, err
		}
		if owned {
			continue
		}
		pcis = append(pcis, sub...)
	}
	if len(pcis) == 0 {
		return nil, false, fmt.Errorf("no PCI device underneath; name the NIC(s) with --mgmt-pci")
	}
	slices.Sort(pcis)
	return slices.Compact(pcis), false, nil
}

// HugepagesFromMeminfo returns HugePages_Total × Hugepagesize in bytes (0 if unknown).
func HugepagesFromMeminfo(s string) uint64 {
	var total, sizeKB uint64
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "HugePages_Total:":
			_, _ = fmt.Sscanf(f[1], "%d", &total)
		case "Hugepagesize:":
			_, _ = fmt.Sscanf(f[1], "%d", &sizeKB)
		}
	}
	return total * sizeKB << 10
}

// PluginSwitches extracts `plugins { plugin <file> { enable|disable } }` from a start-up file.
// A plugin section with anything but exactly one enable/disable is an error (we cannot carry over
// what we do not understand).
func PluginSwitches(conf []byte) (map[string]bool, error) {
	root, err := Parse(conf)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, sec := range root.Sections {
		if sec.Name != "plugins" {
			continue
		}
		for _, p := range sec.Sections {
			file, ok := strings.CutPrefix(p.Name, "plugin ")
			if !ok || len(p.Entries) != 1 || len(p.Sections) != 0 || (p.Entries[0] != "enable" && p.Entries[0] != "disable") {
				return nil, fmt.Errorf("unsupported plugins entry %q", p.Name)
			}
			out[file] = p.Entries[0] == "enable"
		}
		if len(sec.Entries) != 0 {
			return nil, fmt.Errorf("unsupported plugins entry %q (path/add-path are not generated)", sec.Entries[0])
		}
	}
	return out, nil
}
