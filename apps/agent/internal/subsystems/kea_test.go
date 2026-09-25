package subsystems

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/dhcp"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/renderers/kea"
	"ngfw/agent/internal/scheduler"
)

// registerFor wires a fresh agent of owner with the given id scope on a fake VPP.
func registerFor(t *testing.T, owner string, ids IDScope) (*scheduler.MapRegistry, *Wiring, *coretest.VPP, error) {
	t.Helper()
	dir := t.TempDir()
	owned, err := ownertable.Open(dir, owner)
	if err != nil {
		t.Fatal(err)
	}
	v := coretest.New()
	reg := scheduler.NewRegistry()
	w, err := Register(reg, Env{Client: v, Owner: owner, StateDir: dir, Owned: owned, IDs: ids})
	return reg, w, v, err
}

// Review M3: the relay families own the rx VRFs of the agent's id range and fail closed. A zero IDScope (no
// VRX_VPP_TABLE_BASE, no VRX_VPP_ID_RANGE=all) makes Wiring.IDRange return NoIDs with ErrNoIDRange: a relay in any
// table is neither retrieved (so never deleted) nor created. A refactor to "on error, own everything" (rng = nil)
// would hand a slot agent every relay of the shared VPP — this test fails then.
func TestRegisterKeaRelayScope(t *testing.T) {
	t.Setenv(EnvKeaMode, "off")
	ctx := context.Background()
	proxyIn := func(table uint32) dhcp.Proxy {
		return dhcp.Proxy{RxVRF: table, ServerVRF: table, Server: "10.9.2.2", Src: "10.9.2.1"}
	}
	cases := []struct {
		name  string
		ids   IDScope
		table uint32
		owns  bool
	}{
		{"no range: fail closed", IDScope{}, 5000, false},
		{"no range: table 0 too", IDScope{}, 0, false},
		{"slot range, inside", IDScope{Range: &IDRange{Lo: 5000, Hi: 5999}}, 5001, true},
		{"slot range, outside", IDScope{Range: &IDRange{Lo: 5000, Hi: 5999}}, 7001, false},
		{"product: every VRF", IDScope{All: true}, 7001, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg, _, v, err := registerFor(t, "wk3", tc.ids)
			if err != nil {
				t.Fatal(err)
			}
			d, ok := reg.Get(dhcp.NameProxy)
			if !ok {
				t.Fatal("dhcp.proxy not registered")
			}
			// a relay someone configured in VPP (another slot, the product): retrieved only when owned
			other := dhcp.NewProxy(v) // unscoped: writes whatever it is told
			if _, err := other.Create(ctx, proxyIn(tc.table).Proto()); err != nil {
				t.Fatal(err)
			}
			kvs, err := d.Retrieve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(kvs) == 1; got != tc.owns {
				t.Fatalf("retrieved %d relays of table %d, owns=%v", len(kvs), tc.table, tc.owns)
			}
			// creating one in that table: refused unless owned (nothing is written to VPP then)
			p2 := proxyIn(tc.table)
			p2.Server = "10.9.2.3"
			before := len(v.DHCPProxies())
			_, err = d.Create(ctx, p2.Proto())
			switch {
			case tc.owns && err != nil:
				t.Fatalf("create in an owned table: %v", err)
			case !tc.owns && (err == nil || !strings.Contains(err.Error(), "outside this agent's VRF scope")):
				t.Fatalf("create outside the scope: want refused, got %v", err)
			case !tc.owns && len(v.DHCPProxies()) != before:
				t.Fatal("a refused create wrote to VPP")
			}
			// the relay records follow the same scope
			rd, _ := reg.Get(dhcp.NameRelay)
			_, err = rd.Create(ctx, dhcp.Relay{Name: "r", Enabled: true, RxVRF: tc.table, Src: "10.9.2.1", Servers: []string{"10.9.2.2"}}.Proto())
			if (err == nil) != tc.owns {
				t.Fatalf("relay record in table %d: err=%v owns=%v", tc.table, err, tc.owns)
			}
		})
	}
}

// Review M3: VRX_KEA_MODE and VRX_KEA_IFMAP / VRX_KEA_NETNS parsing; review Q7: test mode is refused for the product
// agent (owner "vrx" or VRX_VPP_ID_RANGE=all).
func TestRegisterKeaModes(t *testing.T) {
	slot := IDScope{Range: &IDRange{Lo: 4000, Hi: 4999}}
	cases := []struct {
		name, mode, netns, ifmap, owner string
		ids                             IDScope
		wantErr                         string
		kea                             bool
		confDir, ns                     string
	}{
		{name: "default is product", owner: "wk4", ids: slot, kea: true, confDir: "/etc/kea"},
		{name: "product", mode: "product", owner: "wk4", ids: slot, kea: true, confDir: "/etc/kea"},
		{name: "off", mode: "off", owner: "wk4", ids: slot},
		{name: "bogus mode refuses to start", mode: "lab", owner: "wk4", ids: slot, wantErr: "want product, test or off"},
		{name: "test", mode: "test", netns: "ns-wk4-wan", ifmap: "host-wk4l0=wk4w1, host-wk4x=wk4x1", owner: "wk4", ids: slot,
			kea: true, confDir: "/run/vrx-test/wk4/kea/etc", ns: "ns-wk4-wan"},
		{name: "test, default netns", mode: "test", owner: "wk4", ids: slot, kea: true, confDir: "/run/vrx-test/wk4/kea/etc", ns: "ns-wk4-a"},
		{name: "test, bad map pair", mode: "test", ifmap: "host-wk4l0:wk4w1", owner: "wk4", ids: slot, wantErr: "is not <vpp-if>=<linux-if>"},
		{name: "test, linux name too long", mode: "test", ifmap: "host-wk4l0=wk4-way-too-long-name", owner: "wk4", ids: slot, wantErr: "is not <vpp-if>=<linux-if>"},
		{name: "test, bad netns", mode: "test", netns: "-exec", owner: "wk4", ids: slot, wantErr: "Paths.Netns"},
		{name: "Q7: test refused for owner vrx", mode: "test", owner: "vrx", ids: IDScope{All: true}, wantErr: "refused for the product agent"},
		{name: "Q7: test refused with id range all", mode: "test", owner: "wk4", ids: IDScope{All: true}, wantErr: "refused for the product agent"},
		{name: "Q7: test refused for owner vrx on a range", mode: "test", owner: "vrx", ids: slot, wantErr: "refused for the product agent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvKeaMode, tc.mode)
			t.Setenv(EnvKeaNetns, tc.netns)
			t.Setenv(EnvKeaIfMap, tc.ifmap)
			reg, w, _, err := registerFor(t, tc.owner, tc.ids)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			_, has4 := reg.Get(kea.NameDhcp4)
			_, has6 := reg.Get(kea.NameDhcp6)
			if has4 != tc.kea || has6 != tc.kea {
				t.Fatalf("kea descriptors registered: %v/%v, want %v", has4, has6, tc.kea)
			}
			for _, n := range []string{dhcp.NameProxy, dhcp.NameProxyVSS, dhcp.NameRelay} {
				if _, ok := reg.Get(n); !ok {
					t.Fatalf("%s not registered", n)
				}
			}
			rt := w.DHCP()
			if rt == nil || rt.Client == nil {
				t.Fatal("no DHCP runtime")
			}
			if !tc.kea {
				if rt.Kea != nil {
					t.Fatal("off: a Kea renderer in the runtime")
				}
				return
			}
			if got := rt.Kea.Paths().ConfDir; got != tc.confDir {
				t.Fatalf("conf dir %s, want %s", got, tc.confDir)
			}
			if got := rt.Kea.Paths().Netns; got != tc.ns {
				t.Fatalf("netns %q, want %q", got, tc.ns)
			}
		})
	}
}

// The test-mode interface map is the only scope: listed names map, anything else is refused, and no "<if>/<addr>"
// binding is rendered (the mapped netdev does not carry the VPP address).
func TestKeaTestModeMapper(t *testing.T) {
	t.Setenv(EnvKeaIfMap, "host-wk5l0=wk5w1")
	t.Setenv(EnvKeaNetns, "")
	r, desc, err := keaRenderer("wk5", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(desc, "ns-wk5-a") || !strings.Contains(desc, "1 mapped") {
		t.Fatalf("description %q", desc)
	}
	ds := func(ifn string) *vrxv1.DesiredState {
		return &vrxv1.DesiredState{
			Interfaces: map[string]*vrxv1.Interface{ifn: {Ipv4: []string{"10.5.1.1/24"}}},
			Services: &vrxv1.ServicesConfig{Dhcp: &vrxv1.DhcpService{Servers: map[string]*vrxv1.DhcpServer{"lan": {
				Interfaces: []string{ifn},
				Subnets: map[string]*vrxv1.DhcpSubnet{"lan": {Subnet: proto.String("10.5.1.0/24"),
					Pools: []*vrxv1.DhcpPool{{Start: proto.String("10.5.1.100"), End: proto.String("10.5.1.150")}}}},
			}}}},
		}
	}
	files, err := r.RenderFamily(kea.Input(ds("host-wk5l0"), 4), 4)
	if err != nil {
		t.Fatal(err)
	}
	conf := string(files[r.Paths().Dhcp4Conf()].Content)
	if !strings.Contains(conf, `"wk5w1"`) || strings.Contains(conf, "wk5w1/") {
		t.Fatalf("mapped interface / binding:\n%s", conf)
	}
	if _, err := r.RenderFamily(kea.Input(ds("host-wk5x9"), 4), 4); err == nil || !strings.Contains(err.Error(), EnvKeaIfMap) {
		t.Fatalf("an unmapped interface must be refused, got %v", err)
	}
}
