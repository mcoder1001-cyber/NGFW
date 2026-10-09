# R4 SNMP transaction-owner correction review

Verdict: **APPROVE** the owner-binding correction at `91d1040a3157323fa3d59d7b8d38373728f417c4`. This is a focused security/shared-host review; BFD range and PPP protobuf-presence changes are outside this receipt.

The product projection now passes the transaction's explicit owner into both SNMP preflight-check selection and sealed-generation selection. Each lookup uses only that owner's registry entry. It cannot select another owner's sole entry when the requested owner or selected key is absent. Configurations with secret references fail closed when the explicit owner has no generation provider; provider errors are replaced with non-secret unavailable-generation errors and emit no SNMP scheduler object. Legacy ownerless test callers preserve the existing ambiguity rejection.

The generation envelope still requires exact coverage of community/auth/privacy references, accepts only validated keyed HMAC generation identifiers, and contains no plaintext. Runtime rendering resolves only the generation bound to that reference through the stage's own sealed history; missing bindings or history remain unavailable errors. This correction does not add a cross-owner fallback or alter that consumer boundary.

The new regression creates two independent sealed stores and registrations. It proves the selected checker is used, the selected store's generation is emitted, the foreign checker is never called, missing/ambiguous ownership emits no SNMP object, and a removed selected key does not fall back to the other owner's still-present key. The corpus fixture now stages its SNMP references under a named sealed owner instead of adding a new error exemption. The Syslog assertion remains a negative test: it requires exactly one unavailable-generation error at the actual whole-object binding pointer.

Independent tests in the review checkout, Go1.26, local offline caches, race and count one:

```text
go test -race -count=1 ./internal/agent ./internal/desired -run 'TestSnmpProjectionSelectsSealedOwner|TestHostServicesRefusedAtDryRun|TestSnmpGenerationEnvelopePersistenceAndValidation'
ok ngfw/agent/internal/agent 1.438s
ok ngfw/agent/internal/desired 1.287s

go test -race -count=1 ./internal/subsystems -run 'TestSnmp(SealedRotationRollbackRestart|ProjectionChecksBeforeVPP|CheckPerOwner|FixtureResolverRefusesRealSecrets)$'
ok ngfw/agent/internal/subsystems 1.155s
```

Both completed without skips. No CI or native service activation was performed by this reviewer. No unresolved blocker or major finding remains in this scoped correction; final cumulative lint/tests and hosted exact-tree gate remain required.
