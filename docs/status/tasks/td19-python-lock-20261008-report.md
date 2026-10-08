# TD19 offline lab Python closure candidate generation

> Release follow-up: [TD19 release closure](td19-release-20261008-wip.md) now
> supplies exact production pins, official PyPI artifact provenance, a complete
> 22-distribution hash lock and reproducible offline wheel preparation. The
> original helper report below is historical; its missing-input statements are
> superseded by that follow-up. Ubuntu 26.04 native lab acceptance remains open.


The lab installer requires a reviewed complete hash lock but no authoritative release lock is available. The optional builder scripts/lab-python-lock.py makes a candidate reviewable from explicitly pinned direct package inputs and an independently reviewed local wheelhouse. It installs nothing and uses no index. Existing installer defaults remain fail-closed.

Input direct file must contain exactly one name==version entry for pip, robotframework, robotframework-sshlibrary, scapy, pytest and requests. No URLs, directives, markers, floating or duplicate pins. Wheelhouse must contain only bounded regular wheels; symlinks, unsafe ZIP members and direct URL Requires-Dist declarations refuse before pip. A private immutable snapshot feeds trusted current-interpreter pip in isolated mode, configuration disabled, cache disabled, no-index, wheel-only, ignore-installed dry-run. Resolved archive digests must match the snapshot. A second dry-run requires every generated hash and verifies the same full closure.

Run in the intended resolver environment, after reviewing exact direct versions and the upstream authenticity of wheel artifacts:

```bash
python3 -I scripts/lab-python-lock.py \
  --direct /path/to/reviewed-direct.txt \
  --wheelhouse /path/to/reviewed-wheels \
  --output /path/to/new-candidate \
  --target-python 3.14 \
  --target-platform linux-x86_64 \
  --target-os ubuntu:26.04
```

Target flags assert the actual interpreter/environment rather than simulating another runtime. The Python version shown is an explicit intended target input, not a claim that target acceptance occurred. requirements.lock includes complete resolver-selected transitive pins and actual wheel SHA256 values. provenance.json records selected filenames/digests, input/output digests and resolver environment. This receipt proves consistency and offline closure only; it does not authenticate upstream publishers or validate installation/boot. Only independently reviewed release artifacts may become installer inputs.

Synthetic wheels in scripts/tests/lab-python-lock.py are deliberately version 1.0 and never production pins. Nine focused tests passed in 2.634s with zero skips, including actual installer dry-run lock acceptance. Compile/diff/source checks passed. Full hosted quick gate and independent review pending. Reviewed upstream versions/artifacts plus Ubuntu 26.04 target generation/install acceptance remain required; TD19 is not complete.


## Independent review corrections

Initial reviews correctly blocked verification-report artifact bypass, metadata-only decompression bounds and output pathname replacement. Both reports now validate unique canonical package identities, exact snapshot wheel paths and digests, and compare full artifact tuples. Every archive member is bounded (metadata members 1 MiB, other members 16 MiB), with 64 MiB expanded per-wheel and 512 MiB expanded wheelhouse limits before any pip resolution. Output children use directory-fd anchored exclusive/no-follow opens; cleanup never follows a replaced output pathname.

Fourteen focused tests now pass8.114s, zero skips, including corrupted first/second reports, duplicate/malformed reports, actual active/inactive dependency markers, compressed RECORD/aggregate bomb refusal before pip, and deterministic symlink swaps before/after directory open. The separate read-only hosted lab-python-lock-fixtures workflow runs this suite on relevant changes. Existing quick gate remains unchanged. Re-review and hosted checks pending; initial production/target acceptance limits above still apply.


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
