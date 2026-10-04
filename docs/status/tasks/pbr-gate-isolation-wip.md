# Recovery checkpoint

Branch fix/pbr-gate-isolation-20261004, base integration6eda5e94.
Owned source: agent/projection.go, service.go, approved narrow dynsource.go planSources
hunks, new agent/acl_scope_test.go and subsystem/rpf_adl_pbr_test.go fixture cleanup.
No board/schema/binapi/lab changes. Manager publishes final branch/checkpoints.

Confirmed cause: scopeOf unconditionally appended the whole ACL family for interfaces
or security, and projectWithBasePolicy reprojected ACL despite no stored/prospective
ACL or active security overlay context. Preseeded same-owner ACL references therefore
became deletions during unrelated interface transactions. The scheduler's mandatory
PBR dependency rejection was correct. Omitted Subsystems is NOT full-authoritative:
authoritative() uses document-present domains; earlier diagnosis was disproven.

Fix: scopeOf exact requested domains; projection expands ACL only when old/new ACL or
enabled AutoBlock configuration or cached runtime entries requires it (old context
preserves disable/removal). projected.scopeDomains carries dependent scope to Apply,
DryRun/drift, dynamic merge planning. Stored/requested authority metadata remains original.
Disabled AutoBlock without entries/ACL context no longer expands interface scope.

Actual verification: before full package-count2 failed all3RPF cases plus independent
LinuxNetdevKind EPERM; isolated RPF also failed, so not suite ordering/global state.
After fix focused PBR package race-count2 PASS1.591s; full subsystem package-count2
21.272s failed ONLY TestLinuxNetdevKindOnThisHost twice (netlink operation not permitted).
Agent focused race-count2 scope/runtime tests PASS2.004s. Independent R1 focused race
agent4.100s/subsystems1.370s PASS. Final broader agent race-count2 passed5.538s; pinnedgolangci-lint2.13.2 returned0issues; exact
logs /tmp/pbr-before.log, /tmp/pbr-fix-focused.log, /tmp/pbr-scope-full.log,
/tmp/pbr-final-agent.log, /tmp/pbr-final-lint.log.

Next: manager publishes frozen source/report and integrates after independent reviews; source/security check passed.
Complete gate is BLOCKED-ENV until netlink access exists; no unit skips/weakening.
