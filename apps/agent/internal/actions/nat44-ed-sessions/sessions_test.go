package nat44edsessions_test

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"testing"

	natsessions "ngfw/agent/internal/actions/nat44-ed-sessions"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
)

var ctx = context.Background()

// seeded returns a fake whose nat44-ed plugin holds `users` inside hosts of slot 4 with `per` sessions each (tcp to
// 10.4.2.2:<port>), plus two sessions of another slot's user (never visible to w4).
func seeded(t *testing.T, users, per int) (*coretest.VPP, *nat44ed.Plugin) {
	t.Helper()
	v := coretest.New()
	n := v.Nat44ED()
	n.NatEnable()
	for u := 0; u < users; u++ {
		h := 256 + 10 + u // 10.4.1.10, 10.4.1.11, … (continuing into 10.4.2.x past 245 hosts)
		in := fmt.Sprintf("10.4.%d.%d", h/256, h%256)
		for s := 0; s < per; s++ {
			n.AddNatSession(0, 6, in, uint16(10000+s), "10.4.2.100", uint16(20000+u*per+s), "10.4.2.2", uint16(80+s%3)) //nolint:gosec // test data
		}
	}
	n.AddNatSession(0, 17, "10.9.1.5", 5000, "10.9.2.100", 6000, "10.9.2.2", 53)
	n.AddNatSession(0, 17, "10.9.1.5", 5001, "10.9.2.100", 6001, "10.9.2.2", 53)
	return v, nat44ed.New(v, "w4")
}

func TestListPagesByUserCounts(t *testing.T) {
	v, p := seeded(t, 25, 100) // 2 500 sessions of w4
	n := v.Nat44ED()
	scope := natcommon.ScopeFor("w4")

	n.Lock()
	n.SessionDumps = 0
	n.Unlock()
	page, err := natsessions.List(ctx, p, scope, natsessions.Filter{}, 0, 100, natsessions.Caps{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != 100 || page.TotalSessions != 2500 || page.TotalUsers != 25 || page.Next == nil || *page.Next != 100 || page.Truncated {
		t.Fatalf("first page: rows %d total %d users %d next %v truncated %v", len(page.Rows), page.TotalSessions, page.TotalUsers, page.Next, page.Truncated)
	}
	if page.Rows[0].Inside.IP != "10.4.1.10" || page.Rows[99].Inside.IP != "10.4.1.10" {
		t.Fatalf("users are not in (table, address) order: %v … %v", page.Rows[0].Inside, page.Rows[99].Inside)
	}
	n.Lock()
	dumps := n.SessionDumps
	n.Unlock()
	if dumps != 1 {
		t.Fatalf("the first page dumped %d users, want 1 (whole users are paged by their counts)", dumps)
	}

	// a page in the middle spans two users and dumps only those two
	n.Lock()
	n.SessionDumps = 0
	n.Unlock()
	page, err = natsessions.List(ctx, p, scope, natsessions.Filter{}, 1250, 100, natsessions.Caps{})
	if err != nil {
		t.Fatal(err)
	}
	n.Lock()
	dumps = n.SessionDumps
	n.Unlock()
	if len(page.Rows) != 100 || dumps != 2 || page.Rows[0].Inside.IP != "10.4.1.22" || page.Rows[49].Inside.IP != "10.4.1.22" || page.Rows[50].Inside.IP != "10.4.1.23" {
		t.Fatalf("middle page: rows %d dumps %d first %v", len(page.Rows), dumps, page.Rows[0].Inside)
	}

	// every page is bounded, the pages cover the table exactly once, the last one has no next
	seen := map[string]bool{}
	for off := 0; ; {
		pg, err := natsessions.List(ctx, p, scope, natsessions.Filter{}, off, 100, natsessions.Caps{})
		if err != nil {
			t.Fatal(err)
		}
		if len(pg.Rows) > 100 {
			t.Fatalf("page at %d has %d rows", off, len(pg.Rows))
		}
		for _, r := range pg.Rows {
			k := fmt.Sprintf("%s:%d", r.Inside.IP, r.Inside.Port)
			if seen[k] {
				t.Fatalf("session %s on two pages", k)
			}
			seen[k] = true
		}
		if pg.Next == nil {
			break
		}
		off = int(*pg.Next)
	}
	if len(seen) != 2500 {
		t.Fatalf("pages covered %d sessions", len(seen))
	}

	// limits
	for _, l := range []int{0, natsessions.MaxLimit + 1} {
		if _, err := natsessions.List(ctx, p, scope, natsessions.Filter{}, 0, l, natsessions.Caps{}); !errors.Is(err, natsessions.ErrInvalid) {
			t.Errorf("limit %d: %v", l, err)
		}
	}
	if pg, _ := natsessions.List(ctx, p, scope, natsessions.Filter{}, 2500, 100, natsessions.Caps{}); len(pg.Rows) != 0 || pg.Next != nil {
		t.Errorf("past the end: %d rows next %v", len(pg.Rows), pg.Next)
	}
}

func TestListOwnership(t *testing.T) {
	_, p := seeded(t, 2, 3)
	w9, err := natsessions.List(ctx, p, natcommon.ScopeFor("w9"), natsessions.Filter{}, 0, 100, natsessions.Caps{})
	if err != nil || w9.TotalSessions != 2 || len(w9.Rows) != 2 || w9.Rows[0].Inside.IP != "10.9.1.5" {
		t.Fatalf("w9 view %+v %v", w9, err)
	}
	all, err := natsessions.List(ctx, p, natcommon.ScopeFor("vrx"), natsessions.Filter{}, 0, 100, natsessions.Caps{})
	if err != nil || all.TotalSessions != 8 || all.TotalUsers != 3 {
		t.Fatalf("product view %+v %v", all, err)
	}
}

func TestListFilters(t *testing.T) {
	v, p := seeded(t, 5, 30) // 150 sessions of w4
	n := v.Nat44ED()
	n.AddNatSession(4001, 17, "10.4.1.10", 7000, "10.4.2.101", 7100, "10.4.2.3", 53)
	scope := natcommon.ScopeFor("w4")
	port := uint32(81)
	vrf := uint32(4001)
	for name, c := range map[string]struct {
		f     natsessions.Filter
		total uint64
		users uint64
	}{
		"inside":   {natsessions.Filter{Inside: netip.MustParseAddr("10.4.1.12")}, 30, 1},
		"vrf":      {natsessions.Filter{VRF: &vrf}, 1, 1},
		"outside":  {natsessions.Filter{Outside: netip.MustParseAddr("10.4.2.101")}, 1, 6},
		"external": {natsessions.Filter{External: netip.MustParseAddr("10.4.2.2")}, 150, 6},
		"port":     {natsessions.Filter{Port: &port}, 50, 6},
		"protocol": {natsessions.Filter{Protocol: "udp"}, 1, 6},
		"and":      {natsessions.Filter{Inside: netip.MustParseAddr("10.4.1.10"), Port: &port, Protocol: "tcp"}, 10, 2},
	} {
		pg, err := natsessions.List(ctx, p, scope, c.f, 0, 7, natsessions.Caps{})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if pg.TotalSessions != c.total || pg.TotalUsers != c.users || len(pg.Rows) > 7 {
			t.Errorf("%s: total %d users %d rows %d, want %d/%d", name, pg.TotalSessions, pg.TotalUsers, len(pg.Rows), c.total, c.users)
		}
		wantNext := c.total > 7
		if (pg.Next != nil) != wantNext {
			t.Errorf("%s: next %v", name, pg.Next)
		}
	}
	// the filtered scan is capped: totals become lower bounds
	pg, err := natsessions.List(ctx, p, scope, natsessions.Filter{Protocol: "tcp"}, 0, 10, natsessions.Caps{ScanCap: 40})
	if err != nil || !pg.Truncated || pg.TotalSessions >= 150 || len(pg.Rows) != 10 || pg.Next == nil {
		t.Fatalf("capped scan %+v %v", pg, err)
	}
}

func TestParse(t *testing.T) {
	for in, want := range map[string]string{"tcp": "tcp", "UDP": "udp", "1": "icmp", "47": "47"} {
		if got, err := natsessions.ParseProtocol(in); err != nil || got != want {
			t.Errorf("protocol %q → %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "any", "0", "gre"} {
		if _, err := natsessions.ParseProtocol(bad); !errors.Is(err, natsessions.ErrInvalid) {
			t.Errorf("protocol %q accepted", bad)
		}
	}
	resolve := func(n string) (uint32, bool) { return map[string]uint32{"cust": 4001}[n], n == "cust" }
	for in, want := range map[string]uint32{"": 0, "default": 0, "cust": 4001, "4002": 4002} {
		if got, err := natsessions.ParseVRF(in, resolve); err != nil || got != want {
			t.Errorf("vrf %q → %d %v", in, got, err)
		}
	}
	if _, err := natsessions.ParseVRF("nope", resolve); !errors.Is(err, natsessions.ErrInvalid) {
		t.Error("unknown VRF accepted")
	}
	if _, err := natsessions.ParseIPv4("x", "2001:db8::1"); !errors.Is(err, natsessions.ErrInvalid) {
		t.Error("IPv6 accepted")
	}
}

func TestKill(t *testing.T) {
	v, p := seeded(t, 2, 3)
	n := v.Nat44ED()
	scope := natcommon.ScopeFor("w4")
	k, err := natsessions.ParseKill(scope, "tcp", "10.4.1.10", 10001, "10.4.2.2", 81, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	before := n.SessionCount()
	if code, sum := k.Do(ctx, p); code != natsessions.KillDeleted || n.SessionCount() != before-1 {
		t.Fatalf("kill: %d %s (sessions %d → %d)", code, sum, before, n.SessionCount())
	}
	if code, sum := k.Do(ctx, p); code != natsessions.KillNotFound {
		t.Fatalf("second kill: %d %s", code, sum)
	}
	if st := k.Stats(); st["inside_address"] != "10.4.1.10" || st["external_port"] != "81" || st["protocol"] != "tcp" || st["table_id"] != "0" {
		t.Fatalf("stats %v", st)
	}
	// the request that reached VPP: NAT_IS_INSIDE, the full ED 5-tuple
	calls := v.CallsNamed("nat44_del_session")
	if len(calls) != 2 {
		t.Fatalf("%d nat44_del_session calls", len(calls))
	}
	// validation: foreign inside address, bad fields
	for name, c := range map[string][]any{
		"foreign":  {"tcp", "10.9.1.5", uint32(5000), "10.9.2.2", uint32(53), ""},
		"proto":    {"sctp", "10.4.1.10", uint32(1), "10.4.2.2", uint32(1), ""},
		"inside":   {"tcp", "bogus", uint32(1), "10.4.2.2", uint32(1), ""},
		"external": {"tcp", "10.4.1.10", uint32(1), "", uint32(1), ""},
		"port":     {"tcp", "10.4.1.10", uint32(70000), "10.4.2.2", uint32(1), ""},
		"vrf":      {"tcp", "10.4.1.10", uint32(1), "10.4.2.2", uint32(1), "nope"},
	} {
		_, err := natsessions.ParseKill(scope, c[0].(string), c[1].(string), c[2].(uint32), c[3].(string), c[4].(uint32), c[5].(string), nil)
		if !errors.Is(err, natsessions.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// the plugin disabled: VPP answers UNSUPPORTED → a failed kill (exit 2), not "not found"
	n.Lock()
	n.Enabled = false
	n.Unlock()
	if code, _ := k.Do(ctx, p); code != natsessions.KillFailed {
		t.Fatalf("kill on a disabled plugin: %d", code)
	}
}

func TestSummarize(t *testing.T) {
	v, p := seeded(t, 3, 4)
	n := v.Nat44ED()
	n.AddNatSession(0, 17, "10.4.1.10", 7000, "10.4.2.101", 7100, "10.4.2.3", 53)
	n.AddNatSession(0, 1, "10.4.1.11", 7, "10.4.2.1", 7, "10.4.2.3", 7)
	pools := []natsessions.Pool{
		{First: netip.MustParseAddr("10.4.2.100"), Last: netip.MustParseAddr("10.4.2.103"), VRF: "default"},
		{Interface: "host-w4w0", Addrs: []netip.Addr{netip.MustParseAddr("10.4.2.1")}, First: netip.MustParseAddr("10.4.2.1"), Last: netip.MustParseAddr("10.4.2.1")},
	}
	sum, err := natsessions.Summarize(ctx, p, natcommon.ScopeFor("w4"), pools, natsessions.Caps{})
	if err != nil {
		t.Fatal(err)
	}
	if sum.TotalUsers != 3 || sum.TotalSessions != 14 || sum.PoolSessions[0] != 13 || sum.PoolSessions[1] != 1 ||
		sum.ByProtocol["tcp"] != 12 || sum.ByProtocol["udp"] != 1 || sum.ByProtocol["icmp"] != 1 || sum.Truncated {
		t.Fatalf("summary %+v", sum)
	}
	if pools[0].Size() != 4 || pools[1].Size() != 1 {
		t.Fatalf("sizes %d %d", pools[0].Size(), pools[1].Size())
	}
	if enabled, limit, err := natsessions.RunningConfig(ctx, v); err != nil || !enabled || limit != 63*1024 {
		t.Fatalf("running config %v %d %v", enabled, limit, err)
	}
}

// sessionDumps returns how many nat44_user_session_v3_dump the model answered since the last reset.
func sessionDumps(v *coretest.VPP, reset bool) int {
	n := v.Nat44ED()
	n.Lock()
	defer n.Unlock()
	d := n.SessionDumps
	if reset {
		n.SessionDumps = 0
	}
	return d
}

// Review H1: every per-user dump walks VPP's whole session pool under the barrier, so one call visits a bounded
// number of users whatever the table size; the totals stay exact (they come from the one nat44_user_dump).
func TestUserDumpCaps(t *testing.T) {
	v, p := seeded(t, 600, 1) // 600 inside hosts with one session each
	scope := natcommon.ScopeFor("w4")
	sessionDumps(v, true)

	// a filtered scan: at most MaxUserDumps users, truncated, the page still bounded
	pg, err := natsessions.List(ctx, p, scope, natsessions.Filter{Protocol: "tcp"}, 0, 100, natsessions.Caps{})
	if err != nil {
		t.Fatal(err)
	}
	if d := sessionDumps(v, true); d != natsessions.MaxUserDumps || !pg.Truncated || pg.TotalUsers != 600 || len(pg.Rows) != 100 || pg.TotalSessions != natsessions.MaxUserDumps {
		t.Fatalf("filtered: %d dumps (cap %d), truncated %v, users %d, rows %d, matched %d", d, natsessions.MaxUserDumps, pg.Truncated, pg.TotalUsers, len(pg.Rows), pg.TotalSessions)
	}
	// a 1000-row page over one-session hosts: at most MaxUserDumps dumps, a short page that says so and continues
	pg, err = natsessions.List(ctx, p, scope, natsessions.Filter{}, 0, 1000, natsessions.Caps{})
	if err != nil {
		t.Fatal(err)
	}
	if d := sessionDumps(v, true); d != natsessions.MaxUserDumps || !pg.Truncated || len(pg.Rows) != natsessions.MaxUserDumps ||
		pg.Next == nil || *pg.Next != natsessions.MaxUserDumps || pg.TotalSessions != 600 {
		t.Fatalf("unfiltered: %d dumps, truncated %v, rows %d, next %v, total %d", d, pg.Truncated, len(pg.Rows), pg.Next, pg.TotalSessions)
	}
	// a 100-row page never needs the cap
	pg, err = natsessions.List(ctx, p, scope, natsessions.Filter{}, 0, 100, natsessions.Caps{})
	if err != nil || sessionDumps(v, true) != 100 || pg.Truncated || len(pg.Rows) != 100 {
		t.Fatalf("100-row page: %v truncated %v rows %d", err, pg.Truncated, len(pg.Rows))
	}
	// the summary breakdown: at most MaxSummaryUserDumps users; totals exact
	sum, err := natsessions.Summarize(ctx, p, scope, nil, natsessions.Caps{})
	if err != nil {
		t.Fatal(err)
	}
	if d := sessionDumps(v, true); d != natsessions.MaxSummaryUserDumps || !sum.Truncated || sum.TotalSessions != 600 || sum.TotalUsers != 600 || sum.ByProtocol["tcp"] != natsessions.MaxSummaryUserDumps {
		t.Fatalf("summary: %d dumps (cap %d), truncated %v, sessions %d, users %d, tcp %d", d, natsessions.MaxSummaryUserDumps, sum.Truncated, sum.TotalSessions, sum.TotalUsers, sum.ByProtocol["tcp"])
	}
	// the session scan cap stops inside one user's stream (review L4: never more than the cap is looked at)
	v2, p2 := seeded(t, 1, 500)
	sum, err = natsessions.Summarize(ctx, p2, scope, nil, natsessions.Caps{ScanCap: 100})
	if err != nil || !sum.Truncated || sum.ByProtocol["tcp"] != 100 || sum.TotalSessions != 500 || sessionDumps(v2, true) != 1 {
		t.Fatalf("scan cap: %v %+v", err, sum)
	}
}

// fakeSource is a Source whose user dump is scripted (multi-worker duplicates).
type fakeSource struct {
	users []nat44ed.User
	*nat44ed.Plugin
}

func (f fakeSource) Users(context.Context) ([]nat44ed.User, error) { return f.users, nil }

// Review L2: a multi-worker VPP reports a user once per worker — the rows are merged and their counts summed; the
// same inside address in two VRFs (one address-only dump) is partitioned between its users, never shown twice.
func TestUsersMergedAndSharedAddress(t *testing.T) {
	v := coretest.New()
	n := v.Nat44ED()
	n.NatEnable()
	for i := 0; i < 3; i++ {
		n.AddNatSession(0, 6, "10.4.1.10", uint16(1000+i), "10.4.2.100", uint16(2000+i), "10.4.2.2", 80) //nolint:gosec // test data
	}
	n.AddNatSession(4001, 17, "10.4.1.10", 53, "10.4.2.101", 3000, "10.4.2.2", 53)
	p := nat44ed.New(v, "w4")
	scope := natcommon.ScopeFor("w4")

	src := fakeSource{Plugin: p, users: []nat44ed.User{
		{IP: "10.4.1.10", VRF: 0, Sessions: 2}, {IP: "10.4.1.10", VRF: 0, Sessions: 1}, // one user, two workers
		{IP: "10.4.1.10", VRF: 4001, Sessions: 1},
	}}
	pg, err := natsessions.List(ctx, src, scope, natsessions.Filter{}, 0, 100, natsessions.Caps{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, r := range pg.Rows {
		k := fmt.Sprintf("%s:%d/%s", r.Inside.IP, r.Inside.Port, r.Protocol)
		if _, dup := seen[k]; dup {
			t.Fatalf("session %s shown twice: %+v", k, pg.Rows)
		}
		seen[k] = fmt.Sprint(r.VRF)
	}
	if pg.TotalUsers != 2 || pg.TotalSessions != 4 || len(pg.Rows) != 4 || seen["10.4.1.10:53/udp"] != "4001" {
		t.Fatalf("users %d total %d rows %d labels %v", pg.TotalUsers, pg.TotalSessions, len(pg.Rows), seen)
	}
	// the filtered scan and the summary dump each address once
	sessionDumps(v, true)
	fp, err := natsessions.List(ctx, src, scope, natsessions.Filter{Protocol: "tcp"}, 0, 100, natsessions.Caps{})
	if err != nil || fp.TotalSessions != 3 || sessionDumps(v, true) != 1 {
		t.Fatalf("filtered: %v total %d", err, fp.TotalSessions)
	}
	sum, err := natsessions.Summarize(ctx, src, scope, nil, natsessions.Caps{})
	if err != nil || sum.TotalSessions != 4 || sum.ByProtocol["tcp"] != 3 || sum.ByProtocol["udp"] != 1 || sessionDumps(v, true) != 1 {
		t.Fatalf("summary: %v %+v", err, sum)
	}
}

// Review M1: nat44_ed_del_session looks up the session's i2o flow, whose remote end is the NAT'd external host of a
// twice-NAT session — the kill must carry external_nat_*, and the session list reports it (0.0.0.0 without twice-NAT).
func TestKillTwiceNatSession(t *testing.T) {
	v := coretest.New()
	n := v.Nat44ED()
	n.NatEnable()
	n.AddTwiceNatSession(0, 6, "10.4.1.20", 5000, "10.4.2.100", 6000, "10.4.2.2", 80, "10.4.2.120", 1024)
	n.AddNatSession(0, 6, "10.4.1.21", 5001, "10.4.2.100", 6001, "10.4.2.2", 80)
	p := nat44ed.New(v, "w4")
	scope := natcommon.ScopeFor("w4")
	pg, err := natsessions.List(ctx, p, scope, natsessions.Filter{}, 0, 10, natsessions.Caps{})
	if err != nil || len(pg.Rows) != 2 {
		t.Fatal(err, pg)
	}
	tw, plain := pg.Rows[0], pg.Rows[1]
	if !tw.TwiceNAT || tw.ExtHostNAT.IP != "10.4.2.120" || tw.ExtHostNAT.Port != 1024 || plain.TwiceNAT || plain.ExtHostNAT.IP != "0.0.0.0" || plain.ExtHostNAT.Port != 0 {
		t.Fatalf("rows %+v %+v", tw, plain)
	}
	// the external host as the outside sees it: no such session in VPP's flow hash
	k, err := natsessions.ParseKill(scope, "tcp", "10.4.1.20", 5000, "10.4.2.2", 80, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if code, sum := k.Do(ctx, p); code != natsessions.KillNotFound {
		t.Fatalf("kill by the untranslated external end: %d %s", code, sum)
	}
	// the NAT'd external end (what the UI sends for a twice-NAT row): deleted
	k, err = natsessions.ParseKill(scope, "tcp", "10.4.1.20", 5000, "10.4.2.120", 1024, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if code, sum := k.Do(ctx, p); code != natsessions.KillDeleted || n.SessionCount() != 1 {
		t.Fatalf("kill by the NAT'd external end: %d %s (sessions %d)", code, sum, n.SessionCount())
	}
}
