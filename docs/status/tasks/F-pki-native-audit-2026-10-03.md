# PKI reuse and native certificate integration audit — 2026-10-03

Status: reviewed existing work read-only; **native certificate authentication remains unavailable**. No duplicate PKI implementation, shared VPP mutation, global-key setter or certificate material transfer was performed.

## Existing implementation to reuse

The active PKI work is coordinated in a separate chat/workspace. Its reviewed current checkpoint is `/root/.codex/worktrees/0b16/developers/PKI-current`, commit `2ac4ddc5`. It contains the issuance/import/export API, bounded DER/PKCS#12 processing, CRL/OCSP handling, additive contracts and public bilingual inventory. Reviewed repairs include `d0017c93` (DER/KDF/OCSP bounds) and the public inventory route/expiry fixes. It contains no agent `internal/pki` materializer; its own integration reports explicitly leave agent wiring unfinished. Importing those standalone files should be coordinated with that owner rather than reimplementing them here.

The historical `/root/ngfw-wt/F-pki` checkpoint contains a tested agent materializer (`ce47ebd8`) and projection/runtime/RPC sources. Its runtime is enabled only when `ipsecMode` selects strongSwan, writes to the swanctl directory and receives no product source (`pkiSource` returns nil). Its desired builder selects only strongSwan certificate tunnels. Copying this wiring unchanged would keep native certificate delivery disabled. The safe file writer and validation tests can be reused after adapting consumer selection, root, source and current contracts; the old runtime and renderer hook should not be installed as a route-based fallback.

The current native PSK implementation and separate sealed-secret delivery remain intact. Changes in this audit turn were limited to accurate certificate refusal/documentation and the separate CLI acceptance task.

## Why file delivery alone does not enable native certificate authentication

The pinned VPP source is authoritative:

- `src/plugins/ikev2/ikev2.c:4534`: `ikev2_profile_set_auth` with RSA signature loads the certificate file into `p->auth.key`.
- `src/plugins/ikev2/ikev2.c:2075`: the incoming peer signature is verified directly with that configured public key. The configured file therefore represents the **peer's pinned certificate key**; it is not a CA trust store.
- The local signing private key is a plugin-wide singleton, exposed as `ikev2.local-key/global`. Existing descriptor documentation and registration enforce globals-owner access; unrelated profiles cannot safely select different local keys.

The current product authentication contract names a **local** `auth.certificate` and a trusted `auth.remoteCa`. It supplies neither an explicit peer-leaf pin nor native CA-chain verification semantics. Pointing VPP at the local certificate or a CA certificate would verify the peer signature against the wrong public key. Exporting a CA signing key to solve that mismatch would violate the intended API-side-only CA-key boundary.

A native implementation needs an explicit agreed trust model: either a separate public peer-certificate pin and its validation/expiry/revocation behavior, or actual certificate-chain support in the plugin. It also needs owned local-key provisioning, a defined singleton-key restriction across profiles, dependency ordering and restore behavior. A PKI inventory/file tree cannot establish those capabilities.

## Current behavior and checks

Native certificate requests fail at the authentication leaf before resolving any material or projecting a profile. The failure now names peer-certificate trust mapping and plugin-global key provisioning, rather than suggesting PKI file delivery alone is sufficient. A focused regression asserts no secret resolution and no native profile projection for a certificate request.

Focused validation passed: `tools/heavy.sh go -C apps/agent test ./internal/desired -run 'TestNativeCertificateRefused|TestIKEv2NativeRefusal' -count=1`. The regression confirms certificate requests resolve no material and project no native profile. Evidence: [certificate-refusal.log](S-cli-ipsec-2026-10-03-evidence/certificate-refusal.log). The CLI task has independent passing race/build evidence.

## Coordinated next step

Reuse the active PKI API/contracts/inventory checkpoint once its owner publishes a reviewed integration result. Reuse the historical materializer core with the current secret-channel resolver and a native consumer root, avoiding a strongSwan runtime dependency. Before enabling native certificate profiles, resolve the peer-certificate/CA-trust contract and global local-key restriction explicitly, then prove certificate negotiation, mismatch refusal, expiry behavior, restart and rollback in disposable VPP. No shared deployment is authorized by this audit.
