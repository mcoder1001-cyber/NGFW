# Review R7 (docs, status-file evidence & scope) — boot-nic-registration-port

Reviewed head `814532721` (base `origin/main` `99a774734`), PR #238. Reviewer: panel R7 (Opus 5.5, read-only agent).
The reviewer handed its report back in chat and did not write this file itself (read-only run); the developer saved it
here unedited. Developer answers follow at the end.

## Verdict: BLOCK — 1 BLOCKER (#1), 2 MAJOR (#2, #3), 5 MINOR (#4–#8), 3 NIT (#9–#11)
The BLOCKER is an evidence gap, not a code defect. The port matches `76484d79f` and stays in scope.

- **Code:** `seedDocument` and `SeedService` match the original rule for rule. The virtio/PCI agent change really is on main
  (`netdevPCI` at `hostfacts.go:155,198,514`; `TestHostNICsVirtioPCIManagementAndData` at `hostnics_test.go:213`). Main's
  `seed.golden.json` already has the newer tunnel domains.
- **Claims not copied:** the original's "66 API tests", its three failing Go tests and its check of the running agent socket
  were correctly left out.
- **Pasted evidence:** the 113 files / 806 tests, 19 + 26 cases and 10/10 tasks all match `turbo-api.log`, and the api lint,
  typecheck and test tasks there were real cache misses. 17 packaging tests confirmed.
- **Decision log and links:** D-249 lists options (a), (b) and (c) and collides with nothing. All markdown links resolve. The
  DEFERRED-ACCEPTANCE row is honest.

1. **BLOCKER** — `PENDING-automatic-network-bootstrap.md:38`, `boot-nic-registration-port-wip.md:46`: the PENDING doc says the
   complete `tools/ci.sh quick` result is recorded in the task status file and on the PR; at HEAD both say "running". Fix: paste
   the actual quick result (command, summary lines, exit code, SHA) and the hosted run once green.
2. **MAJOR** — the "Not run" lists are incomplete: `apps/api/test/e2e/default-vpp-nics.e2e.test.ts` was edited and runs only in
   `test:integration` (full mode); deferral while a commit awaits confirmation is covered only through a mocked `systemCommit`,
   yet the PR body says the wizard interaction is "covered by the new commit-engine test". Fix: a "Not run" block; narrow or
   prove the claim.
3. **MAJOR** — reviewer plan lists R1, R2, R7, R8 only; `docs/user/**` changed, so R6 is mandatory; R4 should be added (D-192
   came from an R4 finding; this change decides which NICs become data-plane-owned rows).
4. **MINOR** — `LOG.md`: "Amends D-164" should be `HIST-NATIVE-D-164` (D-164 on main is the stats decision); the blank line
   before the row detaches it from the table.
5. **MINOR** — wip: no remote/local SHAs, stale next command (`gh pr create`), no tested SHA.
6. **MINOR** — user doc vs `seedDocument`: existing installations append only the missing NICs; skipped names (bond/other row/
   reserved/invalid) are silent, no event; "at each boot" means every API start.
7. **MINOR** — `docs/install/upgrade.md:18-19` not updated (first API start after upgrade may add a system revision).
8. **MINOR** — provenance: "This answers question 2" reads the owner's answer from a paraphrase in the dev-host commit; "never
   published" is not quite right (`gh api repos/.../commits/76484d79f…` returns the commit; no branch/tag references it).
9. **NIT** — PENDING: "numbered over the PCI-sorted host inventory" does not match `display-names.ts`.
10. **NIT** — `config.ts:106`, `seed.service.ts:34`: only the API unit sets the variable, not firstboot.
11. **NIT** — leftovers: `default-dataplane-nics.md:111` "a seeded row"; `:62-63` "retries once those edits are committed or
    discarded" (it retries every 60 s and on reconnect); envelope's owned-file list lacks `DEFERRED-ACCEPTANCE.md`.
12. Outside aspect: [other: R8] the packaging test checks line order, which systemd ignores; [other: R5/R8] a management-only
    appliance retries `no-nics` every 60 s forever; [other: R1] the mocked lifecycle test logs "registered 0 missing NIC(s)".

## Evidence (reviewer)
- `grep -n netdevPCI apps/agent/internal/renderers/vppstartup/hostfacts.go` → 155, 195, 198, 514; `grep -n Virtio …/hostnics_test.go` → 213
- `grep -cE "^\s*(it|test)\(" seed.service.test.ts` → 19 (main: 10); `commit.service.test.ts` → 26 (main: 24); `grep -c "    def test_" test_packaging.py` → 17
- `PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_packaging.py -k test_api_unit_registers_default_nics_before_site_overrides -v` → ok / Ran 1 test / OK
- turbo-api.log: `@ngfw/api:{typecheck,test,lint}: cache miss, executing`; `✓ commit.service.test.ts (26 tests)`, `✓ seed.service.test.ts (19 tests)`; `Test Files 113 passed`, `Tests 806 passed`, `Tasks: 10 successful`, `exit=0`
- `python3 tools/board.py --check` → `board valid: 212 tasks; read-only validation`; every relative markdown link in the changed .md files resolves (0 broken)
- D-249 numbering: no other D-249 on main, PR #239's diff or any remote-tracking `LOG.md`
- `grep -rn "waiting for confirmation" apps/api --include=*.test.ts` → only `seed.service.test.ts:327` (mocked)

## Developer answers (second iteration)
- #1: the quick gate result for `814532721` (CI GATE PASSED, 18m22s, log dir) is pasted in the wip file; the second
  iteration's gate and the hosted run are recorded there and on the PR when finished.
- #2: "Not run" block added to the wip, the PENDING doc and the PR (e2e integration-only test, `tools/ci.sh full`, appliance
  boot). The pending-confirmation deferral is now proven through the real commit engine (`commit.service.test.ts`:
  `commitSec: 30` → `deferred`, `confirm` → `seeded`).
- #3: R4 and R6 added to the panel (second round), R7/R8 verify rounds requested.
- #4: fixed (HIST-NATIVE-D-164; the row joins the table). #5: fixed (SHAs, tested SHA, next command). #6: fixed in the user doc
  and in the code (skips are reported in the audit payload and as `system.seed-defaults-skipped`). #7: `upgrade.md` updated.
  #8: wording corrected (owner's go-ahead to be recorded on the PR; SHA reachable but unreferenced; archiving is the owner's
  call). #9, #10, #11: fixed.
