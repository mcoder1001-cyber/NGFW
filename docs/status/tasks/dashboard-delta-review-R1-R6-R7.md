# Dashboard CI delta — independent R1 / R6 / R7 review

Reviewed product/test head: `a675e402f82a2334bcbca6aaac43a8f00c2c0eed` (2026-10-02), relative to `b1dc666c`. Reviewer did not author the dashboard feature or MPLS test change. Scope: the one MPLS test delta and checkpoint documentation; inherited dashboard feature reviews remain applicable.

## Findings and verdicts

No BLOCKER, MAJOR or MINOR found in this delta.

- **R1 APPROVE:** original exact Persian tab and heading assertions remain. The test now follows the application's actual persisted UI-settings language change rather than mutating i18n alone while leaving the theme language English. Closing the modal with Escape restores page accessibility roles. There is no skip, timeout increase, changed fixture, production alteration or weakened assertion. Added HTML direction/language assertions detect incorrect RTL setup. The historical hosted timing failure is not claimed reproduced or conclusively diagnosed.
- **R6 APPROVE:** language selection uses accessible Settings, Language and فارسی controls, then checks both feature translations and actual `html dir="rtl" lang="fa"`. This establishes jsdom coverage, not browser screenshot or appliance acceptance. No user-visible product strings or CSS changed.
- **R7 APPROVE:** checkpoint honestly distinguishes 7/7 focused verification from pending complete hosted CI and deferred real acceptance. This test correction is required to satisfy the existing complete gate; scope and uncertainty are explicit. No privileged or pending security decision is made.

## Independent execution

Worktree: `/workspace/scratch/96b8b6fbc8a7/NGFW-dashboard-fix`; pinned Node `v22.23.2`.

```text
source /workspace/scratch/96b8b6fbc8a7/toolchain/env.sh
pnpm --filter @ngfw/web exec vitest run src/domains/routing/mpls-srmpls/MplsPage.test.tsx
✓ src/domains/routing/mpls-srmpls/MplsPage.test.tsx (7 tests) 8423ms
  ✓ renders in Persian (RTL) with the feature strings 2615ms
Test Files 1 passed (1)
Tests 7 passed (7)
Duration 11.03s
```

Full log: `/tmp/dashboard-independent-mpls.log`. No full quick gate was executed by this reviewer: manager is running the unchanged hosted gate on the final integration tree. Full hosted success remains a merge prerequisite. Real VPP/browser/alarm/restart acceptance is NOT RUN and centralized in `docs/status/DEFERRED-ACCEPTANCE.md`; the product owner's explicit laboratory deferral permits merge without turning these results into PASS.

**Combined delta code verdict: APPROVE.** Final published head must retain this exact test delta and receive green complete hosted CI.
