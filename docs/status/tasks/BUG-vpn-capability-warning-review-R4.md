# BUG-vpn-capability-warning independent R4

Reviewed frozen `dfa9934053047738ecb6f649f9d38455c3a66c5a`, parent actual main d6. Own isolated branch `codex/bug-vpn-capability-r4-20261005`, worktree `/root/ngfw-wt/bug-vpn-capability-r4-20261005`. Reviewer owns this report, reviewer envelope and WIP only; no product edit.

## Findings

No R4 findings. The product diff removes only WireGuard's obsolete whole-root unsupported warnings for IPsec and PKI and retains its remoteAccess warning. Registration of `desired.PKI`, `desired.IKEv2` and `desired.Wireguard` in agent projection is unchanged. PKI intrinsic ACME/HSM refusal and certificate/secret preflight remain in their separate unchanged builder. Native IPsec's exclusive ownership, transform/proposal and certificate/material validation remain unchanged.

No VPP API/generated binding, materializer, reconcile/Retrieve implementation, owner prefix, object ID range, daemon invocation, systemd unit, globals operation, trace, lock, or privileged file is changed. Existing supported VPN configuration can now pass its own builders without a second builder incorrectly labelling the entire root unsupported. The change creates no new forwarding capability or object type, so new lab restart/packet acceptance is not required for this warning-only prerequisite. Native REST traffic acceptance remains the separate TEST-traffic-B task.

## Independent validation

Focused host-independent desired-builder tests run with GOMAXPROCS2/GOFLAGS-p2 under unchanged tools/heavy.sh. Exact completed results below. No integration opt-in was enabled, no shared VPP/system daemon/network/DB mutation performed. Source is frozen during checks.

Command: `GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C apps/agent test ./internal/desired -run 'Test(Wireguard|IPsec|Pki|PKI)' -count=1`

```text
ok ngfw/agent/internal/desired 0.124s
```

Command: `GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C apps/agent test ./internal/desired -run 'Test(Wireguard|NativeCertificate|IKEv2Native(ExclusiveOwnershipBeforeProjection|ProjectionAndLiveDrift|RefusalBeforeMutation))' -count=1 -v`

```text
--- PASS: TestNativeCertificateProjectionAndPreflight
--- PASS: TestNativeCertificateSharedIdentity
--- PASS: TestNativeCertificateMaterialBoundsAndValidity
--- PASS: TestIKEv2NativeExclusiveOwnershipBeforeProjection
--- PASS: TestIKEv2NativeProjectionAndLiveDrift
--- PASS: TestIKEv2NativeRefusalBeforeMutation
--- PASS: TestNativeCertificateRefusedBeforeResolvingMaterial
--- PASS: TestWireguardRouteNextHop
--- PASS: TestWireguardBuilderFindings
--- PASS: TestWireguardRouteLoopRefused
--- PASS: TestWireguardOtherVpnCapabilities
PASS
ok ngfw/agent/internal/desired 0.814s
```

All commands finished; no owned process remains. No live packet assertion inferred from these unit/preflight checks.

Verdict: **APPROVE** (0 BLOCKER/MAJOR/MINOR). R1 owns unchanged complete quick CI and final merge gates.
