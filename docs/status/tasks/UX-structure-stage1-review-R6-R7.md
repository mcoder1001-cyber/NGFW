# UX-structure-stage1 — independent R6 / R7 review

Reviewer branch: `codex/ux-structure-review-web-20261007`.
Reviewed development checkpoint: `504698ff0ce0bc46f9ee4a6a0b79feef68dfafef`.
Base: `origin/main`, `19052bb130ab46977dc5b7aa7be25e976fb5aaf9`.
Owned file: this review record only; no product code edits.

## Findings

- MAJOR, R6: `apps/web/src/shell/AppShell.tsx:49`: React Router NavLink evaluates the pathname while the new destinations differ by query string. Its built-in current-page semantics can disagree with the explicit query-aware selection. Developer already identified this and is replacing NavLink with Link. Verify the exact fix checkpoint before final approval.
- MAJOR, R7: `docs/user/firewall/acl.md:3` and `docs/user/firewall/object-model.md:3` still describe the previous menu paths. Update user documentation for Policies, Objects and routing groups; explain retained ACL technical semantics and URL compatibility. Developer agreed to add documentation.
- MINOR, R6: `/routing/objects?tab=unknown` renders the first prefix-list tab but navigation selects parent `/routing` because currentNavPath only normalizes missing tab values. Normalize unknown values or retain as an explicit minor limitation.

## Scope and static checks

Read shared context, contributing, decision policy, UI specification and R6/R7 reviewer prompts. Compared reference `/frontend/src/modules/network/routing/routing-object` and existing NGFW components. The change reuses genuine candidate configuration editors and their existing permissions/errors. No new backend behavior, fake data, dependencies, visual styling, NAT embedding, schema changes or privilege boundaries were introduced. Existing BGP editor tabs remain available. New visible strings use en/fa namespaces and Persian translations. User authorization requires leaving all changes on the independent branch until explicit merge approval.

Executed independently in the reviewer worktree:

```text
python3: recursively compare en/fa JSON key sets for nav, acl and bgp
nav: en/fa key parity PASS (31 keys)
acl: en/fa key parity PASS (309 keys)
bgp: en/fa key parity PASS (152 keys)
```

No node_modules exist in this reviewer worktree, so no runtime test pass is claimed. R1/developer owns runtime and quick gate validation. The development WIP accurately records unfinished checks; it is not treated as an unsupported completion claim. Final review must re-check the final status and documentation checkpoint. No screenshot or live-stack validation is claimed; any unavailable live acceptance must be explicitly deferred by the manager.

R6 verdict: APPROVE WITH CHANGES.
R7 verdict: APPROVE WITH CHANGES.
Combined verdict: APPROVE WITH CHANGES, pending the two agreed fixes and final evidence.
