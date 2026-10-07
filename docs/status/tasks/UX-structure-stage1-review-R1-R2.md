# UX structure stage 1 — independent R1/R2 review

- Reviewer branch: `codex/ux-structure-review-correctness-20261007`.
- Reviewer worktree: `/root/ngfw-wt/ux-structure-review-correctness-20261007`.
- Product checkpoint reviewed: `504698ff0ce0bc46f9ee4a6a0b79feef68dfafef`; base `19052bb130ab46977dc5b7aa7be25e976fb5aaf9`.
- Ownership: this review file only. No product edits, service changes, or full-gate execution.
- Task: navigation/component reuse and Policy labels, with independent NAT, legacy ACL compatibility, no backend or styling redesign; no merge until user approval.

## R1 findings

1. BLOCKER (known developer fix pending), `apps/web/src/shell/AppShell.tsx:49`: NavLink computes active state using pathname alone and can overwrite `aria-current` for links targeting different `?tab=` values. Multiple entries for `/firewall/objects` consequently identify themselves as current even though only one destination is selected. Use Link with the explicit currentNavPath selection and retain the App regression that asserts exactly one current link.
2. MINOR, `apps/web/src/shell/AppShell.tsx:124`: opening the page's group watches only pathname. Switching `/routing?tab=static` to `/routing?tab=ping` selects a Tools destination without opening a previously collapsed Tools group. Observe the full location or current destination and add a query-only cross-group regression.
3. MINOR, `apps/web/src/nav/nav.ts:250`: `/routing/objects?tab=unknown` renders the default prefix-list editor but selects `/routing` (Static routes). Normalize invalid routing-object tabs to the first tab as well as absent tabs, and test the fallback.

The new routing-object page uses the existing lazy prefix-list and route-map editors against `routing.policy`, rather than introducing BGP-specific storage or duplicating schemas. NAT still targets `/firewall/nat`. Both policy URLs load the same editor and preserve selection parameters. Static/dynamic routing categories retain existing screen implementations. Objects use the actual existing tab identifiers.

Original checkpoint R1 verdict: BLOCK pending finding 1 verification. Superseded by the final verify round below.

## R2 findings

No actionable security findings. The new routes remain children of RequireAuth, the reused routing RecordEditor retains editConfig checks, and API authorization/session/storage mechanisms are unchanged. No contract/API/agent/dependency changes, secret fields, shell invocation, or privileged host actions were introduced.

R2 verdict: APPROVE.

## Independently executed validation

`tools/ci.sh check --base origin/main`:

```text
no contract files changed in the 1 commit(s) of HEAD since origin/main (19052bb13)
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: ngfwtestsecrets only in test code
ok: gitleaks — scanned ~12731 bytes (12.73 KB) in 535ms no leaks found
board valid: 212 tasks; read-only validation
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m13s)
```

Existing ALLOW lines were unchanged test-only OpenSSL calls outside this diff.

A standalone Node assertion harness transpiled the reviewer worktree's actual `nav.ts` with TypeScript (read-only compiler package from developer dependencies; only the DEV_ROUTES import supplied as false). Output:

```text
PASS: 6 independent navigation assertions; legacy ACL alias, objects zones, ping, routing objects default and explicit tabs.
Invalid routing-object tab selects: /routing
```

The full quick gate and React regression suite are manager-owned in this review envelope; they are not claimed as independently run here. No laboratory acceptance is relevant to this UI-only placement change.

## Recovery / next action

- Await developer followup SHA for NavLink correction and optional query-navigation fixes.
- Next command: `git merge --ff-only <developer-followup-SHA>` in the isolated review worktree before this review-file commit, or inspect `git show <SHA>` if the review branch has already diverged.
- Publish this reviewer branch after every review checkpoint; remote SHA is reported to the manager only after successful push. Product merging remains forbidden without owner approval.


## Final verify round — product `885e2c1909c2f1095297bf6eacb90745f29b3d28`

Merged the published followup into this isolated reviewer branch solely to review its actual final product files. No product code was authored by the reviewer.

- Finding 1 resolved: links now use Link with explicit `aria-current`, and NavLink-only `end` was removed. Existing App regression checks one current Objects link and policy alias selection.
- Finding 2 resolved: group reopening observes pathname plus search. New App regression navigates static routes to Ping without a pathname change and checks Tools opens and Ping is current.
- Finding 3 resolved: missing, unknown, and empty routing-object tabs select the prefix-list destination consistent with the page's fallback. New navigation regressions cover absent and unknown tabs.
- ACL Persian heading expectation now matches Policy naming. Documentation additions preserve technical ACL semantics, explain standalone NAT, shared routing object editors and legacy URLs, and explicitly exclude unsupported reference zone behavior.

Independent Node harness against final reviewer nav source:

```text
PASS: 8 independent navigation assertions, including absent/invalid/empty routing-object tabs.
PASS: source checks confirm Link and full-location group reopening.
```

Final security check was rerun with `tools/ci.sh check --base origin/main`; gitleaks scanned 21.07 KB with no leaks, and the command ended `check PASSED (0m14s)`. Full-gate and React test results remain manager-owned and are prerequisites to merge readiness; no merge is authorized by this review.

Final R1 verdict: APPROVE — zero outstanding BLOCKER/MAJOR/MINOR findings.
Final R2 verdict: APPROVE — zero findings.
Next command: `git push origin codex/ux-structure-review-correctness-20261007`, then report the actual remote reviewer SHA to the manager. No product merge until explicit owner approval.

## Supplemental followup review — collapsed navigation preference

Read-only reviewed the two pending developer changes on top of `b31d51736daeffa516f82e341f64b8da26992a0a` in the developer worktree; no developer file was modified by the reviewer. Publication of the final developer SHA and passing manager-run tests remain required.

- `AppShell.tsx` destination key now combines pathname and currentNavPath instead of the raw query. This retains a user's collapsed-group preference when irrelevant query parameters change while still reopening a group for a tab link that selects a different navigation destination. The existing VPN unrelated-query regression and new Static-to-Ping regression cover both behaviors.
- `App.test.tsx` waits for the mounted Routing objects page before changing language inside act. UiSettingsProvider sets language from persisted settings after mount; the revised order tests the actual live language-change behavior and avoids racing that initialization. Assertions for Persian heading and RTL remain present.

Independent Node destination-key harness against reviewer nav source:

```text
PASS: 4 destination-key assertions preserve unrelated-query collapse state and distinguish routing tab destinations.
```

Supplemental R1 verdict: APPROVE for the reviewed working diff, subject to a published developer checkpoint and manager-run regression/gate results.
Supplemental R2 verdict: APPROVE; no security-sensitive changes.

## Final published-source supplementary review

Exact published source reviewed read-only: `f6d8315b6f290aa32de488820308b213d7e13657`. Inspected this commit and the intervening product delta from `885e2c190` with `git show`/`git diff`; no product edits or additional merges were performed.

- The previously approved destination key correction is present in committed AppShell source.
- `App.test.tsx` is identical to origin/main; existing collapsed-group preference coverage remains unchanged.
- New Policy alias, exact current Objects destination, shared routing-object/Persian RTL, and query-only cross-group regressions moved intact into `nav/navigation-structure.test.tsx`.
- Persian RTL test now stores the actual `ngfw.ui.settings` language before mounting. That matches `loadSettings` and UiSettingsProvider initialization; assertions retain the Persian heading, RTL direction and selected route-map tab. afterEach clears storage/session and restores English to avoid cross-test settings leakage.
- No security-sensitive product changes or new secret material are introduced by this followup.

Manager reports the full web suite passed: 109 files, 644 tests. This reviewer did not independently rerun that suite and does not relabel the manager's result as independent evidence. Prior independent nav assertions and security check evidence remain above; complete quick-gate success is a separate manager prerequisite.

Final R1 verdict on `f6d8315b6f290aa32de488820308b213d7e13657`: APPROVE, zero outstanding findings.
Final R2 verdict on `f6d8315b6f290aa32de488820308b213d7e13657`: APPROVE, zero findings.
No merge authorized; explicit owner approval remains required.
