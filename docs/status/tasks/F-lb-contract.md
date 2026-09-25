# F-lb — contract changes (additive)

Branch `task/F-lb` (commits `contract(schema): …` and `contract(proto): …`, the P08 pattern; no own branch).
Numbers from `docs/status/wave-BC-numbers.md` § F-lb: **`ServicesConfig.lb = 11`**, new `Lb*` messages (fields from 1),
RPCs `LbState`, `LbFlushVip`. Nothing else: no `ActionRequest` member, no `EventKind`, no existing field touched.

## Schema — `services.lb` (`packages/schema/src/domains/ext/lb.ts`, key line in `domains/services.ts`)

```
services.lb?: {
  settings?: { ip4Source?, ip6Source?, flowBuckets? (power of 2), flowTimeoutSec? }        # VPP-global lb_conf (D-071)
  vips: { <objectName>: {
    prefix (CIDR, host bits 0), protocol: any|tcp|udp = any, port? (tcp/udp only, required there),
    encap: gre4|gre6|l3dsr|nat4|nat6, dscp? (l3dsr), srvType? + targetPort? + nodePort? (nat4/nat6),
    newFlowsTableLength = 1024 (power of 2, ≤ 2^20), srcIpSticky = false,
    servers: [{ address, flushOnDelete = false }]  (itemKey address) } }
  natInterfaces: [{ interface, family: ip4|ip6 }]  (itemKey interface+family)
}
```

Rules (schema refinements, pointers inside `/services/lb`): encap ↔ VIP family (l3dsr/nat4 IPv4 VIP, nat6 IPv6 VIP);
server family = encap family (gre4/l3dsr/nat4 IPv4, gre6/nat6 IPv6 — "GRE4 VIP with an IPv6 AS" →
`/services/lb/vips/<name>/servers/<i>/address`); port ↔ protocol; powers of two; dscp l3dsr-only; srvType/targetPort/
nodePort nat-only, nodePort needs nodeport; NAT VIPs need tcp/udp + port + targetPort and a `natInterfaces` entry of
their family; unique (prefix, protocol, port); VPP's per-prefix rules (all-port xor per-port VIPs, one encapsulation
per prefix — lb.c lb_vip_add); unique servers per VIP; no two NAT VIPs share an (AS address, target port) pair (VPP
keys the SNAT mapping by exactly that pair; V20 follow-up in F-lb.md). Semantic (`semantic/lb.ts`):
`services.lb-nat-interface-exists`.

## Proto (`packages/proto/vrx/v1/dataplane.proto`)

- `ServicesConfig`: `LbService lb = 11;` (under the `// wave-BC: F-lb` anchor).
- `// ----- F-lb -----` section: `LbService{settings=1, vips=2 map, nat_interfaces=3}`, `LbSettings{1–4}`,
  `LbVip{1–11}`, `LbServer{1–2}`, `LbNatInterface{1–2}` (the DesiredState mirror; drift guard 0 findings), and the RPC
  messages `LbStateRequest`, `LbStateResponse`, `LbServerState`, `LbVipState`, `LbFlushVipRequest`, `LbFlushVipResponse`.
- `service Dataplane`: `rpc LbState`, `rpc LbFlushVip` (under the anchor). Semantics: `docs/contracts/proto.md`
  "F-lb: LbState, LbFlushVip".

## Generated / tests

`pnpm gen` output (apps/agent/gen, packages/proto/gen/ts, packages/api-client/src/generated); fixture
`packages/proto/test/fixtures/lb-full.json` (every leaf, round-trips); `apps/api/src/testing/fake-agent.ts` gets the
two UNIMPLEMENTED stubs under the anchor (P5), replaced by `features/lb/fake.ts` in the feature commit.
