package objects

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

// createN creates n address objects through the descriptor, one Create each (what one transaction of the scheduler
// does), then Retrieves the family once (the scheduler's verification), and returns the elapsed time.
func createN(t *testing.T, dir string, n int) time.Duration {
	t.Helper()
	rt, err := Open(Config{StateDir: dir, Owner: "w3", Lookup: NetLookup([]string{"127.0.0.1:9"}), Log: slogTo(&logBuf{})})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	reg := scheduler.NewRegistry()
	Register(reg, rt)
	d, _ := reg.Get(AddressName)
	ctx := context.Background()
	start := time.Now()
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("h%05d", i)
		v, err := Value(KindAddresses, name, &vrxv1.AddressObject{Type: ptr("host"), Address: ptr(fmt.Sprintf("10.%d.%d.%d", i/65536, i/256%256, i%256)), Tags: []string{}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil || len(kvs) != n {
		t.Fatalf("retrieve: %d objects, %v", len(kvs), err)
	}
	return time.Since(start)
}

// F1 (review): the cost of one transaction of N objects. VRX_OBJECTS_SCALE="1000,2000,4000" prints the times (the
// before/after evidence); VRX_OBJECTS_SCALE_DIR runs it in that directory (a disk, not tmpfs).
func TestStoreScaleReport(t *testing.T) {
	sizes := os.Getenv("VRX_OBJECTS_SCALE")
	if sizes == "" {
		t.Skip("set VRX_OBJECTS_SCALE=1000,2000,4000 to print the transaction cost per store size")
	}
	for _, s := range strings.Split(sizes, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		if base := os.Getenv("VRX_OBJECTS_SCALE_DIR"); base != "" {
			if dir, err = os.MkdirTemp(base, "objects-scale-"); err != nil {
				t.Fatal(err)
			}
			defer func(d string) { _ = os.RemoveAll(d) }(dir) //nolint:gosec // a temp dir this test created
		}
		d := createN(t, dir, n)
		t.Logf("%5d objects: %8.3f s  (%.3f ms per object)  dir=%s", n, d.Seconds(), float64(d.Microseconds())/1000/float64(n), dir)
	}
}

// Review F1: a transaction of 5 000 objects (5 000 Creates + the verification Retrieve) takes well under 5 s, the
// store is written once (dirty until the Retrieve, clean after), and non-FQDN objects never touch the resolver.
func TestStoreScale5000(t *testing.T) {
	dir := t.TempDir()
	if d := createN(t, dir, 5000); d > 5*time.Second {
		t.Fatalf("5 000 Creates + Retrieve took %v, want < 5 s", d)
	} else {
		t.Logf("5 000 objects: %v", d)
	}
	st, err := OpenStore(dir+"/objects-w3.json", slogTo(&logBuf{}))
	if err != nil || len(st.Snapshot().GetAddresses()) != 5000 {
		t.Fatalf("persisted store: %v", err)
	}
}

func TestStoreWritesCoalescedAndFQDNOnlyOnChange(t *testing.T) {
	dir := t.TempDir()
	rt, err := Open(Config{StateDir: dir, Owner: "w3", Lookup: NetLookup([]string{"127.0.0.1:9"}), Log: slogTo(&logBuf{})})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	reg := scheduler.NewRegistry()
	Register(reg, rt)
	d, _ := reg.Get(AddressName)
	ctx := context.Background()
	put := func(name string, a *vrxv1.AddressObject) {
		t.Helper()
		v, _ := Value(KindAddresses, name, a)
		if _, err := d.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	path := dir + "/objects-w3.json"
	for i := 0; i < 50; i++ {
		put(fmt.Sprintf("h%d", i), &vrxv1.AddressObject{Type: ptr("host"), Address: ptr(fmt.Sprintf("192.0.2.%d", i))})
	}
	if _, err := os.Stat(path); err == nil || !rt.Store().Dirty() {
		t.Fatal("a Create wrote the store (want: memory only until the flush)")
	}
	rt.res.mu.Lock()
	dirty, tracked := rt.res.dirty, len(rt.res.objects)
	rt.res.mu.Unlock()
	if dirty || tracked != 0 {
		t.Fatalf("non-FQDN objects touched the resolver: dirty=%v tracked=%d", dirty, tracked)
	}
	if _, err := d.Retrieve(ctx); err != nil || rt.Store().Dirty() {
		t.Fatalf("Retrieve did not flush: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	// an FQDN object is tracked; changing its name moves it; a type change untracks it
	put("cdn", &vrxv1.AddressObject{Type: ptr("fqdn"), Fqdn: ptr("cdn.w3.test")})
	put("cdn", &vrxv1.AddressObject{Type: ptr("fqdn"), Fqdn: ptr("cdn2.w3.test")})
	if st := rt.FQDNStates(); len(st) != 1 || st[0].FQDN != "cdn2.w3.test" {
		t.Fatalf("tracked: %+v", st)
	}
	put("cdn", &vrxv1.AddressObject{Type: ptr("host"), Address: ptr("192.0.2.200")})
	if st := rt.FQDNStates(); len(st) != 0 {
		t.Fatalf("still tracked after the type change: %+v", st)
	}
	// without a Retrieve the timer writes within FlushDelay
	put("late", &vrxv1.AddressObject{Type: ptr("host"), Address: ptr("192.0.2.201")})
	deadline := time.Now().Add(5 * time.Second)
	for rt.Store().Dirty() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if rt.Store().Dirty() {
		t.Fatal("the flush timer did not write the store")
	}
}

// The same through the scheduler (plan Retrieve → N Creates → verification Retrieve): what one commit of the objects
// domain costs in the agent. VRX_OBJECTS_SCALE as above.
func TestStoreScaleSchedulerReport(t *testing.T) {
	sizes := os.Getenv("VRX_OBJECTS_SCALE")
	if sizes == "" {
		t.Skip("set VRX_OBJECTS_SCALE=1000,2000,4000")
	}
	for _, s := range strings.Split(sizes, ",") {
		n, _ := strconv.Atoi(strings.TrimSpace(s))
		rt, err := Open(Config{StateDir: t.TempDir(), Owner: "w3", Lookup: NetLookup([]string{"127.0.0.1:9"}), Log: slogTo(&logBuf{})})
		if err != nil {
			t.Fatal(err)
		}
		reg := scheduler.NewRegistry()
		Register(reg, rt)
		doc := &vrxv1.ObjectsConfig{Addresses: map[string]*vrxv1.AddressObject{}}
		for i := 0; i < n; i++ {
			doc.Addresses[fmt.Sprintf("h%05d", i)] = &vrxv1.AddressObject{Type: ptr("host"), Address: ptr(fmt.Sprintf("10.%d.%d.%d", i/65536, i/256%256, i%256)), Tags: []string{}}
		}
		kvs := kvsOf(t, doc)
		start := time.Now()
		res := scheduler.New(reg, nil).ApplyWith(context.Background(), kvs, scheduler.Only(DescriptorNames()...), scheduler.ApplyOptions{})
		d := time.Since(start)
		if res.Outcome != scheduler.OutcomeApplied || res.Summary.Created != n {
			t.Fatalf("%d: %v %+v %v", n, res.Outcome, res.Summary, res.Err)
		}
		t.Logf("%5d objects, one scheduler transaction: %8.3f s", n, d.Seconds())
		rt.Close()
	}
}
