# Task: F-capture-trace-host — host (lab VPP) evidence and wiring owed by F-capture-trace   (prepend 00-CONTEXT.md)

## Goal
The feature `F-capture-trace` was built and merged in a cloud session **without a VPP host**. Its status file
`docs/status/tasks/F-capture-trace.md` lists, in the section named in your envelope ("Pending host steps" / "Not done here" / "Deferred →
F-capture-trace-host"), exactly what is still owed on the real VPP of this host (`ngfw-a`, `/run/vpp/api.sock`). Do those steps, nothing else,
and produce the evidence. Where the section says a piece of *wiring* is missing (agent → VPP or FRR → VPP), build that wiring
gap-only inside the files your envelope names.

## Read first
`docs/status/tasks/F-capture-trace.md` (whole file; the owed section is your checklist), `prompts/features/F-capture-trace.md` (the acceptance list and the
out-of-scope fence — both still apply), `docs/lab/host-ngfw-a.md`, `docs/lab/shared-host-rules.md`, `docs/decisions/LOG.md` entries the
status file cites, and the "VPP windows" rule in `docs/status/wave-BC-launch-queue.md` §3.
Also: `docs/status/tasks/RV-C-review-F-capture-trace.md` (MAJOR 5 is the checklist below), `docs/status/tasks/S-capture-file-safety.md`
(random 96-bit capture ids, O_NOFOLLOW handling, admin-only routes) and `docs/status/tasks/S-capture-retention-stop.md` (stop route,
start-up Recover, size refusal) — both merged before you start.

## How
1. `eval "$(tools/lab env <N>)"` (your slot). Record `systemctl show vpp -p NRestarts` **before and after** every host step.
2. Integration tests: `NGFW_INTEGRATION=1` on ONE Go package at a time under `flock -s /run/lock/ngfw-lab.lock` (or `tools/lab lock shared …`),
   only objects with your prefix, cleanup in `t.Cleanup`. Packet steps: `tools/lab rig up w<N>` (af_packet path; `path: af_packet` in the evidence).
3. **Global VPP steps** (the launch-queue §3 list: table 0, det44 enable, LB GC, OSI punt, encap/exporter globals, EI HA, BPF filter):
   run them in a manager-serialized window under the globals lock (`flock -x /run/lock/ngfw-globals.lock` inside the shared lab lock,
   D-167, shared-host-rules §7) — one holder at a time, keep the window ≤ 10 min, restore the global to its previous value inside the
   same window, and write the window (start/end, what changed) into your status file.
4. Daemons: none for this row.
5. UI screenshot: owed to T4 on the integrated main stack after merge (D-175). While `plan/NO-TESTS` exists no T4 runs: list it as
   owed in your status file; do not install a browser.
6. Every fix you need in product code must stay inside the files your envelope names; anything else → `docs/status/tasks/F-capture-trace-host-questions.md`.

## Finish
`docs/status/tasks/F-capture-trace-host.md`: the owed checklist with each item → command + pasted real output (trimmed with `…`), NRestarts
before/after, the windows you held, what is still not possible on this host and why (e.g. handover-gated), open questions.
`tools/ci-slot.sh --base main` green in your worktree (compile-only gate, D-220/D-222; never `tools/ci.sh` directly). Commit on your
branch. Never on `main`, never another worktree, never restart VPP.

## Out of scope
Everything not listed in the owed section: no new features, no refactors, no schema/proto changes (a needed contract change →
questions file, stop that item), no performance claims. Specifically: no capture-code fixes (a product bug you find → questions file;
the manager opens a fix row), no trace/PG, no download streaming (TD-H24), no change to `/etc/vpp` or `vpp.service` for the TD-H25
`/tmp` window.

## Evidence rules (D-175)
Evidence logs go to `docs/status/tasks/F-capture-trace-host-evidence/*.txt` (never `.log` — gitignored). NRestarts = pasted `systemctl show vpp -p NRestarts` before/after each step. Merge current `main` into your branch before the final `tools/ci-slot.sh --base main` and paste its tail. Commit every driver script you used. Screenshots: T4 on the main stack after merge — do not install a browser. `PYTHONDONTWRITEBYTECODE=1` for any vpp_papi import from /root/vpp. Global-changing steps: exclusive globals lock inside the shared lab lock (D-167), never `flock -x` on the lab lock.
Anchors: every anchored line you add to a shared file is `// wave-BC: F-capture-trace-host`, and every such hunk is listed under `## Shared hunks` (none expected).
Never `vppctl delete host-interface` by hand (V24/D-101: it crashed the shared VPP on 2026-09-28); cut links at the netns end (`ip link set <ns-veth> down`) and let `tools/lab rig down` remove af_packet interfaces.

## This instance
- source row: `F-capture-trace`; owed sections: `## Not tested (acceptance evidence still owed on a slot)` of
  `docs/status/tasks/F-capture-trace.md` and MAJOR 5 of `docs/status/tasks/RV-C-review-F-capture-trace.md`.
- Stack: your slot's agent (`NGFW_CAPTURE_DIR=/run/ngfw-test/w<N>/captures`, never `/var/lib/ngfw`; `NGFW_GLOBALS_OWNER=0`) + API on
  `NGFW_HTTP_PORT` against `ngfw_w<N>`. VPP holds ONE pcap capture for the whole host: keep each capture ≤ 30 s, only on your rig
  interface `host-w<N>l0`; a 409 from another slot's capture is a wait, not a failure.
- Owed checklist (one evidence file per item):
  1. Capture on `host-w<N>l0` (POST `/api/v1/actions/capture`, admin) while `ip netns exec ns-w<N>-lan ping -c 20 -s 1200 …`; download
     the file and `tcpdump -nr` it: the ICMP packets are there and the count equals the agent's `packets` stat.
  2. `ls -l` of the capture dir: files 0600, dir 0700; `ls -l /tmp/<capture-id>.pcap` after finish → no such file.
  3. 409 `capture-busy` (second start while one runs) and 400 problem+json with pointer `/bpf` (invalid expression) — pasted bodies.
  4. Stop: POST `/api/v1/actions/capture/{id}/stop` ends a running capture early (state + reason); DELETE on a running id → 409.
  5. Agent restart (D-076): start a capture, stop your agent by PID, start it → the record is `interrupted` (reason `agent-restart`)
     without any RPC (start-up Recover), the next start is not busy; log excerpt.
  6. `ls -l` of the capture dir before/after DELETE `/api/v1/state/captures/{id}`; the record is gone from GET `/state/captures`.
  7. `apps/agent/internal/agent/capture_integration_test.go` (`NGFW_INTEGRATION=1`, lab lock, rig interface): capture → file kept 0600 →
     List consistent → Delete; cleanup in `t.Cleanup`. Paste `../../tools/heavy.sh go test -run Capture -v ./internal/agent/` (from apps/agent) output.
  8. `apps/api/test/e2e/capture-trace.e2e.test.ts` (PostgreSQL `ngfw_w<N>` + fake agent, `apps/api/test/support/harness.ts` as in
     `lb.e2e.test.ts`): start/list/file/delete/stop, operator 403 on start/file/delete/stop, 409 busy, 400 `/bpf` pointer, audit rows.
     Paste `tools/heavy.sh pnpm --filter @ngfw/api exec vitest run -c vitest.e2e.config.ts test/e2e/capture-trace.e2e.test.ts`.
  9. BPF capture: only in a manager-granted window with `NGFW_DF8_GLOBALS=1` and the globals lock (step 3 of How): a filtered
     capture keeps only matching packets; the previous filter value is read before and restored after (pasted). If no window is
     granted during your box, mark it owed.
  10. Screenshot (en + fa) → owed to T4 (How 5).
- daemon-owner: none · files you own: `test/topology/capture-trace/**` `apps/api/test/e2e/capture-trace*`
  `apps/agent/internal/agent/capture_integration_test.go` `docs/status/tasks/F-capture-trace-host*`
- Rules: slot prefix `w<N>` on every object; never restart or kill VPP; `timeout 10` on every vppctl; packet trace banned (D-128 —
  pcap capture through the agent is the feature, `trace add`/`show trace` are not); D-210a: your own tests pass (items 7–8),
  through `tools/heavy.sh` (D-224), output pasted, no full suite; no contract changes; stop every process you started (by PID) and `tools/lab rig down w<N>` at the end.
