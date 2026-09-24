# P13 — questions for the manager

1. **tools/ci.sh does not see apps/cli.** apps/cli is a Go module that is neither a pnpm workspace package nor under
   `test/`, so the quick gate never builds, lints or tests it (and `CONTROL_PLANE_PATHS` does not grep it).
   Proposal (manager-owned file): add a step like the apps/agent one — `make -C apps/cli lint test build` (lint includes
   the `check` target: no VPP tool / VPP socket / govpp / agent socket in apps/cli) — and add `apps/cli` to the
   forbidden-pattern paths. Until then I ran `make -C apps/cli all` by hand (output in P13.md).
   Options: (a) ci.sh step (recommended) (b) make apps/cli a pnpm workspace package whose scripts call go (needs a
   pnpm-lock.yaml importer entry — not my file) (c) leave it manual.

2. **Unix-socket transport with peer-credential check (P13 §1).** The API listens on TCP only; accepting a unix socket
   and mapping `SO_PEERCRED` (uid 0 / group `vrx-admin`) to a principal is an apps/api change (not my files). The CLI
   authenticates with login or API key. Proposal for a follow-up (P06-owner or F-aaa): `VRX_HTTP_SOCKET=/run/vrx/api.sock`
   (0660 root:vrx-admin) + a guard that maps peer uid→local user (role from group) and audits `via: "peercred"`; the
   CLI then gets `--api unix:/run/vrx/api.sock` (a Dial override in `internal/api`, ~20 lines).

3. **No state endpoints for `show bgp summary` / `show ipsec sa`.** Both commands exist and exit 10 (not implemented)
   with the reason; `ping`/`traceroute` call `POST /api/v1/actions/{action}` which answers 501 (P08). They need
   `/state/bgp` (P12/RF-1) and `/state/ipsec/sas` (P11) — the CLI side is one function each.

4. **`expect` is not installed** on the host (no tcl either) and workers install no packages. The e2e therefore drives
   the REPL through a real pseudo-terminal from Go (`apps/cli/test/e2e/pty_test.go`, x/sys/unix — Tab, `?`, raw mode
   exercised as with expect). If an expect script is still wanted, it needs `apt install expect` by the manager.

5. **Docs path.** The prompt says `docs/user/cli.md`; the envelope gives me `docs/user/cli/**`. The generated reference is
   `docs/user/cli/reference.md`. A `docs/user/cli.md` stub linking to it can be added by whoever owns `docs/user/`.

6. **Live schema source.** Completion and client-side validation read the configuration schema from the OpenAPI JSON
   the API serves at `GET /api/docs-json` (authenticated; NestJS Swagger default path). That URL is not an OpenAPI
   operation itself. Should it become a documented route (e.g. `GET /api/v1/schema/config` returning the JSON Schema
   that packages/schema already generates for the UI)? Additive; the CLI would switch in one line.

7. **This agent build does not apply `interfaces.mtu`** (commit warning "interfaces.mtu is not implemented by this agent
   build (DF-1/P08)"). The e2e therefore proves the confirmed-commit revert with MTU at the API level (running MTU back to
   1500, no pending commit) and in the data plane with the interface address the agent does apply (.9 → .1 in the
   agent's Retrieve). When DF-1/P08 lands, the same test can assert the MTU in `show interfaces`.
