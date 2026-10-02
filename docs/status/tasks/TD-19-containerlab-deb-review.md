# TD-19 pinned containerlab Debian package review

**R1/R2/R5: APPROVE bounded pinned `.deb` source checkpoint.** Reviewed exact `e3e1daaab2d01fa7bded794f94223a9e36e988af` on 2026-10-02 in an isolated worktree. This delivers the requested Debian-package format, superseding the earlier tar format limitation; the earlier tar security review remains historical evidence. Overall TD-19 still has unfinished repository/key/pinning and integration acceptance work.

## Source/provenance verification

Independently fetched `https://api.github.com/repos/srl-labs/containerlab/releases/tags/v0.79.0`. Exact official asset `containerlab_0.79.0_linux_amd64.deb` reports `sha256:a399d92a622b4664d8d1231bc9b7f53a1d210255a0306fa091c3f63779f65f13`; the source digest, fixed version and official download URL match. This is upstream digest pinning, not independently signed provenance certification.

The script refuses unsupported Linux/architecture and unknown arguments, requires root only for installation, and keeps `--check-config` nonmutating. A fresh private mktemp directory holds the exact downloaded package. Actual SHA256 must match before even `dpkg-deb` inspection, and certainly before APT. Package/Version/Architecture are read as data and must equal containerlab/0.79.0/amd64. APT receives the quoted private absolute package filename, not a floating package-name install; an exact installed-version readback must succeed before the subsequent Python environment setup. EXIT cleanup covers success and rejection. Existing preflight dependencies are required before APT. No installer shell from upstream is executed by curl.

No new MAJOR finding in this scope. Installing a genuine upstream Debian package would execute its maintainer lifecycle through APT; that lifecycle was not executed in this environment. Later Python dependencies and unrelated repository keys remain separate unfinished TD-19 scope.

## Actual independent evidence

Executed all five `TD-19-test-*.py` scripts directly with checked subprocess return codes:

| Suite | Result | Runtime |
|---|---|---|
| Build preflight | 5 PASS | 0.028 s |
| Containerlab Debian package | 6 PASS | 0.183 s |
| Artifact preflight | 6 PASS | 4.229 s |
| Provision ordering | 3 PASS | 0.040 s |
| Transfer verifier | 3 PASS | 0.251 s |

**Combined: 23 actual tests PASS, zero skips.** The new suite builds genuine temporary Debian archives, executes the production marked package block with fake curl/APT/version-query, and checks hash refusal before inspection, each wrong metadata field before APT, exact install path/readback, APT failure, installed-version mismatch and cleanup. Test digests are deliberately recomputed for fixture packages; no real upstream package is downloaded or installed by the fixtures.

An initial generic unittest discovery attempt found zero tests because these filenames contain hyphens; that run is not counted as evidence. The explicit script invocations above provide the actual 23-test result. Future combined gates must execute these files explicitly or use importable filenames and enforce nonzero test discovery.

`bash -n scripts/00-add-repos.sh scripts/20-install-build.sh scripts/40-install-lab.sh tools/lab`: PASS. `git diff --check`: PASS. ShellCheck unavailable locally / NOT RUN. No actual host package install, maintainer script, service, remote SSH, release publication or lab acceptance was executed. Fresh full hosted integration checks and deferred target tests remain required; no full TD-19 DONE claim follows from this checkpoint approval.
