package acl

import (
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/scheduler"
)

// diffPlan applies the scheduler's diff rule (internal/scheduler/descriptor.go, "Transaction
// semantics" §3) to a desired and a retrieved set: absent → Create, different → Update, owned
// but undesired → Delete. The real planner lands in P05; this is what lets the tests show that
// re-applying the same desired state plans nothing.
func diffPlan(desired, actual []scheduler.KV) scheduler.Plan {
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

func planString(p scheduler.Plan) string {
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

// assertEmptyPlan retrieves through d and checks that desired plans nothing (idempotency).
func assertEmptyPlan(t *testing.T, d scheduler.Descriptor, desired ...scheduler.KV) {
	t.Helper()
	actual, err := d.Retrieve(t.Context())
	if err != nil {
		t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	p := diffPlan(desired, actual)
	t.Logf("plan for %s after re-apply of %d object(s):\n%s", d.Name(), len(desired), planString(p))
	if !p.Empty() {
		t.Fatalf("%s: re-applying the same desired state must plan nothing, got %d op(s)", d.Name(), p.Len())
	}
}

func kv(d scheduler.Descriptor, v proto.Message) scheduler.KV {
	return scheduler.KV{Key: d.KeyOf(v), Value: v}
}
