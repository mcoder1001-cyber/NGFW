# F-images independent R7 review

Reviewed source: `bf9fa4b25a62c4009229ab23c9d0a4480e507953`, 2026-10-05.
Reviewer owns only this report; no product edits or host changes.

## Findings

- MINOR — `docs/status/tasks/F-images-wip.md:7`: opening recovery state still lists the builder, formats, tests and PR as remaining; later chronological sections supersede it. Add a current-state summary at the top during integration so recovery does not require reconstructing history.
- MINOR — `docs/status/tasks/F-hardening-lite-review-20261005.md:1`: unrelated historical hardening BLOCK review is bundled with the image branch. Preserve it on the manager/reviewer archive rather than the final image scope, or clearly identify it as superseded historical evidence.

## Evidence inspected

Commands actually run, read-only:
```sh
git -C /root/ngfw-wt/ready-images-20261005 diff --name-only origin/main...bf9fa4b2
cat docs/status/tasks/F-images.md docs/status/tasks/F-images-wip.md
cat deploy/image/vm/README.md deploy/image/cloud/README.md docs/install/images.md
cat docs/status/tasks/F-images-questions.md docs/status/tasks/F-images-ready-envelope.md
tail -25 /tmp/F-images-quick-final.log
cat /root/ngfw-wt/review-r1-seven-20261005/docs/status/tasks/F-images-test-T1.md
```
Actual observed output:
```text
Developer log: mode quick · wall time 16m18s
CI GATE PASSED
Independent T1 exact bf9fa4b2: mode quick · wall time 27m57s
CI GATE PASSED
Image wrapper: ok ngfw/test/topology/images 2.511s
Independent final git status: empty (recorded by T1).
```
This reviewer inspected existing execution evidence rather than claiming to execute the full gate independently. T1 provides exact-source gate provenance. The developer status truthfully calls its own earlier run working-tree evidence because the path-preflight fix landed during that gate; it does not mislabel it an exact starting-SHA gate.

The canonical report describes built behavior, commands/output, explicit unfinished appliance build/boot/import, scope and open release questions. Install documentation matches explicit management NIC, target-only bootloader, shared P14 source, marker bridge, format conversions, unsigned integrity manifests and safe no-pci policy. Missing signed pool/producer manifest and disk headroom are concrete execution prerequisites, not claimed appliance acceptance. VM/cloud execution is explicitly deferred. No new security-boundary decision is silently approved; task-authorized image pipeline and reuse remain within scope. Board changes are manager-owned and absent from this branch. Envelope active override supplies real branch/worktree/base/slot, superseding its historical template placeholders.

No BLOCKER or MAJOR findings. Final latest-main integration gate and hosted quick remain manager requirements; this approval does not assert them complete.

Verdict: APPROVE.

## Corrected integration review, 2026-10-05

Reviewed6bd58ecbe10efb45a486139777655c6c826093cd / tree8d9829d754b58b40842ebff74a30641acda1dee9. Read current wip/canonical and actual independent T1 report. Prior minor cleanup resolved: current summary supplied and unrelated hardening review omitted. R4 GRUB host/alias-root blocker and common mutation_root repair are explicitly recorded, with old-source approvals retained only as provenance. Independent exact corrected-tree full quick PASS22m44s,13 image cases wrapper1.813s,149 fake-host assertions; source/HEAD/tree clean recorded by T1. This reviewer inspected evidence rather than reran gate. Actual appliance build/boot/cloud limits remain explicit. Refresh top pending-review language with latest verification when integrated; remaining R4/R8 affected verification/hosted/current-main prerequisites are manager-owned.

Verdict: APPROVE for corrected image documentation.
