# Codex manager execution — 2026-10-01

## خلاصه
- به درخواست مالک، مدیریت توسعه و واگذاری به ایجنت‌ها شروع شد.
- P11، داشبورد واقعی، Multi-WAN، اعلان‌ها و راه‌انداز اولیه در worktreeهای مستقل شروع شدند.
- نصب محیط تست و اجرای گیت پایه در جریان است.
- هیچ تسکی بدون تست و بازبینی تأیید نمی‌شود؛ آزمایشگاه VPP در این محیط موجود نیست.
- تصمیم کانال اسرار و سخت‌سازی میزبان همچنان باز است؛ فقط بخش‌های مستقل ادامه دارند.

## Scope and merge gates
Workers own isolated branches. Product owner authorizes implementation, tests, independent review and merges. Existing private lab host is not reachable/verified here; no host modifications or restarts are attempted. Full quick gate and mandatory independent reviews are required before any product merge. Live acceptance remains outstanding for applicable features. Tasks whose prerequisites are on main but still marked review may be developed against that baseline; this does not change prerequisite review state.

## Active branches
- task/P11-codex
- task/F-dashboard-prom-alarms-host-codex
- task/F-multiwan-host-codex
- task/F-notifications-codex
- task/F-setup-wizard-codex
