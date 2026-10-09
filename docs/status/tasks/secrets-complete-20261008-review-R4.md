# Credential core — final R4 boundary review

Reviewed local `fd6aa6aa`, reported remote `24ab77b9de785f63ce9f1c0269019ff89b07fb18`, tree `a14f9fce00ee18891033994d81e37a97435c4417`. No remote publication verification is claimed by this reviewer.

## Blocking finding

**BLOCKER [security: R2] — TLS syslog bypasses operational leaf-key validation.** `apps/api/src/secrets/secret-delivery.service.ts:191–203`: every selection under `/management/syslog/` is exempted from the X.509 CA exclusion applied to operational PKI private keys. The earlier `caKeyRefs` guard only derives names from `state.vpn.pki.cas`; it cannot identify a CA certificate/key hidden in a syslog alias when that CA is absent from the configured CA map. A TLS syslog entry referencing `cert/alias` containing a CA certificate and `key/alias` containing its signing key therefore reaches key database lookup, decryption and delivery. The explicit requirement in `PENDING-secret-channel.md` is that CA signing keys remain API-side even when aliased by operational consumers. Renderer rejection after delivery would not preserve that boundary.

Fix: validate each selected syslog private key against its configured matching end-entity certificate before looking up/decrypting the key, including X.509 `ca === false` and appropriate key/certificate pairing. Remove the blanket syslog exemption. Generic key consumer exemptions must also preserve the declared CA exclusion; a selection must not become exempt merely by adding an operational alias.

Required controls: (1) undeclared CA certificate/key hidden only in TLS syslog, refusal before signing-key lookup; (2) valid paired leaf certificate/key delivery at pinned versions; (3) missing/mismatched syslog certificate/private-key reference fails closed; (4) declared CA key alias refusal remains effective; (5) mixed selections cannot bypass leaf validation. The current generic syslog key-only test uses arbitrary `old` bytes and does not exercise operational X.509 safety.

## Other reviewed boundaries

WireGuard and SNMP generation bindings use the existing sealed channel and preserve selected revision identity across rotation, rollback and restart. Production SNMP no longer reads the environment fixture resolver. Malformed/unbound generation envelopes fail closed; plaintext credentials are not placed in desired configuration or scheduler bindings. API reference/count/byte limits and zeroing on errors remain in place. These source and fixture proofs do not establish native consumer acceptance or completion of the additional consumer adapters.

## Independent focused verification

Executed `pnpm exec vitest run src/secrets/secret-delivery.service.test.ts` in `apps/api`: **32 tests passed**, 202ms. Existing tests pass but do not cover the hidden-syslog-CA failure above.

Executed in `apps/agent` with the available local Go toolchain and dependency cache:

```text
go test -race -count=1 ./internal/agent ./internal/subsystems ./internal/desired -run 'TestWireguardSecretService|TestDryRunSecretsRestore|TestSecretSnapshotsFollow|TestSnmpSealed|TestSnmpGeneration'
ok ngfw/agent/internal/agent 1.243s
ok ngfw/agent/internal/subsystems 1.132s
ok ngfw/agent/internal/desired 1.116s
```

No full CI or native service/host mutation was performed. No product source was modified by the reviewer.

## Boundary correction recheck

Rechecked the dedicated correction `9de3a19258baeb996c9b708184c41f3269987ba5` (tree `f024ba8cf339fb4b0edb8d3dcb6ca11daf916f02`). Every selected syslog use now checks its own paired certificate before private-key database lookup, rejects missing or CA certificates, and remains mandatory when the same key also has another operational alias. After decryption the selected certificate must match the private key; exceptions zero the bundle before returning failure. The earlier declared-CA reference exclusion remains in place.

Independent API suite rerun: **36 tests passed**, 274ms. Added controls cover hidden undeclared CA with and without a host-stack alias, absent paired certificate before lookup, pinned valid certificate/key delivery and mismatched-key refusal. This closes the recorded blocker for the dedicated credential-core correction. Subsequent consumer integration commits require their separate review.

Verdict: **APPROVE** for the reviewed credential-core correction; native acceptance and subsequent consumer integration remain separate.
