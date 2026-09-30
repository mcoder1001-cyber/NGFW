package vppstartup

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const inventoryMgmt = "0000:00:01.0"
const inventoryData = "0000:00:02.0"
const inventoryOther = "0000:00:03.0"

type inventoryFixture struct {
	t    *testing.T
	root string
}

func newInventoryFixture(t *testing.T) *inventoryFixture {
	t.Helper()
	f := &inventoryFixture{t: t, root: t.TempDir()}
	f.device(inventoryMgmt, "0x020000", "ixgbe", "1", "ensmgmt")
	return f
}

func (f *inventoryFixture) path(p string) string { return filepath.Join(f.root, p) }

func (f *inventoryFixture) write(p, content string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.path(p)), 0o750); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.path(p), []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *inventoryFixture) link(p, target string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.path(p)), 0o750); err != nil {
		f.t.Fatal(err)
	}
	if err := os.Symlink(target, f.path(p)); err != nil {
		f.t.Fatal(err)
	}
}

func (f *inventoryFixture) remove(p string) {
	f.t.Helper()
	if err := os.Remove(f.path(p)); err != nil {
		f.t.Fatal(err)
	}
}

func (f *inventoryFixture) device(pci, class, driver, group string, netdevs ...string) {
	f.t.Helper()
	p := "sys/bus/pci/devices/" + pci
	f.write(p+"/class", class+"\n")
	f.write(p+"/vendor", "0x8086\n")
	f.write(p+"/device", "0x10fb\n")
	if driver != "" {
		f.link(p+"/driver", "../../drivers/"+driver)
	}
	if group != "" {
		f.link(p+"/iommu_group", "../../../../kernel/iommu_groups/"+group)
		f.link("sys/kernel/iommu_groups/"+group+"/devices/"+pci, "../../../../bus/pci/devices/"+pci)
	}
	netParent := p
	if driver == "virtio-pci" {
		netParent += "/virtio0"
	}
	for _, netdev := range netdevs {
		if err := os.MkdirAll(f.path(netParent+"/net/"+netdev), 0o750); err != nil {
			f.t.Fatal(err)
		}
		f.link("sys/class/net/"+netdev+"/device", "../../../"+strings.TrimPrefix(netParent, "sys/"))
	}
}

func (f *inventoryFixture) read() PCIInventory {
	f.t.Helper()
	got, err := ReadPCIInventory(HostSources{Root: f.root, MgmtPCI: []string{inventoryMgmt}})
	if err != nil {
		f.t.Fatal(err)
	}
	return got
}

func inventoryDevice(t *testing.T, inventory PCIInventory, pci string) PCINetworkDevice {
	t.Helper()
	for _, device := range inventory.Devices {
		if device.PCI == pci {
			return device
		}
	}
	t.Fatalf("PCI %s missing from inventory %+v", pci, inventory)
	return PCINetworkDevice{}
}

func inventoryHasIssue(d PCINetworkDevice, code string) bool {
	return slices.ContainsFunc(d.Issues, func(issue PCIInventoryIssue) bool { return issue.Code == code })
}

func TestPCIInventoryFindsKernelAndAlreadyBoundNICs(t *testing.T) {
	f := newInventoryFixture(t)
	// Intentionally create in reverse order. VFIO devices have no Linux netdev to enumerate.
	f.device(inventoryOther, "0x020000", "vfio-pci", "3")
	f.device(inventoryData, "0x020000", "ixgbe", "2", "ensdata")
	f.device("0000:00:04.0", "0x010802", "nvme", "4")
	got := f.read()
	if len(got.Devices) != 3 || !slices.Equal(got.ManagementPCI, []string{inventoryMgmt}) {
		t.Fatalf("inventory = %+v", got)
	}
	if got.Devices[0].PCI != inventoryMgmt || got.Devices[1].PCI != inventoryData || got.Devices[2].PCI != inventoryOther {
		t.Fatalf("devices are not sorted: %+v", got.Devices)
	}
	management := got.Devices[0]
	if !management.Management || management.TopologyEligible() || !inventoryHasIssue(management, "management-nic") {
		t.Fatalf("management was not excluded: %+v", management)
	}
	data, bound := got.Devices[1], got.Devices[2]
	if !data.TopologyEligible() || data.VendorID != 0x8086 || data.DeviceID != 0x10fb || data.Driver != "ixgbe" || !slices.Equal(data.Interfaces, []string{"ensdata"}) {
		t.Fatalf("kernel NIC facts = %+v", data)
	}
	if !bound.TopologyEligible() || bound.Driver != "vfio-pci" || len(bound.Interfaces) != 0 || !slices.Equal(bound.IOMMUMembers, []string{inventoryOther}) {
		t.Fatalf("VFIO NIC facts = %+v", bound)
	}
	if again := f.read(); !reflect.DeepEqual(got, again) {
		t.Fatalf("repeated read changed inventory:\n%+v\n%+v", got, again)
	}
}

func TestPCIInventoryReusesManagementDiscovery(t *testing.T) {
	f := newInventoryFixture(t)
	f.device(inventoryData, "0x020000", "ixgbe", "2", "ensdata")
	f.write("proc/net/route", "Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\n"+
		"ensmgmt 00000000 0100000A 0003 0 0 100 00000000 0 0 0\n")
	got, err := ReadPCIInventory(HostSources{Root: f.root, MgmtIfaces: []string{"ensdata"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.ManagementPCI, []string{inventoryMgmt, inventoryData}) {
		t.Fatalf("default route and explicit management interface must both be protected: %+v", got)
	}
	for _, d := range got.Devices {
		if d.TopologyEligible() {
			t.Fatalf("protected NIC eligible: %+v", d)
		}
	}
}

func TestPCIInventoryFindsVirtioChildNetworkInterfaces(t *testing.T) {
	f := newInventoryFixture(t)
	f.device(inventoryData, "0x020000", "virtio-pci", "2", "ensvirtio")
	// Real /sys/bus/pci/devices entries themselves are links into /sys/devices. Keep that
	// layout too: containment must use the canonical PCI parent, not the /sys/bus alias.
	canonical := "sys/devices/pci0000:00/" + inventoryData
	if err := os.MkdirAll(filepath.Dir(f.path(canonical)), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(f.path("sys/bus/pci/devices/"+inventoryData), f.path(canonical)); err != nil {
		t.Fatal(err)
	}
	f.link("sys/bus/pci/devices/"+inventoryData, "../../../devices/pci0000:00/"+inventoryData)
	d := inventoryDevice(t, f.read(), inventoryData)
	if !d.TopologyEligible() || d.Driver != "virtio-pci" || !slices.Equal(d.Interfaces, []string{"ensvirtio"}) {
		t.Fatalf("virtio network interface was not discovered through the child device: %+v", d)
	}
	if _, err := os.Stat(f.path(canonical + "/net")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fixture unexpectedly has a direct PCI/net directory: %v", err)
	}
}

func TestPCIInventoryRejectsEscapingAndAmbiguousNetworkPaths(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		setup         func(*inventoryFixture)
	}{
		{"virtio child escapes PCI function", "escapes", func(f *inventoryFixture) {
			f.link("sys/bus/pci/devices/"+inventoryData+"/virtio0", "../"+inventoryMgmt)
		}},
		{"net directory escapes PCI function", "escapes", func(f *inventoryFixture) {
			f.remove("sys/bus/pci/devices/" + inventoryData + "/net/ensdata")
			f.remove("sys/bus/pci/devices/" + inventoryData + "/net")
			f.link("sys/bus/pci/devices/"+inventoryData+"/net", "../"+inventoryMgmt+"/net")
		}},
		{"netdev escapes net directory", "escapes", func(f *inventoryFixture) {
			f.remove("sys/bus/pci/devices/" + inventoryData + "/net/ensdata")
			f.link("sys/bus/pci/devices/"+inventoryData+"/net/ensdata", "../../"+inventoryMgmt+"/net/ensmgmt")
		}},
		{"dangling virtio child", "no such file", func(f *inventoryFixture) {
			f.link("sys/bus/pci/devices/"+inventoryData+"/virtio0", "missing")
		}},
		{"duplicate name in native and virtio paths", "ownership is ambiguous", func(f *inventoryFixture) {
			f.write("sys/bus/pci/devices/"+inventoryData+"/virtio0/net/ensdata/ifindex", "5\n")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newInventoryFixture(t)
			f.device(inventoryData, "0x020000", "ixgbe", "2", "ensdata")
			tc.setup(f)
			d := inventoryDevice(t, f.read(), inventoryData)
			if d.TopologyEligible() || !inventoryHasIssue(d, "unreadable-interfaces") || len(d.Interfaces) != 0 {
				t.Fatalf("incomplete or ambiguous netdev inventory was accepted: %+v", d)
			}
			if !slices.ContainsFunc(d.Issues, func(issue PCIInventoryIssue) bool { return strings.Contains(issue.Message, tc.message) }) {
				t.Fatalf("missing explanation %q: %+v", tc.message, d)
			}
		})
	}
	t.Run("multiple virtio child network interfaces", func(t *testing.T) {
		f := newInventoryFixture(t)
		f.device(inventoryData, "0x020000", "virtio-pci", "2", "ensvirtio")
		f.write("sys/bus/pci/devices/"+inventoryData+"/virtio1/net/other/ifindex", "5\n")
		d := inventoryDevice(t, f.read(), inventoryData)
		if d.TopologyEligible() || !inventoryHasIssue(d, "multiple-interfaces") || !slices.Equal(d.Interfaces, []string{"ensvirtio", "other"}) {
			t.Fatalf("multiple virtio ports must require an ownership decision: %+v", d)
		}
	})
}

func TestPCIInventoryExcludesSharedAndInconsistentGroups(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*inventoryFixture)
		codes []string
	}{
		{"management group", func(f *inventoryFixture) {
			f.device(inventoryData, "0x020000", "ixgbe", "1", "ensdata")
		}, []string{"management-iommu-group", "shared-iommu-group"}},
		{"shared with storage", func(f *inventoryFixture) {
			f.device(inventoryData, "0x020000", "ixgbe", "2", "ensdata")
			f.device(inventoryOther, "0x010802", "nvme", "2")
		}, []string{"shared-iommu-group"}},
		{"shared with another data NIC", func(f *inventoryFixture) {
			f.device(inventoryData, "0x020000", "ixgbe", "2", "ensdata")
			f.device(inventoryOther, "0x020000", "vfio-pci", "2")
		}, []string{"shared-iommu-group"}},
		{"management absent from group listing", func(f *inventoryFixture) {
			f.device(inventoryData, "0x020000", "ixgbe", "1", "ensdata")
			f.remove("sys/kernel/iommu_groups/1/devices/" + inventoryMgmt)
		}, []string{"management-iommu-group", "incomplete-iommu-group"}},
		{"self absent from group listing", func(f *inventoryFixture) {
			f.device(inventoryData, "0x020000", "ixgbe", "2", "ensdata")
			f.remove("sys/kernel/iommu_groups/2/devices/" + inventoryData)
		}, []string{"incomplete-iommu-group"}},
		{"member points to another group", func(f *inventoryFixture) {
			f.device(inventoryData, "0x020000", "ixgbe", "2", "ensdata")
			f.device(inventoryOther, "0x020000", "vfio-pci", "3")
			f.link("sys/kernel/iommu_groups/2/devices/"+inventoryOther, "unused")
		}, []string{"inconsistent-iommu-group", "shared-iommu-group"}},
		{"unreadable membership", func(f *inventoryFixture) {
			f.device(inventoryData, "0x020000", "ixgbe", "2", "ensdata")
			f.remove("sys/kernel/iommu_groups/2/devices/" + inventoryData)
			f.remove("sys/kernel/iommu_groups/2/devices")
		}, []string{"unreadable-iommu-group"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newInventoryFixture(t)
			tc.setup(f)
			d := inventoryDevice(t, f.read(), inventoryData)
			if d.TopologyEligible() {
				t.Fatalf("unsafe group eligible: %+v", d)
			}
			for _, code := range tc.codes {
				if !inventoryHasIssue(d, code) {
					t.Errorf("missing exclusion %s: %+v", code, d)
				}
			}
		})
	}
}

func TestPCIInventoryReportsUnsupportedAndAmbiguousDevices(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		setup      func(*inventoryFixture)
	}{
		{"wireless", "unsupported-class", func(f *inventoryFixture) { f.write("sys/bus/pci/devices/"+inventoryData+"/class", "0x028000\n") }},
		{"unbound", "unknown-driver", func(f *inventoryFixture) { f.remove("sys/bus/pci/devices/" + inventoryData + "/driver") }},
		{"no IOMMU", "unknown-iommu-group", func(f *inventoryFixture) { f.remove("sys/bus/pci/devices/" + inventoryData + "/iommu_group") }},
		{"bad vendor", "invalid-vendor", func(f *inventoryFixture) { f.write("sys/bus/pci/devices/"+inventoryData+"/vendor", "broken\n") }},
		{"device disappeared", "invalid-vendor", func(f *inventoryFixture) { f.write("sys/bus/pci/devices/"+inventoryData+"/vendor", "0xffff\n") }},
		{"bad device id", "invalid-device", func(f *inventoryFixture) { f.write("sys/bus/pci/devices/"+inventoryData+"/device", "0x123456\n") }},
		{"missing netdev", "missing-interface", func(f *inventoryFixture) { f.remove("sys/bus/pci/devices/" + inventoryData + "/net/ensdata") }},
		{"multiple ports", "multiple-interfaces", func(f *inventoryFixture) { f.write("sys/bus/pci/devices/"+inventoryData+"/net/other/ifindex", "4\n") }},
		{"shared kernel driver", "shared-kernel-driver", func(f *inventoryFixture) {
			f.remove("sys/bus/pci/devices/" + inventoryData + "/driver")
			f.link("sys/bus/pci/devices/"+inventoryData+"/driver", "../../drivers/mlx5_core")
		}},
		{"active virtual functions", "active-virtual-functions", func(f *inventoryFixture) {
			f.link("sys/bus/pci/devices/"+inventoryData+"/virtfn0", "../"+inventoryOther)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newInventoryFixture(t)
			f.device(inventoryData, "0x020000", "ixgbe", "2", "ensdata")
			tc.setup(f)
			d := inventoryDevice(t, f.read(), inventoryData)
			if d.TopologyEligible() || !inventoryHasIssue(d, tc.code) {
				t.Fatalf("want exclusion %s, got %+v", tc.code, d)
			}
		})
	}
}

func TestPCIInventoryRefusesUnknownManagementAndIncompleteInventory(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		setup      func(*inventoryFixture, *HostSources)
	}{
		{"unknown management", "known management NIC", func(_ *inventoryFixture, src *HostSources) { src.MgmtPCI = nil }},
		{"management missing", "not a present network controller", func(_ *inventoryFixture, src *HostSources) { src.MgmtPCI = []string{inventoryOther} }},
		{"management is storage", "not a present network controller", func(f *inventoryFixture, _ *HostSources) {
			f.write("sys/bus/pci/devices/"+inventoryMgmt+"/class", "0x010802\n")
		}},
		{"unreadable PCI class", "class", func(f *inventoryFixture, _ *HostSources) { f.remove("sys/bus/pci/devices/" + inventoryMgmt + "/class") }},
		{"malformed PCI class", "hexadecimal", func(f *inventoryFixture, _ *HostSources) {
			f.device(inventoryData, "0x02oops", "ixgbe", "2", "ensdata")
		}},
		{"malformed PCI address", "PCI address", func(f *inventoryFixture, _ *HostSources) {
			f.write("sys/bus/pci/devices/not-pci/class", "0x020000\n")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newInventoryFixture(t)
			src := HostSources{Root: f.root, MgmtPCI: []string{inventoryMgmt}}
			tc.setup(f, &src)
			got, err := ReadPCIInventory(src)
			if !errors.Is(err, ErrHost) || !strings.Contains(err.Error(), tc.want) || len(got.Devices) != 0 {
				t.Fatalf("want ErrHost containing %q and no devices, got %+v, %v", tc.want, got, err)
			}
		})
	}
}
