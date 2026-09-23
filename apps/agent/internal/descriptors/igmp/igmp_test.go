package igmp

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/igmp"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
)

type grp struct {
	idx uint32
	g   string
}

// fakeIGMP models the plugin: enable/disable (-1 when already in that state, like VPP),
// host-mode-only listens (igmp_dump sends every source twice, like VPP), proxy devices and
// downstream interfaces (-1 on a duplicate add).
func fakeIGMP() (*df7test.Fake, map[uint32]uint8, map[grp][]ip_types.IP4Address) {
	f := df7test.NewFake()
	modes := map[uint32]uint8{}
	groups := map[grp][]ip_types.IP4Address{}
	proxies := map[uint32]uint32{}
	down := map[[2]uint32]bool{}
	f.On("igmp_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*igmp.IgmpEnableDisable)
		_, on := modes[uint32(r.SwIfIndex)]
		switch {
		case r.Enable && !on:
			modes[uint32(r.SwIfIndex)] = r.Mode
		case !r.Enable && on:
			delete(modes, uint32(r.SwIfIndex))
		default:
			return []api.Message{&igmp.IgmpEnableDisableReply{Retval: int32(api.UNSPECIFIED)}}, nil
		}
		return []api.Message{&igmp.IgmpEnableDisableReply{}}, nil
	})
	f.On("igmp_listen", func(m api.Message) ([]api.Message, error) {
		r := m.(*igmp.IgmpListen)
		idx := uint32(r.Group.SwIfIndex)
		if modes[idx] != 1 {
			return []api.Message{&igmp.IgmpListenReply{Retval: int32(api.INVALID_INTERFACE)}}, nil
		}
		k := grp{idx, netip.AddrFrom4(r.Group.Gaddr).String()}
		if len(r.Group.Saddrs) == 0 {
			delete(groups, k)
		} else {
			groups[k] = r.Group.Saddrs
		}
		return []api.Message{&igmp.IgmpListenReply{}}, nil
	})
	f.On("igmp_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for k, srcs := range groups {
			for _, s := range srcs {
				d := &igmp.IgmpDetails{SwIfIndex: interface_types.InterfaceIndex(k.idx), Saddr: s, Gaddr: ip_types.IP4Address(netip.MustParseAddr(k.g).As4())}
				out = append(out, d, d)
			}
		}
		return out, nil
	})
	f.On("igmp_proxy_device_add_del", func(m api.Message) ([]api.Message, error) {
		r := m.(*igmp.IgmpProxyDeviceAddDel)
		if r.Add != 0 {
			if _, ok := proxies[r.VrfID]; !ok {
				proxies[r.VrfID] = uint32(r.SwIfIndex)
			}
		} else {
			delete(proxies, r.VrfID)
		}
		return []api.Message{&igmp.IgmpProxyDeviceAddDelReply{}}, nil
	})
	f.On("igmp_proxy_device_add_del_interface", func(m api.Message) ([]api.Message, error) {
		r := m.(*igmp.IgmpProxyDeviceAddDelInterface)
		k := [2]uint32{r.VrfID, uint32(r.SwIfIndex)}
		if _, ok := proxies[r.VrfID]; !ok || (r.Add && down[k]) {
			return []api.Message{&igmp.IgmpProxyDeviceAddDelInterfaceReply{Retval: int32(api.UNSPECIFIED)}}, nil
		}
		if !r.Add && !down[k] {
			return []api.Message{&igmp.IgmpProxyDeviceAddDelInterfaceReply{Retval: int32(api.NO_SUCH_FIB)}}, nil
		}
		down[k] = r.Add
		return []api.Message{&igmp.IgmpProxyDeviceAddDelInterfaceReply{}}, nil
	})
	f.Reply("igmp_group_prefix_set", &igmp.IgmpGroupPrefixSetReply{})
	f.Reply("igmp_clear_interface", &igmp.IgmpClearInterfaceReply{})
	return f, modes, groups
}

func TestInterfaceAndListen(t *testing.T) {
	f, modes, groups := fakeIGMP()
	ctx := t.Context()
	reg := NewModes()
	id, ld := NewInterface(f, df7test.Owner, reg), NewListen(f, df7test.Owner, reg)
	host := df7.Encode(Interface{Interface: "loop0", Mode: ModeHost})
	if id.KeyOf(host) != "igmp.interface/loop0" || id.Dependencies(host)[0].Key != "interface/loop0" {
		t.Fatal(id.KeyOf(host))
	}
	im, err := id.Create(ctx, host)
	if err != nil || modes[1] != 1 {
		t.Fatal(err, modes)
	}
	if _, err := id.Create(ctx, host); err != nil { // write-only resync: VPP's -1 = already on
		t.Fatalf("re-apply: %v", err)
	}
	if _, err := id.Retrieve(ctx); !errors.Is(err, df7.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	if _, err := id.Update(ctx, host, host, im); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}

	l := df7test.Desired(ld, df7.Encode(Listen{Interface: "loop0", Group: "232.0.0.1", Sources: []string{"10.0.0.10", "10.0.0.11"}}))
	deps := ld.Dependencies(l.Value)
	if l.Key != "igmp.listen/loop0/232.0.0.1" || len(deps) != 2 || deps[0].Key != "igmp.interface/loop0" {
		t.Fatalf("%s %v", l.Key, deps)
	}
	lm, err := ld.Create(ctx, l.Value)
	if err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*igmp.IgmpListen](t, f, "igmp_listen"); r.Group.Filter != igmp.INCLUDE || r.Group.NSrcs != 2 {
		t.Fatalf("%+v", r)
	}
	// another owner's joins and joins on a router-mode interface are never reported
	groups[grp{3, "232.0.0.9"}] = []ip_types.IP4Address{{10, 9, 0, 1}}
	df7test.AssertEmptyPlan(t, ld, l)
	router := df7.Encode(Interface{Interface: "loop1", Mode: ModeRouter})
	rm, err := id.Create(ctx, router)
	if err != nil {
		t.Fatal(err)
	}
	groups[grp{2, "239.1.1.1"}] = []ip_types.IP4Address{{10, 0, 1, 1}} // learned from a report
	df7test.AssertEmptyPlan(t, ld, l)

	l2 := df7.Encode(Listen{Interface: "loop0", Group: "232.0.0.1", Sources: []string{"10.0.0.12"}})
	if _, err := ld.Update(ctx, l.Value, l2, lm); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, ld, df7test.Desired(ld, l2))
	if err := ld.Delete(ctx, l2, lm); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*igmp.IgmpListen](t, f, "igmp_listen"); r.Group.NSrcs != 0 {
		t.Fatal("delete = leave (no sources)")
	}
	df7test.AssertEmptyPlan(t, ld)
	if err := id.Delete(ctx, router, rm); err != nil {
		t.Fatal(err)
	}
	if err := id.Delete(ctx, host, im); err != nil || len(modes) != 0 {
		t.Fatal(err, modes)
	}
	if err := id.Delete(ctx, host, im); err != nil { // already off
		t.Fatal(err)
	}
	for i, bad := range []Listen{
		{Interface: "loop0", Group: "10.0.0.1", Sources: []string{"10.0.0.2"}},
		{Interface: "loop0", Group: "232.0.0.1"},
		{Interface: "loop0", Group: "232.0.0.1", Sources: []string{"10.0.0.3", "10.0.0.2"}},
		{Interface: "loop0", Group: "232.0.0.1", Sources: []string{"2001:db8::1"}},
	} {
		if err := bad.Validate(); !errors.Is(err, df7.ErrSpec) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if _, err := id.Create(ctx, df7.Encode(Interface{Interface: "loop9", Mode: ModeHost})); !errors.Is(err, df7.ErrForeignInterface) {
		t.Fatal(err)
	}
	if err := ClearInterface(ctx, f, 1); err != nil {
		t.Fatal(err)
	}
}

func TestProxyAndPrefix(t *testing.T) {
	f, _, _ := fakeIGMP()
	ctx := t.Context()
	pd, sd, gd := NewProxyDevice(f, df7test.Owner), NewDownstream(f, df7test.Owner), NewGroupPrefix(f, df7test.Owner)
	dev := df7.Encode(ProxyDevice{VRF: 5, Upstream: "loop0"})
	deps := pd.Dependencies(dev)
	if pd.KeyOf(dev) != "igmp.proxy-device/5" || len(deps) != 3 || deps[0].Key != "vrf/5" || deps[2].Key != "igmp.interface/loop0" {
		t.Fatal(deps)
	}
	pm, err := pd.Create(ctx, dev)
	if err != nil {
		t.Fatal(err)
	}
	ds := df7.Encode(Downstream{VRF: 5, Interface: "loop1"})
	deps = sd.Dependencies(ds)
	if sd.KeyOf(ds) != "igmp.proxy-downstream/5/loop1" || deps[0].Key != "igmp.proxy-device/5" || deps[2].Key != "igmp.interface/loop1" {
		t.Fatal(deps)
	}
	sm, err := sd.Create(ctx, ds)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sd.Create(ctx, ds); err != nil { // write-only resync: -1 = already downstream
		t.Fatal(err)
	}
	if err := sd.Delete(ctx, ds, sm); err != nil {
		t.Fatal(err)
	}
	if err := sd.Delete(ctx, ds, sm); err != nil { // not a downstream any more
		t.Fatal(err)
	}
	if err := pd.Delete(ctx, dev, pm); err != nil {
		t.Fatal(err)
	}
	for _, d := range []scheduler.Descriptor{pd, sd, gd} {
		if _, err := d.Retrieve(ctx); !errors.Is(err, df7.ErrRetrieveUnsupported) {
			t.Fatal(d.Name(), err)
		}
	}
	gp := df7.Encode(GroupPrefix{Prefix: "239.1.0.0/16"})
	if gd.KeyOf(gp) != "igmp.group-prefix/239.1.0.0/16" {
		t.Fatal(gd.KeyOf(gp))
	}
	if _, err := gd.Create(ctx, gp); err != nil || df7test.Last[*igmp.IgmpGroupPrefixSet](t, f, "igmp_group_prefix_set").Gp.Type != igmp.SSM {
		t.Fatal(err)
	}
	if err := gd.Delete(ctx, gp, nil); err != nil || df7test.Last[*igmp.IgmpGroupPrefixSet](t, f, "igmp_group_prefix_set").Gp.Type != igmp.ASM {
		t.Fatal(err)
	}
	if err := (GroupPrefix{Prefix: "10.0.0.0/8"}).Validate(); !errors.Is(err, df7.ErrSpec) {
		t.Fatal("unicast range must be invalid")
	}
	r := scheduler.NewRegistry()
	Register(r, f, df7test.Owner)
	if r.Len() != 4 {
		t.Fatal("non-owners register no globals (D-071):", r.Names())
	}
	RegisterGlobals(r, f, df7test.Owner)
	if r.Len() != 5 {
		t.Fatal(r.Names())
	}
}

func TestEvents(t *testing.T) {
	f, _, _ := fakeIGMP()
	f.Reply("want_igmp_events", &igmp.WantIgmpEventsReply{})
	ctx, cancel := context.WithCancel(t.Context())
	ch, err := WatchEvents(ctx, f, df7test.Owner)
	if err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*igmp.WantIgmpEvents](t, f, "want_igmp_events"); r.Enable != 1 || r.PID == 0 {
		t.Fatalf("%+v", r)
	}
	f.Emit(&igmp.IgmpEvent{SwIfIndex: 3, Filter: igmp.INCLUDE, Saddr: ip_types.IP4Address{10, 9, 0, 1}, Gaddr: ip_types.IP4Address{232, 0, 0, 9}})
	f.Emit(&igmp.IgmpEvent{SwIfIndex: 2, Filter: igmp.EXCLUDE, Saddr: ip_types.IP4Address{10, 0, 0, 1}, Gaddr: ip_types.IP4Address{232, 0, 0, 1}})
	select {
	case e := <-ch:
		if e.Interface != "loop1" || e.Group != "232.0.0.1" || e.Source != "10.0.0.1" || e.Filter != "exclude" {
			t.Fatalf("%+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
	}
	cancel()
	for e := range ch {
		t.Logf("late event %+v", e)
	}
	if r := df7test.Last[*igmp.WantIgmpEvents](t, f, "want_igmp_events"); r.Enable != 0 {
		t.Fatal("events must be disabled when the watch ends")
	}
}
