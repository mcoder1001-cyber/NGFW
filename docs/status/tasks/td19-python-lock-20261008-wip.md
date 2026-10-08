# TD19 offline Python lock WIP

Branch: codex/td19-python-lock-20261008. Owned files: scripts/lab-python-lock.py, scripts/tests/lab-python-lock.py, .github/workflows/lab-python-lock-fixtures.yml (manager-authorized narrow extension), and td19-python-lock-20261008 task documents. Resolve containing commit with git rev-parse HEAD and published branch with GitHub branch lookup.

Implemented: optional offline wheel-only lock candidate builder. Six direct packages match the installer. Exact target environment assertions, bounded regular inputs, snapshotted wheel hashes, metadata URL/traversal refusal, isolated no-index dry-run resolution, and independent require-hashes closure validation precede output. Existing installer/default fail-closed behavior unchanged. No package installation or service changes performed. Hashes establish byte consistency only; output explicitly requires upstream provenance review.

Actual verification: python3 -I scripts/tests/lab-python-lock.py: 9 tests PASS, 2.634s, no skips. Includes transitive closure/digests and actual installer dry-run syntax interoperability, missing transitive package, incompatible wheel, runtime mismatch, metadata URL, traversal/symlink, malformed pins, pip configuration injection and existing-output preservation. python3 -m py_compile scripts/lab-python-lock.py scripts/tests/lab-python-lock.py PASS. git diff --check PASS. tools/ci.sh check --base main PASS, 3s; gitleaks unavailable, this is the bounded source check, not the full merge gate.

Remaining: upstream-reviewed exact direct versions and wheel provenance, Ubuntu 26.04 target generation and real installation acceptance. No production lock supplied and TD19 remains incomplete. Full unchanged hosted quick gate and independent review required. Next: publish source checkpoint, open draft PR and arrange review/hosted gate.


## Mandatory review repair checkpoint

Published initial checkpoint ec915fcced08cb5200bce8c7e6ad956f993a74ac, draft PR208. Independent R1/R2 reviews found real source gaps, not deferred acceptance: verification report ignored artifact URL/digest/duplicates; ZIP expansion only bounded METADATA; output pathname replacement could overwrite an unrelated file. Fixed by shared strict unique artifact tuple validation for both resolver reports and exact tuple equality; every member and aggregate expansion limits before resolver; directory-fd anchored exclusive no-follow output writes and identity-safe cleanup. Runtime active/inactive Python/platform marker closures now exercise actual pip. Both output swap timings and compressed metadata/aggregate bombs have regressions.

Actual revised verification: python3 -I scripts/tests/lab-python-lock.py: 14 tests PASS8.114s, no skips. Compile and diff checks PASS. New read-only lab-python-lock-fixtures workflow uses existing pinned checkout convention and runs the helper tests on relevant source changes; complete quick gate unchanged. Source re-review and hosted helper/full quick checks pending; exact revised published SHA available via PR head. Next: publish this repair checkpoint and request R1/R2 plus workflow review.


## Published source identity and actual command receipts

Product/workflow checkpoint published successfully on PR208:

- Remote source commit: `7eb0f2a205ae7458d71baca4c0c08c6dadea383b`.
- Local source commit: `720f55ed8c214aca67ad1d77210c9871652a129e`.
- Both trees: `5c946d851afcd6875dceae67b389fd92f16f03c4`. Connector create-tree returned this same local tree; expected-head branch update returned success. Distinct commit IDs reflect connector-created commit metadata, not a source difference.

Actual developer output from the frozen source checkpoint:

```text
$ python3 -I scripts/tests/lab-python-lock.py
..............
----------------------------------------------------------------------
Ran 14 tests in 8.114s

OK
[exit 0]

$ python3 -m py_compile scripts/lab-python-lock.py scripts/tests/lab-python-lock.py
[exit 0; no output]

$ git diff --check
[exit 0; no output]

$ tools/ci.sh check --base main
WARN gitleaks not installed — built-in secret grep only
check PASSED (0m02s)
[exit 0; final result shown, unrelated routine checks omitted]

$ git rev-parse HEAD HEAD^{tree}
720f55ed8c214aca67ad1d77210c9871652a129e
5c946d851afcd6875dceae67b389fd92f16f03c4

$ sha256sum scripts/lab-python-lock.py scripts/tests/lab-python-lock.py .github/workflows/lab-python-lock-fixtures.yml
5acdbe962d25130dfd685292ae5f914a7745e220c97d77412f41d3ecb4fcabf6  scripts/lab-python-lock.py
86c115b627565b85c359e20a674c1aa379a2e3c6f80f70c4351a7bb374a2eb81  scripts/tests/lab-python-lock.py
617ab1773c658d0ef8d60ff5c1a4bf0fa3610d96e587c83a4dc623c5be443d15  .github/workflows/lab-python-lock-fixtures.yml
```

R1/R2 source findings have been repaired; independent reviewers reproduced 14 passing tests on these exact source bytes and are finalizing reports. This documentation receipt does not change source/workflow bytes. Full unchanged hosted quick gate and new hosted helper check remain pending; no full gate success claimed. Next action: manager verifies published source identity, collects independent source/workflow approvals and hosted check outcomes before integration. Production upstream pins/provenance and actual target acceptance remain outstanding.


## Applicable review evidence and open inputs

[R1 correctness/test review](td19-python-lock-20261008-review-R1.md) and [R2 source/security review](td19-python-lock-20261008-review-R2.md) record APPROVE for the frozen helper/test source hashes above. R1 independently ran 14 tests in 7.484s; R2 final independent run passed 14 tests in 7.998s and reproduced refusal of its original archive/output attacks. Their publication/integration is managed separately from developer-owned source/docs. These reviews do not claim an independent remote publication check or replace required hosted checks. Workflow/operations and documentation reviews are separate applicable evidence.

No new architecture or security policy decision is requested: the generator remains an optional candidate tool and installer refusal is unchanged. Open release inputs are approved exact direct versions, independently reviewed upstream wheel authenticity/provenance and target environment acceptance. Runtime compatibility flags assert the generation environment only; hashes prove local byte consistency only; no production lock or real installation/boot result is supplied.
