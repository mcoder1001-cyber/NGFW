package nat44ei6466nptv6

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.fd.io/govpp/api"

	natsessions "ngfw/agent/internal/actions/nat44-ed-sessions"
	"ngfw/agent/internal/descriptors/nat44ei"
	"ngfw/agent/internal/descriptors/nat64"
	"ngfw/agent/internal/descriptors/natcommon"
)

type fakeEI struct {
	users    []nat44ei.User
	sessions map[string][]nat44ei.Session
	dumps    int
	deleted  []string
	delErr   error
}

func (f *fakeEI) Users(context.Context) ([]nat44ei.User, error) { return f.users, nil }

func (f *fakeEI) UserSessions(_ context.Context, u nat44ei.User, offset, limit int) ([]nat44ei.Session, error) {
	f.dumps++
	ss := f.sessions[fmt.Sprintf("%d|%s", u.VRF, u.IP)]
	if offset > len(ss) {
		return nil, nil
	}
	ss = ss[offset:]
	if limit > 0 && len(ss) > limit {
		ss = ss[:limit]
	}
	return ss, nil
}

func (f *fakeEI) DeleteSession(_ context.Context, in nat44ei.Endpoint, proto string, vrf uint32, ext nat44ei.Endpoint) error {
	f.deleted = append(f.deleted, fmt.Sprintf("%s %s:%d %d ext=%s:%d", proto, in.IP, in.Port, vrf, ext.IP, ext.Port))
	return f.delErr
}

func eiFixture() *fakeEI {
	f := &fakeEI{sessions: map[string][]nat44ei.Session{}}
	for u := 0; u < 3; u++ {
		ip := fmt.Sprintf("10.4.1.%d", 10+u)
		f.users = append(f.users, nat44ei.User{IP: ip, Sessions: 5})
		for i := 0; i < 5; i++ {
			f.sessions["0|"+ip] = append(f.sessions["0|"+ip], nat44ei.Session{Inside: nat44ei.Endpoint{IP: ip, Port: uint32(1000 + i)},
				Outside: nat44ei.Endpoint{IP: "10.4.2.100", Port: uint32(2000 + 10*u + i)}, ExtHost: nat44ei.Endpoint{IP: "10.4.2.2", Port: 80}, Protocol: "tcp"})
		}
	}
	f.users = append(f.users, nat44ei.User{IP: "10.9.1.1", Sessions: 7}) // another slot's user
	return f
}

func TestListEIPagesLikeED(t *testing.T) {
	f := eiFixture()
	p, err := ListEI(context.Background(), f, natcommon.ScopeFor("w4"), natsessions.Filter{}, 7, 5, natsessions.Caps{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rows) != 5 || p.TotalSessions != 15 || p.TotalUsers != 3 || p.Next == nil || *p.Next != 12 {
		t.Fatalf("page %+v", p)
	}
	// whole users before the offset are skipped by their counts: users 2 and 3 dumped only
	if f.dumps != 2 || p.Rows[0].Inside.IP != "10.4.1.11" || p.Rows[0].Inside.Port != 1002 {
		t.Fatalf("dumps %d first %+v", f.dumps, p.Rows[0])
	}
	// EI has no twice-NAT: the external host after NAT is 0.0.0.0/0 (as ED reports a session without twice-NAT)
	if r := p.Rows[0]; r.ExtHostNAT.IP != "0.0.0.0" || r.ExtHostNAT.Port != 0 || r.TwiceNAT {
		t.Fatalf("row %+v", r)
	}
}

func TestParseKillEI(t *testing.T) {
	scope := natcommon.ScopeFor("w4")
	k, err := ParseKillEI(scope, "TCP", "10.4.1.10", 1000, "", 0, "", nil)
	if err != nil || k.External.IsValid() || k.Protocol != "tcp" {
		t.Fatalf("%+v %v", k, err)
	}
	for name, c := range map[string][]any{
		"foreign":  {"tcp", "10.9.1.1", uint32(1), "", uint32(0), ""},
		"port":     {"tcp", "10.4.1.10", uint32(70000), "", uint32(0), ""},
		"external": {"tcp", "10.4.1.10", uint32(1), "fd00::1", uint32(1), ""},
		"proto":    {"gre2", "10.4.1.10", uint32(1), "", uint32(0), ""},
		"vrf":      {"tcp", "10.4.1.10", uint32(1), "", uint32(0), "nope"},
	} {
		_, err := ParseKillEI(scope, c[0].(string), c[1].(string), c[2].(uint32), c[3].(string), c[4].(uint32), c[5].(string), nil)
		if !errors.Is(err, natsessions.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	f := eiFixture()
	if code, sum := k.Do(context.Background(), f); code != natsessions.KillDeleted || f.deleted[0] != "tcp 10.4.1.10:1000 0 ext=:0" {
		t.Fatalf("%d %s %v", code, sum, f.deleted)
	}
	f.delErr = api.NO_SUCH_ENTRY
	if code, _ := k.Do(context.Background(), f); code != natsessions.KillNotFound {
		t.Fatalf("not found → %d", code)
	}
	f.delErr = api.UNSUPPORTED
	if code, _ := k.Do(context.Background(), f); code != natsessions.KillFailed {
		t.Fatalf("failure → %d", code)
	}
	if st := k.Stats(); st["variant"] != "ei" || st["inside_port"] != "1000" || st["external_address"] != "" {
		t.Fatalf("stats %v", st)
	}
}

type fake64 struct{ rows []nat64.SessionEntry }

func (f fake64) Sessions(_ context.Context, protocol string, _, _ int) ([]nat64.SessionEntry, error) {
	var out []nat64.SessionEntry
	for _, r := range f.rows {
		if protocol == "" || r.Protocol == protocol {
			out = append(out, r)
		}
	}
	return out, nil
}

func TestListNat64(t *testing.T) {
	var f fake64
	for i := 0; i < 10; i++ {
		f.rows = append(f.rows, nat64.SessionEntry{InsideLocal: fmt.Sprintf("fd00:4::%d", i%4), OutsideLocal: "10.4.64.1", Protocol: "tcp"})
	}
	f.rows = append(f.rows,
		nat64.SessionEntry{InsideLocal: "fd00:9::1", OutsideLocal: "10.9.64.1", Protocol: "tcp", VRF: 9001},      // another slot's
		nat64.SessionEntry{InsideLocal: "2001:db8::1", OutsideLocal: "198.51.100.1", Protocol: "udp", VRF: 4002}) // our table
	scope := natcommon.ScopeFor("w4")
	p, err := ListNat64(context.Background(), f, nil, scope, "", 8, 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rows) != 3 || p.TotalSessions != 11 || p.TotalUsers != 5 || p.Next != nil || p.Truncated {
		t.Fatalf("page %+v", p)
	}
	p, _ = ListNat64(context.Background(), f, nil, scope, "udp", 0, 5, 0)
	if p.TotalSessions != 1 || p.Rows[0].VRF != 4002 {
		t.Fatalf("udp %+v", p)
	}
	p, _ = ListNat64(context.Background(), f, nil, scope, "", 0, 5, 6)
	if !p.Truncated || p.TotalSessions != 6 || len(p.Rows) != 5 || p.Next == nil || *p.Next != 5 {
		t.Fatalf("capped %+v", p)
	}
	if _, err := ListNat64(context.Background(), f, nil, scope, "", 0, 1001, 0); !errors.Is(err, natsessions.ErrInvalid) {
		t.Fatalf("limit: %v", err)
	}
	// the production owner sees every session
	p, _ = ListNat64(context.Background(), f, nil, natcommon.ScopeFor("vrx"), "", 0, 100, 0)
	if p.TotalSessions != 12 {
		t.Fatalf("production owner %+v", p)
	}
}

type fakeBIB map[BIBKey]uint32

func (b fakeBIB) InsidePorts(context.Context) (map[BIBKey]uint32, error) { return b, nil }

// VPP 26.06's nat64_st_details carry the remote port in il_port and no r_port: the page is corrected from the BIB.
func TestListNat64PortWorkaround(t *testing.T) {
	f := fake64{rows: []nat64.SessionEntry{
		{InsideLocal: "fd00:4:1::2", InsidePort: 8000, OutsideLocal: "10.4.64.1", OutsidePort: 11570, OutsideRemote: "10.4.2.2", Protocol: "tcp"},                 // as VPP 26.06 sends it
		{InsideLocal: "fd00:4:1::3", InsidePort: 50000, OutsideLocal: "10.4.64.1", OutsidePort: 1024, OutsideRemote: "10.4.2.2", RemotePort: 53, Protocol: "udp"}, // a fixed VPP
		{InsideLocal: "fd00:4:1::4", InsidePort: 7, OutsideLocal: "10.4.64.1", OutsidePort: 7, OutsideRemote: "10.4.2.2", Protocol: "icmp"},                       // a fixed VPP, remote port 0 (L1)
		{InsideLocal: "fd00:4:1::5", InsidePort: 8000, OutsideLocal: "10.4.64.1", OutsidePort: 11571, OutsideRemote: "10.4.2.2", Protocol: "tcp"},                 // its BIB entry went away between the walks
	}}
	bib := fakeBIB{{Protocol: "tcp", Outside: "10.4.64.1", Port: 11570}: 46001, {Protocol: "udp", Outside: "10.4.64.1", Port: 1024}: 50000, {Protocol: "icmp", Outside: "10.4.64.1", Port: 7}: 7}
	p, err := ListNat64(context.Background(), f, bib, natcommon.ScopeFor("w4"), "", 0, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r := p.Rows[0]; r.InsidePort != 46001 || r.RemotePort != 8000 {
		t.Fatalf("defective row not corrected: %+v", r)
	}
	if r := p.Rows[1]; r.InsidePort != 50000 || r.RemotePort != 53 {
		t.Fatalf("a complete row was changed: %+v", r)
	}
	if r := p.Rows[2]; r.InsidePort != 7 || r.RemotePort != 0 {
		t.Fatalf("a real remote port 0 was replaced: %+v", r)
	}
	if r := p.Rows[3]; r.InsidePort != 8000 || r.RemotePort != 0 {
		t.Fatalf("a row without BIB entry was changed: %+v", r)
	}
}
