package vppstartup

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// ngfwALikeRoot builds a /sys + /proc tree resembling ngfw-a: six data NICs plus ens192 (0000:0b:00.0)
// carrying the IPv4 default route (the management interface). It also adds noise that must NOT be
// enumerated: lo, a veth (no PCI device), and a USB NIC (device without a PCI address).
func ngfwALikeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(p, s string) {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// a physical NIC: device → a real PCI device dir (with a driver symlink), address, carrier
	nic := func(ifname, pci, mac, carrier, driver string) {
		dir := filepath.Join(root, "sys/class/net", ifname)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		devDir := filepath.Join(root, "sys/devices/pci0000:00/0000:00:15.0", pci)
		if err := os.MkdirAll(devDir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../../../bus/pci/drivers/"+driver, filepath.Join(devDir, "driver")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("../../../devices/pci0000:00/0000:00:15.0", pci), filepath.Join(dir, "device")); err != nil {
			t.Fatal(err)
		}
		write("sys/class/net/"+ifname+"/address", mac+"\n")
		write("sys/class/net/"+ifname+"/carrier", carrier+"\n")
	}
	write("proc/net/route", "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n"+
		"ens192\t00000000\t017E1EAC\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"+
		"ens192\t007E1EAC\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n")
	nic("ens192", "0000:0b:00.0", "00:0c:29:00:00:92", "1", "vmxnet3") // management (default route)
	nic("ens161", "0000:04:00.0", "00:0c:29:00:00:61", "1", "vmxnet3")
	nic("ens193", "0000:0c:00.0", "00:0c:29:00:00:93", "0", "vmxnet3")
	nic("ens224", "0000:13:00.0", "00:0c:29:00:00:24", "1", "vmxnet3")
	nic("ens225", "0000:1b:00.0", "00:0c:29:00:00:25", "1", "vmxnet3")
	nic("ens256", "0000:0d:00.0", "00:0c:29:00:00:56", "0", "vmxnet3")
	nic("ens257", "0000:0e:00.0", "00:0c:29:00:00:57", "1", "vmxnet3")
	// noise that must not be enumerated
	if err := os.MkdirAll(filepath.Join(root, "sys/class/net/lo"), 0o750); err != nil {
		t.Fatal(err)
	}
	veth := filepath.Join(root, "sys/class/net/veth0")
	if err := os.MkdirAll(veth, 0o750); err != nil {
		t.Fatal(err)
	}
	usb := filepath.Join(root, "sys/class/net/usb0")
	if err := os.MkdirAll(usb, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../devices/platform/usb1/1-1/1-1:1.0", filepath.Join(usb, "device")); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestHostNICs(t *testing.T) {
	root := ngfwALikeRoot(t)
	nics, notes, err := HostNICs(HostSources{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(nics) != 7 {
		t.Fatalf("want 7 physical NICs (6 data + management), got %d: %+v", len(nics), nics)
	}
	// sorted by PCI, management flagged only on ens192, USB/veth/lo excluded
	var mgmt, data int
	byNetdev := map[string]HostNIC{}
	for i, n := range nics {
		byNetdev[n.Netdev] = n
		if i > 0 && nics[i-1].PCI > n.PCI {
			t.Errorf("NICs not sorted by PCI: %s before %s", nics[i-1].PCI, n.PCI)
		}
		if n.IsManagement {
			mgmt++
		} else {
			data++
		}
		if n.MAC == "" || n.Driver != "vmxnet3" {
			t.Errorf("%s: mac=%q driver=%q", n.Netdev, n.MAC, n.Driver)
		}
	}
	if mgmt != 1 || data != 6 {
		t.Fatalf("want 1 management + 6 data NICs, got mgmt=%d data=%d", mgmt, data)
	}
	if _, ok := byNetdev["usb0"]; ok {
		t.Error("USB NIC (no PCI address) must not be enumerated")
	}
	if _, ok := byNetdev["veth0"]; ok {
		t.Error("veth (no PCI device) must not be enumerated")
	}
	if !byNetdev["ens192"].IsManagement || byNetdev["ens161"].IsManagement {
		t.Errorf("management should be ens192 only: ens192=%v ens161=%v", byNetdev["ens192"].IsManagement, byNetdev["ens161"].IsManagement)
	}
	if byNetdev["ens192"].PCI != "0000:0b:00.0" || byNetdev["ens161"].PCI != "0000:04:00.0" {
		t.Errorf("pci mismatch: %+v", byNetdev)
	}
	if !byNetdev["ens161"].LinkUp || byNetdev["ens193"].LinkUp {
		t.Errorf("carrier: ens161 should be up, ens193 down")
	}
	// the default-route NIC is reported in the management notes
	if !slices.ContainsFunc(notes, func(s string) bool { return len(s) > 0 }) {
		t.Errorf("expected management notes, got %v", notes)
	}
}

// review R2R4 #2: a host without a default route and without a control connection (first boot before DHCP) has
// no identifiable management NIC — HostNICs flags none, and the API refuses to seed on that answer.
func TestHostNICsNoManagementDetected(t *testing.T) {
	root := ngfwALikeRoot(t)
	if err := os.Remove(filepath.Join(root, "proc/net/route")); err != nil {
		t.Fatal(err)
	}
	nics, notes, err := HostNICs(HostSources{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(nics) != 7 {
		t.Fatalf("want 7 NICs, got %d", len(nics))
	}
	for _, n := range nics {
		if n.IsManagement {
			t.Errorf("%s flagged management without a default route or control connection", n.Netdev)
		}
	}
	if len(notes) != 0 {
		t.Errorf("no management decision → no notes, got %v", notes)
	}
}

// review R1R3 #2: a NIC already bound to vfio-pci has no netdev; it is found on the PCI bus with BoundToDpdk.
// A non-network PCI device on vfio (class 0x0300) and an unbound NIC are not enumerated.
func TestHostNICsFindsDpdkBoundNIC(t *testing.T) {
	root := ngfwALikeRoot(t)
	pciDev := func(pci, class, driver string) {
		dir := filepath.Join(root, "sys/bus/pci/devices", pci)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "class"), []byte(class+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if driver != "" {
			if err := os.Symlink("../../../bus/pci/drivers/"+driver, filepath.Join(dir, "driver")); err != nil {
				t.Fatal(err)
			}
		}
	}
	pciDev("0000:1d:00.0", "0x020000", "vfio-pci") // bound data NIC
	pciDev("0000:1e:00.0", "0x030000", "vfio-pci") // a GPU on vfio: not a NIC
	pciDev("0000:1f:00.0", "0x020000", "")         // unbound NIC without netdev
	pciDev("0000:04:00.0", "0x020000", "vmxnet3")  // ens161, already seen via its netdev
	nics, _, err := HostNICs(HostSources{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(nics) != 8 {
		t.Fatalf("want 7 netdev NICs + 1 vfio-bound, got %d: %+v", len(nics), nics)
	}
	var bound []HostNIC
	for _, n := range nics {
		if n.BoundToDpdk {
			bound = append(bound, n)
		}
	}
	if len(bound) != 1 || bound[0].PCI != "0000:1d:00.0" || bound[0].Netdev != "" || bound[0].Driver != "vfio-pci" || bound[0].IsManagement {
		t.Fatalf("bound NICs = %+v", bound)
	}
}

// D-177 / review R1R3 #11: an agent that may not read /proc/net/tcp{,6} still decides the management NIC from the
// default route, with a note, instead of failing the whole inventory.
func TestHostNICsToleratesUnreadableTCPTable(t *testing.T) {
	root := ngfwALikeRoot(t)
	saved := readProcNet
	t.Cleanup(func() { readProcNet = saved })
	readProcNet = func(string) ([]byte, error) { return nil, fs.ErrPermission }
	nics, notes, err := HostNICs(HostSources{Root: root})
	if err != nil {
		t.Fatalf("EACCES on /proc/net/tcp must not fail the inventory: %v", err)
	}
	var mgmt []string
	for _, n := range nics {
		if n.IsManagement {
			mgmt = append(mgmt, n.Netdev)
		}
	}
	if !slices.Equal(mgmt, []string{"ens192"}) {
		t.Fatalf("management from the default route = %v", mgmt)
	}
	if !slices.ContainsFunc(notes, func(s string) bool { return strings.Contains(s, "permission denied") }) {
		t.Fatalf("expected a permission-denied note, got %v", notes)
	}
}

func TestHostNICsVirtioPCIManagementAndData(t *testing.T) {
	root := ngfwALikeRoot(t)
	for _, device := range []struct{ name, pci, child string }{
		{"ens192", "0000:0b:00.0", "virtio0"},
		{"ens161", "0000:04:00.0", "virtio1"},
	} {
		dir := filepath.Join(root, "sys/class/net", device.name)
		link := filepath.Join(dir, "device")
		target, err := os.Readlink(link)
		if err != nil {
			t.Fatal(err)
		}
		child := filepath.Join(dir, target, device.child)
		if err := os.MkdirAll(child, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../../../../bus/virtio/drivers/virtio_net", filepath.Join(child, "driver")); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(target, device.child), link); err != nil {
			t.Fatal(err)
		}
	}
	nics, _, err := HostNICs(HostSources{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(nics) != 7 {
		t.Fatalf("want 7 physical NICs including virtio-pci, got %+v", nics)
	}
	for _, nic := range nics {
		if nic.Netdev == "ens192" && (nic.PCI != "0000:0b:00.0" || !nic.IsManagement) {
			t.Fatalf("management virtio-pci: %+v", nic)
		}
		if nic.Netdev == "ens161" && (nic.PCI != "0000:04:00.0" || nic.IsManagement) {
			t.Fatalf("data virtio-pci: %+v", nic)
		}
	}
}

func TestNetdevPCIRejectsNonPCIDevices(t *testing.T) {
	for _, path := range []string{"../../../devices/platform/virtio-mmio/virtio0", "../../../devices/pci0000:00/0000:00:01.0/usb1/1-1", "../../../devices/pci0000:00/0000:00:01.0/virtioinvalid", "../../../devices/pci0000:00/0000:00:01.0/virtio01"} {
		if pci, err := netdevPCI(path); err == nil {
			t.Errorf("accepted non-PCI layout %s as %s", path, pci)
		}
	}
}
