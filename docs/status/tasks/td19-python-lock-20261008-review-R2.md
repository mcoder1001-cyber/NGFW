# TD19 security review R2 — initial round

Reviewed source: `5122f3e899d3976b6d40c502cb8683eec0868c73`. Source tracked diff clean at inspection; only Python bytecode cache directories untracked. Independent reviewer did not modify product code.

## Findings

1. **MAJOR — unbounded decompression before resolver.** `scripts/lab-python-lock.py:62–83,133`. Only METADATA uncompressed length is bounded. Pip eagerly reads every `.dist-info` member into memory, including RECORD and additional metadata. A wheel within the compressed-size limit can declare enormous uncompressed metadata and exhaust memory; the timeout does not bound allocations. Independent safe reproduction: 4,612 compressed bytes containing a 4,194,304-byte RECORD passed `inspect_wheel`; full generation returned 0 with a candidate. Fix: reject excessive per-member and aggregate uncompressed sizes before pip (including every metadata member), add a compressed-bomb refusal regression. Avoid executing a genuinely enormous bomb for validation.

2. **MAJOR — output pathname replacement follows symlink and overwrites existing files.** `scripts/lab-python-lock.py:166–171`. Creating a 0700 output directory does not prevent an actor with write permission on its parent from renaming that directory and replacing its pathname with a symlink before writes. Deterministic independent reproduction hooked only the scheduling gap after mkdir: replacing output by a symlink to another fixture directory caused its pre-existing requirements.lock to be overwritten; helper returned success. Fix: hold a verified directory descriptor, create children exclusively with no-follow semantics relative to that descriptor, and make cleanup identity-safe; alternatively enforce/document a trusted output-parent boundary clearly before accepting mutable paths. Add a replacement-race regression.

## Other security checks

The resolver uses argv only, trusted interpreter isolated mode, sanitized subprocess environment, disabled pip configuration/cache/version lookup, local find-links, no index, wheels only and dry-run. Direct pins reject options, URLs and markers. Every wheel METADATA is screened for direct dependency URLs before resolution. Wheel content is inspected as data rather than imported or installed; upstream authenticity remains explicitly unverified. Input regular-file no-follow checks and private snapshots are good boundaries. No new API, auth, socket, installed dependency, daemon operation or production release pins. Target acceptance remains unclaimed. Focused secret-pattern scan of the changed source/test/task docs found no matches; this is not a full gitleaks scan.

## Independent verification

Command: `python3 -I scripts/tests/lab-python-lock.py`

```text
.........
Ran 9 tests in 2.843s
OK
```

Safe extra-metadata reproduction:

```text
preflight accepted: compressed bytes=4612 metadata RECORD bytes=4194304
full helper returncode=0 candidate_exists=True
```

Output scheduling-gap reproduction:

```text
race overwritten existing unrelated file=True
```

All reproduction artifacts were synthetic temporary fixtures and removed. Findings notified to developer and manager; final source recheck required.

## Final independent recheck

Rechecked local source commit `720f55ed8c214aca67ad1d77210c9871652a129e`, tree `5c946d851afcd6875dceae67b389fd92f16f03c4`; tracked source clean. Developer reports published corresponding source commit `7eb0f2a205ae7458d71baca4c0c08c6dadea383b`; this reviewer inspected the local tree and does not independently assert remote publication. Helper SHA256: `5acdbe962d25130dfd685292ae5f914a7745e220c97d77412f41d3ecb4fcabf6`; tests SHA256: `86c115b627565b85c359e20a674c1aa379a2e3c6f80f70c4351a7bb374a2eb81`.

Both MAJOR findings resolved. All members now have uncompressed per-member bounds, with aggregate per-wheel and wheelhouse bounds before pip. Independent original 4MiB RECORD reproduction refuses with `wheel member decompression bound exceeded`. Output parent/directory descriptors anchor exclusive no-follow 0600 child creation and cleanup; independent original post-mkdir symlink replacement refuses and preserves the unrelated existing sentinel. The regression suite covers both pre-open and post-open output replacement, compressed metadata and aggregate bombs, malformed/duplicate/off-snapshot/digest-corrupt artifacts in both resolver reports, and active/inactive runtime markers.

New workflow uses fixed offline fixture execution, contents read only, SHA-pinned checkout with persist-credentials false; it neither installs dependencies nor exposes new privileged execution or secret inputs. Resolver artifacts and identities are now equally validated in both dry-run reports. Authenticity and target installation remain honestly unverified.

Command rerun on frozen source: `python3 -I scripts/tests/lab-python-lock.py`

```text
..............
Ran 14 tests in 7.998s
OK
```

Verdict: **APPROVE** (0 outstanding BLOCKER, 0 MAJOR, 0 MINOR). This source/security approval does not replace the unchanged hosted quick gate or target installation/provenance acceptance. Later product-source edits require recheck.
