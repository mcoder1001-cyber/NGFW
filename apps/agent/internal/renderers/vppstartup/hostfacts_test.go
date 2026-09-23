package vppstartup

import (
	"context"
	"errors"
	"maps"
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

// TestPluginOverlay: the current file's switches survive a document without `plugins`; the
// document wins per key.
func TestPluginOverlay(t *testing.T) {
	out, m, err := Generate(parseDoc(t, `{"dataplane":{"plugins":{"npt66_plugin.so":false,"acl_plugin.so":true}}}`), vrxA(t), DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"plugin acl_plugin.so { enable }", "plugin linux_cp_plugin.so { enable }",
		"plugin linux_nl_plugin.so { enable }", "plugin npt66_plugin.so { disable }",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %q", want)
		}
	}
	kept := 0
	for _, w := range m.Warnings {
		if strings.Contains(w, "kept from the current start-up file") {
			kept++
		}
	}
	if kept != 2 {
		t.Errorf("warnings %q (want the two inherited switches)", m.Warnings)
	}
}
