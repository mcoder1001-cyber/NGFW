# F-loopback-bvi-gso-lldp-span — contract changes (additive)

Branch `task/F-loopback-bvi-gso-lldp-span` (P08 pattern: contract commits first on the task branch). Numbers from the
envelope (wave-A-hotspots §2 "Proposed for the follow-ons", binding by D-109 e): `Interface.gso` 20, `Interface.mirror`
21, `ServicesConfig.nsim` 9, `rpc LldpNeighbors`. LLDP configuration needs no change (`services.lldp` exists, P02c).

## Schema (`contract(schema): …`)

| where | what |
|---|---|
| `packages/schema/src/domains/ext/loopback-bvi-gso-lldp-span.ts` (new, owned) | `interfaceGsoField` (`z.boolean().optional()`), `MirrorSessionSchema` + `interfaceMirrorField` (≤ 8 sessions; `direction` rx/tx/both default both, `level` device/l2 default device), `NsimSchema` + `servicesNsimField` (`delayMs` 0.001–10000, `bandwidthMbps` 0.001–100000, `packetSize` 64–9000 default 1500, `dropFraction` 0–1 default 0, `crossConnect{a,b}`?, `outputInterfaces[]` ≤ 64), `nsimWheelSlots()`, the D-105 reserved loopback range constants |
| `domains/interfaces.ts` (C1 anchor) | `gso` and `mirror` key lines under `// wave-A: F-loopback-bvi-gso-lldp-span` in `InterfaceSchema` + one import line (the file has no import anchor — named hunk, like F-bridge-l2's) |
| `domains/services.ts` (C1 anchor) | `nsim` key line + one import line (named hunk) |
| `src/index.ts` (C3) | `export * from './domains/ext/loopback-bvi-gso-lldp-span.js'` |
| `semantic/loopback-bvi-gso-lldp-span.ts` (+ test, owned), `semantic/index.ts` (C2) | `loopbackBviGsoLldpSpanValidators`: `interfaces.loopback-bvi-gso-lldp-span-reserved-loopback` (D-105: loop16000–loop16383 refused at `/interfaces/<key>`), `…-gso-interface`, `…-mirror-destination` (exists; ≠ source), `…-mirror-loop` (a destination is not a source), `…-mirror-duplicate` (one session per destination + level), `services.loopback-bvi-gso-lldp-span-nsim-range` (wheel ≤ 2^20 slots, packets_per_drop fits u32), `…-nsim-interfaces` (exist, hardware — no sub-interface —, a ≠ b, unique) |

GSO on sub-interfaces is refused structurally: the key exists only on `InterfaceSchema` (strict objects; tested).
The LLDP "interface exists" rule is P02c's `services.interface-references`; it is tested in my test file.

Shape (defaults as Zod fills them):

```jsonc
"interfaces": { "<if>": { "gso": true?,
                          "mirror": [{ "destination": "<if | gre<N>>", "direction": "both", "level": "device" }]? } },
"services": { "nsim": { "delayMs": 50, "bandwidthMbps": 100, "packetSize": 1500, "dropFraction": 0,
                        "crossConnect": { "a": "<if>", "b": "<if>" }?, "outputInterfaces": [] }? }
```

Examples: `packages/proto/test/fixtures/loopback-bvi-gso-lldp-span-full.json` (valid corpus of the proto round-trip
and the Go drift guard). `packages/schema/examples/` admits only the P02 group prefixes (`examples.test.ts`, not
owned), as F-bridge-l2 found (its Q4); the invalid cases are in `semantic/loopback-bvi-gso-lldp-span.test.ts`.

## Proto (`contract(proto): …`)

| where | what | number |
|---|---|---|
| `Interface` (anchor) | `optional bool gso` | **20** |
| `Interface` (anchor) | `repeated MirrorSession mirror` | **21** |
| `ServicesConfig` (anchor) | `NsimService nsim` | **9** (8 is F-rpf-adl-pbr's `auto_sdl`) |
| `service Dataplane` (anchor) | `rpc LldpNeighbors(LldpNeighborsRequest) returns (LldpNeighborsResponse)` | — |
| `// ----- F-loopback-bvi-gso-lldp-span -----` | `MirrorSession`, `NsimService` (+ nested `CrossConnect`), `LldpNeighborsRequest`, `LldpNeighborsResponse`, `LldpNeighbor` | from 1 |

- `docs/contracts/proto.md` §11: `### F-loopback-bvi-gso-lldp-span: LldpNeighbors` under the anchor.
- `apps/api/src/testing/fake-agent.ts` (P5 anchor): `lldpNeighbors` answers `UNIMPLEMENTED`; the working fake is
  `apps/api/src/features/loopback-bvi-gso-lldp-span/fake.ts`.
- Generated: `apps/agent/gen/**`, `packages/proto/gen/ts/**`, `packages/api-client/src/generated/schema.d.ts`
  (`pnpm gen`), `apps/cli/internal/api/operations_gen.go` + `docs/user/cli/reference.md` (`make -C apps/cli gen docs`).
- Drift guards: `go test ./internal/contracttest` — "930 scalar leaves and 206 messages compared, 4 accepted
  difference(s), 0 finding(s)"; `packages/proto` vitest 72/72 with the new fixture; `buf lint` clean, `buf breaking`
  against main clean.
