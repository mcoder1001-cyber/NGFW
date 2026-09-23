package vppstartup

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeRoot builds a /sys + /proc tree for ReadHost: ens192 (0000:0b:00.0) carries the IPv4
// default route, ens161 (0000:04:00.0) the IPv6 default route, ens193 has none.
func fakeRoot(t *testing.T) string {
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
	link := func(ifname, pci string) {
		dir := filepath.Join(root, "sys/class/net", ifname)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../../../devices/pci0000:00/0000:00:15.0/"+pci, filepath.Join(dir, "device")); err != nil {
			t.Fatal(err)
		}
	}
	write("sys/devices/system/cpu/online", "0-7\n")
	write("sys/devices/system/cpu/isolated", "6-7\n")
	write("sys/devices/system/node/node0/cpulist", "0-7\n")
	write("proc/meminfo", "MemTotal: 1 kB\nHugePages_Total:    1024\nHugepagesize:       2048 kB\n")
	write("proc/net/route", "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n"+
		"ens192\t00000000\t017E1EAC\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"+
		"ens192\t007E1EAC\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n"+
		"ens193\t0000000A\t00000000\t0001\t0\t0\t0\t000000FF\t0\t0\t0\n")
	write("proc/net/ipv6_route", strings.Repeat("0", 32)+" 00 "+strings.Repeat("0", 32)+" 00 fe800000000000000000000000000001 00000400 00000001 00000000 00000003 ens161\n"+
		strings.Repeat("0", 32)+" 00 "+strings.Repeat("0", 32)+" 00 "+strings.Repeat("0", 32)+" ffffffff 00000001 00000000 00200200 lo\n")
	link("ens192", "0000:0b:00.0")
	link("ens161", "0000:04:00.0")
	link("ens193", "0000:0c:00.0")
	write("plugins/dpdk_plugin.so", "")
	write("plugins/linux_cp_plugin.so", "")
	write("etc/startup.conf", "plugins {\n  plugin linux_cp_plugin.so { enable }  # D-060\n}\n")
	return root
}

func TestReadHost(t *testing.T) {
	root := fakeRoot(t)
	h, err := ReadHost(HostSources{Root: root, PluginDir: filepath.Join(root, "plugins"), CurrentConf: filepath.Join(root, "etc/startup.conf")})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Check(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.ManagementPCI, []string{"0000:04:00.0", "0000:0b:00.0"}) {
		t.Errorf("management = %v (want both default-route NICs)", h.ManagementPCI)
	}
	if FormatCPUList(h.OnlineCPUs) != "0-7" || FormatCPUList(h.IsolCPUs) != "6-7" || h.NUMANodes != 1 || h.HugepageBytes != 2<<30 {
		t.Errorf("%+v", h)
	}
	if !slices.Equal(h.Plugins, []string{"dpdk_plugin.so", "linux_cp_plugin.so"}) || !maps.Equal(h.CurrentPlugins, map[string]bool{"linux_cp_plugin.so": true}) {
		t.Errorf("plugins %v current %v", h.Plugins, h.CurrentPlugins)
	}

	// an extra interface and PCI are added; no current file = no switches
	h, err = ReadHost(HostSources{Root: root, PluginDir: filepath.Join(root, "plugins"), MgmtIfaces: []string{"ens193"}, MgmtPCI: []string{"0000:1C:00.0"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.ManagementPCI, []string{"0000:04:00.0", "0000:0b:00.0", "0000:0c:00.0", "0000:1c:00.0"}) || len(h.CurrentPlugins) != 0 || h.CurrentPlugins == nil {
		t.Errorf("management %v current %v", h.ManagementPCI, h.CurrentPlugins)
	}

	for name, src := range map[string]HostSources{
		"iface without PCI device": {Root: root, PluginDir: filepath.Join(root, "plugins"), MgmtIfaces: []string{"bond0"}},
		"iface path trick":         {Root: root, PluginDir: filepath.Join(root, "plugins"), MgmtIfaces: []string{"../../x"}},
		"bad mgmt pci":             {Root: root, PluginDir: filepath.Join(root, "plugins"), MgmtPCI: []string{"0b:00.0"}},
		"no plugins":               {Root: root, PluginDir: filepath.Join(root, "nothing")},
		"missing current file":     {Root: root, PluginDir: filepath.Join(root, "plugins"), CurrentConf: filepath.Join(root, "etc/none.conf")},
		"no sysfs":                 {Root: filepath.Join(root, "void"), PluginDir: filepath.Join(root, "plugins")},
	} {
		if _, err := ReadHost(src); !errors.Is(err, ErrHost) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestHostCheckRequiresEveryFact(t *testing.T) {
	full := vrxA(t)
	if err := full.Check(); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*Host){
		"management":      func(h *Host) { h.ManagementPCI = nil },
		"bad management":  func(h *Host) { h.ManagementPCI = []string{"x"} },
		"online CPUs":     func(h *Host) { h.OnlineCPUs = nil },
		"NUMA":            func(h *Host) { h.NUMANodes = 0 },
		"hugepages":       func(h *Host) { h.HugepageBytes = 0 },
		"plugins":         func(h *Host) { h.Plugins = nil },
		"current plugins": func(h *Host) { h.CurrentPlugins = nil },
	} {
		h := vrxA(t)
		mut(&h)
		if _, _, err := Generate(parseDoc(t, `{}`), h, DefaultSettings()); !errors.Is(err, ErrHost) {
			t.Errorf("%s missing: err = %v", name, err)
		}
	}
	// the agent-side Renderer has no default host (F7)
	if _, err := New(Host{}).Render(context.Background(), nil); !errors.Is(err, ErrHost) {
		t.Errorf("Renderer without host facts: %v", err)
	}
}

func TestPluginSwitches(t *testing.T) {
	got, err := PluginSwitches([]byte("unix { nodaemon }\nplugins {\n plugin a_plugin.so { enable }\n plugin b_plugin.so { disable }\n}\n"))
	if err != nil || !maps.Equal(got, map[string]bool{"a_plugin.so": true, "b_plugin.so": false}) {
		t.Fatal(got, err)
	}
	for _, bad := range []string{
		"plugins { path /tmp }\n",
		"plugins { plugin a_plugin.so { enable disable } }\n",
		"plugins {\n plugin a_plugin.so {\n enable\n disable\n }\n}\n",
		"plugins {\n plugin a_plugin.so { on }\n}\n",
		"plugins {\n vat-plugin { x }\n}\n",
		"plugins {\n",
	} {
		if _, err := PluginSwitches([]byte(bad)); err == nil {
			t.Errorf("PluginSwitches(%q) accepted", bad)
		}
	}
}

// TestCPUPlacementExplicit: F4 — main-core and corelist-workers are always explicit and follow
// the host's online/isolated sets (reviewer repros).
func TestCPUPlacementExplicit(t *testing.T) {
	for _, c := range []struct {
		name, doc, online, isol string
		wantMain                string
		wantCorelist            string
	}{
		{"workers without main core", `{"workers":2}`, "0-31", "", "main-core 1", "corelist-workers 2-3"},
		{"main core 31 + 2 workers boots (VPP picks 1,2)", `{"mainCore":31,"workers":2}`, "0-31", "", "main-core 31", "corelist-workers 1-2"},
		{"workers go to the isolated CPUs", `{"mainCore":5,"workers":2}`, "0-7", "6-7", "main-core 5", "corelist-workers 6-7"},
		{"default main core avoids isolated CPUs", `{"workers":2}`, "0-7", "1-2", "main-core 3", "corelist-workers 1-2"},
		{"online holes", `{"workers":3}`, "0-1,4-5,8", "", "main-core 1", "corelist-workers 4-5,8"},
		{"CPU 0 only as last resort", `{"workers":1}`, "0-1", "", "main-core 1", "corelist-workers 0"},
		{"no workers", `{}`, "0-3", "", "main-core 1", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := Generate(parseDoc(t, `{"dataplane":`+c.doc+`}`), *hostCPUs(t, c.online, c.isol), DefaultSettings())
			if err != nil {
				t.Fatal(err)
			}
			s := string(out)
			if !strings.Contains(s, "  "+c.wantMain+"\n") {
				t.Errorf("missing %q", c.wantMain)
			}
			if c.wantCorelist != "" && !strings.Contains(s, "  "+c.wantCorelist+"\n") {
				t.Errorf("missing %q", c.wantCorelist)
			}
			if c.wantCorelist == "" && strings.Contains(s, "corelist-workers") {
				t.Error("unexpected corelist-workers")
			}
			if strings.Contains(s, "  workers ") {
				t.Error("implicit `workers N` rendered")
			}
		})
	}
}

// TestManagementFromHost: F1 regressions — the reviewer's repro and the empty document.
func TestManagementFromHost(t *testing.T) {
	_, _, err := Generate(parseDoc(t, `{"dataplane":{"managementPci":["0000:04:00.0"],"devices":{"0000:0b:00.0":{"name":"lan"}}}}`), vrxA(t), DefaultSettings())
	if !errors.Is(err, ErrInput) || !strings.Contains(err.Error(), "does not match the host's management NIC(s) 0000:0b:00.0") {
		t.Fatalf("reviewer repro: %v", err)
	}
	out, _, err := Generate(parseDoc(t, `{}`), vrxA(t), DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "  blacklist 0000:0b:00.0\n") || !strings.Contains(string(out), "  no-pci\n") {
		t.Fatalf("empty document lost the blacklist:\n%s", out)
	}
	// document agrees with the host: fine, and still exactly one blacklist line
	out, _, err = Generate(parseDoc(t, `{"dataplane":{"managementPci":["0000:0B:00.0"],"devices":{"0000:04:00.0":{"name":"wan"}}}}`), vrxA(t), DefaultSettings())
	if err != nil || strings.Count(string(out), "blacklist") != 1 {
		t.Fatalf("%v\n%s", err, out)
	}
	// a second host management NIC is protected even though the document never names it
	h := vrxA(t)
	h.ManagementPCI = append(h.ManagementPCI, "0000:1c:00.0")
	if _, _, err := Generate(parseDoc(t, `{"dataplane":{"devices":{"0000:1c:00.0":{"name":"sync"}}}}`), h, DefaultSettings()); !errors.Is(err, ErrInput) {
		t.Fatalf("second host mgmt NIC accepted: %v", err)
	}
}

// TestPluginSemantics (D-084): `plugins` absent → the current file's switches are kept; present →
// authoritative (exactly its switches; warnings for missing D-060 plugins and removed switches).
func TestPluginSemantics(t *testing.T) {
	count := func(ws []string, sub string) int {
		n := 0
		for _, w := range ws {
			if strings.Contains(w, sub) {
				n++
			}
		}
		return n
	}
	// absent: D-060 block kept
	out, m, err := Generate(parseDoc(t, `{"dataplane":{"mainCore":1}}`), vrxA(t), DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"plugin linux_cp_plugin.so { enable }", "plugin linux_nl_plugin.so { enable }", "plugin npt66_plugin.so { enable }"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("absent: missing %q", want)
		}
	}
	if count(m.Warnings, "kept from the current start-up file") != 3 {
		t.Errorf("absent: warnings %q", m.Warnings)
	}

	// present: exactly the listed switches
	out, m, err = Generate(parseDoc(t, `{"dataplane":{"mainCore":1,"plugins":{"switches":{"npt66_plugin.so":false,"acl_plugin.so":true}}}}`), vrxA(t), DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "plugin acl_plugin.so { enable }") || !strings.Contains(s, "plugin npt66_plugin.so { disable }") ||
		strings.Contains(s, "linux_cp_plugin.so") || strings.Contains(s, "linux_nl_plugin.so") {
		t.Errorf("present: not authoritative:\n%s", s)
	}
	if count(m.Warnings, "(D-060) is not listed") != 2 || count(m.Warnings, "is removed (not in dataplane.plugins.switches)") != 2 {
		t.Errorf("present: warnings %q", m.Warnings)
	}

	// present and empty: no plugins block at all, warnings for all three D-060 plugins
	out, m, err = Generate(parseDoc(t, `{"dataplane":{"mainCore":1,"plugins":{}}}`), vrxA(t), DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "plugins {") || count(m.Warnings, "(D-060) is not listed") != 3 {
		t.Errorf("present, empty: %q\n%s", m.Warnings, out)
	}

	// present, listing the D-060 set: no warnings
	_, m, err = Generate(parseDoc(t, `{"dataplane":{"mainCore":1,"plugins":{"switches":{"linux_cp_plugin.so":true,"linux_nl_plugin.so":true,"npt66_plugin.so":true}}}}`), vrxA(t), DefaultSettings())
	if err != nil || len(m.Warnings) != 0 {
		t.Errorf("present, D-060 listed: %v %q", err, m.Warnings)
	}
}

// procLE4 writes an IPv4 address the way /proc/net/{route,tcp} do (little-endian hex).
func procLE4(a string) string {
	b := netip.MustParseAddr(a).As4()
	return fmt.Sprintf("%02X%02X%02X%02X", b[3], b[2], b[1], b[0])
}

// procTCP6 writes an IPv6 address the way /proc/net/tcp6 does (four little-endian 32-bit words).
func procTCP6(a string) string {
	b := netip.MustParseAddr(a).As16()
	var s strings.Builder
	for w := 0; w < 4; w++ {
		fmt.Fprintf(&s, "%02X%02X%02X%02X", b[w*4+3], b[w*4+2], b[w*4+1], b[w*4])
	}
	return s.String()
}

// mgmtRoot: management reached on a directly connected subnet (ens193), the default route on a
// linux-cp tap (VPP-owned) plus a blackhole default, an IPv6 session on ens224.
func mgmtRoot(t *testing.T, defaultIf string) string {
	t.Helper()
	root := fakeRoot(t)
	w := func(p, s string) {
		if err := os.WriteFile(filepath.Join(root, p), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	line := func(ifname, dst, mask string) string {
		return fmt.Sprintf("%s\t%s\t00000000\t0001\t0\t0\t0\t%s\t0\t0\t0\n", ifname, procLE4(dst), procLE4(mask))
	}
	w("proc/net/route", "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n"+
		line(defaultIf, "0.0.0.0", "0.0.0.0")+line("*", "0.0.0.0", "0.0.0.0")+
		line("ens193", "10.1.2.0", "255.255.255.0")+line("ens161", "10.9.0.0", "255.255.0.0"))
	w("proc/net/ipv6_route", "20010db8000000000000000000000000 40 00000000000000000000000000000000 00 00000000000000000000000000000000 00000100 00000001 00000000 00000001 ens224\n")
	tcp := "  sl  local_address rem_address   st\n" +
		fmt.Sprintf("   0: %s:0016 %s:C350 01 0\n", procLE4("10.1.2.5"), procLE4("10.1.2.50")) + // ssh on the connected subnet
		fmt.Sprintf("   1: %s:01BB %s:C351 01 0\n", procLE4("10.9.0.5"), procLE4("10.9.1.1")) + // https, not a control port
		fmt.Sprintf("   2: %s:0016 %s:C352 0A 0\n", procLE4("0.0.0.0"), procLE4("0.0.0.0")) + // listening
		fmt.Sprintf("   3: %s:0016 %s:C353 01 0\n", procLE4("127.0.0.1"), procLE4("127.0.0.1")) // loopback
	w("proc/net/tcp", tcp)
	w("proc/net/tcp6", "  sl  local_address rem_address   st\n"+
		fmt.Sprintf("   0: %s:0016 %s:C354 01 0\n", procTCP6("2001:db8::1"), procTCP6("2001:db8::5"))+
		fmt.Sprintf("   1: %s:0016 %s:C355 01 0\n", procTCP6("::ffff:10.1.2.5"), procTCP6("::ffff:10.1.2.51")))
	for ifname, pci := range map[string]string{"ens193": "0000:0c:00.0", "ens224": "0000:13:00.0"} {
		if err := os.RemoveAll(filepath.Join(root, "sys/class/net", ifname)); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "sys/class/net", ifname), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../../../devices/pci0000:00/"+pci, filepath.Join(root, "sys/class/net", ifname, "device")); err != nil {
			t.Fatal(err)
		}
	}
	// "wan": a linux-cp tap (tun_flags, no device); "bond0": a kernel bond (no device, no tun_flags)
	for _, d := range []string{"wan", "bond0"} {
		if err := os.MkdirAll(filepath.Join(root, "sys/class/net", d), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	w("sys/class/net/wan/tun_flags", "0x1002\n")
	return root
}

// TestManagementPaths: re-review N4 — control connections on a connected subnet are protected, a
// default route through a VPP-owned linux-cp tap (and a blackhole default) does not stop rendering.
func TestManagementPaths(t *testing.T) {
	root := mgmtRoot(t, "wan")
	src := HostSources{Root: root, PluginDir: filepath.Join(root, "plugins"), CurrentConf: filepath.Join(root, "etc/startup.conf")}
	h, err := ReadHost(src)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.ManagementPCI, []string{"0000:0c:00.0", "0000:13:00.0"}) {
		t.Fatalf("management = %v, want the ssh NICs 0c (IPv4 + v4-mapped) and 13 (IPv6)", h.ManagementPCI)
	}
	notes := strings.Join(h.ManagementNotes, "\n")
	for _, want := range []string{
		"wan (default route) is a tun/tap interface (linux-cp, VPP-owned), not a kernel NIC: not protected",
		"ens193 → 0000:0c:00.0 (control connection from 10.1.2.50)",
		"ens224 → 0000:13:00.0 (control connection from 2001:db8::5)",
	} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes lack %q:\n%s", want, notes)
		}
	}
	if strings.Contains(notes, "ens161") {
		t.Errorf("a non-control connection marked ens161 as management:\n%s", notes)
	}
	// the reviewer's case: the ssh NIC 0c only carries a connected subnet — handing it to DPDK is refused
	h.OnlineCPUs, h.NUMANodes, h.HugepageBytes = []uint32{0, 1, 2, 3}, 1, 2<<30
	h.Plugins = vrxA(t).Plugins
	h.CurrentPlugins = map[string]bool{}
	if _, _, err := Generate(parseDoc(t, `{"dataplane":{"devices":{"0000:0c:00.0":{"name":"lan"}}}}`), h, DefaultSettings()); !errors.Is(err, ErrInput) ||
		!strings.Contains(err.Error(), "0000:0c:00.0 is the host's management NIC") {
		t.Fatalf("ssh NIC accepted as a DPDK device: %v", err)
	}
	out, _, err := Generate(parseDoc(t, `{"dataplane":{"devices":{"0000:04:00.0":{"name":"wan0"}}}}`), h, DefaultSettings())
	if err != nil || !strings.Contains(string(out), "blacklist 0000:0c:00.0") || !strings.Contains(string(out), "blacklist 0000:13:00.0") {
		t.Fatalf("%v\n%s", err, out)
	}

	// an extra control port adds the https peer's NIC (ens161 → 0000:04:00.0)
	src.ControlPorts = []uint16{22, 443}
	if h, err = ReadHost(src); err != nil || !slices.Contains(h.ManagementPCI, "0000:04:00.0") {
		t.Fatalf("control port 443: %v %v", h.ManagementPCI, err)
	}

	// a default route on a kernel bond without resolvable members needs --mgmt-pci (then it is noted)
	root = mgmtRoot(t, "bond0")
	if _, err := ReadHost(HostSources{Root: root, PluginDir: filepath.Join(root, "plugins")}); !errors.Is(err, ErrHost) ||
		!strings.Contains(err.Error(), "bond0 (default route): no PCI device underneath") {
		t.Fatalf("bond default: %v", err)
	}
	h, err = ReadHost(HostSources{Root: root, PluginDir: filepath.Join(root, "plugins"), MgmtPCI: []string{"0000:1b:00.0"}})
	if err != nil || !slices.Contains(h.ManagementPCI, "0000:1b:00.0") || !strings.Contains(strings.Join(h.ManagementNotes, "\n"), "unresolved bond0") {
		t.Fatalf("bond default with --mgmt-pci: %v %v %q", h.ManagementPCI, err, h.ManagementNotes)
	}
	// a bond whose members are PCI NICs resolves through lower_* (re-review N7)
	for _, m := range []string{"ens256", "ens257"} {
		pci := map[string]string{"ens256": "0000:1b:00.0", "ens257": "0000:1c:00.0"}[m]
		if err := os.MkdirAll(filepath.Join(root, "sys/class/net", m), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../../../devices/pci0000:00/"+pci, filepath.Join(root, "sys/class/net", m, "device")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../"+m, filepath.Join(root, "sys/class/net/bond0", "lower_"+m)); err != nil {
			t.Fatal(err)
		}
	}
	h, err = ReadHost(HostSources{Root: root, PluginDir: filepath.Join(root, "plugins")})
	if err != nil || !slices.Contains(h.ManagementPCI, "0000:1b:00.0") || !slices.Contains(h.ManagementPCI, "0000:1c:00.0") {
		t.Fatalf("bond members: %v %v", h.ManagementPCI, err)
	}
}

// TestManagementOnlyVPPOwned: the only default route is on a linux-cp tap and nobody is connected
// → no kernel management NIC is known and rendering is refused (Host.Check), never a guess.
func TestManagementOnlyVPPOwned(t *testing.T) {
	root := mgmtRoot(t, "wan")
	for _, f := range []string{"proc/net/tcp", "proc/net/tcp6"} {
		if err := os.Remove(filepath.Join(root, f)); err != nil {
			t.Fatal(err)
		}
	}
	h, err := ReadHost(HostSources{Root: root, PluginDir: filepath.Join(root, "plugins")})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.ManagementPCI) != 0 || !errors.Is(h.Check(), ErrHost) {
		t.Fatalf("management %v check %v", h.ManagementPCI, h.Check())
	}
	h.ManagementPCI = nil
	if h, err = ReadHost(HostSources{Root: root, PluginDir: filepath.Join(root, "plugins"), MgmtPCI: []string{"0000:0b:00.0"}}); err != nil || !slices.Equal(h.ManagementPCI, []string{"0000:0b:00.0"}) {
		t.Fatalf("--mgmt-pci fallback: %v %v", h.ManagementPCI, err)
	}
}
