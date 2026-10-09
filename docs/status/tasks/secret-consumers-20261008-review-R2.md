# Sealed host credential consumers — independent R2 source/security review

Verdict: APPROVE scoped consumer source at local `7b193d7e609153b7495feb8766248a18c17e22ce`, manager-supplied remote `fdacca035149f3170e7f8ee9dbd429d01cfc9e62`, tree `fb1630c9`. Reviewed the complete consumer diff against `417e8fcd`. No actionable source/security blocker identified in this scope. Central API credential selection and startup/projection hooks are separately owned and require integrated review; this approval does not certify missing hooks or a complete application.

BGP readiness requires both selected-generation and historical-resolution callbacks. Existing FRR generation metadata and context-bound history are retained. NTP and TLS syslog scheduler values contain configuration and exact keyed generations; appended daemon metadata contains references/HMACs, not material. Unwrap validates exact coverage and rejects malformed envelopes; metadata rejects duplicates/noncanonical JSON, including duplicate JSON keys. Bound context maps are copied. Installed readback renders historical generations even when another candidate is active; rollback/restart preserves the same selection. Unbound production resolution has no active-cache or fixture fallback.

Host namespace credentials remain keyed generations in scheduler/state values and become a canonical nonzero decimal uint64 only at the VPP call. Missing/malformed values are refused before a call, delete needs no secret, request material is cleared afterwards, and secret-bearing transport errors are sanitized. Existing namespace ownership and write-only boundaries remain unchanged. NTS server support is still explicitly excluded.

Independent focused race execution on the reviewed source:

```text
go test -race -count=1 -run 'Test(SealedGenerations|TLSSealedGenerations|NamespaceSealedGeneration|BGPSecretGeneration|FRRSecretGeneration|HostServiceSecret|.*Bindings|.*Metadata)' \
  ./internal/secretvalue ./internal/renderers/chrony ./internal/renderers/rsyslog \
  ./internal/descriptors/hoststack ./internal/subsystems
secretvalue  PASS 1.031s
chrony       PASS 1.056s
rsyslog      PASS 1.058s
hoststack    PASS 1.022s
subsystems   PASS 1.155s
```

These fixtures use the real sealed cache and private test files with fake/recording daemon or VPP boundaries. They cover generation rotation, historical rollback, reopening caches/descriptors, revocation/refusal, owner isolation, metadata persistence and FRR scheduler compensation. They do not prove real daemon handshakes, packet forwarding or an installed appliance. Diff whitespace check passed. No CI, host-service mutation or product code edit was performed by this reviewer.

Before whole-feature closure, review the exact combined API selector and agent/projection wiring, retain the separate CA-signing-key isolation review, then execute the owner-deferred final combined gate. Previously reported sandbox socket/PID restrictions are not converted into passing full-package results by this focused review.
