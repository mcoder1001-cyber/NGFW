# Independent identity continuation review

Initial committed head: `4ee854d204267cb2b39fa3c0a5f79aa13a81d5a9`, relative to main `2312bd4a`. Reviewer is independent of feature author. Author edits continued during execution; initial command results below are working-tree evidence, not proof of the initial committed head. Final verification pending.

## Findings

- **R1/R6 MAJOR — incorrect public banner result unwrap**: committed LoginBanner query returns `call()`'s `{data,response}` wrapper but component reads `.banner` directly. Configured banner remains invisible. Author's uncommitted `.data` unwrap resolves code shape; require final regression execution.
- **R1 MAJOR — identity UI fixture replaces existing health**: the new identity test overrides `/state/system` with only identity. AppShell reads existing `agent.reachable`, throws, and the test fails. Restore realistic health fixture fields while keeping installed-host assertion.
- **R6 MAJOR — uptime ignores formatter settings**: raw Math.floor seconds bypasses locale/Persian-digit helpers. Use existing useFormatters and assert Persian-digit behavior through settings.
- **R1 compatibility concern**: old API responses without additive identity return undefined, which TanStack rejects. Normalize absence to null and retain an unavailable-state regression.
- **R3/R7 documentation gaps**: add identity contract report/RPC contract documentation and update user/status/decision evidence for observed state and explicitly public banner. Existing WIP still states tests pending; do not convert historical lab NOT RUN into PASS.

## Aspect inspection

R2: only explicitly configured committed login text is public; no candidate, hostname, MOTD, users or secrets returned. No-store header, 4096 UTF-16-unit cap without trailing split surrogate, React literal escaping, existing route-guard inventory extended exactly. No privilege/session changes found.

R3: new protobuf RPC and field numbers are additive, consumers use matching seconds/names, health fields preserved, generated files attributed to generation. Required docs and final clean regeneration verification remain pending.

R4: fixed renderer paths, reads limited to 16KiB, slot hostname/timezone/drop-in rather than shared host identity. Runtime kernel hostname/resolver observation only enabled for globals owner. No host writes/restarts, VPP operations or privileged decision changes. Read-only file tests cover oversize, invalid uptime, timezone escape and resolver failure.

## Commands actually executed

With pinned Node22.23.2 and workspace Go1.26 env:

```text
go -C apps/agent test -race ./internal/renderers/sysident
ok ngfw/agent/internal/renderers/sysident 1.041s
pnpm --filter @ngfw/api exec vitest run src/state/login-banner.controller.test.ts src/state/system-identity-state.test.ts src/auth/route-guard.test.ts
Test Files 3 passed (3)
Tests 13 passed (13)
pnpm --filter @ngfw/web exec vitest run src/domains/system/identity/LoginBanner.test.tsx src/domains/system/identity/SystemIdentityPage.test.tsx
Tests 1 failed | 4 passed (5)
Failure: AppShell reads undefined agent.reachable after incomplete new health fixture.
```

Logs `/tmp/identity-independent-{go,api,web}.log`. API state regression was untracked while executed; banner unwrap was already modified when UI ran. No full hosted quick executed by this reviewer. Real appliance/browser/restart acceptance NOT RUN, explicitly deferred by owner into DEFERRED-ACCEPTANCE; this does not excuse the actual code/test findings above.

**Initial combined verdict: BLOCK pending corrections and final committed-head verification.**

## Final verify round — 3ccabd52338898e65ccd19d5a3fbd7eaa0a3dcb7

Product tree was clean before and after independent execution (only this untracked review document). All code and test corrections were committed; no dirty overlay was used for these results.

Resolved: banner query now unwraps `.data`; health fixture includes existing API/agent/sync fields; uptime uses shared formatter with a real persisted Persian-digit-setting regression; missing identity normalizes to null. Contract report, protobuf contract documentation and user guide now describe additive operational state, exact public committed banner scope and pending host/restart boundaries. WIP records actual generation/API/typecheck/race evidence and accurately leaves final hosted gate pending.

Actual commands, same pinned environment:

```text
pnpm --filter @ngfw/web exec vitest run src/domains/system/identity/LoginBanner.test.tsx src/domains/system/identity/SystemIdentityPage.test.tsx
Test Files 2 passed (2)
Tests 6 passed (6)
Duration 6.36s
pnpm --filter @ngfw/api exec vitest run src/state/login-banner.controller.test.ts src/state/system-identity-state.test.ts src/auth/route-guard.test.ts
Test Files 3 passed (3)
Tests 13 passed (13)
Duration 5.71s
go -C apps/agent test -race ./internal/renderers/sysident ./internal/agent -run 'Test(ObservedState|SystemIdentityState)'
ok ngfw/agent/internal/renderers/sysident 1.026s
ok ngfw/agent/internal/agent 1.068s
git diff --check 2312bd4a..HEAD
(no output; exit 0)
```

Final logs `/tmp/identity-final-independent-{web,api,go}.log`. UI verified literal markup (no script element), Persian notice, failed banner resilience, partial observed identity, preserved candidate editing and Persian uptime. API tests verify committed-only output, bounds/surrogates, empty/error behavior, health compatibility, absent older RPC and exact public-route inventory. Agent test verifies foreign-owner rejection and wired slot observation without kernel-hostname disclosure; renderer tests exercise real temporary files and error paths.

Final applicable verdicts: **R1 APPROVE; R2 APPROVE; R3 APPROVE; R4 APPROVE; R6 APPROVE; R7 APPROVE** for this code continuation. No unresolved BLOCKER/MAJOR remains from the initial review. No new privilege, auth/session decision or VPP lifecycle was introduced. **Combined code verdict: APPROVE.** Complete hosted quick on the final integration tree remains required before merge. Appliance/browser/restart acceptance remains NOT RUN and owner-authorized deferred; it is not counted as passing evidence.
