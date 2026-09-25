# Task: F-mpls-ldp — LDP via FRR ldpd + agent-side FRR→VPP label sync (V5 fallback)   (prepend 00-CONTEXT.md)

> Generated 2026-09-24 from `prompts/FEATURE-TEMPLATE.md` (MANAGER-PROMPT §6); refreshed the same day on `task/prep-rest` (contract on
> the task branch, allocated numbers, seam S1, VPP 26.06 linux-cp source facts). Your TASK ENVELOPE (`docs/status/tasks/F-mpls-ldp.envelope.md`)
> wins for branch, files, numbers, anchors and process. This is the LDP part of F-mpls-srmpls, split out by
> D-085. When you start, static MPLS, SR-MPLS, the `routing.mpls` model, the MPLS descriptors, the FRR framework and linux-cp are all
> merged. **This task adds only three things:**
> 1. the FRR `mpls ldp` section;
> 2. the agent-side sync that turns FRR's LDP label state into VPP MPLS label routes;
> 3. the LDP state, screen and docs.

## Goal
LDP (RFC 5036, IPv4) label distribution in FAST MODE. FRR `ldpd` runs the protocol. The agent renders its config, then reads
the resulting label state from FRR JSON. **The agent programs VPP MPLS label routes through its own scheduler.**

This is `docs/vpp-code-track.md` **V5**. linux-nl syncs unicast IPv4/IPv6 only, so the fallback — the agent reads FRR JSON and
calls the VPP API, with no VPP code — is the plan, not a workaround. The result is a VRX that works as an LDP **transit LSR and
penultimate hop**. Reference: FRR ldpd (https://docs.frrouting.org/en/latest/ldpd.html); VPP `mpls` (`mpls_route_add_del`).
WBS D2.8 in `plan/wbs.csv` (T2, "MPLS: label operations, LSP, …"); `docs/08-master-schedule-en.md` §3 D2 "MPLS + SR-MPLS"
(the FRR side is D3's framework).

## Dependencies (must be merged before you start; board: F-mpls-srmpls, RF-1, P12 — plus P08, DF-7, TD-2, TD-3)
- **F-mpls-srmpls** — read its `docs/status/tasks/F-mpls-srmpls.md` first. It provides:
  - the `routing.mpls` model in `packages/schema/src/domains/routing.ts` + proto;
  - the MPLS projection (`apps/agent/internal/desired/mpls_srmpls*.go`, wired from `subsystems/mpls_srmpls*.go`);
  - `mpls-table` / `mpls-interface` / `mpls-route` wired into the `routing` domain;
  - the page `apps/web/src/domains/routing/mpls-srmpls/`;
  - the API module `apps/api/src/features/mpls-srmpls/` with `GET /state/routing/mpls/{fib,tunnels}`;
  - its answer on who declares MPLS table 0 (the globals owner);
  - the anchors it seeded for you: `// wave-BC: F-mpls-ldp` in `MplsSchema` (`packages/schema/src/domains/ext/mpls-srmpls.ts`),
    `MplsConfig` field 10 reserved for `ldp` (its proto section), the LDP tab anchor in `apps/web/src/domains/routing/mpls-srmpls/tabs.ts`.
- **DF-7** (`docs/agent/descriptors/mpls.md`). The facts you need from it:
  - `mpls-route/<table>/<label>/<eos|neos>`: `mpls_route_add_del` (not multipath: the path set is replaced) and `mpls_route_dump`.
  - Paths are `df7.Path`: next hop + interface, out-label stack, weight.
  - Dependencies: `mpls-table/<t>` + `interface/<if>`.
  - **Shared table 0 (review H2):** in table 0, a route is reported, updated or deleted only when this owner recorded it
    (D-080 boot record). Create refuses a foreign label (`dfkit.ErrNotOurs`).
  - Table 0 is VPP-global (D-071). On the shared host it does not exist. It is created only with `VRX_DF7_GLOBALS=1` under
    `flock -x /run/lock/vrx-globals.lock` (D-082).
- **RF-1** (`apps/agent/internal/renderers/frr/README.md`, `section.go`, `state.go`, `events.go`, `frrtest/`):
  - `frr.RegisterSection` from `init()` (order 400–899);
  - `frr.RegisterStateReader` (keys `^[a-z][A-Za-z0-9]{0,63}$`, constant `show … json` only);
  - `frr.RegisterPoller`;
  - `Renderer.ShowJSON` (64 MiB bound → `ErrTruncated`);
  - `rc.MapInterface` (VPP/logical name → Linux name) and `rc.Secret`;
  - the built-in redaction of `neighbor … password`;
  - the convergence check: render exactly FRR's `show running-config` form;
  - the `frrtest` harness (own pathspace, `Options.Daemons`, `Options.NetNS`).

  **Protocol readers never dump the full RIB (review M3).**
- **P12** (read its status file). It provides:
  - LCP pairs;
  - the product `frr.New(... WithInterfaceMapper(<linux-cp mapper>))` wired into commit apply;
  - the netns FRR peer pattern on the veth rig;
  - the way FRR state reaches the API (BGP neighbours);
  - the linux-cp mapper `apps/agent/internal/lcpmap` (VPP → Linux name, injected with `frr.WithInterfaceMapper`).

  Blank-import your section package from your own `apps/agent/internal/subsystems/mpls_ldp.go` (no shared import file).

  You need the **reverse** mapping (Linux name → VPP logical name) from the same LCP pair source. If P12 does not export it,
  add a small adapter in your package over P12's exported data. Do not dump LCP pairs a second way. If you cannot, use the
  questions file.
- **P08:** `subsystems.Register` / `Domains`, and the `Service` transaction lock (`agent/service.go`, `txn`): Apply, resync and
  revert are serialised there. D-063/D-076 reconciler rules; D-080 boot identity.
- **Seam S1 — dynamic desired source** (`docs/status/wave-BC-numbers.md`): the generic "plan and apply a scoped KV set under the
  transaction lock" hook F-igmp-mfib needs too. `agent.go`/`service.go` are agent core (A5): features do not edit them. Use the seam if
  the manager seeded it; if it is not on main, keep your loop behind a small interface in `frrsync/ldp` with a fake apply in the unit
  tests, write the question, and leave the wiring to the manager.
- If a dependency is not merged, read it with `git show task/<id>:<path>` and do not start coding — unless your envelope names that
  branch as a speculative base (D-114).

## Host facts (probed read-only 2026-09-24, re-check)
- FRR 10.7.1 is installed and `/usr/lib/frr/ldpd` exists. Never use the system FRR unit or `/etc/frr`; use only `frrtest`
  in your slot's pathspace.
- Kernel MPLS is **not loaded**: `/proc/sys/net/mpls` is absent, and `mpls_router` / `mpls_iptunnel` are on disk only. zebra
  therefore runs with MPLS disabled and **may not build its LFIB** (`show mpls table json` may stay empty). ldpd's own LIB
  (`show mpls ldp binding json`) does not depend on the kernel. **Never load kernel modules or set `net.mpls.*` sysctls on this
  host** (host change → questions file).
- VPP 26.06 source (read-only, `/root/vpp/src/plugins/linux-cp/`): `lcp_router.c` handles **AF_MPLS** netlink routes, so on an image
  with kernel MPLS linux-nl could sync zebra's LFIB itself — not possible here (no kernel MPLS), which is why the V5 agent sync is the
  path on this host; record it under open question 2. `lcp_mpls_sync.c`: MPLS enabled on an LCP-paired interface is mirrored to the host
  tap by writing `net.mpls.conf.<tap>.input` — expect a failure/log line here; record it, do not work around it. `lcp_router.c` installs a
  (*,224.0.0.0/24) accept mfib entry on LCP pairs and `lcp_interface.c` punts unknown UDP/TCP — LDP hellos (224.0.0.2, UDP 646) and the
  session (TCP 646) should reach the tap: confirm (open question 5).
- ldpd runs as a parent process plus child processes. Check that `frrtest`'s Stop leaves none behind. If it does, stop the
  leftovers in your test cleanup, only by PIDs whose parent is your harness's ldpd **and** whose cmdline names your socket dir,
  and report it in the questions file. `frrtest` is RF-1's code (read-only). Also check that ldpd's control socket lands under
  the harness dirs.

## Contract changes
Make one additive change, committed **first on your task branch** as separate `contract(schema): mpls ldp` and `contract(proto): …`
commits (never a `contract/` branch — workers create no branches) + `docs/status/tasks/F-mpls-ldp-contract.md`, then tell the manager in
`F-mpls-ldp-questions.md` and continue without waiting. The change adds `ldp` inside the existing
`routing.mpls`:
`ldp{routerId, transportAddress, interfaces[<ifName>], neighbors?{<lsrId>: {passwordRef?: "password/<name>"}}, labelRange?{min, max}}`.

- The proto field is `MplsConfig` **10** (allocated in `docs/status/wave-BC-numbers.md`; never "next free"); the LDP neighbour
  event is `EventKind` **23**; explicit presence for scalars (D-039).
- Keyed collections are records (D-045/D-053).
- Secrets are D-051 references and are never inline.
- Keep `labelRange` only if FRR 10.7 accepts `mpls label dynamic-block <min> <max>` (probe with `vtysh -C`). Otherwise drop it,
  and document FRR's dynamic range as the rule.
- Do not rename or reshape anything F-mpls-srmpls added (always-PENDING).

## Scope — build exactly this
1. **Schema** (new file `packages/schema/src/semantic/mpls-ldp.ts` + test):
   - `routerId` and `transportAddress` are IPv4;
   - `transportAddress` is an address configured on a default-VRF interface or loopback;
   - every `ldp.interfaces[]` exists, is in the default VRF and is listed in `routing.mpls.interfaces` (MPLS-enabled);
   - neighbour keys are IPv4;
   - `passwordRef` has the form `password/<name>`;
   - when LDP is enabled, no static label route / SR BSID in table 0 lies inside the LDP dynamic label range.

   Pointers go to the offending entry.
2. **FRR section** `renderers/frr/ldp`:
   - Render `mpls ldp` / `router-id` / `neighbor <lsr> password <rc.Secret>` / `address-family ipv4` /
     `discovery transport-address` / `interface <rc.MapInterface name>` / `exit-address-family` / `exit`, plus the zebra
     `mpls label dynamic-block` line if it is kept.
   - Use FRR's canonical `show running-config` form; probe it in the harness and record it.
   - An unmapped interface is an error.
   - `RegisterStateReader{Key: "ldpNeighbors", Command: "show mpls ldp neighbor json"}`. **Do not** register the bindings as a
     reader: the whole LIB would go into every Retrieve.
   - `RegisterPoller("ldp-neighbors", …)` → neighbour up/down events.
   - Unit tests: golden renders, hostile names/refs, and redaction of the password in DryRun/errors.
3. **Sync** `frrsync/ldp` (the V5 core):
   - **Probe first.** In the harness with two ldpd instances, record `show mpls table json`, `show mpls ldp binding json`,
     `show mpls ldp neighbor json` and `show mpls ldp discovery json` as redacted fixtures under `testdata/`.
   - Choose the source and write the choice in `docs/agent/renderers/frr-ldp.md`:
     - zebra's LFIB if it is populated without kernel MPLS;
     - else ldpd's in-use remote bindings + the LDP adjacency (next-hop address + Linux interface).
   - **Translate** each FEC with in-use remote binding(s) into one `mpls-route` object (in-label = local label, EOS entry):
     - one path per in-use next hop (ECMP);
     - Linux interface → VPP logical name via the LCP mapping → `interface/<name>` dependency;
     - out-label = remote label; remote implicit-null (3) = pop with no out-label (PHP);
     - local implicit-null (3) = no entry;
     - explicit-null and the non-EOS entry: build only if DF-7 accepts them unchanged; otherwise skip, raise an event and record.
   - **Scope isolation:** the LDP routes use their own descriptor name and records, e.g. a second `mpls-route` instance
     `mpls-route.ldp/<table>/<label>/<eos>` (an additive named constructor in `descriptors/mpls`, gap-only). The instance
     belongs to **no** `Domains` entry, so that:
     - commits never plan or delete LDP routes;
     - the sync never touches static routes;
     - a label collision surfaces as `ErrNotOurs` → sync error + event, never a crash.
   - **Apply path:** the sync hands its full desired set to the agent. The agent plans and applies it under the `Service`
     transaction lock, with scope = that descriptor only. There is never a second writer to VPP and never a write outside the
     scheduler. Poll at 1 Hz, and apply only when the translated set changed.
   - **Dependencies:** check how the scheduler resolves `mpls-table/<t>` / `interface/<if>` dependencies outside the scope, and
     document it. In the product, LDP routes go to **table 0**. Tests use `WithTable(<N>xxx)`, a table in your slot range.
   - **Failure semantics:**
     - a failed read (ldpd down, vtysh error, timeout, `ErrTruncated`) is **never** treated as "empty": keep the last applied
       set and raise a degraded event;
     - after a documented hold-down (constant, default 60 s), flush;
     - a successful read without a binding means withdraw → delete;
     - LDP removed from config → flush.
   - **Commit ordering (V15, D-087 `fib_table_flush` crash):** a commit that deletes the MPLS table, or the interface an LDP
     route depends on, or disables LDP, removes the LDP routes in it **first**.
   - **Input hygiene:** labels in range, addresses parsed, interfaces only through the LCP mapping. An unknown interface means
     skip + event; never create interfaces.
   - Unit tests on the fake client with the recorded fixtures:
     - learn, ECMP, PHP;
     - withdraw → delete;
     - read failure → no change, then flush after the hold-down;
     - collision → error;
     - agent restart → stale recorded route deleted.
4. **API**: config goes through the generic pointer routes. New module `apps/api/src/features/mpls-ldp/` serves
   `GET /api/v1/state/routing/mpls/ldp/{neighbors,bindings,sync}`:
   - bindings are paged server-side;
   - `sync` returns the last sync time, installed routes, conflicts, last error and the source in use;
   - the data comes from the agent over the FRR-state path P12 built. If that path is BGP-specific, add one read-only RPC
     `MplsLdpState` in your contract commits.

   The API never runs vtysh. Neighbour events go on the WS topic. Update the OpenAPI spec and regenerate `packages/api-client`.
5. **UI**: add an **LDP** tab to the MPLS page (one tab entry in F-mpls-srmpls's page) with:
   - the global form (SchemaForm: router-id, transport address, interfaces, neighbours with password reference);
   - a neighbours grid (state, uptime) with live status;
   - a bindings grid (paged);
   - a sync status chip.

   en + fa in `mpls-ldp.json`. Take a screenshot against the real endpoint.
6. **Docs**:
   - `docs/user/routing/mpls-ldp.md`: an LSR example with two neighbours, what is and is not synced (V5 note; no ingress
     imposition in this release), and the CLI equivalent;
   - `docs/agent/renderers/frr-ldp.md`: section lines ↔ schema, the sync translation table, failure semantics, probe facts.

**Files you own:**
- `apps/agent/internal/renderers/frr/ldp/**`
- `apps/agent/internal/frrsync/ldp/**`
- `docs/agent/renderers/frr-ldp.md`
- `apps/agent/internal/subsystems/mpls_ldp*.go` (new file: registration of the LDP-scoped route instance + the sync wiring + the blank import)
- `apps/agent/internal/desired/mpls_ldp*.go`, `apps/agent/internal/agent/rpc_mpls_ldp*.go` (only if needed), `packages/schema/src/domains/ext/mpls-ldp*.ts`
- `packages/schema/src/semantic/mpls-ldp*.ts`
- `apps/api/src/features/mpls-ldp/**`
- `apps/api/test/e2e/mpls-ldp-*`
- `apps/web/src/domains/routing/mpls-ldp/**`
- `apps/web/src/locales/{en,fa}/mpls-ldp.json`
- `docs/user/routing/mpls-ldp.md`
- `test/topology/mpls-ldp/**`
- `docs/status/tasks/F-mpls-ldp*.md`

**Gap-only** (edit only for a proven need, and say so in the PR): `apps/agent/internal/descriptors/mpls/**` (the additive named
route constructor / record scope) and `docs/agent/descriptors/mpls.md`.

**Read-only** (changes go to the questions file):
- `apps/agent/internal/renderers/frr/*.go`, `frr/frrtest/**` (RF-1 framework)
- P12's LCP/BGP packages
- `apps/agent/internal/descriptors/{df7,dfkit,sr_mpls}/**`
- `apps/agent/internal/frrsync/pim/**` (F-igmp-mfib)
- F-mpls-srmpls's projection and page files, except the one tab entry

**Shared hotspots (append-only, one line under your `// wave-BC: F-mpls-ldp` anchor, conflicts resolved by the manager at merge):**
- F-mpls-srmpls' `packages/schema/src/domains/ext/mpls-srmpls.ts` (`MplsSchema.ldp` key line) + its `MplsConfig` in
  `packages/proto/vrx/v1/dataplane.proto` (field 10); your messages go in `// ----- F-mpls-ldp -----`
- `packages/schema/src/semantic/index.ts`, `packages/schema/src/index.ts`
- `apps/agent/internal/subsystems/subsystems.go` (one call into your `mpls_ldp.go`)
- seam S1 (above) — **not** an edit of `apps/agent/internal/agent/{service,agent}.go`
- `apps/api/src/app.module.ts`, `agent.client.ts` / `fake-agent.ts` (with the RPC), `infra/bus.ts` (`mpls-ldp.events`)
- F-mpls-srmpls's MPLS page tab list (`tabs.ts`), `apps/web/src/i18n.ts`
- generated files (`packages/api-client`, proto stubs) — regenerated, never hand-merged

Do **not** create a shared `apps/agent/internal/frrsync/` root package. Keep your loop inside `frrsync/ldp/`.

## Acceptance (paste the evidence)
- [ ] Commit `routing.mpls.ldp` → the rendered `mpls ldp` block equals the harness FRR's `show running-config` (slot pathspace,
      pasted). Rollback removes it.
- [ ] LDP session with a peer ldpd.
  - Setup: the peer runs in `ns-<prefix>-wan` (own pathspace). The VRX-side FRR runs over the LCP pair of `host-<prefix>w0`,
    P12 pattern, `path: af_packet`. If linux-cp does not deliver the LDP hellos (224.0.0.2 / UDP+TCP 646), use two netns
    ldpd instances on a veth plus a test mapper onto the rig interface instead, and file a V1 entry.
  - Evidence: `show mpls ldp neighbor` shows OPERATIONAL, and the peer's loopback FEC is in `show mpls ldp binding`.
  - The sync installs `mpls-route.ldp` in your slot MPLS table: `vppctl show mpls fib <table>` shows local label → out-label
    (or pop) via the peer address on `host-<prefix>w0` (pasted).
  - The peer withdraws → the entry is gone within 10 s.
  - The table-0 variant runs only with `VRX_DF7_GLOBALS=1` under the exclusive globals lock. Otherwise say it was not run.
- [ ] Stop the VRX-side ldpd (your PID): VPP entries stay until the hold-down, then they are flushed (log excerpt with timestamps).
- [ ] Agent-restart simulation: stop your agent, delete the LDP routes via binapi, plant one stale recorded route, start the
      agent → the set is re-derived from FRR within 30 s, and the stale route is deleted (log + Retrieve).
- [ ] An unrelated routing commit plans **no** `mpls-route.ldp` operation (plan output pasted). A commit deleting the table
      removes the LDP routes before the table.
- [ ] An LDP interface not in `routing.mpls.interfaces`, or a static table-0 label inside the LDP range → 400 problem+json
      with `pointer`.
- [ ] UI screenshot (en + fa) of the LDP tab in `docs/status/tasks/F-mpls-ldp.md`.
- [ ] `tools/ci.sh --base main` green in your worktree. Host tests run one package at a time, with the lab lock only during
      runs, and every FRR/ldpd process you started is stopped.

(A packet-level MPLS test is not required. Optional: labelled ping through the rig — say whether you did it.)

## Out of scope (do not build)
- **Use, do not rebuild:**
  - DF-7 `mpls-*` descriptors;
  - F-mpls-srmpls's `routing.mpls` model, projection, `mpls-interface` enable, static label routes, tunnels, IP bindings,
    SR-MPLS, MPLS page and `/state/routing/mpls/{fib,tunnels}`;
  - RF-1's framework and `frrtest`;
  - P12's LCP pairs, FRR wiring and BGP.

  No second MPLS page, no new MPLS descriptor family, no edits to the FRR framework files.
- **Ingress label imposition.** LDP labels pushed onto IP routes / FEC→LSP at the head end are not built: the IP prefix
  belongs to linux-nl (D-072, one programmer per route). Record the probe asked below instead.
- Labelled BGP, L3VPN / VPNv4 / VPNv6 (P12 follow-up), and 6PE/6VPE.
- LDP IPv6 (RFC 7552), targeted LDP, pseudowires (VPWS/VPLS), mLDP, and LDP in non-default VRFs.
- LDP–IGP sync, session protection, graceful-restart tuning, RSVP-TE, and OSPF/IS-IS segment routing (F-ospf, F-isis-rip).
- PIM / multicast sync (F-igmp-mfib).
- Loading kernel MPLS modules or sysctls, and editing `/etc/frr/daemons` or anything under `/etc/frr`, on this host.
- New Prometheus metrics, performance work, any VPP C change, and any VPP restart (D-012).

## Open questions to surface, not to decide silently
1. **Ingress imposition.** Run a read-only `vppctl show fib source` and record whether the API source outranks linux-nl's
   route source. That decides whether a follow-up row can overlay labelled paths on linux-nl's prefixes. Proposed: a separate
   row after this one.
2. **Kernel MPLS on the product image.** If zebra's LFIB is required (probe result), the product image must load
   `mpls_router` / `mpls_iptunnel` and set `net.mpls.platform_labels` (P14/F-hardening-lite). `ldpd=yes` in the product's FRR
   daemons file is P10/P12 packaging — say what the product needs.
3. **MPLS table 0 in production.** The globals owner declares `mpls-table/0` when `routing.mpls` or LDP is non-empty (carry
   over F-mpls-srmpls's answer).
4. **Label range split.** Pick the default between static labels and LDP's dynamic block.
5. **linux-cp and LDP hellos.** Does linux-cp deliver 224.0.0.2 LDP hellos to the tap (the source says the accept entry and the
   unknown-UDP punt exist)? Confirm on the host and compare with F-ospf's 224.0.0.5 finding.
