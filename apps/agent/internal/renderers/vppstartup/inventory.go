package vppstartup

// PCI inventory is a read-only prerequisite for appliance boot preparation. It does not change
// the generator's defaults, bind drivers, or claim that a device has a compatible DPDK PMD.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// PCIInventory describes PCI network controllers, including devices already bound to VFIO
// which therefore have no Linux interface. Non-PCI NICs are outside this inventory.
type PCIInventory struct {
	Devices         []PCINetworkDevice
	ManagementPCI   []string
	ManagementNotes []string
}

// PCINetworkDevice contains observed facts, never inferred driver compatibility. Before any
// transfer, a caller must separately establish PMD support, driver availability and resource
// requirements, and re-read the inventory immediately before acting on it.
type PCINetworkDevice struct {
	PCI          string
	Class        uint32
	VendorID     uint16
	DeviceID     uint16
	Driver       string
	Interfaces   []string
	IOMMUGroup   string
	IOMMUMembers []string
	Management   bool
	Issues       []PCIInventoryIssue
}

// PCIInventoryIssue is a reason a device must not be transferred automatically. Code is stable
// for consumers; Message supplies the observed detail. Unknown or incomplete facts fail closed.
type PCIInventoryIssue struct {
	Code    string
	Message string
}

// TopologyEligible means the snapshot has no known ownership/topology exclusion. It does NOT
// establish DPDK hardware support or authorize binding the device.
func (d PCINetworkDevice) TopologyEligible() bool {
	return d.PCI != "" && d.Class>>8 == 0x0200 && d.IOMMUGroup != "" && len(d.Issues) == 0 && !d.Management
}

func (d *PCINetworkDevice) issue(code, message string) {
	d.Issues = append(d.Issues, PCIInventoryIssue{Code: code, Message: message})
}

// ReadPCIInventory uses the same management discovery as ReadHost: default routes, control
// connections and explicit HostSources.MgmtIfaces/MgmtPCI. Only Root and those management
// settings are used; plugins, hugepages and startup.conf are not prerequisites for inventory.
// A missing management identity, absent management PCI device, or incomplete PCI enumeration
// is an ErrHost error and returns no usable inventory. Other exclusions are attached to each
// device, allowing callers to explain why an interface was left untouched.
//
// Every shared IOMMU group is excluded, even if all its members are Ethernet controllers:
// assigning several functions as a group requires a separate, explicit ownership decision.
func ReadPCIInventory(src HostSources) (PCIInventory, error) {
	var out PCIInventory
	root := src.Root
	if root == "" {
		root = "/"
	}
	at := func(p string) string { return filepath.Join(root, p) }
	mgmt, notes, err := managementNICs(at, src)
	if err != nil {
		return out, err
	}
	if len(mgmt) == 0 {
		return out, fmt.Errorf("%w: PCI inventory requires a known management NIC", ErrHost)
	}
	dir := at("sys/bus/pci/devices")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out, fmt.Errorf("%w: enumerate PCI devices: %v", ErrHost, err)
	}
	classes := map[string]uint32{}
	paths := map[string]string{}
	groups := map[string]string{}
	for _, entry := range entries {
		pci, err := PCIAddress(entry.Name())
		if err != nil {
			return PCIInventory{}, fmt.Errorf("%w: PCI inventory: %v", ErrHost, err)
		}
		if _, duplicate := paths[pci]; duplicate {
			return PCIInventory{}, fmt.Errorf("%w: duplicate PCI device %s", ErrHost, pci)
		}
		path := filepath.Join(dir, entry.Name())
		class, err := readPCIHex(filepath.Join(path, "class"), 6)
		if err != nil {
			return PCIInventory{}, fmt.Errorf("%w: PCI device %s class: %v", ErrHost, pci, err)
		}
		classes[pci], paths[pci] = class, path
		groups[pci], _ = pciIOMMUGroup(path)
	}
	for pci := range mgmt {
		out.ManagementPCI = append(out.ManagementPCI, pci)
	}
	slices.Sort(out.ManagementPCI)
	for _, pci := range out.ManagementPCI {
		class, exists := classes[pci]
		if !exists || class>>16 != 0x02 {
			return PCIInventory{}, fmt.Errorf("%w: management PCI %s is not a present network controller", ErrHost, pci)
		}
	}
	out.ManagementNotes = notes
	for _, entry := range entries { // ReadDir sorts by name; sort canonical addresses again below.
		pci, _ := PCIAddress(entry.Name())
		class := classes[pci]
		if class>>16 != 0x02 {
			continue
		}
		d := PCINetworkDevice{PCI: pci, Class: class, Management: mgmt[pci]}
		path := paths[pci]
		if d.Management {
			d.issue("management-nic", "the management network controller is protected")
		}
		if class>>8 != 0x0200 {
			d.issue("unsupported-class", fmt.Sprintf("PCI network class %#06x is not Ethernet", class))
		}
		for _, fact := range []struct {
			name string
			dest *uint16
		}{{"vendor", &d.VendorID}, {"device", &d.DeviceID}} {
			value, err := readPCIHex(filepath.Join(path, fact.name), 4)
			if err != nil {
				d.issue("invalid-"+fact.name, err.Error())
			} else if fact.name == "vendor" && (value == 0 || value == 0xffff) {
				d.issue("invalid-vendor", "the PCI vendor ID is reserved or the device is no longer present")
			} else {
				*fact.dest = uint16(value) //nolint:gosec // readPCIHex limits this value to four hex digits
			}
		}
		link, err := os.Readlink(filepath.Join(path, "driver"))
		if err != nil {
			d.issue("unknown-driver", fmt.Sprintf("cannot identify the current PCI driver: %v", err))
		} else {
			d.Driver = filepath.Base(link)
			if d.Driver == "mlx4_core" || d.Driver == "mlx5_core" {
				d.issue("shared-kernel-driver", "this controller uses a shared kernel driver; exclusive PCI transfer needs a hardware-specific plan")
			}
		}
		netdevs, err := readPCINetworkInterfaces(path)
		if err != nil {
			d.issue("unreadable-interfaces", err.Error())
		}
		d.Interfaces = netdevs
		switch {
		case len(d.Interfaces) > 1:
			d.issue("multiple-interfaces", "one PCI function exposes multiple Linux interfaces; port ownership is ambiguous")
		case len(d.Interfaces) == 0 && d.Driver != "vfio-pci" && d.Driver != "igb_uio" && d.Driver != "uio_pci_generic":
			d.issue("missing-interface", "the controller has no Linux interface and is not bound to a recognized userspace driver")
		}
		// Rebinding a physical function with live VFs may disrupt management in another group.
		functions, err := os.ReadDir(path)
		if err != nil {
			d.issue("unreadable-functions", err.Error())
		} else {
			for _, function := range functions {
				if strings.HasPrefix(function.Name(), "virtfn") {
					d.issue("active-virtual-functions", "the physical function has active virtual functions; their ownership must be resolved first")
					break
				}
			}
		}
		inventoryIOMMU(at, path, groups, out.ManagementPCI, &d)
		out.Devices = append(out.Devices, d)
	}
	slices.SortFunc(out.Devices, func(a, b PCINetworkDevice) int { return strings.Compare(a.PCI, b.PCI) })
	return out, nil
}

// readPCINetworkInterfaces reads both native PCI/net and virtio-pci/virtioN/net layouts.
// Resolve the PCI symlink once and require every child directory and interface to remain
// underneath its canonical parent; unrelated devices must never be attributed to this NIC.
// An incomplete or ambiguous read returns no interfaces, so callers cannot mistake it for a
// complete ownership snapshot. A userspace-bound function may legitimately have no net path.
func readPCINetworkInterfaces(path string) ([]string, error) {
	pciPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	children, err := os.ReadDir(pciPath)
	if err != nil {
		return nil, err
	}
	parents := []string{pciPath}
	for _, child := range children {
		id, virtio := strings.CutPrefix(child.Name(), "virtio")
		if !virtio || id == "" {
			continue
		}
		n, err := strconv.ParseUint(id, 10, 32)
		if err != nil || strconv.FormatUint(n, 10) != id {
			continue
		}
		childPath, err := containedPCIDirectory(pciPath, child.Name())
		if err != nil {
			return nil, err
		}
		parents = append(parents, childPath)
	}
	var names []string
	seen := map[string]bool{}
	for _, parent := range parents {
		// Lstat distinguishes an absent net directory (normal for VFIO) from a dangling or
		// redirected net symlink, which must be reported as an incomplete inventory.
		if _, err := os.Lstat(filepath.Join(parent, "net")); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		netPath, err := containedPCIDirectory(parent, "net")
		if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(netPath)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if len(name) > 15 || strings.ContainsAny(name, "/\x00 \t\r\n") {
				return nil, fmt.Errorf("invalid PCI network interface name %q", name)
			}
			if _, err := containedPCIDirectory(netPath, name); err != nil {
				return nil, err
			}
			if seen[name] {
				return nil, fmt.Errorf("network interface %q appears under multiple PCI device paths; ownership is ambiguous", name)
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

func containedPCIDirectory(parent, child string) (string, error) {
	path, err := filepath.EvalSymlinks(filepath.Join(parent, child))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(parent, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("PCI device path %s escapes its parent %s", filepath.Join(parent, child), parent)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("PCI device path %s is not a directory", path)
	}
	return path, nil
}

func readPCIHex(path string, digits int) (uint32, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // fixed sysfs attribute under the configured root
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(raw))
	if len(s) != digits+2 || !strings.HasPrefix(s, "0x") {
		return 0, fmt.Errorf("%s: expected 0x followed by %d hex digits", filepath.Base(path), digits)
	}
	value, err := strconv.ParseUint(s[2:], 16, digits*4)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid hexadecimal value", filepath.Base(path))
	}
	return uint32(value), nil //nolint:gosec // at most six hex digits
}

func pciIOMMUGroup(path string) (string, error) {
	link, err := os.Readlink(filepath.Join(path, "iommu_group"))
	if err != nil {
		return "", err
	}
	group := filepath.Base(link)
	n, err := strconv.ParseUint(group, 10, 32)
	if err != nil || strconv.FormatUint(n, 10) != group {
		return "", fmt.Errorf("invalid IOMMU group %q", group)
	}
	return group, nil
}

func inventoryIOMMU(at func(string) string, path string, groups map[string]string, managementPCI []string, d *PCINetworkDevice) {
	group, err := pciIOMMUGroup(path)
	if err != nil {
		d.issue("unknown-iommu-group", fmt.Sprintf("cannot establish IOMMU isolation: %v", err))
		return
	}
	d.IOMMUGroup = group
	for _, pci := range managementPCI {
		if pci != d.PCI && groups[pci] == group {
			d.issue("management-iommu-group", fmt.Sprintf("IOMMU group %s contains management controller %s", group, pci))
		}
	}
	members, err := os.ReadDir(at(filepath.Join("sys/kernel/iommu_groups", group, "devices")))
	if err != nil {
		d.issue("unreadable-iommu-group", err.Error())
		return
	}
	for _, member := range members {
		pci, err := PCIAddress(member.Name())
		if err != nil {
			d.issue("invalid-iommu-member", err.Error())
			continue
		}
		d.IOMMUMembers = append(d.IOMMUMembers, pci)
		if groups[pci] != group {
			d.issue("inconsistent-iommu-group", fmt.Sprintf("PCI %s does not identify the same IOMMU group %s", pci, group))
		}
	}
	slices.Sort(d.IOMMUMembers)
	if !slices.Contains(d.IOMMUMembers, d.PCI) {
		d.issue("incomplete-iommu-group", "the controller is absent from its reported IOMMU group")
	}
	// The reverse membership must agree too: a disappearing member must not make a shared
	// group appear isolated in a snapshot taken while hardware is changing.
	var omitted []string
	for pci, memberGroup := range groups {
		if memberGroup == group && !slices.Contains(d.IOMMUMembers, pci) {
			omitted = append(omitted, pci)
		}
	}
	slices.Sort(omitted)
	if len(omitted) > 0 {
		d.issue("incomplete-iommu-group", "group membership omits PCI functions "+strings.Join(omitted, ","))
	}
	if len(d.IOMMUMembers) > 1 {
		d.issue("shared-iommu-group", fmt.Sprintf("IOMMU group %s contains %d PCI functions; automatic individual transfer is excluded", group, len(d.IOMMUMembers)))
	}
}
