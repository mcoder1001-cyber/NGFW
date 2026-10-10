package subsystems

import (
	"context"
	"fmt"
	"log/slog"
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
	defer conn.Close()
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
	t.Logf("actual native raw=%d transit=%d token=%s", rawIdx, transitIdx, spec.Token())
	rt := &PppoeRuntime{renderer: pppoe.New(), runner: runner, vpp: conn, owner: "w20", globalsOwner: true, carrierMode: true, allowRoute: func(uint32) bool { return true }, log: slog.Default(), stateDir: "/run/ngfw/pppoe", applied: map[string]pppoe.Session{}}
	s := pppoe.Session{Carrier: &spec, Iface: spec.Logical, HostIf: spec.RawHost(), Username: "w20", Password: "NGFW_TEST_PSK_w20", MTU: 1492, DefaultRoute: true, IPv6: "off", HoldoffSec: 1, MaxFail: 0}
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
		t.Fatalf("native carrier readiness failed: %v; unit=%s", last, run("systemctl", "show", carrierUnit(s), "-p", "ActiveState", "-p", "ExecMainStatus"))
	}
	t.Log("REAL_CURRENT_RUNTIME_PAP_IPCP_MIRROR_FORWARDING_READY PASS")
	run("ip", "-n", "ns-w20-carrier-isp", "route", "replace", "10.20.1.0/24", "dev", "ppp0")
	packet := run("ip", "netns", "exec", "ns-w20-carrier-lan", "ping", "-n", "-c", "3", "-W", "2", "100.64.20.1")
	if !strings.Contains(packet, "3 received") {
		t.Fatal(packet)
	}
	t.Log("REAL_LAN_VPP_TRANSIT_KERNEL_PPP_IPV4_PACKETS PASS")
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
}
