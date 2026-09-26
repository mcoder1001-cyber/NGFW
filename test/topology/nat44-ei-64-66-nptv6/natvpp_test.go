package nat44ei6466nptv6

// NAT access of the test itself (never of the product): the plugin fixtures (D-071: a slot never enables a plugin
// through its agent; the test does and restores the previous state — natcommon/nattest's semantics, re-done here
// because a test module cannot import the agent's internal packages), the slot / globals locks, the simulated loss
// behind the agent's back and the "nothing of ours remains" dumps for nat44-ei, nat64, nat66 and npt66.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	vppapi "go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/binapi/nat64"
	"ngfw/agent/binapi/nat66"
	"ngfw/agent/binapi/nat_types"
	"ngfw/agent/binapi/npt66"
)

const globalsLock = "/run/lock/vrx-globals.lock" // D-082: tests that read VPP-wide settings hold it shared

// fixtureLock is nattest.EnsurePlugin's host-wide lock for a plugin ("nat44-ei", "nat64", "nat66").
func fixtureLock(plugin string) string { return "/run/lock/vrx-nat-fixture-" + plugin + ".lock" }

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

func edEnabled(t *testing.T, conn vppapi.Connection) bool {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	rc, err := nat44_ed.NewServiceClient(conn).Nat44ShowRunningConfig(ctx, &nat44_ed.Nat44ShowRunningConfig{})
	if err != nil {
		t.Fatalf("nat44_show_running_config: %v", err)
	}
	return rc.Sessions != 0
}

type eiRunning struct {
	enabled             bool
	insideVrf, outsideV uint32
	flags               nat44_ei.Nat44EiConfigFlags
	forwarding          bool
	timeouts            nat_types.NatTimeouts
}

func eiRunningConfig(t *testing.T, conn vppapi.Connection) eiRunning {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	rc, err := nat44_ei.NewServiceClient(conn).Nat44EiShowRunningConfig(ctx, &nat44_ei.Nat44EiShowRunningConfig{})
	if err != nil {
		t.Fatalf("nat44_ei_show_running_config: %v", err)
	}
	return eiRunning{enabled: rc.Sessions != 0, insideVrf: rc.InsideVrf, outsideV: rc.OutsideVrf, flags: rc.Flags, forwarding: rc.ForwardingEnabled, timeouts: rc.Timeouts}
}

// ---- dumps of every owner (the D-071 emptiness checks) ---------------------------------------------------------

type eiObjects struct {
	features []*nat44_ei.Nat44EiInterfaceDetails
	outputs  []*nat44_ei.Nat44EiOutputInterfaceDetails
	addrs    []*nat44_ei.Nat44EiAddressDetails
	ifAddrs  []*nat44_ei.Nat44EiInterfaceAddrDetails
	statics  []*nat44_ei.Nat44EiStaticMappingDetails
	idents   []*nat44_ei.Nat44EiIdentityMappingDetails
}

func dumpEI(t *testing.T, conn vppapi.Connection) eiObjects {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	svc := nat44_ei.NewServiceClient(conn)
	var o eiObjects
	chk := func(err error, what string) {
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	s1, err := svc.Nat44EiInterfaceDump(ctx, &nat44_ei.Nat44EiInterfaceDump{})
	chk(err, "nat44_ei_interface_dump")
	o.features, err = drain(s1.Recv)
	chk(err, "nat44_ei_interface_dump")
	s2, err := svc.Nat44EiAddressDump(ctx, &nat44_ei.Nat44EiAddressDump{})
	chk(err, "nat44_ei_address_dump")
	o.addrs, err = drain(s2.Recv)
	chk(err, "nat44_ei_address_dump")
	s3, err := svc.Nat44EiInterfaceAddrDump(ctx, &nat44_ei.Nat44EiInterfaceAddrDump{})
	chk(err, "nat44_ei_interface_addr_dump")
	o.ifAddrs, err = drain(s3.Recv)
	chk(err, "nat44_ei_interface_addr_dump")
	s4, err := svc.Nat44EiStaticMappingDump(ctx, &nat44_ei.Nat44EiStaticMappingDump{})
	chk(err, "nat44_ei_static_mapping_dump")
	o.statics, err = drain(s4.Recv)
	chk(err, "nat44_ei_static_mapping_dump")
	s5, err := svc.Nat44EiIdentityMappingDump(ctx, &nat44_ei.Nat44EiIdentityMappingDump{})
	chk(err, "nat44_ei_identity_mapping_dump")
	o.idents, err = drain(s5.Recv)
	chk(err, "nat44_ei_identity_mapping_dump")
	s6, err := svc.Nat44EiOutputInterfaceGet(ctx, &nat44_ei.Nat44EiOutputInterfaceGet{Cursor: 0})
	chk(err, "nat44_ei_output_interface_get")
	for {
		d, _, err := s6.Recv()
		if err != nil {
			break // EOF, or INVALID_VALUE on an empty table
		}
		o.outputs = append(o.outputs, d)
	}
	return o
}

func (o eiObjects) empty() bool {
	return len(o.features)+len(o.outputs)+len(o.addrs)+len(o.ifAddrs)+len(o.statics)+len(o.idents) == 0
}

type n64Objects struct {
	ifaces   []*nat64.Nat64InterfaceDetails
	prefixes []*nat64.Nat64PrefixDetails
	pool     []*nat64.Nat64PoolAddrDetails
	bibs     []*nat64.Nat64BibDetails // static only
}

func dump64(t *testing.T, conn vppapi.Connection) n64Objects {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	svc := nat64.NewServiceClient(conn)
	var o n64Objects
	chk := func(err error, what string) {
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	s1, err := svc.Nat64InterfaceDump(ctx, &nat64.Nat64InterfaceDump{})
	chk(err, "nat64_interface_dump")
	o.ifaces, err = drain(s1.Recv)
	chk(err, "nat64_interface_dump")
	s2, err := svc.Nat64PrefixDump(ctx, &nat64.Nat64PrefixDump{})
	chk(err, "nat64_prefix_dump")
	o.prefixes, err = drain(s2.Recv)
	chk(err, "nat64_prefix_dump")
	s3, err := svc.Nat64PoolAddrDump(ctx, &nat64.Nat64PoolAddrDump{})
	chk(err, "nat64_pool_addr_dump")
	o.pool, err = drain(s3.Recv)
	chk(err, "nat64_pool_addr_dump")
	s4, err := svc.Nat64BibDump(ctx, &nat64.Nat64BibDump{Proto: 255})
	chk(err, "nat64_bib_dump")
	bibs, err := drain(s4.Recv)
	chk(err, "nat64_bib_dump")
	for _, b := range bibs {
		if b.Flags&nat_types.NAT_IS_STATIC != 0 {
			o.bibs = append(o.bibs, b)
		}
	}
	return o
}

func (o n64Objects) empty() bool { return len(o.ifaces)+len(o.prefixes)+len(o.pool)+len(o.bibs) == 0 }

type n66Objects struct {
	ifaces   []*nat66.Nat66InterfaceDetails
	mappings []*nat66.Nat66StaticMappingDetails
}

func dump66(t *testing.T, conn vppapi.Connection) n66Objects {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	svc := nat66.NewServiceClient(conn)
	var o n66Objects
	s1, err := svc.Nat66InterfaceDump(ctx, &nat66.Nat66InterfaceDump{})
	if err != nil {
		t.Fatalf("nat66_interface_dump: %v", err)
	}
	if o.ifaces, err = drain(s1.Recv); err != nil {
		t.Fatalf("nat66_interface_dump: %v", err)
	}
	s2, err := svc.Nat66StaticMappingDump(ctx, &nat66.Nat66StaticMappingDump{})
	if err != nil {
		t.Fatalf("nat66_static_mapping_dump: %v", err)
	}
	if o.mappings, err = drain(s2.Recv); err != nil {
		t.Fatalf("nat66_static_mapping_dump: %v", err)
	}
	return o
}

func (o n66Objects) empty() bool { return len(o.ifaces)+len(o.mappings) == 0 }

// ---- plugin fixtures (nattest.EnsurePlugin) ---------------------------------------------------------------------

// plugin is one fixture: its lock is held shared for the test; release (in Cleanup, or earlier when the test no
// longer needs the plugin) disables it again only if this test enabled it and it is empty for every owner, under the
// exclusive lock.
type plugin struct {
	name     string
	wasOn    bool
	f        *os.File
	released bool
	empty    func() bool
	disable  func(ctx context.Context) error
}

func (p *plugin) release(t *testing.T) {
	t.Helper()
	if p == nil || p.released {
		return
	}
	p.released = true
	defer func() { _ = p.f.Close() }()
	if p.wasOn {
		t.Logf("fixture: %s was enabled before this test: it stays enabled", p.name)
		return
	}
	if err := syscall.Flock(int(p.f.Fd()), syscall.LOCK_EX); err != nil {
		t.Errorf("fixture: exclusive lock %s: %v", p.name, err)
		return
	}
	if !p.empty() {
		t.Logf("fixture: %s holds objects of another owner now: left enabled (D-071)", p.name)
		return
	}
	ctx, cancel := ctx10()
	defer cancel()
	if err := p.disable(ctx); err != nil {
		t.Errorf("fixture: disable %s: %v", p.name, err)
		return
	}
	t.Logf("fixture: %s disabled again under the exclusive fixture lock (previous state restored)", p.name)
}

func alreadyEnabled(err error) bool {
	var rv vppapi.VPPApiError
	return errors.As(err, &rv) && (rv == vppapi.FEATURE_ALREADY_ENABLED || rv == 1)
}

func ensure(t *testing.T, name string, enable func(context.Context) error, empty func() bool, disable func(context.Context) error) *plugin {
	t.Helper()
	p := &plugin{name: name, f: flock(t, fixtureLock(name), syscall.LOCK_SH), empty: empty, disable: disable}
	ctx, cancel := ctx10()
	defer cancel()
	err := enable(ctx)
	switch {
	case alreadyEnabled(err):
		p.wasOn = true
		t.Logf("fixture: %s was already enabled (by another owner): it stays enabled", name)
	case err != nil:
		_ = p.f.Close()
		t.Fatalf("fixture: enable %s: %v", name, err)
	default:
		t.Logf("fixture: %s enabled for this test", name)
	}
	t.Cleanup(func() { p.release(t) })
	return p
}

func ensureEI(t *testing.T, conn vppapi.Connection) *plugin {
	svc := nat44_ei.NewServiceClient(conn)
	return ensure(t, "nat44-ei",
		func(ctx context.Context) error {
			if eiRunningConfig(t, conn).enabled {
				return vppapi.FEATURE_ALREADY_ENABLED
			}
			_, err := svc.Nat44EiPluginEnableDisable(ctx, &nat44_ei.Nat44EiPluginEnableDisable{Enable: true})
			return err
		},
		func() bool { return dumpEI(t, conn).empty() },
		func(ctx context.Context) error {
			_, err := svc.Nat44EiPluginEnableDisable(ctx, &nat44_ei.Nat44EiPluginEnableDisable{Enable: false})
			return err
		})
}

func ensure64(t *testing.T, conn vppapi.Connection) *plugin {
	svc := nat64.NewServiceClient(conn)
	return ensure(t, "nat64",
		func(ctx context.Context) error {
			_, err := svc.Nat64PluginEnableDisable(ctx, &nat64.Nat64PluginEnableDisable{Enable: true})
			return err
		},
		func() bool { return dump64(t, conn).empty() },
		func(ctx context.Context) error {
			_, err := svc.Nat64PluginEnableDisable(ctx, &nat64.Nat64PluginEnableDisable{Enable: false})
			return err
		})
}

func ensure66(t *testing.T, conn vppapi.Connection) *plugin {
	svc := nat66.NewServiceClient(conn)
	return ensure(t, "nat66",
		func(ctx context.Context) error {
			_, err := svc.Nat66PluginEnableDisable(ctx, &nat66.Nat66PluginEnableDisable{Enable: true})
			return err
		},
		func() bool { return dump66(t, conn).empty() },
		func(ctx context.Context) error {
			_, err := svc.Nat66PluginEnableDisable(ctx, &nat66.Nat66PluginEnableDisable{Enable: false})
			return err
		})
}

// ---- what of this slot is in VPP ---------------------------------------------------------------------------------

func ip4s(b [4]uint8) string            { return netip.AddrFrom4(b).String() }
func ip6s(b ip_types.IP6Address) string { return netip.AddrFrom16(b).String() }
func pfx6(p ip_types.IP6Prefix) string {
	return netip.PrefixFrom(netip.AddrFrom16(p.Address), int(p.Len)).String()
}
func owns6(scope netip.Prefix, a string) bool {
	x, err := netip.ParseAddr(a)
	return err == nil && scope.Contains(x)
}
func ownsTable(s slot, vrf uint32) bool {
	return vrf >= uint32(s.num)*1000 && vrf <= uint32(s.num)*1000+999
}                                                            //nolint:gosec // slot ≤ 11
func tagOf(tag string) string                                { return strings.TrimRight(tag, "\x00") }
func ours4(net netip.Prefix, b [4]uint8) bool                { return net.Contains(netip.AddrFrom4(b)) }
func ourIf(ifIdx map[uint32]string, i uint32) (string, bool) { n, ok := ifIdx[i]; return n, ok }

// scope is what this slot owns: 10.N/16, fd00:N::/32, tables N000–N999, its interfaces, tags "<prefix>:".
type scope struct {
	s     slot
	v4    netip.Prefix
	v6    netip.Prefix
	ifIdx map[uint32]string
}

// ours lists every NAT object of this slot in nat44-ei, nat64 and nat66 (npt66 has no dump: see npt66Lines).
func (sc scope) ours(t *testing.T, conn vppapi.Connection) []string {
	t.Helper()
	var out []string
	ei := dumpEI(t, conn)
	for _, f := range ei.features {
		if n, ok := ourIf(sc.ifIdx, uint32(f.SwIfIndex)); ok {
			out = append(out, fmt.Sprintf("nat44-ei interface-feature %s flags=%d", n, f.Flags))
		}
	}
	for _, f := range ei.outputs {
		if n, ok := ourIf(sc.ifIdx, uint32(f.SwIfIndex)); ok {
			out = append(out, "nat44-ei output-feature "+n)
		}
	}
	for _, a := range ei.addrs {
		if ours4(sc.v4, a.IPAddress) {
			out = append(out, fmt.Sprintf("nat44-ei pool address %s vrf %d", ip4s(a.IPAddress), a.VrfID))
		}
	}
	for _, a := range ei.ifAddrs {
		if n, ok := ourIf(sc.ifIdx, uint32(a.SwIfIndex)); ok {
			out = append(out, "nat44-ei interface-address "+n)
		}
	}
	for _, m := range ei.statics {
		if strings.HasPrefix(tagOf(m.Tag), sc.s.prefix+":") {
			out = append(out, fmt.Sprintf("nat44-ei static mapping %s %s:%d -> %s:%d", tagOf(m.Tag), ip4s(m.LocalIPAddress), m.LocalPort, ip4s(m.ExternalIPAddress), m.ExternalPort))
		}
	}
	for _, m := range ei.idents {
		if strings.HasPrefix(tagOf(m.Tag), sc.s.prefix+":") {
			out = append(out, "nat44-ei identity mapping "+tagOf(m.Tag))
		}
	}
	n6 := dump64(t, conn)
	for _, i := range n6.ifaces {
		if n, ok := ourIf(sc.ifIdx, uint32(i.SwIfIndex)); ok {
			out = append(out, fmt.Sprintf("nat64 interface %s flags=%d", n, i.Flags))
		}
	}
	for _, p := range n6.prefixes {
		if sc.v6.Contains(netip.AddrFrom16(p.Prefix.Address)) || ownsTable(sc.s, p.VrfID) {
			out = append(out, fmt.Sprintf("nat64 prefix %s vrf %d", pfx6(p.Prefix), p.VrfID))
		}
	}
	for _, a := range n6.pool {
		if ours4(sc.v4, a.Address) {
			out = append(out, fmt.Sprintf("nat64 pool address %s vrf %d", ip4s(a.Address), a.VrfID))
		}
	}
	for _, b := range n6.bibs {
		if ours4(sc.v4, b.OAddr) || ownsTable(sc.s, b.VrfID) {
			out = append(out, fmt.Sprintf("nat64 static bib %s:%d -> %s:%d proto %d vrf %d", ip6s(b.IAddr), b.IPort, ip4s(b.OAddr), b.OPort, b.Proto, b.VrfID))
		}
	}
	n66 := dump66(t, conn)
	for _, i := range n66.ifaces {
		if n, ok := ourIf(sc.ifIdx, uint32(i.SwIfIndex)); ok {
			out = append(out, fmt.Sprintf("nat66 interface %s flags=%d", n, i.Flags))
		}
	}
	for _, m := range n66.mappings {
		if sc.v6.Contains(netip.AddrFrom16(m.LocalIPAddress)) || sc.v6.Contains(netip.AddrFrom16(m.ExternalIPAddress)) || ownsTable(sc.s, m.VrfID) {
			out = append(out, fmt.Sprintf("nat66 static mapping %s <-> %s vrf %d", ip6s(m.LocalIPAddress), ip6s(m.ExternalIPAddress), m.VrfID))
		}
	}
	return out
}

var npt66Line = regexp.MustCompile(`^\[(\d+)\] internal: (\S+) external: (\S+)$`)

// npt66Lines are the lines of `vppctl show npt66 bindings` whose internal prefix is in the slot's IPv6 block (the
// command prints every owner's bindings, without the interface; the binary API has no dump).
func (sc scope) npt66Lines(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, l := range strings.Split(vppctl(t, "show", "npt66", "bindings"), "\n") {
		m := npt66Line.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		if p, err := netip.ParsePrefix(m[2]); err == nil && sc.v6.Contains(p.Addr()) {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

// loss deletes this slot's NAT objects behind the agent's back, dependents first (D-095 c): the npt66 binding, then
// mappings / BIBs, pools and prefixes, then the interface features — never the interfaces.
func (sc scope) loss(t *testing.T, conn vppapi.Connection, bindings map[string]string) []string {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	var ev []string
	for ifName, internal := range bindings { // npt66: delete by sw_if_index (the prefixes are ignored by VPP)
		idx, ok := uint32(0), false
		for i, n := range sc.ifIdx {
			if n == ifName {
				idx, ok = i, true
			}
		}
		if !ok {
			continue
		}
		pfx := netip.MustParsePrefix(internal)
		req := &npt66.Npt66BindingAddDel{IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(idx),
			Internal: ip_types.IP6Prefix{Address: pfx.Addr().As16(), Len: uint8(pfx.Bits())}, External: ip_types.IP6Prefix{Address: pfx.Addr().As16(), Len: uint8(pfx.Bits())}} //nolint:gosec // ≤ 64
		_, err := npt66.NewServiceClient(conn).Npt66BindingAddDel(ctx, req)
		ev = append(ev, fmt.Sprintf("npt66_binding_add_del is_add=0 %s (sw_if_index %d) → %v", ifName, idx, errText(err)))
	}
	ei := nat44_ei.NewServiceClient(conn)
	o := dumpEI(t, conn)
	for _, m := range o.statics {
		tag := tagOf(m.Tag)
		if !strings.HasPrefix(tag, sc.s.prefix+":") {
			continue
		}
		_, err := ei.Nat44EiAddDelStaticMapping(ctx, &nat44_ei.Nat44EiAddDelStaticMapping{IsAdd: false, Flags: m.Flags, LocalIPAddress: m.LocalIPAddress, ExternalIPAddress: m.ExternalIPAddress,
			Protocol: m.Protocol, LocalPort: m.LocalPort, ExternalPort: m.ExternalPort, ExternalSwIfIndex: m.ExternalSwIfIndex, VrfID: m.VrfID, Tag: tag})
		ev = append(ev, fmt.Sprintf("nat44_ei_add_del_static_mapping is_add=0 tag=%s → %v", tag, errText(err)))
	}
	for _, m := range o.idents {
		tag := tagOf(m.Tag)
		if !strings.HasPrefix(tag, sc.s.prefix+":") {
			continue
		}
		_, err := ei.Nat44EiAddDelIdentityMapping(ctx, &nat44_ei.Nat44EiAddDelIdentityMapping{IsAdd: false, Flags: m.Flags, IPAddress: m.IPAddress, Protocol: m.Protocol, Port: m.Port, SwIfIndex: m.SwIfIndex, VrfID: m.VrfID, Tag: tag})
		ev = append(ev, fmt.Sprintf("nat44_ei_add_del_identity_mapping is_add=0 tag=%s → %v", tag, errText(err)))
	}
	for _, a := range o.addrs {
		if !ours4(sc.v4, a.IPAddress) {
			continue
		}
		_, err := ei.Nat44EiAddDelAddressRange(ctx, &nat44_ei.Nat44EiAddDelAddressRange{FirstIPAddress: a.IPAddress, LastIPAddress: a.IPAddress, VrfID: a.VrfID, IsAdd: false})
		ev = append(ev, fmt.Sprintf("nat44_ei_add_del_address_range is_add=0 %s → %v", ip4s(a.IPAddress), errText(err)))
	}
	for _, f := range o.features {
		n, ok := ourIf(sc.ifIdx, uint32(f.SwIfIndex))
		if !ok {
			continue
		}
		for _, side := range []nat44_ei.Nat44EiConfigFlags{nat44_ei.NAT44_EI_IF_INSIDE, nat44_ei.NAT44_EI_IF_OUTSIDE} {
			if f.Flags&side == 0 {
				continue
			}
			_, err := ei.Nat44EiInterfaceAddDelFeature(ctx, &nat44_ei.Nat44EiInterfaceAddDelFeature{IsAdd: false, Flags: side, SwIfIndex: f.SwIfIndex})
			ev = append(ev, fmt.Sprintf("nat44_ei_interface_add_del_feature is_add=0 %s flags=%d → %v", n, side, errText(err)))
		}
	}
	n64 := nat64.NewServiceClient(conn)
	o6 := dump64(t, conn)
	for _, b := range o6.bibs {
		if !ours4(sc.v4, b.OAddr) && !ownsTable(sc.s, b.VrfID) {
			continue
		}
		_, err := n64.Nat64AddDelStaticBib(ctx, &nat64.Nat64AddDelStaticBib{IAddr: b.IAddr, OAddr: b.OAddr, IPort: b.IPort, OPort: b.OPort, VrfID: b.VrfID, Proto: b.Proto, IsAdd: false})
		ev = append(ev, fmt.Sprintf("nat64_add_del_static_bib is_add=0 %s:%d → %v", ip6s(b.IAddr), b.IPort, errText(err)))
	}
	for _, a := range o6.pool {
		if !ours4(sc.v4, a.Address) {
			continue
		}
		_, err := n64.Nat64AddDelPoolAddrRange(ctx, &nat64.Nat64AddDelPoolAddrRange{StartAddr: a.Address, EndAddr: a.Address, VrfID: a.VrfID, IsAdd: false})
		ev = append(ev, fmt.Sprintf("nat64_add_del_pool_addr_range is_add=0 %s vrf %d → %v", ip4s(a.Address), a.VrfID, errText(err)))
	}
	for _, p := range o6.prefixes {
		if !sc.v6.Contains(netip.AddrFrom16(p.Prefix.Address)) && !ownsTable(sc.s, p.VrfID) {
			continue
		}
		_, err := n64.Nat64AddDelPrefix(ctx, &nat64.Nat64AddDelPrefix{Prefix: p.Prefix, VrfID: p.VrfID, IsAdd: false})
		ev = append(ev, fmt.Sprintf("nat64_add_del_prefix is_add=0 %s vrf %d → %v", pfx6(p.Prefix), p.VrfID, errText(err)))
	}
	for _, i := range o6.ifaces {
		n, ok := ourIf(sc.ifIdx, uint32(i.SwIfIndex))
		if !ok {
			continue
		}
		for _, side := range []nat_types.NatConfigFlags{nat_types.NAT_IS_INSIDE, nat_types.NAT_IS_OUTSIDE} {
			if i.Flags&side == 0 {
				continue
			}
			_, err := n64.Nat64AddDelInterface(ctx, &nat64.Nat64AddDelInterface{IsAdd: false, Flags: side, SwIfIndex: i.SwIfIndex})
			ev = append(ev, fmt.Sprintf("nat64_add_del_interface is_add=0 %s flags=%d → %v", n, side, errText(err)))
		}
	}
	n66 := nat66.NewServiceClient(conn)
	o66 := dump66(t, conn)
	for _, m := range o66.mappings {
		if !sc.v6.Contains(netip.AddrFrom16(m.LocalIPAddress)) && !sc.v6.Contains(netip.AddrFrom16(m.ExternalIPAddress)) && !ownsTable(sc.s, m.VrfID) {
			continue
		}
		_, err := n66.Nat66AddDelStaticMapping(ctx, &nat66.Nat66AddDelStaticMapping{IsAdd: false, LocalIPAddress: m.LocalIPAddress, ExternalIPAddress: m.ExternalIPAddress, VrfID: m.VrfID})
		ev = append(ev, fmt.Sprintf("nat66_add_del_static_mapping is_add=0 %s → %v", ip6s(m.LocalIPAddress), errText(err)))
	}
	for _, i := range o66.ifaces {
		n, ok := ourIf(sc.ifIdx, uint32(i.SwIfIndex))
		if !ok {
			continue
		}
		_, err := n66.Nat66AddDelInterface(ctx, &nat66.Nat66AddDelInterface{IsAdd: false, Flags: i.Flags, SwIfIndex: i.SwIfIndex})
		ev = append(ev, fmt.Sprintf("nat66_add_del_interface is_add=0 %s → %v", n, errText(err)))
	}
	return ev
}

func errText(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}

// gcSlotVRF removes what a failed run leaves of the slot VRF (the agent never got to the rollback): the NAT64 phase's route and the
// IPv4/IPv6 tables (API source), through the binary API. The IPv6 table can survive with nat64 locks — VPP 26.06's
// nat64_add_del_prefix delete does not unlock its FIB ("TODO: missing fib_table_unlock"), a leak until VPP restarts.
func gcSlotVRF(t *testing.T, conn vppapi.Connection, table uint32, route netip.Prefix) []string {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	svc := ip.NewServiceClient(conn)
	var ev []string
	pfx := ip_types.Prefix{Address: ip_types.Address{Af: ip_types.ADDRESS_IP4, Un: ip_types.AddressUnionIP4(ip_types.IP4Address(route.Addr().As4()))}, Len: uint8(route.Bits())} //nolint:gosec // ≤ 32
	_, err := svc.IPRouteAddDel(ctx, &ip.IPRouteAddDel{IsAdd: false, Route: ip.IPRoute{TableID: table, Prefix: pfx}})
	ev = append(ev, fmt.Sprintf("ip_route_add_del is_add=0 table %d %s → %s", table, route, errText(err)))
	for _, v6 := range []bool{false, true} {
		_, err := svc.IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: false, Table: ip.IPTable{TableID: table, IsIP6: v6}})
		ev = append(ev, fmt.Sprintf("ip_table_add_del is_add=0 table %d ipv6=%v → %s", table, v6, errText(err)))
	}
	return ev
}
