# TD-19 artifact preflight and remote-transfer review, round 2

Reviewed exact source `cb6fdee8b92429da9682db146f727757af015d0a` in an isolated read-only product worktree on 2026-10-02. This supersedes the initial filesystem-input BLOCK in `TD-19-preflight-review.md`; the original finding remains historical evidence.

**R1/R2/R5: APPROVE this bounded source checkpoint.** No additional MAJOR finding. This is not full TD-19 completion, a hosted CI result, a real remote provisioning result, or cryptographic artifact-origin certification.

## Source verification

- `lstat` refuses symlink/nonregular manifest, checksum metadata and manifest package inputs before the unchanged original install verifier reads them. Basename validation prevents traversal. Manifest size is bounded. The original `--require-files` and `--install-gate` validator, original VERSION parser, exact seven shipping package set, patched version and amd64 constraints remain mandatory; no shipping manifest or original validator was weakened.
- Remote provisioning obtains the local checked selection before its first remote command. Exactly seven validated source names enter quoted transfer arrays and a fresh root-created private staging directory. Selection travels as JSON data on stdin, rather than shell interpolation.
- The remote verifier hashes actual staged bytes and reads actual Debian control fields before APT. An existing regular root-owned policy without group/world write must return 101 for VPP start/restart/try-restart/reload/force-reload. Missing, unsafe or permitting policy fails before APT; this change does not install or replace the shared policy.
- Successful APT is followed by exact installed-version checks for every selected package. A mismatch aborts before subsequent sysctl/startup/driver configuration. Existing explicit remote-apply, target-role and self-target guards remain. Later deliberate systemctl activation is outside the package-postinst no-start guard and remains visible in the provisioning plan.
- Manifest hashes establish consistency against the checked builder manifest; they do not establish an independently authenticated origin. Non-VPP third-party repository keys and remaining provisioning/pinning work remain separate unfinished scope.

## Independently executed evidence

- `python3 docs/status/tasks/TD-19-test-preflight.py`: **6 tests PASS**, 4.328 s. Includes foreign symlink rejection before the original verifier, unsafe selection, missing metadata and verifier refusal before host commands. The original missing-manifest assertion now checks its actual stdout; it is not skipped.
- `python3 docs/status/tasks/TD-19-test-transfer.py`: **3 tests PASS**, 0.279 s. Extracts the shipped remote Python body, builds a genuine temporary Debian fixture and tests hash/control/symlink/policy rejection before stub APT, successful install/version checks, and version mismatch refusal after stub APT.
- `bash -n scripts/00-add-repos.sh tools/lab`: PASS. `git diff --check`: PASS. Product worktree was clean before writing this report.

The transfer fixture uses one genuine temporary package to exercise the remote verifier, redirects policy to a temporary file and models its owner identity for the invoking UID. The exact-seven guarantee was independently inspected in the local production selection/transfer code; these tests are not an end-to-end seven-package remote deployment. No SSH provisioning, actual APT, host policy writes, daemon starts, NIC changes, key/network publication or lab acceptance was executed. ShellCheck is unavailable locally and therefore NOT RUN. Full hosted gate and deferred target acceptance remain required evidence; TD-19/P10 must not be marked DONE from this approval.
