# TD-9 questions and decisions (for the manager)

Nothing here blocks the branch. Each item says what I did.

## Q1. D-entries to copy into docs/decisions/LOG.md (numbers are the manager's)
The texts are in `TD-9.md` § "D-entry texts": (a) the agent's timeouts and the "outcome unknown" rule, which amends
proto.md §2 item 3 and the Outcomes table (done on this branch); (b) the confirm deadline counts from `applied_at`
(review 1.5b; this matches proto.md §4.1, so the code changed and the contract did not).

## Q2. `internal/agent/metrics.go` changed although it is not in my files list
- The scope asks for "a Plan-only drift gauge" (1.1b) and a panic "metric" (1.1d). Both are agent families, and
  metrics.go is where the agent's families are rendered.
- The hunk is additive: fields `drift` and `panics`, `panicked()` and `setDrift()`, and two families in `writeAgent`
  (`vrx_agent_drift_objects`, `vrx_agent_panics_total{where}`). The names do not contain "collector", so TD-8's
  `TestMetricsCollectors` invariant holds.
- It does not touch TD-8's collector hunks. The trial merge against main has no conflict in metrics.go.

## Q3. The `ApplyResponse.validation` comment in `packages/proto/vrx/v1/dataplane.proto`
- Line 172 says "Validation issues when status is FAILED because of validation (empty otherwise)."
- Since review 1.2, APPLIED and ROLLED_BACK answers carry the projection's warnings (ok = true). The field and its
  wire format are unchanged, and proto.md §2 documents the new behaviour.
- The .proto file is a contract file and I do not own it. I left it alone.
- Proposal: the next `contract(proto)` commit changes the comment to "FAILED: the errors; APPLIED/ROLLED_BACK: the
  projection's warnings (ok = true) when there are any; unset otherwise".

## Q4. TD-10a: filter warnings in the API's 422 error list
- For a ROLLED_BACK answer, `commit.service.ts` puts every `res.validation.errors` entry into the problem's `errors`.
- With 1.2, that list can now hold WARNING issues too, such as `/system agent.unimplemented-domain`.
- Proposal for TD-10a: map only `severity == ERROR` into the 422 list, and pass WARNING issues as `warnings`.
- Nothing breaks without this change; the 422 just lists the warnings as issues as well.

## Q5. A descriptor panic answers DEGRADED, not ROLLED_BACK (decided, logged here)
- As the envelope asks, the scheduler recovers the panic into `ErrDescriptorPanic`, and the normal rollback undoes
  the journal.
- The status is still DEGRADED, because the panicking call may already have changed VPP before it panicked. The
  journal has no record of that change, so "back at the previous state" cannot be promised.
- The agent then owes a resync, which removes any leftover.
- A panic in the plan phase (Retrieve, Normalize, KeyOf, Dependencies) answers FAILED: nothing was touched.

## Q6. Only an Apply that covers every managed domain clears DEGRADED (decided)
- Before TD-9, any APPLIED transaction cleared DEGRADED, including one over a single domain.
- Now DEGRADED means "a resync is owed". A resync, a confirm revert, or an Apply whose domains cover every managed
  domain pays it. A narrower Apply leaves DEGRADED and the owed resync in place.
- The API always sends every domain (`subsystems` = the implemented ones), so its reconcile after a DEGRADED answer
  still clears it. `TestDegradedWhenRollbackFails` and the N2 tests pass unchanged.

## Q7. Dynamic-source syncs (`dynsource.go`, TD-8's file) have no transaction deadline
- Their VPP calls are bounded by the connection's reply timeout, and their rollback by the scheduler's
  RollbackTimeout.
- The sync itself runs on the Run's ctx, and on `context.Background()` for the agent's retry.
- I did not edit TD-8's file. Proposal: TD-8b (which edits `syncLocked` anyway) wraps it in
  `context.WithTimeout(ctx, s.txnTimeout)`, one line.

## Q8. TD-11b's descriptor ownership declaration (CONTINUE note, 2026-09-25)
TD-9 registers no descriptor. The scheduler tests' `hooked`/`mem` descriptors are package-local test types that never
pass through the agent's registration guard. There is nothing to declare.
