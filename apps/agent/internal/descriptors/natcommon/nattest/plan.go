package nattest

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// Apply mimics one scheduler pass for a single descriptor: Create what is missing, Update
// what differs (Delete+Create on ErrRecreate), Delete owned leftovers. It returns the number
// of operations, so `Apply(desired) == 0` is the "same desired state twice → empty plan"
// check. Works against the fake and the host alike.
func Apply(t testing.TB, d scheduler.Descriptor, desired ...proto.Message) int {
	t.Helper()
	ctx := context.Background()
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatalf("%s retrieve: %v", d.Name(), err)
	}
	have := map[scheduler.Key]scheduler.KV{}
	for _, kv := range actual {
		have[kv.Key] = kv
	}
	ops := 0
	seen := map[scheduler.Key]bool{}
	for _, obj := range desired {
		k := d.KeyOf(obj)
		seen[k] = true
		cur, ok := have[k]
		switch {
		case !ok:
			if _, err := d.Create(ctx, obj); err != nil {
				t.Fatalf("%s create %s: %v", d.Name(), k, err)
			}
			ops++
		case !proto.Equal(cur.Value, obj):
			if _, err := d.Update(ctx, cur.Value, obj, cur.Meta); err != nil {
				if !errors.Is(err, scheduler.ErrRecreate) {
					t.Fatalf("%s update %s: %v", d.Name(), k, err)
				}
				if err := d.Delete(ctx, cur.Value, cur.Meta); err != nil {
					t.Fatalf("%s recreate/delete %s: %v", d.Name(), k, err)
				}
				if _, err := d.Create(ctx, obj); err != nil {
					t.Fatalf("%s recreate/create %s: %v", d.Name(), k, err)
				}
			}
			ops++
		}
	}
	for k, kv := range have {
		if !seen[k] {
			if err := d.Delete(ctx, kv.Value, kv.Meta); err != nil {
				t.Fatalf("%s delete %s: %v", d.Name(), k, err)
			}
			ops++
		}
	}
	return ops
}

// Keys returns the keys Retrieve reports, in dump order.
func Keys(t testing.TB, d scheduler.Descriptor) []string {
	t.Helper()
	kvs, err := d.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("%s retrieve: %v", d.Name(), err)
	}
	out := make([]string, 0, len(kvs))
	for _, kv := range kvs {
		out = append(out, string(kv.Key))
	}
	return out
}

// AssertPlan is the idempotency check on a real or fake VPP: Retrieve must equal the desired
// set exactly (same keys, proto.Equal values) so a re-apply plans nothing.
func AssertPlan(t testing.TB, d scheduler.Descriptor, desired ...proto.Message) {
	t.Helper()
	kvs, err := d.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("%s retrieve: %v", d.Name(), err)
	}
	have := map[scheduler.Key]proto.Message{}
	for _, kv := range kvs {
		have[kv.Key] = kv.Value
	}
	if len(have) != len(desired) {
		t.Fatalf("%s: retrieved %d objects, desired %d: %v", d.Name(), len(have), len(desired), kvs)
	}
	for _, o := range desired {
		got, ok := have[d.KeyOf(o)]
		if !ok {
			t.Fatalf("%s: %s missing after apply; have %v", d.Name(), d.KeyOf(o), kvs)
		}
		if !proto.Equal(got, o) {
			t.Fatalf("%s: %s differs:\n got  %v\n want %v", d.Name(), d.KeyOf(o), got, o)
		}
	}
	t.Logf("plan for %s after re-apply: empty (%d objects converged)", d.Name(), len(desired))
}

// CreateAll creates every object on the host and registers a best-effort deletion in Cleanup
// (by the Meta Retrieve reports at that time), so a failing test leaves nothing behind.
func CreateAll(ctx context.Context, t testing.TB, d scheduler.Descriptor, objs ...proto.Message) {
	t.Helper()
	for _, o := range objs {
		meta, err := d.Create(ctx, o)
		if err != nil {
			t.Fatalf("%s create %s: %v", d.Name(), d.KeyOf(o), err)
		}
		key := d.KeyOf(o)
		t.Cleanup(func() {
			cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			// Prefer what Retrieve reports now; if Retrieve does not see the object (a
			// descriptor bug under test), still delete it with the Create-time Meta so the
			// shared VPP is left clean.
			if kvs, err := d.Retrieve(cctx); err == nil {
				for _, kv := range kvs {
					if kv.Key == key {
						_ = d.Delete(cctx, kv.Value, kv.Meta)
						return
					}
				}
			}
			_ = d.Delete(cctx, o, meta)
		})
	}
}

// DeleteAll deletes every owned object Retrieve reports and asserts nothing is left.
func DeleteAll(ctx context.Context, t testing.TB, d scheduler.Descriptor) {
	t.Helper()
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatalf("%s retrieve: %v", d.Name(), err)
	}
	for _, kv := range kvs {
		if err := d.Delete(ctx, kv.Value, kv.Meta); err != nil {
			t.Fatalf("%s delete %s: %v", d.Name(), kv.Key, err)
		}
	}
	if left, err := d.Retrieve(ctx); err != nil || len(left) != 0 {
		t.Fatalf("%s after delete: %+v %v", d.Name(), left, err)
	}
}

// AssertWriteOnly checks the D-063 contract of a descriptor without a VPP dump: Retrieve
// reports ErrRetrieveUnsupported (never echoed desired state).
func AssertWriteOnly(t testing.TB, d scheduler.Descriptor) {
	t.Helper()
	kvs, err := d.Retrieve(context.Background())
	if !errors.Is(err, natcommon.ErrRetrieveUnsupported) || len(kvs) != 0 {
		t.Fatalf("%s: want ErrRetrieveUnsupported (write-only, D-063), got %v %v", d.Name(), kvs, err)
	}
	t.Logf("%s is write-only: Retrieve → ErrRetrieveUnsupported", d.Name())
}

// CreateWriteOnly creates a write-only object twice (the reconciler re-applies it on every
// resync, so Create must be idempotent) and registers a best-effort Delete in Cleanup.
func CreateWriteOnly(ctx context.Context, t testing.TB, d scheduler.Descriptor, obj proto.Message) {
	t.Helper()
	var meta any
	for i := 0; i < 2; i++ {
		m, err := d.Create(ctx, obj)
		if err != nil {
			t.Fatalf("%s create #%d %s: %v", d.Name(), i, d.KeyOf(obj), err)
		}
		meta = m
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = d.Delete(cctx, obj, meta)
	})
	t.Logf("%s: create re-applied twice without error (idempotent)", d.Name())
}
