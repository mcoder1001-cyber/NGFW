# TD19 offline lab Python closure candidate generation

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
