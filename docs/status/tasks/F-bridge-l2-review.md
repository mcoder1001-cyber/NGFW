# F-bridge-l2 — review

Reviewer: independent agent (did not write this code). Branch `task/F-bridge-l2` @ `a735aa9`. Diff reviewed: `df67a8e..a735aa9`,
the branch's own 18 commits on the speculative W-seed base. `task/W-seed` moved to `a303f0b` (re-cut on main after P08 landed)
while this review ran, so `task/W-seed...task/F-bridge-l2` now also shows P08. Read: 00-CONTEXT, REVIEW-PROMPT, the feature
prompt, the envelope, wave-A-hotspots §0–§2, LOG D-109(c)/D-122/D-126/D-127, D-128 (status 1941), the status, questions
(Q1–Q11) and contract files.

## Verdict: **APPROVE WITH CHANGES**

The contract, the agent, restart safety, rollback and the host evidence are solid. Two things must happen before the merge:
the base (finding 1) and the member-removal bug (finding 2). Findings 3–4 are small test and doc fixes. The rest can be
follow-ups.

## Checklist

| # | check | result |
|---|---|---|
| 1 | Contract | Additive only. D-109(c) shape: per-port leaves `Interface.l2 = 14`, `Subinterface.l2 = 12`, and the named container `routing.l2` (`RoutingConfig.l2 = 20`, confirmed by D-122). **No new root key.** Numbers match §2. Messages are prefixed `BridgeL2*` / `BridgeDomain*`, and the RPCs sit under the service anchor. Contract commits come first (`72eb38b`, `401c0da`, `0ae1a9c`) and `F-bridge-l2-contract.md` is present. Nothing existing was renamed or reshaped. |
| 2 | Real verification | `TestBridgeL2OnHost` runs on the host VPP through the real API and agent. It asserts on binapi dumps, the `vppctl show bridge-domain/l2fib/mode/l3xc/mactime` output, and on the agent's Retrieve compared with running. `TestMactimeOnHost` also runs on the host. Unit tests use the coretest fake, which models VPP's appending add and the stacking enable. |
| 3 | Restart safety | Evidence is pasted. Dependents were deleted first (D-095c): mactime enable → device → tag rewrite → members → xconnect → l3xc → BD. Everything was back in 0.20 s, Retrieve == desired, and NRestarts stayed at 1 before and after. Every object type has a Retrieve. |
| 4 | VPP API provenance | `mactime_*`, `feature_is_enabled`, `l2_*` and `l3xc_*` all come from `apps/agent/binapi`. The branch does not touch `binapi/`, `tools/binapi-gen.sh`, `ifsanitize`, `dfkit`, `interface`, `desired/interfaces.go`, `apps/api/src/state`, P08's web files, `ui-kit` or `tunnels.ts`. |
| 5 | Shared host | Slot w7 only: BD 7001, device `w7:w7-kids`, loopbacks and the rig. Veths are brought down before the af_packet delete (D-101). The lock is shared and held only during a run. Cleanup dumps are pasted. The tests use read-only `vppctl show` only: **no `show trace`, no trace add, no classify/policer sweeps** (D-126/D-128; grep of the branch's own diff). |
| 6 | Security | No `exec.Command` or `child_process` in product code. The new routes are `@Protected` and their parameters are Zod-validated (`id` 1–16777215, `pageSize` ≤ 1000). No secrets. |
| 7 | Transactions | Rollback proven on the host: `show mode` shows l3, no BD, xconnect, l3xc or mactime is left, `vtr_op` is 0, and Retrieve has `routing.l2` = null. |
| 8 | UI honesty | Real endpoints. Screenshots were taken against the real stack (en and fa-RTL). No TODO, mock or stub in product code. |
| 9 | Scope | No second l2/l3xc descriptor (D-104): only DF-1's `l2.bridge-domain` gained an optional `name`, which is Q7 and an owned file. MAC-filter UI shipped per Q5 (the prompt's open question). Nothing else extra. |
| 10 | i18n | en/fa `bridge-l2.json` have the same 146 keys. No hardcoded JSX strings. No physical `margin`/`padding` properties. |
| 11 | CI | **Re-run by the reviewer:** `TMPDIR=/tmp/g-w7 bash <main:tools/ci.sh copy> --base main` at `a735aa9` → **CI GATE PASSED** (wall time 23m31s, logs `/root/ngfw-wt/logs/ci/F-bridge-l2-20260924-224936-3332813`). Generated output is clean, gitleaks is clean, turbo 30/30, apps/agent and apps/cli are green, and the test/ modules pass in unit mode. The only warning is from deploy/vpp: scenario 24 was flaky under load (~30) and passed on the serial rerun. It is unrelated to this branch. The worker's pasted gate (ci.sh with only the D-127 hunk) is consistent with this. |

## Findings (ranked)

### 1. HIGH (merge precondition, process) — the branch is on the stale speculative base and does not merge cleanly
`task/F-bridge-l2` still sits on the old W-seed `df67a8e` (P08@abb6950 merged in). P08 has since been squash-merged to main,
and W-seed was re-cut as `a303f0b`. A read-only `git merge-tree` probe shows:
- `main` + branch: **16 conflicting files** (proto, gen, subsystems.go, fakevpp.go, app.module.ts, web router/nav/i18n, model.ts, …),
  because P08 is duplicated.
- The rebase equivalent (`--merge-base=df67a8e task/W-seed task/F-bridge-l2`): conflicts only in `apps/cli/internal/api/operations_gen.go`
  and `packages/api-client/src/generated/schema.d.ts`, which are generated.

**Fix:** `git rebase --onto task/W-seed df67a8e task/F-bridge-l2`. Resolve the two generated files per hotspots rule 3
(`pnpm gen && make -C apps/cli gen docs`), then re-run `tools/ci.sh --base main` and `apps/api/test/e2e/bridge-l2.e2e.test.ts`.
Between `df67a8e` and `a303f0b` the agent `projection/desired/interfaces` code and P08's web interface code are unchanged. Only
the API datastore/commit/auth code changed (TD-2 follow-ups). So the host evidence stands, but the API e2e must be re-run.

### 2. MEDIUM — removing a bridge member also silently turns off that interface's MAC filter
`apps/web/src/domains/interfaces/bridge-l2/BridgeDomainDrawer.tsx:126-128`. `removeMember` sends `portPatch(port, null)`, which
sets `l2: null` and deletes the whole leaf. That leaf also holds `macFilter`. mactime runs on `device-input` whether the port is
L2 or L3, so it has nothing to do with bridge membership.

**Failure:** `host-X` has `l2: {bridgeDomain: lan, macFilter: true}`. The operator removes `host-X` from `lan`. The commit
disables `mactime.enable/host-X`, and every device the time-range filter was blocking gets through. This is a silent,
security-relevant loosening.

**Fix:** when `port.l2.macFilter` is true, send `{bridgeDomain: null, shg: null, bvi: null, uuFwd: null, tagRewrite: null}`.
Otherwise keep `l2: null`. Add a case to `BridgingPage.test.tsx`.

### 3. MEDIUM (Q10 answer + a missing test) — the opaque `l2` field in P08's drawer
- **Interface drawer (`interfaces/model.ts:24` hunk): no silent deletion.** `saveInterface` diffs against the value the form
  opened with (review N4). An untouched JSON field keeps the same object, so `createMergePatch` emits no `l2`. The JSON input
  only calls `onChange` on blur, and a leaf that was absent stays absent. The only way to delete is explicit: clearing the text
  gives `l2: null`, which removes membership, tag rewrite and MAC filter together, with no confirmation. **Missing:** no test
  covers "l2 set → edit MTU → PATCH body has no `l2` key". P08's 7 drawer tests only cover the absent case. Add that test in an
  owned file, for example render `InterfaceDrawer` from `bridge-l2/*.test.tsx`.
- **Sub-interface dialog: yes, it can delete membership (P08 defect, now reachable for L2).**
  `apps/web/src/domains/interfaces/InterfaceDrawer.tsx:140-150` (`saveSub`) diffs the form value against the *fresh* candidate,
  not against the value the dialog opened with. N4 was not applied there.

  **Failure:** someone opens the `host-w7l0.100` dialog. Meanwhile the Bridging page (another tab or another admin) adds
  `.100` to a BD. The dialog is then saved. The patch contains `l2: null`, and the membership and its `pop-1` disappear without
  any warning.

  **Fix (P08 owner, not this branch):** make `saveSub` diff against the opened value, as `saveInterface` does. → Manager:
  file it as a P08 follow-up.

### 4. LOW (doc correctness; answers the manager's question) — `/state/drift` does **not** ignore `/routing`
`docs/status/tasks/F-bridge-l2.md:146-148` says drift "ignores the whole `/routing` domain". The code says otherwise. P08's
`driftOf` (`apps/api/src/state/state.controller.ts:425-441`) skips only pointers of depth ≥ 2 or `agent.unimplemented-domain`.
The domain-level `/routing` note is *listed* in `ignored` but hides nothing.

Reviewer probe (tsx on the branch's `driftOf`): a `/routing/l2/bridgeDomains/lan/macAgeMin` change together with the report
`[{pointer:"/routing", rule:"agent.unsupported-field"}]` produces `changes:[{…/routing/l2/…}]` and `ignored:[{"/routing"}]`.
So the host run's `changes: []` after apply, restart and rollback **is** a real running-vs-Retrieve comparison of `routing.l2`.
The extra direct Retrieve comparison is harmless.

**For the manager:** this is not a gap for L2. The only real oddity is P08's: `ignored` lists `/routing` without skipping it.
Once RF-1 or P12 put `bgp`/`ospf` in running, drift will report `/routing/bgp` as removed. That belongs to P08/RF-1.

**Fix here:** correct the note, and paste the drift body (it is logged, but truncated in the status file).

### 5. LOW — BridgeDomainState dumps the whole L2 FIB of every BD on each 3 s poll
`apps/agent/internal/agent/rpc_bridge_l2.go:131-141` (and `fibOf` at `:63-81`); `bridge-l2/queries.ts:14` sets `BRIDGE_POLL_MS = 3_000`.
The MAC *paging* bound is correct:
- the agent answers ≤ 1000 per message and `INVALID_ARGUMENT` above 1000 (unit-tested);
- the API rejects `pageSize > 1000` with 400;
- the agent sorts server-side and re-checks ownership (`NOT_FOUND` for a foreign BD).

However, the *list* endpoint streams the full `l2_fib_table_dump` for every owned BD, buffers it, and repeats that every 3 s
for every open browser tab, only to compute two counters. VPP runs that dump on its main thread.

**Fix:** count while streaming (no slice); poll the list at 10–30 s or cache the counts for a few seconds; keep 3 s only for the
open drawer. Not a performance task (FAST MODE), just a cheap safeguard.

### 6. LOW — `mactime.range` Create deletes a device that is not owner-prefixed
`apps/agent/internal/descriptors/mactime/device.go:185-193`. VPP's auto-learned `mac-<mac>` entry is deleted and replaced. The
envelope says "touch only w7/owner-prefixed devices" (D-071, shared table). The behaviour is right for the product (the single
owner schedules a device VPP already learned) but wrong for a test slot on the shared host.

**Fix:** adopt only when this agent is the globals owner (`VRX_GLOBALS_OWNER=1`). Otherwise return `ErrNotOurs`. Mention this in
`mactime.md`.

### 7. LOW — cross-domain coupling of `routing.l2` with the `interfaces` scope
`apps/agent/internal/agent/projection.go:299-301` and `:415-417`, `desired/l2.go:129`. All L2 descriptors live in the
`interfaces` domain, while half of the model is under `routing`:
- an `Apply` scoped to `["routing"]` alone would report APPLIED without touching L2;
- an `Apply(["interfaces"])` whose desired state has no `routing` would delete every xconnect, l3xc, MAC-filter device and
  member-less BD.

This is not a live bug: the API always sends every implemented domain, and resync uses all of them.

**Fix:** a guard in `BridgeL2`: when `routing.l2` is present but `routing` is not in scope (or the reverse), return an error on
`/routing/l2`. Add one line to proto.md §11 and a unit test.

### 8. LOW — UI leaves the candidate invalid instead of cascading or warning
- Removing a BD (`BridgeDomainDrawer.tsx:108-113`) leaves every member's `l2.bridgeDomain` pointing at the deleted record. The
  next commit fails with `interfaces.bridge-l2-domain-exists`.
- Removing an L2 xconnect (`BridgingPage.tsx:351-357`) leaves the rx's `l2.tagRewrite`. The next commit fails with
  `…tag-rewrite-l2-only`.
- The xconnect dialog has no tag-rewrite editor. The only UI path to `translate-1-1` on an xconnect rx is P08's raw JSON field.

**Fix:** cascade (or confirm and list) in both removes, and add `tagRewrite` to the xconnect dialog. Also minor:
`if (!patchRouting.isError) onClose()` (`:112`) reads a stale render value.

### 9. LOW — rename = recreate (Q7) is undocumented for users
The design choice itself is **accepted**: the name travels in the bd_tag `<owner>:<id>/<name>`. That keeps Retrieve honest
(D-063: no echo of cached desired state) and needs no agent state. The old `<owner>:<id>` form still parses, and there is a
unit test. Consequence: renaming a record deletes the BD and re-creates it, which drops all members and learned MACs and cuts
traffic briefly.

**Fix:** one sentence in `docs/user/interfaces/bridge-l2.md`, and optionally a hint in the drawer.

### 10. LOW — API 404 mapping is string-matched
`apps/api/src/features/bridge-l2/bridge-l2.controller.ts:189-193` maps a 502 whose message matches `/not this agent/` to 404.

**Fix:** key it on the gRPC code (`e.extra['grpcCode'] === 'NOT_FOUND'`, as commit.service does for `FAILED_PRECONDITION`).

### 11. INFO
- **Q2 (an interface in two BDs cannot be expressed): acceptable.** The single-valued leaf makes the case impossible by shape,
  which is stronger than a validator. The acceptance equivalent is proven on the host and in e2e: bridge member + xconnect rx
  → 400 problem+json with pointer `/routing/l2/xconnects/host-w7l0`. Bridge + l3xc and xconnect + l3xc are covered too.
  Manager: amend the acceptance line.
- **Q9 (`coretest/fakevpp.go:96`, one line outside the owned files): accept.** Without it every agent unit test breaks, and the
  handlers are in the owned `coretest/bridge_l2.go`. Manager: seed an A6 hook anchor, because F-bonding, F-vrf-static-ecmp and
  F-neighbors-ra will add a line at the same spot.
- Stale text: `F-bridge-l2-contract.md:37` still says "Manager to confirm or renumber (Q1)". D-122 answered it.
- A mactime enable whose record was lost (wiped state dir) and whose config was later removed is never disabled: Retrieve hides
  it without the record (`enable.go:336-372`). This follows from the V23(a) readback caveat. Document it in `mactime.md`.

## Required before merge
1. Finding 1 (rebase onto `task/W-seed` `a303f0b`, regenerate, CI + API e2e green).
2. Finding 2 (member removal keeps `macFilter`, with a test).
3. Finding 3 test (drawer round-trip with `l2` set) and finding 4 note correction.

Findings 5–10 are follow-ups. Finding 3's sub-dialog defect and finding 4's `ignored` oddity belong to P08.
