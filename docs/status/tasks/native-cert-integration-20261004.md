# Native IPsec certificate connection — completed source integration

اتصال گواهی به native IKEv2 طبق گزینهٔ اول مورد تأیید مالک پیاده‌سازی شد.
گواهی عمومی طرف مقابل بدون کلید خصوصی قابل واردکردن است.
کلید، گواهی و هویت محلی مشترک پیش از اعمال بررسی می‌شوند.
بازیابی خودکار و چرخش/بازگشت تنظیمات با agent تولیدی و VPP موقت پاس شد.
مذاکره/ترافیک گواهی‌محور و استقرار این نسخه روی ۲۵۰ هنوز انجام نشده است.

**Overall: 84.7% by hours (1336.0/1577.5 h), 84.4% by tasks (178/211)**

## Result and boundary

D-234 owner approval is implemented. Additive `auth.peerCertificate` names an imported PKI peer leaf; `auth.certificate` names the local operational RSA certificate/key. Public-only PEM import is explicit (`publicOnly: true`) and rejects CA certificates, multiple certificates, private-key inputs and CA-trust options before writes. Other local certificate consumers still require a private key. The native certificate UI requires a peer public-key pin and explains the shared local identity; English/Persian strings are supplied.

Agent preflight checks RSA >=2048, validity, digital-signature usage, SAN/IKE identity, matching local certificate/key, shared local certificate/key/IKE identity, and global-key ownership before projecting a certificate profile. Unsupported CA-chain `remoteCa` remains rejected. Existing PSK functionality remains supported. VPP verifies AUTH using the configured peer leaf public key; it does not compare peer-presented DER, verify an on-wire CA chain or present the local leaf. Peer configuration must independently trust the local public key.

Sealed material stays outside desired metadata and responses. Immutable keyed-fingerprint snapshots are owner-private (0700 directory/0600 files), verify existing content, reject symlinks and retain prior generations for rollback. Required `pki.files -> local-key -> profile` dependencies order real application. Global-key load failures restore the previous verified generation or report uncertain/degraded outcome. RSA profile changes recreate profiles and retire sessions; key rotation refuses foreign RSA profiles and retires owned RSA SAs before changing the singleton. VPP has no unset/getter: removing the last RSA profile leaves an inert loaded key, and historical private generations are retained. Configuration rollback cannot restore negotiated SAs; peers require fresh initiation.

## Frozen source and independent verification

Product freeze: local `9e927a97e40bb0e619649ade69d84173f8a64a9e`, exact-tree remote `9024c5c1ff7bd2139a500913cbddd0bb0f3e36b6` on `codex/native-cert-frozen-9e927a97e`. Later integration changes are documentation/board/report only. Source archives: Go final remote `f26327e29e723c7f5a380d13b2110bc77989f9aa`; contract/API/UI `2cb0468ddf08fcae2d8d58c2b52cad210945b771`; review `5aeda564353a892ae92c7b329736ed939a989dfd`; independent tester `3d9185301312901d47bd67970c01be16e6519101`.

Independent source APPROVE: [safety report](native-cert-safety-review-20261004.md). Independent tester PASS on the exact frozen product: [test report](native-cert-independent-tests-20261004.md): schema124, API23, UI11; Go race desired34.405s/ikev2 descriptors1.211s/subsystems25.224s plus vet; consumer typechecks17/17, regenerated working tree clean, unchanged guard12s. Developer additionally passed secretchannel/PKI race and focused lint/build/generation. Complete hosted/full CI remains owner-waived, never recorded as PASS.

## Manager real production lifecycle proof

Copied native plugin builder passed, reading the pinned VPP reference and writing only `.scratch/native-cert-proof-plugin`. Built the production agent from frozen integrated source. Ran:

```sh
NGFW_INTEGRATION=1 \
NGFW_NATIVE_AGENT_BIN=/root/.codex/worktrees/8c19/NGFW/.scratch/native-cert-product-agent-final \
NGFW_ISOLATED_PLUGIN_PATH=/root/.codex/worktrees/8c19/NGFW/.scratch/native-cert-proof-plugin/plugin:/usr/lib/x86_64-linux-gnu/vpp_plugins \
tools/heavy.sh python3 test/topology/hardware-smoke/isolated-vpp.py \
  go -C apps/agent test ./internal/desired -run '^TestIKEv2NativeCertificateProduction$' -v -count=1 -timeout 2m
```

Actual output: `PASS: TestIKEv2NativeCertificateProduction (4.87s)`; Go package `ok .../desired 4.980s`. Exercises production gRPC Apply with separate secret bundle/sealed cache, PKI materialization, shared native key and RSA profile, live readback of peer reference, replay, local-key rotation, explicit revision revert, autonomous agent restart after deleting only the owned disposable profile and private snapshot files before another Apply, and final profile removal. Disposable VPP PID3655473 stopped; shared VPP PID1014 remained active. Earlier proof runs passed 3.99/3.95s; final proof above is the reviewed source.

## Acceptance still owed

No certificate peer negotiation/packets, active-RSA-SA rotation, certificate appliance/browser acceptance or deployment of this new source to250 is claimed. Existing installed250 NGFW version4c8 predates this change. No shared VPP restart, global key write, host service change or target package installation occurred. Source P11/F-ikev2-native complete; deferred lab acceptance remains in [DEFERRED-ACCEPTANCE](../DEFERRED-ACCEPTANCE.md).
