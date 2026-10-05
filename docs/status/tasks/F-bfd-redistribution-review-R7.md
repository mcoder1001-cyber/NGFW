# F-bfd-redistribution independent R7 review

Source inspected:417845d8f0b0c6329753f2f98fa305c9446da2cf, 2026-10-05; /root/ngfw-wt/bfd-final-gate-20261005. Manager reports arithmetic lint correction still in progress: this report grades inspected documentation, not a future source SHA or its CI.

MINOR: questions file still says multihop remains rejected/source-incomplete. Mark that historical bullet superseded by durable endpoint ownership and one-way globals-owner activation. MINOR: prepend current branch/SHA/gate state to canonical and developer wip; their original branch/checkpoint remains chronological provenance while manager repair is separately documented.

Read-only commands actually executed: cat canonical/wip/questions, F-bfd-compensation-wip.md and docs/user/routing/bfd-redistribution.md; git diff --name-only origin/main...HEAD. Observed compensation report: focused race descriptor1.361s/agent2.264s PASS; scheduler DEGRADED and persisted-store reopened recovery; EEXIST/boot/index reuse refusal; proof-write failure remains partial; UI19 tests117.48s; focused lint0issues. These are inspected execution claims, not tests this reviewer reran. Full unchanged current source quick is pending, explicitly no green inherited from older source.

Docs accurately distinguish VPP and FRR ownership/port conflict, microsecond API units versus whole-millisecond FRR inputs, absent unknown counters/last-flap timestamps, protocol-owned peers, committed redistribution source and unsupported hit-counter API. The recovery note now explains exact successful-add evidence, partial rollback and realistic crash-durability limit during total storage failure; successful recovery of ordinary claim failures is not mislabeled foreign-session adoption. User docs state multihop activation has no disable until VPP restart and removal only releases owned sessions/claims. Actual Up/down/redistributed packets/daemon rollback remain laboratory acceptance, with no shared VPP restart claimed. Scope is allocated additive contracts/consumer integration; necessary shared hunks are listed and prior C-required conjecture is superseded by API-only implementation. No silent new security-boundary decision appears.

No R7 BLOCKER/MAJOR. Remaining independent reviewers, lint correction, frozen exact-tree full quick/hosted gate and latest-main D112 integration remain mandatory. Approval does not certify pending gates or future substantive source edits.

Verdict: APPROVE for inspected documentation.
