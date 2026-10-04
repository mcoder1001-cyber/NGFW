# Task: F-igp-followups-c — RIPng section, rip.version + RIP auth, IS-IS area/domain passwords + per-family switch, EventKind 21   (prepend 00-CONTEXT.md)

> Split 3 of 6 of board row F-igp-followups (D-173). Needs F-igp-followups-a merged; runs after -b (same FRR daemon owner and the same
> `EventOf` / `FRRDoc` / `routingLeaves` hunks — rebase on -b, add your lines below its lines).

## Goal
Render `routing.ripng`, `routing.rip.version`, `routing.rip.interfaces.<if>.auth`, `routing.isis.{areaPasswordRef,domainPasswordRef}` and
`routing.isis.interfaces.<if>.{ipv4,ipv6}` into FRR, and publish IS-IS adjacency changes as `EVENT_KIND_ISIS_ADJACENCY_CHANGED` (21).
Reference: TNSR "RIP" / "IS-IS"; FRR 10.7 `ripngd`, `version 2`, `ip rip authentication mode md5` + key chain, `area-password`/`domain-password`
(`isis password` is already masked by the built-in redaction), `ip router isis` / `ipv6 router isis`; WBS D2.4.

## Inputs to read first
- `apps/agent/internal/renderers/frr/{rip,isis}/` (F-isis-rip: sections `rip` 420, `isis` 470, reader `isisNeighbors`, poller `isis-adjacencies`), `docs/agent/renderers/frr-{rip,isis}.md`
- `apps/agent/internal/renderers/frr/{section.go,secrets.go,interfacelines.go,frrtest/harness.go}` (`ripngd`/`isisd`/`ripd` are known daemons)
- `apps/agent/internal/subsystems/{frr.go,isis_rip.go}` (`EventOf` :427), `desired/bgp.go` `FRRDoc`/`AssembleFRR`, `agent/projection.go` `routingLeaves`
  (`// wave-BC: F-isis-rip` :451) and the IS-IS OSI-punt warning (S-rva-agent-gates item 5 — keep it)
- `docs/user/routing/isis-rip.md`, `docs/status/tasks/F-isis-rip.md`, `docs/status/tasks/S-rva-agent-gates.md` (semantic rule `routing.isis.l1-l2-circuit` exists)
- `docs/status/wave-BC-numbers.md` routing pack (`ripng` order 430; test secrets `NGFW_TEST_PSK_<id>_<n>`, shortened where FRR caps the length)

## Contract changes
None expected (all fields come from F-igp-followups-a). A proven gap → `contract(…)` commit first + `-contract.md` + questions file; never reshape.

## Scope — build exactly this
1. **ripng section** (new package `renderers/frr/ripng`, order 430): `router ripng [vrf]`, `network` / interface enable, passive interfaces,
   redistribute (connected, static, bgp, isis, ospf6), default metric; golden + error cases (hostile names, non-IPv6 prefix, unknown interface).
2. **RIP**: `version 2` from `rip.version`; interface auth: one `key chain ngfw-rip-<if>` with key 1 from `auth.keyRef` via `RenderContext.Secret` +
   `ip rip authentication mode md5` + `ip rip authentication key-chain`; add a redaction pattern for the key-string line (`RegisterRedaction`) with a test.
3. **IS-IS**: `area-password md5 <secret>` / `domain-password md5 <secret>` from the refs; per interface `ip router isis ngfw` only when `ipv4` and
   `ipv6 router isis ngfw` only when `ipv6` (default both, as today); redaction test for both lines.
4. **Wiring**: `FRRDoc`/`AssembleFRR` carry `routing.ripng` (one hunk, `// wave-BC: F-igp-followups-c`); `routingLeaves` row `ripng` `handled: true` under the
   F-isis-rip anchor; blank import in `subsystems/isis_rip.go`; `EventOf` case `isis-adjacencies` → EventKind 21 (attributes `source=frr, vrf, system_id,
   interface, level, old, new`).
5. **Tests**: unit (goldens, error cases, redaction, `EventOf`); ONE frrtest integration in your slot pathspace (`NGFW_INTEGRATION=1`): goldens accepted by
   FRR 10.7.1 (ripd, ripngd, isisd), frr-reload DryRun empty after Apply, RIPng route exchange with a netns `ripngd` peer (FRR RIB), RIPv2 MD5 with matching
   key exchanges routes and with a wrong key does not. IS-IS adjacencies cannot form on VPP without `lcp.osi-proto` — prove the IS-IS lines by FRR
   acceptance + reader parse only (do not enable the OSI punt).
6. **Docs**: `docs/agent/renderers/frr-ripng.md` (new), `frr-rip.md`/`frr-isis.md` updates, `docs/user/routing/isis-rip.md` (RIPng, RIP auth, IS-IS
   passwords and families) with CLI equivalents, secrets shown as `<redacted>`.

## Acceptance (paste the evidence)
- [ ] `go test ./internal/renderers/frr/rip/... ./internal/renderers/frr/ripng/... ./internal/renderers/frr/isis/... ./internal/subsystems/ -run 'EventOf|Isis|Rip'` green (paste)
- [ ] frrtest: running-config shows `router ripng`, `version 2`, the key chain and the IS-IS password lines, masked in every agent output (paste, trimmed)
- [ ] RIPng peer route in FRR's RIB; RIPv2 wrong key → no routes (paste); rollback removes the sections (Retrieve)
- [ ] agent-restart simulation → config back within 30 s (log excerpt)
- [ ] IS-IS interface with `ipv4:false, ipv6:false` → 400 problem+json with `pointer` through the API (needs -a; paste)
- [ ] `tools/ci-slot.sh --base main` green (tail pasted)

## Out of scope (do not build)
OSPF/OSPFv3 (-b); API state routes and UI (-e); `lcp.osi-proto` / enabling the OSI punt (V-new, a manager decision); RIP state readers (FRR has no
RIP JSON); IS-IS SR/TE, multi-topology beyond the two family switches; BFD on IS-IS; RIP key rotation (more than one key); VRRP (-d).

## Open questions to surface, not to decide silently
Which password forms FRR 10.7.1 accepts for `area-password`/`domain-password` (md5 vs clear, `authenticate snp …`) — record what frrtest accepts, do not
guess; whether RIPng needs `aggregate-address` (TNSR has it) — only with a contract gap via the questions file.

## Files you own
`apps/agent/internal/renderers/frr/{rip,isis}/**`, `apps/agent/internal/renderers/frr/ripng/**` (new), `apps/agent/internal/subsystems/isis_rip.go`,
`apps/agent/internal/subsystems/frr.go` (the `EventOf` IS-IS case only), `apps/agent/internal/desired/bgp.go` (one anchored hunk), `apps/agent/internal/agent/projection.go`
(one `routingLeaves` line), `test/topology/ripng/**` (new; `test/topology/isis-rip/**` belongs to F-isis-rip-host), `docs/agent/renderers/frr-{rip,ripng,isis}.md`,
`docs/user/routing/isis-rip.md`, `docs/status/tasks/F-igp-followups-c*`. List every shared-file line under `## Shared hunks`.

## Rules
- Files you own: above. Everything else read-only; a needed edit elsewhere → `docs/status/tasks/F-igp-followups-c-questions.md`.
- Shared VPP: slot prefix `w<N>` on every object, tables N000–N999 (`eval "$(tools/lab env <N>)"`); never restart or kill VPP; `timeout 10` on every vppctl;
  packet trace banned (D-128). Daemons: frr only (daemon-owner frr), test-scoped frrtest instances in your slot's netns/pathspace under /run/ngfw-test/w<N>/,
  left stopped; never `frr.service`, never `/etc/frr`.
- D-210a: write tests for your change and get them passing in your package (paste output); run them through `tools/heavy.sh` (D-224), e.g. `../../tools/heavy.sh go test ./internal/<pkg>/...` from apps/agent, `tools/heavy.sh pnpm --filter <pkg> …` from the repo root; no full suite, no lint, no other packages' tests.
- Contracts: additive only — `contract(schema|proto): …` commits first, numbers only from `docs/status/wave-BC-numbers.md`; renaming/reshaping = PENDING.
- Secrets: fixture resolver keys only (PENDING-secret-channel open); never a plaintext key in any committed file or log.
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never `tools/ci.sh` directly), commit on your branch,
  `docs/status/tasks/F-igp-followups-c.md` with pasted real output; evidence `.txt` under `docs/status/tasks/F-igp-followups-c-evidence/` (D-175).
