# F-acl — MACIP / L3 / L4 ACLs, attachments, hit counters, 100k-rule editor

Branch `task/F-acl` (slot 3, worktree `/root/ngfw-wt/F-acl`), base `task/F-object-model@31249d3` (speculative, D-114), with
`task/F-object-model@c3965939` merged (step 0 of the restart) and `main` merged at `6fe374ce` (TD-8 seams, TD-7).
Contract: `410b5471 contract(proto): acl state` (+ `F-acl-contract.md`). Questions and decisions: `F-acl-questions.md`
(Q1–Q13). WIP notes: `F-acl-wip.md`.

## What
- **Contract (additive):** `rpc AclState` — read-only, paged (≤ 1000 rules per message), filter by sequences / hits only;
  per-list acl_index, VPP rule count and summed counters; per **configuration** rule the number of VPP rules it expanded to,
  its status (applied / disabled / schedule inactive / empty) and hits; the ACLs bound per interface in VPP order, other
  owners' included (D-066); MACIP per interface. `docs/contracts/proto.md` §11 `F-acl: AclState`.
- **Agent**
  - `internal/desired/acl.go` — projection: `acl.lists.<name>` → `acl.acl/<name>` with object expansion from the **request's**
    `objects` (review F3; a rule naming an object in a transaction without objects → `acl.objects-required`), FQDN answers
    from the agent's resolver, `ipVersion: any` → IPv4 + IPv6 rules only for families both sides have, ICMP only in v4 /
    ICMPv6 only in v6, `tcp-udp` → two rules, disabled rules and inactive schedules left out (box time zone, Q4), caps 10 000
    VPP rules per configuration rule (at the rule's pointer) and 100 000 per list (at the list's pointer), `log: true` →
    WARNING `acl.log-unsupported`, `reflect` → permit+reflect; MACIP lists (no source prefix → an IPv4 and an IPv6 any rule);
    attachments (zone → each member interface, ordered by attachment sequence, disabled skipped) → `acl.interface-binding`;
    MACIP attachments → `acl.macip-interface-binding`; `acl.stats-enable/global` only for the globals owner (D-071);
    `acl.host*` → `agent.unsupported-field` (F-host-acl-nftables). `AssembleACL`: Retrieve reports the configuration that
    produced exactly what VPP holds (content fingerprint → recorded expansion), otherwise one rule per VPP rule (drift shows).
  - `internal/actions/acl` — expansion record, tracker wrappers of acl.acl / acl.macip-acl (`TrackedACL`, `TrackedMacip`:
    index + fingerprint per list without dumping 100k rules, D-132), AclState runtime (counters from `/acl/<index>/matches`,
    mapped to configuration rules; counters flag read with the read-only CLI `show acl-plugin tables mask`, Q10; bindings
    from `acl_interface_list_dump` / `macip_acl_interface_list_dump`).
  - `internal/subsystems/acl.go` — registration of the six DF-4 descriptors with `KeyedClaims("acl")`, projection
    environment, **re-projection watcher**: every 60 s (`VRX_ACL_REPROJECT_SEC`) an applied rule whose schedule turned on/off,
    and every FQDN change of an object an applied rule uses, asks for a resync of the stored desired state through TD-8's
    `Wiring.RequestResync` (coalesced ≥ 5 s). `internal/agent/rpc_acl.go` — the RPC.
  - `descriptors/acl/ownership.go` — TD-11b declarations (gap edit, Q6). `coretest/acl.go` — the acl plugin in the agent-level
    fake VPP (+ one line in `coretest.New()`, Q7).
- **API** (`features/acl`, `AclController`): `GET /state/acl/lists` (pending marks + live status), `GET /state/acl/lists/{name}/rules`
  (server-side paging/search in sequence order, `size` for appends, per-rule counters for exactly the page's sequences),
  `GET /state/acl/attachments` (VPP bindings incl. foreign ACLs vs the running configuration, `inSync`), `POST
  /actions/acl/import` (text/csv streamed, dry run by default, ≤ 100 000 rows, errors with line/column, unknown-object
  warnings, replace/append), `GET /actions/acl/export.csv`, `POST /actions/acl/lists/{name}/rules/bulk` (enable / disable /
  delete / move to sequence / renumber as ONE candidate edit with a compact audit row). Config goes through the generic
  pointer routes. Fake agent behaviour in `features/acl/fake.ts`. api-client + CLI operation table regenerated.
- **Web** (`/firewall/acl`, sub-worker, reviewed): lists tab with live status, rule editor on `ServerDataGrid` (server paging up
  to 1000, virtualised rows, quick search, hits-only, candidate/running, checkbox bulk enable/disable/delete/move/renumber,
  drag-and-drop reorder onto a row, add/edit dialog with the object picker, CSV import dialog with dry run, CSV export),
  attachments tab (configured + as VPP holds them, foreign ACLs marked), MACIP tab, ADL/Auto-SDL info link
  (`/firewall/adl`, F-rpf-adl-pbr). Counters refresh every 30 s + Refresh (D-132, Q9). en + fa (RTL).
- **Docs:** `docs/user/firewall/acl.md` (stateless vs reflect, default deny, expansion, counters caveat V7, CSV format, REST +
  CLI), `docs/agent/descriptors/acl.md` §"F-acl: the product wiring".
- **Tests:** agent unit (fake VPP), API unit + e2e (slot DB + fake agent), web unit, `test/topology/acl` (real agent + API +
  slot DB + host VPP through the af_packet rig; opt-in scale, editor-scale, screenshot and janitor runs).

## Decisions (options → choice)
| # | decision | options | why |
|---|---|---|---|
| 1 | Counters per configuration rule through a content-fingerprinted expansion record + descriptor tracker | (a) re-project on every AclState (b) dump the ACL each time (c) record + tracker | (b) dumps 100k rules per refresh (D-132); (a) is time-dependent (schedules/FQDN) and could differ from VPP; (c) maps exactly what VPP holds |
| 2 | Retrieve assembles the domain from the recorded configuration that produced VPP's content, else reconstructs from VPP rules | (a) nothing (b) stored desired echo (c) fingerprint attribution | (b) is forbidden (D-063); (c) is verified against VPP content and shows drift when it differs |
| 3 | Counters availability = the VPP flag (read via `show acl-plugin tables mask`), not the role | (a) non-owners always "unavailable" (b) the flag | the numbers are real whenever the flag is on (Q10); V7 has no API getter |
| 4 | Counters polled every 30 s for the visible rows | (a) WS topic (b) poll | a WS topic would poll the agent anyway (Q9); D-132 |
| 5 | Schedules on the box's time zone (`time.Local`) | (a) `system.timezone` (b) local | `system` is not stored by the agent, resyncs could not see it (Q4) |
| 6 | Re-projection via resync of the stored desired state, only when an **applied** rule's schedule/FQDN changed | (a) periodic resync (b) targeted | a 100k list is not re-applied every minute; compares against VPP's state, never a DryRun |
| 7 | Bulk edits and CSV import as own action routes (one candidate edit each) | (a) N pointer PATCHes (b) one edit | 1000 PATCHes of a 100k-rule candidate are unusable; compact audit rows |
| 8 | Candidate `acl`/`objects` read without full-document redaction | (a) `getCandidate()` (b) raw subtrees | the redaction walk cost ~4 s at 100k rules; neither schema has a secret leaf (unit test), nothing else is read |
| 9 | MACIP rule without a source prefix → one IPv4 and one IPv6 "any" rule | (a) IPv4 only (b) both | the schema says "omit = any address" |
| 10 | `log: true` → WARNING, not an error | (a) error (b) warning | the prompt's default; the commit is not blocked |

## Shared hunks
| file | hunk |
|---|---|
| `apps/agent/internal/subsystems/subsystems.go` (A1, under `wave-A: F-acl`) | const `ACL = "acl"` · Domains `ACL: aclDescriptors(),` · Register `if err := w.registerACL(r); err != nil { return nil, err }` — the `acl` key is shared with F-host-acl-nftables (merge: one entry, union of names) |
| `apps/agent/internal/agent/projection.go` (A2) | `if in["acl"] { desired.ACL(p, ds, in) }` in `project()` · `if in["acl"] { ds.Acl = desired.AssembleACL(kvs) }` in `assemble()` |
| `packages/proto/vrx/v1/dataplane.proto` (C5) | `rpc AclState(...)` under the service anchor · messages + enum in `// ----- F-acl -----` |
| `docs/contracts/proto.md` (C6) | `### F-acl: AclState` |
| `apps/api/src/app.module.ts` (P1) | import `aclFeature` · `...aclFeature.controllers,` · `...aclFeature.providers,` |
| `apps/api/src/agent/agent.client.ts` (P4) | `type AclStateRequest, type AclStateResponse,` · `aclState(req)` |
| `apps/api/src/testing/fake-agent.ts` (P5) | one handler line (dynamic import of `features/acl/fake.js`) |
| `apps/web/src/router.tsx` (W1) | lazy route `domainPath('acl')` → `AclPage` |
| `apps/web/src/nav/nav.ts`, `nav.test.ts` (W2) | `'acl'` in `BUILT_DOMAINS` and in the test's `available` list |
| `apps/web/src/i18n.ts` (W3) | en+fa imports, `'acl'` namespace, `en`/`fa` entries |
| **not an anchor:** `apps/agent/internal/descriptors/core/coretest/fakevpp.go` | `v.installACL()` in `New()` (Q7) |
| **not an anchor:** `apps/agent/internal/agent/service_test.go` | the "unimplemented domain" example `acl` → `management` (acl is implemented now) |
| gap edit (owned package) | `apps/agent/internal/descriptors/acl/ownership.go` (TD-11b declarations, Q6) |
| generated (C7) | `apps/agent/gen/**`, `packages/proto/gen/ts/**`, `packages/api-client/src/generated/schema.d.ts`, `apps/cli/internal/api/operations_gen.go` |

## How verified (real output)

### Host VPP — `VRX_ACL_STATS_GLOBALS=1 test/topology/acl/run.sh -run TestACLTopology` (slot 3, 2026-09-25 04:22, NRestarts 1 → 1)
```
acl_test.go:338: systemctl show vpp -p NRestarts (before) = 1
acl_test.go:457: vppctl show acl-plugin acl (this slot's ACLs):
    acl-index 0 count 1 tag {w3-foreign:guard}
              0: ipv4 deny src 10.3.1.2/32 dst 10.3.2.2/32 proto 17 sport 0-65535 dport 9
      applied inbound on sw_if_index: 8
    acl-index 1 count 5 tag {w3:lan-in}
              0: ipv4 permit src 10.3.1.0/24 dst 10.3.2.2/32 proto 1 sport 8 dport 0-255
              1: ipv4 permit+reflect src 0.0.0.0/0 dst 10.3.2.2/32 proto 6 sport 0-65535 dport 80
              2: ipv4 permit+reflect src 0.0.0.0/0 dst 10.3.2.2/32 proto 6 sport 0-65535 dport 443
              3: ipv4 deny src 0.0.0.0/0 dst 0.0.0.0/0 proto 0 sport 0-65535 dport 0-65535
              4: ipv6 permit src ::/0 dst ::/0 proto 0 sport 0-65535 dport 0-65535
      applied inbound on sw_if_index: 8
acl_test.go:458: vppctl show acl-plugin interface sw_if_index 8 acl:
    sw_if_index 8:
      input acl(s): 0, 1                                   ← the foreign ACL stays first (D-066), ours after it
acl_test.go:460: vppctl show acl-plugin macip acl index 0:
    MACIP acl_index: 0, count: 1 (true len 1) tag {w3:wan-l2} is free pool slot: 0
        rule 0: ipv4 action 1 ip 10.3.2.2/32 mac ce:cb:a6:f1:87:e1 mask ff:ff:ff:ff:ff:ff
      applied on sw_if_index(s): 11
acl_test.go:462: vppctl show acl-plugin macip interface:
      sw_if_index 11: 0
acl_test.go:471: vrx-agentctl retrieve -subsystems acl == running acl (1287 bytes of JSON)
acl_test.go:478: /state/drift: subsystems=[interfaces vrfs routing objects acl] changes=0 (none under /acl)
acl_test.go:483: GET /state/acl/attachments → {…"interface":"host-w3l0","swIfIndex":8,"input":[{"aclIndex":0,"name":null,
    "tag":"w3-foreign:guard","foreign":true},{"aclIndex":1,"name":"lan-in","tag":"w3:lan-in","foreign":false}],…,"inSync":true},…}
acl_test.go:507: VPP counters flag (show acl-plugin tables mask): true
acl_test.go:508: vrx-vpp-preflight: exit 0
    V19 pre-flight ok: no classify binding or classify DPO points at a missing table (0 warning(s))
acl_test.go:515: ping -c 5 10.3.2.2 (permitted by rule 10): 5 packets transmitted, 5 received, 0% packet loss
acl_test.go:520: ping -c 3 10.3.2.1 (VPP's wan address, denied by rule 30): 3 packets transmitted, 0 received, 100% packet loss
acl_test.go:525: GET /state/acl/lists/lan-in/rules (after) → {"applied":true,"countersAvailable":true,"items":[{"index":0,
    "live":{"bytes":672,"packets":8,"status":"applied","vppRules":1},…
acl_test.go:538: rule 10 packets 3 → 8 (+5), rule 30 packets 0 → 3 (+3)
acl_test.go:550: commit → 400 {"type":"https://vrx.dev/problems/validation","title":"Validation failed","status":400,"tier":"semantic",
    …"errors":[{"pointer":"/acl/lists/lan-in/rules/4/destination/name","message":"address group 'none' has no members; the rule would match nothing"}]}
acl_test.go:571: loss: acl_interface_set_acl_list host-w3l0 → [foreign 0]; acl_del 1 (lan-in); macip_acl_interface_add_del del + macip_acl_del 0 (wan-l2)
acl_test.go:597: ACLs and bindings back 0.20 s after the agent start (foreign ACL 0 still first)
acl_test.go:601: agent: 2026-09-25T04:22:22.587 acl domain wired
acl_test.go:601: agent: 2026-09-25T04:22:22.599 reconcile start mode=resync
acl_test.go:601: agent: 2026-09-25T04:22:22.647 reconcile done mode=resync status=APPLY_STATUS_APPLIED summary=created:4  unchanged:13
acl_test.go:618: ping after recovery: 3 packets transmitted, 3 received, 0% packet loss
acl_test.go:637: POST /config/rollback/1 → {"status":"applied",…"results":[{"key":"acl.macip-interface-binding/host-w3w0","op":"delete",…
acl_test.go:651: after rollback: vrx-agentctl retrieve -subsystems acl → {} ; host-w3l0 input = [foreign 0] only
acl_test.go:652: vppctl show acl-plugin acl (this slot):
    acl-index 0 count 1 tag {w3-foreign:guard}          ← only the foreign ACL; no w3: ACL, no MACIP
acl_test.go:341: systemctl show vpp -p NRestarts (after) = 1
--- PASS: TestACLTopology (21.18s)
    --- PASS: commit (0.73s) · traffic-and-counters (4.10s) · validation-empty-group (0.14s) · restart-safety (3.26s)
    --- SKIP: scale (opt-in) · PASS: rollback (0.34s) · PASS: cleanup-through-api (0.46s)
```
The counters flag was off after the host's earlier VPP restart; the test switched it on under `flock -x
/run/lock/vrx-globals.lock` (opt-in `VRX_ACL_STATS_GLOBALS=1`) and, per the envelope (V7), never off.

### Scale 10 000 on the host — `VRX_ACL_SCALE=10000 run.sh -run TestACLTopology` (04:25–04:26, NRestarts 1 → 1)
```
scale 10000: NRestarts before = 1
scale 10000: raw acl_add_replace (10000 rules, one message) 0.013 s → acl_index 2
scale 10000: raw acl_del 0.000 s
scale 10000: CSV dry run (509501 bytes) → 200 in 0.85 s: rows=10000 valid=10000 errors=0
scale 10000: CSV import → 200 in 2.83 s: imported=10000
scale 10000: commit → 200 in 10.80 s: {"status":"applied",…"results":[{"key":"acl.acl/scale","op":"create",…
scale 10000: agent: {"msg":"reconcile done",…"mode":"apply","domains":["interfaces","vrfs","routing","objects","acl"],
                     "status":"APPLY_STATUS_APPLIED","summary":"created:1  unchanged:17","duration":2258962937}
scale 10000: first page (100 rules, source=candidate) → 200 in 0.707 s: total=10000 applied=true mappingKnown=true
scale 10000: first page (100 rules, source=running) → 200 in 0.037 s: total=10000 applied=true mappingKnown=true
scale 10000: last page (source=running) → 200 in 0.030 s
scale 10000: removal commit → 200 in 2.87 s
scale 10000: NRestarts after = 1
```
**100 000 on the host: not run** — opt-in and waiting for a manager window (Q8); the 100k commit also needs Q2's gRPC
limits (a 100k `ApplyRequest` is ≈ 12 MB > 4 MiB). Projection of 100 000 rules (unit, no VPP):
`projection of a 100 000-rule list (100 000 VPP rules): 2.74 s` (`internal/desired TestACLListLimitAndProjectionTime`).

### Rule editor at 100 000 rules (API + slot DB, candidate only, no VPP object) — `VRX_ACL_EDITOR=1 run.sh -run TestACLEditorScale` (04:5x)
```
CSV dry run (100000 rows, 6522195 bytes)                   → 200 in   4.399 s
CSV import into the candidate                              → 200 in  25.153 s
GET rules?page=1&pageSize=100 (candidate)                  → 200 in   2.535 s
GET rules?page=1&pageSize=1000 (candidate)                 → 200 in   1.798 s
GET rules?page=500&pageSize=100 (candidate)                → 200 in   2.401 s
GET rules?page=1000&pageSize=100 (candidate)               → 200 in   2.064 s
GET rules?page=1&pageSize=100&filter=172.17.1. (candidate) → 200 in   2.296 s
GET rules?page=1&pageSize=100&filter=rule%2099999          → 200 in   2.311 s
bulk disable of 1 000 rules                                → 200 in  43.222 s   ← datastore edit of a 100k-rule candidate (Q11)
bulk move of 1 rule to sequence 15                         → 200 in  39.783 s
GET state/acl/lists                                        → 200 in   2.222 s
export.csv (candidate)                                     → 200 in   3.312 s
--- PASS: TestACLEditorScale (145.14s)
```
(Before decision 8 the first page took 5.9 s.) Running-source pages are served from the revision-id cache (0.03 s at 10k).

### Agent unit (fake VPP) — `go test -v -run 'ACL|PBR|RulePage|Record|Tracker' ./internal/{agent,desired,actions/...,subsystems}`
```
--- PASS: TestACLDomainOnFake        DryRun (log warning, nothing applied) → Apply → pointers → VPP expansion (5 rules: families,
                                     ICMP v4 only, inactive schedule omitted) → Retrieve == desired → 2nd Apply empty → AclState
                                     (paging, counters off/on, sequence filter, hits only, NOT_FOUND, limit, owner) → foreign ACL
                                     kept first on update → restart after loss → rollback leaves only the foreign ACL
--- PASS: TestACLDryRunFindings      acl.objects-required, acl.expansion-limit (101 × 100), unknown schedule
--- PASS: TestACLStatsEnableGlobalsOwner
--- PASS: TestACLBindingsOrderAndZones · TestAssembleACLReconstructsUnknownContent · TestACLNameTooLongForTag
    acl_test.go:178: projection of a 100 000-rule list (100 000 VPP rules): 2.735771737s
--- PASS: TestACLListLimitAndProjectionTime · TestACLFQDNUnresolvedAndFamilies
--- PASS: TestRulePageMapsCounters · TestRecordBoundsAndLookup · TestTrackerAndStateOnFake
--- PASS: TestPBRPolicyNamesFACLList (D-131) · TestACLDescriptorsDeclareOwnership (TD-11b) · TestACLWatcher · TestRegisterACLWiring
ok ngfw/agent/internal/agent · desired · actions/acl · subsystems
$ make -C apps/agent lint test      → golangci-lint 0 issues; go test -race ./... all ok
```

### API — unit and e2e (slot 3 PostgreSQL + fake agent)
```
 ✓ src/features/acl/rules.test.ts (6 tests)        sort/search/pending, bulk move/renumber, CSV round trip over split chunks,
                                                    row issues, size cap, acl/objects schemas have no secret leaf
 Test Files 12 passed · Tests 105 passed           (pnpm --filter @ngfw/api test)
 ✓ test/e2e/acl.e2e.test.ts (5 tests)
   ✓ commits a list with attachments; lists, rule page with counters, attachments with a foreign ACL
   ✓ bulk edits the candidate in one edit; pending marks; readonly is refused       (audit row: {"rules":3,"op":"move",…})
   ✓ imports CSV with a dry run first, appends, exports                              (400 with /csv/<line>/<column> pointers)
   ✓ a rule naming an empty address group: 400 problem+json with the pointer; nothing is applied
   ✓ degrades without the agent RPC: config columns stay, live is null with the reason
 drop database vrx_w3 · drop role vrx_w3 · ok nothing named vrx_w3 / vrx_w3 remains
```

### Web (sub-worker run, re-checked)
```
pnpm --filter @ngfw/web lint       → check-logical-css: OK (141 files, no physical left/right CSS), exit 0
pnpm --filter @ngfw/web typecheck  → exit 0
pnpm --filter @ngfw/web test       → Test Files 19 passed (19) · Tests 130 passed (130)   (AclPage 8, model 12, locales parity, nav)
pnpm --filter @ngfw/web build      → bundle-budget: initial 349.1 kB gzipped of 600.0 kB budget; OK
```

### Screenshots — PENDING TD-25
The real-stack screenshot run (`TestACLScreenshots`, headless Chrome-for-Testing + playwright-core from the npx cache, script
kept outside the repository) is ready, but the manager paused host runs: since the VPP restart at 04:27 the TD-3 sanitizer
refuses every interface create on the shared VPP (placeholder cap 64 < 122 freed classify indices; TD-25 fixes it, Q13).
One no-rig attempt before the pause produced the lists / rules / attachments / MACIP screens in English (`pageErrors=0`)
and stopped at the pager of the 5 000-rule editor (script fix done); it cleaned up (no w3 ACL left, NRestarts 2 → 2).
Command once TD-25 lands:
`VRX_ACL_STATS_GLOBALS=1 VRX_ACL_SHOTS=<scratch>/F-acl/acl-shots.mjs VRX_ACL_SHOTS_OUT=<dir> test/topology/acl/run.sh -run TestACLScreenshots`.

### CI — `TMPDIR=/tmp/g-w3 tools/ci.sh --base main` (HEAD c0b93aca; the next commits only change this status and the questions)
```
== VRX CI gate: quick ==
branch    task/F-acl @ c0b93aca   (base: main)
== contract guard: HEAD vs main ==  ok — contract commit on the branch: 410b5471 contract(proto): acl state
== test/ Go modules, unit mode (test/integration/smoke test/topology/acl test/topology/interfaces test/topology/object-model) ==
test/topology/acl: gofmt ok · go vet ok · ok  	ngfw/test/topology/acl	0.015s;
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   2m21s
  forbidden patterns (+ gitleaks)                    0m07s
  packet-trace ban on the shared VPP (D-128)         0m01s
  lint · typecheck · unit tests · build (turbo)   4m11s      (Tasks: 30 successful, 30 total)
  apps/agent: make lint test build                   0m57s
  apps/cli: make lint test build                     0m23s
  test/ Go modules, unit mode (…)                    0m15s
  deploy/vpp: shellcheck + apply-startup fake-host harness   5m11s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      merge main (W-seed a303f0b + P08 squash) into task/F-object-model
      review(F-object-model): architecture review — APPROVE WITH CHANGES
      review(W-seed): verify
  mode quick · wall time 13m30s · logs /root/ngfw-wt/logs/ci/F-acl-20260925-050144-3949872

CI GATE PASSED
```
The warnings are F-object-model's / W-seed's commits (inherited through the speculative base).

## Acceptance
- [x] `vppctl show acl-plugin acl` and `show acl-plugin interface` reflect the committed lists/bindings (+ MACIP); `Retrieve()` == desired
- [x] Hit counters: rig ping increments the matching rule's counter in the API (+5 on rule 10, +3 on rule 30)
- [~] 100k-rule list: projection 2.74 s (unit), 10k on the host (raw `acl_add_replace` 0.013 s, commit 10.8 s, first page 0.04 s),
      100k editor routes measured (first page 2.5 s); 100k on the shared VPP waits for a manager window + Q2 (Q8); UI scroll screenshot pending TD-25
- [x] Agent-restart simulation → bindings back in 0.20 s; foreign ACL untouched and still first (D-066)
- [x] Rollback removes lists and bindings (Retrieve `{}`, VPP dump shows only the foreign ACL)
- [x] Rule referencing an empty group → 400 problem+json with `pointer` (host + e2e)
- [ ] UI screenshot pasted — pending TD-25
- [x] `tools/ci.sh --base main` green (c0b93aca, 13m30s)

## Out of scope (not built)
Host ACLs / nftables (F-host-acl-nftables), objects CRUD and FQDN resolution (F-object-model), ADL / Auto-SDL / uRPF / ABF
descriptors and screens (F-rpf-adl-pbr; only the info link and D-131's test), ACL session sync (F-ha-state-sync), NAT,
classifier ACLs, per-interface counters (VPP has none), connection-table sizing, a CLI command for the state routes.

## Cleanup
Every process the tests started (agent, API, vite preview) was stopped by PID (logged); `vrx_w3` dropped by every run;
`/run/vrx-test/w3` is empty; nothing listens on 3300/5300/9131; the lab lock is taken only by `run.sh` for a run.
No w3 ACL or MACIP ACL is left in VPP:
```
$ vppctl show acl-plugin acl | grep "tag {w3"          → (nothing)
$ vppctl show acl-plugin macip acl | grep w3            → (nothing)
```
An aborted screenshot run (script failure) had left `w3:lan-in` and `w3:wan-l2` behind: removed with the new opt-in
janitor (`VRX_ACL_JANITOR=1 run.sh -run TestACLJanitor`: unbind first, then delete; preflight OK before and after), and
the tests now clean up through the API in `t.Cleanup` (foreign ACL unbound before the interfaces are deleted, D-095c).
The ACL counters flag stays on (V7, envelope). No test ran `show trace` (D-128) or swept classify indices (D-126).

## Fix round 1 (review `d58b9105`, APPROVE WITH CHANGES) — 2026-09-25

| finding | fix | test (fails on the old code) |
|---|---|---|
| **H1** DryRun rewrote the expansion record Retrieve/AclState/watcher read | New agent-local descriptor **`acl.config`** (`actions/acl/config.go`, registered first in `Domains["acl"]`): `acl.config/list/<name>`, `acl.config/macip/<name>`, `acl.config/attachments`, value = the configuration message holding that entry; only Create/Update/Delete of a transaction change it (a rolled-back Apply is reverted by the scheduler), in memory, rebuilt by the resync, `RecordsNoOwnership`. Record entries are keyed by name + VPP fingerprint + **configuration hash**; the applied configuration's entries are pinned (never evicted by DryRun entries or the rule budget; a list's newest entry is never dropped either). `AssembleACL`, `AclState` and the watcher use the entry of the APPLIED configuration only | `agent TestACLDryRunAndRollbackDoNotChangeTheAppliedView` (probe P1 with lists + attachments: Apply A, DryRun B with the same VPP content, then a rolled-back Apply of B → Retrieve == A and counters stay on sequences 10/20) · `actions TestRecordBoundsAndLookup` (20 DryRun entries do not evict the applied one) · `actions TestTrackerAndStateOnFake` (a recorded but unapplied projection does not explain VPP's content) |
| **L10** no agent-level hand-edit test | — | same test: our ACL's rules replaced in VPP behind the agent's back → Retrieve reports VPP's rules (drift), a resync repairs them |
| **M3** (review 3.4) binding→interface dependency optional | `acl.interface-binding` and `acl.macip-interface-binding` depend on their interface **mandatorily** (`descriptors/acl/register.go` `boundInterfaceDependency`; gap edit, DF-4 dependency tests updated); the ethertype whitelist keeps the optional one | `agent TestACLBindingAndInterfaceRemovedInOneCommit`: interface + binding removed in one commit → binding deleted first, nothing bound to the deleted index; a binding to an interface neither configured nor in VPP → `agent.dependency-missing` at `/acl/attachments/0` |
| **M1** counters docs wrong | `docs/user/firewall/acl.md` (hits since the list last changed in VPP; VPP clears them on every `acl_add_replace`, incl. the watcher's re-projections; `enable=false` exists; 0 after every VPP restart; per-packet cost), `docs/agent/descriptors/acl.md`, `docs/vpp-code-track.md` V7 row | docs |
| **M2** host tests flip a VPP-global | `test/topology/acl` `countersScope`: opt-in `flock -x` → save → on → `flock -s` while relied on → `flock -x` → restore EXACTLY the saved value → unlock (shared-host rules §7); the screenshot run too | vet/unit mode (no host run: TD-25) |
| L2 | one `DumpStats` for all tracked lists | covered by the state tests |
| L5 | resync requests ≥ 30 s apart | `TestACLWatcher` |
| L6 | time-zone change reaches schedules after an agent restart (docs) | docs |
| L8 | janitor re-reads each index's tag right before `acl_del` / `macip_acl_del`; MACIP pass uses the `w3:` / `w3-` predicate too | vet |
| L13 | `F-acl-wip.md` updated | — |
| M4, Q3, Q14 | at the rebase, when the manager says TD-23 / F-object-model / F-host-acl-nftables / F-rpf-adl-pbr are in | — |
| M5/Q2, M6/Q11, Q12, L1, L3, L4, L7, L9, L11, L12 | owned elsewhere (manager: new TD rows / after WEB-1) or not in this round | — |

The same tests against the old product code (the fix round's product files taken from `a6fcdff2` in a scratch copy of
`apps/agent`, with the new tests and the fake's new helpers):
```
--- FAIL: TestACLDryRunAndRollbackDoNotChangeTheAppliedView (0.05s)
    rpc_acl_test.go:438: after a DryRun of the candidate: Retrieve != the APPLIED configuration (false drift):
--- FAIL: TestACLBindingAndInterfaceRemovedInOneCommit (0.02s)
    rpc_acl_test.go:507: a binding to a missing interface must be refused at planning: <nil> ok:true  plan:{key:"acl.interface-binding/loop799"  op:APPLY_OPERATION_CREATE …}
FAIL	ngfw/agent/internal/agent	0.166s
```
On the fix:
```
--- PASS: TestACLDryRunAndRollbackDoNotChangeTheAppliedView (0.05s)
--- PASS: TestACLBindingAndInterfaceRemovedInOneCommit (0.01s)
--- PASS: TestRecordBoundsAndLookup · TestTrackerAndStateOnFake · TestACLDescriptorsDeclareOwnership · TestACLWatcher · TestInterfaceBindingDependencies
$ golangci-lint (actions, desired, subsystems, agent, descriptors/acl, descriptors/core) → 0 issues
$ go test -race ./internal/{actions/...,desired,subsystems,agent,descriptors/acl,descriptors/core/...} → all ok
```
No host run in this round (TD-25 pending; manager: host runs paused). Shared hunk added: `docs/vpp-code-track.md` V7 row
(manager's request).

### CI (fix round 1) — `TMPDIR=/tmp/g-w3 tools/ci.sh --base main` (HEAD 122306d4; later commits change only this status)
```
== VRX CI gate: quick ==
branch    task/F-acl @ 122306d4   (base: main)
== summary (quick) ==
  contract guard: HEAD vs main                       0m01s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   1m40s
  forbidden patterns (+ gitleaks)                    0m04s
  packet-trace ban on the shared VPP (D-128)         0m01s
  lint · typecheck · unit tests · build (turbo)   1m49s
  apps/agent: make lint test build                   0m52s
  apps/cli: make lint test build                     0m09s
  test/ Go modules, unit mode (test/integration/smoke test/topology/acl test/topology/interfaces test/topology/object-model)   0m11s
  deploy/vpp: shellcheck + apply-startup fake-host harness   0m09s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(F-acl): APPROVE WITH CHANGES
      merge main (W-seed a303f0b + P08 squash) into task/F-object-model
      review(F-object-model): architecture review — APPROVE WITH CHANGES
      review(W-seed): verify
  mode quick · wall time 5m00s · logs /root/ngfw-wt/logs/ci/F-acl-20260925-100259-760609

CI GATE PASSED
```
