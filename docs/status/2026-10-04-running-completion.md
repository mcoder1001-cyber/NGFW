# Running tasks — completion campaign, 2026-10-04

تونل‌ها و IS-IS/RIPng تکمیل و در PR151 مرج شدند.
دو ریویوی مستقل سورس را تأیید کردند؛ تست‌های متمرکز و typecheck موفق بودند.
رویدادهای native IPsec و کانال رمز IS-IS هم تکمیل شدند.
HA و Multi-WAN در شاخه‌های جدا در حال تکمیل واقعی هستند.
P10 و IPsec گواهی‌محور همچنان تصمیم امنیتی می‌خواهند؛ آزمایشگاه به‌عنوان PASS ثبت نشد.

PR151 merged4508d38f1, verified main tree equals reviewedf9a9cb37e28d42be90918891910c57df7470ca90.
Source counts173/211 merged,6 running,7 ready,15 todo,10 parked,0 review.
TD19 teardown fixture corrected after current helper change; strict36/36 pass,
packaging30/30 pass, FRR pin suite13/13 and Go module6/6 pass.
Exact independent review artifacts are in docs/status/tasks/complete-running-independent-review-20261004.md
and running-security-review-20261004.md. Full CI inherited waiver remains documented;
no complete-suite/lab pass claimed. Repository current guard passes.

P10: isolated product VPP build in Packaging-complete (no host installation),
privilege/atomic-global-file ownership decision and license authority unresolved.
TD19: current fourth FRR signer and NodeSource key authority not inferred from
self-downloaded keys. Official source evidence and refused defaults retained.
Native certificates: local certificate/remoteCa cannot implement VPP peer-leaf
signature trust; explicit peer-pin + global signing-key model awaits owner answer.
