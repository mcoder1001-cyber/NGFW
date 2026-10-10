package pppoe

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIPv4HookUsesPppdSixArgumentABI(t *testing.T) {
	session := minimalSession()
	_, paths := renderTo(t, session)
	hook := filepath.Join(paths.IPUpDir, "ngfw-"+session.HostIf)
	state := filepath.Join(paths.StateDir, session.HostIf+".state")
	for _, args := range [][]string{{}, {"ppp0", "dev", "0", "192.0.2.1", "192.0.2.2", "foreign"}, {"bad/name", "dev", "0", "192.0.2.1", "192.0.2.2", session.Remotename()}} {
		cmd := exec.Command(hook, args...) // #nosec G204 -- Renderer-created executable under this test TempDir; fixed six-argument ABI controls.
		cmd.Env = []string{"PATH=/usr/bin:/bin"}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("negative hook %v: %s", err, out)
		}
		if _, err := os.Stat(state); !os.IsNotExist(err) {
			t.Fatal("foreign/missing ABI wrote state")
		}
	}
	cmd := exec.Command(hook, "ppp0", "dev", "0", "192.0.2.1", "192.0.2.2", session.Remotename()) // #nosec G204 -- Same renderer-created private test hook, fixed positive ABI arguments.
	cmd.Env = []string{"PATH=/usr/bin:/bin", "IPLOCAL=192.0.2.1", "IPREMOTE=192.0.2.2"}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("actual ABI %v: %s", err, out)
	}
	data, err := os.ReadFile(state) // #nosec G304 -- Fixed renderer state filename below this test TempDir; no caller-controlled path.
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"phase=up", "ppp_iface=ppp0", "local=192.0.2.1", "peer=192.0.2.2"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %s: %s", want, data)
		}
	}
}

func TestIPv6HookSixArgumentABIReachesOwnershipGuard(t *testing.T) {
	session := minimalSession()
	session.IPv6 = "slaac"
	_, paths := renderTo(t, session)
	hook := filepath.Join(paths.IPv6UpDir, "ngfw-"+session.HostIf)
	if err := os.WriteFile(filepath.Join(paths.StateDir, session.HostIf+".ipv6.admission"), []byte(strings.Repeat("a", 32)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{}, {"ppp0", "dev", "0", "fe80::1", "fe80::2", "foreign"}} {
		cmd := exec.Command(hook, args...) // #nosec G204 -- Renderer-created executable under this test TempDir; fixed six-argument ABI controls.
		cmd.Env = []string{"PATH=/usr/bin:/bin"}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("negative hook %v: %s", err, out)
		}
	}
	cmd := exec.Command(hook, "ppp0", "dev", "0", "fe80::1", "fe80::2", session.Remotename()) // #nosec G204 -- Same renderer-created private test hook, fixed positive IPv6 ABI arguments.
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LLLOCAL=fe80::1", "LLREMOTE=fe80::2", "PPPD_PID=2147483647"}
	// The actual helper fences a departed daemon without publishing replacement state.
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("actual IPv6 ABI: %v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(paths.StateDir, session.HostIf+".ipv6.action.lock")); err != nil {
		t.Fatalf("ABI skipped actual helper admission guard: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.StateDir, session.HostIf+".ipv6.state")); !os.IsNotExist(err) {
		t.Fatal("departed daemon published IPv6 state")
	}
}

func TestCarrierPeerProvidesHookIdentityToFixedDialer(t *testing.T) {
	spec, err := NewCarrierSpec("ngfw", "pppwan", "wan", 1492)
	if err != nil {
		t.Fatal(err)
	}
	session := minimalSession()
	session.Carrier = &spec
	session.Iface = spec.Logical
	session.HostIf = spec.RawHost()
	session.MTU = spec.MTU
	r := New(WithPaths(PathsUnder(t.TempDir())))
	files, err := r.RenderCarrier(session)
	if err != nil {
		t.Fatal(err)
	}
	data := files[filepath.Join(r.paths.PeersDir, "carrier")].Content
	if !strings.Contains(string(data), "\nipparam "+session.Remotename()+"\n") {
		t.Fatal("fixed carrier dialer cannot pass hook identity")
	}
}
