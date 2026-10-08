# R8 operability and packaging review — TD19 offline lock

Reviewed local HEAD `2ae4c6744df43ea14a5f77ac7aacab8deb7b1277`; product source is unchanged from `720f55e`. Final SHA was rechecked after the fixture run.

Findings: no BLOCKER or MAJOR in the reviewed R8 scope. The optional builder does not install packages or change services. Its target assertions describe the executing runtime; hashes and receipt explicitly establish consistency only, leaving upstream authenticity and target installation unverified. Existing fail-closed installer remains unchanged. New workflow uses SHA-pinned checkout, read-only contents, disabled credential persistence, bounded timeout, and executes isolated offline fixtures on relevant changes. Mandatory complete quick gate is unchanged. Partial output failure cleanup is anchored to owned directory descriptors; existing output is refused.

Actual commands executed:

```text
python3 -I scripts/tests/lab-python-lock.py -v
Ran 14 tests in 7.448s
OK

git diff --check
(no output; exit 0)
```

These are synthetic wheel tests, not authoritative production pins, upstream provenance, installation, boot, or target-runtime acceptance. Hosted execution of this new workflow and unchanged complete quick gate must be verified separately. No package installation, service operation or runtime mutation was executed.

Verdict: **APPROVE** for R8 source scope; external gates and release acceptance remain required.
