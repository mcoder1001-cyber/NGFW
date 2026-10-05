# R1 — correctness and tests

Source: `dfa9934053047738ecb6f649f9d38455c3a66c5a`; tree `53ad32352e21300e6e1a7a018c113e3e83427a34`; base `d6e1646cdfe57168dcb0e4ebf66c87026bc58863`. Independent worktree `/root/ngfw-wt/vpn-warning-r1-t1-20261005`, branch `codex/vpn-warning-r1-t1-20261005`. Date 2026-10-05. No product edits, host service mutation or packet claims.

No BLOCKER, MAJOR or MINOR findings. The delta removes only obsolete blanket IPsec/PKI warnings; remote-access warning remains. Production `subsystems.go` registers PKI and native IKEv2 independently. Their own validation, secret/material guards and unsupported leaf errors remain unchanged. The new nonempty IPsec/PKI regression independently checks both warnings absent and remote access present. Before-fix failure is recorded in `/root/ngfw-wt/logs/two-ready-vpn-warning-before.log` (developer evidence); I independently reproduced after-fix passing behavior.

Additional review-only Go overlay tested intrinsic ACME warning and HSM refusal, native unsupported/refusal cases and certificate pre-resolution refusal. Initial overlay fixture wrongly supplied nonexistent ACME `enabled` field and failed schema decoding; corrected review fixture uses `{}`. This was reviewer fixture error, with unchanged product. Actual final output:

```text
=== RUN   TestIKEv2NativeRefusalBeforeMutation
=== RUN   TestIKEv2NativeRefusalBeforeMutation/secret-unavailable
=== RUN   TestIKEv2NativeRefusalBeforeMutation/policy
=== RUN   TestIKEv2NativeRefusalBeforeMutation/transport
=== RUN   TestIKEv2NativeRefusalBeforeMutation/wrong-underlay
=== RUN   TestIKEv2NativeRefusalBeforeMutation/unsupported-underlay-vrf
=== RUN   TestIKEv2NativeRefusalBeforeMutation/ipv6-underlay
=== RUN   TestIKEv2NativeRefusalBeforeMutation/custom-dpd
=== RUN   TestIKEv2NativeRefusalBeforeMutation/custom-ike-lifetime
=== RUN   TestIKEv2NativeRefusalBeforeMutation/many-selectors
--- PASS: TestIKEv2NativeRefusalBeforeMutation (0.02s)
    --- PASS: TestIKEv2NativeRefusalBeforeMutation/secret-unavailable (0.01s)
    --- PASS: TestIKEv2NativeRefusalBeforeMutation/policy (0.00s)
    --- PASS: TestIKEv2NativeRefusalBeforeMutation/transport (0.00s)
    --- PASS: TestIKEv2NativeRefusalBeforeMutation/wrong-underlay (0.00s)
    --- PASS: TestIKEv2NativeRefusalBeforeMutation/unsupported-underlay-vrf (0.00s)
    --- PASS: TestIKEv2NativeRefusalBeforeMutation/ipv6-underlay (0.00s)
    --- PASS: TestIKEv2NativeRefusalBeforeMutation/custom-dpd (0.00s)
    --- PASS: TestIKEv2NativeRefusalBeforeMutation/custom-ike-lifetime (0.00s)
    --- PASS: TestIKEv2NativeRefusalBeforeMutation/many-selectors (0.00s)
=== RUN   TestNativeCertificateRefusedBeforeResolvingMaterial
--- PASS: TestNativeCertificateRefusedBeforeResolvingMaterial (0.00s)
=== RUN   TestReviewPKIIntrinsicGuards
    review_pki_test.go:8: W agent.unsupported-field /vpn/pki/certificates/site/acme
        E agent.unsupported-field /vpn/pki/hsm
--- PASS: TestReviewPKIIntrinsicGuards (0.00s)
=== RUN   TestWireguardOtherVpnCapabilities
--- PASS: TestWireguardOtherVpnCapabilities (0.00s)
PASS
ok  	ngfw/agent/internal/desired	0.143s
```

Complete unchanged quick gate also PASS; see independent T1 report. Packet acceptance belongs to separate TEST-traffic-B; these unit checks do not claim forwarding acceptance.

Verdict: APPROVE.
