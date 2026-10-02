# Notifications R7 — documentation correction verification

Original R7 BLOCK report `notifications-resume-review-R7.md` remains preserved. Independently reviewed documentation correction `6426244381395357d0b2fa8988abe2da5487fc07`, tree `89155d2cef448f0060cdab8a87374cd14e39f685`, following report integration `db744645` / `656949f8` onto main `8f07ce68`. Own worktree `/workspace/scratch/de92de7d9874/NGFW-notification-r7-verify`, branch `review/notification-r7-verify-20261002`. No product changes.

Both R7 MAJORs are resolved:

1. Central notification acceptance now distinguishes unfinished VRF/IPsec **code**, host-independent successful SMTP/HTTPS receiver/browser **tests**, and actual routing/database/API restart/commit laboratory **NOT RUN** cases. The dated debt table assigns F-notifications ownership, manager scheduling/verification, and completion before full feature acceptance. Queue/history restart loss remains explicit. This is honest owned follow-up work, not a claim that the feature is complete or that a unit fixture proves delivery acceptance.
2. D-175 records the bounded SMTP/webhook increment, alternatives considered, rationale, reversal cost and unchanged secret/security/gate boundaries. No protected decision is silently waived. Telegram removal remains covered by D-171.

The optional navigation issue is also addressed: WIP begins with current bounded status, manager-reported remote checkpoint, review links, exact next gate, and clearly labels historical recovery notes. Original BLOCK reports remain accessible. Board completion is not claimed; broader manager-owned board refresh remains outside this correction.

Actual narrow verification in the isolated worktree:

```text
git diff --exit-code df35285c..HEAD -- apps packages pnpm-lock.yaml tools
(exit 0, no output)
```

Thus the notification application/schema/generated/tooling product matches its reviewed predecessor. Full-tree differences also include separately integrated packaging changes from the newer main; they are not described as notification product edits or re-reviewed here. The final documentation correction itself changes only five Markdown files.

A Python check resolved every relative Markdown target in the changed WIP, central campaign and debt file and verified the new debt heading/anchor:

```text
R7 links: 15 local targets exist; new notification debt anchor verified
```

No broad product tests, generator run, hosted CI or lab acceptance was rerun or claimed. Current text correctly leaves the exact-final-tree hosted quick requirement and full-feature work open.

**Verdict: APPROVE** for notification R7 documentation/process scope at the identified checkpoint. This supersedes the prior R7 BLOCK for its two documentation MAJORs only; all independent product reviews and the unchanged complete hosted quick gate remain integration requirements.
