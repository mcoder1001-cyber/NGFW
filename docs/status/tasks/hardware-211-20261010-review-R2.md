# R2 security review — API native-library dependency metadata

Reviewer: `/root/host_211`, independently assigned by manager; no product code authored or edited by reviewer. Review workspace/branch: `/root/ngfw-wt/hardware-211-20261010`, `codex/hardware-211-20261010`.

Scope: manager candidate `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c`, tree `5532c3a6a869942b74aafa58580c960df2f37b07`, against product base `d2d55984d74fa1d06c32e8271886f11f16375407`. Read `prompts/REVIEW-PROMPT.md` and `prompts/reviewers/R2-security.md` plus the previously read shared architecture, contributing and decision rules.

## Findings

No BLOCKER, MAJOR, MINOR or NIT security findings in the reviewed change.

The sole product change, `deploy/debian/ngfw/debian/control:18`, inserts `${shlibs:Depends}` in the `ngfw-api` dependency field. Existing debhelper rules already run the standard dependency generation pipeline; the change makes package control consume generated native-library requirements rather than discard them. The existing Node.js `>=22` and `<23` bounds remain byte-for-byte unchanged. This change does not introduce a new vendored library, download path, code execution path, API route, shell command, privilege, socket permission, user-controlled substitution, secret-storage policy or authentication/session behavior. Debian's generated dependency substitution is packaging metadata, not shell evaluation of user input.

All application source, contract packages, security configuration, unit files, maintainer scripts, startup/assets, VPP code and the complete hosted quick-gate implementation remain unchanged. Changed task documentation records real package/recovery limitations and does not contain credentials. The scope is enforcing native linkage already present in the prepared package; native-payload provenance and rebuilt archive inspection remain the applicable packaging review's responsibility.

## Independently executed evidence

All commands ran in the review workspace above. No target SSH, package installation, service action, reboot or filesystem repair occurred during this review.

1. `git show --stat --oneline 2045ab8`:

   ```text
   2045ab8b3 fix(packaging): declare native API library dependencies
   deploy/debian/ngfw/debian/control                       |  2 +-
   docs/status/tasks/hardware-manager-20261010-envelope.md |  3 ++-
   docs/status/tasks/hardware-manager-20261010-wip.md      | 16 ++++++++++++----
   3 files changed, 15 insertions(+), 6 deletions(-)
   ```

2. Exact-diff Python assertions, reading both control files through `git show`, required that the only product path is `deploy/debian/ngfw/debian/control`, that the previous API dependency line occurs once, and that replacement by the line containing `${shlibs:Depends}` produces the entire reviewed new control file:

   ```text
   PASS: only product change inserts shlibs:Depends into ngfw-api; existing Node.js bounds preserved
   PASS: source/application/auth/unit/CI/security-policy directories unchanged (git diff --exit-code checked)
   Review source tree: 5532c3a6a869942b74aafa58580c960df2f37b07
   ```

3. `git diff --exit-code d2d55984d74fa1d06c32e8271886f11f16375407 2045ab8 -- apps packages tools/ci.sh .github/workflows .github/gitleaks.toml deploy/systemd deploy/debian/ngfw/assets deploy/vpp`: exit 0, no output. Read `debian/rules` and `ngfw-api.postinst` at reviewed source; standard `dh` processing and the unprivileged `ngfw` user setup are unchanged.

4. `git diff --check d2d55984d74fa1d06c32e8271886f11f16375407 2045ab8`: exit 0, no output.

5. Targeted history-only secret scan, avoiding `node_modules` and broad workspace scans:

   ```text
   gitleaks git --redact --log-opts='d2d55984d74fa1d06c32e8271886f11f16375407..2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c' --config=.github/gitleaks.toml .
   3 commits scanned.
   scanned ~7743 bytes (7.74 KB) in 514ms
   no leaks found
   ```

## Applicability and limits

Verdict applies to source `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c` and final D112 commit `bde83bc87ae817a61bbc70e4029f76109ae77c35`, tree `aeb17e4312cb12714a9c39ca5e7c56b85eda7a54`, on `codex/hardware-manager-20261010`. Reviewer fetched the final published branch and independently verified applicability:

```text
git diff --name-only 2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c bde83bc87ae817a61bbc70e4029f76109ae77c35
docs/status/tasks/hardware-manager-20261010-review-plan.md
docs/status/tasks/hardware-manager-20261010-wip.md

PASS: entire final product tree matches reviewed source; only two manager task documents differ
Reviewed/final control blob: dbf2c0d12f880da0c5b4ffde19711c7f1d4fca9e
Final tree: aeb17e4312cb12714a9c39ca5e7c56b85eda7a54
```

`git diff --exit-code 2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c bde83bc87ae817a61bbc70e4029f76109ae77c35 -- deploy apps packages tools .github` exited 0 with no output. The exact changed-path assertion above proves no product file elsewhere changed either. Final branch has exactly one commit above the reviewed main base. `git diff --check` against that base exited 0. Reviewer read both changed manager task documents; no new security boundary or secrets appear. Targeted final history scan:

```text
gitleaks git --redact --log-opts='d2d55984d74fa1d06c32e8271886f11f16375407..bde83bc87ae817a61bbc70e4029f76109ae77c35' --config=.github/gitleaks.toml .
1 commits scanned.
scanned ~8156 bytes (8.16 KB) in 586ms
no leaks found
```

This receipt does not claim a completed quick gate, artifact installability, appliance installation or hardware acceptance; those tests were not performed here. The preexisting console/offline filesystem recovery block on both targets remains in force. Any later product-head change requires renewed applicability comparison.

**Verdict: APPROVE** for final commit `bde83bc87ae817a61bbc70e4029f76109ae77c35` in the reviewed narrow security scope; no security findings.
