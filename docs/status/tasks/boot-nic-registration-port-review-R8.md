# Review R8 (operability & packaging) — boot-nic-registration-port

Reviewed head `814532721` (base `origin/main` `99a774734`), PR #238. Reviewer: panel R8 (Opus 5.5, read-only agent).
The reviewer handed its report back in chat and did not write this file itself (read-only run); the developer saved it
here unedited. Developer answers follow at the end.

## Verdict: BLOCK — 1 MAJOR without an accepted debt row (#1); becomes APPROVE once #1 is fixed in-branch

Packaging side sound: the unit diff is two comment lines; sandboxing unchanged; `systemd-analyze verify` clean; `prepare.sh:80`
copies the unit unchanged; the new packaging test runs in the hosted "Packaging fixture gate" and passed on 814532721
(`tools/ci.sh quick` does not run it). Boot behaviour: at most one revision per API boot; agent-after-API and PostgreSQL-down
cases retry on agent-ready or after 60 s; a site `NGFW_SEED_DEFAULT_NICS=0` in `/etc/ngfw/api.env` is allowed only after
`firstboot-complete` and `EnvironmentFile=` overrides `Environment=` whatever the order.

1. **MAJOR** — `seed.service.ts:83-107, 179-186`: any error thrown by `systemCommit` lands in the `deferred` class, retried every
   60 s with no backoff or cap; `retryLog` warns only when the class changes (a failure after a wizard deferral is logged at
   debug, which the production logger never prints); a validation failure records no event; the operator cannot add the marker
   by hand and nothing in the UI/CLI explains why. New: on main the loop ended at the first revision. Reproduced with the
   branch's `seedDocument` + validators: (A) management moved onto an engine-owned NIC + new NIC → `dataplane.management-not-dpdk`
   fails; (B) a fifth management PCI → schema max 4. Fix: a `failed` outcome, a `system.seed-defaults-failed` warning event once
   per distinct reason with the problem's pointers, warn-once keyed on outcome+reason, exponential backoff (60 s → ~1 h) or
   stop until the next boot; unit test with a rejecting `systemCommit`. [other: R1] do not append a management PCI already in
   `pciWhitelist`/`devices`; respect the max of 4.
2. **MINOR** — `test_packaging.py:203-212` asserts line order, which systemd ignores; false failures on equivalent units
   (EnvironmentFile first, flag on its own line, quoted); misses `UnsetEnvironment=`, a later `Environment=…=false`, the
   hardening drop-in. Fix: shell-split every `Environment=` value, last assignment wins ∈ {1,true}; no `UnsetEnvironment`.
3. **MINOR** — `docs/install/upgrade.md` not updated; `bare-metal.md`: first API start after upgrade adds one `system`
   revision; backup import needs the box's physical rows; rollback drops rows until the next start; the site opt-out is valid
   only after firstboot and must be set before the first post-upgrade API start.
4. **MINOR** — `apps/api/src/datastore/physical-nics.ts:6-9, 33, 82, 86`: comments and the 403 text state invariants D-252 makes
   false ("created only by the first-boot seed", "a rollback cannot drop one").
5. **MINOR [other: R4]** — `vrrp-config-sync/document.ts:5-12`: HA sync keeps `/dataplane` node-local but copies `/interfaces`;
   a backup with different PCI addresses re-adds its rows at every start and loses them at the next sync. Fix: document the
   identical-hardware requirement or make physical rows node-local; at least a DEFERRED-ACCEPTANCE row.
6. **NIT** — `no-nics` is no longer final: a management-only appliance reads the running document and calls HostNics every
   60 s forever. Fix: back off.
7. **NIT** — `config.ts:106`, `seed.service.ts:33-34`: only the unit sets the variable; the unit comment's "read afterwards"
   suggests line order matters.

## Evidence (reviewer)
```
$ python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_packaging.py -v
test_api_unit_registers_default_nics_before_site_overrides ... ok / Ran 17 tests in 1.683s / OK
$ systemd-analyze verify deploy/systemd/ngfw-api.service ; echo exit=$?   → exit=0 (systemd 259)
$ git diff origin/main...HEAD -- deploy/debian/ngfw/prepare.sh deploy/hardening | wc -l   → 0
mutation run of the new test (scratchpad copies of the unit):
  M0-unchanged OK · M1-flag-removed FAILED (good) · M2-flag-false FAILED (good) · M3-unsetenvironment OK (not caught)
  M4-envfile-first FAILED (false failure) · M5-own-line FAILED (false failure) · M6-quoted FAILED (false failure)
  M7-later-env-line-overrides OK (not caught)
$ gh pr checks 238 → "Offline packaging and temporary-key signing fixtures pass"; "Mandatory quick gate pending"
probe of seedDocument + RootConfig + validateSemantics:
  A mgmt-moved+new-nic: '/dataplane/managementPci/1: management NIC 0000:04:00.0 must never be a DPDK device'
  B managementPci>4: SCHEMA FAIL 'dataplane/managementPci: Too big: expected array to have <=4 items'
  C control: semantic issues = []
app.ts:42 default logger ['error','warn','log']; commit.service.ts:867-872 validation failure throws without an event
```

## Developer answers (second iteration)
- #1: fixed — `failed` outcome; a 4xx ProblemError is final for the boot (one `system.seed-defaults-failed` warning event with
  reason, pointers, type); other failures retry with exponential backoff (`retryMs` → `maxRetryMs` = 1 h), recorded once per
  distinct reason (warn-once keyed on outcome+reason). The two reproduced triggers no longer produce a failing commit: a
  management PCI in `pciWhitelist`/`devices` is reported as `management-engine-owned` instead of being appended, and
  `managementPci` is capped at four (`management-limit`). Tests: rejecting `systemCommit` (final: 1 event, no timer, `skipped`
  afterwards), transient failure (backoff 20 → 40 → 80 ms capped, 1 event), conflict and limit plans.
- #2: fixed — `test_api_unit_enables_default_nic_registration` shell-splits every `Environment=` value of the unit and the
  hardening drop-in, last assignment wins ∈ {1, true}, asserts no `UnsetEnvironment=` of the variable; no order check.
- #3: `upgrade.md` and `bare-metal.md` updated. #4: fixed. #5: documented in the user doc (same-layout requirement), the
  DEFERRED-ACCEPTANCE row and a `docs/tech-debt.md` row. #6: `no-nics` now backs off exponentially. #7: fixed (config comment,
  unit comment states precedence, not order).

---

## Verify round (head `db5042e38`, fix commit `814532721..db5042e38`) — reviewer text, saved unedited by the developer

### Verdict: APPROVE
My only round-1 MAJOR (#1) is fixed in this branch, and #2–#7 are fixed. New items: one MINOR and NITs, all optional.

### Round-1 findings
1. **#1 MAJOR: fixed.** `seed.service.ts:21` adds `failed` (in RETRIED `:29` and BACKED_OFF `:32`); `:207-209` routes every
   `systemCommit` throw to `failed()` (`:260-282`): a `ProblemError` <500 is final for the boot (`done`, `stop()`, no timer), first
   occurrence records `system.seed-defaults-failed` with `{reason, pointers, final, type}`; other errors retry with exponential
   backoff via `nextBackoff()` (`:134-137`), capped at `maxRetryMs` = 1 h; `retryLog` warns once per outcome+reason. `planRegistration`
   never writes a management PCI in `pciWhitelist`/`devices` (`:407-415`, `management-engine-owned`) nor a fifth address
   (`:455-458`, `management-limit`); both appear in the payload `skipped` (`:241`) and in `system.seed-defaults-skipped` (`:218-226`).
   Matches `packages/schema/src/semantic/dataplane.ts:92-114`. Probe scenarios A and B no longer produce a failing commit. The four
   unit tests (`:414-440`, `:442-463`, `:278-309`, `:311-329`) exercise the fix.
2. **#2 MINOR: fixed.** `test_packaging.py:201-226` shell-splits the unit and the hardening drop-in; last assignment wins ∈ {1,true};
   `UnsetEnvironment=` asserted absent; no order check. Mutations: caught M1, M2, M3, M7, M8, M9; no longer falsely failing M4, M5, M6;
   still missed M10 (empty `Environment=` in the drop-in, N5).
3. **#3 MINOR: fixed** (`upgrade.md:20-30`, `bare-metal.md:48-54`); "instead of being retried" true only for 4xx (N1).
4. **#4 MINOR: fixed** (`physical-nics.ts:6-11`, `:34-35`, `:84`, `:87`).
5. **#5 MINOR: fixed, acceptable for a MINOR** (`default-dataplane-nics.md:111-113`, `DEFERRED-ACCEPTANCE.md:81`, `tech-debt.md:226`
   — add `Due:` if the manager wants it to count as accepted debt).
6. **#6 NIT: fixed** (`no-nics` in BACKED_OFF; documented `:72-73`; shares the tested `nextBackoff`).
7. **#7 NIT: fixed** (`config.ts:106`, `seed.service.ts:50-51`, unit `:13-14`).

### New findings from the fix commit
- **N1 MINOR:** `seed.service.ts:262` makes **any** 4xx final for the boot, not just validator rejections: 409 `agent-precondition` /
  `agent-aborted` (`agent.client.ts:767-788`), 422 `apply-failed`, 403 `license-required`. Docs say "rejected by the validators". A
  transient agent refusal at boot would leave the NICs unregistered until the next API start (bounded: one event with `final`/`type`;
  a restart retries). Fix: make only `type` ∈ {`validation`, `license-required`} final and back off the rest, or reword the docs.
- **N2 NIT:** `seed.service.ts:269-280`: if `events.record` throws for a final problem, `done` is never set and the `.catch` at
  `:118-121` retries every 60 s. Fix: set `done`/`stop()` before awaiting `record`.
- **N3 NIT:** `:123`: a `skipped`/`deferred` outcome resets `backoffMs`. Harmless.
- **N4 NIT:** comment at `:407-408` ("reported whether or not anything is registered") is not true when there is no data NIC at all
  (`no-nics` path, probe A2). Reword or report in the `no-nics` path.
- **N5 NIT:** `test_packaging.py:217-221` does not treat an empty `Environment=` as a reset (mutation M10 passes).

### Commands run (head `db5042e38`)
```
$ pnpm --filter @ngfw/api exec vitest run src/features/default-vpp-nics
 ✓ src/features/default-vpp-nics/seed.service.test.ts (25 tests) 802ms · Test Files 2 passed · Tests 26 passed
$ python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_packaging.py -v → test_api_unit_enables_default_nic_registration ok · Ran 17 tests · OK
$ systemd-analyze verify deploy/systemd/ngfw-api.service; echo exit=$? → exit=0 (systemd 259)
$ scratchpad/r8mut2/run2.sh → M0 OK · M1 FAILED · M2 FAILED · M3 FAILED · M4 OK · M5 OK · M6 OK · M7 FAILED · M8 FAILED · M9 FAILED · M10 OK (not caught) · M11 OK
$ node scratchpad/r8probe/probe2.mjs → A: semantic issues [] managementPci=[0b] added=[ens193] skipped=[ens161 management-engine-owned]
  A2: next=null · A3 (device key case): issues [] · B (>4): issues [] skipped=[m5 management-limit] · B2: issues [] · C control: []
$ gh pr checks 238 → Mandatory quick gate pending · Offline packaging pass 49s · pass 1m8s
```

### Developer answers (iteration 3)
N1: fixed — only deterministic problems are final for the boot (`type` ∈ {`validation`, `license-required`}, status < 500);
every other error, 4xx agent refusals included, backs off; test added for a 409 `agent-precondition`. N2: fixed (`done`/`stop()`
before awaiting `record`). N3: won't fix (harmless, as noted). N4: comment reworded (reported whenever a plan is computed, i.e.
when the inventory has a data NIC). N5: fixed (an empty `Environment=` resets the value). #5: `Due: 2026-11-14 (proposed)` added
to the three debt rows for the manager to confirm or adjust.
