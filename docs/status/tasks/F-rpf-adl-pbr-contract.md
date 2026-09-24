# F-rpf-adl-pbr — contract change (schema + proto mirror)

Commit `b7439d4` `contract(schema): urpf, adl, pbr, autoSdl` on `task/F-rpf-adl-pbr` (P08 pattern: no own contract branch)
carries **both** the schema and the proto change (and the regenerated stubs). `63be7be contract(proto): prettier-format the
rpf-adl-pbr fixture` changes no proto — only the JSON fixture under `packages/proto/test/fixtures`; its subject is
misleading (review L6). History is not rewritten; for the squash: the proto change is in `b7439d4`.
Additive only, per the task prompt and `docs/status/wave-A-hotspots.md` (C1–C5, §2 numbers). No RPC is added
(`/state/pbr` is served from Retrieve, see F-rpf-adl-pbr-questions Q4), so there is no `contract(proto): PbrState`.

## Schema (`packages/schema`)
Sub-schemas in the new `src/domains/ext/rpf-adl-pbr.ts`; one key line under this task's anchor in each domain
(`x-vrx-ui` group `rpf-adl-pbr` on all four):

| Path | Shape | Absent / defaults |
|---|---|---|
| `interfaces.<if>.urpf` | `{ ipv4?: loose\|strict, ipv6?: loose\|strict, direction: rx\|tx }` | absent = no check; `direction` default `rx`; a family without a mode is not checked |
| `interfaces.<if>.adl` | `{ ipv4: bool, ipv6: bool, allowVrf?: vrf, defaultAllow: bool }` | absent = off; `ipv4`/`ipv6` default false (off), `defaultAllow` default true; refine: `allowVrf` required when a family is checked |
| `routing.pbr` | `{ policies: record<name, {acl, priority, paths[{address?, interface?, vrf, weight}]}>, attachments[{policy, interface, family}] }` | absent = none; `priority` default 100; `vrf` default `default`; `weight` default 1; `family` default `ipv4`; `paths` 1–255; refine: `vrf` only on paths without an interface |
| `services.autoSdl` | `{ enabled, threshold, removeTimeoutSec }` | absent = off; defaults false / 5 / 300 (VPP's `auto_sdl_config` defaults) |

Names: policy keys are `objectName` (≤ 63, `[A-Za-z0-9][A-Za-z0-9_.-]*`, so no `#`, D-066).

Semantic rules (`src/semantic/rpf-adl-pbr.ts`, one spread line in `SEMANTIC_VALIDATORS`):
`routing.rpf-adl-pbr-acl-exists` (policy ACL in `acl.lists`), `routing.rpf-adl-pbr-path-refs` (path VRF and interface
exist), `routing.rpf-adl-pbr-path-family` (no policy mixes IPv4 and IPv6 next hops), `routing.rpf-adl-pbr-attachment-refs`
(policy and interface exist; attachment family = next-hop family), `routing.rpf-adl-pbr-attachment-unique`,
`interfaces.rpf-adl-pbr-adl-vrf-exists`.

## Proto (`packages/proto/vrx/v1/dataplane.proto`)
Numbers exactly as allocated in wave-A-hotspots §2: `Interface.urpf = 18`, `Interface.adl = 19`, `RoutingConfig.pbr = 11`,
`ServicesConfig.auto_sdl = 8`. New messages in the `// ----- F-rpf-adl-pbr -----` section: `UrpfConfig`, `AdlConfig`,
`PbrConfig`, `PbrPolicy`, `PbrPath`, `PbrAttachment`, `AutoSdlConfig` (every scalar `optional`, D-039). Sub-interfaces are
not mirrored (Subinterface 16–17 stay unused): the contract names `interfaces.<if>` only, and ADL works on physical ports.
`buf lint` clean; the change is additive (`buf breaking` FILE).

## Generated
`pnpm gen` (Go + TS stubs, JSON Schema, OpenAPI, api-client), `make -C apps/cli gen docs`.
