package vppstartup

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

// vrxA returns the facts of the dev host vrx-a (docs/lab/host-vrx-a.md, verified 2026-09-24:
// 32 CPUs, 2 NUMA nodes, 1024 × 2 MB hugepages, no isolcpus) with the on-disk plugin list
// frozen in testdata/plugins-vrx-a.txt so the tests are hermetic.
func vrxA(t *testing.T) Host {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "plugins-vrx-a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return Host{CPUs: 32, NUMANodes: 2, HugepageBytes: 2 << 30, Plugins: strings.Fields(string(b))}
}

func loadDoc(t *testing.T, path string) *structpb.Struct {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test fixture
	if err != nil {
		t.Fatal(err)
	}
	return parseDoc(t, string(b))
}

func parseDoc(t *testing.T, js string) *structpb.Struct {
	t.Helper()
	doc := &structpb.Struct{}
	if err := protojson.Unmarshal([]byte(js), doc); err != nil {
		t.Fatalf("fixture %q: %v", js, err)
	}
	return doc
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // test fixture
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		d, _ := UnifiedDiff(path, "got", want, got)
		t.Errorf("%s mismatch (go test -update to accept):\n%s", name, d)
	}
}

// TestGolden renders every testdata/cases/<name>.json against the vrx-a host facts and compares
// with testdata/<name>.golden byte for byte. Cases: empty document, host-equivalent (the current
// hand-written /etc/vpp/startup.conf semantics), single-core lab, 2-worker lab (corelist and
// auto-pinned), the six-NIC vrx-a layout with a SAMPLE port-group mapping (the real mapping is
// still pending from the product owner — Q2), plugins block, dpdk disabled, whitelist only.
func TestGolden(t *testing.T) {
	cases, err := filepath.Glob(filepath.Join("testdata", "cases", "*.json"))
	if err != nil || len(cases) < 9 {
		t.Fatalf("cases: %v %v", cases, err)
	}
	host := vrxA(t)
	for _, c := range cases {
		name := strings.TrimSuffix(filepath.Base(c), ".json")
		t.Run(name, func(t *testing.T) {
			out, _, err := Generate(loadDoc(t, c), host, DefaultSettings())
			if err != nil {
				t.Fatal(err)
			}
			golden(t, name, out)
			// every rendering parses and is stable
			if _, err := Parse(out); err != nil {
				t.Fatalf("rendered file does not parse: %v", err)
			}
			again, _, err := Generate(loadDoc(t, c), host, DefaultSettings())
			if err != nil || !bytes.Equal(out, again) {
				t.Fatalf("rendering is not deterministic (err %v)", err)
			}
		})
	}
}

// TestSixNICSample pins the acceptance shape: each data NIC as `dev <pci> { name <logical> }`,
// the management NIC blacklisted and never a dev, no no-pci.
func TestSixNICSample(t *testing.T) {
	out, m, err := Generate(loadDoc(t, "testdata/cases/six-nic-sample.json"), vrxA(t), DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		"  dev 0000:04:00.0 {\n    name wan\n  }",
		"  dev 0000:0c:00.0 {\n    name lan\n  }",
		"  dev 0000:13:00.0 {\n    name dmz\n  }",
		"  dev 0000:14:00.0 {\n    name p2p\n  }",
		"  dev 0000:1b:00.0 {\n    name lan2\n    num-rx-queues 1\n  }",
		"  dev 0000:1c:00.0 {\n    name sync\n    num-rx-desc 512\n    num-tx-desc 512\n  }",
		"  blacklist 0000:0b:00.0\n",
		"  corelist-workers 2-3\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(s, "dev 0000:0b:00.0") || strings.Contains(s, "no-pci") {
		t.Error("management NIC listed as a device, or no-pci with devices")
	}
	if len(m.Devices) != 6 || len(m.Warnings) != 0 {
		t.Errorf("devices %d warnings %v", len(m.Devices), m.Warnings)
	}
}

// TestHostEquivalentSemantics renders the equivalent of the current hand-written host file
// (testdata/host-startup.conf = /etc/vpp/startup.conf on vrx-a, 2026-09-24) and diffs it
// semantically: plugins, blacklist, no-pci, unix/api/socksvr/cpu all equal; the only addition
// is the explicit statseg default socket.
func TestHostEquivalentSemantics(t *testing.T) {
	out, _, err := Generate(loadDoc(t, "testdata/cases/host-equivalent.json"), vrxA(t), DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.ReadFile("testdata/host-startup.conf")
	if err != nil {
		t.Fatal(err)
	}
	assertHostSemantics(t, host, out)
}

// TestLiveHostFileSemantics repeats the check against the live file when it is readable, so a
// hand edit on vrx-a that the generator cannot express shows up here. Read only.
func TestLiveHostFileSemantics(t *testing.T) {
	live, err := os.ReadFile(DefaultConfPath)
	if err != nil {
		t.Skipf("no readable %s: %v", DefaultConfPath, err)
	}
	out, _, err := Generate(loadDoc(t, "testdata/cases/host-equivalent.json"), vrxA(t), DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	assertHostSemantics(t, live, out)
}

func assertHostSemantics(t *testing.T, host, rendered []byte) {
	t.Helper()
	onlyHost, onlyRendered, err := SemanticDiff(host, rendered)
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyHost) != 0 {
		t.Errorf("host file entries the generator does not reproduce:\n  %s", strings.Join(onlyHost, "\n  "))
	}
	wantExtra := []string{"statseg > socket-name /run/vpp/stats.sock", "statseg {}"}
	if !slices.Equal(onlyRendered, wantExtra) {
		t.Errorf("rendered-only entries = %q, want only the explicit statseg default %q", onlyRendered, wantExtra)
	}
	p, err := Parse(rendered)
	if err != nil {
		t.Fatal(err)
	}
	canon := p.Canonical()
	for _, want := range []string{
		"dpdk > blacklist 0000:0b:00.0",
		"dpdk > no-pci",
		"plugins > plugin linux_cp_plugin.so > enable",
		"plugins > plugin linux_nl_plugin.so > enable",
		"plugins > plugin npt66_plugin.so > enable",
	} {
		if !slices.Contains(canon, want) {
			t.Errorf("rendered file lacks %q", want)
		}
	}
}

func TestRendererInterface(t *testing.T) {
	dir := t.TempDir()
	s := DefaultSettings()
	s.ConfPath = filepath.Join(dir, "startup.conf")
	r := New(WithHost(vrxA(t)), WithSettings(s))
	if r.Name() != "vpp-startup" {
		t.Fatal(r.Name())
	}
	ctx := context.Background()
	files, err := r.Render(ctx, loadDoc(t, "testdata/cases/six-nic-sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[s.ConfPath].Mode != 0o644 || files[s.ConfPath].Secret {
		t.Fatalf("files = %v", files.Paths())
	}
	if err := r.Validate(ctx, files); err != nil {
		t.Fatal(err)
	}
	bad := renderers.Files{s.ConfPath: {Mode: 0o644, Content: []byte("dpdk {\n")}}
	if err := r.Validate(ctx, bad); !errors.Is(err, renderers.ErrInvalidFiles) {
		t.Fatalf("unbalanced file accepted: %v", err)
	}
	extra := renderers.Files{s.ConfPath: files[s.ConfPath], filepath.Join(dir, "x"): {Mode: 0o644}}
	if err := r.Validate(ctx, extra); !errors.Is(err, renderers.ErrInvalidFiles) {
		t.Fatalf("extra file accepted: %v", err)
	}
	if err := r.Apply(ctx, files); !errors.Is(err, ErrManagerStep) {
		t.Fatalf("Apply = %v", err)
	}
	if _, err := os.Stat(s.ConfPath); !os.IsNotExist(err) {
		t.Fatal("Apply wrote a file")
	}
	if _, err := r.Retrieve(ctx); !errors.Is(err, ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve = %v", err)
	}
	// typed inputs: DesiredState / DataplaneConfig / nil
	for _, msg := range []proto.Message{
		nil,
		&vrxv1.DesiredState{},
		&vrxv1.DesiredState{Dataplane: &vrxv1.DataplaneConfig{Workers: proto.Uint32(2), MainCore: proto.Uint32(1)}},
		&vrxv1.DataplaneConfig{},
	} {
		if _, err := r.Render(ctx, msg); err != nil {
			t.Errorf("Render(%T) = %v", msg, err)
		}
	}
	if _, err := r.Render(ctx, &vrxv1.SystemConfig{}); !errors.Is(err, ErrInput) {
		t.Errorf("wrong input type accepted: %v", err)
	}
}

func TestTypedInputMatchesDocument(t *testing.T) {
	typed := &vrxv1.DataplaneConfig{Workers: proto.Uint32(2), Corelist: []uint32{2, 3}, MainCore: proto.Uint32(1), RxQueues: proto.Uint32(2), HugepagesGb: proto.Uint32(2), PciWhitelist: []string{}}
	doc := parseDoc(t, `{"dataplane":{"workers":2,"corelist":[2,3],"mainCore":1,"rxQueues":2,"hugepagesGb":2}}`)
	a, _, err := Generate(typed, vrxA(t), DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := Generate(doc, vrxA(t), DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("typed and document input differ:\n%s\n---\n%s", a, b)
	}
}

// TestPluginListMatchesHost keeps testdata/plugins-vrx-a.txt honest on vrx-a.
func TestPluginListMatchesHost(t *testing.T) {
	onDisk, _ := filepath.Glob("/usr/lib/x86_64-linux-gnu/vpp_plugins/*.so")
	if len(onDisk) == 0 {
		t.Skip("no VPP plugin directory on this host")
	}
	names := make([]string, 0, len(onDisk))
	for _, p := range onDisk {
		names = append(names, filepath.Base(p))
	}
	slices.Sort(names)
	frozen := vrxA(t).Plugins
	slices.Sort(frozen)
	if !slices.Equal(names, frozen) {
		t.Logf("on-disk plugin list differs from testdata/plugins-vrx-a.txt (VPP rebuilt?) — refresh the fixture:\n disk %v\n file %v", names, frozen)
	}
	for _, p := range names {
		if err := PluginName(p); err != nil {
			t.Errorf("on-disk plugin rejected by PluginName: %v", err)
		}
	}
}

func TestWarnings(t *testing.T) {
	_, m, err := Generate(loadDoc(t, "testdata/cases/whitelist-only.json"), vrxA(t), DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Warnings) != 1 || !strings.Contains(m.Warnings[0], "0000:04:00.0 has no logical name") {
		t.Errorf("warnings = %q", m.Warnings)
	}
	h := vrxA(t)
	h.HugepageBytes = 1 << 30
	_, m, err = Generate(parseDoc(t, `{"dataplane":{"hugepagesGb":2}}`), h, DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Warnings) != 1 || !strings.Contains(m.Warnings[0], "host reserves only 1 GiB") {
		t.Errorf("warnings = %q", m.Warnings)
	}
}

func TestCPUList(t *testing.T) {
	if got := FormatCPUList([]uint32{8, 3, 2, 6, 7, 2}); got != "2-3,6-8" {
		t.Error(got)
	}
	if got := FormatCPUList([]uint32{5}); got != "5" {
		t.Error(got)
	}
	got, err := ParseCPUList("0-3,8,10-11\n")
	if err != nil || !slices.Equal(got, []uint32{0, 1, 2, 3, 8, 10, 11}) {
		t.Error(got, err)
	}
	if got, err := ParseCPUList(" \n"); err != nil || got != nil {
		t.Error(got, err)
	}
	for _, bad := range []string{"a", "3-1", "1-", "0-5000", "1,,2"} {
		if _, err := ParseCPUList(bad); err == nil {
			t.Errorf("ParseCPUList(%q) accepted", bad)
		}
	}
}
