# TD-19 source checkpoint

Base: `8f07ce68744ac3ec87d0c4ec87661c64b59c3884`.
Branch: `task/TD19-artifact-preflight-20261002`.
Source/test checkpoint: `d184471d5911552a6e7079506d13804590c1c430`.
Remote publication is managed separately; no remote SHA is asserted here.

This is a limited source-only dependency exception while P10 remains RUNNING.
It does not make TD-19 DONE or establish complete release readiness.

Implemented: non-mutating manifest selection requires the unchanged original
VPP verifier install gate, rejects symlink/nonregular manifest/checksum/deb
inputs, and selects exactly the original seven shipping runtimes. Floating
FDio installer execution was removed from script 00. Lab provisioning validates
locally before its first remote command, stages only the selected files in a
fresh directory, checks transferred digests and Debian control metadata, requires
an existing root-owned non-writable no-start policy before APT, and checks every
installed version before configuration changes. No automatic shared-lab policy
replacement was added. Checksums establish consistency with the trusted builder
manifest; they do not authenticate an independently supplied manifest.

Actual offline verification:

- Six preflight fixtures PASS (4.636s), including outside-root symlinks and
  original verifier missing-input refusal.
- Three transfer fixtures PASS (0.252s), including real temporary dpkg-deb
  artifacts, tampering/control/policy refusal and installed-version mismatch.
- Three actual provisioning-function ordering fixtures PASS (0.046s): missing
  confirmation, preflight failure and remote OS refusal cannot reach later
  remote staging/configuration commands.
- `bash -n scripts/00-add-repos.sh tools/lab` PASS.
- `tools/ci.sh check --base origin/main` EXIT 0, including gitleaks and guards.
- Shellcheck NOT RUN: binary unavailable. Full hosted gate not yet run.

No live host installation, APT, remote SSH, service, VPP, nft or network key
retrieval was executed. Fixture APT and remote calls are recorders. Actual lab
acceptance remains NOT RUN in the central acceptance campaign.

Remaining source gaps: scripts 20/40 retain old/floating Go/generator/containerlab
installation and script 00 retains unpinned FRR/NodeSource key retrieval.
Authoritative key fingerprints, Go archive SHA256 and containerlab artifact
version/checksum are absent from the reviewed source. They must be supplied by
the release owner before extending those installers; no values were invented.
Independent review of this bounded checkpoint and exact-head hosted checks are
required before merging it. Next command: rerun these three fixture scripts on
the review checkpoint, then address concrete review findings.

## Build bootstrap continuation

Script 20 now uses the existing CI Go 1.26.0/protoc-gen-go v1.36.12 /
protoc-gen-go-grpc v1.6.2 pins and the agent module's govpp v0.13.0.
No generator `@latest` remains. A caller-supplied trusted `VRX_GO_SHA256`
(exactly 64 hex characters) is mandatory before APT or any mutation, including
when a Go installation already exists. The downloaded unique temporary archive
must pass SHA256 before replacing `/usr/local/go`; existing Go matching checks
use the complete version/platform string. `--check-config` is non-mutating.
Three configuration fixtures PASS in 0.036s; Bash syntax and diff checks PASS.
No actual Go download, installation or generator execution was performed.
This explicit checksum requirement is not a claim that an official checksum
has been retrieved or authenticated. FRR/NodeSource/containerlab pin work and
other installer reproducibility gaps remain open. The preserved R2 approval
covers its original bounded preflight snapshot, not this newer build delta.

### Authoritative Go digest update

Manager verified the official https://go.dev/dl/ release listing on 2026-10-02:
Go 1.26.0 linux-amd64 SHA256 is
`aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235`.
Script 20 now pins that digest directly; an optional operator override must
match it exactly. This supersedes the earlier caller-supplied-hash approach
above. Missing override is safe because the authoritative pin is built in;
differing/empty/malformed overrides refuse before any mutation.
