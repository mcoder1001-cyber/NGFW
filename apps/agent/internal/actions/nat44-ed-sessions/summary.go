package nat44edsessions

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/vpp"
)

// Pool is one owned pool as the summary counts it: a range (First..Last) or an interface with its current
// IPv4 addresses.
type Pool struct {
	First, Last netip.Addr // range pools; for interface pools the interface's lowest / highest address (invalid: none)
	Interface   string     // interface pools
	Addrs       []netip.Addr
	VRF         string // range pools: VRF name ("default" for table 0)
	TwiceNAT    bool
}

// Size is the number of addresses in the pool.
func (p Pool) Size() uint32 {
	if p.Interface != "" {
		return uint32(len(p.Addrs)) //nolint:gosec // a handful of interface addresses
	}
	if !p.First.IsValid() || !p.Last.IsValid() || p.Last.Less(p.First) {
		return 0
	}
	a, b := p.First.As4(), p.Last.As4()
	x := uint32(a[0])<<24 | uint32(a[1])<<16 | uint32(a[2])<<8 | uint32(a[3])
	y := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	return y - x + 1
}

func (p Pool) contains(a netip.Addr) bool {
	if p.Interface != "" {
		for _, x := range p.Addrs {
			if x == a {
				return true
			}
		}
		return false
	}
	return p.First.IsValid() && !a.Less(p.First) && !p.Last.Less(a)
}

// Summary is one NatSummary result.
type Summary struct {
	Enabled        bool
	SessionLimit   uint32
	TotalUsers     uint64
	TotalSessions  uint64
	StaticSessions uint64
	PoolSessions   []uint64 // per pool, same order as the pools passed in
	ByProtocol     map[string]uint64
	Truncated      bool
}

// RunningConfig reads whether nat44-ed is enabled and its per-thread session limit (nat44_show_running_config;
// VPP zeroes the config on disable, so sessions == 0 ⇔ disabled — DF-3's reading).
func RunningConfig(ctx context.Context, c vpp.Client) (enabled bool, sessions uint32, err error) {
	rc, err := nat44_ed.NewServiceClient(c).Nat44ShowRunningConfig(ctx, &nat44_ed.Nat44ShowRunningConfig{})
	if err != nil {
		return false, 0, fmt.Errorf("nat44_show_running_config: %w", err)
	}
	return rc.Sessions != 0, rc.Sessions, nil
}

// Summarize counts the owner's users and sessions (from the user dump) and, in one capped scan, the sessions per
// pool (by outside address) and per protocol.
func Summarize(ctx context.Context, src Source, scope natcommon.Scope, pools []Pool, scanCap int) (Summary, error) {
	if scanCap <= 0 {
		scanCap = DefaultScanCap
	}
	users, err := ownUsers(ctx, src, scope, Filter{})
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{TotalUsers: uint64(len(users)), PoolSessions: make([]uint64, len(pools)), ByProtocol: map[string]uint64{}}
	for _, u := range users {
		sum.TotalSessions += uint64(u.Sessions + u.StaticSessions)
		sum.StaticSessions += uint64(u.StaticSessions)
	}
	scanned := 0
	for _, u := range users {
		if scanned >= scanCap {
			sum.Truncated = true
			break
		}
		ss, err := src.UserSessions(ctx, u, 0, 0)
		if err != nil {
			return Summary{}, err
		}
		scanned += len(ss)
		for _, s := range ss {
			sum.ByProtocol[s.Protocol]++
			out, err := netip.ParseAddr(s.Outside.IP)
			if err != nil {
				continue
			}
			for i, p := range pools {
				if p.contains(out) {
					sum.PoolSessions[i]++
					break
				}
			}
		}
	}
	return sum, nil
}

// Deleter is the kill side (nat44ed.Plugin implements it).
type Deleter interface {
	DeleteSession(ctx context.Context, inside nat44ed.Endpoint, protocol string, vrf uint32, extHost nat44ed.Endpoint) error
}

// Kill is a validated NatSessionKillAction.
type Kill struct {
	Protocol string
	Inside   netip.AddrPort
	External netip.AddrPort
	Table    uint32
}

// ParseKill validates the kill request fields: protocol, IPv4 addresses, 16-bit ports, the VRF, and that the inside
// address belongs to the owner (a slot kills only sessions of its own 10.N.0.0/16 users, D-071 claim rule).
func ParseKill(scope natcommon.Scope, protocol, inside string, insidePort uint32, external string, externalPort uint32, vrf string, resolve func(string) (uint32, bool)) (Kill, error) {
	proto, err := ParseProtocol(protocol)
	if err != nil {
		return Kill{}, err
	}
	in, err := ParseIPv4("inside_address", inside)
	if err != nil {
		return Kill{}, err
	}
	ext, err := ParseIPv4("external_address", external)
	if err != nil {
		return Kill{}, err
	}
	if insidePort > 65535 || externalPort > 65535 {
		return Kill{}, invalid("ports must be 0–65535")
	}
	if !scope.OwnsAddr(in) {
		return Kill{}, invalid("inside address %s does not belong to owner %s", in, scope.Owner)
	}
	table, err := ParseVRF(vrf, resolve)
	if err != nil {
		return Kill{}, err
	}
	return Kill{Protocol: proto, Inside: netip.AddrPortFrom(in, uint16(insidePort)), External: netip.AddrPortFrom(ext, uint16(externalPort)), Table: table}, nil
}

// Exit codes of the kill's `done`.
const (
	KillDeleted  = 0
	KillNotFound = 1
	KillFailed   = 2
)

// Do deletes the session and reports the `done` exit code and summary (nat44_del_session: NO_SUCH_ENTRY = no such
// session; anything else is a failure with VPP's message).
func (k Kill) Do(ctx context.Context, d Deleter) (int, string) {
	err := d.DeleteSession(ctx,
		nat44ed.Endpoint{IP: k.Inside.Addr().String(), Port: uint32(k.Inside.Port())}, k.Protocol, k.Table,
		nat44ed.Endpoint{IP: k.External.Addr().String(), Port: uint32(k.External.Port())})
	switch {
	case err == nil:
		return KillDeleted, fmt.Sprintf("session deleted: %s %s -> %s (table %d)", k.Protocol, k.Inside, k.External, k.Table)
	case natcommon.IsNoSuchEntry(err):
		return KillNotFound, fmt.Sprintf("no such session: %s %s -> %s (table %d)", k.Protocol, k.Inside, k.External, k.Table)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return KillFailed, "cancelled: " + err.Error()
	default:
		return KillFailed, "nat44_del_session failed: " + err.Error()
	}
}

// Stats are the flat `done.stats` of a kill.
func (k Kill) Stats() map[string]string {
	return map[string]string{
		"protocol": k.Protocol, "inside_address": k.Inside.Addr().String(), "inside_port": fmt.Sprint(k.Inside.Port()),
		"external_address": k.External.Addr().String(), "external_port": fmt.Sprint(k.External.Port()), "table_id": fmt.Sprint(k.Table),
	}
}
