// Package nat44edsessions is the NAT44-ED session browser and kill of F-nat44-ed-sessions, as pure functions over
// DF-3's Retrieve-only helpers (descriptors/nat44ed: Plugin.Users, Plugin.UserSessions, Plugin.DeleteSession). The
// agent's NatSessions / NatSummary RPCs and the NatSessionKillAction case of Action (internal/agent/rpc_nat44_ed.go)
// only translate to and from vrx.v1 (docs/contracts/proto.md §11).
//
// Paging (never the whole table): VPP dumps sessions per user (inside host). List reads the user dump (one small
// row per user, with its session counts), keeps this owner's users, sorts them by (table, address) and — without a
// session-level filter — skips whole users by their counts and dumps only the users that cover the page. A filter on
// the outside / external address, a port or the protocol has to look at sessions: users are dumped one at a time, only
// the page is kept, and the scan stops at a cap (Truncated: the totals are lower bounds).
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

// Limits of one page (proto: limit 0 = 100, more than 1000 is INVALID_ARGUMENT).
const (
	DefaultLimit = 100
	MaxLimit     = 1000
	// DefaultScanCap bounds the sessions a filtered List or a Summary looks at.
	DefaultScanCap = 200_000
)

// ErrInvalid marks a request error (the RPC answers INVALID_ARGUMENT).
var ErrInvalid = errors.New("invalid request")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Source is the read side (nat44ed.Plugin implements it).
type Source interface {
	Users(ctx context.Context) ([]nat44ed.User, error)
	UserSessions(ctx context.Context, user nat44ed.User, offset, limit int) ([]nat44ed.Session, error)
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
	Truncated     bool
}

// ownUsers returns this owner's users that the filter selects, in table order (table id, then address).
func ownUsers(ctx context.Context, src Source, scope natcommon.Scope, f Filter) ([]nat44ed.User, error) {
	all, err := src.Users(ctx)
	if err != nil {
		return nil, err
	}
	var out []nat44ed.User
	for _, u := range all {
		if scope.OwnsAddrString(u.IP) && f.matchUser(u) {
			out = append(out, u)
		}
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

// List returns one page (offset, limit) of the owner's sessions that match f. limit must be 1..MaxLimit; scanCap
// bounds the sessions a filtered scan looks at (0 = DefaultScanCap).
func List(ctx context.Context, src Source, scope natcommon.Scope, f Filter, offset, limit, scanCap int) (Page, error) {
	if limit < 1 || limit > MaxLimit {
		return Page{}, invalid("limit %d outside 1–%d", limit, MaxLimit)
	}
	if offset < 0 {
		return Page{}, invalid("offset %d is negative", offset)
	}
	if scanCap <= 0 {
		scanCap = DefaultScanCap
	}
	users, err := ownUsers(ctx, src, scope, f)
	if err != nil {
		return Page{}, err
	}
	p := Page{TotalUsers: uint64(len(users))}
	if !f.scansSessions() {
		return listByCounts(ctx, src, users, offset, limit, p)
	}
	matched, scanned := 0, 0
	for _, u := range users {
		if scanned >= scanCap {
			p.Truncated = true
			break
		}
		ss, err := src.UserSessions(ctx, u, 0, 0)
		if err != nil {
			return Page{}, err
		}
		scanned += len(ss)
		for _, s := range ss {
			if !f.matchSession(s) {
				continue
			}
			if matched >= offset && len(p.Rows) < limit {
				p.Rows = append(p.Rows, Row{Session: s, VRF: u.VRF})
			}
			matched++
		}
	}
	p.TotalSessions = uint64(matched) //nolint:gosec // a count
	if end := offset + len(p.Rows); end < matched || (p.Truncated && len(p.Rows) == limit) {
		n := uint32(end) //nolint:gosec // bounded by the session table
		p.Next = &n
	}
	return p, nil
}

// listByCounts pages by the users' session counts: whole users before offset are skipped without a dump.
func listByCounts(ctx context.Context, src Source, users []nat44ed.User, offset, limit int, p Page) (Page, error) {
	var total int
	for _, u := range users {
		total += int(u.Sessions + u.StaticSessions)
	}
	p.TotalSessions = uint64(total) //nolint:gosec // a count
	skip := offset
	for _, u := range users {
		if len(p.Rows) >= limit {
			break
		}
		n := int(u.Sessions + u.StaticSessions)
		if skip >= n {
			skip -= n
			continue
		}
		ss, err := src.UserSessions(ctx, u, skip, limit-len(p.Rows))
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
