package ipfixsflow

// TestExporterZeroToSlotCollector — opt-in exporter evidence on the shared host (D-082): points VPP's IPFIX exporter 0
// at this slot's Go UDP collector and checks that IPFIX template sets (and, with VRX_IPFIX_TRAFFIC=1, data sets)
// arrive. Exporter 0, flowprobe.params and sflow.global are VPP globals: the test holds flock -x on
// /run/lock/vrx-globals.lock, saves exactly the previous values and restores exactly those (never VPP's defaults).
//
// Needs: VRX_INTEGRATION=1, VRX_IPFIX_GLOBALS=1 (a manager window), root, VRX_TEST_PREFIX=w<N>, the slot rig up with
// its lan interface configured (tools/lab rig up + P08 interfaces: host-w<N>l0 = 10.<N>.1.1/24 in VPP, the collector
// side 10.<N>.1.2 in ns-w<N>-lan). Overrides: VRX_IPFIX_IFACE, VRX_IPFIX_SRC, VRX_IPFIX_COLLECTOR, VRX_IPFIX_NETNS.
// Traffic (data sets) only with VRX_IPFIX_TRAFFIC=1 and after TD-3's V19 preflight on the rig interface (D-095).
// A flowprobe interface of ANY owner blocks flowprobe_set_params (DF-8): then the test skips, it does not fail.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	govpp "go.fd.io/govpp"
	vppapi "go.fd.io/govpp/api"
	"golang.org/x/sys/unix"

	"ngfw/agent/binapi/flowprobe"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/ipfix_export"
)

const (
	apiSocket   = "/run/vpp/api.sock"
	globalsLock = "/run/lock/vrx-globals.lock"
)

type saved struct {
	exporter ipfix_export.IpfixExporterDetails
	params   flowprobe.FlowprobeGetParamsReply
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func nrestarts(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("systemctl", "show", "vpp", "-p", "NRestarts").Output()
	if err != nil {
		t.Fatalf("systemctl show vpp: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// listenIn starts the collector inside netns (the socket stays in the namespace it was created in).
func listenIn(netns, addr string) (*Collector, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	self, err := os.Open("/proc/thread-self/ns/net")
	if err != nil {
		return nil, err
	}
	defer func() { _ = self.Close() }()
	target, err := os.Open("/run/netns/" + netns)
	if err != nil {
		return nil, err
	}
	defer func() { _ = target.Close() }()
	if err := unix.Setns(int(target.Fd()), unix.CLONE_NEWNET); err != nil {
		return nil, fmt.Errorf("setns %s: %w", netns, err)
	}
	c, lerr := Listen(addr)
	if err := unix.Setns(int(self.Fd()), unix.CLONE_NEWNET); err != nil {
		panic(fmt.Sprintf("cannot return to the test's netns: %v", err)) // the thread must not be reused
	}
	return c, lerr
}

func TestExporterZeroToSlotCollector(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" || os.Getenv("VRX_IPFIX_GLOBALS") != "1" {
		t.Skip("exporter-0 evidence changes VPP globals (D-082): set VRX_INTEGRATION=1 and VRX_IPFIX_GLOBALS=1 in a manager window")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (VPP API socket, netns)")
	}
	m := regexp.MustCompile(`^w([1-9]|1[01])$`).FindStringSubmatch(os.Getenv("VRX_TEST_PREFIX"))
	if m == nil {
		t.Fatal("VRX_TEST_PREFIX must be the slot prefix w1..w11")
	}
	n := m[1]
	iface := env("VRX_IPFIX_IFACE", "host-w"+n+"l0")
	src := netip.MustParseAddr(env("VRX_IPFIX_SRC", "10."+n+".1.1"))
	coll := netip.MustParseAddr(env("VRX_IPFIX_COLLECTOR", "10."+n+".1.2"))
	netns := env("VRX_IPFIX_NETNS", "ns-w"+n+"-lan")
	port, _ := strconv.Atoi("3" + n + "71")

	lock, err := os.OpenFile(globalsLock, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("flock -x %s: %v", globalsLock, err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	before := nrestarts(t)
	t.Logf("before: %s", before)
	defer func() {
		if after := nrestarts(t); after != before {
			t.Errorf("VPP restarted during the test: %s → %s", before, after)
		} else {
			t.Logf("after: %s", after)
		}
	}()

	conn, err := govpp.Connect(apiSocket)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Disconnect()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sw := swIfIndex(ctx, t, conn, iface)
	if others := flowprobeInterfaces(ctx, t, conn); len(others) > 0 {
		t.Skipf("flowprobe is enabled on sw_if_index %v by some owner: VPP refuses flowprobe_set_params (DF-8) — documented skip", others)
	}
	var s saved
	s.exporter = exporter0(ctx, t, conn)
	p, err := flowprobe.NewServiceClient(conn).FlowprobeGetParams(ctx, &flowprobe.FlowprobeGetParams{})
	if err != nil {
		t.Fatal(err)
	}
	s.params = *p
	t.Logf("saved exporter 0: %s:%d src %s vrf %d mtu %d tmpl %d; flowprobe flags %d timers %d/%d",
		s.exporter.CollectorAddress, s.exporter.CollectorPort, s.exporter.SrcAddress, s.exporter.VrfID, s.exporter.PathMtu,
		s.exporter.TemplateInterval, s.params.RecordFlags, s.params.ActiveTimer, s.params.PassiveTimer)
	defer restore(t, conn, s)

	c, err := listenIn(netns, fmt.Sprintf("%s:%d", coll, port))
	if err != nil {
		t.Fatalf("collector in %s: %v", netns, err)
	}
	defer func() { _ = c.Close() }()

	ipfix := ipfix_export.NewServiceClient(conn)
	if _, err := ipfix.SetIpfixExporter(ctx, &ipfix_export.SetIpfixExporter{
		CollectorAddress: ip_types.NewAddress(coll.AsSlice()), CollectorPort: uint16(port), SrcAddress: ip_types.NewAddress(src.AsSlice()),
		VrfID: 0, PathMtu: 1400, TemplateInterval: 1,
	}); err != nil {
		t.Fatal(err)
	}
	fp := flowprobe.NewServiceClient(conn)
	if _, err := fp.FlowprobeSetParams(ctx, &flowprobe.FlowprobeSetParams{
		RecordFlags: flowprobe.FLOWPROBE_RECORD_FLAG_L3 | flowprobe.FLOWPROBE_RECORD_FLAG_L4, ActiveTimer: 1, PassiveTimer: 2,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fp.FlowprobeInterfaceAddDel(ctx, &flowprobe.FlowprobeInterfaceAddDel{IsAdd: true, Which: flowprobe.FLOWPROBE_WHICH_IP4,
		Direction: flowprobe.FLOWPROBE_DIRECTION_RX, SwIfIndex: interface_types.InterfaceIndex(sw)}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := fp.FlowprobeInterfaceAddDel(context.Background(), &flowprobe.FlowprobeInterfaceAddDel{IsAdd: false, Which: flowprobe.FLOWPROBE_WHICH_IP4,
			Direction: flowprobe.FLOWPROBE_DIRECTION_RX, SwIfIndex: interface_types.InterfaceIndex(sw)}); err != nil {
			t.Errorf("flowprobe disable on %s: %v", iface, err)
		}
	}()
	vppctl(t, "show flowprobe params")
	vppctl(t, "show flowprobe interface")

	st, ok := c.WaitFor(30*time.Second, func(s Stats) bool { return s.TemplateSets > 0 })
	t.Logf("collector %s:%d in %s: %+v", coll, port, netns, st)
	if !ok {
		t.Fatal("no IPFIX template set from exporter 0 within 30 s")
	}
	if os.Getenv("VRX_IPFIX_TRAFFIC") == "1" {
		out, err := exec.Command("ip", "netns", "exec", netns, "ping", "-c", "5", "-i", "0.2", src.String()).CombinedOutput()
		t.Logf("ping %s from %s: %v\n%s", src, netns, err, out)
		st, ok = c.WaitFor(30*time.Second, func(s Stats) bool { return s.DataSets > 0 })
		t.Logf("collector after traffic: %+v", st)
		if !ok {
			t.Fatal("no IPFIX data set within 30 s after traffic")
		}
	}
}

func swIfIndex(ctx context.Context, t *testing.T, conn vppapi.Connection, name string) uint32 {
	t.Helper()
	stream, err := interfaces.NewServiceClient(conn).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
	if err != nil {
		t.Fatal(err)
	}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimRight(d.InterfaceName, "\x00") == name {
			return uint32(d.SwIfIndex)
		}
	}
	t.Skipf("%s does not exist: bring the slot rig up and configure its interfaces first", name)
	return 0
}

func flowprobeInterfaces(ctx context.Context, t *testing.T, conn vppapi.Connection) []uint32 {
	t.Helper()
	stream, err := flowprobe.NewServiceClient(conn).FlowprobeInterfaceDump(ctx, &flowprobe.FlowprobeInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
	if err != nil {
		t.Fatal(err)
	}
	var out []uint32
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, uint32(d.SwIfIndex))
	}
}

func exporter0(ctx context.Context, t *testing.T, conn vppapi.Connection) ipfix_export.IpfixExporterDetails {
	t.Helper()
	stream, err := ipfix_export.NewServiceClient(conn).IpfixExporterDump(ctx, &ipfix_export.IpfixExporterDump{})
	if err != nil {
		t.Fatal(err)
	}
	var first *ipfix_export.IpfixExporterDetails
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = d
		}
	}
	if first == nil {
		t.Fatal("ipfix_exporter_dump returned nothing")
	}
	return *first
}

// restore puts back exactly the saved exporter 0 and flowprobe parameters (never VPP's defaults).
func restore(t *testing.T, conn vppapi.Connection, s saved) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e := s.exporter
	if _, err := ipfix_export.NewServiceClient(conn).SetIpfixExporter(ctx, &ipfix_export.SetIpfixExporter{
		CollectorAddress: e.CollectorAddress, CollectorPort: e.CollectorPort, SrcAddress: e.SrcAddress, VrfID: e.VrfID,
		PathMtu: e.PathMtu, TemplateInterval: e.TemplateInterval, UDPChecksum: e.UDPChecksum,
	}); err != nil {
		t.Errorf("restore exporter 0: %v", err)
	}
	if _, err := flowprobe.NewServiceClient(conn).FlowprobeSetParams(ctx, &flowprobe.FlowprobeSetParams{
		RecordFlags: s.params.RecordFlags, ActiveTimer: s.params.ActiveTimer, PassiveTimer: s.params.PassiveTimer,
	}); err != nil {
		t.Errorf("restore flowprobe params: %v", err)
	}
	after := exporter0(ctx, t, conn)
	if after.CollectorAddress != e.CollectorAddress || after.CollectorPort != e.CollectorPort || after.SrcAddress != e.SrcAddress {
		t.Errorf("exporter 0 not restored: %+v, saved %+v", after, e)
	}
	vppctl(t, "show flowprobe params")
}

func vppctl(t *testing.T, cmd string) {
	t.Helper()
	out, err := exec.Command("vppctl", strings.Fields(cmd)...).CombinedOutput()
	t.Logf("vppctl %s (%v):\n%s", cmd, err, out)
}
