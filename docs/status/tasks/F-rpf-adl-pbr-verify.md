# F-rpf-adl-pbr — focused verify of fix round 1

Reviewer: the agent that wrote the review (`9da6a2f`), 2026-09-24. Scope: my own findings M1–M3 and L1–L6 against
`task/F-rpf-adl-pbr` @ `b191839` (fixes `8984ff3`, `443dc5f`). This was read-only: unit tests only, no host, no `tools/ci.sh`, no
DB-backed e2e.

**Verdict: APPROVE.** Every finding is fixed or documented as the review asked. The empty `services` domain now returned by
Retrieve creates no false drift and breaks no other wave-A branch (details under M3). The only open item is the merge note at the
end, which is for the manager.

## Tests run by the verifier
```
apps/agent: go vet ./... && go test -count=1 ./...                         → exit 0
  focused -v: TestACLBridgeRegistration PASS · TestPolicyIDs PASS · TestRpfAdlPbrOnFake PASS · TestRpfAdlPbrPolicyNamesPersist PASS
              TestRpfAdlPbrWithoutFAcl PASS · TestRpfAdlPbrProjection/Errors/Assemble PASS · (P08) TestApplyRetrieveIdempotent PASS
              TestGRPCRoundTrip PASS · TestRpfAdlPbrOnHost SKIP (no VRX_INTEGRATION)
apps/api:   vitest src/features/rpf-adl-pbr → drift.test.ts 3 passed, pbr-state.test.ts 3 passed
apps/web:   vitest src/nav src/domains/routing/rpf-adl-pbr → 13 passed
packages/schema: vitest src/semantic/rpf-adl-pbr.test.ts → 10 passed
```

## Findings

| # | Status | Evidence |
|---|---|---|
| M2 | **fixed** | The harness registers `acl.acl` only while the name is free (`subsystems/rpf_adl_pbr_test.go:80-82`). The product skips the bridge when `acl.acl` is already registered (`registerACLBridge`, `subsystems/rpf_adl_pbr.go:87-95`). `TestACLBridgeRegistration` covers both orders. "acl.acl first": the bridge is not registered and nothing panics. "acl.acl after": the bridge is active before, then inert — Retrieve returns nil, `ProvidedKeys` returns nil, no `acl_dump` sent. One nit: the "harness does not register twice" step builds a fresh registry, so the harness guard itself will only really be exercised when F-acl merges. The guard is one line and obviously correct. |
| M3 | **fixed** | `COVERAGE_RULES` now includes `'agent.write-only'` (`apps/api/src/state/state.controller.ts:412-416`; manager-approved and listed under shared hunks). `drift.test.ts` covers three cases: write-only leaves skipped with real drift kept; ADL switched off in VPP is still drift; an error-severity issue is never a coverage note. The `/services` domain-level note and the `"autoSdl"` hard-code are gone. The replacement is `desired.ServicesMembers`: append-only, one `init()` line per feature in its own file (`desired/rpf_adl_pbr.go:382-387`, used at `:416`). `services.autoSdl` now gets a field-level `agent.write-only` note on the globals owner and `agent.unsupported-field` on other agents. |
| M3 side effect | **acceptable** | Retrieve always returns `services: {}` (`RpfAdlPbrAssemble`, created only when nil). **No false drift:** every member of `ServicesConfig` is a message (fields 1–8). A non-empty member carries a field-level note, so the diff skips it (the drift view skips notes at depth ≥ 2); an empty member is an empty container, which is not drift; a running document without `services` gives `add /services {}`, which is skipped too. **Other branches:** the only whole-document Retrieve comparisons are P08's `canonicalDoc` (`agent/service_test.go`) and `hostDoc` (`agent/agent_integration_test.go`), and both were updated here. Every other Retrieve assertion in F-bonding, F-bridge-l2, F-neighbors-ra, F-vrf-static-ecmp, F-nat44-ed-sessions, F-object-model and F-acl is scoped (`Subsystems: [...]`) or length-only (`agent_integration_test.go:257`). At merge, F-nat44-ed-sessions and F-object-model edit the same two P08 test files; keep `"services": {}` in the union. A later services implementer must add its members to the existing struct, not replace it. F-loopback's assemble anchor runs before this task's, so that is safe; wave-B assemblers anchored after this one must mutate `ds.Services`, not reassign it. |
| M1 | **documented (manager item)** | Questions file Q10 lists the `feature_is_enabled` collision with F-bridge-l2 (mactime), the `adl_interface_enable_disable`/`acl_dump` overrides, and the duplicate `extensions` seam (F-nat44-ed-sessions). The fix belongs at merge time, as the review said. |
| L1 | **fixed** | The user page has the "until F-acl is merged … `agent.dependency-missing`" note. `registerACLBridge` logs a warning when the registry cannot be queried. |
| L2 | **fixed** | Policy ids are sticky: recorded ids from the `pbr.policy` store are kept, and only new names are probed (`desired/rpf_adl_pbr.go:98-133`). `RpfAdlPbrEnv().RecordedIDs` comes from the store. `TestPolicyIDs` proves three things: a colliding earlier name moves `b` without records; `b` is kept with records; records out of range or taken twice are re-probed. `TestRpfAdlPbrPolicyNamesPersist` checks that the records survive a restart. |
| L3 | **documented (accepted)** | `docs/agent/descriptors/adl.md` describes the "Known window" with both paths and states that no path stores `~0`. |
| L4 | **fixed** | The V-new text now says allow-list instances survive an interface delete. The wrong "(ADL per-index config is re-initialised…)" remark is removed from V23. |
| L5 | **fixed** | The web hotspots are anchor-only against the W-seed tip `df67a8e`: `router.tsx` +2, `nav.ts` +2, `nav.test.ts` +2, `i18n.ts` +6, and no reformatting. |
| L6 | **fixed** | `F-rpf-adl-pbr-contract.md` states that `b7439d4` carries both the schema and the proto change, and that `63be7be` changes no proto. |

## Merge note (manager)
`task/W-seed` was squash-merged into main (`0ae3559`), so this branch's merge base with main is old. `git merge-tree main HEAD`
reports 25 conflicts, all from that lost ancestry. With the W-seed tip this branch merged as the base
(`git merge-tree --merge-base=df67a8e main HEAD`), only two generated files conflict: `operations_gen.go` and
`api-client schema.d.ts`. Regenerate them (rule 3). Merge on that basis, for example by applying `df67a8e..task/F-rpf-adl-pbr` onto
main with a 3-way apply, then `pnpm gen && make -C apps/cli gen docs`. Carry M1 (Q10) into that merge and into F-bridge-l2's.

**APPROVE**
