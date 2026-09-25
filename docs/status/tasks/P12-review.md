# P12 review — FRR + linux-cp: BGP, policy, linux-cp pairs, RoutingState, /routing/bgp

**Verdict: APPROVE WITH CHANGES.** H1–H3 must be fixed before the merge. After the fix round, a focused re-verify of
H1–H3 and M1 is enough; a full re-review is not needed.

| | |
|---|---|
| reviewed | `task/P12` @ b14d68d1 (main merged twice: TD-8, then TD-11b/TD-4); merge base with main 4f472cc7 |
| reviewer | 2026-09-25, read-only on the product code; no host runs |
| what was run | the Go race tests, the web and API unit tests, the schema test, `buf breaking`, gitleaks (history, squash diff and tree), `merge-tree` against main and against F-vrf, scheduler/renderer probes run through `go test -overlay` (nothing written to the tree), a VPP source read of `/root/vpp/src/plugins/linux-cp` |

The BGP and policy renderers are careful work: every token is validated and hostile input is refused (see §2). The S2 and
S3 seams are clean, the contract is right, and the restart and withdraw evidence is good.

Three things stop the merge:
- **H1:** the single FRR stage object depends hard on every linux-cp pair, so an unrelated edit tears down every BGP
  session on the box.
- **H2:** the gitleaks finding survives the squash.
- **H3:** the headline acceptance (routes in the **VPP** FIB) is still unproven, and the opt-in T2 proof is broken as
  written, so it must not be run.

## Findings

| # | sev | where | finding | fix |
|---|---|---|---|---|
| H1 | **H** | `apps/agent/internal/desired/bgp.go:137-143` (`FRRDependencies`), `subsystems/frr.go:338-344`, `:360-368`; scheduler `reconciler.go:968-996` (`around`) | `frr.config/vrx` hard-depends on `lcp.itf-pair/<n>` for every paired interface. When a pair is deleted or recreated, the scheduler first deletes its live dependents. Deleting `frr.config` means applying the framework-only configuration, which drops **every** `router bgp`, filter and tap address; the stage is then created again. My overlay probe gives: removing one pair → `delete frr.config, delete pair w0, create frr.config`; recreating one paired interface (e.g. an MTU back to its default, `ErrRecreate`) → `delete frr.config, delete pair, delete iface, create iface, create pair, create frr.config`. Effect: an edit to one interface flaps all BGP sessions and withdraws every BGP route. In the product layout (linux-nl listening in the taps' netns), the teardown also strips the tap addresses. linux-nl mirrors that into VPP (`lcp_router.c:660-705`), deleting the operator's addresses from **every** paired VPP interface. The removed pair's interface never gets them back, so the scheduler's verify rolls the transaction back or the addresses stay missing. Untested: T1's pairs are invisible to linux-nl (see Q1), and no test removes or recreates a pair while BGP is up. | Drop the hard dependency. zebra and bgpd accept configuration for an interface that appears later (the restart evidence in step 4 shows exactly this), so no descriptor needs to order the stage after the pairs. If an order is still wanted, use a dependency that does not cascade: the scheduler recreates dependents through optional dependencies too (`reconciler_test.go:583-598`), so an optional dependency is not enough. Add a unit test: remove one of two pairs, and recreate a paired interface, with BGP set; the fake FRR must see an Update and never the framework-only apply. |
| H2 | **H** | `docs/status/tasks/P12-questions.md:167` | The statement in Q16 that "the squash drops it" is wrong. The Q16 text itself quotes the literal (`` Key: "ipv4/default/bgp" ``). `gitleaks stdin` over `git diff 4f472cc7 HEAD` (the squashed commit's content) reports 1 `generic-api-key`. `gitleaks dir` over the branch's changed files in the current tree reports the same line. The history scan also finds 2b14358f at that line, besides 6dff01fc. The merge gate (D-130) would fail on the squashed commit. The claim in P12.md, "gitleaks detect --no-git over every tree P12 touched: no leaks", does not hold for the doc. | Reword Q16 without the `Key: "…"` form (for example "a test literal of the form family/vrf/protocol"). Re-run `git diff $(git merge-base main HEAD) HEAD \| gitleaks stdin --exit-code 1` and paste the output. |
| H3 | **H** | `apps/agent/internal/agent/p12_topology_integration_test.go:391-435`, `:527-537`, `:578-594`, `:616-618`; `test/topology/bgp/run.sh:22` | Acceptance #1 ("routes in the **VPP** FIB, not only in vtysh — the whole point") is **not proven**. Every count in P12.md is FRR's RIB. The T2 path (`VRX_P12_LINUXNL=1`) is broken; see Q1 for the source citations. (a) Step 4 deletes both pairs. That closes linux-nl's socket (`lcp_nl.c:761-770`), and the recreated pairs reopen it in **root**, because the default netns has been restored. From then on the proof is blind. (b) Deleting a pair flushes no routes (`lcp_router.c:1608-1612`). The 100 `lcp-rt-dynamic` routes stay in VPP table 0, so step 4's VPP count passes **falsely** and step 6's "0 in VPP" fails. (c) Those 10.8/16 routes then stay in the **shared** table 0 as `lcp-rt-dynamic` entries. The agent cannot remove them, because its API route source is different, so they stay until a VPP restart. (d) Restore is only `t.Cleanup`: it does not run on the `-timeout 20m` panic, on SIGINT or on SIGKILL. A default netns left set places every later `netns ""` pair from any slot, or from the product, into `ns-w8-frr`. After that netns is deleted, those pair creates fail (`tap.c:499-506`). (e) The globals lock does not exclude other pair creators: `lcp.itf-pair` Create never takes it. | Do not run T2 as written. Take the redesign in Q1, or drop the T2 code and track the FIB proof as an open acceptance item (the manager decides; see Q1). Mark acceptance #1 in P12.md as **open**, not as "pending a window". |
| M1 | M | `apps/api/src/features/vrf-static-ecmp/vrf-static-ecmp.controller.ts:36,65,166,193` (P12's shared hunk) | P12 carries a **stale copy** of F-vrf-static-ecmp (89ccb9b1). `merge-tree main task/P12` is clean, but `merge-tree task/F-vrf-static-ecmp(a1105598) task/P12` gives **9 conflicts**. Five are generated files. The other four are `actions.controller.ts`, `vrf-static-ecmp.controller.ts`, `nav.ts` and `docs/vpp-code-track.md`. In all of them F-vrf's side wins, and only P12's `proto` query field, its `@ApiQuery` and the BGP nav line are re-applied. F-vrf's `RoutesQuery` is now a `.superRefine`d object, so `proto` must go inside it. `pageSize` max 1000 and `ROUTES_WINDOW` are compatible with `PROTO_PAGE_MAX` 100. | Merge order: F-vrf first. P12 then `merge main`s (a worker step, not the merger's, per D-134), regenerates, and only then squashes. The squash subject starts with `contract(proto):` (it touches contract files, D-112). |
| M2 | M | `renderers/frr/escape.go:93-121` vs `packages/schema/src/primitives.ts:214-221`; `renderers/frr/policy/policy.go:230-233` vs `domains/routing.ts:44`; `desired/bgp.go:205-209` | One schema, two rules. Zod `descriptionText` accepts 255 chars, Unicode, `\|`, a leading `!` and double blanks. `frr.Description` accepts ≤ 80 bytes of printable ASCII without those. Route-map `seq` is `uint32` in Zod and 1–65535 in Go. The render check reports these at pointer **`/routing`**, not at the field. Probes: a Persian **interface** description on a paired interface ("لینک اصلی") refuses the whole commit; so do `"uplink \| isp"`, a 95-byte neighbour description and seq 70000. This bites the fa users. | Add Zod refinements, as semantic rules in `semantic/bgp.ts`, for the FRR-rendered descriptions (neighbour, peer group, prefix list, route-map entry, and the description of a paired interface) and for route-map seq ≤ 65535. Also map the render error's path (`interfaces.X.description`, `routing.bgp.neighbors.A.description`) to a JSON pointer instead of `/routing`. Alternatively, do not render interface descriptions into FRR at all (FRR does not need them). |
| M3 | M | `apps/web/src/domains/routing/bgp/api.ts:33-37`, `apps/api/src/telemetry/relay.service.ts:41-43`; `features/bgp/routes.ts:70-90`; `subsystems/frr.go:483-577` | D-132 is met on paper (30 s polling) but defeated in practice. **Every** `routing.events` batch invalidates `/state/bgp`, and batches arrive at up to 1 Hz. `EVENT_KIND_ROUTING_CHANGED` fires whenever any FRR RIB count moves, so under route churn every open BGP screen calls `RoutingState` about once a second. Each call runs 4 `vtysh` spawns plus `lcp_itf_pair_get` and `sw_interface_dump` on VPP. `RoutingState` is not serialised. `annotateFrr` also runs on **every** FIB page with 1–100 FRR routes, even without `proto`: one `RoutingState` per VRF on the page, with up to 100 sequential `vtysh` lookups each. A gRPC error (NotFound or Unavailable) there fails F-vrf's whole page instead of leaving `proto` unset, as its own comment promises. | Invalidate only on `EVENT_KIND_BGP_NEIGHBOR_CHANGED`, or throttle invalidation to ≥ 30 s. Add a one-at-a-time semaphore (or single-flight) around `FRR.State`. Annotate only when `proto` is given (or cache per `retrievedAt`), and catch errors into "proto unset". |
| M4 | M | `packages/schema/src/domains/ext/frr-linuxcp.ts:62-64`; P12-questions Q1/Q2; VPP `lcp_interface.c:268-273` | A pair's `netns` is not only where its tap goes. The pair enters linux-nl's vif table **only if its netns equals VPP's default netns at add time, or both are unset**. A pair created with any other netns is invisible to linux-nl: no routes, addresses, neighbours, admin state or MTU through it. That covers T1's pairs, and any product pair whose `netns` differs from `linux-cp { default netns }`. The UI help ("default: the linux-cp default namespace (where FRR runs)") and Q1's "both layouts work with the same agent code" are right only when `netns` is empty or equal to the default. | Add a semantic warning (or error) and a doc line: `netns` must be empty or equal the box's linux-cp default netns (F-startup-gen owns that value). Correct the Q1/Q2 facts in the questions doc. |
| L1 | L | `docs/status/tasks/P12-questions.md:90` | Q8 says "one page ≤ 1000 prefixes"; the code caps at 100 (`MaxRIBLookups`, `PROTO_PAGE_MAX`). | Fix the text. |
| L2 | L | `docs/status/tasks/P12.md` §1–§5 | The "after 300ms" figures start when the preceding `apply` returns. They measure poll granularity after the reload, not time from the event. Acceptance is still met: link-down BGP < 1 s, withdraw < 5 s. | Say so in the evidence. |
| L3 | L | `subsystems/frr.go:255-264` | After an agent restart `last` is nil, so Retrieve reports `unknown` with an empty document. The first resync therefore always re-applies (an empty frr-reload diff, harmless), and until then the assembled running view has no `routing.bgp`. This is not a D-063 echo: with `last` set, the reported value is checked against FRR's running configuration through `frr-reload.py --test`, so it is verified, not echoed. | Acceptable; one line in `frr-bgp.md`. |
| L4 | L | `subsystems/frr.go:312-323` | The tap gate looks for **any** `tap*`/`tun*` interface in VPP, including other owners' taps, so on the shared host it rarely saves the `lcp_itf_pair_get`. | Harmless; comment only. |
| L5 | L | envelope/prompt | Decision D-137 is cited in the review brief but does not exist in `docs/decisions/LOG.md` (main 56200c3e); the highest entry is D-135. | Manager: log it, or drop the reference. |

## Review dimensions

1. **Architecture.**
   - Node never talks to VPP or FRR: the API uses only `AgentClient.routingState`/`listRoutes`.
   - The agent is declarative. The singleton `frr.config/vrx` renders every registered section and applies them with
     `vtysh -C` then `frr-reload.py --reload`, plus a convergence `--test`.
   - Timeouts are bounded: validate 30 s, reload 120 s, show 15 s, State 20 s, the VPP part 10 s.
   - Failure is side-effect free: the snapshot is restored and reloaded (RF-1).
   - FRR is never restarted; the test checks the PIDs.
   - Retrieve reads FRR's running configuration through `frr-reload.py --test`, and a diff means drift, which triggers
     an Update. That is real drift detection, not a D-063 echo; see L3.
   - Q6 (no dynamic desired source; the route sync is linux-nl's) fits TD-8. Events go through `Wiring.Publish`, and
     restart recovery works without the API (evidence §4).
   - Q5 is consistent only with H1 fixed, and in the product layout FRR plus linux-nl are a second programmer of VPP
     interface addresses; see Q5.
   - S2 `RegisterInterfaceLines`: every line and interface name passes `checkLine`/`IfName`; names are sorted and
     order is deterministic. Good.
   - S3: the table-driven warning (`projection.go` `routingLeaves`) has the wave-BC anchors. Good.
2. **Security.**
   - `passwordRef` is refused at projection time (`routing.bgp-password-unavailable`, at the exact pointer).
   - No resolver is wired in the product.
   - Show, ShowJSON, DryRun and errors are redacted (the secret set plus the `password …` patterns).
   - The MD5 redaction is tested in `bgp/render_test.go:103-130` (redacted files, mode 0640) and in
     `bgp/integration_test.go:197-209` (running-config shows `password <redacted>`; Retrieve and DryRun hold no
     password).
   - Hostile input: every section line passes the framework's `checkLine` (no `\n`/`\r`, no `| `, no bare `end`).
     Names use `^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`, the same as Zod `objectName`. Communities and as-path use strict
     regexes, and as-path has no letters, so it cannot carry `exit`/`end`. Addresses, prefixes and the router id are
     parsed by `netip`. Interfaces and VRFs go through `IfName`/`VRFName`, secrets through `secretValueRe`. I probed
     `"x\nrouter bgp 9"` in a description, a peer group named `"a b"` and an as-path of `"^65000$ exit"`: all were
     refused. **A newline cannot inject a command.**
   - Host state (read-only check): `frr.service` is disabled and inactive; `/etc/frr` still has its 2026-08-26
     package mtimes; `/run/frr` is empty; no `w8` netns is left.
   - frrtest `Options.Instance` stays under `/run/vrx-test/w8/frr-<i>` with pathspace `w8<i>`.
3. **Q1:** see below.
4. **TD-11b/TD-11c.**
   - `frr.config` declares `RecordsNoOwnership`; the pair wrapper declares `CheckPersistent`, through the DF-1 claim
     store.
   - `ProvidedKeys` gives `interface/lcp-host.<host>`; the test is `lcp_test.go:230-241`.
   - The TD-11c allowlist entry is `task/TD-11c:apps/agent/internal/subsystems/creators_guard_test.go:104`. Whichever
     branch reaches main second deletes it (Q17).
5. **D-132.**
   - `/state/bgp` walks no FIB. It reads `lcp_itf_pair_get`, `sw_interface_dump` and 4 scoped `vtysh` shows, bounded
     at 10 s and 20 s.
   - The `proto=` filter is bounded at 100 lookups within 20 s.
   - Nothing is serialised, and the UI's event invalidation breaks the 30 s intent (M3).
   - F-vrf's own FIB walk semaphore still applies to `ListRoutes`.
6. **Contract.**
   - `Interface.lcp` 22, `StaticRoute.tag` 8, `EventKind` 14/15 and `rpc RoutingState` all match the ledger
     (wave-A-hotspots §2 lines 80-85; wave-BC-numbers "Batch-2 follow-ons" line 451). `RoutingConfig` 12 stays
     reserved.
   - No collisions with F-bonding, F-loopback (Interface 20/21), F-kea, F-vrf (NextHop 4), F-neighbors-ra (EventKind
     10) or F-wireguard (EventKind 13). I checked every branch's `dataplane.proto`.
   - `buf breaking --against main`: exit 0. The change is additive.
7. **Shared hunks.**
   - `merge-tree main task/P12` is clean (067370b0).
   - Against F-vrf's tip there are 9 conflicts, all from P12's stale F-vrf copy (M1).
   - P12's own hunks in `projection.go`, `subsystems.go`, `server.go`, `app.module.ts`, `agent.client.ts`, `bus.ts`,
     `relay.service.ts`, `router.tsx` and `i18n.ts` are append-only under their anchors.
8. **Tests and evidence.** See "Tests run".
   - The evidence is credible for the FRR side: two sessions over the linux-cp punt path, 200 → 100 → 50 → 100 → 0 in
     FRR's RIB, no daemon restart, restart recovery in about 3 s with the pairs deleted behind the agent's back,
     link-down reaching the tap and BGP in under 1 s, and rollback leaving `% BGP instance not found`.
   - It proves **nothing about VPP's FIB** (H3).
   - No host reruns (TD-25).
9. **CI red items.**
   - gitleaks: see H2. The squash does **not** clean the tree. The history literal in 6dff01fc does disappear with the
     squash; `refs/archive/P12` keeps it, but the gate scans only `MB..TIP`.
   - The svs declarations (Q15) are now on F-vrf's tip (a1105598). With them overlaid, every touched package is green
     except F-vrf's `TestSvsRangeFromSlot`, which is F-vrf's.

## Q1 — linux-nl FIB proof (`VRX_P12_LINUXNL=1`): reviewer's analysis

Sources: `/root/vpp/src/plugins/linux-cp`, read-only.

**Socket and netns.**
- linux-nl has one netlink socket for the whole VPP.
- It opens with the first pair of the whole VPP (`lcp_nl.c:747-755`), in the default netns of that moment (`:904-917`).
- It closes with the last pair (`:761-770`).
- Changing the default netns later does not move the socket.
- A read error such as ENOBUFS, or a poll error, triggers a resync, which reopens the socket in the **current**
  default netns (`:585-617`, `:975-983`).
- A pair is visible to linux-nl only if its netns equals the default netns at add time (`lcp_interface.c:268-273`).

**What can reach VPP.**
- Routes: kernel table 254/255 goes to VPP table 0; kernel table N goes to VPP table N (`lcp_router.c:916-923`).
- A route is installed only if its next-hop ifindex belongs to a visible pair (`:1114-1116`).
- Exception: blackhole, unreachable and prohibit routes go in from any table of the listened netns (`:1183-1204`).
- Any route in a non-main table creates or locks VPP table N permanently, even when the route itself is skipped
  (`:1355-1366`). That can collide with another slot's table range.
- Addresses, neighbours, admin state and MTU are applied only for visible pairs.
- No dump happens when the socket opens.

**Resync, delete and close.**
- A resync marks and sweeps the addresses and neighbours of **every** pair's phy, not only the visible ones
  (`:719-745`, `:873-898`). On the shared host, a resync while the socket listens where some taps are not deletes those
  pairs' VPP addresses, other slots' included.
- Deleting a pair, or closing the socket, flushes no routes (`:1608-1612`).

**T2 during the window (< 1 s).**
- Any pair created with `netns ""` in that second lands in `ns-w8-frr`. That covers the product agent, P11's fixture
  and other slots.

**T2 after the window.**
- While any pair exists, the socket listens in `ns-w8-frr`. Pairs created later with `netns ""` are "visible" with
  root ifindexes, but nothing is heard for them.
- The step-4 defect and the leaked routes are covered in H3.

**Restore reliability.**
- Restore is `t.Cleanup` only; it does not run on a timeout panic, SIGINT or SIGKILL.
- If the test dies inside the window, nothing detects the leftover default except the next T2 start and the DF-8
  restarttest skips.
- Once `ns-w8-frr` is deleted, `netns ""` pairs fail on the whole VPP (`tap.c:499-506`).

**Recommendation (reviewer): NO to T2 as written.**
1. **Preferred:** prove the FIB on a private VPP (LAB-vpp-per-slot, which is parked on
   PENDING-vpp-host-hardening). No VPP-global changes are needed there.
2. **If the shared VPP must be used**, run it in a manager window with these rules:
   - Stop every other pair creator first: the product stack, P11 fixtures, the DF-8/restarttest host tests and other
     slots' agents.
   - Keep the default netns set to `ns-w8-frr` for the **whole** test, so the recreated pairs stay visible.
   - Restore from a `trap` in `run.sh` and from a standalone restore step, not only `t.Cleanup`.
   - Before the test and after the rollback, assert that no `lcp-rt-dynamic` route in 10.8/16 is left in table 0.
   - Run the rollback **before** any pair is deleted, so linux-nl withdraws the routes while it still listens.
   - Under T2, step 4 must keep one pair, or re-set the default before restarting.
3. **Alternative:** T3 (a root zebra plus a kernel VRF with table 8001) needs no global change, but a root zebra is
   host-wide, so it runs one slot at a time.
4. **Product:** `linux-cp { default netns … }` in startup.conf (F-startup-gen), with pair `netns` empty or equal to it
   (M4).

## Reviewer's recommendations on Q1–Q17

| Q | recommendation |
|---|---|
| Q1 | No to T2 as written (above). Keep acceptance #1 open; the manager chooses (1) or (2). |
| Q2 | Accept `Interface` 22 (`RoutingConfig` 12 stays reserved). Add the netns rule (M4). |
| Q3 | Accept the singleton `frr.config/vrx`, with H1 fixed (no cascading dependency on the pairs). |
| Q4 | Accept: refuse `passwordRef` until PENDING-secret-channel is decided; the redaction is tested. The UI help should say that the field is refused for now. |
| Q5 | Accept (a) for now, with a documented caveat. In the product layout FRR plus linux-nl become a second programmer of VPP interface addresses, and any FRR teardown strips them (H1). F-startup-gen should evaluate `linux-cp { lcp-sync }` (VPP → Linux) so FRR no longer renders the addresses. |
| Q6 | Accept: no agent-side route sync; linux-nl does it. If that ever changes, it waits for TD-8b. |
| Q7 | Accept: render plus a warning. The kernel VRF device is a follow-up row (the agent has no netlink or `ip` today). |
| Q8 | Accept with M3 (annotate only with `proto`, serialise, degrade on error) and L1. |
| Q9 | Numbers verified against both ledgers and every branch. Accept. |
| Q10 | Accept frrtest `Options.Instance`: gap-only, and existing callers are unchanged. |
| Q11 | Accept S2 and S3. |
| Q12 | Noted; matches D-132 and TD-8. |
| Q13 | Closed (dns_plugin; not P12's). |
| Q14 | The CLI owner wires `vrx show bgp summary` to `GET /api/v1/state/bgp` (`Bgp_state`) in a follow-up row. Not a blocker. |
| Q15 | Resolved on F-vrf's tip a1105598 (`svs/ownership.go`); P12 picks it up with the merge in M1. |
| Q16 | Reject "the squash drops it": fix H2 first. After that, accept (as D-067 did for P02c): the squashed tree is clean, and `refs/archive/P12` is outside the gate's range. |
| Q17 | Accept `ProvidedKeys` → `interface/lcp-host.<host>`. Whichever of TD-11c and P12 reaches main second deletes `creators_guard_test.go:104`. |

## Tests run (reviewer)

| what | result |
|---|---|
| `go test -race -count=1` in `renderers/frr/...`, `descriptors/lcp`, `lcpmap` | ok (frr, frr/bgp, frrtest, lcp, lcpmap; `frr/policy` has no test files; it is covered by `bgp/render_test` and the golden file) |
| same for `subsystems/...`, `agent/...`, `desired/...` on the branch as is | red **only** from main's TD-11b guard: F-vrf's svs descriptors lack ownership declarations (Q15); 0 data races |
| same with `-overlay` of F-vrf's `svs/ownership.go`, plus all of `descriptors/...` | `subsystems` ok, all 66 descriptor packages with tests ok; `agent` red only on F-vrf's `TestSvsRangeFromSlot` ("product range {0 0}"); 0 data races |
| `apps/web` vitest `src/domains/routing src/nav` | 3 files, 13 tests passed |
| `apps/api` vitest (unit) | 11 files, 102 tests passed. P12 adds no API unit test; its API is covered by `test/e2e/bgp.e2e.test.ts` |
| `apps/api` e2e (bgp, vrf-static-ecmp) | **not rerun**: it creates and drops the slot database in the shared PostgreSQL, and the reviewer's run was refused. The worker's pasted 4/4 and 5/5 stand. |
| `packages/schema` `semantic/bgp.test.ts` | 8 passed |
| `buf breaking --against /root/ngfw/.git#branch=main` | exit 0 |
| gitleaks: history (`MB..HEAD`), squash diff (`stdin`), changed files (`dir`) | 2 (6dff01fc, 2b14358f) / 1 / 1: H2 |
| probes (overlay, not committed) | scheduler cascade (H1); `CheckFRR` on hostile and mismatched input (§2, M2) |

## Fix round checklist (then a focused re-verify)

1. H1: remove the cascading dependency of `frr.config` on the pairs, plus the unit test described in the table.
2. H2: reword Q16, and paste the gitleaks `stdin` output of the squash diff.
3. H3: fix or remove T2 per Q1, and mark acceptance #1 open in P12.md.
4. M1: after F-vrf merges, `merge main`, resolve the 9 files (F-vrf's side plus P12's three hunks) and regenerate.
5. M2, M3, M4 (each about 1 h), then L1 and L2.

The downstream FRR tasks (F-ospf, F-isis-rip, F-bfd-redistribution, F-mpls-ldp, F-igmp-mfib) can start speculatively on
`task/P12` once H1 is fixed. The S2 and S3 seams they need are final.
