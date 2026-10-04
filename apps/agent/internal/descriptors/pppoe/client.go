package pppoe

// PPPoE *client* (F-pppoe-client / F-pppoe-client-host, D-168): reflect the pppd-negotiated address, default
// route and MSS clamp into VPP. This is a RUNTIME mirror, not a scheduler descriptor: the address and route are
// learned from the live pppd session (the ip-up/ip-down hook, read by renderers/pppoe.ReadState), never from the
// desired config document, so the agent's pppoe subsystem calls ClientMirror.Apply when a session comes up or goes
// down rather than registering a Create/Delete descriptor. The VPP-plugin PPPoE *server* descriptors in this same
// package (cp.go, session.go) are unrelated (the AC/decap side).
//
// It resolves the WAN interface by its logical name to the running sw_if_index (df6.DumpInterfaces, owner-scoped,
// so a foreign interface is refused) and, for up=true, adds the ISP-assigned local IPv4/IPv6 address, an optional
// default route via the peer in the interface's own FIB, and (when requested) enables TCP MSS clamping in both
// directions; up=false withdraws exactly those.
//
// Route safety (R4 B1): the default route is added and removed with IsMultipath=true and a SINGLE path — VPP then
// only adds/removes this session's path, so a static default, an ECMP default or a second PPPoE session's default
// survives when this one goes down (fib_api.c:466-480; a whole-entry delete would wipe them). The FIB table is the
// interface's own table (sw_interface_get_table), not a hard-coded 0, and the default route is REFUSED unless this
// agent may write it — the globals owner, or the table is inside the agent's id range (RouteTablePolicy, fail
// closed) — so a slot agent never mutates a table it does not own.
//
// Static overlap (R4 m1): Apply(down) withdraws the ISP address and this session's default path unconditionally. If
// an operator had configured the same address or a static default on this interface, the withdrawal removes it too;
// the PPPoE client owns the WAN address on a dial-up interface (semantic rule: no static address on a pppoe client),
// so this is the intended ownership, documented in docs/agent/descriptors/pppoe.md and docs/user/network/pppoe.md.

import (
	"context"
	"errors"
	"fmt"
	"go.fd.io/govpp/api"
	"io"
	"log/slog"
	"net/netip"

	fib_types "ngfw/agent/binapi/fib_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	mssclamp "ngfw/agent/binapi/mss_clamp"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/vpp"
)

// ErrNotGlobalsOwner is returned when a default route is requested for a FIB table this agent may not write
// (not the globals owner and the table is outside its id range) — the mirror fails closed.
var ErrNotGlobalsOwner = errors.New("pppoe: default route refused: not the globals owner and table outside the agent's id range")

// Mirror is the negotiated state of one PPPoE client session to reflect into VPP.
type Mirror struct {
	// Interface is the logical (owner-scoped) name of the WAN VPP interface the session runs over.
	Interface string
	// LocalIPv4 / LocalIPv6 are the ISP-assigned addresses in CIDR form ("a.b.c.d/32", "…/64"); empty = none.
	LocalIPv4 string
	LocalIPv6 string
	// PeerIPv4 is the peer/gateway address (the default-route next hop); empty = a link-only default route.
	PeerIPv4 string
	// DefaultRoute adds/removes the default 0.0.0.0/0 path out this interface via the peer.
	DefaultRoute bool
	// MSSClamp enables TCP MSS clamping (RX+TX) on the interface; MSS is derived from MTU.
	MSSClamp bool
	// MTU is the negotiated link MTU; the IPv4 clamp is MTU-40, the IPv6 clamp MTU-60.
	MTU uint32
}

// RouteTablePolicy reports whether this agent may write the default route in FIB table id. It fails closed: a
// ClientMirror with no policy refuses every default route.
type RouteTablePolicy func(table uint32) bool

// ClientMirror reflects PPPoE client sessions into VPP for one agent owner.
type ClientMirror struct {
	c          vpp.Client
	owner      string
	log        *slog.Logger
	allowRoute RouteTablePolicy
}

// Option configures a ClientMirror.
type Option func(*ClientMirror)

// WithRouteTablePolicy sets the guard that decides whether the default route may be written in a FIB table.
func WithRouteTablePolicy(p RouteTablePolicy) Option {
	return func(m *ClientMirror) { m.allowRoute = p }
}

// NewClientMirror returns a mirror for owner (log may be nil). Without WithRouteTablePolicy it refuses every default
// route (fail closed).
func NewClientMirror(c vpp.Client, owner string, log *slog.Logger, opts ...Option) *ClientMirror {
	if log == nil {
		log = slog.Default()
	}
	m := &ClientMirror{c: c, owner: owner, log: log, allowRoute: func(uint32) bool { return false }}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Apply reflects m into VPP (up=true) or withdraws it (up=false). It resolves the WAN interface first; a session
// whose interface is gone on the running VPP is a no-op on withdrawal (nothing to remove) and an error on add.
func (m *ClientMirror) Apply(ctx context.Context, mir Mirror, up bool) error {
	if mir.Interface == "" {
		return fmt.Errorf("%w: pppoe mirror needs an interface", df6.ErrBadValue)
	}
	// Validate the whole negotiated record before the first write.
	for _, raw := range []string{mir.LocalIPv4, mir.LocalIPv6} {
		if raw != "" {
			if _, err := netip.ParsePrefix(raw); err != nil {
				return fmt.Errorf("%w: invalid negotiated address", df6.ErrBadValue)
			}
		}
	}
	if mir.PeerIPv4 != "" {
		peer, err := netip.ParseAddr(mir.PeerIPv4)
		if err != nil || !peer.Is4() {
			return fmt.Errorf("%w: invalid negotiated peer", df6.ErrBadValue)
		}
	}
	if mir.MSSClamp && (mir.MTU < 128 || mir.MTU > 1500) {
		return fmt.Errorf("%w: invalid negotiated MTU", df6.ErrBadValue)
	}

	ifs, err := df6.DumpInterfaces(ctx, m.c, m.owner)
	if err != nil {
		return err
	}
	idx, err := ifs.Index(mir.Interface)
	if err != nil {
		if !up && df6.IsNoSuchInterface(err) {
			return nil // interface gone: nothing left to withdraw
		}
		return err
	}
	// Resolve the table and check the route policy BEFORE touching VPP: a refusal is permanent, so an up that would be
	// refused must not leave the address behind.
	wantRoute := mir.DefaultRoute && mir.LocalIPv4 != ""
	var table uint32
	if wantRoute {
		if table, err = m.tableOf(ctx, idx); err != nil {
			return err
		}
		if !m.allowRoute(table) {
			return fmt.Errorf("%w (table %d)", ErrNotGlobalsOwner, table)
		}
	}
	if err := m.address(ctx, idx, mir.LocalIPv4, up); err != nil {
		return err
	}
	if err := m.address(ctx, idx, mir.LocalIPv6, up); err != nil {
		return err
	}
	if wantRoute {
		if err := m.defaultRoute(ctx, idx, table, mir.PeerIPv4, up); err != nil {
			return err
		}
	}
	if mir.MSSClamp {
		if err := m.mssClamp(ctx, idx, mir.MTU, up); err != nil {
			return err
		}
	}
	m.log.Info("pppoe mirror", "interface", mir.Interface, "sw_if_index", uint32(idx), "up", up,
		"local4", mir.LocalIPv4, "local6", mir.LocalIPv6, "default_route", mir.DefaultRoute, "mss_clamp", mir.MSSClamp)
	return nil
}

// address adds or removes one interface address given in CIDR form (empty = skip).
func (m *ClientMirror) address(ctx context.Context, idx interface_types.InterfaceIndex, cidr string, add bool) error {
	if cidr == "" {
		return nil
	}
	pfx, err := ip_types.ParseAddressWithPrefix(cidr)
	if err != nil {
		return fmt.Errorf("%w: pppoe address %q: %v", df6.ErrBadValue, cidr, err)
	}

	stream, err := ip.NewServiceClient(m.c).IPAddressDump(ctx, &ip.IPAddressDump{SwIfIndex: idx, IsIPv6: pfx.Address.Af == ip_types.ADDRESS_IP6})
	if err != nil {
		return err
	}
	present := false
	for {
		row, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if row.Prefix.String() == pfx.String() {
			present = true
		}
	}
	if present == add {
		return nil
	}

	if _, err := interfaces.NewServiceClient(m.c).SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{
		SwIfIndex: idx, IsAdd: add, Prefix: pfx,
	}); err != nil {
		return fmt.Errorf("sw_interface_add_del_address %s add=%v: %w", cidr, add, err)
	}
	return nil
}

// tableOf returns the IPv4 FIB table (VRF) the interface is bound to.
func (m *ClientMirror) tableOf(ctx context.Context, idx interface_types.InterfaceIndex) (uint32, error) {
	rep, err := interfaces.NewServiceClient(m.c).SwInterfaceGetTable(ctx, &interfaces.SwInterfaceGetTable{SwIfIndex: idx, IsIPv6: false})
	if err != nil {
		return 0, fmt.Errorf("sw_interface_get_table sw_if_index=%d: %w", uint32(idx), err)
	}
	if rep.Retval != 0 {
		return 0, fmt.Errorf("sw_interface_get_table sw_if_index=%d: retval %d", uint32(idx), rep.Retval)
	}
	return rep.VrfID, nil
}

// defaultRoute adds or removes this session's single default path (IsMultipath=true, so only this path is touched)
// in the interface's own FIB table (resolved and policy-checked by Apply), out idx via peer when set.
func (m *ClientMirror) defaultRoute(ctx context.Context, idx interface_types.InterfaceIndex, table uint32, peer string, add bool) error {
	pfx, err := ip_types.ParsePrefix("0.0.0.0/0")
	if err != nil {
		return err
	}
	fp := fib_types.FibPath{
		SwIfIndex: uint32(idx),
		TableID:   table,
		Type:      fib_types.FIB_API_PATH_TYPE_NORMAL,
		Proto:     fib_types.FIB_API_PATH_NH_PROTO_IP4,
		Weight:    1,
	}
	if peer != "" {
		a, err := netip.ParseAddr(peer)
		if err != nil || !a.Is4() {
			return fmt.Errorf("%w: pppoe peer %q is not an IPv4 address", df6.ErrBadValue, peer)
		}
		fp.Nh.Address = ip_types.AddressUnionIP4(ip_types.IP4Address(a.As4()))
	}
	// IsMultipath=true on BOTH add and delete: VPP adds/removes only this path, leaving any other 0.0.0.0/0 path
	// (a static default, ECMP, or a second PPPoE session) intact (fib_api.c:466-480).
	route := ip.IPRoute{TableID: table, Prefix: pfx, NPaths: 1, Paths: []fib_types.FibPath{fp}}
	if _, err := ip.NewServiceClient(m.c).IPRouteAddDel(ctx, &ip.IPRouteAddDel{IsAdd: add, IsMultipath: true, Route: route}); err != nil {
		if !add && errors.Is(err, api.NO_SUCH_ENTRY) {
			return nil
		}
		return fmt.Errorf("ip_route_add_del default in table %d via %q add=%v: %w", table, peer, add, err)
	}
	return nil
}

// mssClamp enables or disables TCP MSS clamping (RX+TX) on idx. The IPv4 clamp is MTU-40, IPv6 MTU-60; a clamp is
// only sent when the MTU leaves room for a TCP/IP header.
func (m *ClientMirror) mssClamp(ctx context.Context, idx interface_types.InterfaceIndex, mtu uint32, enable bool) error {
	req := &mssclamp.MssClampEnableDisable{SwIfIndex: idx}
	if enable {
		if mtu > 40 {
			req.IPv4Mss = uint16(mtu - 40) //nolint:gosec // mtu ≤ 1500 (renderer-validated)
			req.IPv4Direction = mssclamp.MSS_CLAMP_DIR_RX | mssclamp.MSS_CLAMP_DIR_TX
		}
		if mtu > 60 {
			req.IPv6Mss = uint16(mtu - 60) //nolint:gosec // mtu ≤ 1500
			req.IPv6Direction = mssclamp.MSS_CLAMP_DIR_RX | mssclamp.MSS_CLAMP_DIR_TX
		}
	}
	// enable=false leaves both directions NONE and both MSS 0 → VPP disables the clamp on the interface.
	if _, err := mssclamp.NewServiceClient(m.c).MssClampEnableDisable(ctx, req); err != nil {
		return fmt.Errorf("mss_clamp_enable_disable sw_if_index=%d enable=%v: %w", uint32(idx), enable, err)
	}
	return nil
}
