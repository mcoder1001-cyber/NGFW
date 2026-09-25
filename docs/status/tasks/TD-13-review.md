# TD-13 review: scheduler tier-3 Validator and VPP→daemon stage (ARCH-02, D-125)

Reviewer run on `task/TD-13 @ 3a83c96b` (4 commits on `main@4f472cc7`), 2026-09-25. Nothing edited or merged. The
scratch copies (TD-13 on its own, and TD-13 on top of TD-9) lived in the reviewer's scratchpad, never in `/root/ngfw`.

## Verdict: **APPROVE WITH CHANGES**

The design is right and small:
- One hook runs at the end of `plan()`. DryRun, the commit engine's tier 3, Apply, resync and revert all share it.
- It adds no outcome, no proto field and nothing to the executor.
- The stage is only a tie-breaker, and the rollback is still the journal reversed.
- The tests prove that a rejection is FAILED with zero fake-VPP writes.
- The Kea recipe fits `task/F-kea-dhcp-relay` exactly.

The changes:
- **Before merge: one change (M1).** The scheduler's secret masking uses a hand list of field names and map keys. It
  hides non-secrets (user object names, BGP communities) and rests on a wrong premise about SNMP.
- **At the second rebase: one condition (M2).** It applies to whichever of TD-9 and TD-13 rebases second: TD-9's drift
  check must not run the validators.
- **Doc and test details: the L items.**

None of these changes the design. The fix round is under an hour, and a **focused verify** of M1, M2 and the L lines is
enough; a full re-review is not needed.

If the manager wants TD-13 on `main` at once, because six branches wait on it: merge as is, and file M1 as a tech-debt
row due before the first adopter whose findings can hold secrets or communities (P11, P12, F-snmp). Kea is only
marginally affected (a DHCP server whose name contains "secret", "psk", …).

## Evidence (reviewer's own runs)

| check | result |
|---|---|
| `go test -race -count=1 ./internal/scheduler/... ./internal/agent/... ./internal/subsystems/...` (`TMPDIR=/tmp/g-td13rv`) | `ok` scheduler 1.5 s, agent 12.6 s, subsystems 6.7 s |
| same with a long `TMPDIR` (scratchpad path) | 7 socket tests fail with `bind: invalid argument`, because the unix-socket path is longer than 108 bytes. This is the environment, not TD-13. The envelope's short `TMPDIR` is required. |
| new tests `-race -count=10` (scheduler) / `-count=5` (agent), load average ≈ 44 | stable, `ok` |
| `go vet`, `gofmt -l` on scheduler and agent | clean |
| `git merge-tree --write-tree main task/TD-13` | clean (tree `506b548a`) |
| `merge-tree task/TD-11c task/TD-13` | clean (tree `dcd4d0f0`) |
| `merge-tree task/TD-9 task/TD-13` | 27 conflicted files, **identical** to the set of `main × task/TD-9`. `scheduler/reconciler.go` auto-merges. The 6 `service.go` conflicts are main×TD-9's (the TD-13 side is empty), and TD-13's `report()` hunk auto-merges. |
| TD-13 on TD-9 (scratch: `git archive task/TD-9 apps/agent`, then `git apply` of TD-13's `apps/agent` diff) | applies with offsets only; the three packages `ok` under `-race` (scheduler 1.7 s, agent 21.5 s, subsystems 1.1 s) |

Mutation spot-check (each in a fresh scratch copy; the named test catches it):
```
M3  value not cloned (runValidator passes value)      => validator_test.go:345: the daemon received listen="evil" …
M9  deletes validated too                             => validator_test.go:289: daemon events [validate daemon.fake/dns @3 delete …]
M11 view = actual instead of after                    => validator_test.go:333: … the view shows an object the transaction deletes
M12 no stage tie-break                                => stage_test.go:38 / :56: ops = create k/x,create a/y,create b/z
R1  (mine) report() ignores Issue.Rule                => validator_td13_test.go:128: DryRun: ok=false plan=[] findings: (empty)
R2  (mine) InvalidAt's "/" prefix rule dropped        => SURVIVES (see L4)
```

## Findings

### M1: the redaction hides non-secrets, and its "community" premise is wrong (Q5)

Where:
- `apps/agent/internal/scheduler/validator.go:241`: `secretNameParts`.
- `validator.go:298`: map keys matched by name.
- `docs/agent/scheduler-validators.md:50`, `:114`.
- `TD-13-questions.md:35-40`.

What is in the value (D-040, proto.md §1, the `dataplane.proto` header):
- Schema leaves marked `secret: true` (`writeOnly`; today `passwordHash`) have **no proto field**.
- Every other secret crosses only as a `*_ref` whose value is a D-051 reference. The pattern is `secretRefOf`,
  `packages/schema/src/primitives.ts:262-290`, the same kinds as the scheduler's `secretRefRe`.
- No proto message uses `google.protobuf.Struct`.

What the name list does in the product proto:
- **It catches no secret the D-051 regex misses.** The only proto fields it matches are `secret_ref`,
  `password_ref`, `preshared_key_ref` and `private_key_ref`, and their values are references the regex already
  masks.
- **"community" masks BGP communities.** `RouteMapMatch.community` and `RouteMapSet.community`
  (`dataplane.proto:1066`, `:1086`) are not secrets. P12's `vtysh -C` finding about a malformed community list would
  lose the offending value.
- **The SNMP premise is wrong.** `SnmpService.TrapReceiver.community` is the *name* of a community in `communities`:
  the schema checks it with "community '…' is not defined", `services.ts:639-641`. The plaintext crosses only as
  `Community.secret_ref` (`dataplane.proto:1754`).
- **Map keys are user object names in the product proto** (interfaces, VRFs, DHCP servers, IPsec connections…). An
  object named `psk0`, `secret-lab` or `community-wifi` makes every string under it a "secret".

Probe with `RedactLeaves` on a `DesiredState` in the scratch copy. The object named `psk0` and the route map are both
real documents:
```
in : interface psk0: address 10.0.0.1/24 overlaps (description to branch); route-map rm1 seq 10: set community 65000:70000: malformed
out: interface psk0: address <redacted> overlaps (description <redacted>); route-map rm1 seq 10: set community <redacted>: malformed
```

This is not a leak: it masks too much, never too little. But the finding is the text the operator uses to fix the
configuration.

Nothing enforces "derived from the schema". The schema-derived rule already exists: D-040 plus D-051.

Fix (about 20 lines):
- Drop `secretNameParts` and the map-key rule.
- Mask (a) every string that matches the D-051 reference form anywhere in the value, and (b) the value of every
  string field whose name ends in `_ref`. Rule (b) is the proto side of `secretRefOf`, and it covers a malformed
  reference too.
- Say in the godoc and the doc page:
  - by D-040, no other secret leaf crosses the boundary;
  - the plaintexts a validator resolved are **its** job (`rfkit.Redactor`, `toolMessage`), and the scheduler cannot
    know them.
- Move the structpb fixture of `TestValidatorFindingRedactsSecretLeaves` to `*_ref`-named fields plus references.
- Add a negative case: a BGP community and an object named `psk0` stay readable.

Rating of the current list: **2/5**. It is a hand list, it is not derived from the schema, it adds no protection on
top of the D-051 regex, and it hides useful text.

### M2: with TD-9, the drift check would run the validators and under-count drift (Q2)

`Service.CheckDrift` on `task/TD-9` (`service.go` ≈ `:864-915` there) computes `n := len(plan.Ops) +
len(plan.Issues)`. TD-13 empties `Ops` whenever a validator rejects (`validator.go:147`). So if 10 VPP objects drifted
and one drifted daemon object's checker now fails, the gauge says 1, and the event's "first" keys show the checker
text instead of the drifted keys.

The run is bounded: the Plan runs under `driftPlanTimeout = 60 s` (TD-9), and each call is capped at 30 s. The
validators run only when a daemon object drifted (an Update op). Even so:
- the drift check answers "does the running state match the stored desired state", not "would the daemon accept it";
- it holds the transaction lock while it runs daemon checkers every 5 min for as long as the drift persists, and for
  P11 that means a scratch charon.

Recommendation: **(b), not (a).** The drift Plan skips the validators:
- add `ApplyOptions.SkipValidators` (plan already takes `opts`), used only by `CheckDrift` through a `PlanWith`, or a
  `planSources` variant;
- about 10 lines and one test (a drifted daemon object with a rejecting validator counts as 1 op, not 1 issue).

Whichever of TD-9 and TD-13 rebases second carries it. It is a **merge condition for that rebase**, not for TD-13 on
today's `main` (TD-9 is not merged).

### L1: the stage claim is stronger than the greedy sort delivers

Where: `stage.go:4-5`, `descriptor.go:78-79`, `docs/agent/scheduler-validators.md:75`.

The claim: "Among operations that no dependency orders, every VPP-stage create and update runs before every
daemon-stage one." The tie-break is applied greedily in Kahn's loop (`reconciler.go:581-590`).

Probe:
- `a/v1` (VPP) depends on `k/d1` (daemon).
- `k/c2` (daemon) and `b/z` (VPP) are independent.

Result:
```
create order: create b/z, create k/c2, create k/d1, create a/v1     ← k/c2 (daemon) before a/v1 (VPP), no dependency between them
delete order: delete a/v1, delete k/d1, delete k/c2, delete b/z
rollback run (a/v1 fails): create b/z, create k/c2, create k/d1, delete k/d1, delete k/c2, delete b/z   ← exact reverse ✔
```

- **Correctness is fine:** a dependency always wins, the rollback is the exact reverse, and the stage never reorders
  across the delete→create boundary.
- **Only the wording overclaims.** VPP-on-daemon dependencies are rare.

Fix, either one:
- reword: "the stage is the first tie-breaker of the topological sort; a daemon object that no VPP object waits for
  runs after every VPP object that is ready";
- or use an effective stage = min(stage of the node, stage of every transitive dependent), so `k/d1` counts as VPP
  and `k/c2` goes last (about 15 lines in `topo`, plus this probe as a test).

### L2: the panic value is logged unredacted

`validator.go:178` logs `fmt.Sprint(r)` and the stack. A renderer that panics with a configuration line in its
message (`panic(fmt.Sprintf("bad line %q", l))`) could log a resolved plaintext, against 00-CONTEXT rule 10. Pass
the panic text through the same masking and `boundText` as the finding. `dynsource.go:203` has the same pattern:
put it in tech debt.

### L3: the contract should forbid calling back into the Scheduler

- `Plan` holds `s.mu.RLock`. A validator that calls `Scheduler.Retrieve`/`Plan` there takes a recursive RLock,
  which deadlocks as soon as an Apply is waiting for the write lock.
- Under `Apply`, which holds the write lock, it blocks until the bound: a 30 s stall, then a false finding.

Add one line to the `Validator` godoc, the package doc and the doc page: "never call the Scheduler; use the view".

### L4: the `InvalidAt` "/" prefix rule is untested

`validator.go:197` ignores a pointer that does not start with "/". Mutation R2 (the check removed) survives the whole
suite. Add a case to `TestPlanRunsValidators`: `InvalidAt("services/dhcp", err)` gives the object's own pointer.

### L5: the D-entry and TD-13.md cite "D-136", which does not exist

`TD-13.md:147` and `:201` cite "D-136". `git grep D-136` finds nothing on `main` or any task branch. The corrected
entry below drops it. The manager records the merge order (F-kea before TD-13) under whatever number it gets.

### L6: Q3's alias suggestion would invert a layer

`validator.go:77-79` suggests the rebase "may reuse" `vpp.DefaultReplyTimeout`. The scheduler package imports no
`ngfw/...` package: `go list` on TD-9's tree shows no ngfw import. Aliasing would make the generic reconciler depend
on the VPP transport, for a bound that is about daemon checkers, not VPP replies. Keep the local constant and change
the comment.

### L7: the FEATURE-TEMPLATE rule misses two things

At `prompts/FEATURE-TEMPLATE.md:25-27`, the rule is clear. It says what to implement, where the recipe is, read only
and bounded, and why. Add half a line for each of:
- "masks every plaintext it resolved (`rfkit.Redactor`)";
- "test: a failing checker leaves the fake VPP untouched, and `Validate` makes exactly one checker call on a staged
  path and nothing else".

That second test is the only practical enforcement of side-effect freedom: Go cannot enforce it, and review plus that
test is the mechanism. Cite the new D number once it exists.

### L8: the D-051 reference regex now has a fourth copy (tech debt)

`validator.go:237` has the same regex as `renderers/frr/secrets.go:48`, `renderers/strongswan/secrets.go:43`
and `renderers/rfkit/secrets.go:45`. Move one exported pattern into a leaf package (the scheduler can't import
rfkit). Put it in tech debt, owner TD-16, which already owns descriptors/kit.

### L9: validators that ignore ctx are abandoned with no trace (tech debt)

`validator.go:167-189`. On timeout, `runValidator` returns, its deferred `cancel()` ends the derived ctx, and a
checker started with `exec.CommandContext`/`renderers.Command{Timeout}` dies. The transaction does **not** continue:
the timeout is a finding, so the Apply is FAILED. A validator that ignores ctx, for example one blocked on a lock,
keeps its goroutine until it returns. Each later Plan can then add another. Suggestions:
- a counter of abandoned validators (a metric through TD-8's hook);
- or a per-descriptor in-flight guard that answers "previous validation still running" at once instead of stacking.

## Semantics checked (no finding)

- **Where it runs.** `plan()` calls `s.validate(ctx, p, after)` last (`reconciler.go:513`). That covers `Plan`
  (DryRun), `ApplyWith` (Apply, resync, revert) and TD-8's `applySources`/`planSources` retry. It runs only when the
  plan has no earlier issue, only for in-scope Create/Update ops, and before `ApplyWith` executes anything.
- **A rejection.** It becomes `Issue{CodeInvalid, Rule "agent.validator", Pointer}`, and `Ops`/`Unchanged` are reset
  as for every plan with issues. `ApplyWith` then answers `OutcomeFailed` with results `INVALID` and no executor.
  `mustNoVPPWrite` allows only `sw_interface_dump`/`control_ping`.
- **FAILED is the right outcome.**
  - proto.md §2 defines `FAILED` as "untouched — validation/planning failed; `validation` explains", which is exactly
    this case. ROLLED_BACK would claim writes that never happened.
  - API: `commit.service.ts:586-596` maps FAILED to 422 `apply-failed`, with `validation.errors` (pointer, message,
    rule) plus the INVALID results. `validation.service.ts:34-38` passes the DryRun rule and pointer through, as tier
    `agent`.
  - UI: `ProblemAlert.tsx:72-77` lists each pointer and message.
  - The duplicate entry (the validation issue plus the INVALID result with the object's pointer) is how every plan
    issue already behaves.
  - `/state/drift` uses only non-ERROR coverage rules (`state.controller.ts` `driftOf`), so it is unaffected.
- **Copies.** `proto.Clone(value)` per call. `Get`/`List` clone, `Meta` is never exposed, `afterView.objs` is never
  written after the plan, and its fields are unexported. A validator therefore cannot change what is applied (M3, M4
  and M4b are caught).
- **Timeout vs cancellation.** An error while the *parent* ctx is live is a finding. The parent ctx ending is a plan
  error: FAILED, `Err = ctx.Err()`, no finding. Right, because a cancelled commit is not the configuration's fault.
- **Architecture.**
  - The agent stays declarative: validators only read, and the contract is written in three places.
  - Nothing touches the Node↔VPP boundary, the schema, the proto or binapi names.
  - The rollback is still the journal reversed.
  - No existing descriptor declares a stage, so the ordering on `main` is unchanged: the whole suite is green.

## Q1–Q6: recommendations

- **Q1: one pass is enough. Accept.** Inside `ApplyWith`, `plan()` and the executor run under the write lock with
  nothing in between. The view is a fixed snapshot and validators are pure. So a second call "before the first
  Create" would see the same input and return the same verdict, at double the cost. The gap the envelope's (b) was
  about is the one between the commit engine's DryRun and the Apply. Apply closes it, because it re-plans and
  re-validates. Environmental changes in the microseconds between them are what Create's own check (defence in depth)
  is for.
- **Q2: take (b), skip the validators in the drift check.** See M2. The call is bounded (60 s Plan, 30 s per call),
  but it changes the gauge's meaning and hides the drifted ops. It is a condition for whichever of the two branches
  rebases second.
- **Q3: keep `DefaultValidateTimeout` local, do not alias it (L6).** At TD-9's rebase, normalising to TD-9's style
  (set in `New`, 0 = no bound, like `RollbackTimeout`) is optional; if done, update `SetValidateTimeout(s, 0)` in
  `TestValidatorIsBounded`.
- **Q4: docs only, no `contract(` commit.**
  - `CONTRACT_PATHS` (`tools/ci.sh:51`) are `packages/schema`, `packages/proto`, `apps/agent/gen` and the api-client's
    generated code. `docs/contracts/proto.md` is not among them.
  - TD-9 set the precedent with `docs(contracts): proto.md §2 …` (`2445fcf5`).
  - `rule` is a free string, no API or UI code lists rule ids, and the id is additive.
  - Recommendation: TD-13 adds the proposed §3 line itself in its fix round. It is the natural owner, and TD-9 edits
    §2, not §3.
  - After the D-112 squash the single subject stays `feat(agent): …`, since no contract path is touched.
- **Q5: rewrite the masking as M1 says.** Mask D-051 references anywhere, plus `*_ref`-named string fields. Drop the
  field-name and map-key list and the SNMP claim. Keep the 2 KiB cap. Secrets the validator resolved stay the
  validator's job.
- **Q6: acceptable; keep Create's check.** The count is higher than the question says: through the commit engine, a
  changed daemon object is checked up to **three** times per commit (commit-engine DryRun, Apply's plan, Create). An
  Apply whose dynamic source is to blame runs again, which adds more.
  - Kea: about 100 ms each.
  - P11's scratch charon is the costly one. Its own review should measure it and may drop the Create check (the undo
    and the recreate cascade only ever re-create values that ran before).
  - Write the ×3 into the doc's Limits.

## Adoption recipe

- **Kea.** It matches `task/F-kea-dhcp-relay`:
  - the descriptor value is `*vrxv1.DesiredState`;
  - `RenderFamily(in, family)` exists;
  - `Renderer.Validate` stages into a private dir (`renderers.Stage`, closed by `defer`) and runs `kea-dhcp<N> -t`
    through `renderers.Command{Timeout}` with ctx, under `ip netns exec` when a netns is set;
  - the checker opens no sockets, and Kea tolerates an interface created later (the descriptor comment), so checking
    before the VPP ops cannot falsely reject.
- **The unbound, nftables and snmpd lines** name what exists on their branches: the `prepare` hook, the store and
  `d.mu`, `r.red`.
- **P11 and P12** must show at their review that the checker is concurrency-safe. `Plan` runs under RLock, so two
  DryRuns can validate at once: a fixed scratch-charon socket path would collide.

## Corrected D-entry (for the manager to number)

| 2026-09-25 | D-1xx | Scheduler tier-3 Validator and VPP→daemon stage (TD-13, audit ARCH-02, D-125).
(1) Optional `scheduler.Validator`: `Validate(ctx, key Key, value proto.Message, view ReadOnlyView) error`. It is
called for every Create/Update of the descriptor's objects at the end of planning. That covers Plan (the DryRun RPC,
so the commit engine's tier 3) and every Apply, resync and confirm revert, always after planning and before the first
operation. It runs **once per plan**, with no second pass before the first Create: the plan, its view and the
validators' inputs are fixed under the transaction lock. Deletes, unchanged objects, recreate-cascade dependents and
rollback undos are not validated. A rejection is a plan issue: DryRun `ok=false` and no plan; Apply **FAILED**
(proto.md §2 "untouched") with nothing written and no rollback.
(2) Contract:
- read only: a private temp dir only; no daemon write, reload or restart; no VPP call; no ownership claim; no call
  back into the Scheduler;
- bounded by ctx: per call `Scheduler.ValidateTimeout`, default `DefaultValidateTimeout` = 30 s, a scheduler-local
  constant; a call past its bound is abandoned, its ctx cancelled, and it is a finding;
- panics recovered as findings;
- safe for concurrent use;
- `InvalidAt(pointer, err)` names the document leaf.
(3) Secrets: a validator masks every plaintext it resolved (`rfkit.Redactor`, `toolMessage`). The scheduler also
masks every D-051 reference and every `*_ref` value in the object, and caps the text at 2 KiB. By D-040 no other
secret leaf crosses the boundary.
(4) Finding = `ValidationIssue{rule: "agent.validator", pointer: InvalidAt's or the object's, message: "<key>:
validator: …"}`, in the DryRun report and in `ApplyResponse.validation` of a FAILED Apply (results: the key,
`INVALID`). No proto change (`rule` is a free string). proto.md §3 lists the id, a `docs(contracts)` line.
(5) Optional `scheduler.Stager`: `StageVPP` (the default) or `StageDaemon`. It is the first tie-breaker of the
topological sort, before registration order and key:
- among ready operations, VPP creates/updates run before daemon ones, and deletes run in reverse (daemon first);
- a dependency always wins;
- the rollback stays the exact reverse of the journal.
(6) The periodic drift check (TD-9) plans **without** validators: it counts drift, it does not re-validate.
(7) Adoption = two methods on a daemon's singleton descriptor (D-109 d): `Stage()` returns `StageDaemon`, and
`Validate` = render plus the renderer's staged checker. Create keeps its own check (defence in depth; up to 3 checker
runs per commit). FEATURE-TEMPLATE carries the rule. F-kea-dhcp-relay is the first adopter, done by whichever of the
two branches rebases second. P11, P12, F-unbound-chrony-syslog, F-snmp and F-host-acl-nftables adopt at their own
rebase |
(a) validate inside Create (status quo) (b) a separate service.go renderer stage (c) an optional descriptor Validator
run by the scheduler's plan; stage: (i) registration order (ii) a hard phase split (iii) a stage tie-breaker; drift:
(x) validators in the drift Plan (y) skipped |
(a) writes VPP before the daemon refuses, and DryRun runs no checker; (b) duplicates the scheduler's ordering and
rollback (D-109 d rejected it); (c) one hook for DryRun and Apply, nothing to change for VPP descriptors; (ii) breaks
real VPP-after-daemon dependencies, (iii) keeps them; (x) would hide drifted ops behind one issue and run checkers
under the txn lock every 5 min |
low (additive; no descriptor on main implements either interface yet) |
scheduler, agent DryRun/Apply, TD-9 drift check, P11, P12, F-kea-dhcp-relay, F-unbound-chrony-syslog, F-snmp,
F-host-acl-nftables, FEATURE-TEMPLATE |

## Fix-round checklist (focused verify)

1. M1: the masking rewrite, the test fixture, a negative test (BGP community and an object named `psk0` stay
   readable), and the doc lines `scheduler-validators.md:50`, `:114`.
2. L1: reword the three claims, or add the effective-stage tie-break plus the probe as a test.
3. L2: mask and bound the panic text. L3: the one-line contract. L4: the pointer-prefix test case. L6: the comment.
   L7: the half-lines in the template.
4. Q4: the proto.md §3 line (`docs(contracts)`); Q6: the ×3 note in the Limits.
5. Drop "D-136" from `TD-13.md` (L5).
6. At the second rebase (TD-9 × TD-13): M2 plus its test.

Tech debt, not this round:
- L8: one exported D-051 reference pattern (TD-16).
- L9: an abandoned-validator counter or in-flight guard.
- `dynsource.go:203`: the panic text (L2's twin).
