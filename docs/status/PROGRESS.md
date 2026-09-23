# Progress

Updated 2026-09-23 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 1.4% by hours (12/845 h), 3.8% by tasks (3/78)**

| state | tasks |
|---|---|
| merged | 3 |
| review | 0 |
| running | 7 |
| ready | 0 |
| parked | 1 |
| failed | 0 |
| todo | 67 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 6 / 68 | 8.8% | 2/8 | 6 | 0 | 0 |
| S2 | 0 / 215 | 0.0% | 0/16 | 1 | 0 | 0 |
| S3 | 0 / 16 | 0.0% | 0/1 | 0 | 0 | 0 |
| S4 | 0 / 372 | 0.0% | 0/36 | 0 | 0 | 1 |
| S5 | 0 / 120 | 0.0% | 0/12 | 0 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- P02a — Schema group (a): system, dataplane, interfaces, vrfs, routing, management + primitives/diff/merge-patch/gen (running, desktop-agent slot1)
- P02b — Schema group (b): nat, objects, acl (running, desktop-agent slot5)
- P02c — Schema group (c): vpn, tunnels, services, ha (running, desktop-agent slot6)
- P03 — gRPC contract agent↔api (messages for all domains, RPC semantics doc) (running, desktop-agent slot7)
- P04 — Lab tooling (local mode for vrx-a), veth/netns packet rig, binapi for all plugins, local postgres/valkey (running, desktop-agent slot3)
- P09 — CI gate for local-only git (tools/ci.sh quick|full|--base, hooks, golangci-lint, gitleaks) (running, desktop-agent slot4)
- P07a — UI shell: theme/RTL, i18n, SchemaForm, ServerDataGrid, WS hook, frame skeleton (running, desktop-agent slot8)

## Parked

- P12 — parked_on: handover
