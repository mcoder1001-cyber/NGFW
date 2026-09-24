# Review request — contracts-v1 (informational; nothing is parked on it)

Tagged by the manager on 2026-09-24 at the merge of P03b (`cbfcdef`), per MANAGER-PROMPT §4. Later additive changes land as
`contract(…)` commits reviewed by the manager; reshaping/renaming after this tag is PENDING (decision-policy #1).

## What is frozen
- `packages/schema` — 13 domains in three groups (P02a system/dataplane/interfaces/vrfs/routing/management, P02b nat/objects/acl,
  P02c vpn/tunnels/services/ha), semantic validators, merge-patch, diff, redaction helpers. Contract docs: `docs/contracts/schema*.md`.
- `packages/proto` — `vrx.v1` API contract (DesiredState + agent gRPC service), explicit presence everywhere (D-039), 64-bit counters as
  strings, secrets only as `<kind>/<name>` references (D-051). `vrx.model.{acl,nat,iface}.v1` = agent-internal factory models (not API).
- Drift guard (`apps/agent/internal/contracttest`): schema ⊆ proto in both directions — 868 leaves / 191 messages, 0 problems,
  4 documented exceptions (shared `Redistribute` message).

## Breaks vs the first P03 merge (`4ec9518`) — all before the tag, field numbers reserved
`SystemConfig.ntp` removed (NTP lives in `services.ntp`, D-050) · `RoutingConfig.prefix_lists/route_maps` moved under `routing.policy`
(D-045, D-070) · `HaConfig.vrrp` became a map on field 2 (D-053) · cnat `side` → `table` (D-043, D-061).

## Where a human reviewer should look first
1. `routing.policy` prefix-list / route-map shape (objects, not lists — protobuf map values cannot be lists; D-070) — P12 is the consumer.
2. Secret handling: `<kind>/<name>` refs, `redactSecrets` on every GET/audit path (D-051, D-070).
3. `vrx.model.*` packages deliberately do not follow the `optional`-everywhere rule (internal, Go only) — P03b-questions.md.
4. Decisions D-039…D-078 in `docs/decisions/LOG.md`.
