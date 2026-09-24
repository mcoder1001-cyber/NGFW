# PENDING: agent-privileges

This is about the **AI build agents** (the manager and the workers), not the product's `vrx-agent`. The `vrx-agent` unit's privileges are decided by P10 (D-119 M9) under decision-policy #4.

- raised: 2026-09-24 by the manager (ngfw-46), from the REVIEW-2026-09-24 report §7.3 (verified, D-125)
- decision: **<empty until the product owner fills it>**
- parked tasks: none

## Context
- `tools/manager-supervisor.sh:21-24` runs the manager's Claude CLI as root with `--dangerously-skip-permissions`. It sets `IS_SANDBOX=1` only to get past the CLI's refusal to run that flag as root.
- The fallback settings allow bare Bash, WebFetch, WebSearch and Agent. The 8 prefix deny patterns are easy to bypass.
- `deploy/systemd/ngfw-manager.service` runs as `User=root` with `Restart=always` and no sandbox. It is not installed today.
- Every agent runs as uid 0 in tmux on vrx-a. That host also runs the shared VPP, PostgreSQL, Valkey, nginx and SSH.
- The host-safety rules (no VPP restart, no `pkill`, slot prefixes and so on) are enforced only by the prompts.

## Options
| # | Option | Cost now (agent-h) | Reversal cost (agent-h) | Risk |
|---|---|---|---|---|
| 1 | Keep today's setup and document it | 0 | – | high |
| 2 | An unprivileged `ngfw` user in group `vpp`, plus sudoers entries for fixed commands. The repo path is hard-coded in `tools/lab:27`, `apply-startup.sh`, the supervisor and `ci.sh`, so these must change too | 8–12 | 1 | low–medium |
| 3 | Stay root but reduce the blast radius: `Restart=on-failure` with `StartLimitBurst`; `ReadOnlyPaths=/etc/vpp /root/vpp /boot`; `ProtectKernel*`; drop `IS_SANDBOX`; give workers web access only when their envelope needs it; add an egress allow-list | 2–3 | 0.5 | medium (tmux and `systemd-run` escape it) |
| 4 | A separate agent VM that drives vrx-a over SSH with a restricted key | vSphere work + ~8 | 2 | lowest |

## Recommendation
Do option 3 now and aim for option 4. Use option 2 only if option 4 is refused. Move `ngfw-manager.service` from `deploy/systemd/` to `tools/manager/`, because it is not a product unit.

## What continues meanwhile
Everything continues. Nothing is parked.

## خلاصهٔ فارسی
- **موضوع:** این پرونده دربارهٔ ایجنت‌های هوش مصنوعی (مدیر و کارگرها) است، نه `vrx-agent` محصول.
- **مسئله:** مدیر و ایجنت‌ها با root و بدون محدودیت روی خود روتر vrx-a اجرا می‌شوند. همین میزبان VPP مشترک، PostgreSQL، Valkey، nginx و SSH را هم اجرا می‌کند. قواعد ایمنی میزبان فقط در متن پرامپت‌ها آمده‌اند.
- **پیشنهاد:** همین حالا محدودسازی ارزان (گزینهٔ ۳)، و بعداً یک VM جدا برای ایجنت‌ها (گزینهٔ ۴). گزینهٔ ۲ (کاربر بدون root) فقط اگر VM جدا ممکن نباشد.
- **تا تصمیم شما:** هیچ کاری متوقف نمی‌شود.
- **تصمیم لازم است:** لطفاً در خط `decision:` بنویسید یا در چت بگویید.
