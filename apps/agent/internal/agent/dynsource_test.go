package agent

// TD-8b (TD-8 verify V1, V4; TD-9 review L7): per-key quarantine of dynamic objects, fair blame of a
// failed verification, and the sync deadline. Probes G and H are the verify's.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// skippedKey returns the SKIPPED result of key k in resp, nil when there is none.
func skippedKey(resp *vrxv1.ApplyResponse, k string) *vrxv1.ObjectResult {
	for _, r := range resp.GetResults() {
		if r.GetKey() == k && r.GetCode() == vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_SKIPPED {
			return r
		}
	}
	return nil
}

// Probe G (V1): after a VPP restart, one dynamic object VPP rejects no longer holds back the rest of
// its source — neither in the resync nor in the source's own sync.
func TestDynamicSourceOneRejectedObjectDoesNotHoldBackItsSource(t *testing.T) {
	v := coretest.New()
	s, md, _ := syncedSrcSvc(t, v, "loop701", "loop702")
	v.DeleteInterface("loop701") // VPP restarted: everything is gone ...
	v.DeleteInterface("loop702")
	v.DeleteTable(7001, false)
	v.DeleteTable(7001, true)
	md.drop("loop701")
	md.drop("loop702")
	md.failOn("loop702", errors.New(errLabelInUse)) // ... and VPP now rejects one dynamic object

	resp := s.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{})
	if err != nil || !proto.Equal(got.GetDesiredState(), doc(t, canonicalDoc)) {
		t.Fatalf("the configuration was not rebuilt: %v", err)
	}
	if md.list() != "loop701" {
		t.Errorf("resync: dynamic objects %q, want loop701 (the creatable object restored, loop702 quarantined)", md.list())
	}
	if skippedKey(resp, dynDesc+"/loop702") == nil || !s.source("test-sync").inSync.Load() {
		t.Errorf("resync: loop702 reported %v, source in sync %v (want SKIPPED, in sync)", skippedKey(resp, dynDesc+"/loop702"), s.source("test-sync").inSync.Load())
	}

	// The source's own sync (the retry path): the creatable object comes back, the rejected one stays out.
	md.drop("loop701")
	err = s.sourceSync("test-sync")(context.Background())
	if md.list() != "loop701" || err == nil || !strings.Contains(err.Error(), dynDesc+"/loop702") {
		t.Errorf("sync: %v, dynamic objects %q (want loop701 restored and an error naming loop702)", err, md.list())
	}
}

// Probe H (V1): a valid commit that deletes loop702 — whose dynamic object Desired drops — and adds
// loop703, whose dynamic object VPP rejects, ends APPLIED with both deletions.
func TestDynamicSourceRejectedObjectDoesNotBlockAConfigDelete(t *testing.T) {
	v := coretest.New()
	s, md, src := syncedSrcSvc(t, v, "loop701", "loop702")
	md.failOn("loop703", errors.New(errLabelInUse))
	src.set("loop701", "loop702", "loop703")
	next := withLoop703(t)
	delete(next.Interfaces, "loop702")

	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: next})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	deleted := map[string]bool{}
	for _, r := range resp.GetResults() {
		if r.GetOp() == vrxv1.ApplyOperation_APPLY_OPERATION_DELETE && r.GetCode() == vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_OK {
			deleted[r.GetKey()] = true
		}
	}
	for _, k := range []scheduler.Key{scheduler.Join(dynDesc, "loop702"), scheduler.Join(core.LoopbackName, "loop702")} {
		if !deleted[string(k)] {
			t.Errorf("%s was not deleted: results %v", k, resp.GetResults())
		}
	}
	_, has702 := v.InterfaceByName("loop702")
	_, has703 := v.InterfaceByName("loop703")
	if has702 || !has703 || md.list() != "loop701" || skippedKey(resp, dynDesc+"/loop703") == nil {
		t.Fatalf("after the commit: loop702 %v, loop703 %v, dynamic %q, results %v", has702, has703, md.list(), resp.GetResults())
	}
	if !s.source("test-sync").inSync.Load() || strings.Join(quarantinedKeys(s, "test-sync"), ",") != dynDesc+"/loop703" {
		t.Fatalf("in sync %v, quarantined %v (want in sync, loop703 quarantined)", s.source("test-sync").inSync.Load(), quarantinedKeys(s, "test-sync"))
	}
}

// V1 at the plan stage: a dynamic key whose dependency is gone is quarantined (left out), not its
// source; DryRun warns the same way.
func TestDynamicSourceMissingDependencyQuarantinesTheKey(t *testing.T) {
	v := coretest.New()
	s, md, src := syncedSrcSvc(t, v, "loop701")
	orphan := scheduler.Join(dynDesc, "loop709") // its loopback is in no document
	src.mu.Lock()
	src.extra = []scheduler.KV{{Key: orphan, Value: wrapperspb.String("loop709")}}
	src.mu.Unlock()

	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: withLoop703(t)})
	if err != nil || !rep.GetOk() {
		t.Fatalf("dry run: %v %v", err, rep)
	}
	var rules []string
	for _, is := range rep.GetErrors() {
		rules = append(rules, is.GetRule())
	}
	if !strings.Contains(strings.Join(rules, " "), "agent.dynamic-object-quarantined") || strings.Contains(strings.Join(rules, " "), "agent.dynamic-source-skipped") {
		t.Errorf("dry run issues %v (want the key quarantined, the source not skipped)", rules)
	}
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: withLoop703(t)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if skippedKey(resp, string(orphan)) == nil || !s.source("test-sync").inSync.Load() || md.list() != "loop701" {
		t.Fatalf("apply: orphan %v, in sync %v, dynamic %q", skippedKey(resp, string(orphan)), s.source("test-sync").inSync.Load(), md.list())
	}
}

// V1: a Delete VPP refuses keeps the object (desired as it was) instead of failing every later
// transaction; the key retry deletes it once VPP lets go. The documented exception: a config change
// that deletes what that object depends on fails meanwhile.
func TestDynamicSourceRefusedDeleteKeepsTheObject(t *testing.T) {
	v := coretest.New()
	s, md, src := syncedSrcSvc(t, v, "loop701", "loop702")
	md.failDeleteOn("loop702", errors.New("VPP: label busy"))
	src.set("loop701") // the source stops producing loop702
	sync := s.sourceSync("test-sync")
	if err := sync(context.Background()); !isQuarantinedErr(err) || md.list() != "loop701,loop702" || !s.source("test-sync").inSync.Load() {
		t.Fatalf("sync with a refused delete: %v, dynamic %q, in sync %v", err, md.list(), s.source("test-sync").inSync.Load())
	}
	// A config commit keeps the held object as it is: no operation on it at all.
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: withLoop703(t)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	for _, r := range resp.GetResults() {
		if strings.HasPrefix(r.GetKey(), dynDesc+"/") {
			t.Fatalf("the commit touched a dynamic object: %v", r)
		}
	}
	// The exception: deleting loop702 while VPP refuses to delete its dynamic object fails.
	next := withLoop703(t)
	delete(next.Interfaces, "loop702")
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "t3", DesiredState: next})
	if _, ok := v.InterfaceByName("loop702"); resp.GetStatus() == vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || !ok || md.list() != "loop701,loop702" {
		t.Fatalf("deleting loop702 under a live dynamic object: %s, loop702 %v, dynamic %q", resp.GetStatus(), ok, md.list())
	}
	if err := sync(context.Background()); !isQuarantinedErr(err) { // rejoins (the failed commit left the source out)
		t.Fatalf("sync after the failed commit: %v", err)
	}
	md.failDeleteOn("loop702", nil)
	retryQuarantinedNow(t, s, "test-sync")
	if md.list() != "loop701" || len(quarantinedKeys(s, "test-sync")) != 0 {
		t.Fatalf("key retry once VPP lets go: dynamic %q, quarantined %v", md.list(), quarantinedKeys(s, "test-sync"))
	}
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t4", DesiredState: next}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
}

// V1: each quarantined key backs off on its own (retryMin doubling to retryMax), and a sync retries it
// at once when the source's value for it changed.
func TestDynamicSourceQuarantineBackoffAndRelease(t *testing.T) {
	v := coretest.New()
	s, md, src := syncedSrcSvc(t, v, "loop701")
	s.retryMin, s.retryMax = time.Hour, 4*time.Hour // no timer fires during the test
	md.failOn("loop703", errors.New(errLabelInUse))
	src.set("loop701", "loop703")
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: withLoop703(t)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	k := dynDesc + "/loop703"
	for i, want := range []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 4 * time.Hour} {
		if i > 0 {
			retryQuarantinedNow(t, s, "test-sync") // VPP still rejects it
		}
		if got := quarantineDelay(s, "test-sync", k); got != want || md.list() != "loop701" || !s.source("test-sync").inSync.Load() {
			t.Fatalf("retry %d: backoff %v (want %v), dynamic %q, in sync %v", i, got, want, md.list(), s.source("test-sync").inSync.Load())
		}
	}
	src.set("loop701") // the source no longer wants loop703: its next sync releases the key at once
	if err := s.sourceSync("test-sync")(context.Background()); err != nil || len(quarantinedKeys(s, "test-sync")) != 0 {
		t.Fatalf("sync after the source changed: %v, quarantined %v", err, quarantinedKeys(s, "test-sync"))
	}
}

// V1: quarantine is bounded (maxKeyReruns per transaction). Past it the commit falls back to the
// configuration alone and keeps the keys found so far; the source's rejoin sync then finds the next.
func TestDynamicSourceQuarantineIsBounded(t *testing.T) {
	v := coretest.New()
	s, md, src := syncedSrcSvc(t, v, "loop701")
	next := doc(t, sampleDoc)
	names := []string{"loop701"}
	for _, n := range []string{"loop703", "loop704", "loop705", "loop706"} {
		next.Interfaces[n] = &vrxv1.Interface{}
		md.failOn(n, errors.New(errLabelInUse))
		names = append(names, n)
	}
	src.set(names...)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: next})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, ok := v.InterfaceByName("loop706"); !ok || s.source("test-sync").inSync.Load() || len(quarantinedKeys(s, "test-sync")) != maxKeyReruns {
		t.Fatalf("commit: loop706 %v, in sync %v, quarantined %v (want the config applied, the source left out, %d keys quarantined)", ok, s.source("test-sync").inSync.Load(), quarantinedKeys(s, "test-sync"), maxKeyReruns)
	}
	if err := s.sourceSync("test-sync")(context.Background()); !isQuarantinedErr(err) || len(quarantinedKeys(s, "test-sync")) != 4 || md.list() != "loop701" || !s.source("test-sync").inSync.Load() {
		t.Fatalf("rejoin sync: %v, quarantined %v, dynamic %q", err, quarantinedKeys(s, "test-sync"), md.list())
	}
}

// V4: a failed verification blames a dynamic source only when every key it names is that source's.
func TestDynamicSourceVerifyFailureBlamesTheSourceOnlyForItsOwnKeys(t *testing.T) {
	ds := &dynSource{DynamicSource: subsystems.DynamicSource{Name: "test-sync", Descriptors: []string{dynDesc}}, own: map[string]bool{dynDesc: true}}
	mg := &srcMerge{owner: map[string]*dynSource{dynDesc: ds}, merged: []*dynSource{ds}}
	dyn, cfg := scheduler.Join(dynDesc, "loop703"), scheduler.Join(core.LoopbackName, "loop703")
	plan := &scheduler.TxnPlan{Ops: []scheduler.PlannedOp{{Key: cfg, Op: scheduler.OpCreate}, {Key: dyn, Op: scheduler.OpCreate}}}
	verify := func(problems ...string) *scheduler.TxnResult {
		return &scheduler.TxnResult{Outcome: scheduler.OutcomeRolledBack, Plan: plan,
			Err: fmt.Errorf("verify: actual state differs from desired: %s", strings.Join(problems, "; "))}
	}
	for _, tc := range []struct {
		name   string
		res    *scheduler.TxnResult
		blamed bool
	}{
		{"a config key and a dynamic key differ", verify(string(cfg)+" differs: vrx.v1.Interface fields [enabled] (values redacted)", string(dyn)+" missing"), false},
		{"only a config key differs", verify(string(cfg) + " missing"), false},
		{"only the source's key differs", verify(string(dyn) + " missing"), true},
		{"the source's descriptor cannot be retrieved", &scheduler.TxnResult{Outcome: scheduler.OutcomeRolledBack, Err: errors.New("verify: retrieve " + dynDesc + ": boom")}, true},
	} {
		if _, ok := culprit(tc.res, mg); ok != tc.blamed {
			t.Errorf("%s: source blamed %v, want %v", tc.name, ok, tc.blamed)
		}
	}
}

// TD-9 review L7 (Q7): a dynamic-source sync has a deadline of its own, so a slow sync cannot hold the
// transaction lock indefinitely; it rolls back and the agent retries it.
func TestDynamicSourceSyncHasADeadline(t *testing.T) {
	v := coretest.New()
	s, md, src := syncedSrcSvc(t, v)
	setSyncTimeout(t, 150*time.Millisecond)
	slow := true
	md.mu.Lock()
	md.onCreate = func() { // the first Create stalls (a slow VPP), past the sync's deadline
		if slow {
			slow = false
			time.Sleep(600 * time.Millisecond)
		}
	}
	md.mu.Unlock()
	src.set("loop701", "loop702")
	start := time.Now()
	err := s.sourceSync("test-sync")(context.Background())
	if took := time.Since(start); err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) || md.list() != "" {
		t.Fatalf("sync past its deadline: %v after %v, dynamic %q (want it cut after the stalled operation and rolled back)", err, took, md.list())
	}
	_ = s.lock(context.Background())
	armed := s.source("test-sync").retry != nil
	s.unlock()
	if !armed {
		t.Fatal("no retry armed after the sync's own deadline (only a caller that went away skips the retry)")
	}
	if err := s.sourceSync("test-sync")(context.Background()); err != nil || md.list() != "loop701,loop702" {
		t.Fatalf("next sync: %v, dynamic %q", err, md.list())
	}
}
