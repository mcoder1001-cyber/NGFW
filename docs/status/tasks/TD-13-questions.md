# TD-13 questions (none of them blocks; each says what I did)

All six are answered (manager, fix round 1, 2026-09-25). Each answer is written under its question.

**Q1: "once more before the first Create" (envelope scope 1b).** I read this as: the commit engine's DryRun validates,
and then Apply validates once more before it writes anything.
- What I built: Apply validates once, inside `plan()`. That is after planning and before the first operation, deletes
  included.
- I did not add a second run inside the same Apply. Nothing between the plan and the first operation can change a
  validator's input: the plan and its view are fixed under the transaction lock, and validators are read only.
- A second run would only double the checker cost. If a literal second call right before the first Create is wanted,
  it is one call in ApplyWith. Say so.
- **Answer:** one pass is enough.

**Q2: TD-9's drift check runs Plan.** `Service.CheckDrift` (TD-9, not merged) runs a Plan every 5 min. After TD-9
merges, that Plan also runs the validators of drifted daemon objects. They are read only and bounded (≤ 30 s each),
and they run under the txn lock that CheckDrift takes.
- Two drift-count effects:
  - A rejected object now counts as 1 issue instead of 1 op, because a plan with issues keeps no ops.
  - A drifted daemon object whose checker fails is reported as an issue, which is arguably more useful.
- Options for TD-9's rebase:
  - (a) accept (my recommendation);
  - (b) add a `PlanOptions{SkipValidators}` for the drift check. That is a small scheduler hunk, and I have not built
    it.
- **Answer: (b).** Built in fix round 1 as `Scheduler.PlanWith(…, PlanOptions{SkipValidators: true})`, tested by
  `TestPlanWithSkipValidators`. At its rebase, TD-9's `CheckDrift` passes it (TD-13.md, "Fix round 1").

**Q3: timeout constant.** TD-9 is not merged, so `DefaultValidateTimeout = 30 s` is a local constant in
`scheduler/validator.go`, with a note in its godoc.
- ~~At TD-9's rebase it may alias `vpp.DefaultReplyTimeout` (or core's), if the manager wants one constant.~~
  **Answer:** no alias. The scheduler imports no ngfw package, so the constant stays local (the godoc now says why).
- `Scheduler.ValidateTimeout` uses 0 for the default rather than a value set in `New()`, so I stayed out of TD-9's
  `New()` hunk. The rebase may normalise that to TD-9's `RollbackTimeout` style: set in `New`, 0 = no bound.

**Q4: `docs/contracts/proto.md` §3.** The new stable rule id `agent.validator` is documented in
`docs/agent/scheduler-validators.md` and in the D-entry draft. proto.md is not in my owned files. Proposed line for
§3: "`agent.validator`: a descriptor's tier-3 check (the daemon's own checker on a staged copy) rejected the object;
`pointer` is the leaf the checker named, else the object's".
- **Answer:** docs only. Added to proto.md §3 in a `docs(contracts):` commit (no `contract(` subject: proto.md is
  not a contract path).

**Q5: redaction scope.** (Superseded; review M1, fixed in round 1.) The first version masked strings under
secret-named fields or map keys, plus D-051 references. That masked non-secrets: BGP communities, and everything
under an object named `psk0`.

Its SNMP reasoning was also wrong. `SnmpService.TrapReceiver.community` is the *name* of a community defined in
`communities`, not its plaintext. The plaintext crosses only as `Community.secret_ref`.

The scheduler now masks only what D-040 lets carry a secret:
- every D-051 reference;
- every `*_ref` string field.

Plaintexts that a renderer resolved are the renderer's job (`rfkit.Redactor`, the strongSwan and FRR
`toolMessage`).

**Q6: double checker run.** An adopter keeps its Create check (defence in depth), so a changed daemon object is
checked twice per Apply: once in the plan and once in Create. Kea, for example, runs `kea-dhcp4 -t` twice. That is
~100 ms extra per daemon per commit. A feature may drop the Create check later, but I recommend keeping it.
- **Answer:** keep it, and document "up to 3 checker runs per commit": the commit engine's DryRun, Apply's plan and
  Create. Now in the Limits of `docs/agent/scheduler-validators.md`.
