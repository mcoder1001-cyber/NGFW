# Review R6 (UX / web / i18n — user documentation) — boot-nic-registration-port

Reviewed head `db5042e38` (base `origin/main` `99a774734`), PR #238. Reviewer: panel R6 (Opus 5.5, read-only agent).
The reviewer handed its report back in chat and did not write this file itself (read-only run); the developer saved it
here unedited. Developer answers follow at the end.

Scope: `git diff origin/main...HEAD -- docs/user docs/install apps/web`. The branch changes `docs/user/interfaces/default-dataplane-nics.md`,
one line of `docs/user/interfaces/basics.md`, `docs/install/bare-metal.md` and `docs/install/upgrade.md`. No web code, locales or
screenshots change, so R6 checks 1, 3 and 4 (screens, RTL/logical CSS, ui-kit) do not apply and no screenshot is required.

## Verdict: APPROVE WITH CHANGES (0 BLOCKER, 0 MAJOR, 8 MINOR, 9 NIT)

Most of the user doc matches `planRegistration`, `SeedService.seedOnce` and `failed()` sentence by sentence. The labels match the web
UI in both en and fa. All links resolve. The CLI example is still correct. The MINORs are precision and operator-guidance gaps; no
verify round is needed.

### MINOR
1. `default-dataplane-nics.md:51-52` (also `bare-metal.md:48-49`, `upgrade.md:25-26`): the doc says a commit "rejected by the validators"
   is not retried and other failures "are retried"; the code records `system.seed-defaults-failed` for transient failures too
   (`final: false`), the Dashboard shows the same message either way, and "final" is any 4xx (incl. 422 apply-failed, 403 guards).
   Fix (doc): every failed commit is recorded once per distinct reason; `data.final: true` → not retried before the next API start,
   fix the pointers and restart; `false` → retried with increasing delays up to an hour. Fix (code, optional): add the "(not retried
   …)/(will retry)" suffix to the event message. [other: R1] should a 422 apply-failed be final for the whole boot?
2. `default-dataplane-nics.md:111-113` (same in `PENDING…md:40-42` and the tech-debt HA row): "a backup with different NICs would
   register its own rows at every start and lose them at the next sync" is half right — when the backup lacks a PCI the primary has
   as an engine-owned row, `dataplane.owner-consistent` (`packages/schema/src/semantic/default-vpp-nics.ts:12-33`) rejects the cluster
   commit (sync refused, nothing lost); churn happens only when the backup has extra NICs. Inferred from code, not run. [other: R4]
3. `default-dataplane-nics.md:45-50`: the skip reason codes the operator sees (`describeSkipped`: `ens161 (0000:04:00.0): name-in-use`)
   are not documented; no remedy per reason; the doc does not say `system.seed-defaults-skipped` repeats at every API start. Fix: a
   table code / meaning / what to do (rename the conflicting bond or PPPoE row, or give the NIC another logical name via
   `dataplane devices <pci> name <name>` and restart — R1 to confirm `devices.<pci>.name` is a supported path; `management-engine-owned`
   → Release to host).
4. The doc does not say where the events are visible; fa users get English text. System events appear only on the Dashboard, Recent
   events (newest 6, message only; `DashboardOverview.tsx:195`, `:852-854`); the `system.seed-defaults*` codes are not in `EVENT_CODES`
   (`apps/web/src/domains/dashboard/overview/model.ts:207-215`); the CLI has no events command. Already true on main for the two older
   codes. Fix: one sentence (Dashboard → Recent events; code and payload via `GET /api/v1/state/events`). Follow-up: add the four codes
   to `EVENT_CODES` with en/fa strings, or a tech-debt row.
5. `default-dataplane-nics.md:5, 22-37, 134`: logical names vs the `Ethernet1…N` labels on the screen (since `4f70b3315`, numbered by
   PCI, management included) — explain the mapping and that an unregistered NIC shows as a read-only **host interface** row; link
   `basics.md#interface-names`.
6. `default-dataplane-nics.md:3-6, 41`, `upgrade.md:20-30`: no review step for existing installations — the first API start after the
   upgrade registers every non-management NIC as engine-owned (storage link, HA heartbeat link without the default route included);
   the next guarded start-up configuration apply hands them to the engine. Tell the operator to review the Interfaces screen and
   **Release to host** such NICs before that apply. [other: R4/R8]
7. "the next API start" is the trigger for three remedies but no doc says how to cause one (no `restart ngfw-api` in docs). Fix: state
   the supported way (reboot, or a service restart if supported — R8 to confirm).
8. `bare-metal.md:49-50`: "NICs that cannot be registered are reported as `system.seed-defaults-skipped`" — only when nothing was
   registered; otherwise they are in the `system.seed-defaults` payload.

### NIT
9. Operator-visible "seed" wording left over: `physical-nics.ts:73` per-pointer message (rendered in `ProblemAlert`), `seed.service.ts:187`
   "default NIC seeding deferred …", `state.controller.ts:172` OpenAPI "a built-in (seeded) physical NIC" (outside owned files).
10. `apps/web/src/domains/interfaces/model.ts:36` and `InterfaceDrawer.tsx:145` stale code comments (not user-visible); no web string
    contradicts the doc; the Remove button is hidden for physical rows as the doc says. Fix in a later web change.
11. `default-dataplane-nics.md:5` (~190 columns), `:78`, `:85` break the 120-column wrap.
12. `:108-109` "a release to the host made after the rollback target is not restored" — simpler: "registers the NIC again as
    engine-owned; if it had been released to the host, release it again."
13. `:43` adoption: add "it becomes a built-in row: no longer deletable, only releasable".
14. `:48-49` "then it is protected" is vague: "**Release to host**; the engine then does not take it over at the next start-up
    configuration apply, and a later registration saves it in `dataplane.managementPci`."
15. `upgrade.md:21` quotes the revision comment without " (F-default-vpp-nics)"; `:27` "(see the user guide on restoring a backup)" is
    not a link.
16. `basics.md:135` "registered automatically at every boot" vs "every start of the management API".
17. The user doc does not mention the site opt-out (`NGFW_SEED_DEFAULT_NICS=0` in `/etc/ngfw/api.env` after firstboot).

### Outside my aspect
- [other: R7] `python3 tools/docs/generate-reference.py --check` fails with `documentation drift: docs/user/README.md` — already broken on
  `origin/main` (four pages missing from the index); the branch does not touch README.md.
- [other: R1] finality of a 422 apply-failed (#1). [other: R4] the HA consequence (#2) and the post-upgrade takeover review (#6).

### Checks with no finding
Code vs doc sentence by sentence (PCI matching, preservation, adoption rules, reserved/invalid skips, saved `managementPci`, no-management
once per streak, empty-inventory backoff, deferral, unchanged inventory, `enp29s0f0`, binds nothing) — all match. Terminology matches
`apps/web/src/locales/en/interfaces.json:15-31`, `:194-196` and the fa strings. No Persian counterpart of the interfaces docs exists
(only `docs/user/system/notifications.fa.md`). CLI example matches `docs/user/cli/reference.md`.

### Commands run (abridged)
```
$ git rev-parse origin/main HEAD → 99a774734e2c… / db5042e38803…
$ git diff --stat origin/main...HEAD → 23 files changed, 1297 insertions(+), 132 deletions(-) (no apps/web, no locales)
$ grep -rn "seed-defaults" apps/web packages apps/cli … | wc -l → 0
$ sed -n 205,216p apps/web/src/domains/dashboard/overview/model.ts → EVENT_CODES = {VPP_CONNECTED, VPP_DISCONNECTED, DEGRADED, COMMIT_APPLIED, COMMIT_CONFIRMED, CONFIRM_REVERTED, RUNNING_IN_SYNC}
$ grep -rn "name-in-use\|reserved-name\|invalid-name\|management-engine-owned\|management-limit" docs/user docs/install → (none)
$ grep -rn "restart ngfw-api\|systemctl restart ngfw" docs/user docs/install → (none)
$ relative-link check of the four changed docs → 0 broken
$ PYTHONDONTWRITEBYTECODE=1 python3 -I tools/docs/generate-reference.py --check → documentation drift: docs/user/README.md (exit=1); README unchanged vs origin/main
$ find docs -iname "*fa*" (user docs) → docs/user/system/notifications.fa.md
```

## Developer answers (iteration 3)
- #1: fixed in code and docs — only deterministic problems (`validation`, `license-required`) are final for the API start; the event
  message carries "(not retried before the next API start)" / "(will retry)"; docs describe `data.final` and the remedy.
- #2: reworded in the user doc, the PENDING doc and the tech-debt row (sync refused by `dataplane.owner-consistent` when the primary has
  NICs the backup lacks; churn only with extra NICs on the backup).
- #3: a skip-code table with remedies added; `devices.<pci>.name` adoption is the supported path (covered by the unit test "keeps
  explicit logical names …"). #4: sentence added (Dashboard → Recent events; `GET /api/v1/state/events`); tech-debt row for the web
  `EVENT_CODES` en/fa strings. #5: sentence + link to `basics.md#interface-names`. #6: post-upgrade review step added to the user doc
  and `upgrade.md`. #7: "restart the management API service (`ngfw-api.service`) or reboot" stated. #8: fixed.
- #9: fixed (`physical-nics.ts` per-pointer message, deferred event message); `state.controller.ts` OpenAPI text left (outside owned
  files; no behaviour). #10: left for a web change (comments only). #11–#15, #17: fixed. #16: fixed earlier (R7 N2).
- README drift: pre-existing on `origin/main`, not touched by this branch; noted in the wip.
