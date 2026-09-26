# F-nat44-ed-sessions — review

Reviewer: vrx-bot (review agent), 2026-09-24. Branch `task/F-nat44-ed-sessions` @ `acc1877`. The diff base is the old W-seed
tip `df67a8e`, which the branch merged. `task/W-seed` was re-cut during this review as `a303f0b` on main after the P08
squash, so `task/W-seed...task/F-nat44-ed-sessions` now also shows P08 and W-seed history.
Read: 00-CONTEXT, REVIEW-PROMPT, the feature prompt, the envelope, wave-A-hotspots (§0, §2), wave-BC-numbers (NAT rows), the
status, questions (Q1–Q10) and contract files, the whole diff (78 files), and the VPP 26.06 sources in `/root/vpp`
(`nat44_ed_api.c`, `nat44_ed.c`, `nat44_ed_out2in.c`) for the dump, delete and worker behaviour. No host runs.

**Verdict: APPROVE WITH CHANGES.** The architecture, contract, restart safety, rollback and evidence are sound. Before
merge, fix one bounded-cost defect (H1) and one kill defect (M1). The other findings can go in the same fix round or
into tech debt.

## Findings (ranked)

### H1 — the summary and the filtered session scans cost VPP O(users × sessions) and are polled every 5 s (fix before merge)
- `apps/agent/internal/actions/nat44-ed-sessions/summary.go:86-110` (`Summarize`) and `sessions.go:176-195` (filtered `List`)
  call `UserSessions(ctx, u, 0, 0)` once for **every** owned user. In VPP 26.06, `vl_api_nat44_user_session_v3_dump_t_handler`
  (`nat44_ed_api.c:1659-1690`) runs `pool_foreach` over the **whole** session pool of the worker to find one address. None of
  the nat44-ed API handlers is mp-safe (no `mp_safe`/`thread_safe` in the plugin), so each dump holds the worker barrier. A
  summary therefore costs U barrier syncs × S sessions. The 200 000 scan cap counts only the sessions returned, not what
  VPP iterates. Example: 5 000 users and 100 000 sessions give about 5·10⁸ pool iterations per call.
- The UI polls this every 5 s: `apps/web/src/domains/firewall/nat44-ed-sessions/queries.ts:62-68` (`useNatSummary`,
  `refetchInterval: NAT_POLL_MS`) runs on the **default** Outbound tab and on Pools. `SessionsTab.tsx:288` re-runs a
  session-level filter (port/protocol/outside/external) every 5 s. Every open browser tab adds its own load. On a real box
  this means repeated data-plane stalls. The 2-user rig (2 105 sessions) cannot show it.
- The unfiltered page is fine: one user dump plus the one or two users that cover the page.
- **Fix** (all in owned files, no contract change):
  (a) Bound the per-user dumps per call, not only the returned sessions. Example: `MaxUserDumps = 64` for the summary
      breakdown and 256 for a filtered scan, then set `truncated`. The users/sessions/static totals keep coming from the
      single `nat44_user_dump`.
  (b) Cache `NatSummary` in the agent for ≥ 30 s behind a single flight (`retrieved_at` already tells the UI how old it is).
  (c) UI: poll the summary at ≥ 30 s and only while Pools/Outbound is visible. Do not auto-refresh a grid with a
      session-level filter (manual refresh button).
  (d) Add a unit test on the fake: N users, assert at most `MaxUserDumps` `nat44_user_session_v3_dump` calls, and assert
      `truncated`.

### M1 — killing a twice-NAT session always returns 404; non-TCP/UDP/ICMP rows are killed as `tcp`
- `nat44_ed_del_session` (`nat44_ed.c:3599-3636`) ignores `is_in`. It looks up `init_ed_k(addr, port, eh_addr, eh_port,
  fib, proto)` in the flow hash, which is the session's **i2o** match. For a twice-NAT session that key's remote end is
  `ext_host_nat_addr:port`, not `ext_host_addr:port`. In `nat44_ed_out2in.c:421-456` the twice-NAT branch calls
  `nat44_ed_alloc_i2o_*`, and that function builds the i2o flow on the allocated twice-NAT address:
  `nat_6t_i2o_flow_init(…, i2o_addr, i2o_port, a->addr, …)` at `:276`. `killBodyOf` (`apps/web/src/domains/firewall/nat44-ed-sessions/model.ts:111-122`) always
  sends `externalAddress/externalPort`. So every kill of a twice-NAT row returns "no such session" (404), although the
  session exists. Twice-NAT static mappings and pools are in scope and projected by this task.
- The same function maps any other protocol (for example `"47"`) to `'tcp'` (line 113-114). That sends a kill for a
  different 5-tuple.
- **Fix:** in `killBodyOf` use `twiceNat ? externalNat* : external*`. Disable the kill button, or leave it out, for
  protocols other than tcp/udp/icmp (the API `KillBody` enum already refuses them). Clarify
  `NatSessionKillAction.external_address` in `dataplane.proto:3614` (a comment-only change: "the remote end as the inside
  host addresses it: `external_nat_*` for a twice-NAT session"). Add an agent unit test that the kill of a twice-NAT row
  from the coretest model uses the NAT'd external end.

### L1 — `NatSession.external_nat_*` doc does not match VPP
`dataplane.proto:3514` says "equal to external_* without twice-NAT". VPP fills `ext_host_nat_*` only for twice-NAT sessions
(`nat44_ed_api.c:1612-1615`). Run 6 shows `"externalNatAddress":"0.0.0.0","externalNatPort":0` for the PAT row. The API
fake (`features/nat44-ed-sessions/fake.ts`, `fakeSession`) copies external, so the fake and the real agent disagree.
**Fix:** in `rpc_nat44_ed.go:129` set `ExternalNat* = ExtHost*` when `!r.TwiceNAT` (this matches the comment), or change
the comment and the fake. Prefer the agent side, because M1's `killBodyOf` can then always use `externalNat*`.

### L2 — the VPP dump quirks break the counts-based pager for overlapping inside addresses (multi-VRF) and for LB sessions on multi-worker
- Q6 is real but the pager does not tolerate it. `nat44_user_session_v3_dump` matches the address only, and its details
  carry no FIB. With the same inside address in two VRFs, `listByCounts` (`sessions.go:207-226`) takes up to the page's
  remainder from each user's dump. It then shows every session twice, and the rows of VRF B carry VRF A's label, so a kill
  on them returns 404. Overlapping tenant address space is the main multi-VRF NAT use case.
- On multi-worker, `nat44_user_dump` emits one row per (thread, user) (`nat44_ed_api.c:1447-1455`), while the v3 dump reads
  only the in2out worker (`:1676-1681`). LB-mapping sessions on other workers are counted but never listed, and a user can
  appear twice.
- **Fix (cheap mitigation):** merge `ownUsers` rows by (vrf, ip) and sum the counts. In `listByCounts`, take at most
  `count − skip` rows from one user's dump.
- **Record** a `### V-new (F-nat44-ed-sessions)` item: the v3 dump should also match `s->in2out.fib_index ==
  ukey.fib_index` and walk all threads. This is a one-line C change, VPP-code track only. The user page should say
  "overlapping inside addresses in several VRFs: the session browser can mislabel their VRF".

### L3 — the per-RPC `nat44ed.New(s.vpp, s.owner)` builds a second plugin with an in-memory claim store
`rpc_nat44_ed.go:118,198,225`. The helpers never touch claims, so this is harmless today. It still breaks the letter of
"no second claim store / no in-memory default" (D-080, the review 3.2 checklist line), and it constructs 11 descriptors
per call. `Service` has no Wiring handle (A5 read-only), so I accept it for now. **Tech debt:** when A5 gets a seam,
expose the plugin returned by `nat44ed.Register` through `subsystems/nat44_ed.go` (`Wiring.Nat44ED()`) and use it.

### L4 — a filtered scan holds a whole user's session list in memory
`UserSessions(u, 0, 0)` returns every session of that user before the filter runs. The size is bounded by one user's
sessions (no per-user limit in ED, up to the per-thread limit, around 15 MB at 64 k sessions). The cap is checked only
between users, so a scan can pass it by one user. **Tech debt (DF-3 gap-only):** add a streaming
`EachUserSession(ctx, u, func(Session) bool)` so a scan keeps only the page. H1(a) limits how often this happens.

### L5 — merge mechanics: the branch sits on the old W-seed history
The branch merged `df67a8e` (P08's original commits plus the old anchors). Main now carries P08 as a squash (`a5cbf58`) and
W-seed was re-cut as `a303f0b`. **Before merge,** rebase or re-merge onto the new `task/W-seed`/main. Do not bring the
un-squashed P08 history into main (D-112). I compared the hotspot files between `df67a8e` and `a303f0b`. The only
differences are `service_test.go` (TD-12's `waitForSubscriber`, in hunks next to but not on this task's 4 lines),
`app.module.ts` (UsersController) and `nav.ts` (`isCollapsible`). The anchors are identical, so the rebase is mechanical.
Re-run the gate after it.

### L6 — status file: the pasted topology block is run 5 with run-6 notes
The status file presents it that way, and I checked the run-6 log `/root/ngfw-wt/logs/F-nat44-ed-sessions-topo-6.log`
against the claims. Please paste the run-6 lines themselves (same content, 19:40 timestamps, 7 339 / 73 040 B messages) so
the pasted evidence and the final code match one to one.

## Focus checks (all pass unless noted above)

| check | result |
|---|---|
| `desired/nat.go` projects onto DF-3 descriptors | ok. `mode: ed` + `Nat44Enabled` (D-062 mirror, tested) → the 10 nat44-ed descriptors via `natcommon.Encode`. Interfaces are referenced by name, and DF-3 adds `interface/<name>` deps. VRFs go through `vrfID`. `external.pool` resolves to the range's first address or the interface. Pool > 1024 addresses → error with a pointer. `mode: ei`, nat64/66, nptv6 are in the EI anchor group, det44/dslite/map/cnat + the pnat call site are in the CGNAT group, and ipfix sits outside both (envelope obligation met). `AssembleNat` is canonical (sorted, labels never invented) with a round trip on the coretest model as owner and as non-owner |
| persisted `KeyedClaims("nat")`, globals flag | ok. `subsystems/nat44_ed.go:39-46` passes `WithGlobalsOwner(env.GlobalsOwner)` and `WithClaims(w.KeyedClaims("nat"))`, the persisted `claims-nat-<owner>.json`. That satisfies the checklist line "every Register in the A1 hunk passes a Wiring store". `Domains["nat"]` has the two sibling anchors |
| declarative Retrieve | ok. `projection.go` gets one call in `project()` and one in `assemble()`. The run-6 Retrieve equals the canonical desired (proto.Equal) on the slot. An empty NAT domain stays unset (decision 5) |
| contract additive, §2 numbers | ok. `rpc NatSessions`/`NatSummary` under the anchor; `ActionRequest.nat_session_kill = **5**` (§2 and wave-BC-numbers agree: 5 = F-nat44-ed); messages only in the `// ----- F-nat44-ed-sessions -----` section; no `NatConfig` number used; no existing field renamed or renumbered. Commits `bd4e9e3 contract(proto)`, `a7d7469 contract(schema)`, `2b6b70d contract(api-client)` + `-contract.md`. The guard with main's ci.sh passes (below) |
| kill action | 5-tuple required (ED semantics, correct: `nat44_ed_del_session` needs the external end). Audited by the global interceptor with resource `nat/sessions/<proto>/<in>/<ext>/<vrf>` and before/after; the 200 and 404 audit rows are in run 6. Authz: POST defaults to `operator` (`auth.guard.ts:11`), and the UI disables the button without edit rights. Scope: a slot kills only 10.N/16 users. No shell anywhere: API → gRPC → binapi `nat44_del_session`; the topology tests use `exec.Command` with fixed args. Defects: M1, L1 |
| paging bounds | ok for the message size: `limit` ≤ 1000 in the agent (INVALID_ARGUMENT, shown at 1001), the API (`pageSize` max 1000) and the fake; run 6 gives 7 339 B at 100 and 73 040 B at 1000; `page` ≤ 4·10⁶ keeps `offset` inside uint32; unfiltered paging skips whole users by count. Scan cap 200 000. VPP-side cost and memory: H1, L4 |
| fake-agent `destroy` fix (Q2.3) | **correct.** In @grpc/grpc-js 1.14.5 `ServerWritableStreamImpl` (`build/src/server-call.js:121-162`), the `'error'` listener sets `pendingStatus` and calls `end()`, and `_final` then runs `sendStatus`. `destroy(err)` tears the stream down without `_final`, so no status is ever sent (the client hangs until the deadline, which matches the worker's 504). `emit('error', err)` is the documented pattern. Keep it; F-vrf-static-ecmp keeps the dispatch |
| D-128: no `show trace`/`trace add` | ok. `grep -rniE 'show trace\|trace add\|clear trace\|trace filter\|pcap trace\|\btrace\b'` over `test/topology/nat44-ed-sessions`, the agent/API NAT code and the e2e finds only the two "never trace" comments (`vpp_test.go:4`, `nat_test.go:420`). The run-6 log contains no trace command. Port-forward proof = the static session in `vppctl show nat44 sessions` + the API row + tcpdump in the lan netns |
| evidence | verified in the run-6 log: NRestarts 1 → 1; tcpdump PAT `10.4.2.100.40001 > 10.4.2.2.8000` (never 10.4.1.2); port forward wan → `10.4.2.110:8080` reaches `10.4.1.2:80` (tcpdump lan, static session "o2i 10.4.2.110 … 8080", API `static: true`); 1:1 both directions; kill → "Showed: 0" in vppctl, second kill 404, audit rows success/200 + failure/404; restart simulation → NAT back in 0.22 s (reconcile 0.118 s); rollback → 0 slot objects and the plugin still enabled (D-071); 2 105 sessions, `pageSize=100` gives 100 per page. The screenshots (en + fa/RTL, 2 101 live sessions) are in the status file |
| shared hunks | all under own anchors, with the four out-of-anchor edits declared in Q2 (below) |
| i18n | en/fa `nat44-ed-sessions.json` have identical keys (126 = 126); no `margin-left/right`, no TODO/mock/stub in the NAT UI |
| scope | no creep. `coretest` `extensions` seam and `docs/vpp-code-track.md` V-new are declared |

## Recommendations on the open questions

- **Q2 (out-of-anchor edits): keep all four.** (1) The `service_test.go` assertions derived from `implementedDomains()`
  are the only form that survives every wave-A domain merge. (2) Adopt the `coretest.extensions` seam as the official A6
  seam: every later domain model registers in its own `coretest/<slug>.go` `init()` instead of editing `New()`.
  (3) The fake-agent fix is correct (above). (4) The `_` → `stream` rename merges identically if the siblings make it.
- **Q3 (drift for NAT lists): accept (c) for this task, and open a tech-debt item for option (d).** The noise is wider
  than labels. The globals-owner Retrieve also emits `enabled: true`, `forwarding: false` and all four timeouts
  (`TestAssembleNatEmptyAndDefaults`), which a running document that relies on D-062 or omits defaults does not carry. So
  the product box will show `/nat` drift even with no labels. Option (a) needs identity-keyed list matching per domain in
  the API. (b) needs A5. **(d):** let the agent canonicalise the running document itself, e.g. a DryRun field
  `canonical_desired` = `assemble(project(desired))`, which this task's builder and assembler already compute. The API
  then diffs canonical against Retrieve for every domain. That is one contract field and one generic change, and it fixes
  labels, defaults and ordering for NAT and every later domain.
- **Q4 (interface pool keyed by interface only): do the DF-3 gap fix, but not in this task.** Key the twice-NAT variant
  `<if>/twice-nat` and keep plain `<if>` for normal pools, the same scheme as `address-pool`'s `PoolID`. Existing normal
  interface pools keep their key, so nothing is recreated: the recreate concern only applies to twice-NAT interface
  pools, and those cannot exist today because they collide. Assign it to F-nat44-ei-64-66-nptv6, which touches DF-3's
  NAT families anyway, or to a TD. Until then, the 400 (`agent.duplicate-object` with a pointer) is an acceptable
  behaviour.
- **Q5:** agreed. The A4 dispatch owner (F-vrf-static-ecmp) should add a per-member fake dispatch that `fake.ts` exports
  into.
- **Q7 (rig tx checksum offload): adopt it in `tools/lab rig up` for every slot (manager).** Run `ethtool -K <peer> tx off`
  on the **netns-side** veth ends only, never on the VPP-side `host-*` ends or host NICs. It is an af_packet lab-path
  artefact (partial checksums rewritten by NAT), so do not change product code. Keep the V-new so it is re-checked on
  the DPDK path. Until tools/lab has it, the topology test's own two `ethtool` lines (`rig_test.go:74-75`) are fine.
- **Q9:** superseded by D-127 (main 7edac8c). The gate below uses main's ci.sh.
- Decisions 1–9 in the status file: agreed, including decision 2 (a pool without `vrf` = table 0). VPP prefers
  `a->fib_index == rx_fib_index` and falls back to `~0` "any" (`nat44_ed_in2out.c:140-205`). So an inside interface in a
  non-default VRF gets no translation from a pool without `vrf`, and the user page states this (`nat44.md:19`).

## CI (reviewer run, main's ci.sh)

```
$ git -C /root/ngfw-wt/F-nat44-ed-sessions show main:tools/ci.sh > /tmp/g-rev4/ci.sh     # main @ f603980 (D-127 guard fix)
$ cd /root/ngfw-wt/F-nat44-ed-sessions && TMPDIR=/tmp/g-rev4 /tmp/g-rev4/ci.sh --base main
branch    task/F-nat44-ed-sessions @ acc1877   (base: main)
== contract guard: HEAD vs main ==
ok — contract commit(s) on the branch:
  2b6b70d contract(api-client): nat44-ed-sessions routes (Nat44EdSessions_sessions/_summary/_kill), regenerated; CLI operation table
  a7d7469 contract(schema): nat adjacent pools
  bd4e9e3 contract(proto): nat sessions
  (+ the P08 / W-seed contract commits still on the branch, see L5)
== generate + generated-output gate ==            (clean)
== forbidden patterns (+ gitleaks) ==             ok: no kill-by-pattern · no secret-shaped strings · gitleaks no leaks found
== lint · typecheck · unit tests · build (turbo) == Tasks: 30 successful, 30 total (24 cached) · 2m37s
== apps/agent: make lint test build ==            0 issues · 91 packages ok, 0 FAIL (incl. actions/nat44-ed-sessions, internal/agent)
== apps/cli: make lint test build ==              ok
== test/ Go modules, unit mode ==                 test/topology/nat44-ed-sessions: gofmt ok · go vet ok · ok
== deploy/vpp: shellcheck + apply-startup fake-host harness ==
  scenario 30 failed in the parallel run (lock busy) and passed on the serial rerun (host load; not this branch's files)
  mode quick · wall time 23m16s · logs /root/ngfw-wt/logs/ci/F-nat44-ed-sessions-20260924-224632-3293681
CI GATE PASSED
```
This matches the worker's pasted `quick` run (it failed only in the D-127 guard, now fixed on main). After the gate I
removed the git-ignored build outputs it created (`apps/*/dist`, `packages/*/dist`, `apps/{agent,cli}/bin`). The gate
has to run again after the L5 rebase.

## Required before merge
1. H1: bound the per-user VPP dumps and cache or slow the summary; no 5-s auto-refresh with a session-level filter.
2. M1 (+ L1): kill twice-NAT rows by their NAT'd external end; no silent `tcp` fallback.
3. L5: rebase onto the new `task/W-seed`/main, then re-run `tools/ci.sh --base main`.

L2 (cheap mitigation + V-new), L6 and the Q3/Q4 tech-debt items can go in the same round. L3 and L4 go to tech debt.
