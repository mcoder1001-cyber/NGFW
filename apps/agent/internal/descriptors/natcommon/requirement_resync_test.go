package natcommon_test

import (
	"context"
	"testing"

	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// A requirement rechecks a shared singleton; it never resets the dataplane.
// Cascading a Create through its existing pool would destroy native sessions.
func TestNonOwnerRequirementResyncPreservesLivePoolSessions(t *testing.T) {
	ctx := context.Background()
	state := natcommon.GlobalState[gspec]{Value: gspec{V: 1}, Present: true, Observable: true}
	reads, writes, deletes := 0, 0, 0
	global := natcommon.Global(natcommon.BuildConfig(nil), natcommon.GlobalOps[gspec]{
		Name: "test.enable", ID: "global",
		Read:  func(context.Context) (natcommon.GlobalState[gspec], error) { reads++; return state, nil },
		Set:   func(context.Context, gspec) error { writes++; return nil },
		Reset: func(context.Context, gspec) error { writes++; return nil },
	})
	exists := false
	sessions := map[int]struct{}{}
	pool := natcommon.New(natcommon.Ops[gspec]{
		Name: "test.pool", ID: func(gspec) string { return "wan" },
		Deps:   func(gspec) []scheduler.Dependency { return []scheduler.Dependency{{Key: "test.enable/global"}} },
		Create: func(context.Context, gspec) (any, error) { exists = true; return nil, nil },
		Delete: func(context.Context, gspec, any) error { exists = false; deletes++; clear(sessions); return nil },
		Retrieve: func(context.Context) ([]natcommon.Item[gspec], error) {
			if !exists {
				return nil, nil
			}
			return []natcommon.Item[gspec]{{Spec: gspec{V: 1}}}, nil
		},
	})
	reg := scheduler.NewRegistry()
	reg.Register(global)
	reg.Register(pool)
	engine := scheduler.New(reg, nil)
	desired := []scheduler.KV{{Key: global.Key(gspec{V: 1}), Value: natcommon.MustEncode(&gspec{V: 1})}, {Key: pool.Key(gspec{V: 1}), Value: natcommon.MustEncode(&gspec{V: 1})}}
	if result := engine.ApplyWith(ctx, desired, nil, scheduler.ApplyOptions{Resync: true}); result.Outcome != scheduler.OutcomeApplied {
		t.Fatal(result.Err)
	}
	for i := range 1000 {
		sessions[i] = struct{}{}
	}
	initialReads := reads
	for range 3 {
		if result := engine.ApplyWith(ctx, desired, nil, scheduler.ApplyOptions{Resync: true}); result.Outcome != scheduler.OutcomeApplied {
			t.Fatal(result.Err)
		}
	}
	if reads <= initialReads || writes != 0 || deletes != 0 || len(sessions) != 1000 {
		t.Fatalf("requirement replay reads=%d writes=%d pool deletes=%d surviving sessions=%d", reads, writes, deletes, len(sessions))
	}
	state.Value.V = 2
	if result := engine.ApplyWith(ctx, desired, nil, scheduler.ApplyOptions{Resync: true}); result.Outcome == scheduler.OutcomeApplied {
		t.Fatal("changed shared configuration accepted")
	}
	if writes != 0 || deletes != 0 || len(sessions) != 1000 {
		t.Fatalf("failed requirement destroyed live sessions: writes=%d deletes=%d sessions=%d", writes, deletes, len(sessions))
	}
	owner := natcommon.Global(natcommon.BuildConfig([]natcommon.Option{natcommon.WithGlobalsOwner(true)}), natcommon.GlobalOps[gspec]{Name: "test.owner", ID: "global", Read: func(context.Context) (natcommon.GlobalState[gspec], error) { return state, nil }, Set: func(context.Context, gspec) error { return nil }, Reset: func(context.Context, gspec) error { return nil }})
	if owner.CreateIsReadOnly() || pool.CreateIsReadOnly() {
		t.Fatal("mutable owner/object must retain normal dependency cascade")
	}
}
