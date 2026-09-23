package vppstartup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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
}

// DefaultPluginDir is where the vpp-plugin-* packages install plugins on Ubuntu.
const DefaultPluginDir = "/usr/lib/x86_64-linux-gnu/vpp_plugins"

// ReadHost collects the Host facts of a running system: online/isolated CPUs, NUMA nodes,
// hugepages, on-disk plugins, the current plugin switches and the management NIC(s) — the PCI
// devices behind the interfaces that carry an IPv4/IPv6 default route (plus src.MgmtIfaces /
// src.MgmtPCI). It never guesses: a fact it cannot read is an error; callers run Host.Check (as
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

	ifaces, err := defaultRouteIfaces(at)
	if err != nil {
		return h, err
	}
	for _, i := range src.MgmtIfaces {
		if !slices.Contains(ifaces, i) {
			ifaces = append(ifaces, i)
		}
	}
	mgmt := map[string]bool{}
	for _, ifname := range ifaces {
		pci, err := ifacePCI(at, ifname)
		if err != nil {
			return h, err
		}
		mgmt[pci] = true
	}
	for _, p := range src.MgmtPCI {
		pci, err := PCIAddress(p)
		if err != nil {
			return h, fmt.Errorf("%w: --mgmt-pci: %v", ErrHost, err)
		}
		mgmt[pci] = true
	}
	for p := range mgmt {
		h.ManagementPCI = append(h.ManagementPCI, p)
	}
	slices.Sort(h.ManagementPCI)
	return h, nil
}

// defaultRouteIfaces returns the interfaces carrying an IPv4 or IPv6 default route (main table,
// /proc/net/route and /proc/net/ipv6_route), sorted, without "lo".
func defaultRouteIfaces(at func(string) string) ([]string, error) {
	set := map[string]bool{}
	if b, err := os.ReadFile(at("proc/net/route")); err == nil { //nolint:gosec // fixed procfs path
		for _, line := range strings.Split(string(b), "\n")[1:] {
			f := strings.Fields(line)
			// Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT
			if len(f) >= 8 && f[1] == "00000000" && f[7] == "00000000" {
				set[f[0]] = true
			}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %v", ErrHost, err)
	}
	if b, err := os.ReadFile(at("proc/net/ipv6_route")); err == nil { //nolint:gosec // fixed procfs path
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			// dest destlen src srclen nexthop metric refcnt use flags iface
			if len(f) == 10 && f[0] == strings.Repeat("0", 32) && f[1] == "00" && f[9] != "lo" {
				set[f[9]] = true
			}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %v", ErrHost, err)
	}
	delete(set, "lo")
	out := make([]string, 0, len(set))
	for i := range set {
		out = append(out, i)
	}
	slices.Sort(out)
	return out, nil
}

// ifacePCI resolves /sys/class/net/<if>/device to its PCI address.
func ifacePCI(at func(string) string, ifname string) (string, error) {
	if ifname == "" || ifname == "." || ifname == ".." || strings.ContainsAny(ifname, "/\x00") || len(ifname) > 15 {
		return "", fmt.Errorf("%w: bad interface name %q", ErrHost, ifname)
	}
	target, err := os.Readlink(at(filepath.Join("sys/class/net", ifname, "device")))
	if err != nil {
		return "", fmt.Errorf("%w: management interface %s has no PCI device (%v); name the NIC with --mgmt-pci", ErrHost, ifname, err)
	}
	pci, err := PCIAddress(filepath.Base(target))
	if err != nil {
		return "", fmt.Errorf("%w: management interface %s: %v", ErrHost, ifname, err)
	}
	return pci, nil
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
