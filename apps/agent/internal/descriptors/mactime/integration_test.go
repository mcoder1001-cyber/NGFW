package mactime_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/mactime"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

func vppctl(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("vppctl", args...).CombinedOutput() //nolint:gosec // fixed evidence commands
	if err != nil {
		t.Fatalf("vppctl %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// TestMactimeOnHost: the mactime descriptors on the host VPP 26.06 — a slot-prefixed device with weekly ranges
// (Retrieve == desired, update = delete + add, no appended ranges), the feature enable applied once per boot on a
// slot loopback (feature_is_enabled readback), and nothing left afterwards. Only w<N>-prefixed devices are touched.
func TestMactimeOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	c := ifacetest.Connect(t)
	ctx := context.Background()

	idx, _ := ifacetest.Loopback(t, c, owner, 30)
	tbl, err := dfkit.DumpInterfaces(ctx, c, owner)
	if err != nil {
		t.Fatal(err)
	}
	name, ok := tbl.Logical(idx)
	if !ok {
		t.Fatalf("loopback %d has no logical name", idx)
	}

	// the readback quirk: an index the device-input arc never reached reports "enabled"
	before, err := mactime.IsEnabled(ctx, c, idx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("feature_is_enabled(device-input, mactime, %d=%s) on a fresh loopback = %v", idx, name, before)

	// device
	dd := mactime.NewDevice(c, owner)
	dev := mactime.Device{Name: "it-dev", MAC: "02:07:00:00:99:01", Drop: false, Ranges: []mactime.Range{
		{Start: 1*86400 + 16*3600, End: 1*86400 + 20*3600}, {Start: 6*86400 + 9*3600, End: 6*86400 + 21*3600},
	}}
	desired := dd.Normalize(dev.Proto())
	meta, err := dd.Create(ctx, desired)
	if err != nil {
		if errors.Is(err, dfkit.ErrPluginNotLoaded) {
			t.Skip("mactime plugin not loaded")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dd.Delete(context.Background(), desired, meta) })
	assertRetrieved(t, dd, mactime.RangeKey("it-dev"), desired)
	upd := dd.Normalize(mactime.Device{Name: "it-dev", MAC: "02:07:00:00:99:01", Drop: true, Ranges: []mactime.Range{{Start: 0, End: 3600}}}.Proto())
	if meta, err = dd.Update(ctx, desired, upd, meta); err != nil {
		t.Fatal(err)
	}
	assertRetrieved(t, dd, mactime.RangeKey("it-dev"), upd)
	t.Log("vppctl show mactime:\n" + vppctl(t, "show", "mactime"))

	// enable, applied once
	store := dfkit.NewMemoryBootStore()
	ed := mactime.NewEnable(c, owner, store)
	en := mactime.Enable{Interface: name}.Proto()
	emeta, err := ed.Create(ctx, en)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ed.Create(ctx, en); err != nil { // a resync re-runs Create: must not stack the feature
		t.Fatal(err)
	}
	assertRetrieved(t, ed, mactime.EnableKey(name), en)
	feats := vppctl(t, "show", "interface", "features", name)
	t.Log("vppctl show interface features " + name + ":\n" + feats)
	if n := strings.Count(feats, "mactime\n") + strings.Count(feats, "mactime \n"); n > 1 {
		t.Errorf("mactime listed %d times on device-input (stacked)", n)
	}
	if err := ed.Delete(ctx, en, emeta); err != nil {
		t.Fatal(err)
	}
	if on, _ := mactime.IsEnabled(ctx, c, idx); on {
		t.Error("mactime still enabled after Delete")
	}
	if kvs, _ := ed.Retrieve(ctx); len(kvs) != 0 {
		t.Errorf("enable retrieved after Delete: %v", kvs)
	}
	if err := dd.Delete(ctx, upd, meta); err != nil {
		t.Fatal(err)
	}
	if kvs, _ := dd.Retrieve(ctx); len(kvs) != 0 {
		t.Errorf("devices retrieved after Delete: %v", kvs)
	}
	t.Log("vppctl show mactime (after delete):\n" + vppctl(t, "show", "mactime"))
}

func assertRetrieved(t *testing.T, d scheduler.Descriptor, key scheduler.Key, want proto.Message) {
	t.Helper()
	kvs, err := d.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range kvs {
		if kv.Key == key {
			if !proto.Equal(kv.Value, want) {
				t.Fatalf("%s retrieved %v, want %v", key, kv.Value, want)
			}
			t.Logf("%s: Retrieve == desired", key)
			return
		}
	}
	t.Fatalf("%s not retrieved (%d objects)", key, len(kvs))
}
