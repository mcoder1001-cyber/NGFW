# F-ab-upgrade — independent R2 security review

Reviewed source: `ac421a6e72912b621a5063df2bd59140dba2a54e`.

Review performed independently as R2 security; no product files edited. Diff reviewed against the merge base with origin/main. Focused tests used disposable fixtures with TMPDIR=/root/.cache/review-r2; no shared service, package, boot configuration or VPP changes. Full CI and appliance runtime acceptance belong to separate gates.

Secret scan: `gitleaks detect --no-git --source docs/status/tasks --config .github/gitleaks.toml --redact --no-banner` in this task worktree: exit 0, no leaks found. A second scan copied exactly changed existing files into disposable scratch and used the same configuration: exit 0, no leaks found. Neither scan establishes that a runtime release builder cannot export a key.

BLOCKER — deploy/upgrade/build-bundle:30 and :43. Signing private key may be published in the signed update it authenticates. The builder accepts --key beneath the offline root, checks its mode and key type, then walks that root while excluding only selected persistent paths and fixed credential filenames. A legitimate mode0600 root/root/release-signing.pem is included as a regular archive member. Anyone obtaining the published bundle can recover the release signing key and forge future updates.

Disposable reproduction: populate required appliance files using test_upgrade.root_tar, generate a temporary Ed25519 key at root/root/release-signing.pem, chmod0600, invoke build-bundle with that key and a new output, decompress rootfs.tar.zst, compare its root/release-signing.pem member to the original without printing bytes. Actual output:

```text
builder_exit 0
SIGNING_PRIVATE_KEY_INCLUDED True
```

Required fix: refuse a signing key resolving inside the source root before creating output; refuse included files sharing the signing key inode (hardlink alias with external key); exclude credential-bearing root/.ssh, root/.gnupg and administrator home trees or fail closed on them. Add regressions that output is not published and no private-key bytes are included. Sent to manager and source developer; awaiting targeted verification.

Otherwise reviewed snapshot-before-consume signature/hash verification, Ed25519 pinning, archive traversal/symlink/special-node/hardlink preflight, size budgets, shared-state parent ownership and mode, directory ownership restoration, mount identities, explicit GRUB paths, private upgrade state/lock and backup permissions, no shell execution, health checks and generic errors.

Command executed in /root/ngfw-wt/ready-ab-20261005: `TMPDIR=/root/.cache/review-r2 PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy/upgrade/tests -v`

```text
Ran 14 tests in 12.433s
OK
```
These existing tests do not cover the reproduced key export.

Initial verdict at ac421a6e: BLOCK (resolved below).

## Targeted verification of fix

Source eb9dbc46d4db684a6f6301dd4eb0302547c73504 inspected against ac421a6e. Builder now refuses canonical signing key paths inside offline root and scans regular source inode identities for external hardlink aliases before archive/output creation. /root and /home are excluded from the shipped archive. New regressions assert rejection before output for contained key and hardlink alias; round trip asserts SSH/GnuPG administrator credential trees absent while legitimate tool hardlinks still export. The confirmed BLOCKER is resolved.

Independently reran the same upgrade unittest discovery command in the developer worktree with private TMPDIR and PYTHONDONTWRITEBYTECODE=1:

```text
test_builder_refuses_signing_key_inside_root_and_inode_alias ... ok
test_builder_round_trip ... ok
Ran 15 tests in 27.191s
OK
```

Final verdict for eb9dbc46: APPROVE. No outstanding BLOCKER/MAJOR/MINOR findings. Full gate remains separate.
