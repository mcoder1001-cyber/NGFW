# P08 — verify of fix round 2 (focused; envelope `P08.verify2.md`, manager ngfw-46)

Branch `task/P08` @ abb6950 (code 1a90988), main @ 35f3a96, 2026-09-24 18:08–18:25. Read-only except this file.
Scope: whether each round-2 item is fixed with a test that fails on the old code, and nothing regressed. "Old code" runs were
done on `git archive` copies in my scratchpad (never in the worktree): the fix reverted, the new test kept.

## Per item

| item | commit(s) | status | evidence |
|---|---|---|---|
| T1 V24 guard skips `*ast.TypeAssertExpr` | 7c06b88 | **fixed** | `guard_test.go:119-123,133` exempts only the ident that `assertedIdent` returns for a type assertion's asserted type (`T`, `*T`, `pkg.T`, `*pkg.T`, parens); `m.(type)` has `Type == nil`, so type-switch cases stay flagged. The new `xtest/fakevpp.go` fixture expects lines 12 (`svc.AfPacketDelete(ctx, m.(*afpapi.AfPacketDelete))`, a raw send) and 14 (a type-switch case) to be flagged, and lines 5-6 (the assertions) not flagged. Exemption removed (scratch): `TestEveryAfPacketDeleteIsQuiesced` FAILS on `coretest/ifext.go:170:21`, and `TestGuardCatchesARawAfPacketDelete` FAILS (it flags fakevpp.go:5 and :6). With the exemption, both pass |
| R1-agent remembered default → ErrRecreate | 7ae4701 | **fixed** | `tolerant.go:84-90`: the key is read under `t.mu`. If the old value is remembered, `Update` returns `ErrRecreate`. The reconciler (`reconciler.go:896-903`) then runs a journaled `del` (the wrapper's no-op, which forgets the key) and `create` (DF-1 writes and claims). Revert (`reconciler.go:1063-1068`) handles `ErrRecreate` too. `TestRememberedDefaultThenChange` covers one RECREATE 1500→1400, Retrieve, an empty re-apply, 1400→1500→9000, the rolled-back 1500→1400 path (VPP stays at 1500), and rx-mode polling→interrupt. On the old `tolerant.go` (7ae4701^) it FAILS: `APPLY_STATUS_DEGRADED … interface.mtu/lan missing … REVERT_FAILED` |
| R1-cli skips `config == nil` rows | 0643ef4 | **fixed** | `cmd_op.go:201,218-220,294-296`: the table, the name lookup and completion all skip nil-config rows. `--json` stays the raw API answer. Only a JSON `null` decodes to a nil map, and `state.controller.ts:266` sets `config: actIfs.get(name)?.value ?? null`, so the test fixture has the real response shape. On the old `cmd_op.go` (0643ef4^), `TestShowInterfacesListsOnlyRetrievedRows` FAILS: 3 extra table rows, 3 lookups exit 0 with an empty body, and 3 names completed |
| contract 6ce08c2 additive | 6ce08c2 | **ok** | The diff changes 1 line in `state.controller.ts` (`.describe()` text) and 1 line in `schema.d.ts` (the `@description` comment). It changes no field, type or nullability. The `contract(` subject and the `P08-contract.md` update are present. None of `binapi/` or `tools/binapi-gen.sh` changed on the branch |
| T2 e2e config vs running | c1f9518, 1a90988 | **fixed (by reading)** | `interfaces.e2e.test.ts:65-87` mutates `h.fake.current.interfaces[L].mtu = 9000`. The fake's Retrieve reads `this.current` by reference (`fake-agent.ts:428`). The test asserts `config.mtu === 9000`, `running.mtu === 1400` and `hasPendingChange === false`, and restores the value in `finally`. Swapping `config`/`running` at `state.controller.ts:266-267` must fail it. I did not run it (the e2e needs the slot DB, which is outside the allowed test list). CI run 3 passed turbo 30/30, which includes `@ngfw/api:typecheck` |
| T3 N4/N5 web tests | 25f8cc5, 5204b81 | **fixed** | N5: a 400 with the typed-id pointer `/…/subinterfaces/200/vlanId` becomes the VLAN field's accessible description plus `aria-invalid`, and no alert appears. The old code built the prefix from `subDialog.id`, which is `''` on add, so the old code fails this test. N4: after `refetchQueries` (the fetch count rises) the typed MTU `1400` and Description `lan` stay. The old code keyed the form by `JSON.stringify(formValue)`, so a refetch remounted it with 9000 / "set elsewhere". I ran `pnpm --filter @ngfw/web test -- interfaces`: the filter does not narrow the run, so all 13 files ran, **88/88 passed**, and InterfacesPage 7/7 |
| T4 I4 list | 393047b | **ok** | P08.md "Out of scope" names every out-of-ownership file from `git diff --name-only main...task/P08`: coretest, guard_test, proto + gen + proto.md, agent.client.ts, fake-agent.ts, interfaces e2e, schema.d.ts, operations_gen.go, cmd_op/cli_test, i18n.ts, router.tsx, nav.ts(+test) and the locales |
| T5 Q3/Q4 | 393047b | **ok** | `P08-questions.md`: Q3 is resolved by TD-5 (D-105 M1, D-110, D-113) and Q4 is fixed on main (63178d2). The original reports are kept |
| R2-stores / R3-gauge | 393047b | **ok** | `docs/tech-debt.md` has the "P08 re-review (D-118)" section with file:line and the fix for each. No code changed |
| Round-2 scope | f2ac0e2..abb6950 | **ok** | 14 files changed, all of them in the fix list. No scope creep |

## Topology + restart-safety evidence (P08.md "Fix round 2"; judged, not re-run)
**Accepted.** I cross-checked the pasted run against the host:
- **Agent log.** `/run/vrx-test/w1/p08/agent.log` still exists. Its six `reconcile done` lines have the pasted times and summaries: created:8 at 18:02:25.033, created:2 unchanged:8, deleted:2 unchanged:8, the resync created:8 at 18:02:50.750, and deleted:6 at 18:02:57.718. All six are APPLIED.
- **VPP did not restart.** `systemctl show vpp`: NRestarts=0, MainPID=8760 (active since 13:03:29). This matches "NRestarts 0 → 0, pid 8760 throughout".
- **Management NIC.** The commit that puts af_packet on ens192 returns 400 with pointer `/interfaces/host-ens192`, rule `interfaces.af-packet-veth`, tier=agent.
- **Data path.** Ping is routed at ttl 63; the first-packet loss is ARP. The trace runs af-packet-input → … → ip4-rewrite → host-w1w0-tx. The four counters match between vppctl and WS.
- **MTU test.**
  - The MTU 1400 commit shows in Retrieve and in `vppctl show int`.
  - A 1472-byte DF ping fails with "Frag needed (mtu = 1400)", and the test asserts it (`interfaces_test.go:357`).
  - Rollback to revision 1 deletes `interface.mtu/host-w1w0` and the second address. VPP returns to 9000, and a DF ping passes again (asserted at `:395`).
- **Restart-safety.**
  - The agent was stopped and the tagged `w1:` af_packet interfaces deleted. The dump confirmed they were gone, and the API answered 503 while the agent was stopped.
  - After the agent started: interfaces back at +0.41 s, ping at +1.82 s, with no config API call. The resync took 0.405 s.
- **CI logs.** `/root/ngfw-wt/logs/ci/P08-20260924-174653-1608551` backs the pasted CI output: turbo 30/30, agent/cli `ok`, topology unit mode `ok`.

## Findings (by severity)

1. **MERGE-PREP (must be done before the merge; not a code defect).** I ran `git merge-tree --write-tree main task/P08` against main 35f3a96. It is **not clean**: `CONFLICT (content)` in `apps/cli/internal/api/operations_gen.go`.
   - **Cause.** main moved after P08's step-0 merge (f2ac0e2, base 63178d2). TD-2's `7082cc6 contract(api-client)` re-aligned the generated table with gofmt, changed the `Auth_password` summary and added operations. P08 adds `State_counters` and a new `State_interfaces` summary.
   - **Other files.** `state.controller.ts` and `schema.d.ts` merge automatically. I found no semantic interaction: TD-2's `route-guard.test.ts` change only adds a readonly-allowed route, and `safeText` applies to TD-2's own inputs.
   - **Fix, during the D-112 squash + rebase.** Regenerate both generated files instead of hand-merging: `pnpm --filter @ngfw/api-client gen`, then `make -C apps/cli gen` (opgen reads the generated `openapi.json`). Keep the `contract(` subject, because the squash touches `packages/api-client/src/generated`. Then re-run `tools/ci.sh --base main`: the generated-output gate also covers the auto-merged `schema.d.ts`. After that, P08.md's note that "`operations_gen.go` … regenerate unchanged" (6ce08c2) no longer holds.
2. **Info. The T1 exemption is deliberately narrow, with one residual case.** `ch.SendRequest(m.(*afpapi.AfPacketDelete))` is no longer flagged when `m` comes from a place the guard does not scan (a test file, or `binapi` helpers such as `AllMessages()`). Every construction site in scanned code (`&afpapi.AfPacketDelete{}`, `new(...)`, `var x afpapi.AfPacketDelete`) is still flagged. Before this change, a send of an `api.Message` from those unscanned places was not caught either. This is acceptable under D-118 option (b); no action.
3. **Info. The V24 signature is still in the VPP journal** for the cleanup deletes, although both netdevs were down first. This matches D-107 I1 (quiesce is not the fix) and VPP survived. It belongs to the manager's V24 record, not to P08.
4. **Info. Leftovers.** `/run/vrx-test/w1/p08` (0700 run logs) and the git-ignored `apps/agent/bin` and `apps/{api,web}/dist` remain. P08 could not remove them (permission policy). They are harmless; the manager may delete them.

## Tests run (worktree HEAD abb6950, host load ~15-22)
- `cd apps/agent && go test -count=1 ./internal/agent/... ./internal/subsystems/... ./internal/descriptors/af_packet/...` → ok agent 8.9 s, subsystems, and af_packet.
- `make -C apps/cli test` (`-race`) → every package ok, including `internal/cli` and `test/e2e`.
- `pnpm --filter @ngfw/web test -- interfaces` → 13 files, 88/88 passed (InterfacesPage 7/7, including the N4 and N5 tests).
- Old-code checks (scratch copies): T1 exemption off → both guard tests fail. `tolerant.go` @7ae4701^ → `TestRememberedDefaultThenChange` fails. `cmd_op.go` @0643ef4^ → `TestShowInterfacesListsOnlyRetrievedRows` fails.

**APPROVE**. The code items are all verified. Before the merge, the manager must clear the `operations_gen.go` conflict (finding 1) by regenerating during the D-112 rebase, then run CI again.
