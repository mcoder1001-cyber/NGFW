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

R1 verdict: BLOCK at the original checkpoint pending finding 1 verification. Findings 2–3 are optional MINOR corrections.

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
