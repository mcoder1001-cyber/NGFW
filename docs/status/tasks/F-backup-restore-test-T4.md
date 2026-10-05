# Independent T4 browser verification — F-backup-restore

Date: 2026-10-05. UI product source `5eb2378df`, product tree `eea361b241567947405742458d6817e39ca133b8`. Tested in reviewer worktree `/root/ngfw-wt/f-backup-ux-review-20261005`. Actual runtime Vite `127.0.0.1:16600`, proxy to Nest API `127.0.0.1:12600`, real private PostgreSQL and ordinary auth. Manager-owned API session 95976/web session 89382; reviewer does not stop these processes. Login read only from private mode-0600 file, never committed. Backend compiled dist is source5eb237 plus safe Agent STATUS fixture; no real host upgrade mutation is allowed. Read-only upgrade fixture reports active/default A, staged B, confirmed true, no pending trial. Later backend concurrency fixes require their own narrow verification.

## Scenarios

| Scenario | Expected | Observed | Result |
|---|---|---|---|
| Real browser login, both languages and themes | Real auth then authenticated screen | Ordinary login succeeds in all four new contexts | PASS |
| Backup page en/fa × light/dark | Render translated layout with RTL in fa | Four actual screenshots, <html dir> correct | PASS |
| Backup/archive prerequisites | Empty passphrase/file prevent mutation | Download and restore disabled in every combination | PASS |
| Schedule/history/templates | Real reads, empty feedback | No recent runs state rendered; actual candidate schedule and templates endpoints succeed | PASS |
| Upgrade page en/fa × light/dark | Read-only API status and translated controls | Four actual screenshots; active A/staged B shown, no pending trial | PASS |
| Upgrade control gates | Derive from returned status and file | Upload disabled without file; activate enabled for staged B; confirm/rollback disabled without trial | PASS |
| Browser errors | Zero uncaught page exceptions | assertNoPageErrors passes in all four contexts | PASS |
| Network | No feature endpoint errors | Four expected anonymous-refresh401s; navigation aborts of shell requests only | PASS |

Eight primary screenshots in `docs/status/tasks/F-backup-restore-shots/`: backup-restore-{en,fa}-{light,dark}-real.png and upgrade-{en,fa}-{light,dark}-real.png. Persian dark images inspected visually: correct RTL/sidebar alignment, readable translated controls, no overlap. Native file-picker text is browser-provided English because the harness browser locale is English. The no-installed-license banner is existing baseline fixture state, explicitly visible; it does not prevent these feature screens. Version numbers and identifiers remain conventional machine strings.

First two attempts before manager restarted API had real login failures (proxy500/direct connection refused) and no acceptance screenshots. After restart, initial harness incorrectly assumed every upgrade control disabled; staged B legitimately enables activate. Corrected harness compares actual API status and passes. These were environment/harness assumptions, not product failures.

Command used (existing WEB-3 library; no framework added):

```sh
NGFW_CHROME=/tmp/fbr-ux-chrome/chrome-headless-shell-linux64/chrome-headless-shell NGFW_PLAYWRIGHT_CORE=/root/.npm/_npx/e41f203b7505f1fb/node_modules/playwright-core LD_LIBRARY_PATH=/tmp/fbr-ux-chrome/libroot/usr/lib/x86_64-linux-gnu node docs/status/tasks/F-backup-restore-shots/verify.mjs
```

Actual full output is committed in `F-backup-restore-shots/browser.log`; complete captured failures/checklist in `network.json`. Output below omits repetitive navigation-abort array; it is retained in those files.

```text
ok   backup requires passphrase en/light
ok   restore requires file and passphrase en/light
shot backup-restore-en-light-real.png  (ltr/en)  /system/backup-restore
ok   backup-restore-en-light-real.png: <html dir="ltr"> matches en (expected "ltr")
ok   real API upgrade status en/light
ok   upgrade uploadStage matches real status en/light
ok   upgrade activate matches real status en/light
ok   upgrade confirm matches real status en/light
ok   upgrade rollback matches real status en/light
STATUS {"format":1,"active_slot":"A","default_slot":"A","versions":{"A":"1.0.0","B":"1.1.0"},"pending_slot":null,"staged_slot":"B","confirmed":true,"next_entry":null,"migration_backup":null}
shot upgrade-en-light-real.png  (ltr/en)  /system/upgrade
ok   upgrade-en-light-real.png: <html dir="ltr"> matches en (expected "ltr")
ok   backup requires passphrase en/dark
ok   restore requires file and passphrase en/dark
shot backup-restore-en-dark-real.png  (ltr/en)  /system/backup-restore
ok   backup-restore-en-dark-real.png: <html dir="ltr"> matches en (expected "ltr")
ok   real API upgrade status en/dark
ok   upgrade uploadStage matches real status en/dark
ok   upgrade activate matches real status en/dark
ok   upgrade confirm matches real status en/dark
ok   upgrade rollback matches real status en/dark
STATUS {"format":1,"active_slot":"A","default_slot":"A","versions":{"A":"1.0.0","B":"1.1.0"},"pending_slot":null,"staged_slot":"B","confirmed":true,"next_entry":null,"migration_backup":null}
shot upgrade-en-dark-real.png  (ltr/en)  /system/upgrade
ok   upgrade-en-dark-real.png: <html dir="ltr"> matches en (expected "ltr")
ok   backup requires passphrase fa/light
ok   restore requires file and passphrase fa/light
shot backup-restore-fa-light-real.png  (rtl/fa)  /system/backup-restore
ok   backup-restore-fa-light-real.png: <html dir="rtl"> matches fa (expected "rtl")
ok   real API upgrade status fa/light
ok   upgrade uploadStage matches real status fa/light
ok   upgrade activate matches real status fa/light
ok   upgrade confirm matches real status fa/light
ok   upgrade rollback matches real status fa/light
STATUS {"format":1,"active_slot":"A","default_slot":"A","versions":{"A":"1.0.0","B":"1.1.0"},"pending_slot":null,"staged_slot":"B","confirmed":true,"next_entry":null,"migration_backup":null}
shot upgrade-fa-light-real.png  (rtl/fa)  /system/upgrade
ok   upgrade-fa-light-real.png: <html dir="rtl"> matches fa (expected "rtl")
ok   backup requires passphrase fa/dark
ok   restore requires file and passphrase fa/dark
shot backup-restore-fa-dark-real.png  (rtl/fa)  /system/backup-restore
ok   backup-restore-fa-dark-real.png: <html dir="rtl"> matches fa (expected "rtl")
ok   real API upgrade status fa/dark
ok   upgrade uploadStage matches real status fa/dark
ok   upgrade activate matches real status fa/dark
ok   upgrade confirm matches real status fa/dark
ok   upgrade rollback matches real status fa/dark
STATUS {"format":1,"active_slot":"A","default_slot":"A","versions":{"A":"1.0.0","B":"1.1.0"},"pending_slot":null,"staged_slot":"B","confirmed":true,"next_entry":null,"migration_backup":null}
shot upgrade-fa-dark-real.png  (rtl/fa)  /system/upgrade
ok   upgrade-fa-dark-real.png: <html dir="rtl"> matches fa (expected "rtl")
NETWORK [{"lang":"en","mode":"light","status":401,"path":"/api/v1/auth/refresh"},{"lang":"en","mode":"dark","status":401,"path":"/api/v1/auth/refresh"},{"lang":"fa","mode":"light","status":401,"path":"/api/v1/auth/refresh"},{"lang":"fa","mode":"dark","status":401,"path":"/api/v1/auth/refresh"}]

```

Scope: bounded changed-screen T4 verification, plus independently run nine unit workflow/transport tests and typecheck in R6 report. Real signed host upgrade upload/stage/activate/boot/confirm/rollback is deferred appliance acceptance; this run makes no such claim. Restore backend transactionality and account concurrency belong to T2/R1. Full unrelated screen regression is not claimed.

**Verdict: PASS for bounded changed-screen rendering, state gating and browser regression checks on the recorded UI/runtime source.**

## Actual template candidate workflow

Through the real UI, staged template w26browser_ux with description and typed required string parameter; applied exact documented placeholder to system.banner.motd. PUT template and POST apply both200; apply returned staged:true and diff with resolved parameter value. No commit or account mutation. An initial intentionally invalid management.banner target was rejected400 with a field pointer; test then used the correct system.banner.motd schema. This validates actual candidate preview behavior and problem rendering rather than relying solely on unit stubs. Ninth screenshot `backup-restore-en-light-template-candidate.png`.

```text
CANDIDATE_RESPONSES [{"path":"/api/v1/config-templates/w26browser_ux","status":200,"body":{"staged":true}},{"path":"/api/v1/config-templates/w26browser_ux/apply","status":200,"body":{"staged":true,"diff":{"baseRevision":null,"changes":[{"op":"add","pointer":"/management/templates/w26browser_ux","to":{"patchJson":"{\"system\":{\"banner\":{\"motd\":\"${banner}\"}}}","parameters":{"banner":{"type":"string","required":true}},"description":"Browser review template"}},{"op":"add","pointer":"/system/banner/motd","to":"w26browser review candidate only"}]}}}]
shot backup-restore-en-light-template-candidate.png  (ltr/en)  /system/backup-restore

```
