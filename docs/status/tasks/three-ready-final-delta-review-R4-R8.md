# Final published batch — R4/R8 delta approval

Reviewed published PR145 head `864ee7ac923d3524375bd651f6fe30cf65ae5766` through identical local integration product tree `f4dd43b9611fc41d72edc10d9ffca0b08e5aba29` (local head `80041e1a`). Independent read-only product review of delta after prior integration `6eda5e94`.

**R4 APPROVE; R8 APPROVE. No remaining blockers.**

ACL/PBR scope correction `0bbc78fa` conditionally adds the dependent ACL projection only for relevant old/new ACL, enabled AutoBlock or runtime-entry context. It retains old/runtime context during overlay disable/removal, and does not broaden unrelated interface transactions into authority over owner-tagged external ACLs. The actual projected domain list reaches both config-only fallback and merged dynamic-source Apply scopes; DryRun uses the same effective list, including each retry. Dynamic descriptor names remain appended independently. Source Desired uses the prospective configuration assembled from the original requested domains, and persisted authoritative metadata remains those original domains. No PIM/LDP ownership collision, omission of dynamic source scope or source lifecycle/teardown interference introduced. Guarded uint32 LDP installed counting preserves actual ownership-retrieved state semantics. Remaining exported-symbol/lab-boundary changes are documentation only.

Independent test run on this exact tree, cwd `integration/apps/agent`:

```text
/workspace/scratch/e4f791ef53f7/go/bin/go test -race -count=1 ./internal/agent -run 'InterfaceScope|DisabledAutoBlock|Dynamic|Source'
ok ngfw/agent/internal/agent 3.582s
```

Prior per-task and combined R4/R8 approvals continue to apply. This approval is not a hosted CI result or lab packet/restart acceptance; merge still requires the complete hosted quick gate to pass.
