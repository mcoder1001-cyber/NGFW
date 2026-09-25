# F-srv6 — contract changes (additive, for the manager's review)

Branch `task/F-srv6`, two commits at the start of the branch (never a `contract/` branch, envelope):

1. `contract(schema): routing srv6` — `packages/schema/src/domains/ext/srv6.ts` (sub-schema), one key line
   `srv6: srv6Field` under `// wave-BC: F-srv6` in `RoutingSchema` (+ one import line at the end of the import block of
   `domains/routing.ts`), one `export *` line in `src/index.ts`, rule file `semantic/srv6.ts` + its import/spread lines in
   `semantic/index.ts`, tests `semantic/srv6.test.ts`, regenerated `packages/api-client/src/generated/schema.d.ts`.
2. `contract(proto): routing srv6, Srv6State` — `RoutingConfig` field **17 `srv6`** (the only allocated number,
   `docs/status/wave-BC-numbers.md` § F-srv6), the `Srv6State` rpc under the service anchor, the messages in
   `// ----- F-srv6 -----`, regenerated Go/TS stubs, fixture `packages/proto/test/fixtures/srv6-full.json`, the
   UNIMPLEMENTED `srv6State` stub in `apps/api/src/testing/fake-agent.ts` (wave-A-hotspots P5).

Config home: **`routing.srv6`** (decided in wave-BC-numbers.md; not `tunnels.srv6`).

## Shape

```
routing.srv6?: {
  encapSource?: ipv6            // default outer source of encap policies; VPP global (globals owner only, write-only)
  encapHopLimit?: 1..255        // VPP global (globals owner only, write-only)
  localSids: { <sid>: { behavior: end|end.x|end.t|end.dx2|end.dx4|end.dx6|end.dt4|end.dt6,
                        psp=false, vrf='default', interface?, nextHop?, lookupVrf? } }
  policies:  { <bsid>: { type: default|spray|tef = default, encap = true, vrf = 'default', encapSource?,
                         sidLists: [{ sids: ipv6[1..16], weight: 1..65535 = 1 }] (1..64) } }
  steering:  [ { type: 'l3', prefix, vrf = 'default', bsid } | { type: 'l2', interface, bsid } ]
}
```

Proto: `Srv6Config{encap_source 1, encap_hop_limit 2, local_sids 3 map, policies 4 map, steering 5 repeated}`,
`Srv6LocalSid`, `Srv6Policy`, `Srv6SidList`, `Srv6Steering` (one message for the discriminated union, proto.md §1),
every scalar `optional` (D-039). State: `Srv6StateRequest{owner}`, `Srv6StateResponse{local_sids, policies, steering,
owner, retrieved_at}`, `Srv6StateLocalSid` (config fields + `good/bad_packets/bytes` uint64 from
`sr_localsids_with_packet_stats_dump`), `Srv6StatePolicy`, `Srv6StateSidList`, `Srv6StateSteering`. Names start with
`Srv6` (wave-A-hotspots §0 rule 5); no EventKind, no ActionRequest member.

## Decisions taken in the contract (also in F-srv6-questions.md)

- **Per-policy `encapSource` override: modelled** (the prompt's open question). D-074 requires every encap policy to
  carry its own source; D-071 makes the global `encapSource` a globals-owner setting whose non-owner variant can never
  be satisfied (write-only → `ErrNotGlobalsOwner`). Without the override a slot agent (and any non-owner) could not
  run an encapsulating policy at all. Resolution order: policy `encapSource`, else `routing.srv6.encapSource`, else
  validation error `routing.srv6-encap-source`. Options: (a) global only (b) per-policy override (c) per-policy only.
  (b) chosen: TNSR parity for the common case, testable on slots, cheap.
- **Steering is an array with a `type` discriminator** (`l3` / `l2`), as the prompt's array shape, with a
  uniqueness rule (`routing.srv6-steering-unique`) — routing.ts' convention for lists (D-045). A discriminated union
  keeps presence clean: an `l3` entry always has `vrf` (default `default`), an `l2` entry never has one, so Retrieve
  compares equal. Retrieve returns the list sorted (L3 by VRF then prefix, then L2 by interface), like
  `routing.static`; the UI saves it in that order.
- **Canonical form is a rule** (`routing.srv6-canonical`): SID/BSID keys, segments, sources, next hops and prefixes
  are written as the agent reports them (lower case, shortest `::` form), so Retrieve and the running document compare
  equal. The UI canonicalises on input.
- **Not modelled**: End.AD/AM/AS proxies (no binary API in VPP 26.06: `src/plugins/srv6-{ad,am,as}` have no `.api`,
  their behaviours are not in `sr_types.api`), SRv6-mobile (D-074, D-085), uSID (`sr_localsid_add_del_v2`), path
  tracing (`sr_pt`). The behaviour enum refuses `end.ad` etc. at the schema.
- `description` leaves are not offered (not VPP state; D-073b would need service-side storage).

## Checks run on the contract

- `packages/schema`: `vitest run src/semantic/srv6.test.ts` — 23 tests (defaults, limits, every rule id).
- `packages/proto`: `vitest run` — 70 tests (the new fixture round-trips `toJSON(fromJSON(parse(doc)))`).
- `apps/agent`: `go test ./internal/contracttest/` — `TestSchemaProtoDrift` PASS against the regenerated JSON Schema
  (both directions: no srv6 leaf without a field, no field without a leaf).
- `buf breaking`: additive only (new field number 17, new messages, new rpc) — run by the CI gate.
