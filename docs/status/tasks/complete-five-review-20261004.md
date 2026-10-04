# Five incomplete tasks — first reviewed integration

F-dataplane-ui: remote 4067bfb7f29bf966c60e7634fa46046c9ff87294; independent management-agent review approved after localized thread-label fix. Focused API3/UI5, typecheck/lint/vet and real read-only VPP runtime probes pass.

F-management-ui: remote 33e774485eca185cf3bcc9c4035393279d4bcd4b; independent dataplane-agent review approved and reran15 TLS/WSS tests successfully. API15/UI+locale11, typecheck/lint pass.

Combined proto/API client generated and compiled. Hosted/full CI waived by owner. Lab/browser and privileged restart cases remain in DEFERRED-ACCEPTANCE.md. Setup, PKI and OSPF continue on their separate published branches.

## Final three-task integration

Base: main15ce0d28bc63f1da1772a6e38b36ee0658ee9554 (PR147). All task branches preserved reviewed checkpoints. Final source refs:

- Setup: b496d619b49a5f6f57c377e68c5022c4dcf66a4b; product337c50f5411b79264bbb28364ecdc8af20c99f76 independently approved (later status only).
- PKI:85747e3a6cad1148a0af5969594fad81c8565db8, exact tree3d8b80b9a8a5ef4e713153c398d326961e3c7d51 independently approved; independent final UI5/secret10 passed.
- OSPF:b21e40cbbe8a4c64342985352cc2f242b5045848; productc2460d469 independently approved, later OpenAPI summaries/status only. Reviewer verified summaries in integration.

The generated Go conflict was resolved by regeneration from combined proto, preserving dataplane runtime fields. Setup metadata stays API-only: a narrow contract guard enforces readOnly and rejects any wire field, with regression tests. No arbitrary schema drift exclusion was introduced. OSPF OpenAPI missing summaries were fixed before client generation.

Final combined checks: focused API125, web24 and schema9 passed; Go pki/subsystems/desired/agent/frr/ospf passed and contract guard rerun passed after current schema generation and setup guard fix. Package builds/API client generation/web typecheck and final source secret scan are checked before merge. Complete_setup independently reviewed combined integration seams and source equality; exact final tree acknowledgment follows freeze.

Hosted/full CI waived by owner. No code failure was deferred as lab acceptance. True boundaries are in DEFERRED-ACCEPTANCE.md; no production MD5, native certificate consumer or shared-host restart is claimed.

Final integration APPROVE: independent complete_setup reviewer verified frozen localc6757f949c34fb932d9aec510e09ca2d90f4ac0d / tree4e429407b91ce6e4e9a9e68ed8d2f050025e23c5; clean worktree and source diff, combined contract boundary regressions and focused Go packages passed. Subsequent changes: this status paragraph and a named public reader constant in the on-demand regression fixture to remove a generic-key scanner false positive; no production behavior changed. API/client generation and web typecheck completed with exit0; final single-commit source secret scan is rerun after the fixture correction; no actual credential was present.
