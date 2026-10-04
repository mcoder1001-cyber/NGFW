# Task: F-igp-followups-b — OSPFv3 (ospf6d) section, OSPF MD5 authentication, EventKind 20 OSPF neighbour events   (prepend 00-CONTEXT.md)

> Split 2 of 6 of board row F-igp-followups (D-173). Needs F-igp-followups-a merged (contract). FRR rows run one at a time (daemon-owner frr).

## Goal
Render `routing.ospf6` and `routing.ospf.interfaces.<if>.auth` into FRR and publish OSPF neighbour changes as `EVENT_KIND_OSPF_NEIGHBOR_CHANGED`
(20) in FAST MODE. Reference: TNSR "OSPFv3" and "OSPF authentication"; FRR 10.7 `ospf6d`, `ip ospf authentication message-digest` /
`ip ospf message-digest-key`; WBS D2.3. VPP is untouched by this row except through linux-nl (routes FRR installs).

## Inputs to read first
- `apps/agent/internal/renderers/frr/ospf/{ospf.go,state.go,render_test.go,testdata/}` (F-ospf: section `ospf` order 440, `ip ospf …` interface lines via
  `frr.RegisterInterfaceLines` in `renderers/frr/interfacelines.go`, readers `ospfNeighbors`/`ospfInterfaces`, poller `ospf-neighbors`)
- `apps/agent/internal/renderers/frr/{section.go,state.go,secrets.go,interfacelines.go,frrtest/harness.go}` — secrets resolve only through
  `RenderContext.Secret`; everything returned passes the redactor; `frrtest` starts `ospf6d` when `Options.Daemons` names it
- `apps/agent/internal/subsystems/frr.go` (`EventOf` :427 maps poller events → Event; only bgp/routes today), `subsystems/ospf.go` (blank import)
- `apps/agent/internal/desired/bgp.go` `FRRDoc` :57 / `AssembleFRR` :226 (the F-ospf hunk pattern, accepted by D-173), `apps/agent/internal/agent/projection.go`
  `routingLeaves` :445 (S3 table, `// wave-BC: F-ospf` anchor :449)
- `docs/agent/renderers/frr-ospf.md`, `docs/user/routing/ospf.md`, `docs/status/tasks/F-ospf-host.md` (root vs netns mode, 224.0.0.5 punt, `test/topology/ospf/run.sh`)
- `docs/status/wave-BC-numbers.md` routing pack (FRR section order: `ospf6` 450; test secrets `NGFWTPSKospf<n>`, OSPF MD5 ≤ 16 chars)

## Contract changes
None expected (F-igp-followups-a carries `RoutingConfig.ospf6`, `OspfInterface.auth`, EventKind 20). A gap you prove → `contract(…)` commit
first + `docs/status/tasks/F-igp-followups-b-contract.md` + questions file; never reshape.

## Scope — build exactly this
1. **ospf6 section** (new package `renderers/frr/ospf6`, order 450): `router ospf6 [vrf]`, router-id, areas (stub/nssa as the schema allows),
   `ipv6 ospf6 area/cost/passive/network/hello/dead/priority` interface lines through `RegisterInterfaceLines`, redistribute; strict escaping, every
   hostile/undefined value an error case (mirror the 21 OSPFv2 cases where they apply); golden render; reader `ospf6Neighbors`
   (`show ipv6 ospf6 vrf all neighbor json`) + poller `ospf6-neighbors`.
2. **OSPFv2 MD5 auth** in `renderers/frr/ospf`: `ip ospf authentication message-digest` + `ip ospf message-digest-key <id> md5 <secret>` from `auth.key_ref`
   resolved via `RenderContext.Secret`; `type none` renders nothing; redaction test: the plaintext never appears in Retrieve, State, DryRun diff or an error.
3. **Wiring**: `FRRDoc`/`AssembleFRR` carry `routing.ospf6` (one hunk, `// wave-BC: F-igp-followups-b`); `routingLeaves` row `ospf6` `handled: true` under the
   F-ospf anchor; blank import in `subsystems/ospf.go`; `EventOf` cases for `ospf-neighbors` and `ospf6-neighbors` → EventKind 20 with attributes
   `source=frr, family=ipv4|ipv6, vrf, neighbor, old, new` (no secret, no free text from FRR beyond the state names).
4. **Tests**: unit (golden, error cases, reader parsers on captured FRR 10.7 JSON, `EventOf`); ONE frrtest integration in your slot's pathspace
   (`NGFW_INTEGRATION=1`, frrtest in `ns-w<N>-frr`, peer `ospf6d` in `ns-w<N>-p1` over a slot veth): golden accepted by FRR 10.7.1, frr-reload DryRun empty
   after Apply, OSPFv3 neighbour Full, OSPFv2 MD5 adjacency Full with the same key and stuck below Full with a wrong key, poller event observed.
   VPP FIB evidence only if it fits the F-ospf-host modes (netns mode: FRR RIB; root mode needs the manager's globals window) — else state why.
5. **Docs**: `docs/agent/renderers/frr-ospf6.md` (new), auth lines in `frr-ospf.md`, `docs/user/routing/ospf.md` OSPFv3 + authentication sections with the
   CLI equivalent (the `ngfw` CLI line and the rendered FRR snippet, secret shown as `<redacted>`).

## Acceptance (paste the evidence)
- [ ] `go test ./internal/renderers/frr/ospf/... ./internal/renderers/frr/ospf6/... ./internal/subsystems/ -run 'EventOf|Ospf'` green (paste)
- [ ] frrtest run: `vtysh … show running-config` contains the rendered `router ospf6` block and the md5 lines with the key masked in every agent output; neighbours Full (paste, trimmed)
- [ ] wrong MD5 key → no Full adjacency (paste); rollback removes `router ospf6` and the auth lines (Retrieve, not assumption)
- [ ] agent-restart simulation: stop your agent, start it → ospf6 config back within 30 s, no duplicate events (log excerpt)
- [ ] an ospf6 interface in an undefined area → 400 problem+json with `pointer` through the API (needs -a; paste)
- [ ] `tools/ci-slot.sh --base main` green (tail pasted)

## Out of scope (do not build)
RIPng / IS-IS / RIP changes (-c); API state routes and UI (-e); VRRP (-d); OSPF API state RPC (the RoutingState readers are enough); BFD on OSPF
(F-bfd-redistribution); OSPFv3 authentication trailer/IPsec; virtual links, sham links, multi-instance; a TD-13 Validator for the FRR stage (TD-H27 —
if you add one, it needs a call-local mapper; otherwise leave it); any VPP plugin change.

## Open questions to surface, not to decide silently
Whether linux-cp punts ff02::5/ff02::6 to the tap in netns mode (the IPv6 twin of F-ospf-host Q6) — record what you see, V-new via questions file if not.

## Files you own
`apps/agent/internal/renderers/frr/ospf/**`, `apps/agent/internal/renderers/frr/ospf6/**` (new), `apps/agent/internal/subsystems/ospf.go`,
`apps/agent/internal/subsystems/frr.go` (the `EventOf` OSPF cases only), `apps/agent/internal/desired/bgp.go` (one anchored hunk), `apps/agent/internal/agent/projection.go`
(one `routingLeaves` line), `test/topology/ospf/ospf6*` (new), `docs/agent/renderers/frr-ospf*.md`, `docs/user/routing/ospf.md`, `docs/status/tasks/F-igp-followups-b*`.
List every shared-file line under `## Shared hunks` in your status file.

## Rules
- Files you own: above. Everything else read-only; a needed edit elsewhere → `docs/status/tasks/F-igp-followups-b-questions.md`.
- Shared VPP: slot prefix `w<N>` on every object, tables N000–N999 (`eval "$(tools/lab env <N>)"`); never restart or kill VPP; `timeout 10` on every vppctl;
  packet trace banned (D-128). Daemons: frr only (daemon-owner frr), test-scoped frrtest instances in your slot's netns/pathspace under
  /run/ngfw-test/w<N>/, left stopped; never `frr.service`, never `/etc/frr`. Root-mode FIB steps only inside a window the manager grants (globals lock inside the shared lab lock, D-167).
- D-210a: write tests for your change and get them passing in your package (paste output); run them through `tools/heavy.sh` (D-224), e.g. `../../tools/heavy.sh go test ./internal/<pkg>/...` from apps/agent, `tools/heavy.sh pnpm --filter <pkg> …` from the repo root; no full suite, no lint, no other packages' tests.
- Contracts: additive only — `contract(schema|proto): …` commits first, numbers only from `docs/status/wave-BC-numbers.md`; renaming/reshaping = PENDING.
- Secrets: test keys only `NGFWTPSKospf<n>` from the slot fixture resolver (PENDING-secret-channel is open); never a key in a log, status file or fixture otherwise.
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never `tools/ci.sh` directly), commit on your branch,
  `docs/status/tasks/F-igp-followups-b.md` with pasted real output; evidence logs as `.txt` under `docs/status/tasks/F-igp-followups-b-evidence/` (D-175).
