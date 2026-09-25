package dns

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/dns"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// F-unbound-chrony-syslog, VPP 26.06 crash (2026-09-25 04:27, D-137, docs/vpp-code-track.md). The fake models the
// defect as the source reads (dns.c:576-624): every request starts with the IPv4 family and dereferences
// `ip4_name_servers + rotor`, which is NULL until an IPv4 server was added since VPP started — so a resolve (API) or an
// IPv4 UDP-53 request while enabled crashes when no IPv4 server was EVER added; a vector that deletes emptied is still
// allocated (no crash, but a query to a deleted server: "stale"). IPv6 servers are kept apart and never help.

type crashModel struct {
	boot    int // VPP instance; a restart increments it and wipes everything
	enabled bool
	v4Ever  bool
	v4, v6  map[netip.Addr]bool
	crashes []string
	stale   []string
	msgs    int
}

func (m *crashModel) restart() {
	m.boot++
	m.enabled, m.v4Ever = false, false
	m.v4, m.v6 = map[netip.Addr]bool{}, map[netip.Addr]bool{}
}

func (m *crashModel) identity(context.Context, vpp.Client) (string, error) {
	return fmt.Sprintf("boot-%d", m.boot), nil
}

func newCrashFake(t *testing.T) (*dfkittest.FakeVPP, *crashModel) {
	t.Helper()
	f := dfkittest.NewFake()
	m := &crashModel{}
	m.restart()
	check := func(what string) {
		m.msgs++
		if m.enabled && !m.v4Ever {
			m.crashes = append(m.crashes, fmt.Sprintf("after message %d (%s): enabled without an IPv4 server ever added (NULL vector)", m.msgs, what))
		} else if m.enabled && len(m.v4) == 0 {
			m.stale = append(m.stale, fmt.Sprintf("after message %d (%s): enabled with only deleted IPv4 servers", m.msgs, what))
		}
	}
	f.On("dns_enable_disable", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dns.DNSEnableDisable)
		if r.Enable == 1 && len(m.v4) == 0 && len(m.v6) == 0 {
			return []api.Message{&dns.DNSEnableDisableReply{Retval: int32(api.NO_NAME_SERVERS)}}, nil
		}
		m.enabled = r.Enable == 1
		check(fmt.Sprintf("enable=%d", r.Enable))
		return []api.Message{&dns.DNSEnableDisableReply{}}, nil
	})
	f.On("dns_name_server_add_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dns.DNSNameServerAddDel)
		set := m.v4
		a := netip.AddrFrom4([4]byte(r.ServerAddress[:4]))
		if r.IsIP6 == 1 {
			set, a = m.v6, netip.AddrFrom16([16]byte(r.ServerAddress))
		}
		if r.IsAdd == 1 {
			set[a] = true
			if r.IsIP6 == 0 {
				m.v4Ever = true
			}
		} else {
			if !set[a] {
				return []api.Message{&dns.DNSNameServerAddDelReply{Retval: int32(api.NAME_SERVER_NOT_FOUND)}}, nil
			}
			delete(set, a)
		}
		check(fmt.Sprintf("name server %s add=%d", a, r.IsAdd))
		return []api.Message{&dns.DNSNameServerAddDelReply{}}, nil
	})
	resolve := func() {
		if !m.v4Ever {
			m.crashes = append(m.crashes, "dns_resolve_* without an IPv4 server ever added")
		}
	}
	f.On("dns_resolve_name", func(api.Message) ([]api.Message, error) {
		resolve()
		return []api.Message{&dns.DNSResolveNameReply{IP4Set: 1, IP4Address: []byte{192, 0, 2, 1}, IP6Address: make([]byte, 16)}}, nil
	})
	f.On("dns_resolve_ip", func(api.Message) ([]api.Message, error) {
		resolve()
		return []api.Message{&dns.DNSResolveIPReply{Name: make([]byte, 256)}}, nil
	})
	return f, m
}

// desiredCache is what the agent projects for vppCache {enabled, upstreams}.
func desiredCache(ups ...string) []scheduler.KV {
	var kvs []scheduler.KV
	for _, u := range ups {
		v := NameServer{Address: u}.Proto()
		kvs = append(kvs, scheduler.KV{Key: (*NameServerDescriptor)(nil).KeyOf(v), Value: v})
	}
	if len(ups) > 0 {
		kvs = append(kvs, scheduler.KV{Key: KeyEnable, Value: Enable{Enabled: true, Upstreams: ups}.Proto()})
	}
	return kvs
}

// owner wires the globals-owner descriptors with the model's identity (the product's RegisterGlobalsReady).
func owner(f *dfkittest.FakeVPP, m *crashModel) (*scheduler.Scheduler, *Readiness) {
	reg := scheduler.NewRegistry()
	ready := NewReadiness(f).WithIdentity(m.identity)
	g := dfkit.GlobalsOwner(true)
	reg.Register(NewNameServer(f, g).WithReadiness(ready))
	reg.Register(NewEnable(f, g).WithReadiness(ready))
	return scheduler.New(reg, nil), ready
}

func TestUpstreamChangesNeverLeaveAnEnabledResolverWithoutServers(t *testing.T) {
	f, m := newCrashFake(t)
	s, ready := owner(f, m)
	ctx := context.Background()
	for i, ups := range [][]string{
		{"10.5.0.53"},
		{"10.5.0.54"}, // replace the only server: the scheduler deletes 10.5.0.53 before it creates 10.5.0.54
		{"10.5.0.53", "10.5.0.54"},
		{"10.5.0.54", "2001:db8::53"},
		{"10.5.0.55"},
		nil, // cache off
	} {
		res := s.ApplyWith(ctx, desiredCache(ups...), scheduler.All, scheduler.ApplyOptions{})
		if res.Outcome != scheduler.OutcomeApplied {
			for _, r := range res.Results {
				t.Logf("  %s %s err=%v", r.Op, r.Key, r.Err)
			}
			t.Fatalf("step %d %v: %v %v", i, ups, res.Outcome, res.Err)
		}
		if want := len(ups) > 0; m.enabled != want || ready.Ready(ctx) != want {
			t.Fatalf("step %d %v: enabled=%v ready=%v", i, ups, m.enabled, ready.Ready(ctx))
		}
	}
	if len(m.crashes) != 0 || len(m.stale) != 0 {
		t.Fatalf("VPP would have crashed %v / queried a deleted server %v", m.crashes, m.stale)
	}
}

// H1 (review): a cache with only IPv6 upstreams is refused by the descriptor itself — the switch is never enabled, so
// no request can reach the NULL IPv4 vector. The old code enabled it (the model records the crash window).
func TestIPv6OnlyUpstreamsAreNeverEnabled(t *testing.T) {
	f, m := newCrashFake(t)
	s, ready := owner(f, m)
	ctx := context.Background()
	res := s.ApplyWith(ctx, desiredCache("2001:db8::53"), scheduler.All, scheduler.ApplyOptions{})
	if res.Outcome == scheduler.OutcomeApplied || !errors.Is(res.Err, ErrNoIPv4Upstream) {
		t.Fatalf("IPv6-only cache applied: %v %v", res.Outcome, res.Err)
	}
	for _, c := range f.CallsNamed("dns_enable_disable") {
		if c.(*dns.DNSEnableDisable).Enable == 1 {
			t.Fatal("dns_enable_disable(1) sent with only IPv6 servers")
		}
	}
	if m.enabled || len(m.crashes) != 0 || ready.Ready(ctx) {
		t.Fatalf("enabled=%v crashes=%v ready=%v", m.enabled, m.crashes, ready.Ready(ctx))
	}
	// the old shape (no readiness, no IPv4 check) did enable it: the window the model sees
	f2, m2 := newCrashFake(t)
	ns := NewNameServer(f2, dfkit.GlobalsOwner(true))
	if _, err := ns.Create(ctx, NameServer{Address: "2001:db8::53"}.Proto()); err != nil {
		t.Fatal(err)
	}
	if _, err := dns.NewServiceClient(f2).DNSEnableDisable(ctx, &dns.DNSEnableDisable{Enable: 1}); err != nil {
		t.Fatal(err)
	}
	if len(m2.crashes) != 1 {
		t.Fatalf("the model missed the IPv6-only window: %v", m2.crashes)
	}
}

// H2 (review): readiness is a live fact of the running VPP instance. After a simulated VPP restart the stored desired
// state still says "enabled", but nothing is ready until the resync applied the servers and the switch again.
func TestReadinessDoesNotSurviveAVPPRestart(t *testing.T) {
	f, m := newCrashFake(t)
	s, ready := owner(f, m)
	ctx := context.Background()
	desired := desiredCache("10.5.0.53")
	if res := s.ApplyWith(ctx, desired, scheduler.All, scheduler.ApplyOptions{}); res.Outcome != scheduler.OutcomeApplied {
		t.Fatal(res.Err)
	}
	if !ready.Ready(ctx) {
		t.Fatal("not ready after the apply")
	}
	m.restart() // VPP crashed / restarted: empty resolver, another instance
	sent := len(f.Calls())
	if ready.Ready(ctx) {
		t.Fatal("ready on a restarted VPP before the resync")
	}
	if _, _, err := ResolveName(ctx, f, "gw.lab.example", Ready(ready.Ready(ctx))); !errors.Is(err, ErrResolverNotReady) {
		t.Fatalf("lookup after the restart: %v", err)
	}
	if len(f.Calls()) != sent || len(m.crashes) != 0 {
		t.Fatalf("messages sent after the restart: %d (crashes %v)", len(f.Calls())-sent, m.crashes)
	}
	// the agent's resync re-applies the write-only objects on the new instance → ready again
	if res := s.ApplyWith(ctx, desired, scheduler.All, scheduler.ApplyOptions{Resync: true}); res.Outcome != scheduler.OutcomeApplied {
		t.Fatal(res.Err)
	}
	if !ready.Ready(ctx) {
		t.Fatal("not ready after the resync")
	}
	if _, _, err := ResolveName(ctx, f, "gw.lab.example", Ready(ready.Ready(ctx))); err != nil || len(m.crashes) != 0 {
		t.Fatalf("lookup after the resync: %v %v", err, m.crashes)
	}
	// an unidentifiable instance is never ready
	ready.WithIdentity(func(context.Context, vpp.Client) (string, error) { return "", ErrIdentityUnknown })
	if ready.Ready(ctx) {
		t.Fatal("ready without a VPP identity")
	}
}

// D-137: the helpers refuse to send dns_resolve_* unless the caller states that it set the resolver up.
func TestResolveHelpersRefuseWithoutReady(t *testing.T) {
	f, m := newCrashFake(t)
	if _, _, err := ResolveName(context.Background(), f, "gw.lab.example", false); !errors.Is(err, ErrResolverNotReady) {
		t.Fatalf("ResolveName without Ready: %v", err)
	}
	if _, err := ResolveIP(context.Background(), f, netip.MustParseAddr("192.0.2.1"), false); !errors.Is(err, ErrResolverNotReady) {
		t.Fatalf("ResolveIP without Ready: %v", err)
	}
	if len(f.Calls()) != 0 || len(m.crashes) != 0 {
		t.Fatalf("messages were sent: %d (crashes %v)", len(f.Calls()), m.crashes)
	}
}
