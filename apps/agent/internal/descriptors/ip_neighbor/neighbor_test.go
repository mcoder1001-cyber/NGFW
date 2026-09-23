package ipneighbor

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ethernet_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_neighbor"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/fake"
)

// fakeVPP models the neighbour table of a VPP with three interfaces: local0 (untagged),
// loop300 (ours, w3) and loop200 (another worker's).
type fakeVPP struct {
	*fake.Client
	neighbors []ip_neighbor.IPNeighbor
	config    map[ip_types.AddressFamily]*ip_neighbor.IPNeighborConfig
}

func newFakeVPP() *fakeVPP {
	v := &fakeVPP{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), config: map[ip_types.AddressFamily]*ip_neighbor.IPNeighborConfig{}}
	v.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "loop300", Tag: "w3:loop300"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 7, InterfaceName: "loop200", Tag: "w2:loop200"},
	)
	v.On("ip_neighbor_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip_neighbor.IPNeighborAddDel)
		if r.Neighbor.SwIfIndex > 7 {
			return []api.Message{&ip_neighbor.IPNeighborAddDelReply{Retval: -2}}, nil // INVALID_SW_IF_INDEX
		}
		for i, n := range v.neighbors {
			if n.SwIfIndex == r.Neighbor.SwIfIndex && n.IPAddress == r.Neighbor.IPAddress {
				if r.IsAdd {
					v.neighbors[i] = r.Neighbor
				} else {
					v.neighbors = append(v.neighbors[:i], v.neighbors[i+1:]...)
				}
				return []api.Message{&ip_neighbor.IPNeighborAddDelReply{}}, nil
			}
		}
		if r.IsAdd {
			v.neighbors = append(v.neighbors, r.Neighbor)
		}
		return []api.Message{&ip_neighbor.IPNeighborAddDelReply{}}, nil
	})
	v.On("ip_neighbor_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip_neighbor.IPNeighborDump)
		var out []api.Message
		for _, n := range v.neighbors {
			if n.IPAddress.Af == r.Af {
				out = append(out, &ip_neighbor.IPNeighborDetails{Neighbor: n})
			}
		}
		return out, nil
	})
	v.On("ip_neighbor_config", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip_neighbor.IPNeighborConfig)
		cp := *r
		v.config[r.Af] = &cp
		return []api.Message{&ip_neighbor.IPNeighborConfigReply{}}, nil
	})
	v.On("ip_neighbor_config_get", func(req api.Message) ([]api.Message, error) {
		af := req.(*ip_neighbor.IPNeighborConfigGet).Af
		c, ok := v.config[af]
		if !ok {
			c = &ip_neighbor.IPNeighborConfig{Af: af, MaxNumber: DefaultMaxNumber}
		}
		return []api.Message{&ip_neighbor.IPNeighborConfigGetReply{Af: af, MaxNumber: c.MaxNumber, MaxAge: c.MaxAge, Recycle: c.Recycle}}, nil
	})
	return v
}

func mac(s string) ethernet_types.MacAddress {
	m, err := ethernet_types.ParseMacAddress(s)
	if err != nil {
		panic(err)
	}
	return m
}

func addr(s string) ip_types.Address {
	a, err := df2.ParseAddr(s)
	if err != nil {
		panic(err)
	}
	return df2.ToAddress(a)
}

func TestNeighborKeyAndDependencies(t *testing.T) {
	d := NewNeighbor(nil, "w3")
	n := &Neighbor{Interface: "loop300", IpAddress: "::ffff:10.3.1.10", MacAddress: "AA:BB:CC:00:11:22"}
	if got := d.KeyOf(n); got != "ip-neighbor.neighbor/loop300/10.3.1.10" {
		t.Fatalf("KeyOf = %s", got)
	}
	n6 := &Neighbor{Interface: "loop300", IpAddress: "2001:DB8:3::0001", MacAddress: "aa:bb:cc:00:11:22"}
	if got := d.KeyOf(n6); got != "ip-neighbor.neighbor/loop300/2001:db8:3::1" {
		t.Fatalf("KeyOf v6 = %s", got)
	}
	deps := d.Dependencies(n)
	if len(deps) != 1 || deps[0].Key != "interface/loop300" || deps[0].Optional {
		t.Fatalf("Dependencies = %+v", deps)
	}
	if !scheduler.ValidName(d.Name()) {
		t.Fatalf("invalid name %q", d.Name())
	}
}

func TestNeighborLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	// Pre-existing entries that must never be ours: a learned one on our interface, a static
	// one on another worker's interface and one on the untagged local0.
	v.neighbors = []ip_neighbor.IPNeighbor{
		{SwIfIndex: 5, IPAddress: addr("10.3.1.99"), MacAddress: mac("00:00:00:00:00:99")},
		{SwIfIndex: 7, Flags: ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC, IPAddress: addr("10.2.1.1"), MacAddress: mac("00:00:00:00:00:02")},
		{SwIfIndex: 0, Flags: ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC, IPAddress: addr("192.168.0.1"), MacAddress: mac("00:00:00:00:00:00")},
	}
	d := NewNeighbor(v, "w3")

	desired := &Neighbor{Interface: "loop300", IpAddress: "10.3.1.10", MacAddress: "aa:bb:cc:00:11:22"}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	if meta != (NeighborMeta{SwIfIndex: 5}) {
		t.Fatalf("meta = %+v", meta)
	}
	calls := v.CallsNamed("ip_neighbor_add_del")
	if len(calls) != 1 {
		t.Fatalf("add_del calls = %d", len(calls))
	}
	req := calls[0].(*ip_neighbor.IPNeighborAddDel)
	if !req.IsAdd || req.Neighbor.SwIfIndex != 5 || req.Neighbor.Flags != ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC ||
		req.Neighbor.MacAddress != mac("aa:bb:cc:00:11:22") || req.Neighbor.IPAddress != addr("10.3.1.10") {
		t.Fatalf("request = %+v", req)
	}

	// IPv6 neighbour with no-fib-entry on the same interface.
	desired6 := &Neighbor{Interface: "loop300", IpAddress: "2001:db8:3::1", MacAddress: "aa:bb:cc:00:11:33", NoFibEntry: true}
	if _, err := d.Create(ctx, desired6); err != nil {
		t.Fatal(err)
	}
	req = v.CallsNamed("ip_neighbor_add_del")[1].(*ip_neighbor.IPNeighborAddDel)
	if req.Neighbor.Flags != ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC|ip_neighbor.IP_API_NEIGHBOR_FLAG_NO_FIB_ENTRY {
		t.Fatalf("flags = %v", req.Neighbor.Flags)
	}

	// Retrieve: exactly our two static entries, decoded equal to desired, Meta as Create.
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != 2 {
		t.Fatalf("Retrieve = %d objects, want 2: %+v", len(actual), actual)
	}
	byKey := map[scheduler.Key]scheduler.KV{}
	for _, kv := range actual {
		byKey[kv.Key] = kv
	}
	for _, want := range []*Neighbor{desired, desired6} {
		kv, ok := byKey[d.KeyOf(want)]
		if !ok || !proto.Equal(kv.Value, want) || kv.Meta != (NeighborMeta{SwIfIndex: 5}) {
			t.Fatalf("Retrieve[%s] = %+v, want %+v", d.KeyOf(want), kv, want)
		}
	}

	// Update: MAC changes in place; address / interface / flags need a recreate.
	newMeta, err := d.Update(ctx, desired, &Neighbor{Interface: "loop300", IpAddress: "10.3.1.10", MacAddress: "aa:bb:cc:00:11:44"}, meta)
	if err != nil || newMeta != meta {
		t.Fatalf("Update mac: %v %+v", err, newMeta)
	}
	if actual, _ = d.Retrieve(ctx); byKV(actual, d.KeyOf(desired)).(*Neighbor).GetMacAddress() != "aa:bb:cc:00:11:44" {
		t.Fatalf("mac not updated: %+v", actual)
	}
	for _, changed := range []*Neighbor{
		{Interface: "loop300", IpAddress: "10.3.1.11", MacAddress: "aa:bb:cc:00:11:44"},
		{Interface: "loop301", IpAddress: "10.3.1.10", MacAddress: "aa:bb:cc:00:11:44"},
		{Interface: "loop300", IpAddress: "10.3.1.10", MacAddress: "aa:bb:cc:00:11:44", NoFibEntry: true},
	} {
		if _, err := d.Update(ctx, desired, changed, meta); !errors.Is(err, scheduler.ErrRecreate) {
			t.Fatalf("Update %+v: got %v, want ErrRecreate", changed, err)
		}
	}

	// Delete both, Retrieve shows nothing of ours, foreign entries untouched.
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, desired6, NeighborMeta{SwIfIndex: 5}); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 {
		t.Fatalf("after Delete Retrieve = %+v", actual)
	}
	if len(v.neighbors) != 3 {
		t.Fatalf("foreign neighbours touched: %+v", v.neighbors)
	}
}

func byKV(kvs []scheduler.KV, k scheduler.Key) proto.Message {
	for _, kv := range kvs {
		if kv.Key == k {
			return kv.Value
		}
	}
	return nil
}

func TestNeighborErrors(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	d := NewNeighbor(v, "w3")
	if _, err := d.Create(ctx, &Neighbor{Interface: "loop999", IpAddress: "10.3.1.1", MacAddress: "aa:bb:cc:00:11:22"}); !errors.Is(err, df2.ErrNoSuchInterface) {
		t.Fatalf("unknown interface: %v", err)
	}
	if _, err := d.Create(ctx, &Neighbor{Interface: "loop300", IpAddress: "not-an-ip", MacAddress: "aa:bb:cc:00:11:22"}); err == nil {
		t.Fatal("bad address accepted")
	}
	if _, err := d.Create(ctx, &Neighbor{Interface: "loop300", IpAddress: "10.3.1.1", MacAddress: "zz"}); err == nil {
		t.Fatal("bad mac accepted")
	}
	// D-071: a stale sw_if_index (interface gone, or the index now names another interface)
	// is re-verified right before the delete and nothing is sent to VPP.
	before := len(v.CallsNamed("ip_neighbor_add_del"))
	if err := d.Delete(ctx, &Neighbor{Interface: "loop300", IpAddress: "10.3.1.1", MacAddress: "aa:bb:cc:00:11:22"}, NeighborMeta{SwIfIndex: 99}); err != nil {
		t.Fatalf("delete on a vanished interface: %v", err)
	}
	if n := len(v.CallsNamed("ip_neighbor_add_del")); n != before {
		t.Fatalf("delete by a stale sw_if_index reached VPP (%d calls)", n-before)
	}
	if err := d.Delete(ctx, &Neighbor{}, "wrong"); !errors.Is(err, df2.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
	v.SetConnected(false)
	if _, err := d.Retrieve(ctx); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected: %v", err)
	}
}

func TestConfigLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	d := NewConfig(v)
	if d.Dependencies(&Config{}) != nil || !scheduler.ValidName(d.Name()) {
		t.Fatal("config has no dependencies and a valid name")
	}
	if k := d.KeyOf(&Config{Af: df2.AddressFamily_IPV6}); k != "ip-neighbor.config/ipv6" {
		t.Fatalf("KeyOf = %s", k)
	}
	// Retrieve always returns both families, even before anything was configured.
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 2 {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	desired := &Config{Af: df2.AddressFamily_IPV4, MaxNumber: 1000, MaxAge: 300, Recycle: true}
	if _, err := d.Create(ctx, desired); err != nil {
		t.Fatal(err)
	}
	req := v.CallsNamed("ip_neighbor_config")[0].(*ip_neighbor.IPNeighborConfig)
	if req.Af != ip_types.ADDRESS_IP4 || req.MaxNumber != 1000 || req.MaxAge != 300 || !req.Recycle {
		t.Fatalf("request = %+v", req)
	}
	actual, _ = d.Retrieve(ctx)
	if !proto.Equal(byKV(actual, d.KeyOf(desired)), desired) {
		t.Fatalf("Retrieve = %+v", actual)
	}
	if _, err := d.Update(ctx, desired, &Config{Af: df2.AddressFamily_IPV4, MaxNumber: 2000}, nil); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); byKV(actual, d.KeyOf(desired)).(*Config).GetMaxNumber() != 2000 {
		t.Fatalf("not updated: %+v", actual)
	}
	// Delete restores the VPP defaults.
	if err := d.Delete(ctx, desired, nil); err != nil {
		t.Fatal(err)
	}
	actual, _ = d.Retrieve(ctx)
	if !proto.Equal(byKV(actual, d.KeyOf(desired)), &Config{Af: df2.AddressFamily_IPV4, MaxNumber: DefaultMaxNumber}) {
		t.Fatalf("defaults not restored: %+v", actual)
	}
	v.Fail("ip_neighbor_config_get", errors.New("boom"))
	if _, err := d.Retrieve(ctx); err == nil {
		t.Fatal("dump error must surface")
	}
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	Register(reg, fake.New(), "w3")
	if got := reg.Names(); len(got) != 1 || got[0] != NeighborName {
		t.Fatalf("Names = %v (ip-neighbor.config is a global: RegisterGlobals only, D-071)", got)
	}
	RegisterGlobals(reg, fake.New())
	if _, ok := reg.Get(ConfigName); !ok {
		t.Fatal("RegisterGlobals did not register ip-neighbor.config")
	}
	var _ interface_types.InterfaceIndex // keep the import used by the fixture types above
}
