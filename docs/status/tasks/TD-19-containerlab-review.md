# TD-19 pinned containerlab review

**R1/R2/R5: APPROVE bounded containerlab source checkpoint.** Independently reviewed frozen `f592f0c63f8f0f0cbedb98c969805e671eb1ee63` in an isolated worktree on 2026-10-02. This does not complete TD-19 or constitute live lab/tool installation acceptance. The TD-19 task explicitly requests a pinned `.deb`; this tar-binary checkpoint is a bounded security review only and does not satisfy that task delivery format. A matching pinned Debian-package continuation remains necessary.

## Provenance and implementation

Independently fetched the official GitHub release API `https://api.github.com/repos/srl-labs/containerlab/releases/tags/v0.79.0`. Its exact `containerlab_0.79.0_linux_amd64.tar.gz` asset reports `sha256:f90d36d58bb6c4afd3b3a4dca006b81594c6d16f7a04be0184b03f44291085a2`, matching the fixed source version, digest and official download URL. This is a pinned upstream digest, not a claim of an independently signed release.

`40-install-lab.sh` checks Linux amd64 and rejects unknown arguments; the nonmutating configuration check returns before root/APT checks. Download occurs into a fresh private mktemp directory and SHA256 verification precedes all APT. The extractor selects precisely one member named `containerlab`, requires a regular file and a positive declared size bounded at 256 MiB. It copies only that member's byte stream into a new private file; archive path extraction is never invoked. Other archive entries cannot create files, symlinks or traversal paths. Duplicate selected names, selected links and missing binary refuse before APT.

Installation uses a fresh target temporary file on the destination filesystem, copies bytes with explicit mode 0755 and atomically renames that file over the destination. Archive privilege bits, owner metadata and capabilities are not restored. APT failure leaves the existing binary intact. EXIT cleanup removes work/temporary target files. No curl-to-shell installer remains. This source change does not pin every later Python dependency or finish other TD-19 repository/key work.

## Actual independent checks

- `python3 docs/status/tasks/TD-19-test-containerlab.py`: **5 tests PASS**, 0.191 s. Production marked block executes with real temporary tar/hash/file operations and stub curl/APT. Cases cover successful replacement stripping archive 06755 to installed 0755, incorrect hash rejection before APT, symlink/missing/traversal-name/duplicate refusal, APT failure preserving the original, cleanup and fixed nonmutating configuration output.
- `bash -n scripts/40-install-lab.sh`: PASS.
- `git diff --check`: PASS.
- Official API asset name, version, digest and download URL comparison: PASS.

The fixture replaces the configured archive/digest with temporary test data to exercise both valid and invalid archive paths; it does not download or execute the real upstream binary. No actual APT, host binary installation, live services, remote lab mutation or full hosted gate was performed. ShellCheck remains unavailable locally / NOT RUN. Full hosted integration checks and remaining deferred lab acceptance retain their existing requirements. No new MAJOR finding in this scoped delta.
