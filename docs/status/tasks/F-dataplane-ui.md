# F-dataplane-ui: Dataplane screen (VPP start-up settings) with preview and gated apply

Branch `claude/modest-keller-upaw4m` (cloud session modest-keller). D-152, WBS D0.6.
Contract: **additive proto change** (`contract(F-dataplane-ui)` commit). Two read-only RPCs,
`DataplaneStartupState` and `DataplaneStartupPreview`, plus messages in a `// ----- F-dataplane-ui -----` section.
The prompt's API scope needs them: no existing RPC exposed the Go renderer or the installed file. No field numbers
were taken from an existing message and there is no schema change. `docs/contracts/proto.md` §11 has an entry.

## What was built
| layer | what |
|---|---|
| agent | `internal/agent/rpc_dataplane_startup.go`: State reads the installed `/etc/vpp/startup.conf` (cpu workers / corelist-workers / main-core, plugin switches via `vppstartup.Parse`/`PluginSwitches`), `/sys/.../cpu/online`, and hugepages total/free from `/proc/meminfo`. Preview runs `vppstartup.ReadHost` + `Generate` on the given `DataplaneConfig` and `UnifiedDiff` against the installed file. Document error → INVALID_ARGUMENT, host → FAILED_PRECONDITION. Env overrides `NGFW_VPP_STARTUP_CONF`, `NGFW_VPP_PLUGIN_DIR`, `NGFW_SYS_ROOT`. Writes nothing, restarts nothing |
| API | `features/dataplane`: `GET /api/v1/state/dataplane` and `POST /api/v1/actions/dataplane/preview` (renders the **candidate** `dataplane`; the response carries `restartRequired: true`, `applyAvailable: false`; agent INVALID_ARGUMENT → 400 problem, pointer `/dataplane`). The fake-agent handlers (`fake.ts`) are wired in `testing/fake-agent.ts`. `AgentClient` methods and the `app.module.ts` line are marked `// F-dataplane-ui (unanchored)`. OpenAPI, api-client and CLI opgen are regenerated |
| web | `domains/system/dataplane/DataplanePage.tsx`: restart warning banner, SchemaForm of `domainSchemas.dataplane` (groups CPU / DPDK and NICs / Memory / Plugins, i18n `dataplane:field`), a candidate-vs-running table with an uncommitted chip, an installed-file + host-facts panel, a preview dialog (diff, rendered file, sha256, warnings) and a **disabled** "Apply and restart VPP" button with the TD-17 reason next to it. Server pointers `/dataplane/...` are mapped onto the fields. Route `/system/dataplane`, `'dataplane'` in `BUILT_DOMAINS` (nav no longer "soon"), en + fa `dataplane.json` |
| docs | `docs/user/system/dataplane.md` (each setting, why a restart is needed, rollback) |

Validation: nothing is duplicated in the UI. Saving goes through the generic PATCH, which runs the schema's semantic
rules (`dataplane.workers-match-corelist`, `main-core-not-worker`, `pci-unique`, descriptor bounds). Their pointers
show at the field.

## Tests run here (cloud sandbox: no VPP, no PostgreSQL, no lab slot)
- agent `go test ./...` all ok. New: `TestDataplaneStartupState`, `TestDataplaneStartupPreview` (fake /sys+/proc tree;
  workers 2 → 4 gives a diff with `-  workers 2` / `+  corelist-workers 2-5`; installed file untouched; bad plugin →
  INVALID_ARGUMENT). `golangci-lint` could not run (binary built with go1.25, module targets 1.26).
- cli `go test ./...` ok (opgen regenerated). proto package vitest ok.
- API: `tsc --noEmit` ok. Unit vitest ok (47 files), including the new `dataplane.test.ts`. The e2e tests were **not run**
  (they need PostgreSQL).
- Web: `tsc --noEmit`, eslint, logical-CSS check ok. The full `vitest run` passed: 71 files / 452 tests, including
  the new page test (nav available, banner, candidate vs running, installed panel, disabled apply + reason, preview
  diff, pointer → field) and the locale parity test. `nav.test.ts` gains `'dataplane'`, because the behaviour changed.
- The agent tests that use `dataplane` as the "unimplemented domain" example are unchanged: the agent still has no
  Apply for `dataplane` (the start-up file is not a commit-time object).
- No restart path: `grep -niE 'restart|systemctl|exec\.Command'` over the new agent/API/web files only matches
  strings and comments.

## Not tested / open
- **No real-endpoint run**: no screenshot against a live agent, no preview on a real host, and `tools/ci.sh --base main`
  was not run (turbo cannot spawn here). The acceptance evidence is still owed on a slot.
- The "running" VPP numbers come from the **installed start-up file**, not from VPP's live thread table
  (`show threads`). They are the same unless someone edited the file without restarting VPP. A live read-back would
  need a VPP API dump. Follow-up if wanted.
- Preview on a host without a detectable management NIC or with no hugepages fails with 409 (FAILED_PRECONDITION),
  as ngfw-startupgen does. No override for the management NIC is exposed.
- Open question (for TD-17): who may press "apply" once it is enabled (admin only? a confirmed-commit style dead-man?).
