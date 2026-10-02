# Independent TD19 repository artifact preflight review

Frozen `0fb76c7fa015a88616fb2e9efda3a2cdae5d8bf6`. Scope initial artifact selection before repository mutations, R1/R2/R5; other provisioning/key/20/40 steps remain unfinished. Reviewer changed only this report.

**MAJOR filesystem-boundary gap:** selected artifact names are validated as basenames, but actual symlink/regular-file identity is not checked. Existing original verifier uses os.path.isfile, open/hash and dpkg-deb, all following symlinks; a matching product .deb outside artifact directory can therefore pass through a selected symlink. The new consumer does not refuse this foreign-path case requested by its scope. Require non-symlink regular manifest/SHA256SUMS/selected runtime files before selection and verify containment; add genuine private symlink-outside rejection tests. Keep the original byte/control/hash/install validator mandatory, not replaced by these checks.

Independent consumer control-flow fixture (original verifier stub) with first selected file linked to an outside temporary file returned exit 0 and no host command. This demonstrates missing extra consumer refusal, not valid artifact provenance of that fixture. Source inspection establishes original verifier follows symlinks; real complete forged release was not constructed.

R1/R2 BLOCK until filesystem guard verified; R5 no additional scale finding in bounded seven-package selection. Positives: original --require-files --install-gate runs before parsing/host commands; unsuffixed/demo/dirty builds remain original validator failures; VERSION parsed as data, exact original seven ship names, product version, architecture/digest and basename constrained; JSON output is sorted and argv/path usage quoted. Floating FD.io installer/curl-to-shell removed without upstream VPP fallback.

Actual independent checks:

```text
python3 docs/status/tasks/TD-19-test-preflight.py
Ran 5 tests in 4.181s
OK
bash -n scripts/00-add-repos.sh
exit 0
git diff --check
exit 0
```

Four fixtures test consumer control flow using verifier stub; the fifth actually invokes unchanged original verifier against missing manifest and requires failure. Successful fixture does not create/verify real .deb binaries and should not be read as a full release-validation PASS. The CLI check branch precedes root gate, supporting nonroot use; fixture itself does not switch process UID. Repo/apt/curl/ssh/systemctl commands are stubbed or not reached; no host mutation/signing/network publication executed. Existing original validator read-only APT simulations are not installation.

Manifest hashes/control fields are corruption/build-consistency checks on a trusted builder, not cryptographic proof of build origin or malicious-writer resistance. No arbitrary input must be called authenticated merely because metadata matches. Full provisioning, host quick, shellcheck and actual artifact/repository acceptance remain pending; TD19 and dependency P10 are not complete.
