# PENDING: system-identity

- raised: 2026-09-24 by the manager (ngfw-46), from the architecture audit (PLAN-1, D-125)
- decision: **<empty until the product owner fills it>**
- parked tasks: none. No System screen is built until this is answered.

## Context
- The `system` schema domain (`packages/schema/src/domains/system.ts`, docs/04) models the router's own identity: `hostname`, `timezone`, `banner.login`/`banner.motd` (rendered into /etc/issue, /etc/motd and the web login page), and the DNS client (`dns` upstream resolvers and `searchDomains`).
- **No board row applies any of these fields.** No agent renderer or descriptor exists for them, and the 102-item WBS (docs/11 §3) has no item for them. D7.3 is the Unbound server and D7.4 is NTP, not the box's own identity.
- A user can commit these fields today and nothing on the box changes. The new DoD ("reachable from the desired state through the API and the UI", D-125) would flag it.
- Adding or removing a WBS item needs the product owner (decision-policy #8).

## Options
| # | Option | Cost now (agent-h) | Reversal cost (agent-h) | Risk |
|---|---|---|---|---|
| 1 | A new WBS item and one row, `F-system-identity`, in wave B (deps P08, W-seed-BC). It covers the hostname (`/etc/hostname` + sethostname), timezone (`/etc/localtime`), banners, the DNS client (systemd-resolved drop-in) and one System screen | ~8 | 1 | low: one owner for one schema domain and one screen |
| 2 | Fold it into existing rows: the DNS client and timezone go to F-unbound-chrony-syslog (+4 h); banners and hostname go to F-hardening-lite (+3 h) | ~7 | 2 | medium: one domain split across a wave-B row and an S5 row, so the System screen can only be finished in S5 |
| 3 | Record it as a have-not and hide the `system` domain in the UI | 0 | 8+ later | high: a basic appliance setting is missing, and the schema holds fields that nothing applies |

## Recommendation
Option 1. These fields form one schema domain and one screen, and every appliance needs them. A single wave-B row finishes them early. Splitting them (option 2) would leave the screen half done until S5.

## What continues meanwhile
Everything continues. Only the System screen and the `system` domain renderer wait.

## خلاصهٔ فارسی
- **مسئله:** در schema بخش `system` هست (نام میزبان، منطقهٔ زمانی، بنرهای ورود و تنظیمات DNS خود روتر)، ولی هیچ کاری در بورد این مقادیر را روی دستگاه اعمال نمی‌کند. در ۱۰۲ قلم WBS هم قلمی برایش نیست.
- **پیامد:** کاربر می‌تواند این مقادیر را commit کند، ولی روی دستگاه چیزی عوض نمی‌شود.
- **پیشنهاد، گزینهٔ ۱:** یک قلم WBS و یک ردیف جدید (`F-system-identity`، حدود ۸ ساعت) در موج B، همراه با یک صفحهٔ System.
- **گزینهٔ دیگر:** پخش کار بین F-unbound-chrony-syslog و F-hardening-lite (۲). در این حالت صفحه تا مرحلهٔ S5 ناقص می‌ماند.
- **تا تصمیم شما:** فقط صفحهٔ System ساخته نمی‌شود؛ بقیهٔ کارها ادامه دارد.
- **تصمیم لازم است:** لطفاً در خط `decision:` بنویسید یا در چت بگویید.
