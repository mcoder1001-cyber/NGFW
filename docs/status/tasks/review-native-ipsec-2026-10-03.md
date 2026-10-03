# Reviewer 1 — native route-based IPsec (2026-10-03)

Scope: current working source for native projection/descriptors, protected-interface ownership, native state/actions, API-delivered PSK snapshots, restart/rollback selection, CLI controls and VPP patch 0002. Read-only review; no shared VPP mutation, product edits or task-state changes. This reviewer contributed to the earlier native implementation; this is a fresh technical audit, not a claim of separation from its original author.

## Findings

**P2 — Agent projection lacks exclusive protected-interface and fixed-peer validation — resolved.** `apps/agent/internal/desired/ikev2.go:29–35` independently projects every enabled tunnel; it never records already-used `routeBased.ipipInterface` or `(underlayVrf, localAddr, remoteAddr)`. API semantic validation rejects these conflicts in `packages/schema/src/semantic/vpn.ts:145–171` and `:222–240`, but the production Unix gRPC Apply/DryRun contract also accepts DesiredState directly. Two enabled native profiles can therefore claim the same IPIP or fixed peer through that path. Those profiles share plugin-managed tunnel protection/admin state: negotiation, deletion or DPD of one can interfere with the other. Mirror the uniqueness rules in the agent builder, rejecting conflicting enabled tunnels before dataplane execution. Add a direct-agent regression for duplicate bindings and duplicate fixed-peer tuples that verifies validation refusal and no VPP writes. Ordinary API requests already have this guard; this finding concerns the direct-agent contract.

No additional actionable blocker found in the reviewed secret-channel, state/action, CLI or patch-0002 scope.

## Evidence and limits

Fresh focused race checks passed in all four packages: `internal/secretchannel` (1.433s), `internal/desired` (1.514s), `internal/subsystems` (1.431s) and `internal/agent` (3.378s), selecting `IKEv2|Native|Secret|Sealed|Cache|Bundle|Retain|DryRun`. [Command evidence](review-native-ipsec-2026-10-03-evidence/focused-race.txt). These tests do not establish the missing duplicate-conflict refusal.

Inspected existing real disposable evidence: default 30s×3 peer-loss/automatic recovery PASS108.93s; bidirectional ICMP, exact 1MiB TCP and ESP-only underlay; active-SA Apply and actual agent restart preserving SPIs; partial Apply before negotiation/after deletion remaining closed; measured owned-SA counters; API PSK commit/rotation/restart/version-pinned rollback with no material echo. Reviewed full CLI race/build acceptance and REST-only paging/action tests. These hardware tests were not rerun for this read-only review.

The sealed store binds authenticated ciphertext to the owner and retains immutable current/confirmed snapshots for rollback; product responses/scheduler values carry references rather than plaintext. Runtime actions verify owned profiles/SAs, local-role rekey constraints and precise SPI widths. Generic admin writes are guarded against live protected IPIP ownership, and runtime-owned admin state is excluded from reconciliation. Patch 0002 guards unselected/freed profile indices in V2/V3 SA dumps and converts action SPIs from network order; the runtime gate requires exact major 1 and safe revision `0x56525801`.

Approval boundary: the verified route-based PSK milestone is approved within the reviewed scope; the P2 direct-agent validation gap is now closed. This is not deployment approval. Required patched plugins were tested privately; shared VPP deployment remains outside this review. Certificate/CA-chain trust and global private-key provisioning remain explicitly unavailable, as do documented unsupported configuration fields. IPv6 overlay/NAT traversal and full VPP-process restart recovery remain unverified. Full certificate/PKI feature acceptance is not approved.

## Finding resolution

Reviewed the root's whole-set preflight in `apps/agent/internal/desired/ikev2.go:29–66`. It tracks enabled protected-IPIP bindings and normalized underlay/local/remote peer tuples across the complete set, emits validation errors and returns before capability probing, material resolution or any profile projection. Disabled tunnels do not reserve ownership. The regression covers duplicate bindings, distinct IPIPs with identical peer endpoints and disabling the conflicting tunnel; it verifies zero secret resolutions and zero profile projections on conflict, then successful projection for the remaining enabled tunnel.

Focused desired race validation passed (1.519s): [resolution evidence](review-native-ipsec-2026-10-03-evidence/ownership-fix-race.txt). The P2 is closed; no remaining actionable blocker was found in this review. Existing deployment, certificate, IPv6/NAT and full VPP-process restart boundaries above remain unchanged. The fix was authored by root; the reviewer made no product changes.
