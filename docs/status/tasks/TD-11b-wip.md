# TD-11b — WIP log

- 19:27 start (slot 1, base task/P08@998e391). Read 00-CONTEXT, shared-host-rules, architecture, envelope, review s3, LOG D-071/076/080/105/110/126.
- D-128 (manager, ~19:40): never run `show trace` / `trace add` on the shared VPP; do not run test/topology/interfaces.
- Verified on base: 3.2 (in-memory defaults; no guard), 3.3 (write-then-claim in natcommon + six DF-1 attributes; executor.create drops Meta on error), R2-stores (claim refresh has its own 5 s, subsystems.go:119) — all still open.
- Finding: df6 keyed/bypass claims through the product's persisted IfaceClaims (the documented `df6.WithClaims(w.IfaceClaims())` / df6 default iface.Claims(owner)) fail with ErrClaimUnbound — their ids are not interface names.
- done: scheduler executor.create journals Meta returned with an error (+ descriptor.go contract); DF-1 six attributes claim first (ctx-bounded claim via ContextClaimStore); natcommon generic Create claims first. Each has a test that fails on the base (outputs in TD-11b.md).
- 20:57 session-limit stop; manager salvaged the tree as 33e213c; resumed 23:00 (P08 merged on main as c2ca3ed; this branch stays on P08's pre-squash tip, the merger rebases).
- 23:00 all unit tests: agent TestRollbackReported went DEGRADED — core interface-ip returns IfMeta{} with a failed add, so journaling every non-nil Meta deleted what was never added. Fix: explicit opt-in, scheduler.PartialCreate(err) (2c1f8db).
- done: dfkit/persist guard + CheckPersistent (natcommon, DF-1, df6 keyed/bypass; dfkit.CheckClaims/CheckBoot and df2.Options.CheckPersistent helpers); subsystems.Register refuses to start on a volatile store; claim index refresh bounded by the caller's ctx (IfaceClaims.ClaimContext/ClaimedContext); PairClaims for df6; df6 keyed claim-first; df2/df6 FileClaimStore write hygiene; dfkit Target.ClaimFirst; df2.ClaimFirst (4b68f51).
- base-failure evidence: /tmp/g-w1/base-fail-evidence.txt (new tests on 998e391). Host proof passed twice on w1, NRestarts 1 → 1.
- next: CI, TD-11b.md, questions file.
- 23:40 CI --base main PASSED at c0372fd (first run: one staticcheck QF1001, fixed). TD-11b.md + TD-11b-questions.md written; IfaceClaims doc fixed (df6 → PairClaims). dist/ and bin/ removed. No processes left running (both agent processes stopped by PID in the test; tap deleted).
- 2026-09-25 00:10 fix round 1 (review be3a57a5): M2 c74a0319, M1 4c19106d, L1+L5 38591aee; fix-round tests fail on be3a57a5 (TD-11b.md); CI --base main PASSED @ 38591aee. M3 untouched (manager decision). No host runs.
