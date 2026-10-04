# Task: F-tunnels-host — host (lab VPP) evidence and wiring owed by F-tunnels   (prepend 00-CONTEXT.md)

## Goal
The feature `F-tunnels` was built and merged in a cloud session **without a VPP host**. Its status file
`docs/status/tasks/F-tunnels.md` lists, in the section named in your envelope ("Pending host steps" / "Not done here" / "Deferred →
F-tunnels-host"), exactly what is still owed on the real VPP of this host (`ngfw-a`, `/run/vpp/api.sock`). Do those steps, nothing else,
and produce the evidence. Where the section says a piece of *wiring* is missing (agent → VPP or FRR → VPP), build that wiring
gap-only inside the files your envelope names.

## Read first
`docs/status/tasks/F-tunnels.md` (whole file; the owed section is your checklist), `prompts/features/F-tunnels.md` (the acceptance list and the
out-of-scope fence — both still apply), `docs/lab/host-ngfw-a.md`, `docs/lab/shared-host-rules.md`, `docs/decisions/LOG.md` entries the
status file cites, and the "VPP windows" rule in `docs/status/wave-BC-launch-queue.md` §3.
Also: `docs/status/tasks/RV-C-review-F-tunnels.md` (MAJOR 1 + 3 are your checklist) and `docs/status/tasks/S-tunnels-contract.md`
§ "Slot run against the real engine — owed" (its driver `docs/status/tasks/S-tunnels-contract-evidence/t1-slot.sh` was never run:
VPP's CLI hung host-wide on 2026-09-28; that evidence is owed to this row).

## How
1. `eval "$(tools/lab env <N>)"` (your slot). Record `systemctl show vpp -p NRestarts` **before and after** every host step.
2. Integration tests: `NGFW_INTEGRATION=1` on ONE Go package at a time under `flock -s /run/lock/ngfw-lab.lock` (or `tools/lab lock shared …`),
   only objects with your prefix, cleanup in `t.Cleanup`. No packet step is owed by this row (no rig needed).
3. **Global VPP steps**: none for this row — tunnels, loopbacks and tables are per-object. If you find you need one, stop and write it
   in your questions file.
4. Daemons: none for this row.
5. UI screenshot (en + fa, RV-C MAJOR 3): owed to T4 on the integrated main stack after merge (D-175). While `plan/NO-TESTS` exists
   no T4 runs: list it as owed in your status file; do not install a browser.
6. Every fix you need in product code must stay inside the files your envelope names; anything else → `docs/status/tasks/F-tunnels-host-questions.md`.

## Finish
`docs/status/tasks/F-tunnels-host.md`: the owed checklist with each item → command + pasted real output (trimmed with `…`), NRestarts
before/after, the windows you held, what is still not possible on this host and why (e.g. handover-gated), open questions.
`tools/ci-slot.sh --base main` green in your worktree (compile-only gate, D-220/D-222; never `tools/ci.sh` directly). Commit on your
branch. Never on `main`, never another worktree, never restart VPP.

## Out of scope
Everything not listed in the owed section: no new features, no refactors, no schema/proto changes (a needed contract change →
questions file, stop that item), no performance claims. Specifically: no tunnel-code fixes (a product bug → questions file; the
manager opens a fix row), no `docs/user/vpn/tunnels.md` edit (S-tunnels-contract left it; not yours), no GTP-U forwarding entries,
no PPPoE sessions, no 6RD run, and **never create an L2TPv3 tunnel on the shared VPP** (VPP 26.06 cannot delete it — permanent residue).

## Evidence rules (D-175)
Evidence logs go to `docs/status/tasks/F-tunnels-host-evidence/*.txt` (never `.log` — gitignored). NRestarts = pasted `systemctl show vpp -p NRestarts` before/after each step. Merge current `main` into your branch before the final `tools/ci-slot.sh --base main` and paste its tail. Commit every driver script you used. Screenshots: T4 on the main stack after merge — do not install a browser. `PYTHONDONTWRITEBYTECODE=1` for any vpp_papi import from /root/vpp. Global-changing steps: exclusive globals lock inside the shared lab lock (D-167), never `flock -x` on the lab lock.
Anchors: every anchored line you add to a shared file is `// wave-BC: F-tunnels-host`, and every such hunk is listed under `## Shared hunks` (none expected).
Never `vppctl delete host-interface` by hand (V24/D-101: it crashed the shared VPP on 2026-09-28).

## This instance
- source row: `F-tunnels`; owed sections: `## Not tested / not built` of `docs/status/tasks/F-tunnels.md` (host part only), RV-C
  MAJOR 1 + 3, and the S-tunnels-contract slot run.
- Stack: your slot's agent (`NGFW_GLOBALS_OWNER=0`, `NGFW_VPP_TABLE_BASE=<N>000`) + API on `NGFW_HTTP_PORT` against `ngfw_w<N>`, as
  `t1-slot.sh` starts them. Tunnel `instance` values and any underlay/overlay VRF come from your slot range N000–N999 (rule
  `tunnels.instance-range`); the source address lives on a prefixed loopback `loop<N>0xx` in `10.<N>.0.0/16`, created through the
  API like `t1-slot.sh` does (the agent's creators reset ip4/ip6 classify, D-185); a `vppctl create …` in your driver needs
  `set ip classify intfc … table-index -1` (+ ip6) within 15 lines or the quick gate's classify guard fails.
- Driver: copy `t1-slot.sh` to `test/topology/tunnels/run.sh` (keep its stack/cleanup logic) and extend it; wrap every `vppctl` in
  `timeout 10`; add a `--dry-run` mode and pass `bash -n`.
- Owed checklist (one evidence file per item):
  1. Commit one GRE (l3, p2p), one IPIP and one VXLAN tunnel through the slot API → `GET /api/v1/state/tunnels` and the agent's
     Retrieve equal the desired document; `vppctl show gre tunnel`, `show ipip tunnel`, `show vxlan tunnel` list them with
     src/dst/instance/VRF (pasted).
  2. The S-tunnels-contract run: GRE without `instance` → 400 `tunnels.instance-required` with pointer; GRE + VXLAN-GPE committed →
     `show vxlan-gpe tunnel`, `GET /state/tunnels`, drift empty, re-commit is a no-op.
  3. Duplicate (src, dst, vni) VXLAN → 400 problem+json whose `pointer` names the second entry (pasted body).
  4. Agent-restart simulation (FAST MODE DoD 3): stop your agent by PID, delete your prefixed tunnels via binapi (vpp_papi or a Go
     helper — not by hand-picking other objects), start the agent → the tunnels and their addresses are back within 30 s (log
     excerpt + show output with timestamps).
  5. Rollback to the revision before step 1 → Retrieve empty for your owner, the `show … tunnel` outputs no longer list your
     instances, no `tunnels.meta` leftover.
  6. `apps/agent/internal/agent/tunnels_integration_test.go` (`NGFW_INTEGRATION=1`, lab lock; pattern of `srv6_integration_test.go`):
     apply gre/ipip/vxlan → Retrieve == desired → VPP dump contains them → restart simulation → rollback leaves nothing; cleanup in
     `t.Cleanup`. Paste `../../tools/heavy.sh go test -run Tunnels -v ./internal/agent/` (from apps/agent) output.
  7. Screenshot (en + fa) → owed to T4 (How 5).
- daemon-owner: none · files you own: `test/topology/tunnels/**` `apps/agent/internal/agent/tunnels_integration_test.go`
  `docs/status/tasks/F-tunnels-host*`
- Rules: slot prefix `w<N>` on every object; never restart or kill VPP; `timeout 10` on every vppctl; packet trace banned (D-128);
  D-210a: your own test (item 6, through `tools/heavy.sh`, D-224) and the driver's `bash -n`/dry-run pass, output
  pasted, no full suite; no contract changes; stop every
  process you started (by PID) and drop your slot database objects the driver created.
