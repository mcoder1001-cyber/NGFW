package sysident

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObservedStateSlotIsolationAndRuntimeFailure(t *testing.T) {
	root := t.TempDir()
	p := PathsUnder(root)
	p.ZoneinfoDir = filepath.Join(root, "zones")
	for _, dir := range append(p.Dirs(), p.ZoneinfoDir, filepath.Join(root, "proc")) {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(p.Hostname, "slot-router\n")
	write(filepath.Join(p.ZoneinfoDir, "UTC"), "zone")
	if err := os.Symlink(filepath.Join(p.ZoneinfoDir, "UTC"), p.Localtime); err != nil {
		t.Fatal(err)
	}
	write(p.ResolvedDropIn, "[Resolve]\nDNS=192.0.2.53 2001:db8::53\nDomains=example.test\n")
	write(filepath.Join(root, "proc/uptime"), "123.75 100.0\n")
	runtime := filepath.Join(root, "runtime")
	write(runtime, "nameserver 203.0.113.53\n")
	d := New(p, nil)
	st := d.State(filepath.Join(root, "proc"), runtime)
	if st.Hostname != "slot-router" || st.Timezone != "UTC" || st.UptimeSeconds == nil || *st.UptimeSeconds != 123.75 {
		t.Fatalf("state %#v", st)
	}
	if st.KernelHostname != nil || st.ResolverStatus != "slot-only" || len(st.ObservedNameServers) != 0 {
		t.Fatalf("slot leaked host state %#v", st)
	}
	if len(st.ConfiguredNameServers) != 2 || len(st.ConfiguredSearchDomains) != 1 {
		t.Fatalf("resolver %#v", st)
	}
	// Enabling product observation remains read-only and reports configured and observed servers separately.
	p.SetKernelHostname = true
	d = New(p, nil)
	st = d.State(filepath.Join(root, "proc"), runtime)
	if st.ResolverStatus != "observed" || len(st.ObservedNameServers) != 1 || st.ObservedNameServers[0] != "203.0.113.53" {
		t.Fatalf("runtime %#v", st)
	}
	if err := os.Remove(runtime); err != nil {
		t.Fatal(err)
	}
	st = d.State(filepath.Join(root, "proc"), runtime)
	if st.ResolverStatus != "unavailable" {
		t.Fatalf("missing runtime %#v", st)
	}
	if b, err := os.ReadFile(p.Hostname); err != nil || string(b) != "slot-router\n" {
		t.Fatalf("read-only state changed hostname")
	}
}

func TestObservedStateRejectsOversizeAndInvalidFacts(t *testing.T) {
	root := t.TempDir()
	p := PathsUnder(root)
	for _, dir := range append(p.Dirs(), filepath.Join(root, "proc")) {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(p.Hostname, []byte(strings.Repeat("a", 16385)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "proc/uptime"), []byte("NaN 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", p.Localtime); err != nil {
		t.Fatal(err)
	}
	st := New(p, nil).State(filepath.Join(root, "proc"), filepath.Join(root, "missing"))
	if st.Hostname != "" || st.Timezone != "" || st.UptimeSeconds != nil {
		t.Fatalf("invalid facts accepted %#v", st)
	}
}

func TestObservedStateRefusesDisconnectedManagedTargets(t *testing.T) {
	root := t.TempDir()
	p := PathsUnder(root)
	for _, dir := range p.Dirs() {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(p.Hostname, []byte("configured-private-target"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := New(p, nil)
	d.provisioned = func() error { return errors.New("public hostname link redirected") }
	st := d.State(filepath.Join(root, "proc"), filepath.Join(root, "runtime"))
	if st.Hostname != "" || st.Timezone != "" || st.KernelHostname != nil || st.UptimeSeconds != nil ||
		len(st.ConfiguredNameServers) != 0 || len(st.ObservedNameServers) != 0 ||
		st.ResolverStatus != "unavailable" || len(st.Errors) != 1 || st.Errors[0] != "provisioning" {
		t.Fatalf("disconnected targets presented as installed host facts: %#v", st)
	}
	if got, err := os.ReadFile(p.Hostname); err != nil || string(got) != "configured-private-target" {
		t.Fatal("read-only state mutated target")
	}
}
