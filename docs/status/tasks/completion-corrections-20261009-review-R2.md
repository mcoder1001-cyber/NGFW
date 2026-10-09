# R2 final campaign correction review

Verdict: **APPROVE** the scoped source corrections at `c41bc2253d5f9984d08bd8772f4782802e984f2c`, tree `ed7be7c2cff6ea26dec9c34f8a8759f32a824993` (last product commit `1d7f3d96707c01f9d8bd9008f95782a7e7ae4815`). No blocker, major or minor finding remains in the correction categories below. This is not an assertion that the complete hosted gate has passed.

Independent worktree: `/workspace/scratch/9baf7442ffbf/ngfw-final-review`, branch `codex/final-review-20261009`. Owned file: this report only. Read AGENTS, shared context, contribution and decision policy, the prior R2 recovery/full-campaign failure report and the carrier completion work log. Reviewed the correction diff against the failed `69e28859` baseline, with focused inspection of changes after `09bc6cf9`; the separate R4 receipt `f409acab` covers the lint/security changes and SNMP consumer boundary. No product edits, CI trigger or native service activation.

## Findings resolved

- PPP delegation target scalars now have explicit protobuf presence without changing wire field numbers/types. Generated Go pointers and TS optional fields preserve absence versus explicit zero; consumers use generated getters. The D039 explicit-presence regression passes.
- PPP inventory injection exists only in construction-time `subsystems.Env`; no production caller supplies it. Generic test wiring explicitly supplies an empty inventory instead of accidentally invoking a privileged broker through an unrelated fake runner. A nil injection still calls the fixed broker even with no desired PPP configuration, validates receipt count/owner/leases and returns broker errors. Orphan discovery and fail-closed production inventory were not disabled.
- SNMP projection passes its transaction owner to both checker and sealed-generation selection. Missing selected owner or key never falls back to a foreign registration. The two-store test independently passes, including ambiguous ownerless lookup and missing selected generation. The schema corpus supplies a real sealed fixture generation; it adds no new error exemption.
- Syslog DryRun still refuses the unavailable TLS generation. The assertion now matches the whole-object binding pointer `/management/syslog`, requires exactly one error and retains the secret-channel rule. The separately existing no-channel path still points to the individual TLS entry.
- The IPv6 summary fixture now supplies current PD generation and lease deadlines. Missing, expired, reversed and invalid-generation leases remain rejected by the unchanged negative control.
- BFD `all` ID scope intentionally returns nil from `SlotIDRange`; `BfdIDSpan` now returns the full uint32 range instead of dereferencing nil. Invalid scope still yields the empty range, while a numbered slot remains restricted to its 1000 assigned IDs.

## Independent verification

Loaded `/workspace/scratch/9baf7442ffbf/toolchain/env.sh`: Go 1.26, race detector, `GOMAXPROCS=2`, `GOFLAGS=-p=2 -mod=readonly`; additionally `GOPROXY=off`. Commands ran from `apps/agent` in the independent frozen worktree.

```text
go test -race -count=1 ./internal/contracttest ./internal/desired ./internal/renderers/pppoe -run 'TestExplicitPresenceEverywhere|TestPppoeDelegation|TestReadStateIncludesIPv6|TestReadIPv6DelegationExpiry'
ok ngfw/agent/internal/contracttest 1.123s
ok ngfw/agent/internal/desired 1.150s
ok ngfw/agent/internal/renderers/pppoe 1.044s

go test -race -count=1 ./internal/agent -run 'Test(SnmpProjectionSelectsSealedOwner|ProjectSchemaExamples|HostServicesRefusedAtDryRun|BfdMultihopApplyStateRestartAndRollback|BfdMultihopFailedApplyRollbackReleasesTuple|IpfixSflowGlobalsOwnerLifecycle|IpfixExporterNamesOnlyFromAppliedState|LispFullExampleApplyRetrieveResyncRollback|LispGpeEntryReappliedAfterVPPRestart|LispRequiresGlobalsOnSlotAgents|LispStateRPC|LoopbackBviGsoLldpSpanGlobalsOwner|MplsTableZeroRequired)$'
ok ngfw/agent/internal/agent 2.125s

go test -race -count=1 ./internal/subsystems -run 'Test(BfdIDSpanUnscopedAndInvalid|CarrierRegistrationKeepsRATapOwnershipSeparate|CarrierRestartReadbackStopsBeforeSecretReplacement|CarrierResolverPrivateFile|CarrierWANTransactionOrdersRoutesAndRollsBackFailure|CarrierWANMembershipWithdrawsBeforeRestartAndRollbackRestoresPolicy|PppoeDelegationProductLifecycle|PppoeDelegationRefusesStaticAdoption|PppoeDelegationRefusesStaticRAAdoption)$'
ok ngfw/agent/internal/subsystems 1.305s
```

All three commands passed. `git diff 69e28859..c41bc225 --check` passed. The selected schema corpus exists in this checkout; selected controls exercise fake VPP/daemon boundaries and do not constitute native acceptance. No full-suite repeat or generator run was performed by this reviewer: generated output was inspected, and exact-tree regeneration/full validation remains part of the unchanged hosted gate.

The previous complete agent run's PID/proc, foreign UID/GID and socket failures are not relabelled PASS or laboratory-only. They must pass on the hosted runner before merge. Native PPP/VPP/ISP and appliance acceptance remains NOT RUN. Manager owns publication of this report, the remaining final campaign rerun, and expected-head merge after a green exact-tree gate.
