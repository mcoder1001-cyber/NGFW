package lcp

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"ngfw/agent/binapi/lcp"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/vpp"
)

// TestMfibGuardOnHost (S-ospf-mfib-stale): a default-namespace pair whose host tap has an IPv4
// address gets linux-cp's (*,224.0.0.0/24) Accept on the phy; Delete must leave none behind.
func TestMfibGuardOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.SkipUnlessCompatible(t, Plugin, &lcp.LcpItfPairAddDelV3{}, &lcp.LcpItfPairGet{})
	c := h.Client()
	ctx := context.Background()
	if ns, err := NewDefaultNetns(c).Current(ctx); err != nil || ns != "" {
		t.Skipf("default netns %q (%v): the guard covers root-namespace pairs only", ns, err)
	}
	pd := NewItfPair(c, h.Owner)
	ifName, idx := h.Loopback(t, 87)
	host := h.Owner + "-lcp7"
	v := ItfPair{Interface: ifName, HostIfName: host, HostIfType: "tap"}.Proto()
	t.Cleanup(func() { _ = pd.Delete(context.Background(), v, nil) })
	if _, err := pd.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	ifc, err := net.InterfaceByName(host)
	if err != nil {
		t.Fatal(err)
	}
	if err := setUp(ifc.Index); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		t.Fatal(err)
	}
	if err := addrReq(fd, unix.RTM_NEWADDR, unix.NLM_F_CREATE|unix.NLM_F_EXCL, 1, ifc.Index, net.IPv4(10, 211, 7, 1).To4(), 24); err != nil {
		t.Fatal(err)
	}
	waitAccept(t, c, idx, true)
	t.Logf("before delete: %s", mfibShow(t))
	if err := pd.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	waitAccept(t, c, idx, false)
	t.Logf("after delete: %s", mfibShow(t))
}

func setUp(ifindex int) error {
	ifc, _ := net.InterfaceByIndex(ifindex)
	return exec.Command("ip", "link", "set", ifc.Name, "up").Run() // test-only, fixed arguments
}

func waitAccept(t *testing.T, c vpp.Client, idx uint32, want bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		got, err := StaleAccept(context.Background(), c, idx)
		if err != nil {
			t.Fatal(err)
		}
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("(*,224.0.0.0/24) Accept on sw_if_index %d = %v, want %v\n%s", idx, got, want, mfibShow(t))
		}
	}
}

func mfibShow(t *testing.T) string { return mfibShowIn(0) }

// mfibShowIn is `vppctl show ip mfib table <table> 224.0.0.0/24` (read-only, bounded by timeout).
func mfibShowIn(table uint32) string {
	out, _ := exec.Command("timeout", "5", "vppctl", "show", "ip", "mfib", "table", strconv.FormatUint(uint64(table), 10), "224.0.0.0/24").CombinedOutput() //nolint:gosec // fixed arguments, numeric table id
	return string(out)
}

// TestNetnsPairRefusedInTable0OnHost (D-217): table 0 holds other slots' default-namespace pairs,
// so a pair outside the lcp default namespace on a table-0 loopback is refused before any add and
// no API (*,224.0.0.0/24) source appears.
func TestNetnsPairRefusedInTable0OnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.SkipUnlessCompatible(t, Plugin, &lcp.LcpItfPairAddDelV3{}, &lcp.LcpItfPairGet{})
	c := h.Client()
	ctx := context.Background()
	def, err := NewDefaultNetns(c).Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pairs, err := Pairs(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	var defPairs []string
	for _, p := range pairs {
		if ns := strings.TrimRight(p.Netns, "\x00"); ns == "" || ns == def {
			if tb, err := phyTable(ctx, c, uint32(p.PhySwIfIndex)); err == nil && tb == 0 {
				defPairs = append(defPairs, strings.TrimRight(p.HostIfName, "\x00"))
			}
		}
	}
	pd := NewItfPair(c, h.Owner)
	if len(defPairs) == 0 { // none of another slot's right now: make our own default-namespace pair
		if def != "" {
			t.Skipf("default netns %q set by someone else", def)
		}
		dn, _ := h.Loopback(t, 89)
		dv := ItfPair{Interface: dn, HostIfName: h.Owner + "-lcp9", HostIfType: "tap"}.Proto()
		t.Cleanup(func() { _ = pd.Delete(context.Background(), dv, nil) })
		if _, err := pd.Create(ctx, dv); err != nil {
			t.Fatal(err)
		}
		defPairs = append(defPairs, h.Owner+"-lcp9 (own)")
	}
	t.Logf("default-namespace pairs in table 0: %v", defPairs)
	t.Logf("before: %s", mfibShow(t))
	ifName, idx := h.Loopback(t, 88)
	v := ItfPair{Interface: ifName, HostIfName: h.Owner + "-lcp8", HostIfType: "tap", Netns: "ns-" + h.Owner + "-d217"}.Proto()
	t.Cleanup(func() { _ = pd.Delete(context.Background(), v, nil) })
	_, err = pd.Create(ctx, v)
	if err == nil || !strings.Contains(err.Error(), "D-217") {
		t.Fatalf("create: %v, want D-217 refusal", err)
	}
	t.Logf("refused: %v", err)
	after, err := Pairs(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range after {
		if uint32(p.PhySwIfIndex) == idx {
			t.Fatal("pair created despite the refusal")
		}
	}
	out := mfibShow(t)
	t.Logf("after: %s", out)
	if strings.Contains(out, "src:API") {
		t.Fatal("an API (*,224.0.0.0/24) source appeared in table 0")
	}
}
