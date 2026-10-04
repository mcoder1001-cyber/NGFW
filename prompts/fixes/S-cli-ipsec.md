# S-cli-ipsec — CLI `show ipsec sa` / `show ipsec tunnels` over the P11 state routes (replace the notImplemented stub)
Source: `docs/status/tasks/P11-questions.md` Q7 (round 1, R7 M1): `apps/cli/internal/cli/cmd_op.go:41-48` still registers
`show ipsec sa` as `notImplemented` (exit 10) although `GET /api/v1/state/ipsec/sas` and `/tunnels` exist
(`apps/api/src/features/ipsec/ipsec.controller.ts:241-266`; operations `Ipsec_sas` {tunnel, offset, limit ≤ 1000, response `total`} and
`Ipsec_tunnels` {tunnel} are already in the generated `apps/cli/internal/api/operations_gen.go:82-83`).
## Do
1. In a new `apps/cli/internal/cli/cmd_op_ipsec.go`: `show ipsec sa [<tunnel>]` (Ops `Ipsec_sas`) pages with `offset`/`limit=1000` until
   `offset >= total` or an empty page (as `showRoutes` pages, cmd_op.go:319-338), and `show ipsec tunnels [<tunnel>]` (Ops `Ipsec_tunnels`).
   Text output through `a.emit` + `table`: SAs — tunnel, IKE state, local→remote host, then one indented row per CHILD_SA (state,
   SPI in/out, bytes/packets in/out, rekey s); tunnels — tunnel, conn, status (up/connecting/down), local/remote addrs, IKE version,
   children. Header lines for `charonRestarted` (warning), `unlistedSas`, a non-empty `pendingAction`, `retrievedAt`. `--json` = the API body.
   Never print key material (the API returns none; do not add fields).
2. Remove the stub registration from `cmd_op.go` (that hunk only) and the "`show ipsec sa` … exit 10" half of the sentence in
   `apps/cli/internal/cli/docs.go:110` (BGP keeps its line); regenerate `docs/user/cli/reference.md` with `make -C apps/cli docs` (generated —
   never hand-edit; its staleness test is in cli_test.go). If `make -C apps/cli gen` reports `operations_gen.go` stale after you merge
   main (M-origin-sync regenerates it), regenerate — never hand-edit.
3. Tests in `apps/cli/internal/cli/cmd_op_ipsec_test.go` with the existing httptest fixture of `cli_test.go`: two pages of SAs (offset
   advances, all rows printed once), tunnel filter passed as `tunnel=`, empty answer, a 400 problem for a bad tunnel name → the CLI's usual
   exit code, `--json` passthrough; `TestEveryCommandMapsToDocumentedREST` stays green.
4. Evidence: `eval "$(tools/lab env <N>)"`, one real run of your branch's CLI (`cd apps/cli && go run ./cmd/ngfw --api
   http://127.0.0.1:$NGFW_HTTP_PORT show ipsec tunnels`, then `… show ipsec sa`) against your slot API (paste whatever the slot stack answers:
   a list, an empty list, or the API's problem when charon is not running — the CLI must render each cleanly).
## Out of scope
New API routes or fields, `show bgp summary`, completion of tunnel names, initiate/terminate actions (ActionRequest 8), strongSwan or
VPP access from the CLI (the CLI only talks to ngfw-api), any change under `apps/api/**` or `packages/**`.
## Rules
- Files you own: `apps/cli/internal/cli/cmd_op*.go` (new `cmd_op_ipsec*.go` + the stub hunk of `cmd_op.go`), `apps/cli/internal/cli/docs.go`
  (the :110 sentence), `docs/user/cli/reference.md` (regenerated), `docs/status/tasks/S-cli-ipsec*`. Everything else read-only; a needed edit
  elsewhere → `docs/status/tasks/S-cli-ipsec-questions.md`.
- Does not touch VPP or any daemon (strongSwan stays with its daemon owner); if you ever need vppctl: slot prefix only, never restart
  or kill VPP, `timeout 10`, packet trace banned (D-128).
- D-210a: your tests pass (`cd apps/cli && ../../tools/heavy.sh go test ./internal/cli/...`, D-224), output pasted; no full suite, no lint, no
  other packages' tests.
- No contract change.
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never tools/ci.sh directly), commit on your branch,
  `docs/status/tasks/S-cli-ipsec.md` with pasted real output.
