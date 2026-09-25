# PENDING: session-revocation

- raised: 2026-09-24 by the manager (ngfw-46), from TD-4 question Q3 (`docs/status/tasks/TD-4-questions.md` on task/TD-4), confirmed by the TD-4 review and the REVIEW-2026-09-24 report 2.3c (D-125)
- decision: **<empty until the product owner fills it>**
- parked tasks: none as whole rows. Only the demotion/deletion hunk of TD-10b (review 2.3c) waits. Logout revocation and the rest of TD-10b go ahead.

## Context
- TD-4 made **account disable** revoke tokens at once (c299859). **Role demotion** and **user deletion** through the config API do not.
- A deleted user's access token keeps working until it expires, up to 15 min, because `verifyAccess` does not read `app_user`. Refresh fails, because the row is gone.
- A demoted user keeps the old role inside the JWT until it expires, up to 15 min. An admin demoted to viewer can still make admin calls in that window.
- WebSockets already close in both cases (the `usersChanged` re-check in the relay).
- The fix is small: `syncUsers` also returns the reasons `demoted` and `deleted` (bump the generation, or call `revokeUser` by id before the delete), and `configResets` handles them.
- This changes the auth/session model (decision-policy #4).

## Options
| # | Option | Cost now (agent-h) | Reversal cost (agent-h) | Risk |
|---|---|---|---|---|
| 1 | Revoke on demotion and on deletion, the same way as disable. The user's sessions end, and a demoted user logs in again with the new role | ~1 (inside TD-10b) | trivial | low |
| 2 | Accept the window and record it: up to 15 min of the old role or of a deleted account | 0 | – | medium: a removed or demoted admin keeps admin rights for up to 15 min |
| 3 | `verifyAccess` reads the current role and generation on every request (Valkey cache) instead of trusting the JWT role | 2–3 | 1 | low; one lookup per request |

## Recommendation
Option 1. It follows the rule TD-4 already applies to disable and costs about an hour. Option 3 is only worth it if roles start changing often.

## What continues meanwhile
Everything, including the rest of TD-10b (per-session logout revocation, lockout, trusted proxy, audit gaps).

## خلاصهٔ فارسی
- **مسئله:** اگر نقش کاربری پایین بیاید یا کاربر حذف شود، توکن دسترسی‌اش تا ۱۵ دقیقه معتبر می‌ماند. برای مثال، مدیری که به نقش بیننده تغییر کرده تا ۱۵ دقیقه هنوز دسترسی مدیر دارد. غیرفعال‌کردن حساب این مشکل را ندارد و در TD-4 درست شده است.
- **پیشنهاد، گزینهٔ ۱:** در حذف و پایین‌آوردن نقش هم مثل غیرفعال‌کردن، همهٔ نشست‌های کاربر فوراً باطل شوند. حدود یک ساعت کار در TD-10b است.
- **تا تصمیم شما:** فقط همین بخش کوچک از TD-10b منتظر می‌ماند.
- **تصمیم لازم است:** لطفاً در خط `decision:` بنویسید یا در چت بگویید.
