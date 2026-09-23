package igmp

import (
	"errors"
	"strconv"
	"testing"

	"ngfw/agent/binapi/igmp"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Host test: IP table <base>+80 (unicast + multicast FIB), this slot's loopbacks loop<slot>80
// (host mode, proxy upstream), loop<slot>81 (router mode, downstream), loop<slot>82 (host mode,
// static joins); groups in 232.<slot>.0.0/16, sources in 10.<slot>.80.0/24; the SSM range test
// uses 239.<slot>.0.0/16 and removes it again.
func TestIGMPOnHost(t *testing.T) {
	h := df7test.StartHost(t)
	vrf := h.TableB + 80
	h.IPTable(vrf)
	up, upIdx := h.Loopback(80, true, true)
	down, downIdx := h.Loopback(81, true, true)
	host, hostIdx := h.Loopback(82, true, true)
	for i, idx := range []uint32{upIdx, downIdx, hostIdx} {
		h.BindTable(idx, vrf)
		h.Address(idx, h.Addr(80+i, 1)+"/24")
	}
	modes := NewModes()
	id, ld, gd, pd, sd := NewInterface(h.C, h.Owner, modes), NewListen(h.C, h.Owner, modes), NewGroupPrefix(h.C, h.Owner), NewProxyDevice(h.C, h.Owner), NewDownstream(h.C, h.Owner)
	h.CleanupOwned(ld)

	events, err := WatchEvents(h.Ctx, h.C, h.Owner)
	h.Must("want_igmp_events", err)

	ifaces := []scheduler.KV{
		df7test.Desired(id, df7.Encode(Interface{Interface: up, Mode: ModeHost})),
		df7test.Desired(id, df7.Encode(Interface{Interface: down, Mode: ModeRouter})),
		df7test.Desired(id, df7.Encode(Interface{Interface: host, Mode: ModeHost})),
	}
	ci := h.Apply(id, ifaces...)
	t.Cleanup(func() { h.DeleteAll(id, ci) })
	h.Apply(id, ifaces...) // idempotent (write-only resync)

	g := "232." + itoa(h.Slot) + ".0.1"
	listens := []scheduler.KV{df7test.Desired(ld, df7.Encode(Listen{Interface: host, Group: g, Sources: []string{h.Addr(80, 10), h.Addr(80, 11)}}))}
	cl := h.Apply(ld, listens...)
	h.ExpectRetrieved(ld, listens...)

	t.Run("listen update", func(t *testing.T) {
		n := df7.Encode(Listen{Interface: host, Group: g, Sources: []string{h.Addr(80, 12)}})
		_, err := ld.Update(h.Ctx, listens[0].Value, n, cl[0].Meta)
		h.Must("update", err)
		listens[0].Value, cl[0].Value = n, n
		h.ExpectRetrieved(ld, listens...)
	})

	t.Run("proxy (write-only)", func(t *testing.T) {
		dev := df7.Encode(ProxyDevice{VRF: vrf, Upstream: up})
		cp := h.Apply(pd, df7test.Desired(pd, dev))
		ds := h.Apply(sd, df7test.Desired(sd, df7.Encode(Downstream{VRF: vrf, Interface: down})))
		h.Apply(sd, df7test.Desired(sd, df7.Encode(Downstream{VRF: vrf, Interface: down}))) // idempotent
		h.Hold("igmp proxy")
		h.DeleteAll(sd, ds)
		h.DeleteAll(pd, cp)
	})

	t.Run("group prefix dump (read-only probe)", func(t *testing.T) {
		stream, err := igmp.NewServiceClient(h.C).IgmpGroupPrefixDump(h.Ctx, &igmp.IgmpGroupPrefixDump{})
		h.Must("igmp_group_prefix_dump", err)
		dets, err := df7.Collect(stream.Recv)
		t.Logf("igmp_group_prefix_dump (why igmp.group-prefix is write-only): %d details, err %v", len(dets), err)
	})

	t.Run("group prefix (write-only, globals owner only)", func(t *testing.T) {
		df7test.GlobalsOptIn(t, "igmp.group-prefix (the SSM range list)")
		v := df7.Encode(GroupPrefix{Prefix: "239." + itoa(h.Slot) + ".0.0/16"})
		cg := h.Apply(gd, df7test.Desired(gd, v))
		h.DeleteAll(gd, cg)
		if _, err := gd.Retrieve(h.Ctx); !errors.Is(err, df7.ErrRetrieveUnsupported) {
			t.Fatal(err)
		}
	})

	select {
	case e := <-events:
		t.Logf("igmp event: %+v", e)
	default:
		t.Log("no igmp_event (host-mode joins produce none; events come from router-mode learning)")
	}

	t.Run("restart simulation", func(t *testing.T) {
		// a fresh process has an empty Modes registry: host-mode joins are still reported
		h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return NewListen(c, h.Owner, nil) }, listens...)
	})

	h.DeleteAll(ld, cl)
	h.ExpectNone(ld)
	h.DeleteAll(id, ci)
	ci = nil
}

func itoa(n int) string { return strconv.Itoa(n) }
