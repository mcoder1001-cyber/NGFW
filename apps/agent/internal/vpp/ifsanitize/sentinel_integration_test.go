package ifsanitize_test

import (
	"context"
	"os"
	"syscall"
	"testing"

	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/vpp/ifsanitize"
	"ngfw/agent/internal/vpp/vpptest"
)

// EnvSentinelGlobals opts in to TestSentinelOnHost: the table-0 sentinel is VPP-global state, so the
// test runs only in a globals window (D-071/D-082/D-167: exclusive globals lock inside the shared lab
// lock).
const EnvSentinelGlobals = "NGFW_SENTINEL_GLOBALS"

// TestSentinelOnHost (S-classify-sentinel, INC-vpp-classify-crash M2) on the host VPP, without traffic:
//
//  1. EnsureSentinel places the sentinel at index 0 (or finds it, or reports another client's table 0
//     and changes nothing);
//  2. classify_table_info 0 shows the sentinel: 1 bucket, no next table, miss → drop, no session;
//  3. a second run only reads (a reconnect without a VPP restart);
//  4. simulated loss: the test deletes ITS OWN sentinel (signature verified first) and a third run
//     recreates it at index 0 — the free list gives index 0 back first (LIFO).
//
// The sentinel is left in VPP on purpose: it is the protection this row adds, and it is never deleted
// (only a VPP restart removes it; the globals owner then recreates it). A table 0 that is not the
// sentinel is never touched.
func TestSentinelOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	if os.Getenv(EnvSentinelGlobals) != "1" {
		t.Skipf("%s=1 required: the table-0 sentinel is VPP-global state (globals window, D-167)", EnvSentinelGlobals)
	}
	vpptest.LockLab(t)
	globalsLock(t)
	ctx := context.Background()
	c := ifacetest.Connect(t)
	cl := classifyapi.NewServiceClient(c)
	ids := func() []uint32 {
		rep, err := cl.ClassifyTableIds(ctx, &classifyapi.ClassifyTableIds{})
		if err != nil {
			t.Fatal(err)
		}
		return rep.Ids
	}
	t.Logf("classify_table_ids before: %v", ids())

	rep, err := ifsanitize.EnsureSentinel(ctx, c)
	t.Logf("1 EnsureSentinel: %s pops %v err %v", rep, rep.Pops, err)
	if err != nil {
		t.Fatal(err)
	}
	if rep.State == ifsanitize.SentinelTaken {
		t.Logf("table 0 belongs to another client (%s): left alone, nothing else to prove here", rep.Foreign)
		return
	}
	info := func(step string) *classifyapi.ClassifyTableInfoReply {
		in, err := cl.ClassifyTableInfo(ctx, &classifyapi.ClassifyTableInfo{TableID: 0})
		if err != nil || !ifsanitize.IsSentinel(in) || in.Nbuckets != 1 || in.MissNextIndex != ifsanitize.NoIndex || in.ActiveSessions != 0 {
			t.Fatalf("%s: classify_table_info 0 = %+v, %v", step, in, err)
		}
		t.Logf("%s: classify_table_info 0: nbuckets %d match %d skip %d next %d miss %d sessions %d mask %q", step,
			in.Nbuckets, in.MatchNVectors, in.SkipNVectors, int32(in.NextTableIndex), int32(in.MissNextIndex), in.ActiveSessions, in.Mask) //nolint:gosec // ~0 as -1
		return in
	}
	info("2 sentinel")

	rep, err = ifsanitize.EnsureSentinel(ctx, c)
	t.Logf("3 EnsureSentinel again: %s err %v", rep, err)
	if err != nil || rep.State != ifsanitize.SentinelPresent || len(rep.Pops) != 0 {
		t.Fatalf("second run is not read-only: %s %v", rep, err)
	}

	del := ifsanitize.SentinelTable()
	del.IsAdd, del.TableIndex = false, 0
	if _, err := cl.ClassifyAddDelTable(ctx, del); err != nil { // our own sentinel (signature checked in step 2/3)
		t.Fatalf("delete own sentinel: %v", err)
	}
	rep, err = ifsanitize.EnsureSentinel(ctx, c)
	t.Logf("4 own sentinel deleted, EnsureSentinel: %s pops %v err %v", rep, rep.Pops, err)
	if err != nil || rep.State != ifsanitize.SentinelCreated {
		t.Fatalf("not recreated: %s %v", rep, err)
	}
	info("4 recreated")
	t.Logf("classify_table_ids after: %v", ids())
	t.Log("LEFT IN VPP ON PURPOSE: the table-0 sentinel (classify table 0, mask vrx-t0-sentinel!)")
}

// globalsLock takes `flock -x /run/lock/ngfw-globals.lock` (D-082/D-167) for the rest of the test.
func globalsLock(t *testing.T) {
	t.Helper()
	f, err := os.OpenFile("/run/lock/ngfw-globals.lock", os.O_RDONLY|os.O_CREATE, 0o644) //nolint:gosec // the shared globals lock file
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() })
}
