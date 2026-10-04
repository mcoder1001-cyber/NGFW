# Native certificate independent safety review — in progress

- Reviewer branch: `codex/native-cert-safety-review-20261004`.
- Base: `1af871f0b`; implementation: `codex/native-cert-pin-20261004`, worktree `/root/.codex/worktrees/8c19/developers/native-cert`.
- Owned files: this review report only. No product writes or lab/VPP changes.
- Owner authorized bounded peer leaf public-key pinning and one shared local identity. Final frozen-SHA verdict is pending.

## Initial design observations sent to developer and manager

1. Pinned VPP source `ikev2.c` selects RSA profiles using the configured leaf public key to verify AUTH. It does not compare certificate DER or perform CA-chain validation. Documentation must distinguish leaf-public-key pinning plus configured IKE identity from exact presented-certificate pinning.
2. Current VPP native implementation does not present the local certificate; the local imported certificate can validate key pairing and configured identity agent-side. Interoperability with a daemon peer may require explicit peer public-certificate configuration. Runtime interoperability remains unverified until lab acceptance.
3. Existing `Profile.Update` returns on a setter failure without restoring earlier setters, and scheduler journals only successful updates. New certificate trust changes must use journaled recreation or bounded compensation; failed compensation must report uncertain/degraded state.
4. Peer pin/local key rotation must retire previously authenticated SAs. Changing only the future handshake pin does not revoke already active SAs.
5. Immutable owner-private snapshots must survive rollback and confirmed revert despite ordinary PKI file rotation/removal. Snapshot paths must lie under the writable agent state directory; private content never enters persisted desired metadata or errors.
6. VPP local private key is global, has no getter or unset, and frees its previous value before loading a replacement. Failed setters need old-key compensation. First-key failure/removal residual state must be reported honestly; no claim that loaded key is erased.

## Verification

Read-only inspection of descriptor/scheduler/PKI and pinned VPP source only. No product tests run yet; no PASS verdict. Await developer frozen SHA, inspect final diff and focused evidence, then commit final review report. Next command: `git -C /root/.codex/worktrees/8c19/developers/native-cert log -3 --oneline` after developer handoff.

## Final source verdict — APPROVE

Frozen integration reviewed: `9e927a97e40bb0e619649ade69d84173f8a64a9e`. Its Go product descriptor/projection/subsystem files match independently inspected `4211f098e4ad599d3e227560a61a6ce834e36538`; contracts/API/UI match reviewed product `5e351f43a` and test follow-up `d42db5447`. No unresolved source findings. This verdict is source/design approval; manager must complete final integration checks before merge.

Resolved observations: explicit peer public-key pinning without CA/DER/presentation claims; private immutable generations under the agent state directory; foreign RSA ownership checked before writes and at application; shared local certificate/key/IKE identity enforced; RSA profile changes use journaled recreation; key-load failures revalidate and compensate the old immutable generation or report uncertain; session-retirement errors report uncertain; key generation changes force all RSA profiles to recreate. Historical private files and loaded-key no-unset residual state are documented, not claimed erased.

Public-only certificate imports reject CA certificates, chains, private material and CA options before writing; peer import does not require or deliver a peer private key. Only PKI inventory key references become optional; WireGuard stays mandatory. Native local and remote-access certificate consumers enforce local key requirements; management TLS retains its explicit key checks. Earlier in-progress wrong-field edit was fixed before freeze.

Independent commands and actual output:

- Contracts frozen worktree: `pnpm exec vitest run src/domains/vpn.test.ts src/semantic/vpn.test.ts` — 2 files, **124/124 PASS**, 8.47 seconds.
- Go frozen `4211f098e`: `go test -race -count=1 ./internal/descriptors/ikev2 -run 'TestNative|TestProfileRSASig'` — **PASS**, 1.225 seconds. Includes immutable snapshot permissions/tampering/symlink refusal, foreign ownership before writes, load compensation/uncertainty, mandatory dependencies/recreation, and retirement-failure uncertainty.
- Go frozen `4211f098e`: `go test -race -count=1 ./internal/desired -run 'TestNativeCertificate'` — **PASS**, 2.213 seconds. Includes certificate projection and unsupported/invalid material and identity refusal.
- `git diff --check 1af871f0b 4211f098e` — **PASS**.

Root-owned production disposable test reviewed in frozen integration: real sealed-secret/projection/scheduler/profile lifecycle, replay, key rotation, explicit revision revert, and autonomous restart after deleting only the fixture's profile and private snapshots before another Apply. It correctly states peer negotiation and packets are not exercised. Final exact-tree execution belongs to manager evidence, not this reviewer result.

Remaining acceptance limits: peer RSA negotiation/packet interoperability; rotation of actually active RSA SAs; configured certificate expiry is checked at projection, not by VPP on-wire certificate validation. Retired sessions cannot be restored by configuration rollback and require a fresh explicit initiation. Full/hosted CI waiver remains separate from the focused PASS evidence above. Reviewer made no product or host changes.
