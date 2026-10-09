# Combined sealed credential integration — independent R2 final review

Verdict: APPROVE reviewed source/security integration at local `9d61005d5af52440584650580b5a13afd75e909f`, tree `f3d9ec4a9c6978a70ab52359544eb1e6efb854ea`, manager-supplied remote `eebb6254a4d4d4128f55e61ed836aa9e83a4b04f`. No actionable integration blocker identified. This extends the earlier scoped consumer review and initial WireGuard/SNMP review; it is not a full-CI or native-appliance certificate.

Reviewed startup/projection commit `894b1b27`, the final WG/SNMP regression and strict-envelope changes, integration of the previously reviewed consumers, and the R4 CA-boundary correction/report. Startup binds WG, SNMP and host-service callbacks to the same owner-bound sealed cache before constructing the service/recovery path. NTP, syslog and namespace projection select HostServiceSecretOptions using the actual projected owner. An unavailable other owner or removed active reference refuses projection instead of using historical material as a new selection. Historical generations remain available only for already-bound descriptor values and rollback/restart.

WG selected references use the appropriate X25519/public-key or keyed PSK identity; historical resolution returns a copy and rejects malformed canonical key text. SNMP wrapper metadata exactly covers community and v3 authentication/privacy references; extra fields and malformed/missing bindings are rejected, and durable readback returns the bound generation. Both production startup adapters replace fixture paths. FRR requires both selected fingerprint and historical resolver readiness; host daemon/namespace adapters preserve the consumer review's exact-history and no-active-fallback guarantees.

The existing transaction mechanism continues to seal/stage before projection, restore durable selection after DryRun/failure, retain confirmed generations for pending-commit rollback, clear decoded RPC material, and recover selected IDs on service reopen. Startup error paths close wiring and the VPP connection rather than exposing a partially initialized service. No second plaintext cache or new secret transport was introduced. Refer to the separate R4 report for the independently tested hidden-CA/syslog alias correction; the final selector preserves its per-use certificate check and post-decryption key match, with bundle zeroization on failure.

## Independent focused verification on the exact reviewed tree

Executed with race instrumentation and count1 across eight packages:

```text
go test -race -count=1 -run 'Test(HostCredentialProjection|WireguardSecretService|DryRunSecretsRestore|SecretSnapshotsFollow|WireguardSealed|SnmpSealed|SnmpGeneration|SealedGenerations|TLSSealedGenerations|NamespaceSealedGeneration|BGPSecretGeneration|FRRSecretGeneration|HostServiceSecret|.*Bindings|.*Metadata)' \
  ./internal/agent ./internal/desired ./internal/secretchannel ./internal/secretvalue \
  ./internal/renderers/chrony ./internal/renderers/rsyslog \
  ./internal/descriptors/hoststack ./internal/subsystems
agent         PASS 1.217s
desired       PASS 1.101s
secretchannel PASS 1.020s
secretvalue   PASS 1.026s
chrony        PASS 1.048s
rsyslog       PASS 1.053s
hoststack     PASS 1.019s
subsystems    PASS 1.169s
```

This includes the actual owner-specific projection seam, WG service Apply/DryRun/confirmed revert/reopen, SNMP community/v3 rotation/reopen/rollback, strict generation envelopes, consumer metadata, historical daemon-file rendering and FRR scheduler compensation. VPP/daemon operations use fake or recording boundaries; real cache/filesystem behavior does not certify native traffic or daemon authentication. Diff whitespace check passed. R4 independently reports 36 passing API selector tests; this reviewer inspected that report and correction but does not relabel them as a second independent API run.

No CI or host-service mutation performed. The final combined unchanged gate and native WG/SNMP/BGP/NTP/syslog/namespace authentication/packet/restart acceptance remain required separately. This reviewed tree does not include the manager's later unnumbered registration or other concurrently integrated feature work.
