package scheduler

import (
	"fmt"
	"slices"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// td21Live builds n live objects: a/i0 (alias if/i0) ← b/i<k> (deps on if/i0 or a/i0) and
// c/i<k> ← b/i<k>, a wide fan-out so dependents(a/i0) returns ~n-1 keys.
func td21Live(n int) map[Key]KV {
	live := map[Key]KV{}
	put := func(d string, o proto.Message) {
		k := Join(d, str(o, "name"))
		live[k] = KV{Key: k, Value: o, Meta: memMeta{}}
	}
	put("a", obj("i0", "v"))
	for i := 1; i < n/2; i++ {
		dep := "a/i0"
		if i%2 == 0 {
			dep = "if/i0"
		}
		put("b", obj(fmt.Sprintf("i%d", i), "v", dep))
		put("c", obj(fmt.Sprintf("i%d", i), "v", fmt.Sprintf("b/i%d", i)))
	}
	return live
}

// dependentsRef is the pre-TD-21 algorithm, kept as the oracle for equivalence.
func dependentsRef(x *executor, key Key, value proto.Message) []Key {
	users := make(map[Key]KV)
	provides := func(k Key, v proto.Message) map[Key]bool {
		out := map[Key]bool{k: true}
		if kp, ok := x.descriptor(k).(KeyProvider); ok && v != nil {
			for _, a := range kp.ProvidedKeys(v) {
				out[a] = true
			}
		}
		return out
	}
	type item struct {
		k Key
		v proto.Message
	}
	frontier := []item{{key, value}}
	for len(frontier) > 0 {
		cur := frontier[0]
		frontier = frontier[1:]
		provided := provides(cur.k, cur.v)
		for _, k := range sortedKeys(x.live) {
			if _, seen := users[k]; seen || k == key {
				continue
			}
			kv := x.live[k]
			for _, dep := range x.descriptor(k).Dependencies(kv.Value) {
				if provided[dep.Key] {
					users[k] = kv
					frontier = append(frontier, item{k, kv.Value})
					break
				}
			}
		}
	}
	order, _ := x.s.topo(users)
	return order
}

// topoRef is the pre-TD-21 ready-set loop (re-sort per pop) over a plain dependency map.
func topoRef(s *Scheduler, nodes map[Key]KV) []Key {
	stage, rank := s.stages(), map[string]int{}
	for i, n := range s.reg.Names() {
		rank[n] = i
	}
	less := func(a, b Key) bool {
		if sa, sb := stage[a.Descriptor()], stage[b.Descriptor()]; sa != sb {
			return sa < sb
		}
		if ra, rb := rank[a.Descriptor()], rank[b.Descriptor()]; ra != rb {
			return ra < rb
		}
		return a < b
	}
	alias := s.aliases(nodes)
	deps, users := map[Key]map[Key]bool{}, map[Key][]Key{}
	for k, kv := range nodes {
		deps[k] = map[Key]bool{}
		d, _ := s.reg.ForKey(k)
		for _, dep := range d.Dependencies(kv.Value) {
			t := dep.Key
			if a, ok := alias[t]; ok {
				t = a
			}
			if _, ok := nodes[t]; !ok || t == k || deps[k][t] {
				continue
			}
			deps[k][t] = true
			users[t] = append(users[t], k)
		}
	}
	var ready, order []Key
	for k := range nodes {
		if len(deps[k]) == 0 {
			ready = append(ready, k)
		}
	}
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
		k := ready[0]
		ready = ready[1:]
		order = append(order, k)
		for _, u := range users[k] {
			delete(deps[u], k)
			if len(deps[u]) == 0 {
				ready = append(ready, u)
			}
		}
	}
	return order
}

func TestTD21DependentsAndTopoMatchReference(t *testing.T) {
	s, _ := fixture(t)
	for _, n := range []int{2, 7, 40, 301} {
		live := td21Live(n)
		x := &executor{s: s, live: live}
		root := live["a/i0"]
		got, want := x.dependents(root.Key, root.Value), dependentsRef(x, root.Key, root.Value)
		if !slices.Equal(got, want) {
			t.Fatalf("n=%d dependents\n got %v\nwant %v", n, got, want)
		}
		order, cycle := s.topo(live)
		if len(cycle) != 0 || !slices.Equal(order, topoRef(s, live)) {
			t.Fatalf("n=%d topo order differs from reference (cycle %v)", n, cycle)
		}
	}
}

// TestTD21Scale: 20k live objects, one dependents walk + one topo sort. Pre-TD-21 this was
// quadratic (4000 objects took 13.7 s end to end); now it is well under a second.
func TestTD21Scale(t *testing.T) {
	if testing.Short() {
		t.Skip("scale test")
	}
	s, _ := fixture(t)
	live := td21Live(20000)
	x := &executor{s: s, live: live}
	start := time.Now()
	deps := x.dependents("a/i0", live["a/i0"].Value)
	order, _ := s.topo(live)
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("dependents+topo over %d objects took %v", len(live), el)
	}
	if len(deps) != len(live)-1 || len(order) != len(live) {
		t.Fatalf("dependents %d, order %d, live %d", len(deps), len(order), len(live))
	}
}

func BenchmarkTD21Dependents(b *testing.B) {
	for _, n := range []int{1000, 4000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s, _ := fixture(&testing.T{})
			live := td21Live(n)
			x := &executor{s: s, live: live}
			for b.Loop() {
				x.dependents("a/i0", live["a/i0"].Value)
			}
		})
	}
}

func BenchmarkTD21DependentsRef(b *testing.B) {
	s, _ := fixture(&testing.T{})
	live := td21Live(1000)
	x := &executor{s: s, live: live}
	for b.Loop() {
		dependentsRef(x, "a/i0", live["a/i0"].Value)
	}
}
