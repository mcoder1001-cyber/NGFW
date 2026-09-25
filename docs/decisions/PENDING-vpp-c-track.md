# PENDING: vpp-c-track

This file replaces option D of `PENDING-vpp-host-hardening.md`, so that options A, B and C there can be answered on their own.

- raised: 2026-09-24 by the manager (ngfw-46), from the REVIEW-2026-09-24 report §7.4 (verified, D-125)
- decision: **<empty until the product owner fills it>**
- parked tasks: none. The existing fences stay: det44 disable, the gtpu error paths and the IGMP/VRRP host steps are still not driven.

## Context
- The plan's rule is "no C code in VPP" (docs/11 §4).
- VPP 26.06 crashed 10 times on vrx-a. **6 of those crashes came from 3 upstream bugs:**
  - V9: det44 disable;
  - V8: the gtpu add/del error path;
  - V22b: `ip4_options` on the packet path.

  The report said "6 upstream bugs". The correct count is 6 crashes from 3 bugs.
- Other crash-class items: V24 af_packet double close (suspected, not proven), V22a `fib_table_flush`, V12 ND-proxy OOM, and NULL dereferences in V10 cnat and V11 pnat. None of these is fixed upstream or in 26.10 rc1.
- Agent-side guards cover the crashes that configuration can trigger: TD-3, TD-5, the det44/gtpu rules, opt-in host tests and CI slot 12. V22b is different: it may be a remote DoS, and its trigger is not known yet.
- The F-vpp-debs pipeline works, but its product patch series is empty. It holds only the `0001-DEMO` patch.

## Options
| # | Option | Cost now (agent-h) | Reversal cost (agent-h) | Risk |
|---|---|---|---|---|
| 1 | Keep the rule and list the crashes as known issues | 0 | – | the 3 bugs stay reachable; containment relies on agent guards; V22b stays open |
| 2 | A bounded exception for crash fixes only. `Status: product` patches for V8, V9 and V24 and the V10/V11 guards; cherry-pick `2e2179223`; add V22b once its trigger is pinned. Each patch gets a reproducer on a `+vrx2` build outside the shared VPP and is submitted to gerrit. `VPP_LOCAL_REV=2`, shipped by P10, installed only through the handover gate | 1–1.5 agent-days | 0.5 day | medium: our own patched VPP until upstream merges the fixes |
| 3 | Keep the rule for the 21 days, then fund a VPP engineer | ~3.5–6.5 engineer-days later | – | the crashes stay reachable until then |
| 4 | Move to 26.10 | high | high | the bugs are unchanged in 26.10 |

## Recommendation
Option 2, for crash fixes only (V1–V6 stay excluded). One worker does it after wave A, and it gates nothing. First pin V22b's trigger in a manager window. Use option 3 for the rest.

## What continues meanwhile
Everything continues. The agent guards and fences stay as they are, and `PENDING-vpp-host-hardening` A/B/C can be answered independently.

## خلاصهٔ فارسی
- **مسئله:** VPP 26.06 روی vrx-a ده بار کرش کرده است. شش کرش از **سه** باگ خود VPP آمده است: det44 (V9)، مسیر خطای gtpu (V8) و `ip4_options` (V22b). این باگ‌ها نه در upstream اصلاح شده‌اند و نه در 26.10.
- **وضعیت خط پچ:** خط ساخت پچ (F-vpp-debs) آماده است، ولی سری پچ محصول هنوز خالی است و فقط یک پچ نمایشی دارد.
- **پیشنهاد:** یک استثنای محدود بر قاعدهٔ «بدون کد C در VPP»، فقط برای رفع کرش‌ها. یک ایجنت آن را بعد از موج A انجام می‌دهد و هیچ کاری منتظرش نمی‌ماند. پیش از آن، عامل V22b در یک پنجرهٔ مدیر پیدا می‌شود.
- **تا تصمیم شما:** هیچ کاری متوقف نمی‌شود و محافظ‌های فعلی ایجنت سر جایشان می‌مانند.
- **تصمیم لازم است:** لطفاً در خط `decision:` بنویسید یا در چت بگویید.
