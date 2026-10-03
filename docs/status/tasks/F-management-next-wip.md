# Management TLS shutdown WIP
Branch codex/management-shutdown-20261003; parent eabeb473, local/remote checkpoint pending initial publication.
Completed: read task/context/policies, existing lifecycle/stream source/fixtures and lab audit; selected upgraded-connection shutdown gap. Isolated worktree created; pnpm offline frozen dependency setup running.
Tests: none yet. No passing claim before actual reproduction.
Remaining: real active authenticated TLS/WebSocket shutdown reproduction, bounded owned-socket cleanup, regressions, docs, independent review and hosted gate.
Exact next command: add focused real shutdown fixture, pnpm --filter @ngfw/api exec vitest run src/features/mgmt-tls/mgmt-tls.test.ts -t 'closes live authenticated HTTPS upgrades on shutdown'.
