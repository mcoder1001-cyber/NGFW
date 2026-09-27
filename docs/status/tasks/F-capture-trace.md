# F-capture-trace: pcap capture (Tools → Packet capture); trace / PG reported unavailable

Branch `claude/modest-keller-upaw4m` (cloud session modest-keller). WBS D8.2 / D7.11.

## Contract (separate `contract(F-capture-trace)` commit, numbers from wave-BC-numbers.md)
- `CaptureAction` + `7 drop`, `8 error_filter`; comments updated (snaplen 0 = 9000, max 9000; the file is kept by the
  agent, the first line is `capture <id> started`, no `pcap_chunk` is sent).
- RPCs `CaptureList`, `CaptureRead` (server stream of `CaptureChunk`) and `CaptureDelete`. The agent keeps the files.
  The list also carries `trace_available`/`pg_available` (always false) with a reason.
- PG RPCs were **not** added: no PG is built (see below).

## What was built
| layer | what |
|---|---|
| agent | `internal/actions/capture-trace`: Validate (defaults, bounds, BPF through `trace.BPFFilter.Validate`, error text `<field>: …`). Run: BPF filter + filter function (globals owner only, else FAILED_PRECONDITION), then `pcap.capture` Create (busy → ABORTED) and progress lines. It stops on timeout or cancel via Delete, which only stops this owner's capture (D-076), and restores the filter. It moves `/tmp/<id>.pcap` to `VRX_CAPTURE_DIR` (default `/var/lib/vrx/captures`, dir 0700, file 0600, copy across file systems), counts pcap records, takes the sha256 and keeps a JSON record. Retention: 10 files / 500 MiB. Recover: a record left `running` by an earlier process is stopped and marked `interrupted` (`agent-restart`, or `vpp-restart` when the boot identity changed). `internal/agent/rpc_capture_trace.go` has the Action case under the anchor and the three RPCs |
| API | `features/capture-trace`: `POST /api/v1/actions/capture` (202 `{id}`, a static route that wins over `:action`; busy → 409 `capture-busy`; agent INVALID_ARGUMENT → 400 with `pointer`; zod rejects BPF outside the alphabet → 400 `/bpf`). `GET /api/v1/state/captures`, `GET /api/v1/state/captures/{id}/file` (admin, audited by an explicit audit row, `application/vnd.tcpdump.pcap`), `DELETE /api/v1/state/captures/{id}` (admin). Fake-agent handlers are in `fake.ts` (TD-23 `registerActionHandler('capture')`) |
| web | `domains/tools/capture-trace/CapturePage.tsx`: form (interface, direction, drop, BPF with local alphabet check, limits), live progress (2 s poll while running), file list with Download/Delete (admin). Trace/PG tabs show "Not available on this build" + the agent's reason. The Tools route/nav item replaces the old NotAvailable one. en + fa |
| docs | `docs/user/tools/capture-trace.md` |

Shared-file edits outside the anchors, needed because behaviour changed: `internal/agent/rpc_nat44_ed_test.go` and
`project_vrf_static_ecmp_test.go` used `capture` as their "unimplemented action" example. They now use an empty request
and an invalid capture. `apps/api/src/testing/fake-agent-action.test.ts` gains the two new CaptureAction fields.
`router.tsx` drops the now-unused `NotAvailableByKey`. `nav.ts` drops the `tools` placeholder item.

## Not built / decisions
- **Trace**: not built. TD-20/D-128 bans trace on the shared VPP, and there is no tracedump binapi (V18). Typed reason in CaptureList.
- **PG**: not built (no `descriptors/pg`). Stream definition has no binary API. `pg_create_interface`/`pg_capture` on
  their own give no usable feature, and adding the package would grow the TD-11a pending allowlist (shrink-only). `pcap`/`trace` stay
  `pending` in reachability_test.go: they are driven by the action handler, not `subsystems.register()`, so maxPending is unchanged.
- Recovery after an agent restart runs on the first capture RPC, not at start-up (there is no start-up anchor).
- Open questions (default kept): no interim `cli_inband` trace. Retention defaults 10 files / 500 MiB.

## Tests run here (cloud sandbox: no VPP, no PostgreSQL, no lab slot)
- agent: `go build ./...`, `go vet ./...`, `go test ./...` (see the PR for the known contracttest drift). The new
  `capture-trace` tests (also with `-race`) cover: file moved and 0600, not left in the VPP dir, filter restored, read/list/delete,
  busy (own + other owner's), BPF without globals owner, another owner's interface, retention, and agent-restart
  recovery. `TestCaptureStatus` pins the code mapping. golangci-lint clean on the new packages.
- api: `tsc`, unit vitest (new `capture-trace.test.ts`). **e2e not run** (needs PostgreSQL).
- web: tsc, eslint, vitest (new `CapturePage.test.tsx`, nav test updated).

## Not tested (acceptance evidence still owed on a slot)
There was no real capture while pinging and no `tcpdump -r` output. The busy/400 checks, the restart log excerpt and
`ls` before/after a delete on a live rig were not run, and there is no screenshot. `tools/ci.sh --base main` was not run.
