// Package df7test holds the unit- and integration-test helpers of the DF-7 descriptor packages:
// a fake VPP with a stateful interface table, the scheduler's diff rule (so tests can show that
// re-applying the same desired state plans nothing) and, for the host tests, a govpp connection
// plus slot-prefixed loopbacks, addresses and tables that are removed in t.Cleanup
// (docs/lab/shared-host-rules.md).
package df7test

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

// Owner is the owner the unit tests use; Other is another agent on the same VPP.
const (
	Owner = "w0"
	Other = "w9"
)

// FakeIf is one interface of the fake VPP.
type FakeIf struct {
	Index uint32
	Name  string
	Tag   string
}

// Fake is a fake VPP client with a mutable interface table (served on sw_interface_dump).
type Fake struct {
	*fake.Client
	mu   sync.Mutex
	ifs  map[uint32]FakeIf
	boot uint32
}

// NewFake returns a fake with the control ping reply registered and the interfaces given.
// Default table: loop0 (1, ours), loop1 (2, ours), loop9 (3, other owner's), eth0 (4, untagged).
func NewFake(ifs ...FakeIf) *Fake {
	if len(ifs) == 0 {
		ifs = []FakeIf{
			{Index: 0, Name: "local0"},
			{Index: 1, Name: "loop0", Tag: Owner + ":loop0"},
			{Index: 2, Name: "loop1", Tag: Owner + ":loop1"},
			{Index: 3, Name: "loop9", Tag: Other + ":loop9"},
			{Index: 4, Name: "eth0"},
		}
	}
	f := &Fake{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), ifs: map[uint32]FakeIf{}, boot: 4242}
	for _, i := range ifs {
		f.ifs[i.Index] = i
	}
	f.On("show_threads", func(api.Message) ([]api.Message, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return []api.Message{&vlib.ShowThreadsReply{Count: 1, ThreadData: []vlib.ThreadData{{ID: 0, Name: "vpp_main", PID: f.boot}}}}, nil
	})
	f.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		idx := make([]uint32, 0, len(f.ifs))
		for k := range f.ifs {
			idx = append(idx, k)
		}
		sort.Slice(idx, func(a, b int) bool { return idx[a] < idx[b] })
		out := make([]api.Message, 0, len(idx))
		for _, k := range idx {
			i := f.ifs[k]
			out = append(out, &interfaces.SwInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(i.Index), InterfaceName: i.Name, Tag: i.Tag})
		}
		return out, nil
	})
	return f
}

// Reboot simulates a VPP restart for the boot-identity records (D-076): show_threads reports
// a new main-thread PID.
func (f *Fake) Reboot() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.boot++
}

// AddIf adds an interface to the fake's table.
func (f *Fake) AddIf(i FakeIf) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ifs[i.Index] = i
}

// OK registers a zero-retval reply of type reply for request name.
func (f *Fake) OK(name string, reply api.Message) { f.Reply(name, reply) }

// Last returns the last request named name (fails the test when there is none).
func Last[T api.Message](t testing.TB, f *Fake, name string) T {
	t.Helper()
	calls := f.CallsNamed(name)
	if len(calls) == 0 {
		t.Fatalf("no %s request sent", name)
	}
	m, ok := calls[len(calls)-1].(T)
	if !ok {
		t.Fatalf("%s request is %T", name, calls[len(calls)-1])
	}
	return m
}

// DiffPlan applies the scheduler's diff rule (internal/scheduler/descriptor.go, "Transaction
// semantics" §3): absent → Create, different → Update, owned but undesired → Delete.
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

// PlanString renders a plan for logs ("(empty plan)" when empty).
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

// Desired builds the desired KV of v through d.
func Desired(d scheduler.Descriptor, v proto.Message) scheduler.KV {
	return scheduler.KV{Key: d.KeyOf(v), Value: v}
}

// AssertEmptyPlan retrieves through d and checks that desired plans nothing (idempotency):
// Retrieve returns exactly the desired objects with equal values.
func AssertEmptyPlan(t testing.TB, d scheduler.Descriptor, desired ...scheduler.KV) []scheduler.KV {
	t.Helper()
	actual, err := d.Retrieve(t.Context())
	if err != nil {
		t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	p := DiffPlan(desired, actual)
	t.Logf("%s: re-apply plan for %d desired object(s):\n%s", d.Name(), len(desired), PlanString(p))
	if !p.Empty() {
		for _, kv := range p.Update {
			for _, a := range actual {
				if a.Key == kv.Key {
					t.Logf("  %s\n    want %v\n    got  %v", kv.Key, kv.Value, a.Value)
				}
			}
		}
		t.Fatalf("%s: re-applying the same desired state must plan nothing, got %d op(s)", d.Name(), p.Len())
	}
	return actual
}

// Keys returns the keys of kvs, sorted.
func Keys(kvs []scheduler.KV) []string {
	out := make([]string, 0, len(kvs))
	for _, kv := range kvs {
		out = append(out, string(kv.Key))
	}
	sort.Strings(out)
	return out
}
