# TD19 offline Python lock R1 correctness and tests

Initial reviewed commit: `5122f3e899d3976b6d40c502cb8683eec0868c73`. Verification source hashes: helper `5acdbe962d25130dfd685292ae5f914a7745e220c97d77412f41d3ecb4fcabf6`; tests `86c115b627565b85c359e20a674c1aa379a2e3c6f80f70c4351a7bb374a2eb81`. Frozen local commit: `720f55ed8c214aca67ad1d77210c9871652a129e`; published matching source commit: `7eb0f2a205ae7458d71baca4c0c08c6dadea383b` (developer publication receipt). Final source hashes independently rechecked unchanged.

## Findings and resolution

Resolved MAJOR — initial `scripts/lab-python-lock.py:156-159`: second report compared only package names/versions, allowing changed URLs, hashes or duplicate entries without explicit rejection. Independent reproduction altered only the second actual resolver report to an outside URL and zero digest; the original helper still emitted output. Both reports now undergo identical bounded snapshot artifact/unique identity validation, and complete version/wheel/digest tuples must match before publication. Negative tests corrupt each report at both stages.

Resolved MAJOR — initial marker coverage: target argument mismatches did not demonstrate conditional transitive resolution. A real pip fixture now activates a dependency with matching Python/platform markers, omits unavailable inactive dependencies, and rejects an absent active dependency.

No remaining correctness finding in these verified source bytes. Direct packages remain exact and limited to installer inputs. The real offline pip resolver supplies the dependency closure; actual require-hashes resolution supplies a second verification. Candidate output records its unverified release status and does not claim target installation or upstream provenance.

## Independent validation

`python3 -I scripts/tests/lab-python-lock.py`:

```text
..............
----------------------------------------------------------------------
Ran 14 tests in 7.484s

OK
```

`git diff --check`: PASS (exit 0, no output). Full unchanged quick gate has not passed in this review; hosted required gate remains a merge prerequisite. Upstream-approved release selections/provenance and actual target installation acceptance remain separate unfinished TD19 work.

Verdict: APPROVE the scoped helper correctness/tests at the recorded source hashes; final frozen commit source recheck passed; required hosted gate remains pending before merge.
