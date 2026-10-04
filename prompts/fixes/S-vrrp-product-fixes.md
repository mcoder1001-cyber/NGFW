# S-vrrp-product-fixes — VRRP product bugs found by the host row (VIP drift, address-order drift, VR on a vanished interface)
Source: `docs/status/tasks/F-vrrp-config-sync-host-questions.md` F1–F4 (evidence `docs/status/tasks/F-vrrp-config-sync-host-evidence/`, driver
`test/topology/vrrp/host.sh`) + S-keepalived-validator open question 1 (`docs/status/tasks/S-keepalived-validator.md` "Open questions"). Note: the 422
rollback half of F1 (-60 on the VIP delete) is already handled by S-ifip-delete-idempotent (`descriptors/core/core.go:129-145`, -60 on delete = success) —
re-check it, do not re-fix it.
## Do
1. **F1 — accept-mode VIP retrieved as `<VIP>/<if-len>` and deleted every commit.** While a VPP-engine VR is Master with `acceptMode`, the vrrp plugin adds
   the VIP to the interface; `interface-ip` Retrieve reports it as an extra address of an owned interface, so every commit deletes it (drift + a spurious
   delete). Fix (option a of the finding): `interface-ip` Retrieve ignores an address that equals a VIP of a VR of **this owner** on that interface (from
   `vrrp_vr_dump`, names only from `apps/agent/binapi/vrrp`), through a small hook the vrrp family registers — no vrrp import inside core. Unit test (fake
   VPP / coretest): Master+accept VR → Retrieve == desired, re-commit plans nothing; a foreign VR's VIP is not hidden. Then verify the `enabled:false` commit
   ends APPLIED with no failover (the S-ifip-delete-idempotent path).
2. **F2 — `ha.vrrp.<n>.addresses` order → permanent drift.** `desired.Vrrp` sorts before writing VPP and `AssembleVrrp` (`desired/vrrp.go:348`) returns VPP's order.
   Fix agent-side: `AssembleVrrp` returns the desired order when the address sets are equal (set compare, not slice compare). Test: unsorted desired →
   apply → assemble → `proto.Equal` with desired; `GET /state/drift` empty for that VR.
3. **F3 — keepalived stage needs a pre-started daemon in slot mode.** Do NOT decide whether the agent may spawn keepalived (agent privileges are an owner
   question: `docs/decisions/PENDING-agent-privileges.md` and P10's unit decide them). Build only the non-privileged part: `ErrNotRunning` from the stage
   becomes a clear DryRun/Apply finding ("keepalived is not running for this agent; slot harnesses start it") with a test, and the README /
   `docs/agent/renderers/keepalived.md` say the harness owns the daemon's lifetime in slot mode. Record F3 as open in your questions file.
4. **F4 — a VR whose interface vanished cannot be deleted.** `vrrp_vr_add_del is_add=0` on the dead sw_if_index returns -2. Fix in `descriptors/vrrp`
   `VRDescriptor.Delete` (`vrrp.go:451`): when the interface no longer resolves, log a WARNING with the VR key, drop the agent's record (claim/meta) and
   return success; Retrieve skips a dumped VR whose interface no longer resolves (logged once, never planned for a delete it cannot execute — no plan
   churn). Test with coretest. Add the residue hazard + manual cleanup (re-use the index with a slot loopback, as the host row did) to `docs/agent/descriptors/vrrp.md`.
5. **S-keepalived-validator OQ1** — one line in `docs/agent/renderers/keepalived.md`: "TD-13 Validator: `keepalived -t` on a staged copy with
   `dynamic_interfaces`; Stage = daemon".
6. **Host check** (one per fix F1/F2/F4): only inside a VRRP window the manager grants in your envelope (`NGFW_VRRP_VPP=on` on your slot agent,
   `flock -s /run/lock/ngfw-lab.lock` + `flock -x /run/lock/ngfw-globals.lock`, as `test/topology/vrrp/host.sh` does; prefixed interfaces/VRs only). Paste
   drift before/after, `vppctl show vrrp vr`, NRestarts before/after. Without a granted window: fake-VPP/coretest evidence only, host checks listed as owed.
## Rules
- Files you own: `apps/agent/internal/descriptors/vrrp/**`, `apps/agent/internal/desired/vrrp.go` + `vrrp_test.go` (the board's `desired/ha_vrrp*.go` glob matches no
  file), `apps/agent/internal/descriptors/core/core.go` (the interface-ip Retrieve hook only), `apps/agent/internal/subsystems/{vrrp,keepalived}.go` (hook
  registration, stage finding), `apps/agent/internal/renderers/keepalived/**` (stage/README only), `docs/agent/descriptors/vrrp.md`,
  `docs/agent/renderers/keepalived.md`, `docs/status/tasks/S-vrrp-product-fixes*`. Anything else → questions file.
- Shared VPP: slot prefix `w<N>`, tables N000–N999 (`eval "$(tools/lab env <N>)"`); never restart or kill VPP; `timeout 10` on every vppctl; packet trace banned
  (D-128); VPP-engine VRRP only in a granted window (V22b, D-167). Daemons: keepalived only if your envelope names it, in your slot netns, left stopped.
  Never `vppctl delete host-interface` (V24/D-101).
- D-210a: write tests for your fix and get them passing (`go test ./internal/descriptors/vrrp/... ./internal/descriptors/core/... ./internal/desired/...`),
  from apps/agent through `../../tools/heavy.sh go test …` (D-224); paste output. No full suite, no lint.
- Out of scope: VRRP state/events (F-igp-followups-d), config sync (F-igp-followups-f), new contract fields (`ha.vrrp` stays as is — no reshaping), VRRPv2
  auth, spawning keepalived from the agent, C code in VPP (park to docs/vpp-code-track.md via the questions file).
- Finish: commit on your branch, `docs/status/tasks/S-vrrp-product-fixes.md` with real output (before/after), `tools/ci-slot.sh --base main` green.
