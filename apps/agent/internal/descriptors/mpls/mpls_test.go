package mpls

import (
	"errors"
	"fmt"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/fib_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/mpls"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
)

type rkey struct {
	table, label uint32
	eos          uint8
}

// fakeMPLS models tables, the per-interface enable counter (a u8 that VPP decrements without
// checking), label routes with VPP's reserved special entries, bindings and tunnels (deleted
// only when the delete carries all their paths).
func fakeMPLS() (*df7test.Fake, map[uint32]string, map[uint32]uint8, map[rkey]mpls.MplsRoute, map[uint32]*mpls.MplsTunnel) {
	f := df7test.NewFake()
	tables := map[uint32]string{}
	enabled := map[uint32]uint8{}
	routes := map[rkey]mpls.MplsRoute{}
	tunnels := map[uint32]*mpls.MplsTunnel{}
	next := uint32(20)
	f.On("mpls_table_add_del", func(m api.Message) ([]api.Message, error) {
		r := m.(*mpls.MplsTableAddDel)
		if r.MtIsAdd {
			if _, ok := tables[r.MtTable.MtTableID]; !ok {
				tables[r.MtTable.MtTableID] = r.MtTable.MtName
				routes[rkey{r.MtTable.MtTableID, 0, 1}] = mpls.MplsRoute{MrTableID: r.MtTable.MtTableID, MrEos: 1} // special entry
			}
		} else {
			delete(tables, r.MtTable.MtTableID)
		}
		return []api.Message{&mpls.MplsTableAddDelReply{}}, nil
	})
	f.On("mpls_table_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for id, n := range tables {
			out = append(out, &mpls.MplsTableDetails{MtTable: mpls.MplsTable{MtTableID: id, MtName: n}})
		}
		return out, nil
	})
	f.On("sw_interface_set_mpls_enable", func(m api.Message) ([]api.Message, error) {
		r := m.(*mpls.SwInterfaceSetMplsEnable)
		if _, ok := tables[0]; !ok {
			return []api.Message{&mpls.SwInterfaceSetMplsEnableReply{Retval: int32(api.NO_SUCH_FIB)}}, nil
		}
		if r.Enable {
			enabled[uint32(r.SwIfIndex)]++
		} else {
			enabled[uint32(r.SwIfIndex)]-- // wraps at 0, like VPP
		}
		return []api.Message{&mpls.SwInterfaceSetMplsEnableReply{}}, nil
	})
	f.On("mpls_interface_dump", func(m api.Message) ([]api.Message, error) {
		want := uint32(m.(*mpls.MplsInterfaceDump).SwIfIndex)
		var out []api.Message
		for idx, n := range enabled {
			if n > 0 && (want == df7.NoIndex || want == idx) {
				out = append(out, &mpls.MplsInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx)})
			}
		}
		return out, nil
	})
	f.On("mpls_route_add_del", func(m api.Message) ([]api.Message, error) {
		r := m.(*mpls.MplsRouteAddDel)
		k := rkey{r.MrRoute.MrTableID, r.MrRoute.MrLabel, r.MrRoute.MrEos}
		if r.MrIsAdd {
			rt := r.MrRoute
			if rt.MrEos == 0 {
				rt.MrEosProto = 2 // VPP reports the MPLS payload for non-EOS entries
			}
			routes[k] = rt
		} else {
			if _, ok := routes[k]; !ok {
				return []api.Message{&mpls.MplsRouteAddDelReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
			}
			delete(routes, k)
		}
		return []api.Message{&mpls.MplsRouteAddDelReply{}}, nil
	})
	f.On("mpls_route_dump", func(m api.Message) ([]api.Message, error) {
		id := m.(*mpls.MplsRouteDump).Table.MtTableID
		var out []api.Message
		for k, rt := range routes {
			if k.table == id {
				out = append(out, &mpls.MplsRouteDetails{MrRoute: rt})
			}
		}
		return out, nil
	})
	f.On("mpls_tunnel_add_del", func(m api.Message) ([]api.Message, error) {
		r := m.(*mpls.MplsTunnelAddDel)
		if r.MtIsAdd {
			t := r.MtTunnel
			t.MtSwIfIndex = interface_types.InterfaceIndex(next)
			t.MtTunnelIndex = next - 20
			tunnels[next] = &t
			next++
			return []api.Message{&mpls.MplsTunnelAddDelReply{SwIfIndex: t.MtSwIfIndex, TunnelIndex: t.MtTunnelIndex}}, nil
		}
		t, ok := tunnels[uint32(r.MtTunnel.MtSwIfIndex)]
		if ok && len(r.MtTunnel.MtPaths) == len(t.MtPaths) {
			delete(tunnels, uint32(r.MtTunnel.MtSwIfIndex))
		}
		return []api.Message{&mpls.MplsTunnelAddDelReply{}}, nil
	})
	f.On("mpls_tunnel_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, t := range tunnels {
			out = append(out, &mpls.MplsTunnelDetails{MtTunnel: *t})
		}
		return out, nil
	})
	f.Reply("sw_interface_tag_add_del", &interfaces.SwInterfaceTagAddDelReply{})
	f.Reply("mpls_ip_bind_unbind", &mpls.MplsIPBindUnbindReply{})
	return f, tables, enabled, routes, tunnels
}

func paths(t *testing.T, ps ...df7.Path) []df7.Path {
	t.Helper()
	n, err := df7.NormalizePaths(ps)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTable(t *testing.T) {
	f, tables, _, _, _ := fakeMPLS()
	ctx := t.Context()
	d := NewTable(f, df7test.Owner, df7.WithIDRange(100, 199))
	v := df7test.Desired(d, df7.Encode(Table{ID: 101}))
	if v.Key != "mpls-table/101" || d.Dependencies(v.Value) != nil {
		t.Fatal(v.Key)
	}
	if _, err := d.Create(ctx, v.Value); err != nil {
		t.Fatal(err)
	}
	if tables[101] != "w0:101" {
		t.Fatalf("name %q", tables[101])
	}
	tables[102] = df7test.Other + ":102"
	tables[0] = ""
	df7test.AssertEmptyPlan(t, d, v)
	if _, err := d.Create(ctx, df7.Encode(Table{ID: 102})); !errors.Is(err, df7.ErrForeignInterface) {
		t.Fatalf("another owner's table: %v", err)
	}
	if _, err := d.Create(ctx, df7.Encode(Table{ID: 5})); !errors.Is(err, df7.ErrSpec) {
		t.Fatalf("outside the range: %v", err)
	}
	if err := d.Delete(ctx, df7.Encode(Table{ID: 102}), nil); err != nil || tables[102] == "" {
		t.Fatal("another owner's table must not be deleted", err)
	}
	if err := d.Delete(ctx, v.Value, nil); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, d)
}

func TestInterface(t *testing.T) {
	f, tables, enabled, _, _ := fakeMPLS()
	ctx := t.Context()
	d := NewInterface(f, df7test.Owner)
	v := df7test.Desired(d, df7.Encode(Interface{Interface: "loop0"}))
	deps := d.Dependencies(v.Value)
	if v.Key != "mpls-interface/loop0" || deps[0].Key != "interface/loop0" || deps[1].Key != "mpls-table/0" || !deps[1].Optional {
		t.Fatal(v.Key, deps)
	}
	if _, err := d.Create(ctx, v.Value); !df7.IsVPPError(err, api.NO_SUCH_FIB) {
		t.Fatalf("no table 0: %v", err)
	}
	tables[0] = ""
	if _, err := d.Create(ctx, v.Value); err != nil {
		t.Fatal(err)
	}
	enabled[3] = 1 // another owner's interface
	df7test.AssertEmptyPlan(t, d, v)
	if _, err := d.Create(ctx, v.Value); err != nil || enabled[1] != 1 {
		t.Fatal("a repeated Create never adds a second reference (D-076)", err, enabled[1])
	}
	enabled[1] = 2 // plus another consumer's reference (review L1: it survives our delete)
	if err := d.Delete(ctx, v.Value, nil); err != nil || enabled[1] != 1 {
		t.Fatalf("one disable: %v counter %d", err, enabled[1])
	}
	enabled[1] = 0
	if err := d.Delete(ctx, v.Value, nil); err != nil || enabled[1] != 0 {
		t.Fatalf("delete must not disable a disabled interface (u8 wrap): %v %d", err, enabled[1])
	}
	df7test.AssertEmptyPlan(t, d)
	// review M1: an enabled untagged interface is never adopted; our own enable claims it
	e := df7.Encode(Interface{Interface: "eth0"})
	enabled[4] = 1
	if _, err := d.Create(ctx, e); !errors.Is(err, dfkit.ErrNotOurs) {
		t.Fatalf("adopted: %v", err)
	}
	if err := d.Delete(ctx, e, nil); err != nil || enabled[4] != 1 {
		t.Fatal("never disable what is not ours", err)
	}
	enabled[4] = 0
	if _, err := d.Create(ctx, e); err != nil || enabled[4] != 1 || !df7test.Claimed(ctx, f, df7test.Owner, "eth0", "mpls-interface/eth0") {
		t.Fatal(err, enabled)
	}
	if err := d.Delete(ctx, e, nil); err != nil || enabled[4] != 0 || df7test.Claimed(ctx, f, df7test.Owner, "eth0", "mpls-interface/eth0") {
		t.Fatal(err, enabled)
	}
}

func TestRoute(t *testing.T) {
	f, tables, _, routes, _ := fakeMPLS()
	ctx := t.Context()
	tables[100] = "w0:100"
	tables[200] = df7test.Other + ":200"
	routes[rkey{200, 5000, 1}] = mpls.MplsRoute{MrTableID: 200, MrLabel: 5000, MrEos: 1}
	d := NewRoute(f, df7test.Owner)
	r := Route{Table: 100, Label: 1000, EOS: true, EOSProto: PayloadIP4,
		Paths: paths(t, df7.Path{Interface: "loop0", NextHop: "10.0.0.2", Labels: []df7.Label{{Label: 100}, {Label: 200, TTL: 64, Exp: 3}}})}
	v := df7test.Desired(d, df7.Encode(r))
	deps := d.Dependencies(v.Value)
	if v.Key != "mpls-route/100/1000/eos" || deps[0].Key != "mpls-table/100" || deps[1].Key != "interface/loop0" || !deps[1].Optional {
		t.Fatal(v.Key, deps)
	}
	if _, err := d.Create(ctx, v.Value); err != nil {
		t.Fatal(err)
	}
	req := df7test.Last[*mpls.MplsRouteAddDel](t, f, "mpls_route_add_del")
	p := req.MrRoute.MrPaths[0]
	if req.MrIsMultipath || p.SwIfIndex != 1 || p.NLabels != 2 || p.LabelStack[1].Label != 200 || p.LabelStack[1].TTL != 64 || p.Weight != 1 || p.Proto != fib_types.FIB_API_PATH_NH_PROTO_IP4 {
		t.Fatalf("%+v", req)
	}
	neos := Route{Table: 100, Label: 1001, Paths: paths(t, df7.Path{Type: df7.PathDrop})}
	if _, err := d.Create(ctx, df7.Encode(neos)); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, d, v, df7test.Desired(d, df7.Encode(neos)))

	r2 := r
	r2.Paths = paths(t, df7.Path{Interface: "loop0", NextHop: "10.0.0.2", Labels: []df7.Label{{Label: 100}}}, df7.Path{Interface: "loop1", NextHop: "10.0.1.2"})
	if _, err := d.Update(ctx, v.Value, df7.Encode(r2), nil); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, d, df7test.Desired(d, df7.Encode(r2)), df7test.Desired(d, df7.Encode(neos)))
	r3 := r2
	r3.EOSProto = PayloadIP6
	if _, err := d.Update(ctx, df7.Encode(r2), df7.Encode(r3), nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, df7.Encode(r2), nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, df7.Encode(r2), nil); err != nil { // already gone
		t.Fatal(err)
	}
	if err := d.Delete(ctx, df7.Encode(Route{Table: 200, Label: 5000, EOS: true}), nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := routes[rkey{200, 5000, 1}]; !ok {
		t.Fatal("a route in another owner's table was deleted")
	}
	for i, bad := range []Route{
		{Table: 100, Label: 15, Paths: neos.Paths},
		{Table: 100, Label: 1000, EOS: true, Paths: neos.Paths},
		{Table: 100, Label: 1000, EOSProto: PayloadIP4, Paths: neos.Paths},
		{Table: 100, Label: 1000},
		{Table: 100, Label: 1000, Paths: []df7.Path{{NextHop: "10.0.0.1"}}}, // not canonical
	} {
		if err := bad.Validate(); !errors.Is(err, df7.ErrSpec) {
			t.Errorf("case %d: %v", i, err)
		}
	}
}

// Review H2: in the shared table 0 (owned by the globals owner, D-071) only labels this owner
// added itself are reported, updated or deleted — never SR-MPLS BSIDs, mpls-ip-bind local labels
// or FRR / linux-cp labels of other features.
func TestRouteSharedTable0(t *testing.T) {
	f, tables, _, routes, _ := fakeMPLS()
	ctx := t.Context()
	df7.SetBootStore(df7test.Owner, nil)
	tables[0] = df7test.Owner + ":0" // this owner is the globals owner of table 0
	foreign := map[string]rkey{
		"sr-mpls bsid": {0, 20001, 1},
		"mpls-ip-bind": {0, 24001, 1},
		"frr/linux-cp": {0, 30001, 0},
		"reserved":     {0, 0, 1},
	}
	for _, k := range foreign {
		routes[k] = mpls.MplsRoute{MrTableID: 0, MrLabel: k.label, MrEos: k.eos}
	}
	d := NewRoute(f, df7test.Owner)
	if kvs, err := d.Retrieve(ctx); err != nil || len(kvs) != 0 {
		t.Fatalf("foreign labels in table 0 reported: %v %v", df7test.Keys(kvs), err)
	}
	drop := paths(t, df7.Path{Type: df7.PathDrop})
	for what, k := range foreign {
		if k.label < 16 {
			continue
		}
		r := Route{Table: 0, Label: k.label, EOS: k.eos == 1, Paths: drop}
		if k.eos == 1 {
			r.EOSProto = PayloadIP4
		}
		if _, err := d.Create(ctx, df7.Encode(r)); !errors.Is(err, dfkit.ErrNotOurs) {
			t.Fatalf("%s: Create must refuse an existing label: %v", what, err)
		}
		if _, err := d.Update(ctx, df7.Encode(r), df7.Encode(r), nil); !errors.Is(err, dfkit.ErrNotOurs) {
			t.Fatalf("%s: Update must refuse: %v", what, err)
		}
		if err := d.Delete(ctx, df7.Encode(r), nil); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	for what, k := range foreign {
		if _, ok := routes[k]; !ok {
			t.Fatalf("%s entry in table 0 was deleted", what)
		}
	}
	ours := Route{Table: 0, Label: 1600, EOS: true, EOSProto: PayloadIP4, Paths: drop}
	v := df7test.Desired(d, df7.Encode(ours))
	if _, err := d.Create(ctx, v.Value); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, v.Value); err != nil {
		t.Fatal("re-create of our own recorded label", err)
	}
	df7test.AssertEmptyPlan(t, d, v)
	if _, err := d.Update(ctx, v.Value, v.Value, nil); err != nil {
		t.Fatal(err)
	}
	gone := Route{Table: 0, Label: 1602, EOS: true, EOSProto: PayloadIP4, Paths: drop}
	if _, err := d.Create(ctx, df7.Encode(gone)); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, df7.Encode(gone), nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := routes[rkey{0, 1602, 1}]; ok {
		t.Fatal("our own recorded label must be deleted")
	}
	routes[rkey{0, 1602, 1}] = mpls.MplsRoute{MrTableID: 0, MrLabel: 1602, MrEos: 1} // re-used by another feature
	if err := d.Delete(ctx, df7.Encode(gone), nil); err != nil || len(routes) == 0 {
		t.Fatal(err)
	}
	if _, ok := routes[rkey{0, 1602, 1}]; !ok {
		t.Fatal("the record is dropped with our delete: the label is not ours any more")
	}
	// a failed add records nothing
	f.On("mpls_route_add_del", func(api.Message) ([]api.Message, error) {
		return []api.Message{&mpls.MplsRouteAddDelReply{Retval: -1}}, nil
	})
	failed := Route{Table: 0, Label: 1601, EOS: true, EOSProto: PayloadIP4, Paths: drop}
	if _, err := d.Create(ctx, df7.Encode(failed)); err == nil {
		t.Fatal("failed add must surface")
	}
	routes[rkey{0, 1601, 1}] = mpls.MplsRoute{MrTableID: 0, MrLabel: 1601, MrEos: 1} // someone else's, later
	if kvs, _ := d.Retrieve(ctx); len(kvs) != 1 || kvs[0].Key != v.Key {             // not 1601, not 1602
		t.Fatalf("only our label: %v", df7test.Keys(kvs))
	}
	// a VPP restart (same PID, new start time): the record is of an earlier instance
	f.RestartSamePID()
	if kvs, _ := d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("a record of an earlier VPP instance must not match: %v", df7test.Keys(kvs))
	}
}

func TestTunnelAndBind(t *testing.T) {
	f, _, _, _, tunnels := fakeMPLS()
	ctx := t.Context()
	d := NewTunnel(f, df7test.Owner)
	tn := Tunnel{Name: "mt1", Paths: paths(t, df7.Path{Interface: "loop0", NextHop: "10.0.0.2", Labels: []df7.Label{{Label: 500}}},
		df7.Path{Interface: "loop1", NextHop: "10.0.1.2", Labels: []df7.Label{{Label: 501}}})}
	v := df7test.Desired(d, df7.Encode(tn))
	if v.Key != "mpls-tunnel/mt1" || len(d.Dependencies(v.Value)) != 2 {
		t.Fatal(v.Key)
	}
	meta, err := d.Create(ctx, v.Value)
	if err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*interfaces.SwInterfaceTagAddDel](t, f, "sw_interface_tag_add_del"); r.Tag != "w0:mt1" || uint32(r.SwIfIndex) != meta.(TunnelMeta).SwIfIndex {
		t.Fatalf("tunnel interface tag %+v", r)
	}
	tunnels[99] = &mpls.MplsTunnel{MtSwIfIndex: 99, MtTag: df7test.Other + ":mt1"}
	df7test.AssertEmptyPlan(t, d, v)
	// a duplicate of ours (lost reply + retry) is reported under a never-desired key → deleted
	dup := *tunnels[meta.(TunnelMeta).SwIfIndex]
	dup.MtSwIfIndex = 77
	tunnels[77] = &dup
	kvs, _ := d.Retrieve(ctx)
	if len(kvs) != 2 || kvs[1].Key != "mpls-tunnel/mt1#77" && kvs[0].Key != "mpls-tunnel/mt1#77" {
		t.Fatalf("dup: %v", df7test.Keys(kvs))
	}
	for _, kv := range kvs { // the scheduler deletes the never-desired duplicate key
		if kv.Key == "mpls-tunnel/mt1#77" {
			if err := d.Delete(ctx, kv.Value, kv.Meta); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, ok := tunnels[77]; ok {
		t.Fatal("duplicate not deleted")
	}
	df7test.AssertEmptyPlan(t, d, v)
	if _, err := d.Update(ctx, v.Value, v.Value, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, v.Value, nil); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*mpls.MplsTunnelAddDel](t, f, "mpls_tunnel_add_del"); r.MtIsAdd || r.MtTunnel.MtNPaths != 2 {
		t.Fatalf("delete must carry all paths: %+v", r)
	}
	df7test.AssertEmptyPlan(t, d)
	if _, ok := tunnels[99]; !ok {
		t.Fatal("another owner's tunnel deleted")
	}

	bd := NewIPBind(f, df7test.Owner)
	b := df7.Encode(IPBind{MPLSTable: 100, Label: 2000, VRF: 7, Prefix: "10.0.0.0/24"})
	deps := bd.Dependencies(b)
	if bd.KeyOf(b) != "mpls-ip-bind/100/2000/7/10.0.0.0/24" || deps[0].Key != "mpls-table/100" || deps[1].Key != "vrf/7" {
		t.Fatal(bd.KeyOf(b), deps)
	}
	if _, err := bd.Create(ctx, b); err != nil || !df7test.Last[*mpls.MplsIPBindUnbind](t, f, "mpls_ip_bind_unbind").MbIsBind {
		t.Fatal(err)
	}
	if err := bd.Delete(ctx, b, nil); err != nil || df7test.Last[*mpls.MplsIPBindUnbind](t, f, "mpls_ip_bind_unbind").MbIsBind {
		t.Fatal(err)
	}
	if _, err := bd.Retrieve(ctx); !errors.Is(err, df7.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	r := scheduler.NewRegistry()
	Register(r, f, df7test.Owner)
	if r.Len() != 5 || fmt.Sprint(r.Names()[0]) != NameTable {
		t.Fatal(r.Names())
	}
}
