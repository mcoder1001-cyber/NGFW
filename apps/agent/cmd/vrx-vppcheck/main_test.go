package main

import (
	"bytes"
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/binapi/vpe"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/fake"
)

const showPlugins = ` Plugin path is: /usr/lib/x86_64-linux-gnu/vpp_plugins, plugins shown in load order

     Plugin                                   Version                          Description
  1. abf_plugin.so                            26.06-release                    Access Control List (ACL) Based Forwarding
  2. dpdk_plugin.so                           26.06-release                    Data Plane Development Kit (DPDK)
 12. linux_cp_plugin.so                       26.06-release                    Linux Control Plane - Interface Mirror
`

// noBackstop keeps the hard-exit timer from ever firing inside a test process.
func noBackstop(time.Duration, func()) *time.Timer { return time.AfterFunc(time.Hour, func() {}) }

func fakeVPP() *fake.Client {
	f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
	f.Reply("show_version", &vpe.ShowVersionReply{Program: "vpe", Version: "26.06-release"})
	f.Reply("cli_inband", &vlib.CliInbandReply{Reply: showPlugins})
	have := map[string]bool{"local0": true, "wan": true, "wan2": true}
	f.On("sw_interface_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*interfaces.SwInterfaceDump)
		var out []api.Message
		for n := range have { // substring filter, like VPP
			if !r.NameFilterValid || strings.Contains(n, r.NameFilter) {
				out = append(out, &interfaces.SwInterfaceDetails{InterfaceName: n})
			}
		}
		return out, nil
	})
	return f
}

func withFake(f *fake.Client) dialer {
	return func(context.Context, string) (vpp.Client, func(), error) { return f, func() {}, nil }
}

func runWith(t *testing.T, d dialer, args ...string) (int, string, string) {
	t.Helper()
	var o, e bytes.Buffer
	rc := run(args, &o, &e, d, noBackstop)
	return rc, o.String(), e.String()
}

func TestVersion(t *testing.T) {
	rc, out, _ := runWith(t, withFake(fakeVPP()), "version")
	if rc != exitOK || out != "vpp 26.06-release\n" {
		t.Fatalf("rc=%d out=%q", rc, out)
	}
}

func TestPluginsByContent(t *testing.T) {
	f := fakeVPP()
	rc, out, _ := runWith(t, withFake(f), "plugins")
	if rc != exitOK || out != "abf_plugin.so\ndpdk_plugin.so\nlinux_cp_plugin.so\n" {
		t.Fatalf("rc=%d out=%q", rc, out)
	}
	if c := f.CallsNamed("cli_inband"); len(c) != 1 || c[0].(*vlib.CliInband).Cmd != "show plugins" {
		t.Fatalf("calls %v", c)
	}
	// a reply without plugin rows is an error, never "nothing loaded"
	f.Reply("cli_inband", &vlib.CliInbandReply{Reply: "unknown input `plugins'\n"})
	if rc, _, _ := runWith(t, withFake(f), "plugins"); rc != exitVPP {
		t.Fatalf("empty table: rc=%d, want %d", rc, exitVPP)
	}
}

func TestIfacesExactMatch(t *testing.T) {
	f := fakeVPP()
	rc, out, _ := runWith(t, withFake(f), "ifaces", "local0", "wan")
	if rc != exitOK || !strings.Contains(out, "present: local0 wan") {
		t.Fatalf("rc=%d out=%q", rc, out)
	}
	// "lan" is not present; "wa" is a substring of wan/wan2 but not an interface
	rc, out, _ = runWith(t, withFake(f), "ifaces", "lan", "wan", "wa")
	if rc != exitMissing || out != "missing: lan wa\n" {
		t.Fatalf("rc=%d out=%q", rc, out)
	}
	for _, c := range f.CallsNamed("sw_interface_dump") {
		if r := c.(*interfaces.SwInterfaceDump); !r.NameFilterValid || r.NameFilter == "" {
			t.Fatalf("dump without a name filter: %+v", r)
		}
	}
}

func TestDisconnectedIsExit2(t *testing.T) {
	f := fakeVPP()
	f.SetConnected(false)
	if rc, _, e := runWith(t, withFake(f), "ifaces", "local0"); rc != exitVPP || !strings.Contains(e, "sw_interface_dump") {
		t.Fatalf("rc=%d err=%q", rc, e)
	}
}

func TestUsage(t *testing.T) {
	for _, a := range [][]string{nil, {"ifaces"}, {"version", "x"}, {"bogus"}, {"--timeout", "0s", "version"}} {
		if rc, _, _ := runWith(t, withFake(fakeVPP()), a...); rc != exitUsage {
			t.Errorf("%v: rc=%d, want %d", a, rc, exitUsage)
		}
	}
}

func TestNoSocket(t *testing.T) {
	rc, _, e := runWith(t, dialVPP, "--socket", filepath.Join(t.TempDir(), "absent.sock"), "version")
	if rc != exitVPP || !strings.Contains(e, "no VPP API socket") {
		t.Fatalf("rc=%d err=%q", rc, e)
	}
}

// TestHungVPPTimesOut is re-review N1: a VPP that accepts the API socket but never answers
// (stuck main loop) must end the check within the deadline, not block the apply script.
func TestHungVPPTimesOut(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "api.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	var held []net.Conn
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			held = append(held, c) // accept, read nothing, answer nothing
		}
	}()
	start := time.Now()
	rc, _, e := runWith(t, dialVPP, "--socket", sock, "--timeout", "700ms", "ifaces", "local0")
	took := time.Since(start)
	if rc != exitVPP {
		t.Fatalf("rc=%d, want %d (stderr %q)", rc, exitVPP, e)
	}
	if took > 3*time.Second {
		t.Fatalf("hung VPP blocked the check for %s (deadline 700ms)", took)
	}
	t.Logf("hung VPP: exit %d after %s: %s", rc, took.Round(time.Millisecond), strings.TrimSpace(e))
}
