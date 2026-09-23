package vrrp

import (
	"testing"
	"time"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Host test: VRs on this slot's loopback (loop<slot>70, 10.<slot>.70.1/24), VR ids = slot and
// slot+1, tracking loop<slot>71. VRs are started on the loopback: adverts never leave the host.
// Assertions cover configuration and state shape, not protocol behaviour.
func TestVRRPOnHost(t *testing.T) {
	h := df7test.StartHost(t)
	vd, pd, td, sd := NewVR(h.C, h.Owner), NewPeers(h.C, h.Owner), NewTrack(h.C, h.Owner), NewState(h.C, h.Owner)
	ifA, idxA := h.Loopback(70, true, true)
	ifT, _ := h.Loopback(71, true, true)
	h.Address(idxA, h.Addr(70, 1)+"/24")
	h.CleanupOwned(vd)
	h.CleanupOwned(sd) // registered last → stops VRs first in Cleanup

	events, err := WatchEvents(h.Ctx, h.C, h.Owner)
	h.Must("want_vrrp_vr_events", err)

	id := uint8(h.Slot) //nolint:gosec // slots are 1–12
	v1 := VR{Interface: ifA, VRID: id}
	v2 := VR{Interface: ifA, VRID: id + 1}
	vrs := []scheduler.KV{
		df7test.Desired(vd, df7.Encode(VRSpec{VR: v1, Priority: 100, Interval: 100, Preempt: true, Addresses: []string{h.Addr(70, 254)}})),
		df7test.Desired(vd, df7.Encode(VRSpec{VR: v2, Priority: 50, Interval: 200, Unicast: true, Accept: true, Addresses: []string{h.Addr(70, 252), h.Addr(70, 253)}})),
	}
	peers := []scheduler.KV{df7test.Desired(pd, df7.Encode(Peers{VR: v2, Peers: []string{h.Addr(70, 2)}}))}
	tracks := []scheduler.KV{df7test.Desired(td, df7.Encode(Track{VR: v1, Tracked: ifT, Priority: 20}))}
	states := []scheduler.KV{df7test.Desired(sd, df7.Encode(State{VR: v1, Running: true})), df7test.Desired(sd, df7.Encode(State{VR: v2, Running: true}))}

	cv := h.Apply(vd, vrs...)
	cp := h.Apply(pd, peers...)
	ct := h.Apply(td, tracks...)
	cs := h.Apply(sd, states...)
	h.ExpectRetrieved(vd, vrs...)
	h.ExpectRetrieved(pd, peers...)
	h.ExpectRetrieved(td, tracks...)
	h.ExpectRetrieved(sd, states...)

	t.Run("events", func(t *testing.T) {
		select {
		case e := <-events:
			t.Logf("vrrp event: %+v", e)
		case <-time.After(3 * time.Second):
			t.Fatal("no vrrp_vr_event after starting two VRs")
		}
	})

	t.Run("peers update on a running VR", func(*testing.T) {
		p := df7.Encode(Peers{VR: v2, Peers: []string{h.Addr(70, 2), h.Addr(70, 3)}})
		_, err := pd.Update(h.Ctx, peers[0].Value, p, cp[0].Meta)
		h.Must("peers update", err)
		peers[0].Value, cp[0].Value = p, p
		h.ExpectRetrieved(pd, peers...)
		h.ExpectRetrieved(sd, states...) // restarted after the change
	})

	t.Run("track priority update", func(*testing.T) {
		n := df7.Encode(Track{VR: v1, Tracked: ifT, Priority: 30})
		m, err := td.Update(h.Ctx, tracks[0].Value, n, ct[0].Meta)
		h.Must("track update", err)
		tracks[0].Value, ct[0].Value, ct[0].Meta = n, n, m
		h.ExpectRetrieved(td, tracks...)
	})

	st, err := States(h.Ctx, h.C, h.Owner)
	h.Must("states", err)
	t.Logf("live state: %+v", st)
	h.Hold("vrrp VRs")

	t.Run("restart simulation + update without a pool index", func(t *testing.T) {
		c := df7test.Connect(t)
		fresh := NewVR(c, h.Owner)
		kvs := h.ExpectRetrieved(fresh, vrs...)
		h.ExpectRetrieved(NewPeers(c, h.Owner), peers...)
		h.ExpectRetrieved(NewTrack(c, h.Owner), tracks...)
		h.ExpectRetrieved(NewState(c, h.Owner), states...)
		var meta any
		for _, kv := range kvs {
			if kv.Key == vrs[0].Key {
				meta = kv.Meta
			}
		}
		n := df7.Encode(VRSpec{VR: v1, Priority: 120, Interval: 100, Preempt: true, Addresses: []string{h.Addr(70, 250), h.Addr(70, 254)}})
		m, err := fresh.Update(h.Ctx, vrs[0].Value, n, meta)
		h.Must("update after restart (pool walk)", err)
		t.Logf("pool index found by the walk: %+v (Create returned %+v)", m, cv[0].Meta)
		if m.(Meta).Index != cv[0].Meta.(Meta).Index {
			t.Fatalf("walk found index %d, Create had %d", m.(Meta).Index, cv[0].Meta.(Meta).Index)
		}
		vrs[0].Value, cv[0].Value = n, n
		h.ExpectRetrieved(fresh, vrs...)
	})

	t.Run("restart simulation with loss (state, tracking)", func(*testing.T) {
		h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return NewState(c, h.Owner) }, states...)
		h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return NewTrack(c, h.Owner) }, tracks...)
	})

	h.DeleteAll(sd, cs)
	h.ExpectNone(sd)
	h.DeleteAll(td, ct)
	h.DeleteAll(pd, cp)
	t.Run("restart simulation with loss (VRs, after their children are gone)", func(*testing.T) {
		bare := []scheduler.KV{vrs[0]} // v2 is unicast: recreated without peers it is still a valid VR
		h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return NewVR(c, h.Owner) }, append(bare, vrs[1])...)
	})
	h.DeleteAll(vd, cv)
	for _, d := range []scheduler.Descriptor{td, pd, vd} {
		h.ExpectNone(d)
	}
}
