# PENDING: tools-app-transport

- raised: 2026-09-24 by the manager (ngfw-46), from TD-4 question Q2 (`docs/status/tasks/TD-4-questions.md` on task/TD-4), confirmed by the TD-4 review (I3) and the architecture audit (D-125)
- decision: **<empty until the product owner fills it>**
- parked tasks: none as whole rows. Only **TD-4's merge** waits for the answer. TD-10b owns the `tools/app` vite hunk that implements it.

## Context
- `tools/app` is the running product stack on vrx-a. It serves the web UI with `vite preview` on **`0.0.0.0:8080` over plain HTTP**, and vite proxies `/api` to the API on `127.0.0.1:3000`.
- The API sees a loopback peer, so it treats every request as TLS-terminated (D-100). Logins through `:8080` are therefore accepted, while the password crosses the LAN in clear between the browser and vite.
- D-100 (1) forbids exactly this relay pattern for the product nginx (P10). Its assumption "every local relay is TLS-terminated" does not hold for `tools/app`.
- Once TD-10b adds `VRX_TRUST_PROXY` and vite `xfwd`, the API will see `X-Forwarded-Proto: http` and refuse these logins (`403 tls-required`). The lab UI then stops working unless one of the options below is in place.
- This is a transport and security-boundary question (decision-policy #4).

## Options
| # | Option | Cost now (agent-h) | Reversal cost (agent-h) | Risk |
|---|---|---|---|---|
| 1 | Bind the web UI to `127.0.0.1` by default. Remote use goes through an SSH tunnel; an opt-in `VRX_APP_WEB_HOST=0.0.0.0` prints a warning | 0.5 | trivial | low; everyone who opens the lab UI from a desktop needs a tunnel |
| 2 | Self-signed TLS on the vite server. A cert is generated once under the `tools/app` state dir, and plain `:8080` is turned off or redirects | 1.5–2 | 0.5 | low; one browser warning per client |
| 3 | Record `tools/app` as a lab-only exception in D-100 and keep plain `0.0.0.0:8080`. TD-10b then also needs an exception so that trusted-proxy HTTP from vite is still accepted | 0 (+0.5 in TD-10b) | 1 | medium: lab and admin passwords travel in clear on the lab LAN |

## Recommendation
Option 2. The product owner and reviewers open the lab UI from their desktops, so option 1 costs them a tunnel on every visit. Option 3 keeps cleartext passwords on the LAN and contradicts D-100. With option 2, the lab behaves like the product: TLS at the edge, loopback behind it.

## What continues meanwhile
Everything except TD-4's merge. TD-10b builds `VRX_TRUST_PROXY` and `xfwd` either way; only the `tools/app` listener hunk depends on the answer.

## خلاصهٔ فارسی
- **مسئله:** رابط وب آزمایشی (`tools/app`) روی `0.0.0.0:8080` و با HTTP ساده اجرا می‌شود. رمز عبور کاربران در شبکهٔ داخلی بدون رمزنگاری جابه‌جا می‌شود، ولی API آن را می‌پذیرد، چون درخواست از loopback می‌رسد.
- **پیشنهاد، گزینهٔ ۲:** TLS خودامضا روی `tools/app`. مرورگر یک بار هشدار می‌دهد و بعد همه‌چیز مثل محصول نهایی کار می‌کند.
- **گزینه‌های دیگر:** محدودکردن رابط به `127.0.0.1` و دسترسی با تونل SSH (۱)، یا ثبت یک استثنای مخصوص آزمایشگاه (۳).
- **تا تصمیم شما:** فقط merge شاخهٔ TD-4 منتظر می‌ماند؛ بقیهٔ کارها ادامه دارد.
- **تصمیم لازم است:** لطفاً در خط `decision:` بنویسید یا در چت بگویید.
