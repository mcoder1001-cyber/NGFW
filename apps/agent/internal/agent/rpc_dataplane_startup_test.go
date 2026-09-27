package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// startupRoot is a minimal /sys + /proc tree (ens192 = 0000:0b:00.0 carries the default route) with an
// installed start-up file running 2 workers.
func startupRoot(t *testing.T) startupSources {
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
	write("sys/devices/system/cpu/online", "0-7\n")
	write("sys/devices/system/node/node0/cpulist", "0-7\n")
	write("proc/meminfo", "HugePages_Total:    1024\nHugePages_Free:      512\nHugepagesize:       2048 kB\n")
	write("proc/net/route", "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n"+
		"ens192\t00000000\t017E1EAC\t0003\t0\t0\t100\t00000000\t0\t0\t0\n")
	write("proc/net/ipv6_route", "")
	dir := filepath.Join(root, "sys/class/net/ens192")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../devices/pci0000:00/0000:00:15.0/0000:0b:00.0", filepath.Join(dir, "device")); err != nil {
		t.Fatal(err)
	}
	write("plugins/dpdk_plugin.so", "")
	write("plugins/linux_cp_plugin.so", "")
	write("etc/startup.conf", "cpu {\n  main-core 1\n  workers 2\n}\nplugins {\n  plugin linux_cp_plugin.so { enable }\n}\n")
	return startupSources{conf: filepath.Join(root, "etc/startup.conf"), pluginDir: filepath.Join(root, "plugins"), sysRoot: root}
}

func TestDataplaneStartupState(t *testing.T) {
	st := startupState(startupRoot(t))
	if st.Error != "" || !st.StartupPresent || st.GetWorkers() != 2 || st.GetMainCore() != 1 || st.OnlineCpus != "0-7" {
		t.Fatalf("%+v", st)
	}
	if st.HugepagesTotalBytes != 2<<30 || st.HugepagesFreeBytes != 1<<30 || !st.Plugins["linux_cp_plugin.so"] {
		t.Fatalf("%+v", st)
	}
	missing := startupSources{conf: filepath.Join(t.TempDir(), "none.conf"), sysRoot: t.TempDir()}
	st = startupState(missing)
	if st.StartupPresent || st.Error == "" {
		t.Fatalf("missing file / sys root: %+v", st)
	}
}

func TestDataplaneStartupPreview(t *testing.T) {
	src := startupRoot(t)
	w := uint32(4)
	out, err := startupPreview(src, &vrxv1.DataplaneConfig{Workers: &w})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Changed || !strings.Contains(out.Diff, "-  workers 2") || !strings.Contains(out.Rendered, "corelist-workers 2-5") || len(out.Sha256) != 64 {
		t.Fatalf("diff:\n%s\nrendered:\n%s", out.Diff, out.Rendered)
	}
	before, _ := os.ReadFile(src.conf)
	if !strings.Contains(string(before), "workers 2") {
		t.Fatal("the preview must not write the installed file")
	}
	// a document error is INVALID_ARGUMENT
	bad := "not-a-plugin"
	_, err = startupPreview(src, &vrxv1.DataplaneConfig{Plugins: &vrxv1.PluginSet{Switches: map[string]bool{bad: true}}})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("err = %v", err)
	}
}
