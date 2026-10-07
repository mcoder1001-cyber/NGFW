# UX-structure-stage1 — independent R6 / R7 review

Reviewer branch: `codex/ux-structure-review-web-20261007`.
Reviewed development checkpoint: `885e2c1909c2f1095297bf6eacb90745f29b3d28` (initial implementation `504698ff0ce0bc46f9ee4a6a0b79feef68dfafef` plus developer corrections). Followup cherry-picked into reviewer branch without authoring product changes.
Base: `origin/main`, `19052bb130ab46977dc5b7aa7be25e976fb5aaf9`.
Owned file: this review record only; no product code edits.

## Findings

- RESOLVED, R6: `apps/web/src/shell/AppShell.tsx:49`: pathname-only NavLink semantics were replaced with Link plus explicit query-aware `aria-current`; query changes now expand the selected destination's group. The developer added a regression exercising query-only navigation from Ping to static routes.
- RESOLVED, R7: `docs/user/firewall/acl.md:3` and `docs/user/firewall/object-model.md:3` now describe Policies and Objects paths. `docs/user/ux-structure-stage1.md` documents every new group, reused editors, technical ACL semantics, retained URLs and excluded reference-only features.
- RESOLVED, R6: unknown routing object tabs now normalize to the first prefix-list destination, matching the page's rendered fallback.
- MINOR, R7: ancillary host-ACL guide references still use the former Firewall→ACL/Objects labels. The dedicated ACL/object guides and new stage-1 navigation document are accurate. Follow up ancillary wording when preparing final documentation for integration.

## Scope and static checks

Read shared context, contributing, decision policy, UI specification and R6/R7 reviewer prompts. Compared reference `/frontend/src/modules/network/routing/routing-object` and existing NGFW components. The change reuses genuine candidate configuration editors and their existing permissions/errors. No new backend behavior, fake data, dependencies, visual styling, NAT embedding, schema changes or privilege boundaries were introduced. Existing BGP editor tabs remain available. New visible strings use en/fa namespaces and Persian translations. User authorization requires leaving all changes on the independent branch until explicit merge approval.

Executed independently in the reviewer worktree:

```text
python3: recursively compare en/fa JSON key sets for nav, acl and bgp
nav: en/fa key parity PASS (31 keys)
acl: en/fa key parity PASS (309 keys)
bgp: en/fa key parity PASS (152 keys)

tools/ci.sh check --base origin/main
board valid: 212 tasks; read-only validation
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m13s)
```

No node_modules exist in this reviewer worktree, so no runtime test pass is claimed. R1/developer owns runtime and quick gate validation. The development WIP accurately records unfinished checks; it is not treated as an unsupported completion claim. Final test evidence and full unchanged quick gate remain release prerequisites, owned by the manager; this review does not certify them. No screenshot or live-stack validation is claimed. The manager must explicitly record this acceptance gap before integration, per owner-approved deferred-acceptance policy. The user requested an undeployed first stage and has not authorized merge.

R6 verdict: APPROVE.
R7 verdict: APPROVE, one optional ancillary-doc wording followup.
Combined review verdict: APPROVE for the inspected product/documentation checkpoint; runtime gate and evidence prerequisites are independent and still pending.

## Supplemental review

Inspected developer commit `9fa82a6d211cf1daab534f91a3582b26671ab501` read-only: current navigation destination now governs group reopening, preserving intentional collapse across unrelated query changes while reopening when a tab destination changes. The Persian test mounts the page before changing language inside `act`, avoiding a global-language timing assumption. Both changes preserve the requested appearance and configuration behavior. R1 independently owns runtime validation.

Also inspected the pending developer-worktree lint-only change to `RoutingObjectsPage.tsx`: existing `bgp/model` namespace constant replaces the identical literal `bgp`, and constants replace JSX literal translation/domain keys. `NS` is already defined as `bgp`. All values, translation lookup, editors and routes remain identical. No additional finding. This inspection is of the actual uncommitted diff, not a publication claim.

Supplemental R6/R7 verdict: APPROVE; complete quick gate and owner merge authorization remain required separately.
