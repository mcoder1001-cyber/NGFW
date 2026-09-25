# F-acl — work in progress (slot 3)

Updated 2026-09-25 04:20.

## Done (committed)
- `contract(proto): acl state` (410b5471): AclState RPC + messages, fake stub, proto.md §11, F-acl-contract.md.
- Merged main (TD-8 seams: Env.Resync is wired by the agent, so the re-projection watcher works; TD-7).
- Agent: `desired/acl.go` (projection + AssembleACL), `actions/acl` (expansion record, tracker wrappers, AclState
  runtime, counters flag via `show acl-plugin tables mask`), `subsystems/acl.go` (registration with
  KeyedClaims("acl"), watcher: 60 s schedules, FQDN subscription → RequestResync), `agent/rpc_acl.go`,
  `coretest/acl.go` (+ one line in coretest `New()`), `descriptors/acl/ownership.go` (TD-11b declarations).
  Unit tests green (agent, desired, actions, subsystems), golangci-lint 0 issues.
- API: `features/acl` (AclController + AclService + CSV + bulk + fake), agent client method, app.module hunks,
  api-client + CLI table regenerated.

## Running
- Web UI (`apps/web/src/domains/firewall/acl/**`, locales, router/nav/i18n hunks) — sub-worker.

## Next
- API unit tests (rules, csv) + e2e (`apps/api/test/e2e/acl.e2e.test.ts`).
- `test/topology/acl` on the host VPP (slot 3): commit → `vppctl show acl-plugin acl/interface`, Retrieve == desired,
  counters via the rig ping (globals lock, opt-in), restart simulation with a foreign ACL, rollback, 400 for an empty
  group, 10k then 100k (opt-in, NRestarts around each step), screenshots.
- `docs/user/firewall/acl.md`, `docs/agent/descriptors/acl.md` (F-acl section), status file, CI.
