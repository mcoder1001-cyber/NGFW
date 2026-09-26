# F-srv6 — questions and notices for the manager

Written while working; nothing here blocks the task (the higher-precedence rule was applied, work continued).

## Q1 (notice) — contract committed on the task branch
`contract(schema): routing srv6` and `contract(proto): routing srv6, Srv6State` are the first commits of
`task/F-srv6`; details in `docs/status/tasks/F-srv6-contract.md`. Config home `routing.srv6`, `RoutingConfig` 17 —
decided in wave-BC-numbers.md, taken as is.

## Q2 (decision, please confirm) — per-policy `encapSource` override modelled
Prompt open question "whether a per-policy encap source override is worth modelling". Decided **yes**: D-074 wants a
source on every encap policy, and D-071 makes the global `encapSource` unusable on every non-owner agent (a
write-only global's require variant always fails). Resolution: policy `encapSource` → `routing.srv6.encapSource` →
error `routing.srv6-encap-source` (400 with pointer). Options (a) global only (b) override (c) per-policy only.

## Q3 (open, product) — service chaining (End.AD / End.AM / End.AS)
Not buildable in FAST MODE: VPP 26.06 `src/plugins/srv6-{ad,am,as}` have no `.api` file; the behaviours exist only
as CLI and are not in `sr_types.api`'s behaviour enum. Written as `### V-new (F-srv6)` in docs/vpp-code-track.md
(needs a VPP API patch: an `.api` per plugin, or an `sr_localsid_add_del` variant with plugin behaviour + params).
Does the product need service chaining at all? It is only relevant for SFC deployments (T3 in the WBS).

## Q4 (notice) — the D-074 board note "manager generates srv6_ad/am/as binapi" cannot be done
There is nothing for binapi-generator to generate (no `.api.json`). Please drop the note from the board/LOG follow-ups.

## Q5 (notice) — envelope says `df6.WithClaims(Wiring.IfaceClaims())`; main requires `PairClaims("df6")`
Since TD-11b, `Wiring.IfaceClaims()` binds every claim to an interface's sw_if_index and df6 refuses it
(`df6.ErrClaimStoreKind`; the agent would refuse to start). The wiring uses `df6.WithClaims(Wiring.PairClaims("df6"))`
as the code on main documents. F-mpls-srmpls (also df6) shares the same file `claims-df6-<owner>.json` — the keys
carry the holder (`sr.localsid@vpp-…`), so there is no collision.

## Q6 (notice) — TD-11b declaration for DF-6 globals added in `descriptors/sr` (gap)
`df6.SingletonDescriptor` / `df6.RequireDescriptor` (read-only package) declare neither `CheckPersistent` nor
`RecordsNoOwnership`, so registering `sr.encap-source` / `sr.encap-hop-limit` made the product agent refuse to start
(TD-11b guard). `sr.Register` now wraps the two globals in a decorator that declares `RecordsNoOwnership` (a VPP-wide
setting records nothing) and forwards `DeleteOnAbsence`/`Unwrap`. F-lisp (lisp enable singletons) will need the same;
a df6-level fix (declare on the generic types) would serve both — manager's call.

## Q7 (notice) — shared hunks outside anchors
- `packages/schema/src/domains/routing.ts`: one import line at the end of the import block (the key line is under the
  anchor) — same as F-rpf-adl-pbr's `pbrField` import.
- `apps/api/src/testing/fake-agent.ts`: one import line at the end of the imports (the file has no import anchor;
  F-wireguard did the same).
- `apps/agent/internal/descriptors/core/coretest/fakevpp.go`: see Q8.

## Q8 (notice) — coretest extension hook (TD-23 not on main yet)
The SR model lives in the new file `coretest/srv6.go`. Until TD-23's `RegisterExtension` is on main, `coretest.New()`
cannot pick it up without a call, and every agent test that retrieves the routing domain needs the SR dumps. One line
in `New()` calls it (listed under Shared hunks). After TD-23 merges this becomes
`func init() { coretest.RegisterExtension("srv6", (*VPP).installSRv6) }` in `coretest/srv6.go` and the line in
`fakevpp.go` goes away (at the D-112 rebase).

## Q9 (notice) — `sdk/python/vrx/_generated` not regenerated
The OpenAPI document changes (routing.srv6 schema, `GET /api/v1/state/srv6`). `sdk/` is not in this task's file list
and `sdk/gen.sh --check` is not part of `tools/ci.sh` on main; the manager regenerates at merge if wanted
(`sdk/gen.sh --openapi packages/api-client/openapi.json`).

## Q10 (notice) — screenshots taken against the real API with the API's FakeAgent
Host runs are closed until TD-25, so the committed screenshots (`docs/user/vpn/img/srv6-*.png`) show the production
web build against the real vrx-api whose agent was `apps/api/src/testing/fake-agent.ts` served on a unix socket (sample
counters). The real-agent screenshots are one command after TD-25: `test/topology/srv6/stack.sh <shots script> <dir>`.

## Q11 (notice) — pre-existing prettier state and a known trivial conflict
`apps/web/src/nav/nav.ts` / `nav.test.ts` already fail `prettier --check` on main (not reformatted here). The
`import { lazy } from 'react'` line at the top of `apps/web/src/domains/vpn/tabs.ts` is also on task/F-wireguard: the
second of the two to merge drops the duplicate line; `'vpn'` in BUILT_DOMAINS is the same duplicate union.

## Pending host steps (after TD-25)
Listed in `docs/status/tasks/F-srv6.md` → "Pending host steps (after TD-25)": `TestSrv6OnHost` (binapi loss
simulation, vppctl shows, FIB leak scan), `test/topology/srv6/stack.sh` (API-level evidence + real-agent screenshots),
optional `TestSrv6GlobalsOnHost` (VRX_FSRV6_GLOBALS=1, manager window), NRestarts before/after.
