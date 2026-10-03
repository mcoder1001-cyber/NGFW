# F-srv6 review (cloud session, at merge)

Reviewer: cloud manager session, 2026-09-26. Branch `task/F-srv6` @ `e82a9cd2` (base `main@1d3ccf31`), reviewed as
ported onto main (`port/F-srv6`). Checklist: prompts/REVIEW-PROMPT.md.

## Verdict: **APPROVE WITH CHANGES** — no code defect blocks; H1 is the follow-up row `F-srv6-host`, L1/L2 are noted

## Findings

### H1 — no host proof yet (checklist 2)
Fake-VPP (`coretest/srv6.go`) agent tests, API e2e against PostgreSQL with the fake agent, and screenshots of the
production build against the real ngfw-api with a fake agent socket. `srv6_integration_test.go` and
`test/topology/srv6/stack.sh` are written but did not run (host runs were closed until TD-25; a cloud session has no
VPP). **Resolution:** follow-up row `F-srv6-host` (ready, deps F-srv6): the "Pending host steps" of F-srv6.md with
NRestarts before/after and the real-agent screenshots.

### L1 — perpetual drift when a policy's own encapSource equals routing.srv6.encapSource (`desired/srv6.go` AssembleSrv6)
The assembler leaves a policy's `encapSource` unset whenever it equals the global source the globals owner applied
(it cannot tell "inherited" from "explicit and equal"). A document that sets both to the same address therefore
always shows a drift on `/routing/srv6/policies/<bsid>/encapSource`. Harmless (nothing is re-applied), but the drift
view is wrong. Fix options for a later row: normalise it away in the projection-side canonical form, or a schema
rule that refuses an explicit source equal to the global one. Not fixed here.

### L2 — the two SR globals fail the commit on a non-owner agent (decision 5 in F-srv6.md)
`routing.srv6.encapSource` / `encapHopLimit` on a slot agent fail the transaction (df6's require variant). F-lb
settings, LLDP globals and nsim warn `agent.unsupported-field` instead. Both follow D-071; the behaviour differs per
feature. Left to the manager (one line in decisions if the difference is intended).

### Integration (not defects)
- `coretest` SRv6 model registered through TD-23's `RegisterExtension("srv6", …)` as its own comment asked.
- nav: `'vpn'` was listed twice (F-lisp already built the group); the duplicate became an anchor comment.
- `srv6_test.go` idempotency check allows main's read-only getters (`*_get`, `feature_is_enabled`,
  `policer_dump_v2`), the same list `service_test.go` uses.
- reachability: `descriptors/sr` wired, maxPending 26 → 25.

## Checked, no finding
1. Contract: additive — `RoutingConfig.srv6 = 17` (allocated in wave-BC-numbers), Srv6State RPC, schema `routing.srv6`;
   buf breaking against main clean; contract commits and `F-srv6-contract.md` present.
3. Restart safety: SIDs, policies and steering are claimed in the persisted df6 claim store (TD-11b); the globals
   are declared `RecordsNoOwnership` and are idempotent setters; restart tests on the fake VPP.
4. VPP API provenance: only existing `sr_*` messages; binapi untouched.
5. Shared-host rules: the globals are applied only by the globals owner (D-071); objects carry owner claims.
6. Security: no exec, no CLI; the only route is `GET /state/srv6` behind the guard.
7. Transaction semantics: dependencies give the delete order steering → policy → VRF (no SR FIB entry left in a
   deleted table); no table flush.
8. UI: the VPN › SRv6 tab calls the real `/state/srv6` and the config routes; no TODO/mock/stub in product code;
   interim screenshots committed (en + fa/RTL).
10. i18n: en + fa; `check-logical-css` clean.
11. Tests (this session, on the port): gofmt, go vet, golangci-lint 2.13.2 (0 issues), `go test -race` agent,
    pnpm gen (no drift), build, typecheck, lint, vitest; `tools/ci.sh check --base`.
