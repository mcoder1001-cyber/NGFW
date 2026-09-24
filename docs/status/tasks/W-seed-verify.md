# W-seed — verification

**Verdict: APPROVE**

Scope: `git diff task/P08...task/W-seed` (merge-base `e1587c94`, head `8b7558e`). Checked against
`prompts/tech-debt/W-seed.md`, `docs/status/wave-A-hotspots.md` §1/§3, and the worker's own
`docs/status/tasks/W-seed.md` / `W-seed-questions.md`.

## 1. Diff scope
`git diff task/P08...task/W-seed --stat` touches 35 files: the hotspot files named in the prompt
(A1/A2/A4, C1/C2/C3/C5/C6, P1/P4/P5/P6, W1/W2/W3), the seam files (`apps/agent/internal/subsystems/{seams,seams_test}.go`,
`apps/web/src/domains/{DomainTabsPage.tsx,DomainTabsPage.test.tsx,vpn/*,services/*}`, 4 new locale JSONs owned by the
shells), and 3 status docs (`W-seed.md`, `W-seed-questions.md`, `W-seed-wip.md`). No file outside this set. No generated
path touched: `git diff task/P08...task/W-seed -- apps/agent/gen packages/proto/gen packages/api-client/src/generated packages/schema/dist apps/cli/internal/api/operations_gen.go docs/user/cli/reference.md` is empty.

## 2. Logic changes outside the three seams
Only one non-comment structural hunk found outside the three named seams: `apps/agent/internal/agent/server.go:85`
turns `Action` into a type switch (`switch req.GetAction().(type) { default: return status.Error(...) }`) with no
case populated yet. This is the A4 anchor the prompt itself specifies ("turn `Action` into a type switch with a
default `Unimplemented`"), not incidental logic — every input still falls through to the same `default:` branch and
the same error message as before, so it is behaviour-preserving. Confirmed with `go build ./...` and
`go test ./internal/subsystems/...` (below). No other non-comment/non-reformat hunk exists; every other diff hunk is
either a `// wave-A: <task-id>` comment block or a pure one-line-per-entry reformat.

## 3. Reformatted lists — same entries
- `subsystems.go` `Domains[Interfaces/VRFs/Routing]`: same 12/1/1 entries, now one per line, anchors are comments only.
- `nav.ts` `BUILT_DOMAINS` (nav.ts:50): still exactly `{'interfaces'}` — the new anchor lines under it are comments,
  not new set members (Q5 in W-seed-questions.md explains this is deliberate: seeding an entry would be a behaviour
  change).
- `nav.test.ts` `available` array: same 7 entries (`dashboard, interfaces, users, revisions, dev-schema-form,
  dev-data-grid, dev-stream`) — verified this is what the array evaluates to by running the test (below).
- `i18n.ts` `NAMESPACES`/`en`/`fa`: same 8 original entries plus exactly `services` and `vpn` (the two shell
  namespaces the spec calls for), one per line.
All confirmed by diff read, not just by trusting the worker's report.

## 4. Seams inert + tested
- `apps/agent/internal/subsystems/seams.go:46` `Wiring.Publish` and `:54` `RequestResync` are nil-checked
  (`w.env.Publish != nil` / `w.env.Resync != nil`); a zero-value `Wiring{}` (no `Env` set) is a no-op.
  `seams_test.go:11` `TestEventAndResyncHooksDefaultInert` calls both on a zero `Wiring{}` first (asserting no panic),
  then re-checks forwarding with hooks set.
- `seams.go:29` `SlotIDRange()` returns `nil, nil` when `VRX_VPP_TABLE_BASE` is unset (default = agent owns every id).
  `seams_test.go:29` `TestSlotIDRange` covers unset, a normal value, five malformed/out-of-range values, and the
  top-of-range edge.
- Ran `cd apps/agent && go test ./internal/subsystems/...`: `ok ngfw/agent/internal/subsystems 0.058s`, and verbose
  `-run 'TestEventAndResyncHooksDefaultInert|TestSlotIDRange'`: both PASS.
- `apps/web/src/domains/DomainTabsPage.tsx:38` returns `<DomainPlaceholderPage domainKey={domainKey} />` verbatim
  when `tabs` is empty (the shells' `tabs.ts` registries start empty). `DomainTabsPage.test.tsx` covers: placeholder
  when empty, one tab per registration with `?tab=`, fallback for an unknown tab id, and (line 53) that `/vpn` and
  `/services` are each registered exactly once in `buildRoutes()`.

## 5. vpn/services shells — hidden + i18n parity
`router.tsx:44` `SHELL_DOMAINS` is excluded from the generic placeholder-route map (`:64`), so the path is registered
once via `SHELL_ROUTES` (`:40`), not twice. `nav.ts` `BUILT_DOMAINS` (§3 above) does not include `vpn`/`services`, so
they render exactly as unavailable/placeholder in nav as they already did pre-W-seed (no new reachability). en/fa key
sets for both new namespaces are identical (`title`, `tabs`, `loading`) — verified both by direct read and by
`apps/web/src/locales/locales.test.ts`, an existing generic test that iterates every `locales/en/*.json` file and
asserts fa has the exact same key set; it now covers `vpn`/`services` automatically and passes.

## 6. Generated files
Confirmed empty diff against every C7 path (command in §1). Worktree `git status --porcelain` is clean.

## 7. Anchor placement (spot-check, matched against hotspots.md §3, not just W-seed.md's own table)
Checked 10+: F-vlan-qinq correctly appears only in W3 (not A1/A2/A4/C1/C3/P*/W1/W2 — it's W5/W3/D1 per §3, and its
C2 touch is defect-conditional so was reasonably left unanchored); F-bonding appears in A1/A2/C1(InterfaceSchema)/P1/P4/P5/W1/W2(non-domain list)/W3,
absent from BUILT_DOMAINS (matches the noted critic decision "Bonds is its own route, not a W5 tab"); F-object-model
and F-vrf-static-ecmp appear in `BUILT_DOMAINS` (domain items) while bonding/bridge-l2/neighbors-ra/rpf/host-acl/P12
appear only in the non-domain `buildNav` list — matches the W2 row's domain-vs-non-domain split; F-nat44-ed-sessions
appears in A4's case list and P1/P4/P5/W1/W2/W3 but correctly not in C1 (its envelope has no C1 touch); `server.go`
Action switch has exactly the 4 cases §-documented (vrf-static-ecmp ping, neighbors-ra arp-flush, nat44-ed
session-kill, plus wave-B unbound-chrony-syslog dns-lookup, explained in W-seed-questions.md Q4). No misplaced or
missing anchor found in the sample.

## Test counts (independently re-run, not just trusted from W-seed.md)
- `go test ./internal/subsystems/...`: pass (seam tests included).
- `pnpm --filter @ngfw/web test -- nav`: needed `pnpm --filter {ui-kit,schema,proto,api-client} build` first (the
  worktree had no `dist/` for workspace deps — an environment/build-order issue unrelated to W-seed, not a defect in
  the branch); after that, **14 files / 92 tests, all pass**, matching the worker's report. Verified the +6 over the
  claimed base-86 breaks down as +4 from the new `DomainTabsPage.test.tsx` and +2 from `locales.test.ts` (an existing
  `it.each(namespaces)` test that auto-discovered the two new `vpn`/`services` JSON files) — the worker's status doc
  attributes all +6 to `DomainTabsPage.test.tsx`, which is a minor reporting inaccuracy, not a correctness problem;
  the aggregate counts (92) are correct and independently confirmed.

## Notes (non-blocking)
- Several anchors go beyond strict wave-A (e.g. wave-B tasks in A1/A4/C2/P1 etc.); this is called out and reasoned
  in `W-seed-questions.md` Q4 and matches the branch's stated intent to also seed the 5 named wave-B envelopes. Since
  every addition is a comment, this carries no behaviour risk.
- Q6 (main/TD-5 not merged) is correctly out of the worker's control: the guard-test failure is a P08×TD-5
  interaction fixed on `task/P08@7c06b88`, not touched by W-seed; recommendation (a) (rebase after P08 lands) is
  reasonable and does not block this task's own scope (base `task/P08`, no behaviour change).

No git command was run outside `/root/ngfw-wt/W-seed` during this verification; only this file was written/committed.
