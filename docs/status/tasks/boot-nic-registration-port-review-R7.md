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
- **Decision log and links:** D-252 lists options (a), (b) and (c) and collides with nothing. All markdown links resolve. The
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
- D-252 numbering: no other D-252 on main, PR #239's diff or any remote-tracking `LOG.md`
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

---

## Verify round (head `db5042e38`, fix commit `814532721..db5042e38`) — reviewer text, saved unedited by the developer

### Status of findings #1–#11
1. **BLOCKER, evidence gap: fixed.** Iteration 1 (`wip.md:45-81`) has the command, step summary, `CI GATE PASSED`, wall time,
   log directory, tested SHA and the hosted runs — checked independently: the log directory exists (`08-turbo.log`: Tasks 35/35,
   7m20.453s; `09-agent.log`/`10-cli.log` name `814532721`; file times 09:15:40–09:34:02 = 18m22s); `gh run view` shows run
   39020868049 "CI gate 814532721 completed success". Iteration 2 is described honestly (`wip.md:97`, `PENDING…md:47-48`). The
   gate on `db5042e38` was still running (local log at the turbo step; hosted run 39022880662 in_progress). Merge condition:
   the next checkpoint pastes the iteration-2 local and hosted results.
2. **MAJOR, not-run lists and the confirmation claim: fixed in the repo; the PR body is still stale (N1).** "Not run" lists at
   `wip.md:99-104` and `PENDING…md:58-61`. The deferral is now tested through the real engine (`commit.service.test.ts:205-241`:
   `confirmSec: 30` → `pending`, `seedIfNeeded()` → `deferred` with 0 revisions, `confirm` → `confirmed`, then `seeded` with
   revisions `[[1,'commit'],[2,'system']]`). I could not run it myself (filtered vitest denied while the gate runs); evidence is
   the developer's paste at `wip.md:86-88` plus the api test task inside the running gate.
3. **MAJOR, panel: fixed.** `wip.md:108-112`: R1, R2, R7 mandatory, plus R8, R6 (`docs/user/**`) and R4.
4. **MINOR, LOG.md: fixed.** `LOG.md:286` says "Amends HIST-NATIVE-D-164". Correction to round 1: the D-252 row already sat
   directly under D-247 at `814532721`; the table detachment is older (main breaks at `LOG.md:39`) — out of scope.
5. **MINOR, SHAs: fixed.** `wip.md:3-6` full base and iteration-1 SHAs; iteration-2 SHA promised for the next checkpoint
   (acceptable: a commit cannot contain its own SHA). `git ls-remote` → `db5042e38`; next command at `wip.md:120`.
6. **MINOR, user doc vs `planRegistration`: fixed** — every API start (`:3`), missing NICs appended (`:11`), skips listed with
   reasons matching `SkipReason` (`:45-50`), `system.seed-defaults-skipped` only when nothing registered (`:50`), 4xx final
   (`:51-52`), retries every 60 s / on reconnect, empty-inventory backoff capped at 1 h (`:71-73`).
7. **MINOR, upgrade.md: fixed** (`:20-30`); wording NIT N4.
8. **MINOR, provenance: partially fixed.** `PENDING…md:30-36` corrected; `gh api …/commits/76484d79f` returns the commit. But
   `PENDING…md:11` still says "(never published)" and the PR body "exists only in the dev-host workspace". Fix: "(on no branch
   or tag; reachable by SHA, see below)".
9. **NIT, label wording: fixed** (`PENDING…md:26-28` matches `display-names.ts:9-28`).
10. **NIT, "appliance API unit": fixed** (`config.ts:106`, `seed.service.ts:51`).
11. **NIT, leftovers: fixed** (`default-dataplane-nics.md:125` "registered row"; retry wording `:71`; envelope owned files
    cover all 23 changed paths).

### New findings
- **N1 MINOR: PR #238 body is still the round-1 text** (old test name, "pins it ahead of the site environment file", "quick:
  running locally", not-run list only the appliance boot, reviewers R1/R2/R7/R8, "exists only in the dev-host workspace").
  `wip.md:116` honestly lists "PR body update" as remaining. Fix before the PR leaves draft.
- **N2 NIT: mixed decision ids in code comments** — `config.ts:103`, `default-vpp-nics/index.ts:4`, `test/e2e/default-vpp-nics.e2e.test.ts:47`
  still cite plain D-164 (the stats decision); `basics.md:135`, `main.ts:27` say "every boot" vs "every API start".
- **N3 NIT: `bare-metal.md:49-50`** says skipped NICs "are reported as `system.seed-defaults-skipped`" — only when nothing was
  registered; otherwise they appear in the `system.seed-defaults` payload.
- **N4 NIT: `upgrade.md:22`** names only `COMMIT_APPLIED`; the engine also records `COMMIT_NOT_ENFORCED` (`commit.service.ts:1028-1035`).
- Side notes: the running gate briefly rewrote `packages/api-client/src/generated/schema.d.ts` (clean again by 09:5x) and created
  the untracked `tools/docs/__pycache__/…pyc` — do not commit it.

### Links and review files
All 18 relative markdown links in the 13 changed `.md` files resolve. `review-R7.md` is consistent with the round-1 tree
(spot-checked against `814532721`). I cannot byte-compare the saved files with the chat hand-backs; R2/R8 files are internally
consistent, carry the "saved unedited" header and end with a developer-answers section.

### Commands run (abridged)
```
$ git rev-parse origin/main HEAD → 99a774734e2c… / db5042e38803…
$ git ls-remote origin refs/heads/claude/boot-nic-registration-port → db5042e38803304c1f5ed6756dc3720fb4d53928
$ cat /root/ngfw-wt/logs/ci/priceless-lewin-09c0d6-20261017-091540-1233287/08-turbo.log → Tasks: 35 successful, 35 total · Time: 7m20.453s
$ gh run view 39020868049|39020803916|39020868110|39022880662 → CI gate 814532721 success · Packaging 814532721 success ×2 · CI gate db5042e38 in_progress
$ gh pr view 238 → headRefOid db5042e38…, isDraft true, OPEN; body = round-1 text (N1)
$ grep -cE "^\s*(it|test)\(" commit.service.test.ts seed.service.test.ts → 26 / 25
$ gh api repos/mcoder1001-cyber/NGFW/commits/76484d79f → 76484d79fc99… 2026-10-10 fix(interfaces): register missing data NICs automatically at boot
$ python3 -I <link check over changed .md> → checked 18 relative links; broken 0
$ python3 tools/board.py --check → board valid: 212 tasks; read-only validation
```

### Verdict: APPROVE WITH CHANGES
Conditions before the PR leaves draft: (a) next checkpoint pastes the iteration-2 `tools/ci.sh quick` result and the hosted run;
(b) PR body updated (N1); (c) `PENDING…md:11` no longer says "never published" (#8). N2–N4 optional.

### Developer answers (iteration 3)
(a) pasted in the wip (gate 2 on `db5042e38`: CI GATE PASSED, 11m43s; hosted run 39022880662 recorded when finished);
(b) PR body rewritten with the iteration-2/3 evidence; (c) fixed. N2, N3, N4 fixed; the stray `__pycache__` was removed, not committed.
