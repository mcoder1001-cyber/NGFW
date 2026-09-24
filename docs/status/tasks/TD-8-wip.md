# TD-8 — WIP

- 18:40 seams wired in code + unit tests green (`go test -race ./internal/agent/ ./internal/subsystems/`):
  Env.Publish → bus (copy, seq cleared), Env.Resync → watchVPP → Service.Resync (coalesced; dropped while VPP is down),
  Env.IDs (ResolveIDScope, fail closed; Wiring.IDRange), S1 dynamic sources (merged into every txn + DryRun, own scoped
  sync, Run loops after the first resync), metrics collectors on /metrics.
- next: docs/agent/README.md coverage table, §11 text, TD-8.md + questions, full agent test/lint, ci.sh.
