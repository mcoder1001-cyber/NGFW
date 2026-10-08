# Sealed credential consumers completion

Branch codex/secret-consumers-20261008 from main417e8fcd. Owner instructed completion, one final CI after source work. API selectors and central agent/projection wiring belong to secrets_complete; this worker owns BGP readiness, NTP/syslog descriptor generation bindings and host-stack namespace adapter, tests and docs.

Contract help correction published as fb06e3ff (local18fb6004), exact treeafe81d284629276abd7da2c42c62ac990d922ad2. No generated output hand-edited; manager runs final generation.

In progress: internal/secretvalue is a bounded serializer/context-binding helper, not a second cache, descriptor or renderer. It uses the existing keyed HMAC contract and carries no material. TD11a scans direct descriptor/renderer package directories only, so no library exemption or pending count change applies. Host-service production resolver accepts only context-bound historical generations from the selected sealed cache. NTP/syslog persist only generation metadata with their existing renderer input for restart and rollback. Host namespace stores only keyed generation in scheduler values and resolves canonical nonzero decimal uint64 at the VPP call. FRR existing sealed history path now enables BGP credential projection when both callbacks exist.

Focused checks so far: secretvalue roundtrip/exact coverage/rotation metadata PASS1.041s; NTP actual file rotation/rollback/restart/removal PASS1.065s. Earlier broader consumer packages exposed environment limitations (Unix datagram sockets EPERM; daemon PID/proc mismatch) and a changed error pointer regression, which has been corrected and is being rechecked. Do not claim full package or final CI pass. Syslog/host-stack/BGP focused generation tests, source review, central wiring integration and final validation remain.

No live VPP/daemon mutation, no CI. Next: finish consumer regressions, ensure missing/revoked selections cannot use active fallback, independently review and publish coherent source checkpoints.

## Consumer source completion checkpoint

Added exact-history fixtures using the real sealed store: TLS file rotation, historical rollback, cache/descriptor reopen, selection revocation and stale-file removal; namespace generation rotation, old-value replay after cache/descriptor reopen, revoked selection and malformed uint64 refusal without a VPP call. BGP and existing OSPF exercise actual scheduler compensation after a later dependent operation fails. FRR intentionally reconstructs ownership by scheduler reapply after restart, rather than inferring secret refs from running-config; the restart fixture explicitly reapplies the historical value against reopened sealed history. Owner adapter tests reject unbound access and cross-owner fallback. Metadata decoding rejects duplicate JSON keys and noncanonical encoding. Chrony clears contextual bytes on validation failure as well as success.

Focused race results: secretvalue PASS 1.035s; hoststack generation PASS 1.018s; rsyslog TLS generation PASS 1.048s; chrony generation PASS 1.049s; desired refusal compatibility PASS 1.070s. Full consumer suites and daemon handshakes are not claimed: earlier chrony Unix datagram EPERM and rsyslog PID/proc fixture mismatch remain environment limitations to reproduce on the final validation host. Central API selection and agent/projection integration are separately owned and must be reviewed together before release.

Feature docs and schema help now describe the credential formats and sealed selection accurately. Schema shape unchanged; final generation belongs to the manager. No CI and no production host actions performed.

Final focused FRR/BGP scheduler rollback, reopen/reapply, revocation, owner isolation and TD11a reachability checks: PASS 3.008s with race detection. The first restart fixture incorrectly expected runtime-owned readback before reapply; corrected to the documented FRR ownership contract, then reran successfully.
