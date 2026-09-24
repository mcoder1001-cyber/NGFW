package nsim_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.fd.io/govpp/api"

	nsimapi "ngfw/agent/binapi/nsim"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/descriptors/nsim"
	"ngfw/agent/internal/scheduler"
)

const owner = "w7"

var ctx = context.Background()

// fakeNsim models VPP's nsim plugin: nsim_configure2 (re)allocates the model, the cross-connect and the
// output feature need it (-76 before) and stack a feature on every enable.
type fakeNsim struct {
	*dfkittest.FakeVPP
	mu         sync.Mutex
	configured bool
	configures int
	last       nsimapi.NsimConfigure2
	cross      int
	crossPair  [2]uint32
	output     map[uint32]int
}

func newFake() *fakeNsim {
	f := &fakeNsim{
		FakeVPP: dfkittest.NewFake(
			dfkittest.Iface{Index: 1, Name: "loop7001", Tag: "w7:loop7001"},
			dfkittest.Iface{Index: 2, Name: "loop7002", Tag: "w7:loop7002"},
			dfkittest.Iface{Index: 3, Name: "loop9", Tag: "w3:loop9"},
		),
		output: map[uint32]int{},
	}
	f.On("nsim_configure2", func(m api.Message) ([]api.Message, error) {
		r := m.(*nsimapi.NsimConfigure2)
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.DelayInUsec == 0 || r.BandwidthInBitsPerSecond == 0 {
			return []api.Message{&nsimapi.NsimConfigure2Reply{Retval: -77}}, nil
		}
		f.configured, f.last = true, *r
		f.configures++
		return []api.Message{&nsimapi.NsimConfigure2Reply{}}, nil
	})
	f.On("nsim_cross_connect_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*nsimapi.NsimCrossConnectEnableDisable)
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.configured {
			return []api.Message{&nsimapi.NsimCrossConnectEnableDisableReply{Retval: -76}}, nil
		}
		f.crossPair = [2]uint32{uint32(r.SwIfIndex0), uint32(r.SwIfIndex1)}
		if r.EnableDisable {
			f.cross++
		} else if f.cross > 0 {
			f.cross--
		}
		return []api.Message{&nsimapi.NsimCrossConnectEnableDisableReply{}}, nil
	})
	f.On("nsim_output_feature_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*nsimapi.NsimOutputFeatureEnableDisable)
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.configured {
			return []api.Message{&nsimapi.NsimOutputFeatureEnableDisableReply{Retval: -76}}, nil
		}
		if r.EnableDisable {
			f.output[uint32(r.SwIfIndex)]++
		} else if f.output[uint32(r.SwIfIndex)] > 0 {
			f.output[uint32(r.SwIfIndex)]--
		}
		return []api.Message{&nsimapi.NsimOutputFeatureEnableDisableReply{}}, nil
	})
	return f
}

func TestConfigAppliedOncePerValue(t *testing.T) {
	f := newFake()
	store := dfkit.NewMemoryBootStore()
	d := nsim.NewConfig(f, store)
	c := nsim.Config{DelayUsec: 20000, BandwidthBps: 1e8, PacketSize: 1500, PacketsPerDrop: 100}
	if d.KeyOf(c.Proto()) != "nsim.config/global" || d.Dependencies(c.Proto()) != nil {
		t.Fatal("key / deps")
	}
	for i := 0; i < 3; i++ { // write-only: every resync re-runs Create; VPP reallocates on each configure
		if _, err := d.Create(ctx, c.Proto()); err != nil {
			t.Fatal(err)
		}
	}
	if f.configures != 1 || f.last.DelayInUsec != 20000 || f.last.BandwidthInBitsPerSecond != 1e8 || f.last.AveragePacketSize != 1500 || f.last.PacketsPerDrop != 100 {
		t.Fatalf("configures %d last %+v", f.configures, f.last)
	}
	c2 := c
	c2.DelayUsec = 30000
	if _, err := d.Update(ctx, c.Proto(), c2.Proto(), nil); err != nil || f.configures != 2 {
		t.Fatalf("update: %v configures %d", err, f.configures)
	}
	f.RestartVPP() // a new VPP instance: configured once more
	if _, err := d.Create(ctx, c2.Proto()); err != nil || f.configures != 3 {
		t.Fatalf("after restart: %v configures %d", err, f.configures)
	}
	if err := d.Delete(ctx, c2.Proto(), nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get("nsim.config/global"); ok {
		t.Fatal("record survived Delete")
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve must be write-only: %v", err)
	}
	for _, bad := range []nsim.Config{{BandwidthBps: 1, PacketSize: 64}, {DelayUsec: 1, PacketSize: 64}, {DelayUsec: 1, BandwidthBps: 1, PacketSize: 9001}} {
		if _, err := d.Create(ctx, bad.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Fatalf("%+v accepted: %v", bad, err)
		}
	}
}

func TestCrossConnectAndOutput(t *testing.T) {
	f := newFake()
	store := dfkit.NewMemoryBootStore()
	x := nsim.NewCrossConnect(f, owner, store)
	o := nsim.NewOutput(f, owner, store)
	xc := nsim.CrossConnect{A: "loop7001", B: "loop7002"}.Proto()
	out := nsim.Output{Interface: "loop7002"}.Proto()
	if x.KeyOf(xc) != "nsim.cross-connect/global" || len(x.Dependencies(xc)) != 3 || x.Dependencies(xc)[0].Key != "nsim.config/global" {
		t.Fatal("cross-connect key / deps")
	}
	if o.KeyOf(out) != "nsim.output/loop7002" || o.Dependencies(out)[1].Key != "interface/loop7002" {
		t.Fatal("output key / deps")
	}
	// before the model is configured VPP refuses both (the dependency on nsim.config orders them)
	if _, err := x.Create(ctx, xc); err == nil {
		t.Fatal("cross-connect before the model accepted")
	}
	if _, err := nsim.NewConfig(f, store).Create(ctx, nsim.Config{DelayUsec: 1000, BandwidthBps: 1e6, PacketSize: 1500}.Proto()); err != nil {
		t.Fatal(err)
	}
	var meta any
	for i := 0; i < 3; i++ { // write-only re-applies never stack the features
		var err error
		if meta, err = x.Create(ctx, xc); err != nil {
			t.Fatal(err)
		}
		if _, err := o.Create(ctx, out); err != nil {
			t.Fatal(err)
		}
	}
	if f.cross != 1 || f.crossPair != [2]uint32{1, 2} || f.output[2] != 1 {
		t.Fatalf("cross %d pair %v output %v", f.cross, f.crossPair, f.output)
	}
	if _, err := x.Update(ctx, xc, nsim.CrossConnect{A: "loop7002", B: "loop7001"}.Proto(), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("another pair must be a recreate: %v", err)
	}
	if err := x.Delete(ctx, xc, meta); err != nil || f.cross != 0 {
		t.Fatalf("delete cross-connect: %v cross %d", err, f.cross)
	}
	if err := o.Delete(ctx, out, nil); err != nil || f.output[2] != 0 {
		t.Fatalf("delete output: %v output %v", err, f.output)
	}
	// a delete without this boot's record (VPP restarted) sends nothing
	if _, err := o.Create(ctx, out); err != nil {
		t.Fatal(err)
	}
	f.RestartVPP()
	f.Reset()
	if err := o.Delete(ctx, out, nil); err != nil || len(f.CallsNamed("nsim_output_feature_enable_disable")) != 0 {
		t.Fatalf("delete after a VPP restart sent a disable: %v", err)
	}
	for _, bad := range []nsim.CrossConnect{{A: "loop7001", B: "loop7001"}, {A: "loop7001"}} {
		if _, err := x.Create(ctx, bad.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Fatalf("%+v accepted: %v", bad, err)
		}
	}
	if _, err := o.Create(ctx, nsim.Output{Interface: "loop9"}.Proto()); err == nil {
		t.Fatal("foreign interface accepted")
	}
	for _, d := range []scheduler.Descriptor{x, o} {
		if _, err := d.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
			t.Fatalf("%s Retrieve must be write-only: %v", d.Name(), err)
		}
	}
}
