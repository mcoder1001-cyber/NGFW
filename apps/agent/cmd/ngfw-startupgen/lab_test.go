package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLabSlot: --lab-slot renders the slot instance exactly like the renderer's golden file, writes
// it with -o, and refuses everything that could reach the shared VPP.
func TestLabSlot(t *testing.T) {
	doc := filepath.Join(fixtures, "lab", "slot.json")
	code, out, stderr := runCLI(t, "", append(hostFlags(t), "--lab-slot", "20", doc)...)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want, err := os.ReadFile(filepath.Join(fixtures, "lab-slot-20.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if out != string(want) {
		t.Fatalf("stdout differs from lab-slot-20.golden:\n%s", out)
	}
	if !strings.Contains(stderr, "lab slot 20 instance in /run/ngfw-test/w20/vpp (api /run/ngfw-test/w20/vpp/api.sock") {
		t.Fatalf("stderr: %s", stderr)
	}

	root := t.TempDir()
	dst := filepath.Join(root, "startup.conf")
	code, _, stderr = runCLI(t, "", append(hostFlags(t), "--lab-slot", "3", "--lab-root", root, "-o", dst, doc)...)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	got, _ := os.ReadFile(dst) //nolint:gosec // test temp file
	if !strings.Contains(string(got), "runtime-dir "+filepath.Join(root, "w3", "vpp")) || strings.Contains(string(got), "/run/vpp") {
		t.Fatalf("lab-root rendering:\n%s", got)
	}

	for name, c := range map[string]struct {
		stdin string
		args  []string
		want  string
	}{
		"slot 13":         {"", []string{"--lab-slot", "13", doc}, "13 is reserved for tools/app"},
		"slot 0":          {"", []string{"--lab-slot", "0", doc}, "must be 1..12 or 14..32"},
		"slot 33":         {"", []string{"--lab-slot", "33", doc}, "must be 1..12 or 14..32"},
		"root inside vpp": {"", []string{"--lab-slot", "3", "--lab-root", "/run/vpp", doc}, "inside the shared VPP"},
		"relative root":   {"", []string{"--lab-slot", "3", "--lab-root", "run", doc}, "absolute"},
		"root alone":      {"", []string{"--lab-root", root, doc}, "--lab-root needs --lab-slot"},
		"dpdk enabled":    {`{"dataplane":{"plugins":{"switches":{"dpdk_plugin.so":true}}}}`, []string{"--lab-slot", "3"}, "must disable dpdk_plugin.so"},
		"dpdk kept":       {`{"dataplane":{}}`, []string{"--lab-slot", "3"}, "must disable dpdk_plugin.so"},
	} {
		t.Run(name, func(t *testing.T) {
			code, out, stderr := runCLI(t, c.stdin, append(hostFlags(t), c.args...)...)
			if code != 2 || out != "" || !strings.Contains(stderr, c.want) {
				t.Fatalf("exit %d out %q stderr %q (want %q)", code, out, stderr, c.want)
			}
		})
	}
}
