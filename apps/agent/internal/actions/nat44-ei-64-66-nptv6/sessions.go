// Package nat44ei6466nptv6 is the state side of F-nat44-ei-64-66-nptv6 as pure functions over DF-3's Retrieve-only
// helpers: the NAT44-EI session browser and kill (descriptors/nat44ei Plugin.Users / UserSessions / DeleteSession) and
// the NAT64 session table (descriptors/nat64 Plugin.Sessions). The agent's NatSessions RPC and the NatSessionKillAction
// case of Action (internal/agent/rpc_nat44_ei.go) dispatch here on NatSessionVariant and only translate to and from
// vrx.v1 (docs/contracts/proto.md §11 "F-nat44-ei-64-66-nptv6: NAT session variants").
//
// EI pages exactly like ED (F-nat44-ed-sessions' pager, reused through an adapter): users first, whole users skipped
// by their counts, filtered scans capped. NAT64 has one dump for the whole table (nat64_st_dump per protocol): the
// agent keeps only this owner's rows of the page and counts the rest up to the scan cap.
package nat44ei6466nptv6

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	natsessions "ngfw/agent/internal/actions/nat44-ed-sessions"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/nat44ei"
	"ngfw/agent/internal/descriptors/nat64"
	"ngfw/agent/internal/descriptors/natcommon"
)

// EISource is the read side of nat44-ei (nat44ei.Plugin implements it).
type EISource interface {
	Users(ctx context.Context) ([]nat44ei.User, error)
	UserSessions(ctx context.Context, user nat44ei.User, offset, limit int) ([]nat44ei.Session, error)
}

// eiAdapter presents nat44-ei users and sessions to the ED pager (the same shape; EI has no twice-NAT, so the external
// host after NAT is "0.0.0.0"/0 as for an ED session without twice-NAT, and no timed-out flag). nat44-ei keeps each
// user's sessions on a per-user list, so a per-user dump walks only that user's sessions (unlike nat44-ed's pool walk).
type eiAdapter struct{ src EISource }

var noTwiceNAT = nat44ed.Endpoint{IP: "0.0.0.0"}

func (a eiAdapter) Users(ctx context.Context) ([]nat44ed.User, error) {
	us, err := a.src.Users(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]nat44ed.User, 0, len(us))
	for _, u := range us {
		out = append(out, nat44ed.User{IP: u.IP, VRF: u.VRF, Sessions: u.Sessions, StaticSessions: u.StaticSessions})
	}
	return out, nil
}

func (a eiAdapter) UserSessions(ctx context.Context, u nat44ed.User, offset, limit int) ([]nat44ed.Session, error) {
	ss, err := a.src.UserSessions(ctx, nat44ei.User{IP: u.IP, VRF: u.VRF, Sessions: u.Sessions, StaticSessions: u.StaticSessions}, offset, limit)
	if err != nil {
		return nil, err
	}
	out := make([]nat44ed.Session, 0, len(ss))
	for _, s := range ss {
		out = append(out, edSession(s))
	}
	return out, nil
}

// EachUserSession implements the pager's streaming read (one user's sessions, fn false = stop using them).
func (a eiAdapter) EachUserSession(ctx context.Context, u nat44ed.User, fn func(nat44ed.Session) bool) error {
	ss, err := a.src.UserSessions(ctx, nat44ei.User{IP: u.IP, VRF: u.VRF, Sessions: u.Sessions, StaticSessions: u.StaticSessions}, 0, 0)
	if err != nil {
		return err
	}
	for _, s := range ss {
		if !fn(edSession(s)) {
			return nil
		}
	}
	return nil
}

func edSession(s nat44ei.Session) nat44ed.Session {
	return nat44ed.Session{
		Inside: nat44ed.Endpoint{IP: s.Inside.IP, Port: s.Inside.Port}, Outside: nat44ed.Endpoint{IP: s.Outside.IP, Port: s.Outside.Port},
		ExtHost: nat44ed.Endpoint{IP: s.ExtHost.IP, Port: s.ExtHost.Port}, ExtHostNAT: noTwiceNAT, Protocol: s.Protocol, Static: s.Static,
		IdleSeconds: s.IdleSeconds, TotalBytes: s.TotalBytes, TotalPkts: s.TotalPkts,
	}
}

// ListEI returns one page of the owner's NAT44-EI sessions (same contract as natsessions.List, the same caps).
func ListEI(ctx context.Context, src EISource, scope natcommon.Scope, f natsessions.Filter, offset, limit int, caps natsessions.Caps) (natsessions.Page, error) {
	return natsessions.List(ctx, eiAdapter{src}, scope, f, offset, limit, caps)
}

// EIDeleter is the kill side of nat44-ei (nat44ei.Plugin implements it).
type EIDeleter interface {
	DeleteSession(ctx context.Context, inside nat44ei.Endpoint, protocol string, vrf uint32, extHost nat44ei.Endpoint) error
}

// KillEI is a validated NAT44-EI NatSessionKillAction: nat44_ei_del_session finds the session by the inside endpoint
// (address, port, protocol, VRF); the external endpoint is optional and only echoed.
type KillEI struct {
	Protocol string
	Inside   netip.AddrPort
	External netip.AddrPort // zero when not given
	Table    uint32
}

// ParseKillEI validates the kill fields: protocol, IPv4 inside address in the owner's scope, 16-bit ports, the VRF;
// the external address may be empty.
func ParseKillEI(scope natcommon.Scope, protocol, inside string, insidePort uint32, external string, externalPort uint32, vrf string, resolve func(string) (uint32, bool)) (KillEI, error) {
	proto, err := natsessions.ParseProtocol(protocol)
	if err != nil {
		return KillEI{}, err
	}
	in, err := natsessions.ParseIPv4("inside_address", inside)
	if err != nil {
		return KillEI{}, err
	}
	if insidePort > 65535 || externalPort > 65535 {
		return KillEI{}, fmt.Errorf("%w: ports must be 0–65535", natsessions.ErrInvalid)
	}
	if !scope.OwnsAddr(in) {
		return KillEI{}, fmt.Errorf("%w: inside address %s does not belong to owner %s", natsessions.ErrInvalid, in, scope.Owner)
	}
	table, err := natsessions.ParseVRF(vrf, resolve)
	if err != nil {
		return KillEI{}, err
	}
	k := KillEI{Protocol: proto, Inside: netip.AddrPortFrom(in, uint16(insidePort)), Table: table}
	if external != "" {
		ext, err := natsessions.ParseIPv4("external_address", external)
		if err != nil {
			return KillEI{}, err
		}
		k.External = netip.AddrPortFrom(ext, uint16(externalPort))
	}
	return k, nil
}

func (k KillEI) String() string {
	return fmt.Sprintf("%s %s (table %d)", k.Protocol, k.Inside, k.Table)
}

// Do deletes the session: exit code natsessions.KillDeleted, KillNotFound (VPP NO_SUCH_ENTRY) or KillFailed.
func (k KillEI) Do(ctx context.Context, d EIDeleter) (int, string) {
	var ext nat44ei.Endpoint
	if k.External.IsValid() {
		ext = nat44ei.Endpoint{IP: k.External.Addr().String(), Port: uint32(k.External.Port())}
	}
	err := d.DeleteSession(ctx, nat44ei.Endpoint{IP: k.Inside.Addr().String(), Port: uint32(k.Inside.Port())}, k.Protocol, k.Table, ext)
	switch {
	case err == nil:
		return natsessions.KillDeleted, "NAT44-EI session deleted: " + k.String()
	case natcommon.IsNoSuchEntry(err):
		return natsessions.KillNotFound, "no such NAT44-EI session: " + k.String()
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return natsessions.KillFailed, "cancelled: " + err.Error()
	default:
		return natsessions.KillFailed, "nat44_ei_del_session failed: " + err.Error()
	}
}

// Stats are the flat `done.stats` of an EI kill.
func (k KillEI) Stats() map[string]string {
	st := map[string]string{
		"variant": "ei", "protocol": k.Protocol, "inside_address": k.Inside.Addr().String(), "inside_port": fmt.Sprint(k.Inside.Port()),
		"table_id": fmt.Sprint(k.Table),
	}
	if k.External.IsValid() {
		st["external_address"], st["external_port"] = k.External.Addr().String(), fmt.Sprint(k.External.Port())
	}
	return st
}

// ---- NAT64 ------------------------------------------------------------------------------------------------------

// Nat64Source is the read side of nat64 (nat64.Plugin implements it).
type Nat64Source interface {
	Sessions(ctx context.Context, protocol string, offset, limit int) ([]nat64.SessionEntry, error)
}

// BIBKey identifies a BIB entry by its outside endpoint (what a session row carries intact).
type BIBKey struct {
	Protocol string
	Outside  string
	Port     uint32
}

// BIBSource returns the inside port of every BIB entry (dynamic and static) by its outside endpoint.
type BIBSource interface {
	InsidePorts(ctx context.Context) (map[BIBKey]uint32, error)
}

// fixPorts works around VPP 26.06's nat64_st_details (nat64_api.c nat64_api_st_walk sets il_port twice — first to
// the BIB's in_port, then to the remote port — and never sets r_port). The inside port is always the BIB entry's
// (looked up by the outside endpoint, which the details carry intact); a row whose il_port differs from it carries the
// remote port there, and only such a row is swapped (review L1: a fixed VPP's real remote port 0, e.g. ICMP, stays 0).
// A row whose BIB entry went away between the two walks is left as reported. Left over: on 26.06 a session whose
// remote port equals its inside port shows remote port 0. docs/vpp-code-track.md V-new (b).
func fixPorts(rows []nat64.SessionEntry, bib map[BIBKey]uint32) {
	for i := range rows {
		r := &rows[i]
		in, ok := bib[BIBKey{Protocol: r.Protocol, Outside: r.OutsideLocal, Port: r.OutsidePort}]
		if !ok || r.InsidePort == in {
			continue
		}
		r.RemotePort, r.InsidePort = r.InsidePort, in
	}
}

// Nat64Page is one NAT64 page.
type Nat64Page struct {
	Rows          []nat64.SessionEntry
	Next          *uint32
	TotalUsers    uint64 // distinct IPv6 clients among the counted sessions
	TotalSessions uint64
	Truncated     bool
}

// OwnsNat64 reports whether a NAT64 session belongs to the owner: its IPv6 client or IPv4 pool address is in the
// owner's range, or its VRF is in the owner's table range (a production owner owns all).
func OwnsNat64(scope natcommon.Scope, s nat64.SessionEntry) bool {
	return scope.OwnsAddrString(s.InsideLocal) || scope.OwnsAddrString(s.OutsideLocal) || scope.OwnsTable(s.VRF)
}

// ListNat64 returns one page (offset, limit 1..MaxLimit) of the owner's NAT64 sessions for protocol ("" = all),
// counting at most scanCap owned sessions (0 = natsessions.DefaultScanCap); the page's ports are corrected with the
// BIB (fixPorts, VPP 26.06 defect) when bibs is not nil.
//
// Cost (review M3, a follow-up in the status file): each page is one whole-table walk — src.Sessions (DF-3's
// nat64.Sessions) materialises every owner's rows before the scope filter, the scan cap bounds only the owned rows
// counted — plus, when the page has rows, one nat64_bib_dump of all protocols. Paging repeats both walks. The follow-up
// streams the ST dump keeping only offset+limit rows and the counters, and dumps the BIB only for the page's protocols.
func ListNat64(ctx context.Context, src Nat64Source, bibs BIBSource, scope natcommon.Scope, protocol string, offset, limit, scanCap int) (Nat64Page, error) {
	if limit < 1 || limit > natsessions.MaxLimit {
		return Nat64Page{}, fmt.Errorf("%w: limit %d outside 1–%d", natsessions.ErrInvalid, limit, natsessions.MaxLimit)
	}
	if offset < 0 {
		return Nat64Page{}, fmt.Errorf("%w: offset %d is negative", natsessions.ErrInvalid, offset)
	}
	if scanCap <= 0 {
		scanCap = natsessions.DefaultScanCap
	}
	all, err := src.Sessions(ctx, protocol, 0, 0)
	if err != nil {
		return Nat64Page{}, err
	}
	var p Nat64Page
	users := map[string]bool{}
	matched := 0
	for _, s := range all {
		if !OwnsNat64(scope, s) {
			continue
		}
		if matched >= scanCap {
			p.Truncated = true
			break
		}
		if matched >= offset && len(p.Rows) < limit {
			p.Rows = append(p.Rows, s)
		}
		users[s.InsideLocal] = true
		matched++
	}
	p.TotalSessions, p.TotalUsers = uint64(matched), uint64(len(users)) //nolint:gosec // counts
	if len(p.Rows) > 0 && bibs != nil {
		bib, err := bibs.InsidePorts(ctx)
		if err != nil {
			return Nat64Page{}, err
		}
		fixPorts(p.Rows, bib)
	}
	if end := offset + len(p.Rows); end < matched || (p.Truncated && len(p.Rows) == limit) {
		n := uint32(end) //nolint:gosec // bounded by the session table
		p.Next = &n
	}
	return p, nil
}
