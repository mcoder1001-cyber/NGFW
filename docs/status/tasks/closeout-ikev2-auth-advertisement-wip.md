# IKEv2 certificate authentication capability correction

Branch: `codex/closeout-ikev2-auth-advertisement`; base `d62c0d42`.
Publication: local only; parent owns remote publication.
Owned: patch 0004, product patch series, private native-plugin builder, this status.

Pinned native initiator advertises SIGNATURE_HASH_ALGORITHMS, but its authentication
verification accepts RSA_SIG (1), not DIGITAL_SIGNATURE (14), and uses SHA1.
Modern stock certificate peer fails authentication in retained certificate evidence.
The minimal patch removes only the unsupported advertisement. No new algorithm,
trust bypass, global service mutation, or modern certificate interoperability claim.

Validation pending: copied-source plugin build, exact patch independent review,
actual default stock certificate-peer protected packets. Legacy compatibility diagnostic
is independently queued on the certificate fixture branch. No P11 Done claim.
Next: `tools/heavy.sh python3 test/topology/ipsec/build-native-plugin.py --output .scratch/auth-capability-plugin`.

Frozen patch/build source: `2e2513d1d3d3c95b80a6ee6401d5f836513b5465`. Full deploy verification: 72 tests passed, zero failed. Independent exact-source review: `1761ee54a03a1743a2997a39b52d0548d45ce116` (traffic reviewer). Copied-source private plugin build PASS; SHA256 `2c2079209cc69db8ebe1e6682df365d9313ab5545728324c6996aba28924cda6`. No system installation/reference source edits. Default stock peer live proof remains pending.

Default stock peer actual run: fixture HEAD49820876; patch2e2513d1;
private plugin SHA2562c2079209cc69db8ebe1e6682df365d9313ab5545728324c6996aba28924cda6;
production agent source821ef578/binaryff9a241b322af565c53604929d330a8e72766ad6773a16c2784f7d3be5ac2f6e.
No legacy environment/config toggle. Both stock peer and native report successful RSA
certificate authentication, bidirectional ICMP and exact1MiB TCP passed. Whole proof
FAILED18.81s: production restart did not retain active IKE/CHILD SA. Logs show write-only
LocalKey resync recreates dependent profile. No fixture assertion weakened; original
modern AUTH_FAILED and corrected lifecycle FAIL preserved separately. Minimal notify
fix resolves negotiation only, full certificate acceptance/P11 remains NOT PASSED.
Shared VPP PID1014/NRestarts0 and startup SHA unchanged; disposable VPP stopped.
Next: manager decision/independent review for separate LocalKey resync dependency repair.
