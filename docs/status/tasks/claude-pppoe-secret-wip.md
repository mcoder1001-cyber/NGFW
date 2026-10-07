# F-pppoe-client secret resolution fix (claude-pppoe-secret, 2026-10-06)

Branch `claude/pppoe-secret-20261006` from origin/main `75434a3e5`, worktree `/root/ngfw-wt/claude-pppoe-secret`.
Local commits only (not pushed, per owner instruction for this task). Fix commit `fd6c5788d`; this doc is committed on top.

Owned files: `apps/agent/internal/descriptors/pppoe/{client_config.go,client_config_test.go,client_secretchannel_test.go}`,
`apps/agent/internal/subsystems/pppoe_client.go`, `apps/agent/internal/agent/agent.go` (one wiring line),
`docs/status/tasks/claude-pppoe-secret-wip.md`, `docs/status/tasks/claude-pppoe-secret-evidence/**`.

## Root cause

Defect 1 of `claude-pppoe-wip.md`. `agent.go` wired PPPoE with `SetPppoeSecrets(cfg.Owner, cache)` (a `vpn.Resolver`),
and `ClientConfig.sessions` called `resolver.Resolve(ctx, "password/<name>")`. `secretchannel.Store.Resolve` matches
keyed fingerprints (`keyer.Ref(value)`, the output of `Store.Ref`) only, so a literal config reference never resolved:
validation fell back to a placeholder (non-strict) and every real Apply rolled back with
"PPPoE password reference is unavailable". The unit-test fixture resolver ignored the reference, hiding it.

## Fix

Same pattern as FRR (`SetFRRSecrets(owner, cache.Text)`) and PKI (`SetPKISecrets(owner, cache.Text)`):
- `subsystems.SetPppoeSecrets(owner, source func(string) ([]byte, error))`; `agent.go` passes `cache.Text`
  (literal reference lookup in the transaction-selected snapshot).
- `pppoe.ClientConfig.SetResolver(vpn.Resolver)` replaced by `SetSecretSource(func(string) ([]byte, error))`;
  `sessions` calls it with the reference. Material is still copied, added to the redactor and cleared as before.
  The type change makes passing the fingerprint-only `Resolve` a compile error.
- Tests: new `TestClientResolvesPasswordFromRealSecretChannel` (real `secretchannel.Store`, staged + activated
  bundle, Validate + Create, manifest holds only the reference, password only in 0600 Secret chap/pap files, absent
  reference fails closed without leaking). Fixture in `client_config_test.go` is now key-strict.

Secret guarantees unchanged: no new logging; errors still pass through the redactor.

Caveat (unchanged design, same as PKI): `Text` reads the active snapshot. A scheduler rollback inside a failed
transaction re-renders with whatever snapshot is active at that time (FRR additionally has
`SetFRRSecretGenerations` for historical fingerprints; PPPoE does not). Confirm-timeout revert re-activates the
confirmed snapshot first (`service.go` revert) and passed live.

## Test results (actual)

- Fail before (main + regression test only): `claude-pppoe-secret-evidence/01-before-fix.txt` —
  `apply with sealed password: PPPoE password reference is unavailable`.
- Pass after: `02-after-fix.txt`.
- `go test -race -count=1` pppoe descriptor, secretchannel, subsystems, agent: all ok (`03-race-packages.txt`).
- `TMPDIR=/root/.cache/ngfw-ci-host tools/ci.sh quick --base origin/main`: **CI GATE PASSED** (16m10s,
  `04-ci-quick.txt`). First run failed on revive unused-parameter `ctx` in `sessions` (renamed `_`, amended).

## Live product run (slot 9, `05-live-product-run.txt`)

Driver: copy of `/root/ngfw-wt/claude-pppoe` `test/topology/pppoe/{run.py,pppoe_live.go}` (claude-pppoe `bc9a69f96`)
in `.scratch/pppoe-driver/pppoe/` (gitignored) so ROOT resolves to this worktree. Only change: the shim-mode line
now adapts to the new signature (`SetPppoeSecrets(prefix, func(ref string) ([]byte, error) { return e.shim.Resolve(context.Background(), ref) })`);
the shim is not used in `product` mode. Command: `python3 .scratch/pppoe-driver/pppoe/run.py product`.

| Case | before (claude-pppoe 10-product-run) | after (this build, SOURCE_SHA fd6c5788d) |
|---|---|---|
| Apply PPPoE with sealed password | FAIL (ROLLED_BACK) | **PASS** (APPLIED) |
| Password only in 0600 chap/pap | NOT RUN | PASS |
| Dial-up | NOT RUN | FAIL — no PADO (defect 2: VPP pppoe_plugin consumes discovery; not fixed here) |
| Withdrawal cleanup | NOT RUN | PASS |
| Confirm-timeout rollback | NOT RUN | PASS |
| Password grep (test output + journal) | PASS | PASS (0/0) |
| Shared VPP / leftovers | PASS | PASS (MainPID 1014 NRestarts 0, no ppp processes, no netns) |

Note: the API-side validation warning `pppoe.secret-unavailable` ("must resolve in the agent secret cache before
apply") is emitted unconditionally by `internal/desired/pppoe.go` and still appears on successful applies; it is
advisory, not a resolution failure.

## Remaining / next

- claude-pppoe driver `pppoe_live.go` line 387 needs the same one-line adaptation once this lands.
- Defects 2–5 of `claude-pppoe-wip.md` remain (VPP pppoe_plugin vs linux-cp discovery, no dataplane encap, IPv6, noise).
- Next command for review: `git -C /root/ngfw-wt/claude-pppoe-secret show fd6c5788d`.
