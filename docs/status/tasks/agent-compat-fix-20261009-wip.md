# Agent projection compatibility fixes

Branch `codex/agent-compat-fix-20261009`, based on `09bc6cf9` plus carrier owner's BFD nil-range fix `af0c0c6a` (local cherry `a6a770ff`).

Owned: SNMP owner-selection projection and new regression, schema-corpus fixture, unavailable Syslog DryRun assertion. Carrier owner approved desired SNMP production changes and the single `projection.go` call.

The schema corpus exposed a real owner-selection gap: SNMP chose a process-global sole checker/fingerprint instead of the transaction owner. Projection now passes its owner; selected owners resolve only their checker and sealed generations. Legacy ownerless callers still refuse ambiguous registries. Missing owner/generation remains an error and emits no SNMP object. The corpus now stages its SNMP references in one sealed owner store; no additional error exemption was added. Syslog's unavailable-generation error names `/management/syslog` because binding occurs at the whole-object stage; the test requires exactly one error and the existing secret-channel rule.

Negative control: the new two-owner sealed-store regression against the prior production files failed with `services.snmp.render`, refusing to guess among registered owners. The old Syslog assertion also reproduced the reported exact pointer mismatch. An initial test-author reference typo was corrected to the channel's existing `password/` contract before the meaningful negative control.

Focused command: `tools/heavy.sh go test -race ./internal/agent -run 'Test(SnmpProjectionSelectsSealedOwner|HostServicesRefusedAtDryRun|ProjectSchemaExamples)$' -count=1`, from apps/agent with restored pinned toolchain. First full focused PASS: `ok ngfw/agent/internal/agent 1.349s`; additional explicit ambiguous-secret and missing-generation assertions rechecked before commit.

Standalone corpus additionally exposed BfdIDSpan nil dereference under the existing `all` range; carrier owner supplied the production fix, without masking the failure in this fixture. No hosted CI triggered. Next: independent R2 review, carrier cherry-pick, final existing campaign gate.
