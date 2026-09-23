package qos

import (
	"errors"
	"sort"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/qos"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
)

type key struct {
	idx uint32
	src qos.QosSource
}

// fakeQoS models VPP's QoS state: reference-counted record/store, egress maps, marks.
func fakeQoS() (*df7test.Fake, map[key]int, map[uint32]qos.QosEgressMap) {
	f := df7test.NewFake()
	recs := map[key]int{}
	stores := map[key]uint8{}
	maps := map[uint32]qos.QosEgressMap{}
	marks := map[key]uint32{}
	f.On("qos_record_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*qos.QosRecordEnableDisable)
		k := key{uint32(r.Record.SwIfIndex), r.Record.InputSource}
		if r.Enable {
			recs[k]++
		} else {
			if recs[k] == 0 {
				return []api.Message{&qos.QosRecordEnableDisableReply{Retval: int32(api.VALUE_EXIST)}}, nil
			}
			recs[k]--
		}
		return []api.Message{&qos.QosRecordEnableDisableReply{}}, nil
	})
	f.On("qos_record_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for k, n := range recs {
			if n > 0 {
				out = append(out, &qos.QosRecordDetails{Record: qos.QosRecord{SwIfIndex: interface_types.InterfaceIndex(k.idx), InputSource: k.src}})
			}
		}
		return out, nil
	})
	storeN := map[key]int{}
	f.On("qos_store_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*qos.QosStoreEnableDisable)
		k := key{uint32(r.Store.SwIfIndex), r.Store.InputSource}
		if r.Enable {
			if storeN[k] == 0 {
				stores[k] = r.Store.Value
			}
			storeN[k]++
		} else {
			if storeN[k] == 0 {
				return []api.Message{&qos.QosStoreEnableDisableReply{Retval: int32(api.VALUE_EXIST)}}, nil
			}
			storeN[k]--
		}
		return []api.Message{&qos.QosStoreEnableDisableReply{}}, nil
	})
	f.On("qos_store_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for k, n := range storeN {
			if n > 0 {
				out = append(out, &qos.QosStoreDetails{Store: qos.QosStore{SwIfIndex: interface_types.InterfaceIndex(k.idx), InputSource: k.src, Value: stores[k]}})
			}
		}
		return out, nil
	})
	f.On("qos_egress_map_update", func(m api.Message) ([]api.Message, error) {
		r := m.(*qos.QosEgressMapUpdate)
		maps[r.Map.ID] = r.Map
		return []api.Message{&qos.QosEgressMapUpdateReply{}}, nil
	})
	f.On("qos_egress_map_delete", func(m api.Message) ([]api.Message, error) {
		delete(maps, m.(*qos.QosEgressMapDelete).ID)
		return []api.Message{&qos.QosEgressMapDeleteReply{}}, nil
	})
	f.On("qos_egress_map_dump", func(api.Message) ([]api.Message, error) {
		ids := make([]uint32, 0, len(maps))
		for id := range maps {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
		var out []api.Message
		for _, id := range ids {
			out = append(out, &qos.QosEgressMapDetails{Map: maps[id]})
		}
		return out, nil
	})
	f.On("qos_mark_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*qos.QosMarkEnableDisable)
		k := key{r.Mark.SwIfIndex, r.Mark.OutputSource}
		if r.Enable {
			if _, ok := maps[r.Mark.MapID]; !ok {
				return []api.Message{&qos.QosMarkEnableDisableReply{Retval: int32(api.NO_SUCH_TABLE)}}, nil
			}
			marks[k] = r.Mark.MapID
		} else {
			if _, ok := marks[k]; !ok {
				return []api.Message{&qos.QosMarkEnableDisableReply{Retval: int32(api.VALUE_EXIST)}}, nil
			}
			delete(marks, k)
		}
		return []api.Message{&qos.QosMarkEnableDisableReply{}}, nil
	})
	f.On("qos_mark_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for k, id := range marks {
			out = append(out, &qos.QosMarkDetails{Mark: qos.QosMark{SwIfIndex: k.idx, OutputSource: k.src, MapID: id}})
		}
		return out, nil
	})
	return f, recs, maps
}

func row(v int) []int {
	r := make([]int, 256)
	for i := range r {
		r[i] = (i + v) % 64
	}
	return r
}

func TestRecordStore(t *testing.T) {
	f, recs, _ := fakeQoS()
	ctx := t.Context()
	rd := NewRecord(f, df7test.Owner)
	sd := NewStore(f, df7test.Owner)
	r := df7test.Desired(rd, df7.Encode(Record{Interface: "loop0", Source: SourceIP}))
	if r.Key != "qos.record/loop0/ip" || rd.Dependencies(r.Value)[0].Key != "interface/loop0" {
		t.Fatalf("key/deps %s", r.Key)
	}
	meta, err := rd.Create(ctx, r.Value)
	if err != nil {
		t.Fatal(err)
	}
	if q := df7test.Last[*qos.QosRecordEnableDisable](t, f, "qos_record_enable_disable"); !q.Enable || q.Record.SwIfIndex != 1 || q.Record.InputSource != qos.QOS_API_SOURCE_IP {
		t.Fatalf("%+v", q)
	}
	// another owner's record is never reported
	recs[key{3, qos.QOS_API_SOURCE_VLAN}] = 1
	df7test.AssertEmptyPlan(t, rd, r)
	if _, err := rd.Update(ctx, r.Value, r.Value, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	// an earlier run enabled twice: Delete still leaves nothing
	if _, err := rd.Create(ctx, r.Value); err != nil {
		t.Fatal(err)
	}
	if err := rd.Delete(ctx, r.Value, meta); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, rd)

	s := df7test.Desired(sd, df7.Encode(Store{Interface: "loop1", Source: SourceIP, Value: 46}))
	sm, err := sd.Create(ctx, s.Value)
	if err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, sd, s)
	if _, err := sd.Update(ctx, s.Value, s.Value, sm); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := sd.Delete(ctx, s.Value, sm); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, sd)
	if _, err := sd.Create(ctx, df7.Encode(Store{Interface: "loop1", Source: SourceVLAN})); !errors.Is(err, df7.ErrSpec) {
		t.Fatalf("store vlan: %v", err)
	}
	if _, err := rd.Create(ctx, df7.Encode(Record{Interface: "loop9", Source: SourceIP})); !errors.Is(err, df7.ErrForeignInterface) {
		t.Fatalf("foreign: %v", err)
	}
	if _, err := rd.Create(ctx, df7.Encode(Record{Interface: "loop0", Source: "pcp"})); !errors.Is(err, df7.ErrSpec) {
		t.Fatalf("bad source: %v", err)
	}
	// deleting what is already gone is success; Delete never trusts the Meta index (D-071)
	if err := rd.Delete(ctx, r.Value, Meta{SwIfIndex: 3}); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.CallsNamed("qos_record_enable_disable") {
		if c.(*qos.QosRecordEnableDisable).Record.SwIfIndex == 3 {
			t.Fatal("a stale Meta index reached VPP")
		}
	}
	// untagged interface: owned only through the claim of this object's key
	eth := df7test.Desired(rd, df7.Encode(Record{Interface: "eth0", Source: SourceMPLS}))
	em, err := rd.Create(ctx, eth.Value)
	if err != nil {
		t.Fatal(err)
	}
	recs[key{4, qos.QOS_API_SOURCE_VLAN}] = 1 // on eth0 but not claimed by us
	df7test.AssertEmptyPlan(t, rd, eth)
	delete(recs, key{4, qos.QOS_API_SOURCE_VLAN})
	if err := rd.Delete(ctx, eth.Value, em); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, rd)
}

func TestEgressMapAndMark(t *testing.T) {
	f, _, maps := fakeQoS()
	ctx := t.Context()
	md := NewEgressMap(f, df7test.Owner, df7.WithIDRange(100, 199))
	kd := NewMark(f, df7test.Owner)
	m := EgressMap{ID: 101, IP: row(1), MPLS: row(2)}
	mkv := df7test.Desired(md, df7.Encode(m))
	if mkv.Key != "qos.egress-map/101" || md.Dependencies(mkv.Value) != nil {
		t.Fatal(mkv.Key)
	}
	if _, err := md.Create(ctx, mkv.Value); err != nil {
		t.Fatal(err)
	}
	req := df7test.Last[*qos.QosEgressMapUpdate](t, f, "qos_egress_map_update")
	if req.Map.ID != 101 || req.Map.Rows[3].Outputs[5] != 6 || req.Map.Rows[0].Outputs[5] != 0 || len(req.Map.Rows[1].Outputs) != 256 {
		t.Fatalf("%+v", req.Map.Rows[3].Outputs[:8])
	}
	maps[5] = qos.QosEgressMap{ID: 5} // outside the owned range
	df7test.AssertEmptyPlan(t, md, mkv)
	if _, err := md.Create(ctx, df7.Encode(EgressMap{ID: 5, IP: row(1)})); !errors.Is(err, df7.ErrSpec) {
		t.Fatalf("id outside range: %v", err)
	}

	mk := df7test.Desired(kd, df7.Encode(Mark{Interface: "loop1", Source: SourceIP, Map: 101}))
	deps := kd.Dependencies(mk.Value)
	if len(deps) != 2 || deps[1].Key != "qos.egress-map/101" {
		t.Fatalf("deps %v", deps)
	}
	meta, err := kd.Create(ctx, mk.Value)
	if err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, kd, mk)

	// update map rows and the mark's map in place
	m.MPLS = nil
	if _, err := md.Update(ctx, mkv.Value, df7.Encode(m), nil); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, md, df7test.Desired(md, df7.Encode(m)))
	if _, err := md.Create(ctx, df7.Encode(EgressMap{ID: 102, IP: row(0)})); err != nil {
		t.Fatal(err)
	}
	mk2 := df7.Encode(Mark{Interface: "loop1", Source: SourceIP, Map: 102})
	if _, err := kd.Update(ctx, mk.Value, mk2, meta); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, kd, df7test.Desired(kd, mk2))
	if _, err := kd.Update(ctx, mk2, df7.Encode(Mark{Interface: "loop1", Source: SourceMPLS, Map: 102}), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := kd.Delete(ctx, mk2, meta); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, kd)
	if err := md.Delete(ctx, df7.Encode(m), nil); err != nil {
		t.Fatal(err)
	}
	if kvs, _ := md.Retrieve(ctx); len(kvs) != 1 || kvs[0].Key != "qos.egress-map/102" {
		t.Fatalf("left %v", df7test.Keys(kvs))
	}
	// validation
	for i, bad := range []EgressMap{{ID: 1, IP: []int{1}}, {ID: 1, IP: make([]int, 256)}, {ID: 1, IP: append(row(0)[:255], 300)}} {
		if err := bad.Validate(); !errors.Is(err, df7.ErrSpec) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	// VPP error surfaces
	if _, err := kd.Create(ctx, df7.Encode(Mark{Interface: "loop0", Source: SourceIP, Map: 999})); !df7.IsVPPError(err, api.NO_SUCH_TABLE) {
		t.Fatalf("missing map: %v", err)
	}
	f.Fail("qos_mark_dump", errors.New("boom"))
	if _, err := kd.Retrieve(ctx); err == nil {
		t.Fatal("dump error must surface")
	}
}

func TestRegister(t *testing.T) {
	r := scheduler.NewRegistry()
	Register(r, df7test.NewFake(), df7test.Owner)
	if r.Len() != 4 {
		t.Fatal(r.Names())
	}
}
