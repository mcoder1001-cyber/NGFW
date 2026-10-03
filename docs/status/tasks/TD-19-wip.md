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
No generator `@latest` remains. A caller-supplied trusted `NGFW_GO_SHA256`
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

### Build PATH review correction

The original build-pin BLOCK is preserved. Installation now checks the explicit
`/usr/local/go/bin/go`, writes a persistent profile that prepends its directory,
and prepends it in the current process, clears Bash command lookup cache, then
requires the selected complete Go version/platform before generator execution.
The supported platform check now requires Linux as well as x86_64.
Five build tests PASS (0.028s), including executing the actual selection and
three generator commands against a fake new Go with an older Go earlier in the
original PATH. A wrong selected version invokes no generators. These are fake
executables in temporary directories; no actual installation occurred.

### Historical containerlab archive checkpoint (superseded by Debian scope)

Script 40 replaces the floating curl-to-shell installer with fixed release
v0.79.0 linux-amd64. The manager verified the primary official release API on
2026-10-02: https://api.github.com/repos/srl-labs/containerlab/releases/latest
(the source records its stable tag endpoint). Asset URL:
https://github.com/srl-labs/containerlab/releases/download/v0.79.0/containerlab_0.79.0_linux_amd64.tar.gz
SHA256: `f90d36d58bb6c4afd3b3a4dca006b81594c6d16f7a04be0184b03f44291085a2`.

The bootstrap requires existing curl/Python/checksum/install tooling, a Linux
amd64 host, and a private temporary directory. It verifies the archive before
APT and extracts only one bounded regular member named `containerlab`, rejecting
links, duplicates and missing entries. Installation uses a sibling temporary
file, forces 0755 (no archive SUID/SGID bits), then atomic rename. Exit cleanup
removes temporary staging. No group, capability or global privilege addition.

Five actual archive-block fixtures PASS (0.215s): correct replacement/mode,
wrong digest, archive links/missing/duplicates, APT failure preserving the old
binary, and non-mutating fixed configuration. Real local tar files and file
operations were used; curl/APT were temporary stub commands. No official
archive was downloaded or executed and no system target was changed. Bash
syntax and diff checks PASS. Independent review of the new script40 delta and
hosted exact-head gate are still required. FRR/NodeSource key fingerprints remain
unresolved; unpinned Python/pnpm package installation is an additional remaining
reproducibility limitation, so no whole TD-19 DONE claim is made.

### Combined checkpoint verification

On source `f592f0c6`, all five independent fixture commands passed:
preflight 6 (4.558s), transfer 3 (0.249s), provisioning order 3 (0.043s),
build preflight/PATH 5 (0.031s), containerlab 5 (0.194s): **22 PASS, zero
skipped**. `bash -n` for scripts 00/20/40 and tools/lab passed.
Unchanged `tools/ci.sh check --base origin/main` EXIT 0 (contract guard,
forbidden patterns, gitleaks ~58.89KB, packet-trace ban and resource slots).
Full quick and hosted final-head checks were not executed by this worker.
These checks do not establish runtime installation/appliance acceptance.

### Current containerlab Debian delivery scope

The board requires a pinned `.deb`, so the earlier tar/binary checkpoint and
its independent security review do not satisfy delivery acceptance. Script40
now downloads only the official v0.79.0 linux-amd64 Debian asset:
https://github.com/srl-labs/containerlab/releases/download/v0.79.0/containerlab_0.79.0_linux_amd64.deb
Manager verified the official release API asset digest on 2026-10-02:
`a399d92a622b4664d8d1231bc9b7f53a1d210255a0306fa091c3f63779f65f13`.

Private-file digest verification precedes every dpkg-deb inspection and APT
call. Package, version and architecture must be exactly containerlab/0.79.0/
amd64. APT receives only the fixed selected path; dpkg-query must confirm the
installed version before the existing Python setup continues. Staging cleanup
runs on success/failure. No extracted-binary install or added permissions,
capabilities or groups remain in the production source. Debian maintainer
script/dependency effects still require actual installation acceptance; none
was executed by this worker. Temporary fixture debs test the gate, not official
package behavior. The official archive binary is not claimed locally inspected.

Current Debian gate fixtures: six tests PASS (0.190s), using actual local
`dpkg-deb --build` artifacts with fake curl/APT/dpkg-query. Wrong hashes invoke
neither package inspection nor APT; mismatched identity fields refuse before
APT; APT error and installed-version mismatch stop continuation and clean up.
The first run exposed a fixture recorder argument-index typo; its field logging
was corrected without changing production checks, then all six passed.
Bash syntax/diff checks PASS. This newer `.deb` source requires fresh review;
the preserved tar checkpoint approval is historical scope only.

### Explicit strict fixture entry point

Hyphenated test filenames are not discovered by ordinary unittest discovery.
Use `python3 docs/status/tasks/TD-19-run-fixtures.py`, which imports a fixed
explicit list of all five suites with safe module aliases and refuses any
zero-test module, loading failure or empty total. Its result also rejects errors,
failures, skips, expected failures and unexpected successes. Bytecode writes
are disabled to keep the source checkout clean.

Actual combined runner: **23 tests PASS in 4.562s**, failures/errors/skips/
expected failures/unexpected successes all zero, EXIT 0. Separate real unittest
status fixtures exercised pass/failure/error/skip/expected-failure/unexpected-
success/zero results: only pass was accepted. No product suite was repeated
for those guard checks. Bash syntax/diff checks PASS. The `.deb` approval report
is preserved; this new runner requires its own independent recheck.

### Operator-pinned public repository keys (bounded source)

Official FRR and NodeSource primary fingerprints are still unresolved. Script00
now refuses repository setup before artifact verification, APT or network when
`NGFW_FRR_KEY_FINGERPRINTS` or `NGFW_NODESOURCE_KEY_FINGERPRINTS` is absent or
malformed. Each is a trusted administrator supplied exact set of 1–8 distinct
uppercase full (40/64 hex) primary fingerprints, comma separated. Values must
come from independently verified release provenance; downloaded keys cannot
supply their own expected identities. No default fingerprints were invented.

Bootstrap curl/GPG/Python must already exist. Downloads go to a private directory
with a 1MiB limit. The actual file gate rejects symlink/nonregular/empty/oversize
inputs. Fixed GPG commands use a private home and `--no-options`; packet checks
reject secret key material, colon readback requires the exact expected primary
set (no extra trust anchors), and refuses missing/duplicate/malformed/revoked/
expired primary identities. Both repository keys must validate before global
APT/keyring mutation. Dearmoring writes private staging; global public keyrings
are copied at0644 to sibling temporary files then renamed. No global policy,
service or capability change was added.

Seven controlled-GPG parser/gate tests plus the original five suites passed
through the explicit runner: **30 tests PASS in 6.639s**, all strict failure/
skip/expected-failure counts zero. GPG is a fixed-argv fake supplying packet and
colon fixtures; these tests do not prove actual release-key behavior. The first
combined run found the old verifier-failure fixture missing the newly required
pin configuration; explicit valid fixture pins now preserve its original
verifier17/no-host-command assertion. No production checks were weakened.
Real public-key retrieval, cryptographic release identity validation and host
installation remain NOT RUN. Security review of this new source is required;
previous runner/source approvals cover their frozen historical checkpoints.

Additional source53f69939 checks: unchanged `tools/ci.sh check --base origin/main`
EXIT0 (including gitleaks ~92.04KB). Actual installed GPG executed the production
helper against the locally installed Ubuntu archive PUBLIC binary keyring,
using a fresh private temporary GPG home/output: EXIT0, output created. Expected
fingerprints were derived only from that fixture to test binary-input mechanics;
this is not an independent trust decision, official FRR/NodeSource identity
verification, or network retrieval. No global keyring was written. This extra
OS-dependent check is recorded separately from the portable30-fixture suite.

### Security review validity correction

The initial security BLOCK is preserved. The primary GnuPG `doc/DETAILS`
(https://github.com/gpg/gnupg/blob/master/doc/DETAILS, checked 2026-10-02)
describes invalid/disabled/revoked/expired/not-valid statuses. The gate now
allows only ordinary unknown/undefined or valid trust states `-`, `o`, `q`,
`m`, `f`, `u`; every other value, including invalid `i`, disabled `d`, revoked
`r`, expired `e`, not-valid `n`, empty and unknown statuses, fails closed.
Supported trust-state parsing does not substitute for independently trusted
expected fingerprint configuration. Structured primary records require numeric
key size/algorithm/creation, full 16-hex key ID, numeric optional expiry and
supported signing capabilities; disabled `D` and non-signing/malformed records
fail before dearmoring. Both downloads now have10s connection/60s total limits.
Public keyring replacement is atomic per file, not a transaction across both
keyrings and source-list files; interrupted setup must be reconciled before
installation acceptance.

Nine key gate tests PASS4.089s, including explicit invalid/disabled/unknown and
malformed record/capability negatives. Combined strict runner: **32 PASS9.120s,
zero errors/failures/skips/expected failures/unexpected successes**. Actual GPG
binary public-fixture check was repeated with the stricter parser: EXIT0,
private output created, no network/global keyring writes. Syntax/diff PASS.
Fresh security recheck remains required; no full task/published-pin acceptance
is asserted.
