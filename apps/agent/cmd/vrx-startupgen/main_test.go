package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtures = "../../internal/renderers/vppstartup/testdata"

// hostFlags pins the host facts so the tests do not depend on the machine they run on; the
// plugin dir is a fake directory with the vrx-a plugin names.
func hostFlags(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	names, err := os.ReadFile(filepath.Join(fixtures, "plugins-vrx-a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range strings.Fields(string(names)) {
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(n)), nil, 0o600); err != nil { //nolint:gosec // fixture names from testdata
			t.Fatal(err)
		}
	}
	return []string{"--no-host", "--mgmt-pci", "0000:0b:00.0", "--plugin-dir", dir, "--online-cpus", "0-31", "--numa-nodes", "2",
		"--hugepages-mb", "2048", "--current", filepath.Join(fixtures, "host-startup.conf")}
}

func runCLI(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestRenderToStdoutMatchesGolden(t *testing.T) {
	code, out, stderr := runCLI(t, "", append(hostFlags(t), filepath.Join(fixtures, "cases", "six-nic-sample.json"))...)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want, err := os.ReadFile(filepath.Join(fixtures, "six-nic-sample.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if out != string(want) {
		t.Fatalf("stdout differs from the golden file:\n%s", out)
	}
}

func TestStdinAndOutputFile(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join(fixtures, "cases", "host-equivalent.json"))
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "startup.conf")
	code, out, stderr := runCLI(t, string(doc), append(hostFlags(t), "-o", dst)...)
	if code != 0 || out != "" {
		t.Fatalf("exit %d out %q err %s", code, out, stderr)
	}
	st, err := os.Stat(dst)
	if err != nil || st.Mode().Perm() != 0o644 {
		t.Fatalf("%v %v", st, err)
	}
	want, _ := os.ReadFile(filepath.Join(fixtures, "host-equivalent.golden"))
	got, _ := os.ReadFile(dst) //nolint:gosec // test temp file
	if !bytes.Equal(got, want) {
		t.Fatal("written file differs from golden")
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(dst), ".*tmp-*"))
	if len(leftovers) != 0 {
		t.Fatalf("temp files left: %v", leftovers)
	}

	// --diff against the file just written: identical → exit 0, no output
	code, out, _ = runCLI(t, string(doc), append(hostFlags(t), "--diff", dst)...)
	if code != 0 || out != "" {
		t.Fatalf("identical diff: exit %d %q", code, out)
	}
}

func TestDiffAgainstHostFile(t *testing.T) {
	host := filepath.Join(fixtures, "host-startup.conf")
	doc := filepath.Join(fixtures, "cases", "host-equivalent.json")
	code, out, _ := runCLI(t, "", append(hostFlags(t), "--diff", host, doc)...)
	if code != 1 || !strings.HasPrefix(out, "--- "+host+"\n+++ rendered\n") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	code, out, _ = runCLI(t, "", append(hostFlags(t), "--diff", host, "--semantic", doc)...)
	want := "+ cpu > main-core 1\n+ statseg > socket-name /run/vpp/stats.sock\n+ statseg {}\n"
	if code != 1 || out != want {
		t.Fatalf("semantic: exit %d:\n%s", code, out)
	}
	// semantic diff of the rendering with itself is empty
	dst := filepath.Join(t.TempDir(), "s.conf")
	if code, _, e := runCLI(t, "", append(hostFlags(t), "-o", dst, doc)...); code != 0 {
		t.Fatal(e)
	}
	if code, out, _ := runCLI(t, "", append(hostFlags(t), "--diff", dst, "--semantic", doc)...); code != 0 || out != "" {
		t.Fatalf("self semantic diff: %d %q", code, out)
	}
}

func TestCheckAndErrors(t *testing.T) {
	flags := hostFlags(t)
	code, out, stderr := runCLI(t, `{"dataplane":{"pciWhitelist":["0000:04:00.0"],"managementPci":["0000:0b:00.0"]}}`, append(flags, "--check")...)
	if code != 0 || out != "" || !strings.Contains(stderr, "warning: dataplane: DPDK device 0000:04:00.0 has no logical name") || !strings.Contains(stderr, "ok (1 DPDK device(s)") {
		t.Fatalf("check: %d %q %q", code, out, stderr)
	}
	for name, c := range map[string]struct {
		stdin string
		args  []string
		want  string
	}{
		"mgmt in dev list": {`{"dataplane":{"pciWhitelist":["0000:0b:00.0"]}}`, nil, "0000:0b:00.0 is the host's management NIC"},
		"reviewer repro":   {`{"dataplane":{"managementPci":["0000:04:00.0"],"devices":{"0000:0b:00.0":{"name":"lan"}}}}`, nil, "does not match the host's management NIC(s) 0000:0b:00.0"},
		"typo key":         {`{"dataplane":{"maincore":3}}`, nil, "unknown field"},
		"mgmt-if no-host":  {`{}`, []string{"--mgmt-if", "ens192"}, "--mgmt-if needs the host's /sys"},
		"bad mgmt-pci":     {`{}`, []string{"--mgmt-pci", "0b:00.0"}, "--mgmt-pci"},
		"bad online":       {`{}`, []string{"--online-cpus", "x"}, "--online-cpus"},
		"bad current":      {`{}`, []string{"--current", "/nonexistent/startup.conf"}, "no such file"},
		"hugepages 0":      {`{}`, []string{"--hugepages-mb", "0"}, "no hugepages"},
		"not json":         {`dataplane {`, nil, "not a JSON object"},
		"array doc":        {`[1]`, nil, "not a JSON object"},
		"duplicate key":    {`{"dataplane":{"devices":{"0000:04:00.0":{"name":"a"},"0000:04:00.0":{"name":"b"}}}}`, nil, "duplicate"},
		"semantic alone":   {`{}`, []string{"--semantic"}, "--semantic needs --diff"},
		"check with -o":    {`{}`, []string{"--check", "-o", "/nonexistent/x"}, "cannot be combined"},
		"two documents":    {`{}`, []string{"a.json", "b.json"}, "at most one document"},
		"missing diff":     {`{}`, []string{"--diff", "/nonexistent/startup.conf"}, "no such file"},
		"missing input":    {``, []string{"/nonexistent/doc.json"}, "no such file"},
		"bad isolcpus":     {`{}`, []string{"--isolcpus", "x"}, "--isolcpus"},
		"bad flag":         {`{}`, []string{"--nope"}, "flag provided but not defined"},
		"empty plugin dir": {`{}`, []string{"--plugin-dir", "/nonexistent-dir"}, "no plugins found"},
	} {
		t.Run(name, func(t *testing.T) {
			code, out, stderr := runCLI(t, c.stdin, append(slicesClone(flags), c.args...)...)
			if code != 2 || out != "" || !strings.Contains(stderr, c.want) {
				t.Fatalf("exit %d out %q stderr %q (want %q)", code, out, stderr, c.want)
			}
		})
	}
	// --no-host without a management NIC never renders
	if code, _, stderr := runCLI(t, `{}`, "--no-host", "--online-cpus", "0-3"); code != 2 || !strings.Contains(stderr, "management NIC is unknown") {
		t.Errorf("no mgmt: %d %q", code, stderr)
	}
	if code, _, _ := runCLI(t, "", "-h"); code != 0 {
		t.Errorf("-h exit %d", code)
	}
}

func slicesClone(s []string) []string { return append([]string(nil), s...) }

// TestHostFactsFromSysRoot reads a fake /sys + /proc tree: the management NIC comes from the
// default-route interface, flags override the rest.
func TestHostFactsFromSysRoot(t *testing.T) {
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
	write("sys/devices/system/node/node0/x", "")
	write("sys/devices/system/node/node1/x", "")
	write("proc/meminfo", "HugePages_Total:    1024\nHugepagesize:       2048 kB\n")
	write("proc/net/route", "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\nens192\t00000000\t017E1EAC\t0003\t0\t0\t100\t00000000\t0\t0\t0\n")
	if err := os.MkdirAll(filepath.Join(root, "sys/class/net/ens192"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../devices/pci0000:00/0000:0b:00.0", filepath.Join(root, "sys/class/net/ens192/device")); err != nil {
		t.Fatal(err)
	}
	write("plugins/dpdk_plugin.so", "")

	o := options{sysRoot: root, pluginDir: filepath.Join(root, "plugins"), current: "none"}
	h, err := hostFacts(o, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.ManagementPCI) != 1 || h.ManagementPCI[0] != "0000:0b:00.0" || len(h.OnlineCPUs) != 8 || h.NUMANodes != 2 || h.HugepageBytes != 2<<30 || len(h.Plugins) != 1 {
		t.Fatalf("%+v", h)
	}
	o.onlineCPUs, o.isolcpus, o.numaNodes = "0-3", "2-3", 1
	h, err = hostFacts(o, map[string]bool{"online-cpus": true, "isolcpus": true, "numa-nodes": true})
	if err != nil || len(h.OnlineCPUs) != 4 || len(h.IsolCPUs) != 2 || h.NUMANodes != 1 {
		t.Fatalf("%+v %v", h, err)
	}
	// no default route and no --mgmt-pci: refused
	write("proc/net/route", "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n")
	if _, err := hostFacts(o, map[string]bool{}); err == nil || !strings.Contains(err.Error(), "management NIC is unknown") {
		t.Fatalf("no mgmt NIC: %v", err)
	}
}
