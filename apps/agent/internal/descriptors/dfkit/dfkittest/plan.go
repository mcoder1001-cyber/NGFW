package dfkittest

import (
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/scheduler"
)

// DiffPlan applies the scheduler's diff rule (internal/scheduler/descriptor.go, "Transaction
// semantics" §3) to a desired and a retrieved set: absent → Create, different → Update, owned
// but undesired → Delete. The real planner is P05's; this is what lets the tests show that
// re-applying the same desired state plans nothing.
func DiffPlan(desired, actual []scheduler.KV) scheduler.Plan {
	var p scheduler.Plan
	got := make(map[scheduler.Key]scheduler.KV, len(actual))
	for _, kv := range actual {
		got[kv.Key] = kv
	}
	want := make(map[scheduler.Key]bool, len(desired))
	for _, kv := range desired {
		want[kv.Key] = true
		a, ok := got[kv.Key]
		switch {
		case !ok:
			p.Create = append(p.Create, kv)
		case !proto.Equal(kv.Value, a.Value):
			p.Update = append(p.Update, scheduler.KV{Key: kv.Key, Value: kv.Value, Meta: a.Meta})
		}
	}
	for _, kv := range actual {
		if !want[kv.Key] {
			p.Delete = append(p.Delete, kv)
		}
	}
	return p
}

// PlanString renders a plan for test logs (the idempotency evidence).
func PlanString(p scheduler.Plan) string {
	var b strings.Builder
	for _, kv := range p.Create {
		fmt.Fprintf(&b, "  create %s\n", kv.Key)
	}
	for _, kv := range p.Update {
		fmt.Fprintf(&b, "  update %s\n", kv.Key)
	}
	for _, kv := range p.Delete {
		fmt.Fprintf(&b, "  delete %s\n", kv.Key)
	}
	if b.Len() == 0 {
		return "  (empty plan)\n"
	}
	return b.String()
}

// AssertEmptyPlan retrieves through d and fails unless desired plans nothing (idempotency).
func AssertEmptyPlan(t testing.TB, d scheduler.Descriptor, desired ...scheduler.KV) {
	t.Helper()
	actual, err := d.Retrieve(t.Context())
	if err != nil {
		t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	p := DiffPlan(desired, actual)
	t.Logf("plan for %s after re-apply of %d object(s):\n%s", d.Name(), len(desired), PlanString(p))
	if !p.Empty() {
		t.Fatalf("%s: re-applying the same desired state must plan nothing, got %d op(s)", d.Name(), p.Len())
	}
}

// KV builds a desired KV through the descriptor's KeyOf.
func KV(d scheduler.Descriptor, v proto.Message) scheduler.KV {
	return scheduler.KV{Key: d.KeyOf(v), Value: v}
}

// Find returns the retrieved KV with key k.
func Find(kvs []scheduler.KV, k scheduler.Key) (scheduler.KV, bool) {
	for _, kv := range kvs {
		if kv.Key == k {
			return kv, true
		}
	}
	return scheduler.KV{}, false
}

// MustRetrieve calls Retrieve and fails the test on error.
func MustRetrieve(t testing.TB, d scheduler.Descriptor) []scheduler.KV {
	t.Helper()
	kvs, err := d.Retrieve(t.Context())
	if err != nil {
		t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	return kvs
}

// AssertRetrieved fails unless Retrieve returns key k with a value proto.Equal to want.
func AssertRetrieved(t testing.TB, d scheduler.Descriptor, want scheduler.KV) scheduler.KV {
	t.Helper()
	got, ok := Find(MustRetrieve(t, d), want.Key)
	if !ok {
		t.Fatalf("%s Retrieve: %s missing", d.Name(), want.Key)
	}
	if !proto.Equal(got.Value, want.Value) {
		t.Fatalf("%s Retrieve %s:\n got  %v\n want %v", d.Name(), want.Key, got.Value, want.Value)
	}
	raw, _ := protojson.Marshal(got.Value)
	t.Logf("Retrieve %s = %s (meta %+v)", got.Key, raw, got.Meta)
	return got
}

// AssertAbsent fails when Retrieve returns key k.
func AssertAbsent(t testing.TB, d scheduler.Descriptor, k scheduler.Key) {
	t.Helper()
	if _, ok := Find(MustRetrieve(t, d), k); ok {
		t.Fatalf("%s Retrieve: %s still present", d.Name(), k)
	}
}
