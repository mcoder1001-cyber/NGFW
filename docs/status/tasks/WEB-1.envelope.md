# TASK ENVELOPE — WEB-1 ui-kit SchemaForm gaps (web-ahead track, D-123)
id: WEB-1   branch: task/WEB-1   worktree: /root/ngfw-wt/WEB-1   base: main@16b622a   started: 2026-09-24T18:08
slot: 11 → eval "$(tools/lab env 11)" (only if you start a dev server; unit tests need no slot)
scope (from the architecture audit's web-ahead plan): in packages/ui-kit SchemaForm — presence toggle for optional objects (P08 Q2); widgets port-range, ip-range, datetime, time, timezone, color; LTR rendering for identifier widgets inside RTL (RTL-1); per-path title/help/group/enum translation (I18N-1); itemKey row summaries and a table view for rule-editor lists. Each widget driven by packages/schema metadata (x-vrx-ui hints), never by per-domain code. Demo coverage on the dev SchemaForm demo page. Keep the existing `withDefaults` export and every current SchemaForm behaviour (P07b/P08 tests stay green).
files you own: packages/ui-kit/src/schema-form/** packages/ui-kit i18n/locales/{en,fa}.ts apps/web/src/pages/dev/{SchemaFormDemoPage.tsx,demoSchema.ts} docs/status/tasks/WEB-1*
est/time box: 7 h / 10 h. Rebase onto main (with P08) happens at merge (D-112).
GIT RULE: run git ONLY inside your own worktree (`git -C <worktree> …`); NEVER in /root/ngfw (main — the merger works there).
CI: `TMPDIR=/tmp/g-<id> tools/ci.sh --base main` (short TMPDIR). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to tools/app — never touch them. Stagger heavy test runs (host load; D-121).
Architecture (00-CONTEXT) is non-negotiable: one schema → three consumers (never hand-duplicate a type; UI derives from packages/schema); every UI string through t() with identical en/fa keys; logical CSS only; no UI screen with a stubbed backend is routed; secrets never enter form state, query cache or logs.
WIP commits every 45 min; finish with docs/status/tasks/<id>.md (what / how verified with pasted output / out of scope / questions), all committed, final message = 8-line summary.
never: merge · edit files you do not own · restart/kill VPP · pkill
