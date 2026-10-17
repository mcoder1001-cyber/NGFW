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
4. **MINOR** — `apps/api/src/datastore/physical-nics.ts:6-9, 33, 82, 86`: comments and the 403 text state invariants D-249 makes
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
