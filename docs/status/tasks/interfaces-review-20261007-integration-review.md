# Bounded Interfaces / wizard integration review

Compared independently via git show/diff without cherry-picking wizard product:
- Interfaces: e9e94d17e56470caf7526a61c59d915cc2c0866e (previous R1–R7 source approvals).
- Wizard: dfb7844a84c8653f7fbf59845da5654d68355697 against3ddb1680e.
- Exact combined integration tree is not supplied yet. No product writes or live host actions.

## R6 MAJOR: partial observation metadata must reach wizard user

Wizard SetupWizardPage.tsx only warns on `interfaces.isError`. New Interfaces state contract intentionally converts retrieve/live/HostNics failures to HTTP200 with available rows plus `observationErrors` and availability flags. In a combined tree, VPP observation may fail while configured rows remain. The wizard then presents that subset without its existing failure/retry warning, concealing incomplete discovery. The Interfaces page itself correctly surfaces this metadata.

Fix: show translated partial observation warning plus retry when the successful payload indicates failed sources. Preserve available configured/live selections. Regression must cover HTTP200 with retrieve/live observationErrors and configured selections still accessible. Root accepted this finding and will implement the contract-consumer fix in its own combined integration worktree; independent reviewer must verify final delta.

This is a combined-contract issue, not a blocker on isolated wizard PR198 against the old all-or-error endpoint.

## Compatible behavior inspected

- Host inventoryOnly rows have state:null and cannot pass wizard live eligibility. Management/host-only discovery remains observational and cannot trigger implicit adoption.
- Configured existing rows retain running-document selection; host-owned/local0 are excluded. Live unconfigured rows must be parentless, unmanaged, eligible physical engine types/defaultVRF with valid parent name. The selected peer cannot also be WAN/LAN.
- API setup preview/stage does not trust inventory UI metadata. Missing selected config names must be observed independently from actual InterfaceState, with eligible type, unmanaged/defaultVRF and nonzero index. It clones running before default insertion; preview is non-mutating, stage retains existing transactional password+config flow and fresh revision/candidate safeguards.
- Optional additive REST fields do not break wizard parsing; explicit state:null short-circuit avoids null dereference. Interface API/client field names remain unchanged.

No command/test pass claimed for the combined tree; final integrated SHA, consumer warning fix/regression and full unchanged gate still pending.

Initial verdict: BLOCK pending the R6 MAJOR consumer fix; superseded by the independent verification below.

## Final independent verification

Combined integration source: `134543ff619feb49bdca7fdce55e3e57b61f0b6f` on `codex/interfaces-integration-20261007`. Imported exact wizard checkpointdfb7844a and consumer134543ff6 into own worktree (own consumer copye014d16d0), keeping Interfaces e9e94 product unchanged. Decision log resolved using exact combined d0ffe4213 blob, preserving D239/D240; no reviewer product edits.

Consumer predicate now displays warning/retry for request errors OR any nonempty observationErrors, covering retrieve/live/hostInventory failures. The original source-specific contract and read-only inventory remain unchanged. EN/FA wording accurately says some network interfaces failed to load. Configured choices remain available and a successful retry clears the warning without resetting WAN selection.

Own actual command/output:

```text
git diff --name-only 134543ff6..HEAD -- apps packages deploy tools .github
(no output: exact product tree matches the combined source)
TMPDIR=/root/ngfw-review-tmp/interfaces-review pnpm --filter @ngfw/web exec vitest run src/domains/system/setup/SetupWizardPage.regression.test.tsx -t 'warns on partial observation failure and retry preserves configured choices'
✓ src/domains/system/setup/SetupWizardPage.regression.test.tsx (4 tests | 3 skipped) 9672ms
✓ warns on partial observation failure and retry preserves configured choices 9664ms
Test Files 1 passed (1)
Tests 1 passed | 3 skipped (4)
Duration 26.69s
```

The selected regression independently proves HTTP200/live error warns, configured WAN is preserved across retry, configured LAN remains usable, at least two state reads occur and no config POST occurs. Other three tests are intentionally excluded; no full-suite claim. Existing router HydrateFallback console warning is unchanged and not a test failure.

R6 MAJOR resolved. No new compatibility/security/atomic-preview issue found. State:null host inventory cannot be selected/adopted; host-owned/local0 remain excluded; missing setup interfaces are independently validated using actual agent InterfaceState before constructing a non-mutating preview/atomic stage. Prior source R1–R7 approvals remain applicable.

Final verdict: APPROVE combined source134543ff6. Mandatory full unchanged integration quick gate, current-tree hosted CI, D112 archive/squash/expected-head merge and post-merge main CI remain manager requirements. No live host/lab acceptance or full integration gate pass is claimed by reviewer.
