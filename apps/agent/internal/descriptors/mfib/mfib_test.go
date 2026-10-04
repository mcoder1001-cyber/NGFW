package mfib

import (
	"context"
	"fmt"
	"go.fd.io/govpp/api"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/mfib_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"testing"
)

func fixture(t *testing.T) (*Descriptor, *dfkittest.FakeVPP, *[]ip.IPMroute, *int) {
	t.Helper()
	f := dfkittest.NewFake(dfkittest.Iface{Index: 1, Name: "in", Tag: "w1:in"}, dfkittest.Iface{Index: 2, Name: "out", Tag: "w1:out"})
	rows := []ip.IPMroute{}
	writes := 0
	f.On("ip_mtable_dump", func(api.Message) ([]api.Message, error) {
		return []api.Message{&ip.IPMtableDetails{Table: ip.IPTable{TableID: 1001, Name: "w1:test"}}}, nil
	})
	f.On("ip_mroute_dump", func(m api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, r := range rows {
			if r.TableID == m.(*ip.IPMrouteDump).Table.TableID {
				out = append(out, &ip.IPMrouteDetails{Route: r})
			}
		}
		return out, nil
	})
	f.On("ip_mroute_add_del", func(m api.Message) ([]api.Message, error) {
		writes++
		req := m.(*ip.IPMrouteAddDel)
		key, _ := rawKey(req.Route)
		var next []ip.IPMroute
		for _, r := range rows {
			k, _ := rawKey(r)
			if k != key {
				next = append(next, r)
			}
		}
		if req.IsAdd {
			next = append(next, req.Route)
		}
		rows = next
		return []api.Message{&ip.IPMrouteAddDelReply{}}, nil
	})
	return New(f, "w1", dfkit.NewMemoryBootStore(), df7.WithIDRange(1000, 1999)), f, &rows, &writes
}
func sample() Route {
	return Route{Table: 1001, Group: "239.1.2.3", Source: "10.1.0.1", Paths: []Path{{"in", "accept"}, {"out", "forward"}}}
}
func TestLifecycleOwnershipRestart(t *testing.T) {
	d, f, rows, writes := fixture(t)
	ctx := context.Background()
	r := sample()
	if _, e := d.Create(ctx, df7.Encode(r)); e != nil {
		t.Fatal(e)
	}
	got, e := d.Retrieve(ctx)
	if e != nil || len(got) != 1 {
		t.Fatalf("retrieve %v %v", got, e)
	}
	if _, e = d.Create(ctx, df7.Encode(r)); e != nil {
		t.Fatal("reassert own", e)
	}
	if e = d.Delete(ctx, df7.Encode(r), nil); e != nil {
		t.Fatal(e)
	}
	before := *writes
	if e = d.Delete(ctx, df7.Encode(r), nil); e != nil || *writes != before {
		t.Fatal("missing delete wrote", e)
	}
	if len(*rows) != 0 {
		t.Fatal("rollback left route")
	}
	if _, e = d.Create(ctx, df7.Encode(r)); e != nil {
		t.Fatal(e)
	}
	f.RestartVPP()
	if _, e = d.Create(ctx, df7.Encode(r)); e == nil {
		t.Fatal("adopted surviving foreign route after new boot")
	}
	*rows = nil
	if _, e = d.Create(ctx, df7.Encode(r)); e != nil {
		t.Fatal("rebuild lost route", e)
	}
}
func TestForeignUndecodableRouteCannotBeAdopted(t *testing.T) {
	d, _, rows, writes := fixture(t)
	raw, e := d.encode(context.Background(), sample())
	if e != nil {
		t.Fatal(e)
	}
	raw.Paths[0].ItfFlags = mfib_types.MFIB_API_ITF_FLAG_ACCEPT | mfib_types.MFIB_API_ITF_FLAG_FORWARD
	raw.Paths[0].Path.SwIfIndex = 999
	*rows = []ip.IPMroute{raw}
	if _, e = d.Create(context.Background(), df7.Encode(sample())); e == nil || *writes != 0 {
		t.Fatalf("foreign overwritten: %v writes=%d", e, *writes)
	}
}
func TestTableGuardAndWriteFailure(t *testing.T) {
	d, f, _, writes := fixture(t)
	r := sample()
	r.Table = 2000
	if _, e := d.Create(context.Background(), df7.Encode(r)); e == nil || *writes != 0 {
		t.Fatal("out of range wrote")
	}
	r.Table = 1002
	if _, e := d.Create(context.Background(), df7.Encode(r)); e == nil {
		t.Fatal("missing table accepted")
	}
	f.On("ip_mroute_add_del", func(api.Message) ([]api.Message, error) { return nil, fmt.Errorf("write failed") })
	r = sample()
	if _, e := d.Create(context.Background(), df7.Encode(r)); e == nil {
		t.Fatal("failed write accepted")
	}
	if _, ok := d.Store.Get(string(Key(r))); ok {
		t.Fatal("failed create retained claim")
	}
}
func TestRouteValidation(t *testing.T) {
	r := sample()
	r.Paths = []Path{{"out", "forward"}}
	if e := r.Validate(); e != nil {
		t.Fatal("valid forward only contract", e)
	}
	cases := []Route{sample(), sample(), sample(), sample()}
	cases[0].Group = "224.0.0.1"
	cases[1].Source = "239.1.2.4"
	cases[2].Paths = []Path{{"in", "accept"}, {"out", "accept"}}
	cases[3].Paths = []Path{{"in", "accept"}, {"in", "forward"}}
	for _, v := range cases {
		if e := v.Validate(); e == nil {
			t.Fatalf("accepted invalid %+v", v)
		}
	}
}

func TestFailedCreateRestoresStaleClaim(t *testing.T) {
	d, f, _, _ := fixture(t)
	r := sample()
	old := dfkit.BootRecord{Key: string(Key(r)), Identity: "prior-boot", Value: "old"}
	if e := d.Store.Put(old); e != nil {
		t.Fatal(e)
	}
	f.On("ip_mroute_add_del", func(api.Message) ([]api.Message, error) { return nil, fmt.Errorf("write failed") })
	if _, e := d.Create(context.Background(), df7.Encode(r)); e == nil {
		t.Fatal("failed create succeeded")
	}
	got, ok := d.Store.Get(old.Key)
	if !ok || got != old {
		t.Fatalf("promoted stale claim: %+v", got)
	}
}

func TestMultipleRoutesRollbackAndIndependentGroups(t *testing.T) {
	d, _, rows, _ := fixture(t)
	ctx := context.Background()
	a := sample()
	b := sample()
	b.Group = "239.1.2.4"
	for _, r := range []Route{a, b} {
		if _, e := d.Create(ctx, df7.Encode(r)); e != nil {
			t.Fatal(e)
		}
	}
	if len(*rows) != 2 {
		t.Fatalf("routes %d", len(*rows))
	}
	if e := d.Delete(ctx, df7.Encode(a), nil); e != nil {
		t.Fatal(e)
	}
	got, e := d.Retrieve(ctx)
	if e != nil || len(got) != 1 || got[0].Key != Key(b) {
		t.Fatalf("deleted sibling: %v %v", got, e)
	}
}
