# Management/dataplane acceptance envelope

- Owner: `management_acceptance` child agent; branch `codex/closeout-management`.
- Worktree: `/root/ngfw-wt/codex-closeout-management`; base/source `6645443499d111d7cbbaa8ed2d5f6bd9105544af`.
- Files owned: `test/topology/management-dataplane/**`, `docs/status/tasks/closeout-management*`.
- Exclusive test slot: 9, HTTPS sub-port HTTP+1 = 3901. Private disposable VPP only; globals off.
- Runtime build artifacts reused read-only from manager workspace at the same source SHA.
- Scope: actual API/DB TLS lifecycle and dataplane state/preview/semantic validation acceptance owed by `RV-B-review-R7`.
- Forbidden: shared VPP/service restart or global configuration, product/board edits, PR/merge.
- Publication: manager's push received HTTP403; do not retry alternative credential routes. Local result must be handed back with publication explicitly blocked.
