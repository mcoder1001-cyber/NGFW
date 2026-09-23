# P13 — WIP (basic CLI)

Updated: 2026-09-24 03:56 (+0330)

## Done
- `apps/cli` Go module (`ngfw/cli`, only dep golang.org/x/sys already used by apps/agent): operation table generated from
  the OpenAPI document (`cmd/vrx-opgen` → `internal/api/operations_gen.go`), REST client by operationId, live JSON Schema
  walker (completion, coercion, client-side validation), path↔pointer mapping, text/set renderers, own line editor
  (raw termios, history, Tab, `?`), operational + configuration commands, exit codes, `--json`.
- Slot-3 dev stack script `apps/cli/test/devstack.sh` (real agent + API, PIDs in /run/vrx-test/w3).
- Manually verified against the real stack: login, show system/interfaces/configuration, set/merge/delete with client-side
  rejection (exit 2), diff, commit (revision 1, VPP loop301 + 10.3.101.1/24), commit confirm 5 → auto-revert observed in
  API (pending gone, running mtu back) and VPP (address back).

## Next
- unit tests (cpath, render, jschema, opgen drift, cli with fake API), docs generator → docs/user/cli/reference.md
- pty-driven e2e (expect is not installed on the host → Go pty harness), RBAC readonly evidence, Makefile, CI gate, P13.md
