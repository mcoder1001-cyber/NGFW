# Development recovery — 2026-10-02

چهار شاخهٔ توسعه از checkpointهای قبلی بازیابی شدند؛ بازسازی از صفر انجام نشد.
تست‌های کامل محلی در حال اجراست؛ نتیجهٔ نهایی در گزارش هر شاخه ثبت می‌شود.
اصلاحات تازه شامل محدودیت ارسال اعلان و پوشه‌های دادهٔ بستهٔ نصب است.
مجوز مستقیم انتشار دریافت شد؛ PRهای 62، 63 و 64 منتشر شده‌اند و مرج تازه‌ای انجام نشده است.
تعداد running برد نشانهٔ ایجنت زنده نیست؛ تست‌های آزمایشگاهی همچنان اجرا نشده‌اند.

- Verified main baseline: `53a43ce5b91e71f3fedc282f9c5c54ba22fc9fb8` (PR60 build DAG fix present).
- Board validation: `board ok: 156 tasks; progress 73.0% by hours, 112/156 merged; ready=10 running=15 parked=2`.
- Four local development branches: `codex/dashboard-resume-20261002`, `codex/identity-resume-20261002`, `codex/notifications-resume-20261002`, `codex/packaging-resume-20261002`. Exact final checkpoint/test evidence is in each branch's task WIP/report.
- Publication approval resolved: the owner directly replied «من تایید میکنم و مجوز میدم» after the publication blocker was explained. A fresh shell push was approved, then failed only because CLI credentials were absent. Authorized GitHub connector publication may proceed; CI/review gates still apply. Confirmed publication: dashboard PR62 `0fc393d8`, packaging checkpoint PR63 `952cd663`, identity PR64 `df239a9b`; unchanged hosted gates started. Notifications publication pending at this checkpoint.
- Current session observed four developers and two dashboard reviewers. This is historical runtime evidence, not a persistent/live service claim after this turn ends. Unassigned running rows are explicitly awaiting resume.
- Dashboard: recovered product unchanged from remote reviewed checkpoint; integrate current build DAG fix and run unchanged quick gate. Replacement PR62 published; old PR58 remains open/unmodified until replacement integration.
- Identity: recovered later local finished work and existing independent review; fresh independent R1 code APPROVE and R2 exact-product carry-forward recorded in `tasks/identity-resume-review-R1.md` and `tasks/identity-resume-review-R2-carry-forward.md`. PR64 hosted gate pending.
- Notifications: SMTP/webhook only. Preserve test-send throttles across config reloads. Real missing event semantics remain code follow-ups, not falsely classified as laboratory-only.
- Packaging: safely create expected /data storage subdirectories without changing existing content or permissions. Independent R4 found/fixed the missing appliance VPP ID scope (`VRX_VPP_ID_RANGE=all`, appliance only). Dynamic firewall/punt integration and privilege-boundary questions remain open; do not claim all of P10 complete.
- Multi-WAN remote checkpoint `codex/network-manager-20261002` remains awaiting resume; monitor slice exists, forwarding/NAT/VRF/ABF scope not complete.
- At this checkpoint no new merge has been performed; branch publication and hosted-gate creation are in progress. Local quick results do not constitute a fresh hosted gate result. Live VPP/browser/appliance acceptance remains NOT RUN under `DEFERRED-ACCEPTANCE.md`.
