# TD-19 bounded artifact preflight envelope

Branch task/TD19-artifact-preflight-20261002; isolated NGFW-TD19-provision.
Exact fresh main base8f07ce68744ac3ec87d0c4ec87661c64b59c3884 fetched successfully.
Owner: CI/build stream delegated developer. Owned scripts00/20/40, tools/lab
provisioning hunk, and TD-19 task reports/tests only; no original manifests,
P11, main/board/decision-log or frozen P10/fixture PR changes.

Actual board row has no standalone task prompt. Its authoritative scope is
review5.5b: download-to-file+sha256 before execution, pinned FRR fingerprint,
pinned containerlab .deb, Go version from go.mod and pinned sha256, pinned
protoc generator versions; review5.6: tools/lab manifest artifact provisioning,
original verify.sh --require-files and installed manifest version readback;
review5.5a: remove FD.io path in00-add-repos.

LIMITED manager-authorized dependency exception: P10 remains RUNNING for
security/ownership/license/appliance acceptance. Its implemented artifact/source
contract can be used for source-only preflight work; this is not full READY,
P10 dependency PASS, release artifacts availability or completed TD-19.
No approval rule blocks safe source/fixture work. Existing decision-policy
security/host mutations remain untouched: no real package install, service,
VPP/nft operation, key download or privileged lab change during this task run.

## First scope and choices

Original deploy/vpp/verify.sh --require-files ARTIFACTDIR --install-gate remains
mandatory and unchanged. Seven ship:true runtimes are selected from validated
manifest/v2, never a filename glob or floating upstream repository. Version is
26.06-release+ngfwN per original VERSION. Preflight can run without root and has
no host mutations; root install/repo paths must reject missing/invalid product
artifacts before APT/network. Consumer profile is Ubuntu26.04; Node22/PG18 are
already runtime contract inputs, not invented package availability claims.

Start with source preflight and FD.io removal; then manifest-based remote
shipping set/preinstall hashes/control fields/postinstall dpkg-query readback.
Foreign key fingerprints/checksums are not present in current tree; do not
invent their authoritative values. Pin-required refusal may be implemented,
but actual third-party download/provenance is outside this source run.

## Tests

All scripts/lab commands run only on redirected temporary fixture copies with
APT/curl/ssh/scp/systemctl blocked or recorded. Exercise missing/invalid artifacts,
original verifier invocation failure before any APT/network/remote mutation,
exact seven shipping packages excluding dev/debug, patched version constraints,
unsafe manifest paths, transfer integrity failure and postinstall mismatch.
Actual original verifier refusal on missing manifest also checked offline.
Syntax/shellcheck/unchanged check gate and independent review required; hosted
full quick on final integration required before merge. Live provisioning NOT RUN.
Estimated bounded work1–2h, not a guarantee. Current source unchanged.
