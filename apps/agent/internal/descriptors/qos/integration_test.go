package qos

import (
	"testing"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Host test: egress map ids from this slot's range (VRX_VPP_TABLE_BASE+1…), record/store/mark on
// this slot's loopbacks (loop<slot>20…); Retrieve only reports this owner's objects.
func TestQoSOnHost(t *testing.T) {
	h := df7test.StartHost(t)
	opts := []df7.Option{df7.WithIDRange(h.TableB, h.TableB+999)}
	md := NewEgressMap(h.C, h.Owner, opts...)
	rd := NewRecord(h.C, h.Owner, opts...)
	sd := NewStore(h.C, h.Owner, opts...)
	kd := NewMark(h.C, h.Owner, opts...)
	ifA, _ := h.Loopback(20, true, true)
	ifB, _ := h.Loopback(21, true, true)
	for _, d := range []scheduler.Descriptor{md, kd, sd, rd} {
		h.CleanupOwned(d) // registered in this order → Cleanup runs record, store, mark, map
	}

	ipRow := make([]int, 256)
	for i := range ipRow {
		ipRow[i] = (i >> 2) & 0x3f // DSCP from TOS: identity-ish, non-zero
	}
	vlanRow := make([]int, 256)
	for i := range vlanRow {
		vlanRow[i] = i % 8
	}
	m1 := EgressMap{ID: h.TableB + 1, IP: ipRow, VLAN: vlanRow}
	m2 := EgressMap{ID: h.TableB + 2, MPLS: vlanRow}
	maps := []scheduler.KV{df7test.Desired(md, df7.Encode(m1)), df7test.Desired(md, df7.Encode(m2))}
	recs := []scheduler.KV{df7test.Desired(rd, df7.Encode(Record{Interface: ifA, Source: SourceIP})), df7test.Desired(rd, df7.Encode(Record{Interface: ifA, Source: SourceVLAN}))}
	stores := []scheduler.KV{df7test.Desired(sd, df7.Encode(Store{Interface: ifB, Source: SourceIP, Value: 46}))}
	marks := []scheduler.KV{df7test.Desired(kd, df7.Encode(Mark{Interface: ifB, Source: SourceIP, Map: m1.ID}))}

	cm := h.Apply(md, maps...)
	cr := h.Apply(rd, recs...)
	cs := h.Apply(sd, stores...)
	ck := h.Apply(kd, marks...)
	h.ExpectRetrieved(md, maps...)
	h.ExpectRetrieved(rd, recs...)
	h.ExpectRetrieved(sd, stores...)
	h.ExpectRetrieved(kd, marks...)

	t.Run("update in place", func(t *testing.T) {
		m1b := m1
		m1b.VLAN = nil
		_, err := md.Update(h.Ctx, maps[0].Value, df7.Encode(m1b), nil)
		h.Must("egress map update", err)
		maps[0].Value = df7.Encode(m1b)
		h.ExpectRetrieved(md, maps...)
		mk := Mark{Interface: ifB, Source: SourceIP, Map: m2.ID}
		meta, err := kd.Update(h.Ctx, marks[0].Value, df7.Encode(mk), ck[0].Meta)
		h.Must("mark update", err)
		marks[0].Value, ck[0].Value, ck[0].Meta = df7.Encode(mk), df7.Encode(mk), meta
		h.ExpectRetrieved(kd, marks...)
	})

	h.Hold("qos objects")

	t.Run("restart simulation", func(t *testing.T) {
		h.ExpectRetrieved(NewEgressMap(df7test.Connect(t), h.Owner, opts...), maps...)
		h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return NewRecord(c, h.Owner, opts...) }, recs...)
		h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return NewStore(c, h.Owner, opts...) }, stores...)
		h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return NewMark(c, h.Owner, opts...) }, marks...)
	})

	h.DeleteAll(kd, ck)
	h.DeleteAll(sd, cs)
	h.DeleteAll(rd, cr)
	t.Run("restart simulation (maps, after their marks are gone)", func(t *testing.T) {
		h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return NewEgressMap(c, h.Owner, opts...) }, maps...)
	})
	h.DeleteAll(md, cm)
	for _, d := range []scheduler.Descriptor{kd, sd, rd, md} {
		h.ExpectNone(d)
	}
}
