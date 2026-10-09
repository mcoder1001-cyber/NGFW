# Recovery closure independent review (R2)

Reviewer branch: `codex/review-recovery-20261009`. Reviewed source: `42b0c993793a497a1597fb36f08729f52a51b2ec` on `codex/pppoe-kernel-carrier-20261008`. Owned file: this report only; no product changes.

Scope: the new scheduler pending-CREATE prerequisite closure, missing-TAP recovery marker/graph, durable namespace ownership declarations and carrier descriptor registration. This is a focused independent security/correctness review, not approval of unrelated carrier changes or native acceptance.

## Findings

No blocker or major finding in this delta from source inspection.

- Prerequisite expansion admits only existing `OpCreate` entries from the validated plan; it does not fabricate an absent optional dependency. It follows provided aliases and observed intermediate dependencies, topologically orders the combined restoration set, and rejects a cycle.
- Early creates use the normal executor/journal path. The done/live check prevents a duplicate later creation, and reverse rollback removes early prerequisites before restoring the previous dependency tree. Existing live prerequisites are traversed without being recreated merely because they are dependencies.
- The repair marker is retrieved-only. Namespace deletion retains the exact observed owner/specification, boot, inode and generation receipt. A replacement namespace gets a new generation before its consumers are restored; foreign or incomplete receipts are rejected.
- Namespace ownership declares the helper's boot-scoped disk ledger as persistent across agent restarts. A volatile provider fails the ownership check. TAP ownership remains the VPP owner tag plus independently verified namespace admission, with a separate descriptor identity from remote-access TAPs.
- This source delta adds no shell command execution, credential output, API endpoint, dependency or broader privilege allowance. Reviewed new tests use synthetic metadata and the sanctioned test-secret literal.

## Verification

Worktree: `/workspace/scratch/9baf7442ffbf/ngfw-review-recovery`, Go 1.26.0, race detector, `GOMAXPROCS=2`, `GOFLAGS=-p=2 -mod=readonly`. Focused commands only; user-requested aggregate CI deferral preserved.

Commands executed from `apps/agent`:

```text
go test -race -count=1 ./internal/scheduler ./internal/descriptors/pppoe
ok  ngfw/agent/internal/scheduler          2.777s
ok  ngfw/agent/internal/descriptors/pppoe  1.103s

go test -race -count=1 ./internal/subsystems -run 'TestCarrier|TestRequirePersistent|TestEveryInterfaceCreatorNamesItsAlias'
ok  ngfw/agent/internal/subsystems        5.510s
```

`git diff HEAD^ HEAD --check` passed. New recovery/ownership and selected carrier controls have no skip path. Existing native opt-in PPP tests in the descriptor package were not enabled and do not count as acceptance. The scheduler suite ran without `-short`.


The carrier recovery graph exercises the actual NamespaceDescriptor and scheduler; TAP transport and daemon operations are recording stand-ins. The subsystem controls exercise actual carrier runtime/registration with fake transport/runner. These are unit-level proofs, not native VPP/kernel/systemd or packet acceptance. No live service or laboratory mutation was performed.

Verdict: **APPROVE** for the specified recovery/ownership delta at `42b0c993`; zero blocker, major or minor findings. Final combined-source gate and native acceptance remain separate requirements.


## Combined integration preservation review

Reviewed final candidate `33db7c632c38e72239e8861bcabf23ac0c45a6aa`, tree `bf2e858290a8789321ba92e30cd944fec23ecd0b`, in independent detached worktree `/workspace/scratch/9baf7442ffbf/ngfw-review-integration`. Source is frozen; this supplement changes only the reviewer report.

The resolved `agent/projection.go` and `desired/interfaces.go` retain both owner-bound `HostServiceSecretOptions(owner)` consumers, unnumbered projection/readback, supervised PPP interface treatment, carrier projection and WAN membership context with the carrier owner argument. No credential/unnumbered hook was lost when integrating PPP.

Reviewed `multiwan.RouteDescriptor` and its product registration: an optional PPP client configuration dependency orders WAN route creation after standalone PPP default/readiness withdrawal. The inverse order removes WAN paths before restoring standalone policy; a system without PPP remains valid. The actual scheduler/client/runtime/WAN descriptor test covers injected route failure and rollback, subsequent join, all-down health without fallback bypass, and leave. Native topology and transport remain stand-ins.

The NCP epoch fence now reads the generation before kernel/VPP observations and compares it again before returning verified forwarding. Product forwarding controls reject drift, in-flight process replacement and same-process NCP replacement. The separate observation-failure source review belongs to the carrier security reviewer; it was not silently folded into this review's scope.

Exact frozen-candidate commands, Go 1.26.0, race, count=1:

```text
go test -race -count=1 ./internal/agent -run 'TestHostCredentialProjectionSelectsOwnerAndRotation|TestUnnumberedDomainApplyRetrieveRevoke|TestWANRoutingOnlyJoinLeaveReprojectsPPPDefaultOwnership'
ok  ngfw/agent/internal/agent       1.234s

go test -race -count=1 ./internal/subsystems -run 'TestCarrierWANTransactionOrdersRoutesAndRollsBackFailure|TestCarrier.*Forward|TestCarrier.*Epoch'
ok  ngfw/agent/internal/subsystems  1.282s
```

Selected controls have no skips. Aggregate/hosted CI remains deferred to the manager's final combined campaign. No native or laboratory acceptance is claimed.

Combined narrow integration verdict: **APPROVE** at `33db7c63`; zero blocker, major or minor findings for the conflict preservation, WAN ordering and NCP fence scope above.


## Final campaign full agent unit run (69e28859)

Manager requested the unchanged agent unit stage after hosted CI stopped at Go lint. Source: `69e28859`; independent worktree `/workspace/scratch/9baf7442ffbf/ngfw-review-integration`; no product edits. Loaded restored toolchain environment, unset `NGFW_INTEGRATION`, set `NGFW_CI_APPLY_SHARDS=2` (the agent Makefile itself does not use that shell-harness setting), then ran `make test` in `apps/agent`, invoking exactly `go test -race -count=1 ./...`.

**Result: FAILED**, make exit 2. Package summary: 128 passed, 15 failed, 166 reported no test files. No data-race warning, panic or compile error appeared. Native opt-in tests remain unexecuted; package pass counts are not native acceptance. Full transient log: `/workspace/scratch/9baf7442ffbf/recovery-final-agent-unit-69e28859.log`.

### Concrete non-environment failures sent to the manager and author

1. `internal/contracttest: TestExplicitPresenceEverywhere`: `delegation_targets.interface` and `delegation_targets.subnet_id` are scalar proto fields without explicit presence required by D-039. Requires contract/generation correction.
2. Globals-owner `internal/agent` tests for BFD, IPFIX, LISP, loopback/LLDP and MPLS retrieve `pppoe.carrier.namespace` and fail because their fake runner has no carrier-helper inventory response. Must reconcile product registration/test wiring while retaining orphan discovery and fail-closed real helper failures; never suppress inventory merely because the desired document lacks PPP.
3. `TestProjectSchemaExamples`: ownerless projection of SNMP example returns `services.snmp.secret: snmpd.config: ambiguous secret owner`. Requires proper owner/secret fixture or explicit intended diagnostic; no blanket error exemption.
4. `TestHostServicesRefusedAtDryRun`: refusal rule remains `agent.secret-channel-pending`, but pointer is `/management/syslog` instead of expected `/management/syslog/0/tls`. Requires precise diagnostic/fixture reconciliation.
5. `renderers/pppoe: TestReadStateIncludesIPv6`: fixture expects a delegated-prefix summary without current lease metadata; actual summary contains only the address. Reconcile fixture with valid PD lifetime/admission; do not weaken expiry checks.

### Environment failures are still failures, not passes

Unix/Unixgram and netlink socket creation returns EPERM. The user namespace maps only UID/GID 0, so foreign UID/private nonzero group chown returns EINVAL. Independently observed `os.getpid()=3` while `/proc/self` links to host PID `37646`; process identity, child boot, held namespace and renderer process-restart controls therefore cannot establish their required proof. This matches failures in RA boundary tests, PPP IPv6 child lifecycle, rfkit, rsyslog and snmpd. These tests must execute unchanged on the hosted runner before merge; they are not converted into laboratory-only deferrals.

Complete failed test-name inventory (top-level tests; nested cases remain in the full log):

- `ngfw/agent/cmd/ngfw-vppcheck`: `TestHungVPPTimesOut`.
- `ngfw/agent/internal/actions/capture-trace`: `TestForeignUIDSrcRefused`.
- `ngfw/agent/internal/agent`: `TestRunStopsOnCancel`, `TestSocketPermissionsAndInUse`, `TestProjectSchemaExamples`, `TestBfdMultihopApplyStateRestartAndRollback`, `TestBfdMultihopFailedApplyRollbackReleasesTuple`, `TestDet44StateOnFake`, `TestCnatStateOnFake`, `TestHostServicesRefusedAtDryRun`, `TestIpfixSflowGlobalsOwnerLifecycle`, `TestIpfixExporterNamesOnlyFromAppliedState`, `TestLispFullExampleApplyRetrieveResyncRollback`, `TestLispGpeEntryReappliedAfterVPPRestart`, `TestLispRequiresGlobalsOnSlotAgents`, `TestLispStateRPC`, `TestLoopbackBviGsoLldpSpanGlobalsOwner`, `TestMplsTableZeroRequired`, `TestNatSessionsSummaryKillOverGRPC`, `TestNatEIAndNat64SessionsOverGRPC`, `TestNatVariantWalksShareTheEDWalkSlot`, `TestAgentStopClosesObjectsRuntimeAndMetrics`, `TestStartWiresFeatureEventsAndResync`, `TestStartIDRangeFailsClosed`, `TestMetricsCollectors`, `TestRequestedResyncsAreRateLimited`, `TestGRPCRoundTrip`, `TestConfigReplyTimeoutAndMetricsOptIn`, `TestOwedResyncTakesTheAgentsResyncPath`, `TestGRPCHandlerPanicAnswersInternal`, `TestConnectHookHasItsOwnDeadline`, `TestInterfaceRemovedDeletesAttributesFirst`.
- `ngfw/agent/internal/contracttest`: `TestExplicitPresenceEverywhere`.
- `ngfw/agent/internal/descriptors/af_packet`: `TestNetlinkLookup`.
- `ngfw/agent/internal/ra_vpn`: `TestPublishSourceGenerationAndForeignPointerRefusal`, `TestSourceAgentReferenceOwnershipAndProcessBinding`, `TestNumericPublisherHeldInstallationRejectsChanges`, `TestNumericPublisherManagerDBusTypesPreserveOriginalPredicates`, `TestNumericPublisherTripletSingleUseAndFreshHeldGuards`, `TestNumericPublisherTripletOriginalManagerPostProofRefusesReplacement`, `TestNumericPublisherValidationDeadlineSeparateFromIPC`, `TestBrokerPlaceholderRootOwnerWithPrivateNonzeroGroup`, `TestStoppedRepairRequiresStableManagerAndGoneGeneration`, `TestTargetsOpenFileReadbackProtectsOwnedFile`, `TestNamespaceTargetsRejectActualSamePIDChanges`, `TestNamespaceTargetsCompareHeldImagesAndCompleteGenerations`, `TestObserverOpenFileReadbackProtectsOwnedFile`, `TestUnitObserverSnapshotRequiresTypedNamespacesAndMatchingIdentity`, `TestUnitObserverFreshCaptureRejectsSamePIDChanges`, `TestSnapshotPrivateGroupDoesNotRequireChown`, `TestRetiredVPPBootRefusesLiveProcessAndUnknownIdentity`.
- `ngfw/agent/internal/renderers/chrony`: `TestApply`, `TestDescriptorRestartPending`.
- `ngfw/agent/internal/renderers/pppoe`: `TestIPv6OwnedShutdownAndLateEvent`, `TestIPv6CollectionRevocationBarrier`, `TestIPv6PIDIdentityRefusesForeignSignals`, `TestIPv6SetupAndClientFailures`, `TestIPv6NaturalLossAndGenerationReplacement`, `TestIPv6AbruptPppdLossStopsChild`, `TestIPv6ApplyStopsBeforeReplacingOrRemovingFiles`, `TestIPv6TransitionAdmissionRemainsFenced`, `TestIPv6PinnedParentRejectsRecycledNumericIdentity`, `TestIPv6MissingHelperPreservesProcessEvidence`, `TestIPv6DepartedParentHooksPreserveReplacement`, `TestReadStateIncludesIPv6`.
- `ngfw/agent/internal/renderers/rfkit`: `TestProcessControllerRefusesForeignPID`.
- `ngfw/agent/internal/renderers/rsyslog`: `TestDescriptorDeferred`.
- `ngfw/agent/internal/renderers/snmpd`: `TestApplyConvergesAndRollsBack`.
- `ngfw/agent/internal/renderers/strongswan`: `TestRASocketRestrictionPinsInodeAndVerifiedPeer`, `TestRAVICISocketRequiresPrivateModeAndExactDaemonPID`.
- `ngfw/agent/internal/renderers/unbound`: `TestDescriptorRestartPendingUntilActedOn`, `TestApply`.
- `ngfw/agent/internal/snmpagent`: `TestSubagentWalk`, `TestSubagentReregisters`.
- `ngfw/agent/internal/subsystems`: `TestLinuxNetdevKindOnThisHost`, `TestActualStopPublicManifestAndPrivateReaderRefuseAmbiguity`, `TestRAMountTargetUsesHeldNSFSAndRejectsProcessReplacement`.

Full-campaign verdict at `69e28859`: **BLOCK pending the real corrections and an unchanged green hosted gate**. Earlier narrow source approvals do not assert full-suite success. No hosted workflow was triggered by this reviewer.
