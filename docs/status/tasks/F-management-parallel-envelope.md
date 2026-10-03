# F-management parallel recovery envelope

Branch: codex/management-resume-parallel-20261003
Base: 19aa88a5
Worktree: /root/Documents/Codex/2026-10-03/check-out-latest-code-from-git/work/NGFW-management-resume
Slot: 2; daemon ownership: none; no host mutations.
Owned: apps/api/src/features/mgmt-tls/**, docs/user/system/management.md, docs/status/tasks/F-management-parallel-*.md.
Scope: recover HTTPS websocket upgrades through existing Fastify stream route, with real TLS transport regression tests. No schema, bootstrap, proxy, secret boundary or pending transport decision changes.
Remaining: certificate removal/start-after-empty/racing reload state gaps; real DB/lab acceptance deferred, not passing.
