# Packaging storage correction — independent R1/R2 verification

Exact reviewed correction `cb16b03fdc528f0bbd667ff1003caee50d18b18a`, tree `6aba53b036bcc0397e91b298f10164ed032cfacd`. Own reviewer branch `codex/packaging-security-verify-20261002`, worktree `/workspace/scratch/de92de7d9874/NGFW-packaging-verify`. No product edits or publication. Original BLOCK report `P10-packaging-resume-review-R1-R2.md` is preserved unchanged.

## R2 verification

Original root storage symlink-traversal BLOCKER is **resolved** in this bounded correction. Postinst no longer runs pathname-based `install -d` on vrx-writable storage. Its fixed-path Python helper opens every absolute-path component with O_DIRECTORY|O_NOFOLLOW, relative to the already-open parent descriptor. mkdir races are followed by no-follow open; a preexisting or swapped-in symlink fails. Permissions and ownership are changed only through fchown/fchmod on the pinned directory descriptor. A swap after open cannot redirect those calls to the symlink target. Existing ancestors are traversed without changing their ownership/mode; only intended leaves receive permissions. No recursive operation deletes or modifies operator content.

Helper is installed into root-owned package code location, invoked through absolute /usr/bin/python3; vrx-api declares direct python3 dependency. No external package library, new service privilege or broad filesystem permission was added. Failure propagates through postinst set -eu. A service-controlled concurrent rename may prevent successful intended provisioning, but cannot redirect privileged metadata changes outside the pinned inode.

Five symlink locations (API root, data root and three children) and deterministic swaps before/after child open are exercised by the new regression. Additional reviewer-created temporary fixture puts a symlink in an intermediate ancestor; helper rejects it without modifying target metadata/content or creating children. No host-sensitive target or actual privileged install used.

R2 corrected bounded verdict: **APPROVE**. This supersedes only the original symlink BLOCKER after the exact correction; it does not retroactively approve the unsafe old head.

## R1 verification

Ordinary repeated provisioning still preserves existing backup bytes, file mode and UID/GID. Directory creation/modes, direct dependency and helper inclusion remain tested. Before-open race fails visibly; after-open race affects the original directory descriptor rather than external target. API postinst shell syntax remains valid. Existing appliance-only VPP ID scope and separate pending daemon-ownership/identity-writable-parent decisions remain unchanged.

R1 corrected bounded verdict: **APPROVE**. This is host-independent packaging behavior; real package lifecycle/boot/service operability is not inferred from it.

## Actual independent checks

```text
python3 deploy/debian/vrx/tests/test_packaging.py
............
Ran 12 tests in 0.522s
OK
sh -n deploy/debian/vrx/debian/vrx-api.postinst
git diff --check cb16b03f^ cb16b03f
```
All exited 0. Additional isolated Python import-and-provision fixture printed:
```text
Additional ancestor-symlink refusal: PASS; target metadata/content/children unchanged
```

`git diff --exit-code cb16b03f 2356919c -- deploy/debian/vrx/assets/provision-api-storage.py deploy/debian/vrx/debian/vrx-api.postinst deploy/debian/vrx/debian/vrx-api.install deploy/debian/vrx/debian/control deploy/debian/vrx/tests/test_packaging.py deploy/systemd/vrx-agent.service` exited 0. Therefore this correction approval carries to those exact files on developer's rebased integration checkpoint `2356919c`; unrelated changes on its new main base are not re-reviewed here.

No full quick gate duplicated per assigned scope; hosted exact-head gate remains mandatory. No real root package install, daemon start, VPP operation, appliance boot, full dependency audit or privileged host write performed. P10 remains incomplete and release/privilege decisions retain their existing gates.

**Combined corrected bounded R1/R2 verdict: APPROVE.**
