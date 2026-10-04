# Management/dataplane acceptance, 2026-10-04

تست واقعی API مدیریت و پیش‌نمایش دیتاپلین اجرا شد.
تعویض گواهی بدون تغییر PID، محدودیت نسخه TLS و رد کلید نامعتبر موفق بود.
نشت PEM در لاگ، audit و پاسخ‌های GET صفر بود.
پیش‌نمایش روی واقعیت میزبان موفق بود؛ فایل startup و سرویس مشترک تغییر نکرد.
اسکرین‌شات مرورگر هنوز اجرا نشده و نباید به‌عنوان موفق گزارش شود.

Branch: `codex/closeout-management`; source/base `6645443499d111d7cbbaa8ed2d5f6bd9105544af`.
Local acceptance commit: see `git log -1` (this file is included in that commit).
Remote publication: blocked, HTTP403 observed by manager; no remote SHA claimed.
Owned files: `test/topology/management-dataplane/**`, `docs/status/tasks/closeout-management*`.

Actual results are in `closeout-management-evidence/acceptance.log` (generated public metadata and sanitized problem responses only).
Real PostgreSQL18.6 + actual current agent + disposable VPP26.06 + current Node API were used. Slot9 database/processes/secret files were removed afterward.

Passed checks:

- `/state/dataplane` exposes observed VPP runtime threads/plugins/queues/memory and real installed startup/host facts.
- Candidate workers change previews correctly with full diff, SHA256, `restartRequired:true`, `applyAvailable:false`; no409.
- Candidate semantic validation rejects repeated core (`/dataplane/corelist/1`), worker/main overlap (`/dataplane/mainCore`), non-power-of-two RX descriptor (device rxDesc pointer). PATCH stages these candidates; `/config/validate` is the semantic400 route.
- Actual `/etc/vpp/startup.conf` SHA256 remains identical; shared service PID/NRestarts are compared before/after.
- Generated certA/keyA stored as secrets → real commit → OpenSSL peer certificateA; certB commit switches toB at unchanged API PID.
- MinimumTLS1.3 rejects TLS1.2 and accepts TLS1.3.
- Mismatched key and expired certificate receive400 at the actual commit route with the expected TLS pointers.
- Raw API logs, audit rows queried using psql, state/config/candidate/secretsGETs contain no PEM markers or generated full material. Secrets/tokens are never printed.

Final run exited0. Shared VPP before/after: `MainPID=1014,NRestarts=0`; unchanged startup SHA256: `a4763491ad8b173929546ba0cf83f438e7cdfa9aecd9068f15e202852ee22af0`. Cert rotation PID: `3467029`. `tools/ci.sh check --base main` passed, including gitleaks (~25.21MB, no leaks), slot scheme and forbidden patterns. `python3 -m py_compile`, `bash -n`, and `shellcheck` for the new driver passed. Complete quick CI runs on the manager's identical product source; no local complete quick pass is claimed.

No product code defects observed. Remaining acceptance: browser screenshots (no installed browser found in PATH, `/opt`, `/root/.cache`, `/usr/bin`; manager's official Playwright CDN download failed HTTP403 "service not available in your location", log `artifacts/test-closeout/browser-install.log` in manager workspace), and broader API e2e/complete unchanged quick gate coordinated by manager. No download mirrors or location bypasses attempted. This driver's live API cases are not a claim that the separate Vitest API e2e suite ran. Host API portions of F-management-ui-host and F-dataplane-ui-host are satisfied; screenshot requirement must be explicitly deferred if rows are closed.

Next exact command (with reserved slot9 available):

```bash
NGFW_ACCEPTANCE_API_MAIN=/root/.codex/worktrees/6189/NGFW/apps/api/dist/main.js NGFW_ACCEPTANCE_AGENT=/root/.codex/worktrees/6189/NGFW/apps/agent/bin/ngfw-agent bash test/topology/management-dataplane/run.sh
```
