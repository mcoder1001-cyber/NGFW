package mactime_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ethernet_types"
	"ngfw/agent/binapi/feature"
	mactimeapi "ngfw/agent/binapi/mactime"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/descriptors/mactime"
	"ngfw/agent/internal/scheduler"
)

const owner = "w7"

var ctx = context.Background()

// fakeMactime models VPP's mactime plugin: the device table keyed by MAC where an add on an
// existing MAC appends its ranges, and the feature enable that stacks on every enable
// (vnet_config_add_feature) with feature_is_enabled answering true for an index the arc never
// reached (VNET_API_ERROR_INVALID_SW_IF_INDEX cast to bool).
type fakeMactime struct {
	*dfkittest.FakeVPP
	mu      sync.Mutex
	devices []*mactimeapi.MactimeDetails
	count   map[uint32]int
	reached map[uint32]bool // indexes the device-input arc vector reaches
	enables int
}

func newFake() *fakeMactime {
	f := &fakeMactime{
		FakeVPP: dfkittest.NewFake(
			dfkittest.Iface{Index: 1, Name: "loop7001", Tag: "w7:loop7001"},
			dfkittest.Iface{Index: 2, Name: "loop7002", Tag: "w7:loop7002"},
			dfkittest.Iface{Index: 3, Name: "loop9", Tag: "w3:loop9"},
		),
		count: map[uint32]int{}, reached: map[uint32]bool{},
	}
	f.On("mactime_add_del_range", func(m api.Message) ([]api.Message, error) {
		r := m.(*mactimeapi.MactimeAddDelRange)
		f.mu.Lock()
		defer f.mu.Unlock()
		for i, d := range f.devices {
			if d.MacAddress != r.MacAddress {
				continue
			}
			if !r.IsAdd {
				f.devices = append(f.devices[:i], f.devices[i+1:]...)
				return []api.Message{&mactimeapi.MactimeAddDelRangeReply{}}, nil
			}
			for _, x := range r.Ranges { // VPP appends
				d.Ranges = append(d.Ranges, mactimeapi.MactimeTimeRange(x))
			}
			d.Nranges = uint32(len(d.Ranges)) //nolint:gosec // test
			return []api.Message{&mactimeapi.MactimeAddDelRangeReply{}}, nil
		}
		if !r.IsAdd {
			return []api.Message{&mactimeapi.MactimeAddDelRangeReply{Retval: -6}}, nil
		}
		d := &mactimeapi.MactimeDetails{MacAddress: r.MacAddress, DeviceName: r.DeviceName}
		for _, x := range r.Ranges {
			d.Ranges = append(d.Ranges, mactimeapi.MactimeTimeRange(x))
		}
		d.Nranges = uint32(len(d.Ranges)) //nolint:gosec // test
		switch {
		case len(r.Ranges) > 0 && r.Drop:
			d.Flags = 1 << 2
		case len(r.Ranges) > 0:
			d.Flags = 1 << 3
		case r.Drop:
			d.Flags = 1 << 0
		default:
			d.Flags = 1 << 1
		}
		f.devices = append(f.devices, d)
		return []api.Message{&mactimeapi.MactimeAddDelRangeReply{}}, nil
	})
	f.On("mactime_dump", func(api.Message) ([]api.Message, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		out := []api.Message{}
		for _, d := range f.devices {
			c := *d
			c.Ranges = append([]mactimeapi.MactimeTimeRange(nil), d.Ranges...)
			out = append(out, &c)
		}
		return append(out, &mactimeapi.MactimeDumpReply{TableEpoch: 7}), nil
	})
	f.On("mactime_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*mactimeapi.MactimeEnableDisable)
		f.mu.Lock()
		defer f.mu.Unlock()
		idx := uint32(r.SwIfIndex)
		f.reached[idx] = true
		if r.EnableDisable {
			f.count[idx]++
			f.enables++
		} else if f.count[idx] > 0 {
			f.count[idx]--
		}
		return []api.Message{&mactimeapi.MactimeEnableDisableReply{}}, nil
	})
	f.On("feature_is_enabled", func(m api.Message) ([]api.Message, error) {
		r := m.(*feature.FeatureIsEnabled)
		f.mu.Lock()
		defer f.mu.Unlock()
		idx := uint32(r.SwIfIndex)
		on := f.count[idx] > 0 || !f.reached[idx] // the out-of-range quirk
		return []api.Message{&feature.FeatureIsEnabledReply{IsEnabled: on}}, nil
	})
	return f
}

func (f *fakeMactime) restartVPP() {
	f.RestartVPP()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.devices, f.count, f.reached = nil, map[uint32]int{}, map[uint32]bool{}
}

func retrieve(t *testing.T, d scheduler.Descriptor) map[scheduler.Key]proto.Message {
	t.Helper()
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := map[scheduler.Key]proto.Message{}
	for _, kv := range kvs {
		out[kv.Key] = kv.Value
	}
	return out
}

func dev(name, mac string, drop bool, ranges ...mactime.Range) mactime.Device {
	return mactime.Device{Name: name, MAC: mac, Drop: drop, Ranges: ranges}
}

func TestDevice(t *testing.T) {
	f := newFake()
	d := mactime.NewDevice(f, owner)
	// another owner's device and VPP's own learned entry share the table
	f.devices = append(f.devices,
		&mactimeapi.MactimeDetails{MacAddress: ethernet_types.MacAddress{2, 0, 0, 0, 3, 1}, DeviceName: "w3:cam", Flags: 1 << 0},
		&mactimeapi.MactimeDetails{MacAddress: ethernet_types.MacAddress{2, 0, 0, 0, 7, 2}, DeviceName: "mac-02:00:00:00:07:02", Flags: 1 << 1},
	)
	if got := retrieve(t, d); len(got) != 0 {
		t.Fatalf("foreign devices retrieved: %v", got)
	}
	// desired ranges out of order and an upper-case MAC: Normalize gives Retrieve's form
	want := dev("kids", "02:00:00:00:07:01", false, mactime.Range{Start: 2*86400 + 57600, End: 2*86400 + 72000}, mactime.Range{Start: 86400 + 57600, End: 86400 + 72000})
	desired := d.Normalize(dev("kids", "02:00:00:00:07:01", false, want.Ranges...).Proto())
	if d.KeyOf(desired) != "mactime.range/kids" {
		t.Fatalf("key %s", d.KeyOf(desired))
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	got := retrieve(t, d)
	if len(got) != 1 || !proto.Equal(got["mactime.range/kids"], desired) {
		t.Fatalf("retrieve = %v, want %v", got, desired)
	}
	// Update replaces the ranges (a plain add would append them)
	upd := d.Normalize(dev("kids", "02:00:00:00:07:01", true, mactime.Range{Start: 0, End: 3600}).Proto())
	if meta, err = d.Update(ctx, desired, upd, meta); err != nil {
		t.Fatal(err)
	}
	if got := retrieve(t, d); !proto.Equal(got["mactime.range/kids"], upd) {
		t.Fatalf("after update = %v", got)
	}
	if _, err := d.Update(ctx, upd, dev("kids", "02:00:00:00:07:09", true).Proto(), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("MAC change: %v", err)
	}
	// a MAC held by another owner is never taken; VPP's learned entry is replaced
	if _, err := d.Create(ctx, dev("cam", "02:00:00:00:03:01", false).Proto()); !errors.Is(err, dfkit.ErrNotOurs) {
		t.Fatalf("foreign MAC: %v", err)
	}
	// VPP's learned entry: a test slot (not the D-071 globals owner) never touches it; the globals owner replaces it
	if _, err := d.Create(ctx, dev("tv", "02:00:00:00:07:02", true).Proto()); !errors.Is(err, dfkit.ErrNotOurs) {
		t.Fatalf("learned entry taken by a non-owner: %v", err)
	}
	if _, err := mactime.NewDevice(f, owner, mactime.WithGlobalsOwner(true)).Create(ctx, dev("tv", "02:00:00:00:07:02", true).Proto()); err != nil {
		t.Fatal(err)
	}
	if got := retrieve(t, d); len(got) != 2 {
		t.Fatalf("after learned-entry takeover: %v", got)
	}
	if err := d.Delete(ctx, upd, meta); err != nil {
		t.Fatal(err)
	}
	if _, ok := retrieve(t, d)["mactime.range/kids"]; ok {
		t.Fatal("device survived Delete")
	}
	if len(f.devices) != 2 || f.devices[0].DeviceName != "w3:cam" {
		t.Fatalf("the other owner's device was touched: %+v", f.devices)
	}
	if _, err := d.Create(ctx, dev("bad", "02:00:00:00:07:05", false, mactime.Range{Start: 5, End: 5}).Proto()); err == nil {
		t.Fatal("empty range accepted")
	}
}

func TestEnableAppliedOnce(t *testing.T) {
	f := newFake()
	store := dfkit.NewMemoryBootStore()
	d := mactime.NewEnable(f, owner, store)
	desired := mactime.Enable{Interface: "loop7001"}.Proto()
	if d.KeyOf(desired) != "mactime.enable/loop7001" || d.Dependencies(desired)[0].Key != "interface/loop7001" {
		t.Fatal("key / deps")
	}
	// never-reached index: feature_is_enabled says "on" but there is no record → not reported
	if got := retrieve(t, d); len(got) != 0 {
		t.Fatalf("quirk leaked into Retrieve: %v", got)
	}
	if _, err := d.Create(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if f.count[1] != 1 {
		t.Fatalf("feature count = %d, want 1", f.count[1])
	}
	// every resync re-runs Create: the feature must not stack (D-076)
	for i := 0; i < 3; i++ {
		if _, err := d.Create(ctx, desired); err != nil {
			t.Fatal(err)
		}
	}
	if f.count[1] != 1 || f.enables != 1 {
		t.Fatalf("stacked: count %d, enables %d", f.count[1], f.enables)
	}
	if got := retrieve(t, d); len(got) != 1 || !proto.Equal(got["mactime.enable/loop7001"], desired) {
		t.Fatalf("retrieve = %v", got)
	}
	// disabled behind the agent's back: Retrieve no longer reports it, Create restores it once
	f.count[1] = 0
	if got := retrieve(t, d); len(got) != 0 {
		t.Fatalf("after loss: %v", got)
	}
	if _, err := d.Create(ctx, desired); err != nil || f.count[1] != 1 {
		t.Fatalf("restore: %v count %d", err, f.count[1])
	}
	// a VPP restart expires the record: enabled once on the new instance
	f.restartVPP()
	if got := retrieve(t, d); len(got) != 0 {
		t.Fatalf("stale record reported after a VPP restart: %v", got)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil || f.count[1] != 1 {
		t.Fatalf("after restart: %v count %d", err, f.count[1])
	}
	// a stale enable without a record (lost state dir): normalised to exactly one
	f.count[2] = 1
	f.reached[2] = true
	if _, err := d.Create(ctx, mactime.Enable{Interface: "loop7002"}.Proto()); err != nil || f.count[2] != 1 {
		t.Fatalf("lost record: %v count %d", err, f.count[2])
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if f.count[1] != 0 {
		t.Fatalf("Delete left the feature on: %d", f.count[1])
	}
	if _, ok := store.Get("mactime.enable/loop7001"); ok {
		t.Fatal("record survived Delete")
	}
	// another owner's interface is never resolved
	if _, err := d.Create(ctx, mactime.Enable{Interface: "loop9"}.Proto()); err == nil {
		t.Fatal("foreign interface accepted")
	}
}
