// Package nat44edsessions is the NAT44-ED session browser and kill of F-nat44-ed-sessions, as pure functions over
// DF-3's Retrieve-only helpers (descriptors/nat44ed: Plugin.Users, Plugin.UserSessions, Plugin.DeleteSession). The
// agent's NatSessions / NatSummary RPCs and the NatSessionKillAction case of Action (internal/agent/rpc_nat44_ed.go)
// only translate to and from vrx.v1 (docs/contracts/proto.md §11).
//
// Paging (never the whole table): VPP dumps sessions per user (inside host). List reads the user dump (one small
// row per user, with its session counts), keeps this owner's users, sorts them by (table, address) and — without a
// session-level filter — skips whole users by their counts and dumps only the users that cover the page. A filter on
// the outside / external address, a port or the protocol has to look at sessions: users are dumped one at a time and
// their sessions streamed, only the page is kept.
//
// Bounded VPP cost (review H1): every nat44_user_session_v3_dump walks the worker's WHOLE session pool under the
// worker barrier (the nat44-ed API handlers are not mp-safe), so one call visits at most Caps.UserDumps users
// (MaxUserDumps for a page, MaxSummaryUserDumps for the summary's breakdown) and looks at most Caps.ScanCap sessions;
// hitting either sets Truncated. The user / session / static totals always come from the single nat44_user_dump.
package nat44edsessions

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
)

// Limits of one call (proto: limit 0 = 100, more than 1000 is INVALID_ARGUMENT).
const (
	DefaultLimit = 100
	MaxLimit     = 1000
	// DefaultScanCap bounds the sessions a filtered List or a Summary looks at.
	DefaultScanCap = 200_000
	// MaxUserDumps bounds the per-user session dumps (each one a walk of the worker's whole session pool under the
	// barrier) of one NatSessions call.
	MaxUserDumps = 256
	// MaxSummaryUserDumps bounds the per-user dumps of one NatSummary breakdown (per pool / per protocol).
	MaxSummaryUserDumps = 64
)

// Caps bound one call's VPP work; zero fields take the defaults (DefaultScanCap, MaxUserDumps).
type Caps struct {
	ScanCap   int // sessions looked at
	UserDumps int // nat44_user_session_v3_dump calls
}

func (c Caps) withDefaults(userDumps int) Caps {
	if c.ScanCap <= 0 {
		c.ScanCap = DefaultScanCap
	}
	if c.UserDumps <= 0 {
		c.UserDumps = userDumps
	}
	return c
}

// ErrInvalid marks a request error (the RPC answers INVALID_ARGUMENT).
var ErrInvalid = errors.New("invalid request")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Source is the read side (nat44ed.Plugin implements it).
type Source interface {
	Users(ctx context.Context) ([]nat44ed.User, error)
	UserSessions(ctx context.Context, user nat44ed.User, offset, limit int) ([]nat44ed.Session, error)
	EachUserSession(ctx context.Context, user nat44ed.User, fn func(nat44ed.Session) bool) error
}

// Filter narrows List; zero fields match everything.
type Filter struct {
	Inside, Outside, External netip.Addr
	Port                      *uint32
	Protocol                  string // canonical name ("tcp", "udp", "icmp") or a number in decimal; "" = any
	VRF                       *uint32
}

// scansSessions reports whether the filter needs to look at sessions (not only at users).
func (f Filter) scansSessions() bool {
	return f.Outside.IsValid() || f.External.IsValid() || f.Port != nil || f.Protocol != ""
}

func (f Filter) matchUser(u nat44ed.User) bool {
	if f.VRF != nil && u.VRF != *f.VRF {
		return false
	}
	return !f.Inside.IsValid() || u.IP == f.Inside.String()
}

func (f Filter) matchSession(s nat44ed.Session) bool {
	switch {
	case f.Outside.IsValid() && s.Outside.IP != f.Outside.String():
		return false
	case f.External.IsValid() && s.ExtHost.IP != f.External.String():
		return false
	case f.Protocol != "" && s.Protocol != f.Protocol:
		return false
	case f.Port != nil && s.Inside.Port != *f.Port && s.Outside.Port != *f.Port && s.ExtHost.Port != *f.Port:
		return false
	}
	return true
}

// ParseProtocol canonicalises a protocol filter / kill protocol ("TCP", "6" → "tcp").
func ParseProtocol(s string) (string, error) {
	n, err := natcommon.ProtoNumber(strings.ToLower(strings.TrimSpace(s)))
	if err != nil || n == 0 {
		return "", invalid("protocol %q is not tcp, udp, icmp or an IP protocol number", s)
	}
	return natcommon.ProtoName(n), nil
}

// ParseIPv4 parses an IPv4 address of a filter or kill field (named for the error).
func ParseIPv4(field, s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil || !a.Is4() {
		return netip.Addr{}, invalid("%s %q is not an IPv4 address", field, s)
	}
	return a, nil
}

// ParseVRF resolves a VRF spelling: "" and "default" are table 0, a configured name through resolve, a decimal
// string is a raw table id (the spellings NatSession.vrf returns).
func ParseVRF(name string, resolve func(string) (uint32, bool)) (uint32, error) {
	switch name {
	case "", "default":
		return 0, nil
	}
	if resolve != nil {
		if id, ok := resolve(name); ok {
			return id, nil
		}
	}
	if id, err := strconv.ParseUint(name, 10, 32); err == nil {
		return uint32(id), nil
	}
	return 0, invalid("VRF %q does not exist", name)
}

// Row is one session of a page with its user's table.
type Row struct {
	nat44ed.Session
	VRF uint32
}

// Page is one List result.
type Page struct {
	Rows          []Row
	Next          *uint32 // offset of the next page; nil on the last page
	TotalUsers    uint64
	TotalSessions uint64
	// Truncated: a cap stopped the call (Caps). With a session-level filter TotalSessions is then a lower bound; either
	// way the page may be short — continue at Next.
	Truncated bool
}

// ownUsers returns this owner's users that the filter selects, in table order (table id, then address). Rows of one
// (VRF, address) are merged and their counts summed: on a multi-worker VPP nat44_user_dump reports a user once per
// worker (review L2).
func ownUsers(ctx context.Context, src Source, scope natcommon.Scope, f Filter) ([]nat44ed.User, error) {
	all, err := src.Users(ctx)
	if err != nil {
		return nil, err
	}
	type key struct {
		vrf uint32
		ip  string
	}
	at := map[key]int{}
	var out []nat44ed.User
	for _, u := range all {
		if !scope.OwnsAddrString(u.IP) || !f.matchUser(u) {
			continue
		}
		k := key{u.VRF, u.IP}
		if i, dup := at[k]; dup {
			out[i].Sessions += u.Sessions
			out[i].StaticSessions += u.StaticSessions
			continue
		}
		at[k] = len(out)
		out = append(out, u)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].VRF != out[b].VRF {
			return out[a].VRF < out[b].VRF
		}
		x, _ := netip.ParseAddr(out[a].IP)
		y, _ := netip.ParseAddr(out[b].IP)
		return x.Less(y)
	})
	return out, nil
}

// List returns one page (offset, limit) of the owner's sessions that match f. limit must be 1..MaxLimit; caps bound
// the VPP work of the call (zero = defaults, MaxUserDumps).
func List(ctx context.Context, src Source, scope natcommon.Scope, f Filter, offset, limit int, caps Caps) (Page, error) {
	if limit < 1 || limit > MaxLimit {
		return Page{}, invalid("limit %d outside 1–%d", limit, MaxLimit)
	}
	if offset < 0 {
		return Page{}, invalid("offset %d is negative", offset)
	}
	caps = caps.withDefaults(MaxUserDumps)
	users, err := ownUsers(ctx, src, scope, f)
	if err != nil {
		return Page{}, err
	}
	p := Page{TotalUsers: uint64(len(users))}
	if !f.scansSessions() {
		return listByCounts(ctx, src, users, offset, limit, caps, p)
	}
	matched, scanned, dumps := 0, 0, 0
	dumped := map[string]bool{} // the dump matches the address only: one dump per address (review L2)
	for _, u := range users {
		if dumped[u.IP] {
			continue
		}
		if scanned >= caps.ScanCap || dumps >= caps.UserDumps {
			p.Truncated = true
			break
		}
		dumped[u.IP] = true
		dumps++
		err := src.EachUserSession(ctx, u, func(s nat44ed.Session) bool {
			scanned++
			if f.matchSession(s) {
				if matched >= offset && len(p.Rows) < limit {
					p.Rows = append(p.Rows, Row{Session: s, VRF: u.VRF})
				}
				matched++
			}
			return scanned < caps.ScanCap
		})
		if err != nil {
			return Page{}, err
		}
	}
	if scanned >= caps.ScanCap {
		p.Truncated = true
	}
	p.TotalSessions = uint64(matched) //nolint:gosec // a count
	if end := offset + len(p.Rows); end < matched || (p.Truncated && len(p.Rows) == limit) {
		n := uint32(end) //nolint:gosec // bounded by the session table
		p.Next = &n
	}
	return p, nil
}

// listByCounts pages by the users' session counts: whole users before offset are skipped without a dump, and at
// most count − skip rows are taken from one user's dump. The dump matches the address only (review L2), so the users
// that share an address (overlapping tenant addresses in several VRFs) partition that address's dump in user order:
// a user starts at the counts of the earlier users of its address — no session is shown twice; which VRF a row
// belongs to cannot be told from the dump (docs/vpp-code-track.md V-new). At most caps.UserDumps users are dumped.
func listByCounts(ctx context.Context, src Source, users []nat44ed.User, offset, limit int, caps Caps, p Page) (Page, error) {
	var total int
	for _, u := range users {
		total += int(u.Sessions + u.StaticSessions)
	}
	p.TotalSessions = uint64(total) //nolint:gosec // a count
	skip, dumps := offset, 0
	before := map[string]int{} // address → sessions of the earlier users of that address
	for _, u := range users {
		n := int(u.Sessions + u.StaticSessions)
		base := before[u.IP]
		before[u.IP] += n
		if len(p.Rows) >= limit {
			break
		}
		if skip >= n {
			skip -= n
			continue
		}
		if dumps >= caps.UserDumps {
			p.Truncated = true
			break
		}
		dumps++
		ss, err := src.UserSessions(ctx, u, base+skip, min(limit-len(p.Rows), n-skip))
		if err != nil {
			return Page{}, err
		}
		skip = 0
		for _, s := range ss {
			p.Rows = append(p.Rows, Row{Session: s, VRF: u.VRF})
		}
	}
	if end := offset + len(p.Rows); end < total {
		n := uint32(end) //nolint:gosec // bounded by the session table
		p.Next = &n
	}
	return p, nil
}
