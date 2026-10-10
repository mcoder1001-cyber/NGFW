package subsystems

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"ngfw/agent/binapi/interface_types"
	mssapi "ngfw/agent/binapi/mss_clamp"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/vpp"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Executes unchanged native runtime, original broker/unit, real TAPs and pppd.
// This fixture owns only w20, an isolated VPP and one bounded carrier namespace.
func TestWANCurrentCarrierLive(t *testing.T) {
	if os.Getenv("NGFW_DISPOSABLE_VPP") != "1" || os.Getenv("NGFW_WAN_NATIVE_CARRIER") != "1" {
		t.Fatal("private carrier opt-in required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Second)
	defer cancel()
	run := func(args ...string) string {
		t.Helper()
		c := exec.CommandContext(ctx, args[0], args[1:]...)
		b, e := c.CombinedOutput()
		if e != nil {
			t.Fatalf("%v: %v: %s", args, e, b)
		}
		return string(b)
	}
	conn := vpp.Dial("/run/vpp/api.sock", vpp.ConnOptions{})
	t.Cleanup(func() { conn.Close() })
	if e := conn.WaitConnected(ctx); e != nil {
		t.Fatal(e)
	}
	spec, e := pppoe.NewCarrierSpec("w20", "w20ppp", "host-w20raw", 1492)
	if e != nil {
		t.Fatal(e)
	}
	runner := renderers.NewSystemRunner(renderers.NewAllowlist(pppoe.Binaries()...))
	host := &pppoeCarrierHost{runner: runner}
	lease, e := host.Provision(ctx, spec)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := host.Remove(context.Background(), lease); e != nil {
			t.Errorf("carrier cleanup: %v", e)
		}
	})
	tap := tapv2.New(conn, "w20")
	tap.SetNamespaceAdmission(func(c context.Context, p *tapv2.Tap) error {
		ls, e := host.Inventory(c, "w20")
		if e != nil {
			return e
		}
		for _, l := range ls {
			if l.Token == p.HostNamespace && l.Generation == lease.Generation {
				return nil
			}
		}
		return fmt.Errorf("owned lease missing")
	})
	rawID, transitID := spec.TapIDs()
	taps := []*tapv2.Tap{{Name: spec.RawLogical(), Id: rawID, HostIfName: spec.RawHost(), HostNamespace: spec.Token(), HostMtu: 1500, RxRingSize: 256, TxRingSize: 256}, {Name: spec.Logical, Id: transitID, HostIfName: spec.TransitHost(), HostNamespace: spec.Token(), HostMtu: 1492, HostIp4Prefix: spec.Host4, HostIp6Prefix: spec.Host6, RxRingSize: 256, TxRingSize: 256}}
	for _, p := range taps {
		meta, e := tap.Create(ctx, p)
		if e != nil {
			t.Fatalf("native TAP create: %v; direct CLI diagnostic: %s; netns=%s", e, run("vppctl", "create", "tap", "id", fmt.Sprint(p.Id), "host-ns", p.HostNamespace, "host-if-name", p.HostIfName), run("ip", "-n", p.HostNamespace, "-j", "link", "show"))
		}
		t.Cleanup(func() {
			if e := tap.Delete(context.Background(), p, meta); e != nil {
				t.Errorf("tap cleanup: %v", e)
			}
		})
	}
	ifs, e := df6.DumpInterfaces(ctx, conn, "w20")
	if e != nil {
		t.Fatal(e)
	}
	rawIdx, ok := ifs.IndexByTag(spec.RawLogical())
	if !ok {
		t.Fatal("raw TAP missing")
	}
	transitIdx, ok := ifs.IndexByTag(spec.Logical)
	if !ok {
		t.Fatal("transit missing")
	}
	// CLI setup is only private topology; runtime later verifies actual crossconnect/address/link state.
	rawName := fmt.Sprintf("tap%d", rawID)
	transitName := fmt.Sprintf("tap%d", transitID)
	for _, pair := range [][2]string{{rawName, "host-w20raw"}, {"host-w20raw", rawName}} {
		run("vppctl", "set", "interface", "l2", "xconnect", pair[0], pair[1])
	}
	for _, name := range []string{rawName, transitName, "host-w20raw", "host-w20lan"} {
		run("vppctl", "set", "interface", "state", name, "up")
	}
	run("vppctl", "set", "interface", "mtu", "1492", transitName)
	run("vppctl", "set", "interface", "ip", "address", transitName, spec.VPP4())
	run("vppctl", "set", "interface", "ip", "address", transitName, spec.VPP6())
	t.Logf("actual native raw=%d transit=%d namespace=%s", rawIdx, transitIdx, spec.Token())
	rt := &PppoeRuntime{renderer: pppoe.New(), runner: runner, vpp: conn, owner: "w20", globalsOwner: true, carrierMode: true, allowRoute: func(uint32) bool { return true }, log: slog.Default(), stateDir: "/run/ngfw/pppoe", applied: map[string]pppoe.Session{}}
	password := os.Getenv("NGFW_WAN_PEER_PASSWORD")
	if password == "" {
		t.Fatal("ephemeral peer credential missing")
	}
	extended := os.Getenv("NGFW_WAN_EXTENDED") == "1"
	s := pppoe.Session{Carrier: &spec, Iface: spec.Logical, HostIf: spec.RawHost(), Username: "w20", Password: password, MTU: 1492, DefaultRoute: true, IPv6: "off", HoldoffSec: 1, MaxFail: 0}
	if extended {
		s.IPv6 = "slaac"
		s.MSSClamp = true
	}
	t.Cleanup(func() {
		if e := rt.Apply(context.Background(), nil); e != nil {
			t.Errorf("runtime cleanup: %v", e)
		}
	})
	if e := rt.Apply(ctx, []pppoe.Session{s}); e != nil {
		t.Fatal(e)
	}
	var last error
	wait := func(predicate func() bool) bool {
		end := time.Now().Add(40 * time.Second)
		for time.Now().Before(end) {
			if predicate() {
				return true
			}
			time.Sleep(300 * time.Millisecond)
		}
		return false
	}
	if !wait(func() bool {
		last = rt.poll(ctx)
		rt.mu.Lock()
		_, ready := rt.carrierReady[s.Iface]
		rt.mu.Unlock()
		return last == nil && ready
	}) {
		diagnostic := exec.CommandContext(ctx, "/usr/bin/python3", "-I", "-c", "import runpy,sys; m=runpy.run_path('/usr/lib/ngfw/pppoe-carrier.py'); c=m['Carrier']();\nwith c.locked():\n c.configure(sys.argv[1],sys.argv[2],True)", spec.Token(), lease.Generation)
		output, diagnosticError := diagnostic.CombinedOutput()
		t.Logf("same original configure diagnostic: %v: %s", diagnosticError, output)
		t.Fatalf("native carrier readiness failed: %v; unit=%s", last, run("systemctl", "show", carrierUnit(s), "-p", "ActiveState", "-p", "ExecMainStatus"))
	}
	t.Log("REAL_CURRENT_RUNTIME_PAP_IPCP_MIRROR_FORWARDING_READY PASS")
	run("ip", "-n", "ns-w20-carrier-isp", "route", "replace", "10.20.1.0/24", "dev", "ppp0")
	localTransit := netip.MustParsePrefix(spec.Host4).Addr().String()
	routeOutput, routeError := exec.CommandContext(ctx, "ip", "-n", spec.Token(), "route", "get", localTransit, "from", spec.Peer4, "iif", spec.TransitHost()).CombinedOutput()
	t.Logf("OWNED_TRANSIT_LOCAL_ROUTE %v: %s", routeError, routeOutput)
	captureCtx, captureCancel := context.WithTimeout(ctx, 8*time.Second)
	capture := exec.CommandContext(captureCtx, "ip", "netns", "exec", spec.Token(), "tcpdump", "-nn", "-l", "-i", "any", "icmp and host 100.64.20.1")
	var captureOutput bytes.Buffer
	capture.Stdout = &captureOutput
	capture.Stderr = &captureOutput
	captureStarted := capture.Start() == nil
	ping := exec.CommandContext(ctx, "ip", "netns", "exec", "ns-w20-carrier-lan", "ping", "-n", "-c", "3", "-W", "2", "100.64.20.1")
	packetBytes, packetError := ping.CombinedOutput()
	packet := string(packetBytes)
	captureCancel()
	if captureStarted {
		capture.Wait()
	}
	t.Logf("BOUNDED_FILTERED_ICMP_CAPTURE %s", captureOutput.String())
	if packetError != nil {
		for _, args := range [][]string{{"vppctl", "show", "interface"}, {"vppctl", "show", "ip", "fib"}, {"vppctl", "show", "ip", "neighbors"}, {"vppctl", "show", "errors"}, {"ip", "-n", spec.Token(), "-j", "address", "show"}, {"ip", "-n", spec.Token(), "-4", "rule", "show"}, {"ip", "netns", "exec", spec.Token(), "nft", "list", "table", "inet", "ngfw_ppp"}} {
			output, error := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
			t.Logf("packet diagnostic %v: %v: %s", args, error, output)
		}
	}
	if !strings.Contains(packet, "3 received") {
		t.Fatal(packet)
	}
	t.Log("REAL_LAN_VPP_TRANSIT_KERNEL_PPP_IPV4_PACKETS PASS")
	if extended {
		if !wait(func() bool {
			last = rt.poll(ctx)
			rt.mu.Lock()
			ready := rt.carrierReady[s.Iface]
			rt.mu.Unlock()
			return last == nil && len(ready.mirror.LocalIPv6) > 0
		}) {
			t.Fatalf("actual IPv6 RA/mirror not ready: %v; peer=%s; carrier=%s", last, run("ip", "-n", "ns-w20-carrier-isp", "-6", "addr", "show"), run("ip", "-n", spec.Token(), "-6", "addr", "show"))
		}
		run("ip", "-n", "ns-w20-carrier-isp", "-6", "route", "replace", "2001:db8:21::/64", "dev", "ppp0")
		packet6 := run("ip", "netns", "exec", "ns-w20-carrier-lan", "ping", "-6", "-n", "-c", "3", "-W", "2", "2001:db8:20::1")
		if !strings.Contains(packet6, "3 received") {
			t.Fatal(packet6)
		}
		t.Log("REAL_CURRENT_RUNTIME_IPV6_RA_MIRROR_LAN_PACKETS PASS")
		stream, err := mssapi.NewServiceClient(conn).MssClampGet(ctx, &mssapi.MssClampGet{SwIfIndex: interface_types.InterfaceIndex(transitIdx)})
		if err != nil {
			t.Fatal(err)
		}
		row, _, err := stream.Recv()
		if err != nil || row == nil || row.IPv4Mss != 1452 || row.IPv6Mss != 1432 || row.IPv4Direction != 3 || row.IPv6Direction != 3 {
			t.Fatalf("actual MSS clamp readback: %+v %v", row, err)
		}
		t.Logf("REAL_MSS_CLAMP_READBACK IPv4=%d IPv6=%d RX_TX=3 PASS", row.IPv4Mss, row.IPv6Mss)
		tcpCtx, tcpCancel := context.WithTimeout(ctx, 5*time.Second)
		tcp := exec.CommandContext(tcpCtx, "ip", "netns", "exec", "ns-w20-carrier-isp", "tcpdump", "-nn", "-l", "-v", "-i", "ppp0", "-c", "1", "tcp and dst port 20020 and tcp[tcpflags] & tcp-syn != 0")
		var tcpOutput bytes.Buffer
		tcp.Stdout = &tcpOutput
		tcp.Stderr = &tcpOutput
		if err := tcp.Start(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(300 * time.Millisecond)
		exec.CommandContext(ctx, "ip", "netns", "exec", "ns-w20-carrier-lan", "python3", "-c", "import socket; s=socket.socket();s.settimeout(2);s.connect(('100.64.20.1',20020))").Run()
		tcp.Wait()
		tcpCancel()
		if !strings.Contains(tcpOutput.String(), "mss 1452") {
			t.Fatalf("real IPv4 TCP MSS packet: %s", tcpOutput.String())
		}
		t.Logf("REAL_TCP_SYN_MSS_1452_PACKET PASS %s", tcpOutput.String())

	}

	accepted, _, e := rt.Reconnect(ctx, s.Iface)
	if e != nil || !accepted {
		t.Fatalf("reconnect %t %v", accepted, e)
	}
	if !wait(func() bool {
		last = rt.poll(ctx)
		rt.mu.Lock()
		_, ready := rt.carrierReady[s.Iface]
		rt.mu.Unlock()
		return last == nil && ready
	}) {
		t.Fatalf("reconnect readiness: %v", last)
	}
	t.Log("REAL_CURRENT_RUNTIME_RECONNECT PASS")
	if extended {
		run("flock", "-x", "/run/lock/ngfw-globals.lock", "vppctl", "nat44", "plugin", "enable", "sessions", "4096")
		run("vppctl", "nat44", "add", "address", "100.64.20.10")
		run("vppctl", "set", "interface", "nat44", "in", "host-w20lan", "out", transitName)
		natPacket := run("ip", "netns", "exec", "ns-w20-carrier-lan", "ping", "-n", "-c", "3", "-W", "2", "100.64.20.1")
		if !strings.Contains(natPacket, "3 received") {
			t.Fatal(natPacket)
		}
		sessions := run("vppctl", "show", "nat44", "sessions")
		if !strings.Contains(sessions, "10.20.1.2") || !strings.Contains(sessions, "100.64.20.10") {
			t.Fatalf("real NAT session evidence: %s", sessions)
		}
		t.Logf("REAL_LAN_VPP_NATIVE_NAT_PPP_PACKETS PASS %s", sessions)
	}

	if os.Getenv("NGFW_WAN_WRONG_PASSWORD") == "1" {
		bad := s
		bad.Password = password + "incorrect"
		bad.MaxFail = 1
		if err := rt.Apply(ctx, []pppoe.Session{bad}); err != nil {
			t.Fatal(err)
		}
		if !wait(func() bool {
			rt.poll(ctx)
			st, e := rt.State(s.Iface, 0, "")
			return e == nil && st.GetFailCount() > 0 && strings.Contains(st.GetLastError(), "authentication")
		}) {
			st, e := rt.State(s.Iface, 0, "")
			t.Fatalf("real wrong-password clear error absent: %v %v", st, e)
		}
		if _, _, _, ready := rt.ForwardingGateway(s.Iface); ready {
			t.Fatal("wrong password retained forwarding readiness")
		}
		t.Log("REAL_CURRENT_RUNTIME_WRONG_PASSWORD_AUTHENTICATION_ERROR_WITHDRAWAL PASS")
		if err := rt.Apply(ctx, []pppoe.Session{s}); err != nil {
			t.Fatal(err)
		}
		if !wait(func() bool {
			last = rt.poll(ctx)
			_, _, _, ready := rt.ForwardingGateway(s.Iface)
			return last == nil && ready
		}) {
			t.Fatalf("restore after wrong password: %v", last)
		}
		t.Log("REAL_CURRENT_RUNTIME_CREDENTIAL_RESTORE PASS")
	}

}
