# Decision policy — approved by the product owner, 2026-09-23

**Agents decide and keep working. The product owner reviews afterwards and may overturn.
Only the cases below wait.**

## The 2× rule
For a decision raised while doing task T, estimate in agent-hours:
- `a` = cost of task T
- `b` = cost to reverse the decision later if it turns out wrong (rework in this and other packages)

If `b > 2a` → **PENDING** (write the file, park only the tasks that depend on it, continue everything else).
Otherwise → **decide, log one line, proceed.**

## Always PENDING, whatever the estimate
1. Changing the *shape* of `packages/schema` or `packages/proto` in a way that deviates from `docs/04-api-datamodel.md` (adding fields/domains as designed there is fine; renaming/reshaping is not)
2. Cross-cutting data-model changes (multi-tenancy is already decided: deferred, see `vdom.md`)
3. Replacing the OS, the VPP version, or a base framework (NestJS, React, MUI, Go, govpp, PostgreSQL)
4. Security boundary: agent privileges, socket permissions, auth/session model, secret storage
5. Licensing exposure: linking GPL code, copyleft dependencies, commercial licences (e.g. MUI X Pro)
6. Anything that costs money or needs the product owner's hands (hardware, vSphere VMs, NICs, accounts)
7. Destroying data or git history; touching `/root/vpp`, `/etc/vpp`, `vpp.service` while `docs/lab/host-vrx-a.md` says `handover: pending`
8. Adding or removing whole WBS items from the 21-day plan (`docs/11-compressed-plan-fa.md` §3)

## Procedure
- **PENDING:** copy `TEMPLATE-PENDING.md` to `PENDING-<slug>.md`; fill context, options, costs, recommendation, parked tasks; mention it in the next `docs/status/` entry; commit. Park dependent tasks in `plan/tasks.yaml` (`state: parked`, `parked_on: PENDING-<slug>`). If *everything* would be parked, implement the recommended option on a branch `provisional/<slug>` and do not merge it to `main`.
- **Answered:** the product owner writes the answer in the file's `decision:` line (or says it in chat); the manager renames it `DEC-<n>-<slug>.md`, logs it, unparks.
- **Decided by an agent:** one line in `LOG.md`:
  `| YYYY-MM-DD | D-nnn | decision | why | reversal cost | tasks affected |`
- **Overturned:** the product owner appends `OVERTURNED → <what instead>` to the LOG line or says so in chat; the manager opens a follow-up task. Nobody argues.

## خلاصهٔ فارسی
ایجنت‌ها خودشان تصمیم می‌گیرند و کار را جلو می‌برند؛ هر تصمیم در `LOG.md` یک خط ثبت می‌شود و شما بعداً بازبینی و در صورت نیاز برگشت می‌دهید. فقط وقتی هزینهٔ برگشت‌دادن یک تصمیم از **۲ برابر هزینهٔ همان کار** بیشتر باشد — یا در فهرست «همیشه بپرس» بالا باشد (تغییر شکل قرارداد، مدل داده، OS/VPP/فریم‌ورک، مرز امنیتی، مجوز، هزینهٔ مالی، حذف داده، دست‌زدن به VPP قبل از تحویل، تغییر دامنهٔ طرح) — فایل `PENDING-*.md` نوشته می‌شود، فقط کارهای وابسته متوقف می‌شوند و بقیه ادامه پیدا می‌کنند. کار هیچ‌وقت به‌طور کلی متوقف نمی‌شود.
