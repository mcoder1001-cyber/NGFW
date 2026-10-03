# Notifications accessible loading and localized workflow — 2026-10-03

Branch `codex/notifications-accessibility-20261003`, separate successor worktree `/root/.codex/worktrees/0b16/developers/Notifications-accessibility`, from frozen modern source `ef1a7392`. Root owns generated output and validation in its separate tree.

Owned files only: NotificationsTab.tsx; its focused test; three notifications loading strings in each management locale; notifications user guide and new Persian counterpart; this status. No contracts, API/service hooks, historical main differences, VPP, live deliveries, pushes or merges touched.

The screen's three generic loading names now identify the candidate configuration, active channels and delivery history independently in English/Persian. The regression delays only the running-config response, checks the localized progressbar name, then releases a failed response and verifies the loading indicator disappears and error appears. Both languages are covered; existing channel-source/disabled/failure regressions remain. The direct harness uses the corresponding product theme language/direction.

English/Persian docs describe save→commit→test, differences between candidate additions/deletions and running channels, admin permissions, queue acceptance versus delivery, observable error codes and concrete troubleshooting, plus existing acceptance limitations. They link to each other. No live verification is claimed.

Static verification: `git diff --check` passed; locale loading key/value symmetry checked by read-only JSON inspection. Runtime tests NOT launched here per parent delegation; validation pending root's finite lane. Root can transfer only this successor diff onto the generated Notifications-validation tree, format owned sources, then execute:

```bash
pnpm exec prettier --write apps/web/src/domains/system/management/NotificationsTab.tsx apps/web/src/domains/system/management/NotificationsTab.test.tsx
pnpm --filter @ngfw/web exec vitest run src/domains/system/management/NotificationsTab.test.tsx src/domains/system/management/locale.test.ts src/domains/system/management/ManagementPage.test.tsx --maxWorkers=1
pnpm --filter @ngfw/web typecheck
```

The successor source SHA will be supplied with the handoff. Run against the assembled generated tree; untouched source checkpoint ef1a7392 remains separate. Mandatory quick CI and independent review remain pending.
