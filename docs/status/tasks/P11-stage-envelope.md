# P11 materialised stage envelope

Developer: current agent; branch `codex/p11-stage-20261003`; isolated worktree `NGFW-p11-stage`; base `2dbdff2405e845da47aa149bb72cdedbce60649c` (PR101 pending).
Slot 2; no daemon ownership. Timebox 90 minutes. Own only `deploy/strongswan/**` and `docs/status/tasks/P11-stage-*`.

Implement bounded real source/dev archive materialisation from the unchanged complete verified intake gate. Mandatory independently trusted source digest at public API; verify/extract the same private snapshot. Reject links, traversal, duplicate/special entries, overwrites; enforce file and total expansion budgets; cleanup failures and publish atomically to a new private owned root.
No source execution, compiler, installation, VPP/service changes, C/plugin/agent/generated/board edits, self-review or merge. Builder, ABI/ID, security/licensing and agent wiring remain unfinished. R2 independently reviews; unchanged hosted quick gate required before merge. Publish checkpoints; if credentials fail, manager publishes exact local SHA without auth changes.
