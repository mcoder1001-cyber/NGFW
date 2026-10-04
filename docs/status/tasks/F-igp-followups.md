# F-igp-followups: actual remaining source completion

2026-10-04. Branch `codex/igp-followups-20261004`, isolated worktree `/root/ngfw-wt/igp-followups-20261004`, base `d5557440c`.
PR: https://github.com/mcoder1001-cyber/NGFW/pull/164.

The board's historical missing-source list was audited against current main. OSPFv3,
OSPF MD5 rendering, OSPF Event20 and state/UI, IS-IS family switches/password rendering,
RIPng and rip.version, Event21 and state/UI, VRRP RPC/events and D9.2 atomic HA sync
are already integrated. This branch builds the remaining RIPv2 authentication and
repairs missing API password selection for OSPF and RIP.

- Additive `routing.rip.interfaces.<name>.auth` contract reuses the existing
  none/md5, keyId1–255, password keyRef object. RIPng rejects the unsupported field.
- FRR renders stable per-VRF/interface owned key chains and interface authentication
  lines. Keys resolve through the existing transaction-selected sealed cache, are
  bounded CLI tokens of at most16bytes, are marked secret and redacted by the framework.
  Missing/invalid material refuses rendering; changing auth to none/removing it
  removes both chain and interface commands from the complete desired configuration.
- API selects only MD5 OSPF/RIP interface password references, using decoded JSON
  pointer segments so escaped interface names work. Foreign kinds reject before
  database access. Unsupported/non-MD5 leaves never select secret material.
- Existing schema-driven candidate API/form consumes this additive field. en/fa
  guidance and user/CLI documentation explain the supported auth and refusal behavior.

Actual validation:

```text
pnpm --filter @ngfw/schema exec vitest run src/rip-auth.test.ts
Test Files 1 passed; Tests 8 passed
pnpm --filter @ngfw/api exec vitest run src/secrets/secret-delivery.service.test.ts
Test Files 1 passed; Tests 18 passed
pnpm --filter @ngfw/web exec vitest run src/domains/routing/isis-rip/IsisRipPage.test.tsx
Test Files 1 passed; Tests 2 passed
Go tests internal/renderers/frr/rip, internal/renderers/frr/ripng, internal/contracttest
all PASS (task-private TMPDIR avoids host-global /tmp inode exhaustion)
pnpm gen
13 successful; generated sources/client committed
```

First quick failed strict optional indexing in the new schema test; corrected in
007e3fa96. Complete quick gate is still running at original base; manager knows
main's existing schema-default/product-text failures and is fixing those before
sequential integration. No complete CI success or merge claimed in this record.
Root independently reviewed the additive contract/renderer/tests: APPROVE,
contingent complete quick. API followup review requested from root.

CLI publish was attempted immediately and rejectedHTTP403. Authorized GitHub
connector checkpoint refs succeeded: contract `c0f31ba39865f0f4284fc11ca78fe9c27eaed599`,
consumer/form `26db6989b731471fa4e8d7dd0ebeb0b022916395`, API followup
`c3aef64ab7a6e713dc29ecc587d73b83a2cb1717`. Corresponding local coherent commits
are a0f15168a,39fe7ed52,007e3fa96,cb91e06fd; remote history preserves the contract first.

Lab-only remainder: real authenticated routing packets/peer acceptance, daemon
rollback/restart, VPP FIB evidence and browser acceptance on the deployed host.
No production secret/socket privilege change, live MD5 result or laboratory
acceptance is claimed. Existing FRR source already binds its resolver to sealed
cache; the previous blanket missing-source secret-channel note was stale for
IS-IS and did not describe this actual audited wiring.
