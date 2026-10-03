Fresh independent R8 packaging and operability review
Verdict: APPROVE (bounded P10 export/archive checksum slice only)

Product worktree: /root/Documents/Codex/2026-10-03/check-out-latest-code-from-git/work/NGFW-p10-resume
Exact reviewed/tested HEAD: 71cee90b2a28421480dfd67a6d64ca7f47f37200
Tree: 538878ff020cae5e4752bb81d1d1c6d8ed95cc87
Base: 19aa88a5
Published commit object: e53bc20bda899ebe5b12d12233e75086fb212edb
Published commit object's tree independently resolved locally to the same tree.

Read AGENTS.md, prompts/00-CONTEXT.md, docs/contributing.md,
docs/decisions/decision-policy.md, prompts/REVIEW-PROMPT.md,
prompts/reviewers/R8-operability-packaging.md, product envelope/WIP,
manager P10-parallel-review-plan.md and prior R2/T1/R7-cli reports.
Inspected all six changed files and surrounding exporter/verifier/test code.

Findings: none in R8 scope.

The full serialized tar is flushed and fsynced before measurement/hashing.
Hashing streams from the same exclusive O_NOFOLLOW mode0600 private descriptor;
no pathname reopen or whole-archive allocation is introduced. archive_bytes
measures complete saved tar bytes (headers/padding/nonshipping archives included),
with fstat size consistency checked. Existing bytes remains member payload sum.
Destination pinning, atomic hard-link no-replace publication, inventory bounds,
cleanup and existing permission/refusal behavior are retained. No dependency,
service change, installation behavior or host privilege boundary is introduced.
Failures remain operator-visible as nonzero CLI errors. Existing cleanup tests
exercise write failure and mutation refusal; full-disk write/fsync errors flow
through the same exception cleanup path. Crash-proof orphan recovery is not a
new claim made by this delta.

The rebuilt .deb fixture preserves control fields while changing valid payload
and digest. Separate truncated malformed .deb test requires InvalidBundle;
verification is not weakened to accept malformed dpkg archives. English/Persian
instructions require a separately trusted report and comparison before extraction,
and correctly distinguish transport integrity from publisher authentication.

Actual commands/results, run in product worktree:
- git rev-parse HEAD HEAD^{tree}: exact HEAD/tree above.
- git diff 19aa88a5 --stat: six files, 71 insertions, 5 deletions.
- git diff 19aa88a5 -- deploy/debian/bundle/export.py deploy/debian/bundle/test_export.py deploy/debian/bundle/test_verify.py docs/user/install/bundle.md: inspected.
- python3 --version: Python 3.14.4.
- git diff --check 19aa88a5: exit0, no output.
- python3 deploy/debian/bundle/test_verify.py:
  Ran 23 tests in 49.123s; OK.
  Includes valid changed payload and malformed archive refusal.
- python3 deploy/debian/bundle/test_export.py:
  Ran 10 tests in 63.289s; OK.
  Includes saved full-tar digest/size independently checked against file bytes,
  deterministic roundtrip, permissions, refusal, publication races and cleanup.
- python3 deploy/debian/bundle/test_install.py:
  Ran 11 tests in 56.794s; OK.
- git status --short: exit0, no output after tests.
- git rev-parse e53bc20bda899ebe5b12d12233e75086fb212edb^{tree}:
  538878ff020cae5e4752bb81d1d1c6d8ed95cc87.
Total independently run synthetic fixture cases:44 PASS, no skips reported.

I did not independently run/query hosted CI. Existing T1 report records unchanged
complete hosted quick37110162496 SUCCESS; that remains manager gate evidence.
No host package installation, restart, service enablement, privilege change or
product edit performed. Test apt/system boundaries are isolated/stubbed fixtures.

Whole P10 remains unfinished. Real complete artifacts, signing/provenance,
checkout-free delivery helpers, actual clean Ubuntu install/upgrade/remove and
hardware/lab acceptance are not certified by this narrow review. These deferred
areas are not falsely DONE and are not blockers newly introduced by this delta.
Final integration still requires applicable panel and exact integration-tree gate.
