package nat44edsessions

// NAT44-ED access of the test itself (never of the product): the plugin fixture (D-071: a slot never enables the
// plugin through its agent; the test does, restoring the previous state — natcommon/nattest's semantics, re-done here
// because a test module cannot import the agent's internal packages), the slot / globals locks, the simulated loss
// behind the agent's back and the "nothing of ours remains" dump.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	vppapi "go.fd.io/govpp/api"

	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/binapi/nat_types"
)

const (
	fixtureLock = "/run/lock/vrx-nat-fixture-nat44-ed.lock" // nattest.EnsurePlugin's lock for "nat44-ed"
	globalsLock = "/run/lock/vrx-globals.lock"              // D-082: tests that read VPP-wide settings hold it shared
)

func flock(t *testing.T, path string, how int) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o644) //nolint:gosec // fixed lock path
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		_ = f.Close()
		t.Fatalf("flock %s: %v", path, err)
	}
	return f
}

// slotLock is nattest.SlotLock(t, name): exclusive, private to the slot (ED and EI are mutually exclusive on one VPP).
func slotLock(t *testing.T, s slot, name string) {
	t.Helper()
	if err := mkdirShared(s.runDir); err != nil {
		t.Fatal(err)
	}
	f := flock(t, filepath.Join(s.runDir, name+".lock"), syscall.LOCK_EX)
	t.Cleanup(func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() })
}

// sharedFlock holds path shared for the test's duration.
func sharedFlock(t *testing.T, path string) {
	t.Helper()
	f := flock(t, path, syscall.LOCK_SH)
	t.Cleanup(func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() })
}

func ctx10() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

type runningConfig struct {
	enabled             bool
	sessions            uint32
	insideVrf, outsideV uint32
	forwarding          bool
	timeouts            nat_types.NatTimeouts
}

func natRunning(t *testing.T, conn vppapi.Connection) runningConfig {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	rc, err := nat44_ed.NewServiceClient(conn).Nat44ShowRunningConfig(ctx, &nat44_ed.Nat44ShowRunningConfig{})
	if err != nil {
		t.Fatalf("nat44_show_running_config: %v", err)
	}
	return runningConfig{enabled: rc.Sessions != 0, sessions: rc.Sessions, insideVrf: rc.InsideVrf, outsideV: rc.OutsideVrf, forwarding: rc.ForwardingEnabled, timeouts: rc.Timeouts}
}

func eiEnabled(t *testing.T, conn vppapi.Connection) bool {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	rc, err := nat44_ei.NewServiceClient(conn).Nat44EiShowRunningConfig(ctx, &nat44_ei.Nat44EiShowRunningConfig{})
	if err != nil {
		t.Fatalf("nat44_ei_show_running_config: %v", err)
	}
	return rc.Sessions != 0
}

func drain[T any](recv func() (T, error)) ([]T, error) {
	var out []T
	for {
		d, err := recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, d)
	}
}

// natObjects dumps every nat44-ed configuration object of ALL owners (the D-071 emptiness check).
type natObjects struct {
	features []*nat44_ed.Nat44InterfaceDetails
	outputs  []*nat44_ed.Nat44EdOutputInterfaceDetails
	addrs    []*nat44_ed.Nat44AddressDetails
	ifAddrs  []*nat44_ed.Nat44InterfaceAddrDetails
	statics  []*nat44_ed.Nat44StaticMappingDetails
	idents   []*nat44_ed.Nat44IdentityMappingDetails
	lbs      []*nat44_ed.Nat44LbStaticMappingDetails
}

func dumpNat(t *testing.T, conn vppapi.Connection) natObjects {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	svc := nat44_ed.NewServiceClient(conn)
	var o natObjects
	must := func(err error, what string) {
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	s1, err := svc.Nat44InterfaceDump(ctx, &nat44_ed.Nat44InterfaceDump{})
	must(err, "nat44_interface_dump")
	o.features, err = drain(s1.Recv)
	must(err, "nat44_interface_dump")
	s2, err := svc.Nat44AddressDump(ctx, &nat44_ed.Nat44AddressDump{})
	must(err, "nat44_address_dump")
	o.addrs, err = drain(s2.Recv)
	must(err, "nat44_address_dump")
	s3, err := svc.Nat44InterfaceAddrDump(ctx, &nat44_ed.Nat44InterfaceAddrDump{})
	must(err, "nat44_interface_addr_dump")
	o.ifAddrs, err = drain(s3.Recv)
	must(err, "nat44_interface_addr_dump")
	s4, err := svc.Nat44StaticMappingDump(ctx, &nat44_ed.Nat44StaticMappingDump{})
	must(err, "nat44_static_mapping_dump")
	o.statics, err = drain(s4.Recv)
	must(err, "nat44_static_mapping_dump")
	s5, err := svc.Nat44IdentityMappingDump(ctx, &nat44_ed.Nat44IdentityMappingDump{})
	must(err, "nat44_identity_mapping_dump")
	o.idents, err = drain(s5.Recv)
	must(err, "nat44_identity_mapping_dump")
	s6, err := svc.Nat44LbStaticMappingDump(ctx, &nat44_ed.Nat44LbStaticMappingDump{})
	must(err, "nat44_lb_static_mapping_dump")
	o.lbs, err = drain(s6.Recv)
	must(err, "nat44_lb_static_mapping_dump")
	// output-feature interfaces: cursor get (one call returns everything from the cursor on)
	s7, err := svc.Nat44EdOutputInterfaceGet(ctx, &nat44_ed.Nat44EdOutputInterfaceGet{Cursor: 0})
	must(err, "nat44_ed_output_interface_get")
	for {
		d, _, err := s7.Recv()
		if err != nil {
			break // EOF, or INVALID_VALUE on an empty table
		}
		o.outputs = append(o.outputs, d)
	}
	return o
}

func (o natObjects) empty() bool {
	return len(o.features)+len(o.outputs)+len(o.addrs)+len(o.ifAddrs)+len(o.statics)+len(o.idents)+len(o.lbs) == 0
}

// ensurePlugin is nattest.EnsurePlugin for nat44-ed: the fixture lock shared for the test, enable if off (the same
// configuration DF-3's fixture uses), and in Cleanup — only if this test enabled it — exclusive lock, the
// all-owner emptiness check (+ no NAT VRF table in `show nat44 vrf tables`), then disable.
func ensurePlugin(t *testing.T, conn vppapi.Connection) (wasOn bool) {
	t.Helper()
	f := flock(t, fixtureLock, syscall.LOCK_SH)
	ctx, cancel := ctx10()
	defer cancel()
	_, err := nat44_ed.NewServiceClient(conn).Nat44EdPluginEnableDisable(ctx, &nat44_ed.Nat44EdPluginEnableDisable{Enable: true, Sessions: 1024})
	var rv vppapi.VPPApiError
	switch {
	case errors.As(err, &rv) && rv == vppapi.FEATURE_ALREADY_ENABLED:
		wasOn = true
		t.Log("fixture: nat44-ed was already enabled (by another owner): it stays enabled")
	case err != nil:
		_ = f.Close()
		t.Fatalf("fixture: enable nat44-ed: %v", err)
	default:
		t.Log("fixture: nat44-ed enabled for this test (sessions 1024)")
	}
	t.Cleanup(func() {
		defer func() { _ = f.Close() }()
		if wasOn {
			return
		}
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
			t.Errorf("fixture: exclusive lock: %v", err)
			return
		}
		o := dumpNat(t, conn)
		vt := vppctl(t, "show", "nat44", "vrf", "tables")
		if !o.empty() || strings.Contains(vt, "table") {
			t.Logf("fixture: nat44-ed holds objects of another owner now: left enabled (D-071)")
			return
		}
		cctx, ccancel := ctx10()
		defer ccancel()
		if _, err := nat44_ed.NewServiceClient(conn).Nat44EdPluginEnableDisable(cctx, &nat44_ed.Nat44EdPluginEnableDisable{Enable: false}); err != nil {
			t.Errorf("fixture: disable nat44-ed: %v", err)
			return
		}
		t.Log("fixture: nat44-ed disabled again under the exclusive fixture lock (previous state restored)")
	})
	return wasOn
}

func ip4(b [4]uint8) string { return netip.AddrFrom4(b).String() }

// ours reports what of this slot is in nat44-ed: features on the slot's interfaces, pool addresses in 10.N/16,
// mappings tagged "<prefix>:".
func ours(t *testing.T, conn vppapi.Connection, prefix string, slotNet netip.Prefix, ifIdx map[uint32]string) []string {
	t.Helper()
	o := dumpNat(t, conn)
	var out []string
	for _, f := range o.features {
		if n, ok := ifIdx[uint32(f.SwIfIndex)]; ok {
			out = append(out, fmt.Sprintf("interface-feature %s flags=%d", n, f.Flags))
		}
	}
	for _, f := range o.outputs {
		if n, ok := ifIdx[uint32(f.SwIfIndex)]; ok {
			out = append(out, "output-feature "+n)
		}
	}
	for _, a := range o.addrs {
		if slotNet.Contains(netip.AddrFrom4(a.IPAddress)) {
			out = append(out, fmt.Sprintf("pool address %s vrf %d flags %d", ip4(a.IPAddress), a.VrfID, a.Flags))
		}
	}
	for _, a := range o.ifAddrs {
		if n, ok := ifIdx[uint32(a.SwIfIndex)]; ok {
			out = append(out, "interface-address pool "+n)
		}
	}
	for _, m := range o.statics {
		if strings.HasPrefix(strings.TrimRight(m.Tag, "\x00"), prefix+":") {
			out = append(out, fmt.Sprintf("static mapping %s %s:%d -> %s:%d", strings.TrimRight(m.Tag, "\x00"), ip4(m.LocalIPAddress), m.LocalPort, ip4(m.ExternalIPAddress), m.ExternalPort))
		}
	}
	for _, m := range o.idents {
		if strings.HasPrefix(strings.TrimRight(m.Tag, "\x00"), prefix+":") {
			out = append(out, "identity mapping "+strings.TrimRight(m.Tag, "\x00"))
		}
	}
	for _, m := range o.lbs {
		if strings.HasPrefix(strings.TrimRight(m.Tag, "\x00"), prefix+":") {
			out = append(out, "lb mapping "+strings.TrimRight(m.Tag, "\x00"))
		}
	}
	return out
}

// natLoss deletes this slot's NAT objects behind the agent's back, dependents first (D-095 c): mappings, then pool
// addresses, then the interface features — never the interfaces.
func natLoss(t *testing.T, conn vppapi.Connection, prefix string, slotNet netip.Prefix, ifIdx map[uint32]string) []string {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	svc := nat44_ed.NewServiceClient(conn)
	o := dumpNat(t, conn)
	var ev []string
	for _, m := range o.statics {
		tag := strings.TrimRight(m.Tag, "\x00")
		if !strings.HasPrefix(tag, prefix+":") {
			continue
		}
		req := &nat44_ed.Nat44AddDelStaticMappingV2{IsAdd: false, Flags: m.Flags, LocalIPAddress: m.LocalIPAddress, ExternalIPAddress: m.ExternalIPAddress,
			Protocol: m.Protocol, LocalPort: m.LocalPort, ExternalPort: m.ExternalPort, ExternalSwIfIndex: m.ExternalSwIfIndex, VrfID: m.VrfID, Tag: tag}
		if _, err := svc.Nat44AddDelStaticMappingV2(ctx, req); err != nil {
			t.Fatalf("loss: delete static mapping %s: %v", tag, err)
		}
		ev = append(ev, fmt.Sprintf("nat44_add_del_static_mapping_v2 is_add=0 tag=%s → ok", tag))
	}
	for _, a := range o.addrs {
		if !slotNet.Contains(netip.AddrFrom4(a.IPAddress)) {
			continue
		}
		req := &nat44_ed.Nat44AddDelAddressRange{FirstIPAddress: a.IPAddress, LastIPAddress: a.IPAddress, VrfID: a.VrfID, IsAdd: false, Flags: a.Flags & nat_types.NAT_IS_TWICE_NAT}
		if _, err := svc.Nat44AddDelAddressRange(ctx, req); err != nil {
			t.Fatalf("loss: delete pool address %s: %v", ip4(a.IPAddress), err)
		}
		ev = append(ev, fmt.Sprintf("nat44_add_del_address_range is_add=0 %s vrf %d → ok", ip4(a.IPAddress), a.VrfID))
	}
	for _, f := range o.features {
		n, ok := ifIdx[uint32(f.SwIfIndex)]
		if !ok {
			continue
		}
		for _, side := range []nat_types.NatConfigFlags{nat_types.NAT_IS_INSIDE, nat_types.NAT_IS_OUTSIDE} {
			if f.Flags&side == 0 {
				continue
			}
			if _, err := svc.Nat44InterfaceAddDelFeature(ctx, &nat44_ed.Nat44InterfaceAddDelFeature{IsAdd: false, Flags: side, SwIfIndex: f.SwIfIndex}); err != nil {
				t.Fatalf("loss: delete interface feature %s: %v", n, err)
			}
			ev = append(ev, fmt.Sprintf("nat44_interface_add_del_feature is_add=0 %s flags=%d → ok", n, side))
		}
	}
	return ev
}
