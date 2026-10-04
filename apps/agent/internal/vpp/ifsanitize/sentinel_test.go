package ifsanitize_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.fd.io/govpp/api"

	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/internal/vpp/fake"
	"ngfw/agent/internal/vpp/ifsanitize"
	"ngfw/agent/internal/vpp/ifsanitize/sanitizetest"
)

// sentinelVPP is a fake VPP with the stateful classify table pool of sanitizetest (LIFO free list).
func sentinelVPP(t *testing.T) (*fake.Client, *sanitizetest.Model) {
	t.Helper()
	f := fake.New()
	m := sanitizetest.NewModel()
	m.Install(f)
	return f, m
}

// deletes are the classify_add_del_table deletes f received, in order.
func deletes(f *fake.Client) []uint32 {
	var out []uint32
	for _, c := range f.CallsNamed("classify_add_del_table") {
		if r := c.(*classifyapi.ClassifyAddDelTable); !r.IsAdd {
			out = append(out, r.TableIndex)
		}
	}
	return out
}

func ensure(t *testing.T, f *fake.Client) ifsanitize.SentinelReport {
	t.Helper()
	rep, err := ifsanitize.EnsureSentinel(context.Background(), f)
	if err != nil {
		t.Fatalf("EnsureSentinel: %v (%s)", err, rep)
	}
	return rep
}

// After a VPP start the pool is empty: one create, index 0, the sentinel's geometry (1 bucket,
// miss → drop), nothing deleted. A second run (a reconnect without a VPP restart) only reads.
func TestSentinelFreshVPPAndIdempotent(t *testing.T) {
	f, m := sentinelVPP(t)
	rep := ensure(t, f)
	if rep.State != ifsanitize.SentinelCreated || len(rep.Pops) != 0 || !m.Tables[0] || m.Created != 1 {
		t.Fatalf("fresh: %s, tables %v, created %d", rep, m.Tables, m.Created)
	}
	add := f.CallsNamed("classify_add_del_table")[0].(*classifyapi.ClassifyAddDelTable)
	if add.Nbuckets != 1 || add.MissNextIndex != ifsanitize.NoIndex || add.NextTableIndex != ifsanitize.NoIndex || add.MatchNVectors != 1 || add.MaskLen != 16 {
		t.Fatalf("sentinel create %+v", add)
	}
	f.Reset()
	rep = ensure(t, f)
	if rep.State != ifsanitize.SentinelPresent || m.Created != 1 || len(f.CallsNamed("classify_add_del_table")) != 0 {
		t.Fatalf("second run: %s, created %d, calls %v", rep, m.Created, f.Calls())
	}
}

// The agent restarted without VPP: tables were created and freed, index 0 is deep in the free list.
// The run pops until 0 comes back, keeps it, deletes every other pop in reverse creation order —
// the free list is then exactly as before, minus 0 — and never touches the live table.
func TestSentinelPopsIndexZeroBackAndRestoresFreeList(t *testing.T) {
	f, m := sentinelVPP(t)
	for i := uint32(0); i < 8; i++ {
		m.Tables[i] = true
	}
	for _, i := range []uint32{0, 3, 1, 7, 2, 6} { // freed in this order; 4 and 5 stay live (another owner)
		m.DeleteTable(i)
	}
	_, before := m.Pool()
	rep := ensure(t, f)
	if rep.State != ifsanitize.SentinelCreated || !slices.Equal(rep.Pops, []uint32{6, 2, 7, 1, 3}) || len(rep.Left) != 0 {
		t.Fatalf("report %s pops %v", rep, rep.Pops)
	}
	if got := deletes(f); !slices.Equal(got, []uint32{3, 1, 7, 2, 6}) {
		t.Fatalf("deletes %v, want reverse creation order", got)
	}
	_, after := m.Pool()
	if want := slices.DeleteFunc(slices.Clone(before), func(i uint32) bool { return i == 0 }); !slices.Equal(after, want) {
		t.Fatalf("free list %v → %v, want %v", before, after, want)
	}
	if !m.Tables[0] || !m.Tables[4] || !m.Tables[5] || len(m.Tables) != 3 {
		t.Fatalf("tables %v", m.Tables)
	}
	f.Reset()
	if rep := ensure(t, f); rep.State != ifsanitize.SentinelPresent {
		t.Fatalf("idempotence: %s", rep)
	}
}

// Index 0 is a live table of another client (an ACL-plugin MACIP table, a slot's classify table):
// nothing is created and nothing is deleted.
func TestSentinelIndexZeroTakenIsLeftAlone(t *testing.T) {
	f, m := sentinelVPP(t)
	m.Tables[0], m.Tables[1] = true, true
	rep := ensure(t, f)
	if rep.State != ifsanitize.SentinelTaken || rep.Foreign == "" || m.Created != 0 || len(deletes(f)) != 0 || !m.Tables[0] {
		t.Fatalf("taken: %s, created %d, deletes %v", rep, m.Created, deletes(f))
	}
}

// Index 0 is taken by another client during the run: the first pop above every index seen re-reads
// classify_table_ids, sees a foreign table 0, stops, and deletes its own pops again.
func TestSentinelRaceLostStopsAndCleansUp(t *testing.T) {
	f, m := sentinelVPP(t)
	m.Tables[0], m.Tables[1], m.Tables[2] = true, true, true
	m.DeleteTable(2)
	m.DeleteTable(0)                          // free list [.., 2, 0]: index 0 would pop first
	ids := m.Handlers()["classify_table_ids"] // the model's own handler
	reads := 0
	f.On("classify_table_ids", func(req api.Message) ([]api.Message, error) {
		reads++
		if reads == 1 { // right after the first read another client pops index 0 (and 2, the free list is empty)
			defer func() {
				_, _ = m.Handlers()["classify_add_del_table"](&classifyapi.ClassifyAddDelTable{IsAdd: true, MaskLen: 16, Mask: make([]byte, 16)})
				_, _ = m.Handlers()["classify_add_del_table"](&classifyapi.ClassifyAddDelTable{IsAdd: true, MaskLen: 16, Mask: make([]byte, 16)})
			}()
		}
		return ids(req)
	})
	rep, err := ifsanitize.EnsureSentinel(context.Background(), f)
	if err != nil || rep.State != ifsanitize.SentinelTaken || !slices.Equal(rep.Pops, []uint32{3}) || !slices.Equal(deletes(f), []uint32{3}) {
		t.Fatalf("race: %s pops %v deletes %v err %v", rep, rep.Pops, deletes(f), err)
	}
	if !m.Tables[0] || m.Tables[3] {
		t.Fatalf("tables %v", m.Tables)
	}
}

// A free list deeper than MaxSentinelPops above index 0: capped, every pop deleted again.
func TestSentinelCapped(t *testing.T) {
	defer ifsanitize.SetMaxSentinelPops(3)()
	f, m := sentinelVPP(t)
	for i := uint32(0); i < 6; i++ {
		m.Tables[i] = true
	}
	for i := uint32(0); i < 6; i++ {
		m.DeleteTable(i)
	}
	rep, err := ifsanitize.EnsureSentinel(context.Background(), f)
	if !errors.Is(err, ifsanitize.ErrSentinelCapped) || rep.State != ifsanitize.SentinelCapped || len(rep.Pops) != 3 || len(m.Tables) != 0 {
		t.Fatalf("capped: %s err %v tables %v", rep, err, m.Tables)
	}
}

// A pop that is no longer the sentinel signature when the run deletes it is left in VPP, and the
// run reports it; table_ids / table_info / add failures are errors, never a silent success.
func TestSentinelNeverDeletesForeignAndReportsFailures(t *testing.T) {
	f, m := sentinelVPP(t)
	m.Tables[0], m.Tables[1] = true, true
	m.DeleteTable(0)
	m.DeleteTable(1)
	info := m.Handlers()["classify_table_info"]
	f.On("classify_table_info", func(req api.Message) ([]api.Message, error) {
		if req.(*classifyapi.ClassifyTableInfo).TableID == 1 { // someone replaced pop 1 meanwhile
			return []api.Message{&classifyapi.ClassifyTableInfoReply{TableID: 1, MatchNVectors: 1, NextTableIndex: ifsanitize.NoIndex, Mask: make([]byte, 16)}}, nil
		}
		return info(req)
	})
	rep, err := ifsanitize.EnsureSentinel(context.Background(), f)
	if err == nil || rep.State != ifsanitize.SentinelCreated || !slices.Equal(rep.Left, []uint32{1}) || len(deletes(f)) != 0 || !m.Tables[1] {
		t.Fatalf("foreign pop: %s err %v deletes %v", rep, err, deletes(f))
	}

	g := fake.New().Fail("classify_table_ids", api.VPPApiError(-1))
	if rep, err := ifsanitize.EnsureSentinel(context.Background(), g); err == nil || rep.State != ifsanitize.SentinelFailed {
		t.Fatalf("ids failure: %s %v", rep, err)
	}
	h, _ := sentinelVPP(t)
	h.Fail("classify_add_del_table", api.VPPApiError(-1))
	if rep, err := ifsanitize.EnsureSentinel(context.Background(), h); err == nil || rep.State != ifsanitize.SentinelFailed {
		t.Fatalf("add failure: %s %v", rep, err)
	}
}

// The caller's ctx is cancelled after k pops (the 20 s connect-hook bound, or a shutdown): the run
// fails, but the cleanup runs on its own deadline — nothing is left in VPP and the pool's table count
// is what it was before the run (review R14 #1 / R78 #1).
func TestSentinelCancelledMidRunStillDeletesPops(t *testing.T) {
	f, m := sentinelVPP(t)
	for i := uint32(0); i < 8; i++ {
		m.Tables[i] = true
	}
	for i := uint32(0); i < 8; i++ {
		m.DeleteTable(i) // free list [0..7]: 7 pops first, 0 last
	}
	before := len(m.Tables)
	ctx, cancel := context.WithCancel(context.Background())
	add := m.Handlers()["classify_add_del_table"]
	pops := 0
	f.On("classify_add_del_table", func(req api.Message) ([]api.Message, error) {
		if req.(*classifyapi.ClassifyAddDelTable).IsAdd {
			if pops++; pops == 3 {
				cancel()
			}
		}
		return add(req)
	})
	rep, err := ifsanitize.EnsureSentinel(ctx, f)
	if !errors.Is(err, context.Canceled) || rep.State != ifsanitize.SentinelFailed {
		t.Fatalf("cancelled run: %s err %v", rep, err)
	}
	if len(rep.Left) != 0 || len(rep.Pops) != 3 || !slices.Equal(deletes(f), []uint32{5, 6, 7}) {
		t.Fatalf("cleanup after cancel: %s pops %v deletes %v", rep, rep.Pops, deletes(f))
	}
	if len(m.Tables) != before {
		t.Fatalf("tables %v, want %d as before the run", m.Tables, before)
	}
}

// Table 0 listed but freed before classify_table_info: the run pops it back instead of giving up.
func TestSentinelZeroFreedBetweenReads(t *testing.T) {
	f, m := sentinelVPP(t)
	m.Tables[0] = true
	m.DeleteTable(0)
	f.On("classify_table_ids", func(api.Message) ([]api.Message, error) {
		return []api.Message{&classifyapi.ClassifyTableIdsReply{Count: 1, Ids: []uint32{0}}}, nil
	})
	if rep := ensure(t, f); rep.State != ifsanitize.SentinelCreated || !m.Tables[0] {
		t.Fatalf("freed between reads: %s", rep)
	}
}

// The sanitizer's placeholders and the sentinel have different signatures: neither deletes the other.
func TestSentinelSignature(t *testing.T) {
	s := ifsanitize.SentinelTable()
	info := &classifyapi.ClassifyTableInfoReply{MatchNVectors: s.MatchNVectors, NextTableIndex: s.NextTableIndex, Mask: s.Mask}
	if !ifsanitize.IsSentinel(info) {
		t.Fatal("the sentinel's own create is not recognised")
	}
	info.Mask = []byte("vrx-td3-v19-hold")
	if ifsanitize.IsSentinel(info) {
		t.Fatal("a sanitizer placeholder is recognised as the sentinel")
	}
}
