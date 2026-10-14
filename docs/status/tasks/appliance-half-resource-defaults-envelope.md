# Appliance resource records recovery envelope

- Recovery task: audit two published operation branches and preserve their unique records on current main, 2026-10-14.
- Worker: `ops_audit`; isolated worktree `/workspace/scratch/8d2d1b380cba/ops-work`.
- Branch: `codex/finish-resource-records-20261014`.
- Base: `4f70b3315b9641738f9d193bbad6fb271c9a3d3f`.
- Owned files: `docs/status/tasks/appliance-half-resource-defaults*.md`, the sibling `appliance-half-resource-defaults/` artifact directory, and `docs/status/tasks/restore-ethernet-display-dual-host-wip.md`.
- Source refs: `codex/appliance-half-resource-defaults` at `fc1fcaa4ae0a78b61b3a04442af466deabb1ebb4`; `codex/restore-ethernet-display-dual-host` at `80810da304f9d5eb7b251aafdb3855c7dfe3c592`.
- Scope: documentation and archival source only; no product source changes, SSH, live appliance operations, authentication changes, lab tests, branch deletion or merge by this worker.
- Review: manager assigns an independent read-only reviewer, then handles hosted gates and integration.

The source envelope recorded an earlier owner-authorized live operation by the root manager in `/root/ngfw-wt/appliance-half-resource-defaults`, targeting only `172.30.110.211` and `172.30.126.37`. That historical operation required the existing root transaction and startup safety pipeline, with `review_unit` as read-only reviewer and no shared-lab VPP edits. Recovery of this record does not authorize replay of its scripts.
