# F-vlan-qinq — verify fix round 1

Verifier agent, 2026-09-24. Branch `task/F-vlan-qinq`@d95f148, read-only checks against
`F-vlan-qinq.fix1.md` scope and review 674b03e (H1, M1, M2, L1, L2). No host/VPP run, no `show trace`/`trace add` issued.

## Verdict: APPROVE

## Findings

1. **H1 (no trace in this branch's tests):** `git grep` for `show trace`/`trace add`/`clear trace` across `*.go` finds
   zero hits in `test/topology/vlan-qinq/*.go`; only comments citing the ban remain (`qinq_test.go:13`, `vpp_test.go:5`).
   The only repo-wide hits are `test/topology/interfaces/interfaces_test.go:291/297/302`. Confirmed this file is not
   this branch's: `git diff task/W-seed HEAD -- test/topology/interfaces/interfaces_test.go` is empty, and
   `git log --follow` on it shows only P08 commits (76850ac, 16356d9, 3a02345, 82d699d, eacba65, 1ef9d02, 3cddf40).
   It is TD-20's/P08's file, correctly out of scope here, to be replaced at rebase.

2. **Q0 root cause corrected:** `F-vlan-qinq-questions.md` Q0, `F-vlan-qinq.md`'s Incident section and D-VQ-4 all now
   state the real cause — `show trace` → `format_vlib_trace` calling a NULL `format_buffer` for a stale trace record
   whose tx node was recycled from a deleted interface — with 802.1ad/V19/V24 explicitly ruled out (kept only as "my
   first reading was wrong"). Matches review 674b03e H1.

3. **M2 evidence:** the packet-free host run is pasted in `F-vlan-qinq.md` "Fix round 1" at HEAD abe6d50, slot 12,
   `VRX_QINQ_PACKETS` unset (packets phase SKIP), `NRestarts before: 1` / `NRestarts after: 1`, PASS on
   commit/restart-safety/rollback/cleanup-through-api. `git log --stat abe6d50..HEAD` shows only docs-only commits
   (fd604b2, d4801d8, d95f148 — all touch only `F-vlan-qinq*.md` status files) plus 5cd5010 touching
   `apps/web/src/domains/interfaces/subinterfaces/tagStack.ts` (the L2 refactor, not test code). No test file changed
   after abe6d50 (`git diff --stat abe6d50 HEAD` confirms the same 4 files).

4. **CI at fd604b2, only docs after:** `F-vlan-qinq.md` pastes `CI GATE PASSED` for
   `TMPDIR=/tmp/g-w12 tools/ci.sh --base main` (main's copy per D-127, since the branch's own copy still hits the Q6
   SIGPIPE flake) at branch HEAD fd604b2. `git log --stat fd604b2..HEAD` shows exactly one commit after it (d95f148),
   touching only `docs/status/tasks/F-vlan-qinq.md`.

5. **M1 closed by D-128b:** reproduced the guard's own diff+filter
   (`git diff --name-only $(merge-base main HEAD) HEAD -- <CONTRACT_PATHS> | grep -vE '(\.test\.ts|_test\.go)$'`) —
   output matches the pasted CI's "contract files changed in HEAD since main" list exactly (11 pre-existing
   generated/schema files, already covered by P08's/W-seed's contract commits in the ancestry).
   `packages/schema/src/semantic/interfaces-qinq.test.ts` appears in the raw (unfiltered) diff but is dropped by the
   filter; `main:tools/ci.sh:327` confirms D-128b is live (`grep -vE '(\.test\.ts|_test\.go)$'`, comment names this
   exact case). Note: the local `task/W-seed` ref has since moved past main's current tip (shared-worktree drift,
   unrelated to the historical df67a8e base used at merge), so a literal diff against it is stale; diffing against
   `main` (which now contains W-seed/P08/D-128b) is the equivalent, correct check, and it passes.

6. **L1/L2 sane:** `abe6d50` replaces every `trace add`/`show trace` call in the packets sub-test with `ifCounters()`
   before/after per-sub-interface rx/tx deltas (asserts the pinged stack's own counters advance by ≥ echo count) and
   removes the dead `proto = "802.1ad"` branch + its stale comment from `vlanDevices`. `5cd5010` defines
   `TagStackFields`/`TagStackRow` via `Pick<>` over `SubinterfaceConfig`/`LiveState`/`InterfaceItem` (wrapped in a
   `Loose<>` helper), exactly as the review asked. Both diffs read as minimal and correctly scoped.

## Unit tests run (read-only, no host/VPP)
```
$ cd apps/agent && go test ./internal/descriptors/interface/...
ok  	ngfw/agent/internal/descriptors/interface	0.055s

$ cd test/topology/vlan-qinq && gofmt -l . && go vet ./... && go test -count=1 ./...
ok  	ngfw/test/topology/vlan-qinq	0.022s
```
`apps/web && npx vitest run src/domains/interfaces src/locales` timed out at 90s in this environment (needs a turbo
workspace build first, per the review's own note) and was not retried to stay in the time box; the pasted CI run at
fd604b2 (turbo 30/30 tasks including web lint/typecheck/test/build) post-dates 5cd5010 and already covers it.

## Not blocking
FR1-a (the `format_vlib_trace` V-item, owned by TD-20) and L3 (fake-agent `innerVlanId`, owned by the P5 file owner)
are correctly left undone here and documented as deferred — out of this branch's fix-round-1 scope.
