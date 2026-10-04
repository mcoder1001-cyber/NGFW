# Task: F-nat46-host — host (lab VPP) evidence and wiring owed by F-nat46   (prepend 00-CONTEXT.md)

## Goal
The feature `F-nat46` was built and merged in a cloud session **without a VPP host**. Its status file
`docs/status/tasks/F-nat46.md` lists, in the section named in your envelope ("Pending host steps" / "Not done here" / "Deferred →
F-nat46-host"), exactly what is still owed on the real VPP of this host (`ngfw-a`, `/run/vpp/api.sock`). Do those steps, nothing else,
and produce the evidence. Where the section says a piece of *wiring* is missing (agent → VPP or FRR → VPP), build that wiring
gap-only inside the files your envelope names.

## Read first
`docs/status/tasks/F-nat46.md` (whole file; the owed section is your checklist), `prompts/features/F-nat46.md` (the acceptance list and the
out-of-scope fence — both still apply), `docs/lab/host-ngfw-a.md`, `docs/lab/shared-host-rules.md`, `docs/decisions/LOG.md` entries the
status file cites, and the "VPP windows" rule in `docs/status/wave-BC-launch-queue.md` §3.

## How
1. `eval "$(tools/lab env <N>)"` (your slot). Record `systemctl show vpp -p NRestarts` **before and after** every host step.
2. Integration tests: `NGFW_INTEGRATION=1` on ONE Go package at a time under `flock -s /run/lock/ngfw-lab.lock` (or `tools/lab lock shared …`),
   only objects with your prefix, cleanup in `t.Cleanup`. Packet steps: `tools/lab rig up w<N>` (af_packet path; `path: af_packet` in the evidence).
3. **Global VPP steps** (the launch-queue §3 list: table 0, det44 enable, LB GC, OSI punt, encap/exporter globals, EI HA, BPF filter):
   run them under `flock -x /run/lock/ngfw-lab.lock` — one holder at a time, keep the window ≤ 10 min, restore the global to its previous
   value inside the same window, and write the window (start/end, what changed) into your status file.
4. Daemons: only the daemon your envelope names, only test-scoped instances in your slot's namespaces / `/run/ngfw-test/w<N>/`; leave it
   stopped. Never touch the system units.
5. UI screenshot (when owed): the slot API + web against the real endpoint; paste the path under `docs/status/tasks/F-nat46-host-shots/`.
6. Every fix you need in product code must stay inside the files your envelope names; anything else → `docs/status/tasks/F-nat46-host-questions.md`.

## Finish
`docs/status/tasks/F-nat46-host.md`: the owed checklist with each item → command + pasted real output (trimmed with `…`), NRestarts
before/after, the windows you held, what is still not possible on this host and why (e.g. handover-gated), open questions.
`tools/ci.sh --base main` green in your worktree. Commit on your branch. Never on `main`, never another worktree, never restart VPP.

## Out of scope
Everything not listed in the owed section: no new features, no refactors, no schema/proto changes (a needed contract change →
questions file, stop that item), no performance claims.

## This instance
- source row: `F-nat46`; owed section: `## Not done / remains` of `docs/status/tasks/F-nat46.md`
- board row: `F-nat46-host` (files_owned: docs/status/tasks/F-nat46-host*)
