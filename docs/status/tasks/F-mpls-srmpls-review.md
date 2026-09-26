# F-mpls-srmpls review (cloud session, at merge)

Reviewer: cloud manager session, 2026-09-26. Branch `task/F-mpls-srmpls` @ `d47c9e68` (base `main@1d3ccf31`), reviewed as
ported onto main (`port/F-mpls-srmpls`). Checklist: prompts/REVIEW-PROMPT.md.

## Verdict: **APPROVE WITH CHANGES** — M1 fixed in the port commit; H1 is the follow-up row `F-mpls-srmpls-host`

## Findings

### H1 — no host proof yet (checklist 2)
Fake-VPP agent tests (`coretest/mpls_srmpls.go`), API with the fake agent, jsdom web tests and screenshots against the
API's FakeAgent. `TestMplsOnHost` is written and did not run (host runs closed until TD-25; the table-0 / SR-MPLS half
needs a manager window; a cloud session has no VPP). **Resolution:** follow-up row `F-mpls-srmpls-host` (ready, deps
F-mpls-srmpls): `TestMplsOnHost`, `vppctl show mpls fib` / `show sr mpls policies` before and after rollback, NRestarts
before/after, real-stack screenshots.

### M1 — write-only leaves showed as permanent drift (`desired/mpls_srmpls.go`)
`routing.mpls.ipBindings` and `routing.mpls.sr` have no dump in VPP 26.06 and are never assembled, but the projection
emitted no coverage note, so `/state/drift` reported them as missing on the data-plane side forever (the task's own
Q7; the user page said so). Main has the mask since D-147: a `agent.write-only-field` warning at a pointer makes the
drift view skip it. **Fixed:** the projection warns `agent.write-only-field` at `/routing/mpls/ipBindings` and
`/routing/mpls/sr` when they are set; `TestMplsProjection` asserts both notes; the user page now says they are
ignored by `show drift`.

### L1 — the FIB view of the shared table 0 lists every owner's labels (`agent/rpc_mpls_srmpls.go`)
`view=fib` on table 0 pages `mpls_route_dump` as VPP has it, including labels other slot agents own; Retrieve still
reports only this owner's recorded labels. On the product (one agent) this is the whole table and correct; on the
shared lab host it is a read-only view across slots, like the VRF FIB browser. Noted, not changed.

### Integration (not defects)
- `coretest`: the MPLS/SR-MPLS model registers through TD-23's `RegisterExtension("mpls-srmpls", …)` (the branch said
  to switch once TD-23 is on main); its `b2u` (uint8) clashed with lisp.go's (uint32) and became `mplsB2u`.
- TD-11c guard: `mpls-tunnel` now provides its alias, so it left `knownAliasCreatorGaps` (shrink-only list).
- reachability: `descriptors/mpls` and `descriptors/sr_mpls` wired, maxPending 25 → 23.
- nav test: `mpls` sits after `routing` (nav.ts pushes it before main's wave-A routing items).

## Checked, no finding
1. Contract: additive — `RoutingConfig.mpls = 15` (allocated), MplsState RPC, schema `routing.mpls`; buf breaking
   against main clean; contract commits and notes present.
3. Restart safety: table-0 label routes record-first in the persisted BootStore (dropped again when the add fails);
   MPLS interfaces claim-first (TD-11b, `c.Undo` on failure); SR-MPLS in the df6 claim store; `TestMplsRestartSimulation`.
4. VPP API provenance: only existing `mpls_*` / `sr_mpls_*` messages; binapi untouched.
5. Shared-host rules: MPLS table 0 is created/deleted only by the globals owner; other agents only require it
   (`NewTableFor`, `TestMplsTableZeroRequired`); table ids from the agent's id range (TD-8).
6. Security: no exec; read-only state routes behind the guard.
7. Transaction semantics: dependency order interface → table → routes/bindings → tunnels → SR policy → steering;
   routes depend on the IP tables their paths look up in (new `lookupTableDeps`).
8. UI: the Routing › MPLS screen calls the real state routes; no TODO/mock/stub in product code.
10. i18n: en + fa; bidi-safe label paths; `check-logical-css` clean.
11. Tests (this session, on the port): gofmt, go vet, golangci-lint 2.13.2 (0 issues), `go test -race` agent,
    pnpm gen (no drift), build, typecheck, lint, vitest; `tools/ci.sh check --base`.
