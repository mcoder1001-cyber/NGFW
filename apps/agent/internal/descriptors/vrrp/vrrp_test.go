package vrrp

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/vrrp"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
)

type vr struct {
	det    *vrrp.VrrpVrDetails
	peers  []ip_types.Address
	tracks []vrrp.VrrpVrTrackIf
}

// fakeVRRP models the plugin: a pool of VRs (vrrp_vr_update / add_del / dump), peers, tracking
// and start/stop, with VPP's rules (no peers change while running, key check on update).
func fakeVRRP() (*df7test.Fake, map[uint32]*vr) {
	f := df7test.NewFake()
	pool := map[uint32]*vr{}
	find := func(idx interface_types.InterfaceIndex, id uint8, v6 bool) (uint32, *vr) {
		for i, v := range pool {
			if v.det.Config.SwIfIndex == idx && v.det.Config.VrID == id && (v.det.Config.Flags&vrrp.VRRP_API_VR_IPV6 != 0) == v6 {
				return i, v
			}
		}
		return 0, nil
	}
	f.On("vrrp_vr_update", func(m api.Message) ([]api.Message, error) {
		r := m.(*vrrp.VrrpVrUpdate)
		conf := vrrp.VrrpVrConf{SwIfIndex: r.SwIfIndex, VrID: r.VrID, Priority: r.Priority, Interval: r.Interval, Flags: r.Flags}
		if r.VrrpIndex == df7.NoIndex {
			idx := uint32(len(pool)) + 3 //nolint:gosec // test pool; leave holes at the start of the pool
			pool[idx] = &vr{det: &vrrp.VrrpVrDetails{Config: conf, Addrs: r.Addrs, NAddrs: r.NAddrs}}
			return []api.Message{&vrrp.VrrpVrUpdateReply{VrrpIndex: idx}}, nil
		}
		v, ok := pool[r.VrrpIndex]
		if !ok {
			return []api.Message{&vrrp.VrrpVrUpdateReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
		}
		if v.det.Config.SwIfIndex != r.SwIfIndex || v.det.Config.VrID != r.VrID {
			return []api.Message{&vrrp.VrrpVrUpdateReply{Retval: int32(api.INVALID_ARGUMENT)}}, nil
		}
		v.det.Config, v.det.Addrs = conf, r.Addrs
		return []api.Message{&vrrp.VrrpVrUpdateReply{VrrpIndex: r.VrrpIndex}}, nil
	})
	f.On("vrrp_vr_add_del", func(m api.Message) ([]api.Message, error) {
		r := m.(*vrrp.VrrpVrAddDel)
		if r.Priority == 0 || r.Interval == 0 {
			return []api.Message{&vrrp.VrrpVrAddDelReply{Retval: int32(api.INVALID_VALUE)}}, nil
		}
		i, v := find(r.SwIfIndex, r.VrID, r.Flags&vrrp.VRRP_API_VR_IPV6 != 0)
		if v == nil || r.IsAdd != 0 {
			return []api.Message{&vrrp.VrrpVrAddDelReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
		}
		delete(pool, i)
		return []api.Message{&vrrp.VrrpVrAddDelReply{}}, nil
	})
	f.On("vrrp_vr_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, v := range pool {
			out = append(out, v.det)
		}
		return out, nil
	})
	f.On("vrrp_vr_start_stop", func(m api.Message) ([]api.Message, error) {
		r := m.(*vrrp.VrrpVrStartStop)
		_, v := find(r.SwIfIndex, r.VrID, r.IsIPv6 != 0)
		if v == nil {
			return []api.Message{&vrrp.VrrpVrStartStopReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
		}
		v.det.Runtime.State = vrrp.VRRP_API_VR_STATE_INIT
		if r.IsStart != 0 {
			v.det.Runtime.State = vrrp.VRRP_API_VR_STATE_BACKUP
		}
		return []api.Message{&vrrp.VrrpVrStartStopReply{}}, nil
	})
	f.On("vrrp_vr_set_peers", func(m api.Message) ([]api.Message, error) {
		r := m.(*vrrp.VrrpVrSetPeers)
		_, v := find(r.SwIfIndex, r.VrID, r.IsIPv6 != 0)
		switch {
		case v == nil:
			return []api.Message{&vrrp.VrrpVrSetPeersReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
		case v.det.Runtime.State != vrrp.VRRP_API_VR_STATE_INIT:
			return []api.Message{&vrrp.VrrpVrSetPeersReply{Retval: int32(api.RSRC_IN_USE)}}, nil
		}
		v.peers = r.Addrs
		return []api.Message{&vrrp.VrrpVrSetPeersReply{}}, nil
	})
	f.On("vrrp_vr_peer_dump", func(m api.Message) ([]api.Message, error) {
		r := m.(*vrrp.VrrpVrPeerDump)
		_, v := find(r.SwIfIndex, r.VrID, r.IsIPv6 != 0)
		if v == nil {
			return nil, nil
		}
		return []api.Message{&vrrp.VrrpVrPeerDetails{SwIfIndex: r.SwIfIndex, VrID: r.VrID, IsIPv6: r.IsIPv6, NPeerAddrs: uint8(len(v.peers)), PeerAddrs: v.peers}}, nil //nolint:gosec // test
	})
	f.On("vrrp_vr_track_if_add_del", func(m api.Message) ([]api.Message, error) {
		r := m.(*vrrp.VrrpVrTrackIfAddDel)
		_, v := find(r.SwIfIndex, r.VrID, r.IsIPv6 != 0)
		for _, t := range r.Ifs {
			kept := v.tracks[:0]
			found := false
			for _, x := range v.tracks {
				if x.SwIfIndex == t.SwIfIndex {
					found = true
					if r.IsAdd != 0 {
						kept = append(kept, x) // VPP keeps the first priority
					}
					continue
				}
				kept = append(kept, x)
			}
			if r.IsAdd != 0 && !found {
				kept = append(kept, t)
			}
			v.tracks = kept
		}
		return []api.Message{&vrrp.VrrpVrTrackIfAddDelReply{}}, nil
	})
	f.On("vrrp_vr_track_if_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, v := range pool {
			if len(v.tracks) > 0 {
				out = append(out, &vrrp.VrrpVrTrackIfDetails{SwIfIndex: v.det.Config.SwIfIndex, VrID: v.det.Config.VrID, NIfs: uint8(len(v.tracks)), Ifs: v.tracks}) //nolint:gosec // test
			}
		}
		return out, nil
	})
	return f, pool
}

var v1 = VR{Interface: "loop0", VRID: 7}

func TestVRLifecycle(t *testing.T) {
	f, pool := fakeVRRP()
	ctx := t.Context()
	d := NewVR(f, df7test.Owner)
	spec := VRSpec{VR: v1, Priority: 100, Interval: 100, Preempt: true, Addresses: []string{"10.0.0.254"}}
	v := df7test.Desired(d, df7.Encode(spec))
	if v.Key != "vrrp.vr/loop0/7/ipv4" || d.Dependencies(v.Value)[0].Key != "interface/loop0" {
		t.Fatal(v.Key)
	}
	meta, err := d.Create(ctx, v.Value)
	if err != nil {
		t.Fatal(err)
	}
	r := df7test.Last[*vrrp.VrrpVrUpdate](t, f, "vrrp_vr_update")
	if r.VrrpIndex != df7.NoIndex || r.SwIfIndex != 1 || r.Flags != vrrp.VRRP_API_VR_PREEMPT || r.NAddrs != 1 {
		t.Fatalf("%+v", r)
	}
	if m := meta.(Meta); !m.HasIndex || m.Index != 3 {
		t.Fatalf("meta %+v", m)
	}
	// another owner's VR is never reported
	pool[99] = &vr{det: &vrrp.VrrpVrDetails{Config: vrrp.VrrpVrConf{SwIfIndex: 3, VrID: 7, Priority: 1, Interval: 1}}}
	df7test.AssertEmptyPlan(t, d, v)

	spec2 := spec
	spec2.Priority, spec2.Addresses = 120, []string{"10.0.0.250", "10.0.0.254"}
	if _, err := d.Update(ctx, v.Value, df7.Encode(spec2), meta); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, d, df7test.Desired(d, df7.Encode(spec2)))

	// after a restart the Meta has no pool index: Update walks the pool (other VR at 99 →
	// INVALID_ARGUMENT, free slots → NO_SUCH_ENTRY) and finds 3
	pool[1] = &vr{det: &vrrp.VrrpVrDetails{Config: vrrp.VrrpVrConf{SwIfIndex: 4, VrID: 1, Priority: 1, Interval: 1}}}
	kvs, _ := d.Retrieve(ctx)
	spec3 := spec2
	spec3.Priority = 90
	m, err := d.Update(ctx, df7.Encode(spec2), df7.Encode(spec3), kvs[0].Meta)
	if err != nil || m.(Meta).Index != 3 {
		t.Fatalf("walk: %v %v", m, err)
	}
	df7test.AssertEmptyPlan(t, d, df7test.Desired(d, df7.Encode(spec3)))
	uni := spec3
	uni.Unicast = true
	if _, err := d.Update(ctx, df7.Encode(spec3), df7.Encode(uni), m); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, df7.Encode(spec3), m); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*vrrp.VrrpVrAddDel](t, f, "vrrp_vr_add_del"); r.IsAdd != 0 || r.Priority == 0 {
		t.Fatalf("%+v", r)
	}
	df7test.AssertEmptyPlan(t, d)
	for i, bad := range []VRSpec{
		{VR: VR{Interface: "loop0"}, Priority: 1, Interval: 1, Addresses: []string{"10.0.0.1"}},
		{VR: v1, Interval: 1, Addresses: []string{"10.0.0.1"}},
		{VR: v1, Priority: 1, Interval: 1},
		{VR: v1, Priority: 1, Interval: 1, Addresses: []string{"10.0.0.2", "10.0.0.1"}},
		{VR: v1, Priority: 1, Interval: 1, Addresses: []string{"2001:db8::1"}},
	} {
		if err := bad.Validate(); !errors.Is(err, df7.ErrSpec) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if _, err := d.Create(ctx, df7.Encode(VRSpec{VR: VR{Interface: "loop9", VRID: 1}, Priority: 1, Interval: 1, Addresses: []string{"10.0.0.1"}})); !errors.Is(err, df7.ErrForeignInterface) {
		t.Fatal(err)
	}
}

func TestPeersTrackState(t *testing.T) {
	f, _ := fakeVRRP()
	ctx := t.Context()
	vd, pd, td, sd := NewVR(f, df7test.Owner), NewPeers(f, df7test.Owner), NewTrack(f, df7test.Owner), NewState(f, df7test.Owner)
	uni := VRSpec{VR: v1, Priority: 100, Interval: 100, Unicast: true, Addresses: []string{"10.0.0.254"}}
	if _, err := vd.Create(ctx, df7.Encode(uni)); err != nil {
		t.Fatal(err)
	}
	p := df7test.Desired(pd, df7.Encode(Peers{VR: v1, Peers: []string{"10.0.0.2"}}))
	if p.Key != "vrrp.vr-peers/loop0/7/ipv4" || pd.Dependencies(p.Value)[0].Key != "vrrp.vr/loop0/7/ipv4" {
		t.Fatal(p.Key)
	}
	pm, err := pd.Create(ctx, p.Value)
	if err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, pd, p)

	s := df7test.Desired(sd, df7.Encode(State{VR: v1, Running: true}))
	deps := sd.Dependencies(s.Value)
	if len(deps) != 2 || deps[0].Key != "vrrp.vr/loop0/7/ipv4" || deps[1].Key != "vrrp.vr-peers/loop0/7/ipv4" || !deps[1].Optional {
		t.Fatalf("deps %v", deps)
	}
	df7test.AssertEmptyPlan(t, sd) // stopped VR: no state object
	sm, err := sd.Create(ctx, s.Value)
	if err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, sd, s)

	// peers change on a running VR: stop, set, start
	f.Reset()
	p2 := df7.Encode(Peers{VR: v1, Peers: []string{"10.0.0.2", "10.0.0.3"}})
	if _, err := pd.Update(ctx, p.Value, p2, pm); err != nil {
		t.Fatal(err)
	}
	ss := f.CallsNamed("vrrp_vr_start_stop")
	if len(ss) != 2 || ss[0].(*vrrp.VrrpVrStartStop).IsStart != 0 || ss[1].(*vrrp.VrrpVrStartStop).IsStart != 1 {
		t.Fatalf("stop/start around set_peers: %v", ss)
	}
	df7test.AssertEmptyPlan(t, pd, df7test.Desired(pd, p2))
	df7test.AssertEmptyPlan(t, sd, s)

	tr := df7test.Desired(td, df7.Encode(Track{VR: v1, Tracked: "loop1", Priority: 20}))
	if tr.Key != "vrrp.vr-track-interface/loop0/7/ipv4/loop1" || td.Dependencies(tr.Value)[1].Key != "interface/loop1" {
		t.Fatal(tr.Key)
	}
	tm, err := td.Create(ctx, tr.Value)
	if err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, td, tr)
	tr2 := df7.Encode(Track{VR: v1, Tracked: "loop1", Priority: 30})
	if _, err := td.Update(ctx, tr.Value, tr2, tm); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, td, df7test.Desired(td, tr2))
	if err := td.Delete(ctx, tr2, tm); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, td)

	if err := sd.Delete(ctx, s.Value, sm); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, sd)
	if err := pd.Delete(ctx, p2, pm); err != nil {
		t.Fatal("peers delete is a no-op", err)
	}
	for i, bad := range []any{State{VR: v1}, Peers{VR: v1}, Peers{VR: v1, Peers: []string{"10.0.0.3", "10.0.0.2"}}, Track{VR: v1, Tracked: "loop0", Priority: 1}, Track{VR: v1, Tracked: "loop1"}} {
		if err := bad.(df7.Spec).Validate(); !errors.Is(err, df7.ErrSpec) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if _, err := pd.Create(ctx, df7.Encode(Peers{VR: VR{Interface: "loop0", VRID: 99}, Peers: []string{"10.0.0.2"}})); err == nil {
		t.Fatal("peers of a missing VR must fail")
	}
	r := scheduler.NewRegistry()
	Register(r, f, df7test.Owner)
	if r.Len() != 4 {
		t.Fatal(r.Names())
	}
}

func TestEvents(t *testing.T) {
	f, _ := fakeVRRP()
	f.Reply("want_vrrp_vr_events", &vrrp.WantVrrpVrEventsReply{})
	ctx, cancel := context.WithCancel(t.Context())
	ch, err := WatchEvents(ctx, f, df7test.Owner)
	if err != nil {
		t.Fatal(err)
	}
	f.Emit(&vrrp.VrrpVrEvent{Vr: vrrp.VrrpVrKey{SwIfIndex: 3, VrID: 1}, NewState: vrrp.VRRP_API_VR_STATE_MASTER})
	f.Emit(&vrrp.VrrpVrEvent{Vr: vrrp.VrrpVrKey{SwIfIndex: 2, VrID: 7, IsIPv6: 1}, OldState: vrrp.VRRP_API_VR_STATE_BACKUP, NewState: vrrp.VRRP_API_VR_STATE_MASTER})
	select {
	case e := <-ch:
		if e.Key != "vrrp.vr/loop1/7/ipv6" || e.OldState != StateBackup || e.NewState != StateMaster {
			t.Fatalf("%+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
	}
	cancel()
	for e := range ch {
		t.Logf("late event %+v", e)
	}
	if r := df7test.Last[*vrrp.WantVrrpVrEvents](t, f, "want_vrrp_vr_events"); r.EnableDisable {
		t.Fatal("events must be disabled when the watch ends")
	}
	if StateName(vrrp.VRRP_API_VR_STATE_INTF_DOWN) != StateIntfDown || StateName(9) != "#9" {
		t.Fatal("state names")
	}
	if _, err := States(t.Context(), f, df7test.Owner); err != nil {
		t.Fatal(err)
	}
}
