package dhcp

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/dhcp"
	"ngfw/agent/binapi/dhcp6_ia_na_client_cp"
	"ngfw/agent/binapi/dhcp6_pd_client_cp"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

const owner = "w5"

// proxyModel is a stateful fake of the relay tables: dhcp_proxy_config / set_vss / dump.
type proxyModel struct {
	servers map[bool]map[uint32][]dhcp.DHCPServer // is_ipv6 → rx vrf → servers
	src     map[bool]map[uint32]ip_types.Address
	vss     map[bool]map[uint32]*dhcp.DHCPProxySetVss
}

func newProxyFake() (*dfkittest.FakeVPP, *proxyModel) {
	f := dfkittest.NewFake()
	m := &proxyModel{
		servers: map[bool]map[uint32][]dhcp.DHCPServer{false: {}, true: {}},
		src:     map[bool]map[uint32]ip_types.Address{false: {}, true: {}},
		vss:     map[bool]map[uint32]*dhcp.DHCPProxySetVss{false: {}, true: {}},
	}
	f.On("dhcp_proxy_config", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dhcp.DHCPProxyConfig)
		v6 := r.DHCPServer.Af == ip_types.ADDRESS_IP6
		list := m.servers[v6][r.RxVrfID]
		if r.IsAdd {
			for _, s := range list {
				if s.DHCPServer == r.DHCPServer && s.ServerVrfID == r.ServerVrfID {
					return []api.Message{&dhcp.DHCPProxyConfigReply{}}, nil
				}
			}
			m.servers[v6][r.RxVrfID] = append(list, dhcp.DHCPServer{ServerVrfID: r.ServerVrfID, DHCPServer: r.DHCPServer})
			m.src[v6][r.RxVrfID] = r.DHCPSrcAddress
		} else {
			for i, s := range list {
				if s.DHCPServer == r.DHCPServer && s.ServerVrfID == r.ServerVrfID {
					m.servers[v6][r.RxVrfID] = append(list[:i], list[i+1:]...)
				}
			}
			if len(m.servers[v6][r.RxVrfID]) == 0 {
				delete(m.servers[v6], r.RxVrfID)
			}
		}
		return []api.Message{&dhcp.DHCPProxyConfigReply{}}, nil
	})
	f.On("dhcp_proxy_set_vss", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dhcp.DHCPProxySetVss)
		if r.IsAdd {
			m.vss[r.IsIPv6][r.TblID] = r
		} else {
			if _, ok := m.vss[r.IsIPv6][r.TblID]; !ok {
				return []api.Message{&dhcp.DHCPProxySetVssReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
			}
			delete(m.vss[r.IsIPv6], r.TblID)
		}
		return []api.Message{&dhcp.DHCPProxySetVssReply{}}, nil
	})
	f.On("dhcp_proxy_dump", func(msg api.Message) ([]api.Message, error) {
		v6 := msg.(*dhcp.DHCPProxyDump).IsIP6
		var out []api.Message
		for vrf, list := range m.servers[v6] {
			d := &dhcp.DHCPProxyDetails{RxVrfID: vrf, IsIPv6: v6, VssType: dhcp.VSS_TYPE_API_INVALID, Servers: list, Count: uint8(len(list))} //nolint:gosec // test model, few servers
			// VPP fills only the union of the details addresses (af stays 0)
			d.DHCPSrcAddress = ip_types.Address{Un: m.src[v6][vrf].Un}
			d.Servers = nil
			for _, s := range list {
				d.Servers = append(d.Servers, dhcp.DHCPServer{ServerVrfID: s.ServerVrfID, DHCPServer: ip_types.Address{Un: s.DHCPServer.Un}})
			}
			if v := m.vss[v6][vrf]; v != nil {
				d.VssType, d.VssVPNAsciiID, d.VssOui, d.VssFibID = v.VssType, v.VPNAsciiID, v.Oui, v.VPNIndex
			}
			out = append(out, d)
		}
		return out, nil
	})
	return f, m
}

func inScope(v uint32) bool { return v >= 5000 && v < 6000 }

func TestProxyLifecycle(t *testing.T) {
	f, m := newProxyFake()
	d := NewProxy(f, WithVRFScope(inScope))
	ctx := context.Background()
	want := Proxy{RxVRF: 5001, ServerVRF: 5002, Server: "10.5.0.1", Src: "10.5.0.2"}.Proto()
	want6 := Proxy{RxVRF: 5001, Server: "fd00:5::1", Src: "fd00:5::2"}.Proto()
	if k := d.KeyOf(want); k != "dhcp.proxy/5001/5002/10.5.0.1" {
		t.Fatalf("key %s", k)
	}
	deps := d.Dependencies(want)
	if len(deps) != 2 || deps[0].Key != "vrf/5001" || deps[1].Key != "vrf/5002" || !deps[0].Optional {
		t.Fatalf("deps %+v", deps)
	}
	if deps := d.Dependencies(want6); len(deps) != 1 { // vrf 0 is implicit
		t.Fatalf("deps6 %+v", deps)
	}
	for _, v := range []proto.Message{want, want6} {
		if _, err := d.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	req := f.CallsNamed("dhcp_proxy_config")[0].(*dhcp.DHCPProxyConfig)
	if !req.IsAdd || req.RxVrfID != 5001 || req.ServerVrfID != 5002 || req.DHCPServer.String() != "10.5.0.1" || req.DHCPSrcAddress.String() != "10.5.0.2" {
		t.Fatalf("request %+v", req)
	}
	// another slot's relay (vrf 7001) is filtered
	m.servers[false][7001] = []dhcp.DHCPServer{{DHCPServer: ip_types.NewAddress([]byte{10, 7, 0, 1})}}
	dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, want))
	dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, want6))
	dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, want), dfkittest.KV(d, want6))
	if _, err := d.Update(ctx, want, want, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("update: %v", err)
	}
	if err := d.Delete(ctx, want, nil); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertAbsent(t, d, d.KeyOf(want))
	// out-of-scope and invalid specs are refused before any VPP call
	f.Reset()
	for _, bad := range []Proxy{
		{RxVRF: 9001, Server: "10.5.0.1", Src: "10.5.0.2"},
		{RxVRF: 5001, Server: "10.5.0.1", Src: "fd00::1"},
		{RxVRF: 5001, Server: "010.5.0.1", Src: "10.5.0.2"},
		{RxVRF: 5001, Server: "0.0.0.0", Src: "10.5.0.2"},
	} {
		if _, err := d.Create(ctx, bad.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	if n := len(f.CallsNamed("dhcp_proxy_config")); n != 0 {
		t.Fatalf("%d calls for invalid specs", n)
	}
}

func TestProxyVSS(t *testing.T) {
	f, m := newProxyFake()
	p := NewProxy(f, WithVRFScope(inScope))
	d := NewProxyVSS(f, WithVRFScope(inScope))
	ctx := context.Background()
	if _, err := p.Create(ctx, Proxy{RxVRF: 5001, Server: "10.5.0.1", Src: "10.5.0.2"}.Proto()); err != nil {
		t.Fatal(err)
	}
	cases := []ProxyVSS{
		{Family: "ip4", VRF: 5001, Type: VSSASCII, VPNASCIIID: "w5-vpn"},
		{Family: "ip4", VRF: 5001, Type: VSSVPNID, OUI: 0xabcdef, VPNIndex: 9},
		{Family: "ip4", VRF: 5001, Type: VSSDefault},
	}
	var prev proto.Message
	for _, c := range cases {
		v := c.Proto()
		var err error
		if prev == nil {
			_, err = d.Create(ctx, v)
		} else {
			_, err = d.Update(ctx, prev, v, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v))
		dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, v))
		prev = v
	}
	if m.vss[false][5001].VssType != dhcp.VSS_TYPE_API_DEFAULT {
		t.Fatalf("vss type %v", m.vss[false][5001].VssType)
	}
	if err := d.Delete(ctx, prev, nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, prev, nil); err != nil { // NO_SUCH_ENTRY = already gone
		t.Fatal(err)
	}
	dfkittest.AssertAbsent(t, d, d.KeyOf(prev))
	for _, bad := range []ProxyVSS{
		{Family: "ip5", VRF: 5001, Type: VSSDefault},
		{Family: "ip4", VRF: 5001, Type: VSSASCII},
		{Family: "ip4", VRF: 5001, Type: VSSVPNID, OUI: 1 << 24},
		{Family: "ip4", VRF: 5001, Type: VSSDefault, VPNIndex: 1},
		{Family: "ip4", VRF: 5001, Type: "x"},
	} {
		if err := bad.Validate(); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func newClientFake() (*dfkittest.FakeVPP, map[uint32]dhcp.DHCPClient) {
	f := dfkittest.NewFake(
		dfkittest.Iface{Index: 7, Name: "loop501", Tag: "w5:loop501"},
		dfkittest.Iface{Index: 8, Name: "loop601", Tag: "w6:loop601"},
		dfkittest.Iface{Index: 9, Name: "ens192"},
	)
	clients := map[uint32]dhcp.DHCPClient{}
	f.On("dhcp_client_config", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dhcp.DHCPClientConfig)
		idx := uint32(r.Client.SwIfIndex)
		_, exists := clients[idx]
		switch {
		case r.IsAdd && exists, !r.IsAdd && !exists:
			return []api.Message{&dhcp.DHCPClientConfigReply{Retval: int32(api.INVALID_VALUE)}}, nil
		case r.IsAdd:
			c := r.Client
			c.ID = append([]byte(nil), c.ID...)
			c.ID = append(c.ID, make([]byte, 64-len(c.ID))...) // VPP returns the u8[64] NUL-padded
			clients[idx] = c
		default:
			delete(clients, idx)
		}
		return []api.Message{&dhcp.DHCPClientConfigReply{}}, nil
	})
	f.On("dhcp_client_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, c := range clients {
			out = append(out, &dhcp.DHCPClientDetails{Client: c, Lease: dhcp.DHCPLease{SwIfIndex: c.SwIfIndex, Hostname: c.Hostname}})
		}
		return out, nil
	})
	return f, clients
}

func TestClientLifecycle(t *testing.T) {
	f, clients := newClientFake()
	d := NewClient(f, owner)
	ctx := context.Background()
	want := Client{Interface: "loop501", Hostname: "w5-host", ClientID: "w5-cid", DSCP: 46, SetBroadcastFlag: true, WantEvents: true}.Proto()
	if deps := d.Dependencies(want); len(deps) != 1 || deps[0].Key != "interface/loop501" || deps[0].Optional {
		t.Fatalf("deps %+v", deps)
	}
	meta, err := d.Create(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	if meta != (ClientMeta{SwIfIndex: 7}) {
		t.Fatalf("meta %v", meta)
	}
	req := f.CallsNamed("dhcp_client_config")[0].(*dhcp.DHCPClientConfig)
	if !req.IsAdd || req.Client.Hostname != "w5-host" || string(req.Client.ID) != "w5-cid" || req.Client.Dscp != 46 || !req.Client.WantDHCPEvent || req.Client.PID == 0 {
		t.Fatalf("request %+v", req)
	}
	// a client of another owner (index 8) and one on an unowned NIC are filtered
	clients[8] = dhcp.DHCPClient{SwIfIndex: 8, Hostname: "w6"}
	clients[9] = dhcp.DHCPClient{SwIfIndex: 9, Hostname: "x"}
	got := dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, want))
	if got.Meta != meta {
		t.Fatalf("retrieved meta %v", got.Meta)
	}
	if kvs := dfkittest.MustRetrieve(t, d); len(kvs) != 1 {
		t.Fatalf("retrieved %d objects, want only ours", len(kvs))
	}
	dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, want))
	// re-apply of the identical client: VPP says INVALID_VALUE, Create compares and succeeds
	if _, err := d.Create(ctx, want); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	// a different client on the same interface is an error, not silently accepted
	other := Client{Interface: "loop501", Hostname: "w5-other"}.Proto()
	if _, err := d.Create(ctx, other); !dfkit.IsVPPError(err, api.INVALID_VALUE) {
		t.Fatalf("conflicting create: %v", err)
	}
	if _, err := d.Update(ctx, want, other, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("update: %v", err)
	}
	if err := d.Delete(ctx, want, meta); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, want, nil); err != nil { // already gone, meta lost (restart)
		t.Fatal(err)
	}
	dfkittest.AssertAbsent(t, d, d.KeyOf(want))
	leases, err := d.Leases(ctx)
	if err != nil || len(leases) != 0 {
		t.Fatalf("leases %v %v", leases, err)
	}
}

func TestClientRefusesForeignInterfaces(t *testing.T) {
	f, _ := newClientFake()
	d := NewClient(f, owner)
	for name, want := range map[string]error{"loop601": dfkit.ErrNotOwned, "loop999": dfkit.ErrNoInterface, "local0": dfkit.ErrNoInterface} {
		if _, err := d.Create(context.Background(), Client{Interface: name, Hostname: "w5-h"}.Proto()); !errors.Is(err, want) {
			t.Errorf("%s: %v, want %v", name, err, want)
		}
	}
	for _, bad := range []Client{{Interface: "loop501"}, {Interface: "loop501", Hostname: "a\nb"}, {Interface: "loop501", Hostname: "h", DSCP: 64}} {
		if _, err := d.Create(context.Background(), bad.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	if n := len(f.CallsNamed("dhcp_client_config")); n != 0 {
		t.Fatalf("%d VPP calls for refused clients", n)
	}
}

// Untagged (physical) interfaces are usable through a claim (D-071): reported only while claimed,
// released on Delete; another owner's claims are separate.
func TestClientOnUntaggedInterface(t *testing.T) {
	f, clients := newClientFake()
	owner := "w5claims"
	d := NewClient(f, owner)
	v := Client{Interface: "ens192", Hostname: "w5-wan"}.Proto()
	meta, err := d.Create(context.Background(), v)
	if err != nil || meta != (ClientMeta{SwIfIndex: 9}) {
		t.Fatalf("create on untagged: %v %v", meta, err)
	}
	dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v))
	if kvs := dfkittest.MustRetrieve(t, NewClient(f, "w5other")); len(kvs) != 0 {
		t.Fatalf("unclaimed untagged client reported to another owner: %v", kvs)
	}
	if err := d.Delete(context.Background(), v, meta); err != nil {
		t.Fatal(err)
	}
	if _, ok := clients[9]; ok || dfkit.Claims(owner).Claimed("ens192", NameClient) {
		t.Fatal("delete must remove the client and release the claim")
	}
}

func TestClientVPPErrors(t *testing.T) {
	f, _ := newClientFake()
	d := NewClient(f, owner)
	f.Reply("dhcp_client_config", &dhcp.DHCPClientConfigReply{Retval: int32(api.UNSPECIFIED)})
	if _, err := d.Create(context.Background(), Client{Interface: "loop501", Hostname: "w5-h"}.Proto()); !dfkit.IsVPPError(err, api.UNSPECIFIED) {
		t.Fatalf("create: %v", err)
	}
	f.SetConnected(false)
	if _, err := d.Retrieve(context.Background()); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("retrieve: %v", err)
	}
}

func TestWatchLeases(t *testing.T) {
	f, _ := newClientFake()
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := WatchLeases(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	f.Emit(&dhcp.DHCPComplEvent{PID: 42, Lease: dhcp.DHCPLease{
		SwIfIndex: 7, State: dhcp.DHCP_CLIENT_STATE_API_BOUND, MaskWidth: 24,
		HostAddress: ip_types.NewAddress([]byte{10, 5, 1, 9}), RouterAddress: ip_types.NewAddress([]byte{10, 5, 1, 1}),
		DomainServer: []dhcp.DomainServer{{Address: ip_types.NewAddress([]byte{10, 5, 1, 53})}},
	}})
	l := <-ch
	if l.State != "BOUND" || l.Address.String() != "10.5.1.9/24" || l.Router.String() != "10.5.1.1" || len(l.DNSServers) != 1 || l.FromEventOf != 42 {
		t.Fatalf("lease %+v", l)
	}
	cancel()
	for range ch { //nolint:revive // drain until closed
	}
}

func newDHCP6Fake() (*dfkittest.FakeVPP, map[string]bool) {
	f := dfkittest.NewFake(dfkittest.Iface{Index: 7, Name: "loop501", Tag: "w5:loop501"})
	state := map[string]bool{}
	f.On("dhcp6_client_enable_disable", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dhcp6_ia_na_client_cp.DHCP6ClientEnableDisable)
		state["na"] = r.Enable
		return []api.Message{&dhcp6_ia_na_client_cp.DHCP6ClientEnableDisableReply{}}, nil
	})
	f.On("dhcp6_pd_client_enable_disable", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dhcp6_pd_client_cp.DHCP6PdClientEnableDisable)
		state["pd:"+r.PrefixGroup] = r.Enable
		return []api.Message{&dhcp6_pd_client_cp.DHCP6PdClientEnableDisableReply{}}, nil
	})
	f.On("ip6_add_del_address_using_prefix", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dhcp6_pd_client_cp.IP6AddDelAddressUsingPrefix)
		k := "addr:" + r.PrefixGroup
		rv := int32(0)
		switch {
		case r.IsAdd && state[k]:
			rv = int32(api.DUPLICATE_IF_ADDRESS)
		case !r.IsAdd && !state[k]:
			rv = int32(api.ADDRESS_NOT_FOUND_FOR_INTERFACE)
		default:
			state[k] = r.IsAdd
		}
		return []api.Message{&dhcp6_pd_client_cp.IP6AddDelAddressUsingPrefixReply{Retval: rv}}, nil
	})
	f.On("dhcp6_duid_ll_set", dfkittest.Retval(&dhcp.DHCP6DuidLlSetReply{}))
	return f, state
}

func TestDHCP6WriteOnly(t *testing.T) {
	f, state := newDHCP6Fake()
	ctx := context.Background()
	na := NewDHCP6Client(f, owner)
	pd := NewDHCP6PDClient(f, owner)
	ad := NewDHCP6PDAddress(f, owner)
	vNA := DHCP6Client{Interface: "loop501"}.Proto()
	vPD := DHCP6PDClient{Interface: "loop501", PrefixGroup: "w5-pd"}.Proto()
	vAD := DHCP6PDAddress{Interface: "loop501", PrefixGroup: "w5-pd", Address: "::1:0:0:0:1/64"}.Proto()
	if k := ad.KeyOf(vAD); k != "dhcp.dhcp6-pd-address/loop501/w5-pd/::1:0:0:0:1/64" {
		t.Fatalf("key %s", k)
	}
	if deps := ad.Dependencies(vAD); len(deps) != 2 || deps[1].Key != "dhcp.dhcp6-pd-client/loop501" {
		t.Fatalf("deps %+v", deps)
	}
	for _, step := range []struct {
		d scheduler.Descriptor
		v proto.Message
	}{{na, vNA}, {pd, vPD}, {ad, vAD}} {
		for range 2 { // idempotent re-apply (write-only resync)
			meta, err := step.d.Create(ctx, step.v)
			if err != nil || meta != (IfaceMeta{SwIfIndex: 7}) {
				t.Fatalf("%s create: %v %v", step.d.Name(), meta, err)
			}
		}
		if kvs, err := step.d.Retrieve(ctx); kvs != nil || !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
			t.Fatalf("%s retrieve: %v %v", step.d.Name(), kvs, err)
		}
	}
	if !state["na"] || !state["pd:w5-pd"] || !state["addr:w5-pd"] {
		t.Fatalf("state %v", state)
	}
	if _, err := pd.Update(ctx, vPD, DHCP6PDClient{Interface: "loop501", PrefixGroup: "other"}.Proto(), nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("pd update: %v", err)
	}
	for _, step := range []struct {
		d scheduler.Descriptor
		v proto.Message
	}{{ad, vAD}, {pd, vPD}, {na, vNA}} {
		for range 2 {
			if err := step.d.Delete(ctx, step.v, IfaceMeta{SwIfIndex: 7}); err != nil {
				t.Fatalf("%s delete: %v", step.d.Name(), err)
			}
		}
	}
	if state["na"] || state["pd:w5-pd"] || state["addr:w5-pd"] {
		t.Fatalf("state after delete %v", state)
	}
	for _, bad := range []proto.Message{
		DHCP6PDAddress{Interface: "loop501", PrefixGroup: "g", Address: "10.0.0.1/24"}.Proto(),
		DHCP6PDAddress{Interface: "loop501", PrefixGroup: "g", Address: "::0001/64"}.Proto(),
		DHCP6PDAddress{Interface: "loop501", PrefixGroup: "", Address: "::1/64"}.Proto(),
	} {
		if _, err := ad.Create(ctx, bad); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%v: %v", bad, err)
		}
	}
}

func TestDHCP6DUID(t *testing.T) {
	f, _ := newDHCP6Fake()
	d := NewDHCP6DUID(f, dfkit.GlobalsOwner(true))
	v := DHCP6DUID{DUIDLL: "00:03:00:01:02:00:00:05:00:01"}.Proto()
	// D-071: a non-owner can neither set nor (no getter) verify the global DUID
	if _, err := NewDHCP6DUID(f, dfkit.GlobalsOwner(false)).Create(context.Background(), v); !errors.Is(err, dfkit.ErrNotGlobalsOwner) {
		t.Fatalf("non-owner create: %v", err)
	}
	if n := len(f.CallsNamed("dhcp6_duid_ll_set")); n != 0 {
		t.Fatalf("non-owner sent %d dhcp6_duid_ll_set", n)
	}
	if d.KeyOf(v) != KeyDHCP6DUID || d.Dependencies(v) != nil {
		t.Fatal("key/deps")
	}
	if _, err := d.Create(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	req := f.CallsNamed("dhcp6_duid_ll_set")[0].(*dhcp.DHCP6DuidLlSet)
	if len(req.DuidLl) != 10 || req.DuidLl[1] != 3 || req.DuidLl[7] != 5 {
		t.Fatalf("duid %x", req.DuidLl)
	}
	if _, err := d.Retrieve(context.Background()); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	if err := d.Delete(context.Background(), v, nil); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"00:01:00:01:02:00:00:05:00:01", "00:03:00", "00:03:00:01:02:00:00:05:00:0G", "0003000102000005 0001"} {
		if err := (DHCP6DUID{DUIDLL: bad}).Validate(); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestRegister(t *testing.T) {
	r := scheduler.NewRegistry()
	f := dfkittest.NewFake()
	RegisterGlobals(r, f)
	Register(r, f, owner)
	for _, n := range []string{NameProxy, NameProxyVSS, NameClient, NameDHCP6Client, NameDHCP6PDClient, NameDHCP6PDAddr, NameDHCP6DUID} {
		if _, ok := r.Get(n); !ok {
			t.Errorf("%s not registered", n)
		}
	}
}
