# Independent R7 evidence review envelope

Task: inspect PR217 documentation/evidence and scope. Initial final candidate:
`bde83bc87ae817a61bbc70e4029f76109ae77c35`; base main
`d2d55984d74fa1d06c32e8271886f11f16375407`.

Role: independent reviewer R7, not product developer or hardware installer.
Branch: `codex/hardware-evidence-review-20261010`.
Worktree: `/root/ngfw-wt/hardware-evidence-review-20261010`.
Owned files: `docs/status/tasks/hardware-evidence-review-20261010-*` only.
No product, manager worktree, board, shared VPP or target mutations. No live
integration, service startup, filesystem repair or reboot is assigned.

Read AGENTS.md, prompts/00-CONTEXT.md, docs/contributing.md,
docs/decisions/decision-policy.md, prompts/REVIEW-PROMPT.md,
prompts/reviewers/R7-docs-evidence.md and docs/lab/shared-host-rules.md.
Owner's publication/parallel-work requirements override historical local-only rules.

Review scope: truthful completed versus pending checks, exact source provenance,
remote reviewed history, preserved management/NIC inventory, no hardware acceptance
claim, recovery boundary, separate observed live roles versus board task states.
Output: precise findings/verdict with commands and selected actual output in an
owned R7 report; commit/push each coherent checkpoint and verify remote SHA.

Initial issue: prose PASS claims lacked committed command/output selections.
Manager published linked evidence appendix on amended final candidate
`5bd7e8b545fc765fd2babd8dda15175d6f33af1b`; focused docs-delta recheck closes R7-1.
No new product or target operations are assigned. Final hosted quick/hardware
acceptance are not established by this R7 review. Current verdict/report is owned
in `hardware-evidence-review-20261010-review-R7.md`.

Additional manager-assigned read-only follow-up: verify postmerge public checkpoint
925703c2 and final recovery checkpoint0f3ab280, exact merge4908716b/treea0d7b7,
source/main-CI distinction, private ready configuration-snapshot metadata and
candidate exclusion. Inspect no secret values; never print/commit backup contents.
No product/target edits or duplicate full quick. Owned addendum:
`hardware-evidence-review-20261010-postmerge.md`. Source APPROVE remains separate.

Resumed for owner-authorized filesystem repair feasibility and independent safety
review. New owned report: `hardware-evidence-review-20261010-ram-recovery.md`.
Read-only research/host-worker evidence only; no target staging, services, network,
root repair, pivot/switchroot, kexec or reboot. Review concrete RAM maintenance
preparation versus verified offline device/recovery/data preservation requirements;
do not confuse missing capability evidence with missing owner authorization.
Additional owned task-only helper report:
`hardware-evidence-review-20261010-block-check.md`. Independently inspect/rebuild
manager helper and staged host receipts without target mutations or product edits.
Additional owned source/runtime review report:
`hardware-evidence-review-20261010-ram-stage.md`. Record immutable stage-source
identity, exact findings and actual checks; keep preparation approval distinct from
transition/repair gates. Controller-only isolated owned-loop helper testing is
permitted; never operate on physical/target devices during reviewer tests.
Additional owned proposed-return review:
`hardware-evidence-review-20261010-return.md`. Source-backed single-force semantics
and exact preconditions, no reboot execution or acceptance claim.
