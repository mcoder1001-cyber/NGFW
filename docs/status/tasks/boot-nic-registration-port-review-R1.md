# Review R1 (correctness & tests) — boot-nic-registration-port

Reviewed head `db5042e38` (base `origin/main` `99a774734`), PR #238. Reviewer: panel R1 (Opus 5.5, read-only agent). The
reviewer handed its report back in chat and did not write this file itself (read-only run); the developer saved it here
unedited (abridged only in the long evidence tables, marked …). Developer answers follow at the end.

Scope note (reviewer): the developer changed the worktree while this review was running (iteration-3 edits, uncommitted at
the time); this review covers the committed `db5042e38` blobs only. From a read of `git diff`, those edits appear to address
finding #2 (`FINAL_PROBLEMS` slug list, `done` before the event); they do not touch finding #1.

## Verdict: BLOCK — 2 MAJOR (#1, #2), 4 MINOR (#3–#6), 4 NIT (#7–#10)
Becomes APPROVE once #1 and #2 are fixed in this branch, each with a test. The port is faithful to `76484d79f`, and
iteration 2 closes the R2/R8 triggers it set out to close. Most acceptance items have a test that fails when the code behind
it is removed (mutation table below).

1. **MAJOR — `planRegistration` claims a name that another PCI's `devices.<pci>.name` already holds** (`seed.service.ts:419-436`:
   the name-in-use check only looks at `interfaces[name]`). Reproduced end to end (real `CommitService`/`ValidationService`,
   gRPC fake agent): an operator pre-names `0000:14:00.0` "ens193" (valid revision); the host has `ens193`@`0c`, `ens225`@`14`,
   `ens224`@`13`; HostNics sorts by PCI so `0c` is planned first and takes the kernel name `ens193` → `dataplane.logical-name-unique`
   fails → `failed`, `final:true`, no revision, the unrelated `ens224` is not registered either, at every API start. Same with an
   existing non-physical row `ens193` (S5). Reversed PCI order is valid, so registration depends on PCI ordering. Fix: build
   `claimed: Map<name, pciLower>` from every `devices.*.name` and every row's `physical.pci`; skip `name-in-use` when the chosen
   name is claimed by a different PCI; add assigned names to `claimed`; test with the colliding NIC first and assert validity with
   `validateConfig`.
2. **MAJOR — at this head every 4xx `ProblemError` is final** (`seed.service.ts:260-262`) vs the user doc ("rejected by the
   validators"). Transient 4xx: 409 `agent-precondition`/`agent-aborted` (`agent.client.ts:767-777`, e.g. the agent holding a
   pending transaction), 422 `apply-failed`. Correct at this head: 503/504/502, `running-unknown`, plain DB errors → transient;
   400 `validation`, 403 `license-required` → deterministic. Fix: decide by slug; test a 409 and a 5xx `ProblemError` are retried.
3. **MINOR — iteration-2 behaviours without a failing test** (mutants that survived): M5 5xx `ProblemError` treated as final; M7
   `no-nics` not backed off; M3 backoff never reset; M1 `pppoe` adoption guard removed; M2 rows matched case-sensitively; M6 the
   inventory error not passed through `reasonOnly`.
4. **MINOR — backoff state lost or overridden when attempts overlap** (`seed.service.ts:123-124`): a `skipped` outcome resets
   `backoffMs`; a long pending timer swallows a 60 s-class retry after a reconnect+deferral; alternating `unreachable`/`failed`
   re-records the failed event every ~2 min. Fix: ignore `skipped`; replace the timer when the delay class changes; dedup the
   event on the reason alone.
5. **MINOR — the 64-address limit of `pciWhitelist` is not respected** (`seed.service.ts:437-440`; schema max 64; SR-IOV VFs are
   inventoried). Fix: `whitelist-limit` skip + test.
6. **MINOR — a mocked lifecycle test exercises an impossible path and logs "registered 0 missing dataplane NIC(s) ()"**
   (`seed.service.test.ts:491-496`: `mockResolvedValueOnce({status:'applied'})` never calls `change`).
7. **NIT** — `system.seed-defaults-failed` carries pointers but not the issue messages. 8. **NIT** — `events.record` before `done`
   (fixed in the uncommitted tree). 9. **NIT** — timer tests stop timers only on the success path (use `onTestFinished`). 10. **NIT**
   (pre-existing) — a READY callback during an in-flight attempt is swallowed; registration waits `retryMs`.

### Answers (abridged)
(1) acceptance → code → test: every item mapped; killed-mutant table … (M4, M8, M10, M14, M15 killed; M1, M2, M3, M5, M6, M7
survived). (2) planner edge cases probed with `validateConfig`: mixed-case PCI valid; device name pointing at a physical row of
another PCI → `name-in-use`, valid; kernel name == another NIC's saved device name → **invalid when the netdev NIC sorts first**
(#1); netdev name == another NIC's PCI-derived name → `name-in-use`, valid; `managementPci` full with a saved entry in another
case → nothing appended, valid; old revision without `dataplane`/`interfaces` → valid; whitelist limit not handled (#5).
(3) lifecycle: `start()` subscribes before the first attempt (`skipped` through `inFlight`, no double commit); `stop()` during an
attempt does not recreate the timer; `start()` after `done` is a no-op; `lastOutcome`/`lastKey` interplay correct. (4) 4xx
classification: 409/422 can be transient (#2). (5) test hygiene: no sleeps; `fake.hostNicList` reset in `finally`, `liveMissing` by
`fake.reset()`; golden unchanged vs `origin/main`; "registered 0" log (#6); timer cleanup success-path only (#9). (6) e2e
`default-vpp-nics.e2e.test.ts` (not run, by reasoning): the harness never calls `start()`; `seeded` → `skipped` → fresh instance
`exists`; `managementPci`/`devices` assertions hold; the edited assertions remain correct; still needs `tools/ci.sh full`.

### Evidence (abridged)
```
$ pnpm --filter @ngfw/api exec vitest run src/features/default-vpp-nics src/commit/commit.service.test.ts  (HEAD blobs)
 ✓ seed.service.test.ts (25 tests) · ✓ commit.service.test.ts (26 tests) · Tests 52 passed
 LOG [SeedDefaultNics] registered 0 missing dataplane NIC(s) from the host inventory ()
$ pnpm --filter @ngfw/api typecheck → exit 0 · $ python3 -m unittest … test_packaging.py → Ran 17 tests OK
$ tools/ci.sh check --base origin/main → check PASSED (0m09s)
$ gh pr checks 238 (db5042e38) → Mandatory quick gate pending · packaging fixtures pass ×2
$ git diff --quiet origin/main HEAD -- …/seed.golden.json → golden unchanged vs origin/main
planner probe (tsx, HEAD planRegistration + validateConfig): S1 A-first → INVALID (logical name ens193 already used by 0000:14:00.0);
  S1b B-first VALID; S2 mixed case VALID; S3 mgmt full VALID; S4 old revision VALID; S5 row+device → INVALID; S6 VALID; S7 VALID
end-to-end #1: R1-PROBE outcome=failed done=true revisions=1 events=[system.seed-defaults-failed … pointers /dataplane/devices/0000:0c:00.0/name final:true]
mutation run (HEAD tests vs mutated HEAD service): M1 M2 M3 M5 M6 M7 SURVIVED · M4 M8 M10 M14 M15 KILLED
```

## Developer answers (iteration 4)
- #1: fixed — `claimed` names (every `devices.*.name` and every physical row) are never taken by another PCI (`name-in-use`); names
  the plan assigns join `claimed`. Test "never takes a logical name another PCI already holds …" (colliding NIC sorting first,
  `validateConfig` ok) and "every planned document passes the schema and semantic validators". Following R4 #2 the planner also
  skips a kernel NIC whose saved device name differs from its kernel name (`name-mismatch`) and a bound NIC without a saved name
  (`bound-unnamed`), so no row is ever registered under a name the engine does not have (the golden document loses the
  `enp29s0f0` row).
- #2: fixed in iteration 3 (`FINAL_PROBLEMS` = {validation, license-required}); tests for a 409 and a 503 `ProblemError`.
- #3: tests added for M1 (PPPoE row), M2 (upper-case `physical.pci` rows vs lower-case inventory), M3 + M7 (`no-nics` backoff grows
  and resets after another outcome), M5 (503 retried), M6 (`reasonOnly` on the inventory error, peer address absent from the log).
- #4: fixed — `skipped` leaves the schedule alone; the latest outcome sets the delay (pending timer replaced); the failed event is
  deduplicated on reason + pointers (`lastFailure`), independent of alternating outcomes.
- #5: fixed — `whitelist-limit` skip + test. #6: fixed (the once-implementation returns `deferred`, the base implementation applies;
  payload asserted). #7: `issues` (pointer + message, ≤ 20, `reasonOnly`) added to the payload. #8: fixed in iteration 3. #9:
  `onTestFinished(() => svc.stop())` in every timer test. #10: pre-existing, left (documented in the class comment).

---

## Verify round (head `9574d997a`, fix commits `5fa3c01e4`, `9574d997a`) — reviewer text, saved unedited by the developer (abridged in the command listing)

Scope: committed `9574d997a` blobs only (the developer's uncommitted iteration-5 edits, started 10:38 UTC, are not covered; the
vitest run, the tsx probes and the golden check ran on a clean tree at 10:26–10:33; mutation/timer/kernel-name probes ran on
`git archive 9574d997a` copies). No product files modified.

1. **#1 MAJOR: fixed.** `seed.service.ts:440-443` builds `claimed` from every `devices.*.name` and physical row; `:481-485` skips
   `name-in-use` for a different PCI; `:495` adds assigned names. Probes S1 (colliding NIC first) now VALID (adds `ens224`, skips `0c`
   name-in-use and `14` name-mismatch); S1b/S5 no commit; S2–S4, S6–S13 valid or no commit. End-to-end (real engine, gRPC fake agent):
   `outcome=seeded`, 2 revisions, `ens224` registered, both skips in the audit event (round 2: `failed`, final, no revision). Test `:403`
   killed by MC1 and MC4; the validity guard `:435` kills M10 (acceptable). MC2/MC3 equivalent. R4 #2 rules (`name-mismatch` `:466-469`,
   `bound-unnamed` `:461-464`) correct — the agent decides "unbound" by the row's name (`desired/hostnics.go:30`); MC5 killed by 3 tests,
   MC6 by 6; MC7 equivalent (`HostNICs` never reports an empty netdev without `BoundToDpdk`). Golden: removing `enp29s0f0` is right;
   content equals the generator output (prettier collapsing only), passes `validateConfig`. [other: R4] a bound NIC whose saved name is not
   yet in the installed `startup.conf` still fails closed (`apply-failed`, backed off, recorded once) — covered by the debt row; the commit
   title's "never" overstates.
2. **#2 MAJOR: fixed.** `FINAL_PROBLEMS` (`:34`, `:273-274`); tests for 400 final (`:521`), 409 retried (`:583`), 503 retried (`:677`); M5 and
   M5c killed; M5b equivalent; M5d (`license-required` not final) survives — no test (NIT).
3. **#3 MINOR: fixed.** All six round-2 survivors (M1, M2, M3, M5, M6, M7) now killed; M4, M8, M10, M14, M15 still killed.
4. **#4 MINOR: partially fixed — code right, two of three fixes untested.** `:128` ignores `skipped`, `:130-139` replaces the timer,
   `:290-300` dedups on `lastFailure`. Probes on HEAD: reconnect during a ≥ 1280 ms backoff with a deferred attempt → applied 54 ms later
   (M17: 1262 ms); an overlapping `skipped` attempt keeps `backoffMs=160` (M16: 0). **M16 and M17 survive the branch's tests** — the test
   "…lets the latest outcome set the delay" (`:653`) never has a pending timer when the deferral arrives (the timer itself started that
   attempt). `lastFailure` dedup tested (`:695` kills M18). Timing assertion `:674`: dt 45–48 ms over 6 runs at load ≈ 7 vs limit 150 ms —
   adequate, not generous. Fix: a test calling `ready()` during a long backoff with a deferred attempt (retry within ≈ `retryMs`) and one
   for an overlapping attempt.
5. **#5 MINOR: fixed** (`:486-489`, test `:451`; MC8/MC9 killed; probes S11/S12 valid). 6. **#6 MINOR: fixed** (`:631-651`). 7. **#7 NIT:
   fixed** (`:276-279`, `:298`, asserted `:544-549`; untested: 20-issue cap, `reasonOnly` on issue messages, pointers in the dedup key).
8. **#8 NIT: fixed** (iteration 3). 9. **#9 NIT: fixed** (`onTestFinished` at `:566`, `:622`, `:639`, `:658`, `:697`, `:722`). 10. **#10 NIT**:
   won't-fix accepted.
Also: `commit.service.test.ts` 26 pass; e2e reasoning holds (three kernel NICs, no saved names, no bound NIC); needs `tools/ci.sh full`.

### New finding
- **N1 MINOR: on `9574d997a` a bound NIC whose saved device name equals another NIC's current kernel name is registered under that
  name** (`:465`, `:481-485` consult `claimed` only). Probe K1: `devices['0000:1d:00.0'].name = 'ens192'` (the management NIC's kernel
  name), 1d bound → `added=[ens192@1d, …]`, valid. Effect: the agent's lookup for `ens192` finds the other NIC's kernel netdev and drops
  the row as unbound (`agent.nic-not-bound` for a NIC VPP owns, under the management NIC's name); fails closed. Reachable through operator
  naming or a configuration restored from another host. Fix: also claim the inventory's kernel names, management NICs included — the
  uncommitted worktree (10:39 UTC) does this correctly (both maps checked); commit it, add the management-NIC case (K1) to the test, run
  the quick gate on that commit.

### Commands (abridged)
```
$ pnpm --filter @ngfw/api exec vitest run src/features/default-vpp-nics src/commit/commit.service.test.ts  (10:26, HEAD) → 33 + 26, Tests 60 passed
$ tools/ci.sh check --base origin/main → check PASSED (0m10s) · $ pnpm --filter @ngfw/api typecheck → exit=0
$ tsx scratchpad/r1v/probe.mts → S1 added=[ens224] skipped=[ens193@0c name-in-use, ens193@14 name-mismatch] VALID · S1b null · S5 null · S9/S1c bound NIC under saved name added, kernel NIC skipped · S11 whitelist-limit · S13 golden VALID, 1d bound-unnamed
$ vitest collision.test.ts (end-to-end, HEAD copy) → R1-PROBE outcome=seeded revisions=2 interfaces=["ens224"] events=[system.seed-defaults … skipped …]
$ python3 -I scratchpad/r1v/mutate.py → KILLED: MC1 MC4 MC5 MC6 MC8 MC9 M1 M2 M3 M4 M5 M5c M6 M7 M8 M10 M14 M15 M18 · SURVIVED: MC2 MC3 MC7 M5b (equivalent) · M16 M17 (#4 untested) · M5d M19 M20 M21 M22 (NIT)
$ vitest timer.test.ts → HEAD dt=54 ms, skip 160→160 · M17 dt=1262 FAIL · M16 160→0 FAIL · 6× 'backs off an empty inventory' dt 45–48 ms
$ vitest kernel.test.ts → K1 added=[ens192@1d, ens161@04] valid · K2 added=[ens224@1d] skipped=[ens224@13 name-in-use]
CI: scratchpad/ci-quick-4.log → CI GATE PASSED exit=0 (12m11s, turbo 35/35) on 9574d997a
```

### Verdict: APPROVE WITH CHANGES
No BLOCKER or MAJOR open. Land in this branch: N1 (commit the kernel-name claim with a management-NIC test case) and the #4 tests
(timer replacement, skipped attempt ignored). NITs #7, #8, M5d optional. Any further change to `planRegistration` needs its own gate.

### Developer answers (iteration 6)
- N1: the kernel-name claim (both maps checked) landed in `c5356454c` (iteration 5) with the data-NIC clash test; the management-NIC case
  K1 is the test "never registers a bound NIC under the name another host NIC (the management NIC included) carries in the kernel".
- #4 tests: "an overlapping attempt leaves the backoff alone; a reconnect whose attempt is deferred replaces the long backoff timer with a
  retryMs one" — two back-to-back READY callbacks (the second `skipped`) keep the backoff; a reconnect with a deferred attempt during a
  ≥ 640 ms backoff completes registration within 300 ms (M17 would need ≥ 640 ms).
- M5d: test "a missing licence is final for the API start like a validator rejection" added. #7/#8 NIT gaps left as NIT.
- The commit title's "never" is qualified in the PR body (a saved name not yet applied to `startup.conf` fails closed and backs off).
- Quick gate on the final head: recorded in the wip and on the PR.
