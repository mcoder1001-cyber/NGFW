package dns

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/dns"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
)

type model struct {
	enabled bool
	servers map[netip.Addr]bool
}

func newFake() (*dfkittest.FakeVPP, *model) {
	f := dfkittest.NewFake()
	m := &model{servers: map[netip.Addr]bool{}}
	f.On("dns_enable_disable", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dns.DNSEnableDisable)
		if r.Enable == 1 && len(m.servers) == 0 {
			return []api.Message{&dns.DNSEnableDisableReply{Retval: int32(api.NO_NAME_SERVERS)}}, nil
		}
		m.enabled = r.Enable == 1
		return []api.Message{&dns.DNSEnableDisableReply{}}, nil
	})
	f.On("dns_name_server_add_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dns.DNSNameServerAddDel)
		var a netip.Addr
		if r.IsIP6 == 1 {
			a = netip.AddrFrom16([16]byte(r.ServerAddress))
		} else {
			a = netip.AddrFrom4([4]byte(r.ServerAddress[:4]))
		}
		if r.IsAdd == 1 {
			m.servers[a] = true
		} else {
			if !m.servers[a] {
				return []api.Message{&dns.DNSNameServerAddDelReply{Retval: int32(api.NAME_SERVER_NOT_FOUND)}}, nil
			}
			delete(m.servers, a)
		}
		return []api.Message{&dns.DNSNameServerAddDelReply{}}, nil
	})
	return f, m
}

func TestDNSLifecycle(t *testing.T) {
	f, m := newFake()
	ctx := context.Background()
	r := scheduler.NewRegistry()
	Register(r, f)
	if names := r.Names(); len(names) != 2 || names[0] != NameNameServer {
		t.Fatalf("registration order %v (name servers must come first)", names)
	}
	ns := NewNameServer(f)
	en := NewEnable(f)
	on := Enable{Enabled: true}.Proto()
	// enabling without a name server fails in VPP: the order matters
	if _, err := en.Create(ctx, on); !dfkit.IsVPPError(err, api.NO_NAME_SERVERS) {
		t.Fatalf("enable without servers: %v", err)
	}
	v4 := NameServer{Address: "10.5.0.53"}.Proto()
	v6 := NameServer{Address: "fd00:5::53"}.Proto()
	if k := ns.KeyOf(v6); k != "dns.name-server/fd00:5::53" {
		t.Fatalf("key %s", k)
	}
	for _, v := range []proto.Message{v4, v6, v4} { // duplicate add is idempotent
		if _, err := ns.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	req := f.CallsNamed("dns_name_server_add_del")[1].(*dns.DNSNameServerAddDel)
	if req.IsIP6 != 1 || req.IsAdd != 1 || len(req.ServerAddress) != 16 || req.ServerAddress[15] != 0x53 {
		t.Fatalf("request %+v", req)
	}
	for range 2 {
		if _, err := en.Create(ctx, on); err != nil {
			t.Fatal(err)
		}
	}
	if !m.enabled || len(m.servers) != 2 {
		t.Fatalf("model %+v", m)
	}
	for _, d := range []scheduler.Descriptor{ns, en} {
		if kvs, err := d.Retrieve(ctx); kvs != nil || !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
			t.Fatalf("%s retrieve: %v %v", d.Name(), kvs, err)
		}
	}
	if _, err := en.Update(ctx, on, Enable{}.Proto(), nil); err != nil || m.enabled {
		t.Fatalf("update to disabled: %v %v", err, m.enabled)
	}
	if err := en.Delete(ctx, on, nil); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := ns.Delete(ctx, v4, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, bad := range []string{"", "10.5.0.053", "0.0.0.0", "::ffff:10.0.0.1", "fe80::1%eth0"} {
		if _, err := ns.Create(ctx, NameServer{Address: bad}.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if kvs := en.Dependencies(on); kvs != nil || en.KeyOf(on) != KeyEnable {
		t.Fatal("enable key/deps")
	}
}

func TestResolveHelpers(t *testing.T) {
	f, _ := newFake()
	f.On("dns_resolve_name", func(api.Message) ([]api.Message, error) {
		return []api.Message{&dns.DNSResolveNameReply{IP4Set: 1, IP4Address: []byte{10, 5, 0, 9}, IP6Address: make([]byte, 16)}}, nil
	})
	name := make([]byte, 256)
	copy(name, "w5-host.example")
	f.Reply("dns_resolve_ip", &dns.DNSResolveIPReply{Name: name})
	ip4, ip6, err := ResolveName(context.Background(), f, "w5-host.example")
	if err != nil || ip4.String() != "10.5.0.9" || ip6.IsValid() {
		t.Fatalf("resolve name: %v %v %v", ip4, ip6, err)
	}
	got, err := ResolveIP(context.Background(), f, netip.MustParseAddr("10.5.0.9"))
	if err != nil || got != "w5-host.example" {
		t.Fatalf("resolve ip: %q %v", got, err)
	}
	for _, bad := range []string{"", "a b", "x;rm -rf /", "$(id)", string(make([]byte, 300))} {
		if _, _, err := ResolveName(context.Background(), f, bad); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}
