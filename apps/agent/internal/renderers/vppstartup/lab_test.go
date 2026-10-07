package vppstartup

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"ngfw/agent/internal/renderers"
)

// labDoc is the document tools/lab renders a slot instance from (testdata/lab/slot.json: no
// workers, 4096 buffers, dpdk/linux_cp/linux_nl disabled, npt66 enabled, an explicit main core —
// tools/lab's lab_vpp_document with slot 20 on a 32-CPU host).
func labDoc(t *testing.T) []byte {
	t.Helper()
	out, _, err := Generate(loadDoc(t, filepath.Join("testdata", "lab", "slot.json")), ngfwA(t), mustLab(t, DefaultLabRoot, 20))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mustLab(t *testing.T, root string, slot int) Settings {
	t.Helper()
	s, err := LabSlotSettings(root, slot)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestLabSlotGolden: slot 20's instance file byte for byte, and the fail-closed check accepts it.
func TestLabSlotGolden(t *testing.T) {
	out := labDoc(t)
	golden(t, "lab-slot-20", out)
	if err := CheckLabRendering(out, mustLab(t, DefaultLabRoot, 20)); err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if strings.Contains(text, "/run/vpp") {
		t.Fatalf("a slot rendering names the shared VPP's runtime dir:\n%s", text)
	}
	for _, want := range []string{
		"runtime-dir /run/ngfw-test/w20/vpp", "cli-listen /run/ngfw-test/w20/vpp/cli.sock", "log /run/ngfw-test/w20/vpp/vpp.log",
		"prefix w20", "socket-name /run/ngfw-test/w20/vpp/api.sock", "socket-name /run/ngfw-test/w20/vpp/stats.sock",
		"main-heap-size 512M", "main-heap-page-size 4k", "page-size 4k", "buffers-per-numa 4096", "gid vpp",
		"plugin dpdk_plugin.so { disable }", "main-core 21",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("slot rendering lacks %q", want)
		}
	}
	if strings.Contains(text, "dpdk {") {
		t.Error("slot rendering has a dpdk section")
	}
}

// TestDefaultSettingsUnchanged: the lab fields are empty in DefaultSettings, so the appliance
// rendering has none of their lines (the golden files of TestGolden pin the bytes).
func TestDefaultSettingsUnchanged(t *testing.T) {
	out, _, err := Generate(parseDoc(t, `{}`), ngfwA(t), DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	for _, never := range []string{"runtime-dir", "poll-sleep-usec", "prefix", "memory {", "main-heap", "page-size", "size "} {
		if strings.Contains(string(out), never) {
			t.Errorf("appliance rendering contains lab line %q", never)
		}
	}
	if !strings.Contains(string(out), "socksvr {\n  default\n}") {
		t.Error("appliance rendering lost socksvr { default }")
	}
}

func TestLabSlotNumbers(t *testing.T) {
	for _, ok := range []int{1, 11, 12, 14, 20, 32} {
		if _, err := LabSlotSettings(DefaultLabRoot, ok); err != nil {
			t.Errorf("slot %d: %v", ok, err)
		}
	}
	for _, bad := range []int{-1, 0, 13, 33, 100} {
		if _, err := LabSlotSettings(DefaultLabRoot, bad); !errors.Is(err, ErrLabSlot) {
			t.Errorf("slot %d: want ErrLabSlot, got %v", bad, err)
		}
	}
	for _, root := range []string{"", "relative", "/run/vpp", "/run/vpp/x", "/run/ngfw-test/../vpp", "/tmp/a b"} {
		if _, err := LabSlotSettings(root, 3); err == nil {
			t.Errorf("root %q accepted", root)
		}
	}
	s := mustLab(t, "/tmp/lab", 3)
	if s.RuntimeDir != "/tmp/lab/w3/vpp" || s.APISocket != "/tmp/lab/w3/vpp/api.sock" || s.APIPrefix != "w3" || s.ConfPath != "/tmp/lab/w3/vpp/startup.conf" {
		t.Fatalf("%+v", s)
	}
}

// TestCheckLabRenderingFailsClosed: every way a lab rendering could reach the shared VPP is refused.
func TestCheckLabRenderingFailsClosed(t *testing.T) {
	good := string(labDoc(t))
	s := mustLab(t, DefaultLabRoot, 20)
	for name, mutate := range map[string]func(string) string{
		"socksvr default": func(c string) string {
			return strings.Replace(c, "socket-name /run/ngfw-test/w20/vpp/api.sock", "default", 1)
		},
		"shared cli socket": func(c string) string {
			return strings.Replace(c, "cli-listen /run/ngfw-test/w20/vpp/cli.sock", "cli-listen /run/vpp/cli.sock", 1)
		},
		"shared runtime dir": func(c string) string {
			return strings.Replace(c, "runtime-dir /run/ngfw-test/w20/vpp", "runtime-dir /run/vpp", 1)
		},
		"no runtime dir": func(c string) string { return strings.Replace(c, "  runtime-dir /run/ngfw-test/w20/vpp\n", "", 1) },
		"no prefix":      func(c string) string { return strings.Replace(c, "  prefix w20\n", "", 1) },
		"other prefix":   func(c string) string { return strings.Replace(c, "prefix w20", "prefix w2", 1) },
		"dpdk section":   func(c string) string { return c + "\ndpdk {\n  no-pci\n}\n" },
		"unbalanced":     func(c string) string { return c + "\n}\n" },
	} {
		if err := CheckLabRendering([]byte(mutate(good)), s); !errors.Is(err, ErrLabSlot) {
			t.Errorf("%s: want ErrLabSlot, got %v", name, err)
		}
	}
	if err := CheckLabRendering([]byte(good), DefaultSettings()); !errors.Is(err, ErrLabSlot) {
		t.Errorf("appliance settings accepted as a lab instance: %v", err)
	}
}

func TestSettingsPageSizeValidated(t *testing.T) {
	m, err := BuildModel(nil, ngfwA(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Settings{
		{LogFile: "/l", CLISocket: "/c", StatsSocket: "/s", Group: "vpp", BuffersPageSize: "4k }"},
		{LogFile: "/l", CLISocket: "/c", StatsSocket: "/s", Group: "vpp", MainHeapPageSize: "3m"},
		{LogFile: "/l", CLISocket: "/c", StatsSocket: "/s", Group: "vpp", MainHeapMB: 8},
		{LogFile: "/l", CLISocket: "/c", StatsSocket: "/s", Group: "vpp", APISocket: "/a b"},
		{LogFile: "/l", CLISocket: "/c", StatsSocket: "/s", Group: "vpp", APIPrefix: "w1 }"},
	} {
		if _, err := RenderModel(m, bad); !errors.Is(err, renderers.ErrUnsafe) {
			t.Errorf("%+v: want ErrUnsafe, got %v", bad, err)
		}
	}
}
