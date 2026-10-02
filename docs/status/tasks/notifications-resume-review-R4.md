# Notifications bounded slice — independent R4 review

Reviewed frozen local `80c6fcd9` (PR66 remote `d2e81e64`, base main `c76774e8`) in isolated branch `review/notifications-r4-r5`. Read shared context/review rules and R4/R5 prompts, shared-host rules and pending lab handover. Reviewer changed no product files.

Findings: no R4 blockers or majors. The only `apps/agent` change is generated protobuf output; no VPP descriptors/messages, binapi generator, C code, host topology, systemd units or host startup configuration changed. API-owned notifications are removed from desired-state projection and drift comparison. Dispatcher listens to existing event buses and sends outbound SMTP/HTTPS; it creates no VPP objects, tables, shared databases, daemon listeners or namespace configuration. No shared-host privilege boundary is expanded.

Outbound routing follows the API process namespace. Non-default management VRF binding remains an explicitly unimplemented product follow-up, not a successful lab acceptance. IPsec event adapter and actual receiver/browser/lab acceptance also remain incomplete; Telegram was removed by explicit owner instruction. This is approval of the bounded SMTP/webhook slice, not full original feature completion.

Independent verification from this reviewer's `apps/api` worktree using installed dependencies (no network, live VPP or external receiver):

```text
node node_modules/vitest/vitest.mjs run src/features/notifications/notifications.test.ts src/features/notifications/transport.test.ts --maxWorkers=2
Test Files 2 passed (2)
Tests 57 passed (57)
Duration 4.39s
```

Includes API-only desired projection, rule revalidation, owner-approved event routing, DNS destination filtering, and owned socket cancellation. Tests use mocks; this does not certify real SMTP message reception or data-plane routing. Initial dependency-link setup pointed at the wrong dependency directory and failed before test collection; correcting local links produced the execution above. No product edit or gate alteration was needed.

Verdict: **APPROVE** for R4 bounded scope. No complete quick gate was duplicated; unchanged hosted gate on final product remains mandatory.
