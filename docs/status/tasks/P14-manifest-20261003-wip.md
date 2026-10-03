# P14 fail-closed VPP manifest completeness

Branch `codex/p14-manifest-20261003`, worktree `developers/P14-manifest`,
base `23308a43`; coordinator owns publication and scheduled tests.
Owned: ISO builder, new verifier/helper regression, aggregate test entry and this doc.

Production now calls the tested verifier, consuming the producer's exact v2 schema
and `deploy/vpp/VERSION` data without sourcing shell code. All producer package
identities must exist exactly once, each ship boolean must match the authoritative
ship subset, and all versions match the selected repository VPP version.
The runtime subset requires main `vpp`. Repository archives are inspected with
read-only `dpkg-deb -f`; all shipped packages must exist exactly once with matching
filename, Debian control identity, architecture and hash. Non-shipped VPP packages,
duplicate filenames and traversal/glob filenames are refused. Resolved pool paths
must remain inside the pool. Ordinary non-VPP dependencies are allowed.

Tests create small real Debian archives in a temporary directory, never install
them, and cover valid producer shape, empty/incomplete/missing-main/duplicate set,
flag/version/hash/architecture/path mismatch, duplicate filenames and missing,
duplicate or escaping repository artifacts. Aggregate suite adds one check.

Actual worker validation: source inspection and `git diff --check` only; no test
launch, network, chroot, ISO or host operations. Coordinator next commands:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 deploy/image/iso/tests/test_vpp_manifest.py
bash deploy/image/iso/tests/run.sh
```

Independent review and scheduled tests remain pending. Existing repository signature
verification stays intact. This closes manifest completeness/parity, not full-build
determinism or resistance to a writer who can alter both unsigned manifest and repo.
