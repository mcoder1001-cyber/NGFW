# S-capture-retention-stop — capture retention, stop, recovery and contract note (RV-C F-capture-trace; S-capture-file-safety review)
Sources: `docs/status/tasks/RV-C-review-F-capture-trace.md` (BLOCKER 1, MAJOR 2/3/4/6, MINOR 7/8, NIT 9) and
`docs/status/tasks/S-capture-file-safety-review.md` (MINOR 2, NITs 3–5); read `docs/status/tasks/S-capture-file-safety.md` and
`-questions.md` first — that row (merged) rewrote `finish`/`openVPPFile` and already sets state `error` on a failed move (its Q4).
Why it is priority 2: VPP runs one pcap capture at a time, so a capture that cannot be stopped blocks every slot for up to 600 s.
Line numbers below are main@55a18e0f (`apps/agent/internal/actions/capture-trace/capture.go` = capture.go).

## Do
1. **BLOCKER 1** — `retain` (capture.go:664-682) must never evict the newest kept file (the capture `finish` just wrote and `done`
   reports). Reject a plan whose `maxPackets × (snaplen + 16) + 24` exceeds `MaxBytes` before VPP is asked (in `Run`, the caps live
   on the Manager; snaplen 0 = 9000) → `ErrInvalid` on `maxPackets` → API 400 problem+json with a pointer. Tests: bytes cap with
   one oversize file keeps it; oversize plan refused; MaxFiles eviction still drops the oldest.
2. **MAJOR 2 (stop)** — `POST /api/v1/actions/capture/{id}/stop` (admin, audited, `@Protected` like start) aborts the Action call the
   API holds (`startCapture`, agent.client.ts ~:388-420: keep `id → call` and `call.cancel()`); the agent sees ctx cancel →
   `reason` "cancelled" → stop + finish + retain as today. 404 for an unknown id, 409 when the id is not running in this API process.
   Prefer this API-side abort (no contract change); a `CaptureStop` RPC would be an additive `contract(proto)` commit with its own
   `###` section in `docs/status/wave-BC-numbers.md` — only if the abort cannot work, say why. DELETE on a running id stays 409.
   UI: a Stop button on the running row (CapturePage.tsx) with confirm; en/fa keys.
3. **MAJOR 2 (packet limit)** — `Run` (capture.go:419-437) waits the full `seconds` even after `max_packets`: end early when VPP has
   captured `max_packets` (reason "limit"). Use only a read-only signal the pcap family / `apps/agent/binapi/` already expose (names
   from binapi); if VPP 26.06 has none, keep the timer, write it in your questions file — no new `cli_inband` command.
4. **MAJOR 3 (recovery)** — `Recover` runs at agent start (after VPP connects), not on the first RPC: wire it from a new
   `apps/agent/internal/subsystems/capture_trace.go` (or the capture RPC file) with exactly one anchored line
   `// wave-BC: S-capture-retention-stop` in the shared file, listed under `## Shared hunks`; also call it from `Delete` (:804) and
   `Read` (:768). Test: a "running" record + a fresh Manager → after start the record is "interrupted" without any RPC.
5. **NIT 4 (file-safety)** — `Recover` (:688-722) continues past a foreign leftover: that record → state `error` + reason, the
   loop goes on, `recovered = true`; no Internal error after a restart. Test with a planted foreign file.
6. **NIT 3 (file-safety)** — `openVPPFile` (:517-540): add `O_NONBLOCK` to the src open so a planted FIFO cannot block
   `finish`/`Recover` (the fstat regular-file check then refuses it). Test with a FIFO.
7. **NIT 5 (file-safety)** — the refusal reason (`foreign`, :512) shows no absolute path to API roles: keep the file name only (log the
   full path in the agent log).
8. **MAJOR 6** — verify only: a move failure other than ENOENT gives state `error` + reason, never `done` size 0 (test exists? if
   not, add it). No new code unless the test fails.
9. **MAJOR 4 + MINOR 7 (contract note)** — write `docs/status/tasks/F-capture-trace-contract.md` (≤ 30 lines): the additive contract of
   04802324 (`CaptureAction` 7 `drop` / 8 `error_filter` per wave-BC-numbers.md § F-capture-trace; `CaptureList`/`CaptureRead`/
   `CaptureDelete`), the snaplen default change (0 = 65535 → 0 = 9000), `pcap_chunk` never sent, S-capture-file-safety's additions
   (state `error`, 403 on start) and your stop route. Update `docs/contracts/proto.md` capture section to match. In the OpenAPI
   description of `GET /state/captures` say it is unpaged and bounded by `maxFiles`.
10. **MINOR 8** — `startCapture`: log errors that arrive after the start (Nest `Logger`, with the capture id) instead of dropping
    them; `docs/user/tools/capture-trace.md`: an API restart cancels a running capture, the stop button, the size refusal.
11. **NIT 9** — `rpc_capture_trace.go:33-42` re-parses `NGFW_GLOBALS_OWNER`: use the value the agent resolved (`Config.GlobalsOwner`,
    agent.go:58-91) and the wiring's boot store (`subsystems.Wiring.BootStore()`, subsystems.go:510) instead of the private
    `.boot-<owner>.json`; if reaching either needs a file you do not own, leave it and note it in your questions file.
12. **MINOR 2 (file-safety, web)** — CapturePage.tsx:195,200 pipe agent free text through `engineWording` ("the the engine binary
    API"): key the two constant reasons (`trace.reasonBanned`, `pg.reasonNoApi`) in en/fa and map the agent's reason codes to them.

## Files you own
apps/agent/internal/actions/capture-trace/** · apps/agent/internal/agent/rpc_capture_trace*.go ·
apps/agent/internal/subsystems/capture_trace*.go (+ one anchored registration line) · apps/api/src/features/capture-trace/** ·
apps/api/src/agent/agent.client.ts (capture hunk under `// wave-BC: F-capture-trace`) · apps/web/src/domains/tools/capture-trace/** ·
apps/web/src/locales/{en,fa}/capture-trace.json · packages/api-client/src/generated/schema.d.ts (regenerated with `pnpm --filter
@ngfw/api-client gen`, never hand-edited; `contract(api-client): …` commit) · docs/contracts/proto.md (capture section) ·
docs/user/tools/capture-trace.md · docs/status/tasks/F-capture-trace-contract.md · docs/status/tasks/S-capture-retention-stop*

## Out of scope
Streaming the pcap download / sha256 (R5 #10 → TD-H24) · the host run, API e2e and screenshot (→ F-capture-trace-host, which
starts after this merges) · trace and PG features (VPP 26.06 stays "not available") · the TD-H25 /tmp window (handover-gated) ·
new capture options · any change to other features' anchors or to the pcap descriptor in `apps/agent/internal/descriptors/pcap/`.

## Rules
- Not a host row: fake-VPP and unit tests prove every item. If you do touch the shared VPP: slot prefix `w<N>` only, never restart
  or kill VPP, `timeout 10` on every vppctl, packet trace banned (D-128), no BPF filter outside a globals window (launch-queue §3).
- D-210a: own tests pass, each through `tools/heavy.sh` (D-224) — from apps/agent `../../tools/heavy.sh go test
  ./internal/actions/capture-trace/...` and `../../tools/heavy.sh go test ./internal/agent/ -run 'Capture'`; `tools/heavy.sh pnpm --filter
  @ngfw/api exec vitest run src/features/capture-trace`; `tools/heavy.sh pnpm --filter @ngfw/web exec vitest run
  src/domains/tools/capture-trace` — paste the output. No full suite, no lint.
- Contracts: additive only (`contract(api-client|proto): …` commits first; numbers only from docs/status/wave-BC-numbers.md);
  reshaping = PENDING.
- Web parts (item 2 UI, item 12): only after M-origin-sync landed (`git merge-base --is-ancestor origin/main main` succeeds in
  /root/NGFW); then `git merge main` into your branch and follow the product wording rules
  (packages/ui-kit/src/i18n/product-wording.ts, docs/status/tasks/network-defaults-web.md — they arrive with that merge).
- Finish: `tools/ci-slot.sh --base main` green (compile-only, D-220/D-222; never tools/ci.sh directly), commit on your branch,
  `docs/status/tasks/S-capture-retention-stop.md` with one line per finding id (done / verified / left, with the test name) and real output.
