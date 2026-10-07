# BUG-vpn-capability-warning independent R2 security review

Source `dfa993405`, base `d6e1646cd`. Own isolated branch/worktree: `codex/bug-vpn-security-review-20261005`, `/root/ngfw-wt/bug-vpn-security-review-20261005`. Reviewer owns this report, reviewer recovery/envelope, and retained independent probe only. Product source was not edited.

No R2 BLOCKER/MAJOR. The product diff removes only obsolete blanket `agent.unsupported-field` warnings for `/vpn/ipsec` and `/vpn/pki` from WireGuard's builder. It retains the unimplemented `/vpn/remoteAccess` warning and does not remove a validation error, secret check, licence check, material resolver, descriptor, auth guard, or privilege rule. IPsec and PKI already have separately registered builders in projection.go. Unsupported or invalid native transforms, certificate capability gaps, secret unavailability, PKI HSM/ACME gaps and missing PKI material remain checked by those builders.

Independent blob equality checks base versus reviewed source:

```text
UNCHANGED apps/agent/internal/desired/pki.go ac5aaaad2f41e241f321d028ba4b005efa1d22f3
UNCHANGED apps/agent/internal/desired/ikev2.go f7bf7b59ef4f7da3bd27f360f103641322b70929
UNCHANGED apps/agent/internal/agent/projection.go 2592e0089fbe3b29e90a314024d4d363cc0238d6
UNCHANGED apps/api/src/commit/validation.service.ts 0f9bbcea9b26c059dfcd0fa75166e5aaf8373e4f
```

The API's licence stage still throws on licence errors before agent validation. No dependency, public API/auth/session model, file/socket permission, secret representation, or host command changed. The underlying warning fix does not grant licences or establish production WireGuard secret delivery. Native REST packet acceptance belongs to TEST-traffic-B.

Actual independent secret scan:

```text
gitleaks dir --config .github/gitleaks.toml --redact .
scanned ~73234545 bytes (73.23 MB) in 9.3s
no leaks found
```

Scoped actual validation tests and PKI overlay probe output follow after completion. Full mandatory quick is assigned to the separate correctness tester and is not duplicated or claimed here. No shared host services or live packet allocation was used.

```text
tools/heavy.sh env GOMAXPROCS=2 GOFLAGS=-p=2 go -C apps/agent test -count=1 -run '^(TestWireguardOtherVpnCapabilities|TestWireguardBuilderFindings|TestIKEv2NativeRefusalBeforeMutation|TestNativeCertificateRefusedBeforeResolvingMaterial|TestIKEv2NativeExclusiveOwnershipBeforeProjection)$' -v ./internal/desired
--- PASS: TestIKEv2NativeExclusiveOwnershipBeforeProjection (0.02s): separate-ipip-false/true both PASS
--- PASS: TestIKEv2NativeRefusalBeforeMutation (0.00s): secret-unavailable,policy,transport,wrong-underlay,unsupported-underlay-vrf,ipv6-underlay,custom-dpd,custom-ike-lifetime,many-selectors all PASS
--- PASS: TestNativeCertificateRefusedBeforeResolvingMaterial (0.00s)
--- PASS: TestWireguardBuilderFindings (0.00s)
--- PASS: TestWireguardOtherVpnCapabilities (0.00s)
PASS
ok ngfw/agent/internal/desired 0.143s
```

Independent actual PKI guard overlay probe (product source untouched):

```text
tools/heavy.sh env GOMAXPROCS=2 GOFLAGS=-p=2 go -C apps/agent test -overlay=/root/ngfw-wt/bug-vpn-security-review-20261005/.scratch/r2-pki-overlay.json -count=1 -run '^TestR2PKIGuardsPreserved$' -v ./internal/desired
=== RUN TestR2PKIGuardsPreserved
--- PASS: TestR2PKIGuardsPreserved (0.02s)
PASS
ok ngfw/agent/internal/desired 0.132s
```

The overlay maps the virtual desired package `r2_pki_probe_test.go` to the retained `BUG-vpn-capability-warning-security-probe_test.go`. It verifies HSM rejection, missing certificate/private-key material errors, and no material object projection. No source file was added to the product package. All owned commands finished; no live fixtures remain.

Verdict: **APPROVE**, R2 only, zero BLOCKER/MAJOR/MINOR for source dfa993405. Mandatory complete quick and native packet acceptance are not claimed by this review.
