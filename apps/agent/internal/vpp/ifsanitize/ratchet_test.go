package ifsanitize_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/vpp/fake"
	"ngfw/agent/internal/vpp/ifsanitize"
	"ngfw/agent/internal/vpp/ifsanitize/sanitizetest"
)

// pool is the classify table pool as vppinfra keeps it: vector length and free list (bottom first).
type pool struct {
	n    uint32
	free []uint32
}

func (p pool) String() string {
	return fmt.Sprintf("vector %d, free list %d %v", p.n, len(p.free), p.free)
}

func poolOf(m *sanitizetest.Model) pool {
	n, free := m.Pool()
	return pool{n, free}
}

// tables creates n classify tables through the model's own classify_add_del_table (the LIFO pool
// pops them), as another owner would.
func tables(t *testing.T, f *fake.Client, n int) []uint32 {
	t.Helper()
	var out []uint32
	for range n {
		rep, err := classifyapi.NewServiceClient(f).ClassifyAddDelTable(context.Background(), &classifyapi.ClassifyAddDelTable{IsAdd: true, TableIndex: none,
			Nbuckets: 2, MemorySize: 2 << 20, MatchNVectors: 1, NextTableIndex: none, MissNextIndex: none, MaskLen: 16, Mask: []byte("someone-elses-tb")})
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, rep.NewTableIndex)
	}
	return out
}

func dropTables(t *testing.T, f *fake.Client, ids ...uint32) {
	t.Helper()
	for _, id := range ids {
		if _, err := classifyapi.NewServiceClient(f).ClassifyAddDelTable(context.Background(), &classifyapi.ClassifyAddDelTable{IsAdd: false, TableIndex: id}); err != nil {
			t.Fatal(err)
		}
	}
}

// bind binds table (ip4 slot unless noted) everywhere VPP keeps a per-index classify binding.
func bind(t *testing.T, f *fake.Client, idx uint32, in, out, pol, flow uint32) {
	t.Helper()
	ctx, cl, i := context.Background(), classifyapi.NewServiceClient(f), interface_types.InterfaceIndex(idx)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	if in != none {
		_, err := cl.InputACLSetInterface(ctx, &classifyapi.InputACLSetInterface{SwIfIndex: i, IP4TableIndex: none, IP6TableIndex: none, L2TableIndex: in, IsAdd: true})
		must(err)
	}
	if out != none {
		_, err := cl.OutputACLSetInterface(ctx, &classifyapi.OutputACLSetInterface{SwIfIndex: i, IP4TableIndex: out, IP6TableIndex: none, L2TableIndex: none, IsAdd: true})
		must(err)
		_, err = cl.OutputACLSetInterface(ctx, &classifyapi.OutputACLSetInterface{SwIfIndex: i, IP4TableIndex: none, IP6TableIndex: none, L2TableIndex: out, IsAdd: true})
		must(err)
	}
	if pol != none {
		_, err := cl.PolicerClassifySetInterface(ctx, &classifyapi.PolicerClassifySetInterface{SwIfIndex: i, IP4TableIndex: none, IP6TableIndex: none, L2TableIndex: pol, IsAdd: true})
		must(err)
	}
	if flow != none {
		_, err := cl.FlowClassifySetInterface(ctx, &classifyapi.FlowClassifySetInterface{SwIfIndex: i, IP4TableIndex: none, IP6TableIndex: flow, IsAdd: true})
		must(err)
	}
	_, err := cl.ClassifySetInterfaceIPTable(ctx, &classifyapi.ClassifySetInterfaceIPTable{SwIfIndex: i, TableIndex: in})
	must(err)
}

// TestNoPoolRatchet is TD-25: 200 interface create/delete cycles through Acquire and
// BeforeDelete on a model of the vppinfra classify pool (LIFO free list, a vector that never
// shrinks). Before TD-25 every create-phase run popped FreshRun fresh indices "to prove the free
// list empty" and freed them again — each one a permanent hole — so the free list grew by up to
// 8 per create until the placeholder cap (64) failed every create closed (the shared VPP,
// 2026-09-25 05:05: "cap 64 for 122 freed indices seen"). Now a run pops only the probe table and,
// on demand, the free-list entries above a freed index a binding names: every sanitize run leaves
// the pool exactly as it found it (vector length and free-list order), stale bindings to live,
// freed and deeply freed tables are all still cleared, and a clean create costs a bounded number
// of API calls.
func TestNoPoolRatchet(t *testing.T) {
	for _, tc := range []struct {
		name        string
		live, freed int
	}{
		{"shared VPP at 05:05: 122 freed indices, no live table", 0, 122},
		{"4 live tables, 6 freed", 4, 6},
		{"no freed index yet", 3, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, m := setup()
			sw := newPool(20)
			sw.install(f)
			ids := tables(t, f, tc.live+tc.freed)
			live := ids[:tc.live]
			for i := tc.live; i < len(ids); i += 2 { // free out of creation order: every other one, then the rest backwards
				dropTables(t, f, ids[i])
			}
			for i := len(ids) - 1; i >= tc.live; i-- {
				if (i-tc.live)%2 == 1 {
					dropTables(t, f, ids[i])
				}
			}
			start := poolOf(m)
			t.Logf("start: %d live tables, %s", len(live), start)
			ctx := context.Background()
			grew, planted := 0, uint32(0)
			// plant creates tables as another owner would; on a pool without free indices they grow
			// it (not the sanitizer's doing: counted apart)
			plant := func(n int) []uint32 {
				before := poolOf(m)
				tb := tables(t, f, n)
				planted += poolOf(m).n - before.n
				return tb
			}
			snap := ifsanitize.Snapshot()
			var clean []int // API calls of the clean creates
			for cycle := range 200 {
				before, calls := poolOf(m), len(f.Calls())
				idx, err := ifsanitize.Acquire(ctx, f, "w1", fmt.Sprintf("host-w1c%d", cycle),
					func() (uint32, error) { return sw.get(), nil },
					func(i uint32) error { sw.put(i); return nil })
				if err != nil {
					t.Fatalf("cycle %d: create failed: %v (%s)", cycle, err, poolOf(m))
				}
				if d := m.Dirty(idx); d != "" {
					t.Fatalf("cycle %d: sw_if_index %d created with inherited %s", cycle, idx, d)
				}
				if after := poolOf(m); !samePool(before, after) {
					// the one index a VPP without a free list gives the probe table
					if len(before.free) != 0 || after.n != before.n+1 || grew > 0 {
						t.Fatalf("cycle %d: the create changed the classify pool:\n before %s\n after  %s", cycle, before, after)
					}
					grew++
				}
				if cycle == 0 || cycle%4 == 1 || cycle%4 == 2 { // after a proper delete: nothing inherited
					clean = append(clean, len(f.Calls())-calls)
				}
				switch cycle % 4 {
				case 0: // clean interface, deleted properly
				case 1: // bindings to a live table, deleted properly
					if len(live) > 0 {
						bind(t, f, idx, live[0], live[0], live[0], live[0])
					}
				case 2: // a raw delete leaves bindings to a table that is deleted next (top of the free list)
					tb := plant(1)
					bind(t, f, idx, tb[0], tb[0], tb[0], tb[0])
					m.DeleteInterface(idx)
					sw.put(idx)
					dropTables(t, f, tb...)
					continue
				case 3: // ... to the deepest of three tables deleted in creation order
					tb := plant(3)
					bind(t, f, idx, tb[0], tb[0], tb[1], tb[0])
					m.DeleteInterface(idx)
					sw.put(idx)
					dropTables(t, f, tb...)
					continue
				}
				before = poolOf(m)
				if err := ifsanitize.BeforeDelete(ctx, f, idx, fmt.Sprintf("host-w1c%d", cycle)); err != nil {
					t.Fatalf("cycle %d: before delete: %v", cycle, err)
				}
				if d := m.Dirty(idx); d != "" {
					t.Fatalf("cycle %d: BeforeDelete left %s", cycle, d)
				}
				if after := poolOf(m); !samePool(before, after) {
					t.Fatalf("cycle %d: the delete-phase run changed the classify pool:\n before %s\n after  %s", cycle, before, after)
				}
				m.DeleteInterface(idx)
				sw.put(idx)
			}
			end, s := poolOf(m), ifsanitize.Snapshot()
			cleared, freed := sum(s.Cleared)-sum(snap.Cleared), sum(s.Freed)-sum(snap.Freed)
			t.Logf("end after 200 cycles: %s (sanitizer growth %d, planted tables' growth %d); stale bindings freed through placeholders %d, bindings to live tables cleared %d; API calls per clean create %d–%d",
				end, grew, planted, freed, cleared, slices.Min(clean), slices.Max(clean))
			if g := grew + int(planted); end.n != start.n+uint32(g) || len(end.free) != len(start.free)+g || !sameSet(end.free, start.free, g) {
				t.Fatalf("the classify pool grew over 200 cycles:\n start %s\n end   %s", start, end)
			}
			if freed < 50*5 || (len(live) > 0 && cleared < 50*4) {
				t.Fatalf("stale bindings freed %d (want ≥ 250), cleared %d", freed, cleared)
			}
			if slices.Max(clean) > 45 {
				t.Fatalf("a clean create costs %d API calls", slices.Max(clean))
			}
		})
	}
}

func samePool(a, b pool) bool { return a.n == b.n && slices.Equal(a.free, b.free) }

// sameSet reports whether b plus up to extra more entries equals a as a set.
func sameSet(a, b []uint32, extra int) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	if extra == 0 {
		return slices.Equal(x, y)
	}
	for _, v := range y {
		if !slices.Contains(x, v) {
			return false
		}
	}
	return len(x) == len(y)+extra
}

func sum(m map[string]int64) int64 {
	var n int64
	for _, v := range m {
		n += v
	}
	return n
}
