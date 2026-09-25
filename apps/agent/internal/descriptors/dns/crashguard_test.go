package dns

import (
	"context"
	"fmt"
	"net/netip"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/dns"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
)

// F-unbound-chrony-syslog, VPP 26.06 crash (2026-09-25 04:27, docs/vpp-code-track.md): a DNS request — dns_resolve_*
// or a UDP 53 packet — while the resolver is enabled without a name server dereferences NULL in ip4_sas. The fake
// models the defect: after every message it records a moment in which VPP "would crash". Driven through the real
// scheduler, every change of the upstream set (including replacing the last server, where the scheduler deletes
// before it creates) must never open such a moment.

type crashModel struct {
	enabled bool
	servers map[netip.Addr]bool
	crashes []string
	msgs    int
}

func newCrashFake(t *testing.T) (*dfkittest.FakeVPP, *crashModel) {
	t.Helper()
	f := dfkittest.NewFake()
	m := &crashModel{servers: map[netip.Addr]bool{}}
	check := func(what string) {
		m.msgs++
		if m.enabled && len(m.servers) == 0 {
			m.crashes = append(m.crashes, fmt.Sprintf("after message %d (%s): enabled without a name server", m.msgs, what))
		}
	}
	f.On("dns_enable_disable", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dns.DNSEnableDisable)
		if r.Enable == 1 && len(m.servers) == 0 {
			return []api.Message{&dns.DNSEnableDisableReply{Retval: int32(api.NO_NAME_SERVERS)}}, nil
		}
		m.enabled = r.Enable == 1
		check(fmt.Sprintf("enable=%d", r.Enable))
		return []api.Message{&dns.DNSEnableDisableReply{}}, nil
	})
	f.On("dns_name_server_add_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dns.DNSNameServerAddDel)
		a := netip.AddrFrom4([4]byte(r.ServerAddress[:4]))
		if r.IsAdd == 1 {
			m.servers[a] = true
		} else {
			if !m.servers[a] {
				return []api.Message{&dns.DNSNameServerAddDelReply{Retval: int32(api.NAME_SERVER_NOT_FOUND)}}, nil
			}
			delete(m.servers, a) // VPP allows removing the last server while enabled — the dangerous state
		}
		check(fmt.Sprintf("name server %s add=%d", a, r.IsAdd))
		return []api.Message{&dns.DNSNameServerAddDelReply{}}, nil
	})
	f.On("dns_resolve_name", func(api.Message) ([]api.Message, error) {
		if m.enabled && len(m.servers) == 0 {
			m.crashes = append(m.crashes, "dns_resolve_name while enabled without a name server")
		}
		return []api.Message{&dns.DNSResolveNameReply{IP4Set: 1, IP4Address: []byte{192, 0, 2, 1}, IP6Address: make([]byte, 16)}}, nil
	})
	return f, m
}

// desired is what the agent projects for vppCache {enabled, upstreams}.
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

func TestUpstreamChangesNeverLeaveAnEnabledResolverWithoutServers(t *testing.T) {
	f, m := newCrashFake(t)
	reg := scheduler.NewRegistry()
	RegisterGlobals(reg, f)
	s := scheduler.New(reg, nil)
	ctx := context.Background()
	for i, ups := range [][]string{
		{"10.5.0.53"},
		{"10.5.0.54"}, // replace the only server: the scheduler deletes 10.5.0.53 before it creates 10.5.0.54
		{"10.5.0.53", "10.5.0.54"},
		{"10.5.0.54"},
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
		if want := len(ups) > 0; m.enabled != want || len(m.servers) != len(ups) {
			t.Fatalf("step %d %v: enabled=%v servers=%v", i, ups, m.enabled, m.servers)
		}
	}
	if len(m.crashes) != 0 {
		t.Fatalf("VPP would have crashed: %v", m.crashes)
	}
}

// The old shape (dns.enable without dependencies, a delete that leaves the switch on) opens the window: the model
// detects it, so the test above is not vacuous.
func TestTheModelSeesTheWindow(t *testing.T) {
	f, m := newCrashFake(t)
	ctx := context.Background()
	ns, en := NewNameServer(f, dfkit.GlobalsOwner(true)), NewEnable(f, dfkit.GlobalsOwner(true))
	if _, err := ns.Create(ctx, NameServer{Address: "10.5.0.53"}.Proto()); err != nil {
		t.Fatal(err)
	}
	if _, err := en.Create(ctx, Enable{Enabled: true}.Proto()); err != nil {
		t.Fatal(err)
	}
	// a raw delete without the guard
	if _, err := dns.NewServiceClient(f).DNSNameServerAddDel(ctx, &dns.DNSNameServerAddDel{ServerAddress: append([]byte{10, 5, 0, 53}, make([]byte, 12)...)}); err != nil {
		t.Fatal(err)
	}
	if len(m.crashes) != 1 {
		t.Fatalf("the model missed the window: %v", m.crashes)
	}
}
