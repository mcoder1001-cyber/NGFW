# بازبینی کد مرج‌نشده و سلامت محصول — ۱۷ اکتبر ۲۰۲۶

درخواست مالک: «اول مطمئن شو کد مرج‌نشده‌ای نمانده؛ بعد بررسی کن محصول مشکلی نداشته باشد.»
این گزارش فقط بازبینی است؛ هیچ کد محصولی تغییر نکرده و هیچ شاخه‌ای مرج یا حذف نشده است.

## ۱. وضعیت مرج در گیت‌هاب

| مورد | نتیجه |
|---|---|
| main گیت‌هاب | `99a774734e2cf455f71563ff0dba5afb58051685` (PR237، ۱۴ اکتبر) |
| PR باز | صفر |
| Issue باز | صفر |
| شاخه‌های ریموت غیر از main | ۸ شاخه (جدول زیر) |
| CI روی main | CI gate، Packaging، Provisioning همگی موفق روی `99a774734` |
| تاریخچهٔ PRهای squash‌شده | همهٔ ۲۳۷ head در `refs/pull/N/head` موجود است؛ SHAهای آرشیو PR180/193/196/197 و 214 روی گیت‌هاب قابل دسترسی‌اند |

### شاخه‌های ریموت (مقایسهٔ مستقیم با main)

| شاخه | خطوط محصول که در main نیست | وضعیت |
|---|---|---|
| `codex/complete-open-work-20261014` | ۰ (درخت دقیقاً برابر main) | head PR237، مرج‌شده |
| `codex/finish-deploy-20261014` | ۰ | کد کامل در PR237 ادغام شد؛ فقط دو فایل wip/envelope باقی است |
| `claude/status-check-report-aa44ab` | ۸۷ (نسخهٔ قدیمی‌تر `deploy/deploy-hosts.sh`) | پیش‌نویس اولیهٔ همان ابزار؛ نسخهٔ main کامل‌تر است (build + deploy هر دو را دارد) |
| `codex/restore-ethernet-display-dual-host` | ۲ (فرمول نسخهٔ قدیمی prepare.sh) | head PR236؛ محصول مرج‌شده، فرمول نسخه با DEC-248 جایگزین شد |
| `archive/complete-open-work-a8b5d2c1` | ۰ | کاندیدای اول PR237، جایگزین‌شده |
| `codex/finish-open-work-20261014` | ۰ (فقط docs) | بازنویسی `DEFERRED-ACCEPTANCE.md` و یک گزارش فارسی؛ مرج‌نشده به تصمیم بازبینی خودکار |
| `codex/finish-resource-records-20261014` | ۰ (فقط docs/records) | سوابق عملیاتی دو appliance با نشانی‌های داخلی؛ بازبینی خودکار انتشارش را رد کرد |
| `codex/appliance-half-resource-defaults` | ۰ (اسکریپت‌های یک‌بارمصرف زیر docs/status/tasks) | سوابق عملیاتی، نه کد محصول |

نتیجه: **هیچ کد محصولی روی شاخه‌های ریموت نیست که در main نباشد.** تنها تصمیم باز، ادغام یا نکردنِ سه شاخهٔ مستندات/سوابق (دو مورد آخر و `finish-open-work`) است.

### PRهای بسته‌شدهٔ بدون مرج (۴۳ مورد)

head هر ۴۳ PR از `refs/pull/N/head` واکشی و خط‌به‌خط با main مقایسه شد (فقط مسیرهای محصول، بدون فایل‌های تولیدی).
همهٔ موارد با خطوط قابل‌توجه، نسخهٔ قدیمی‌تر یا موازی همان کاری هستند که از مسیر دیگری به main رسیده است:

| PR | عنوان | خطوط یکتا | شواهد جایگزینی در main |
|---|---|---|---|
| 51 | F-aaa-login (AAA increment 2) | ۲۱۰۴ | کامنت بستن: «superseded؛ همان کار با `c513dc98` روی main است». `auth.service.ts` روی main منطق `management.aaa.order` را دارد و تست‌های e2e `aaa-login/oidc/mfa-lockout` موجودند |
| 95, 66, 64 | notifications/identity resume | ~۷۱۰۰ هر یک | تماماً `packages/proto/vrx/...` پیش از تغییر نام به ngfw؛ معادل ngfw روی main |
| 65 | multiwan monitor-only runtime | ۲۶۶ | `apps/agent/internal/multiwan/` کامل روی main؛ پذیرش MultiWAN روی دو میزبان PASS (PR221) |
| 58 | dashboard/Prometheus | ۱۹۲ | `apps/agent/internal/promexport/` روی main |
| 119, 115 | deploy/vpp Debian affinity | ۱۶۸–۲۲۱ | `deploy/vpp/{build,lib}.sh` روی main با پیاده‌سازی متفاوت |
| 210 | PPPoE lifecycle fence | ۶۹ | طرح کاملاً جدید carrier (`pppoe_carrier_*.go`) در PR214/228؛ پذیرش PPPoE دو میزبان PASS |
| 108, 100, 99, 97 | mgmt-tls stream shutdown | ۳۰–۵۱ | `mgmt-tls.service.ts` روی main `upgradedSockets` و بسته‌شدن در shutdown را دارد |
| 89 | TD19 repository certificate selector | ۶۷ | `scripts/00-add-repos.sh` روی main منطق selector را دارد؛ workflow «FRR selector fixture gate» سبز |
| 188 | backup contract | ۳۸ | جدول `f_backup_run` در migration 0010 روی main |
| سایر (۲۰۹، ۲۱۳، ۲۱۵، ۱۸۱، …) | ≤۳۰ خط | — | تفاوت‌های جزئی تست/فیکسچر |
| ۱۰ مورد (۹۶، ۱۳۶، ۱۴۱، ۱۸۳، ۱۸۴، ۱۸۶، ۱۸۷، ۲۰۶، ۲۱۱، ۲۱۲) | | ۰ | کاملاً در main |

### شاخه‌های محلی (۴۹۰ شاخهٔ حذف‌شده از ریموت که فقط در این workspace باقی‌اند)

هر ۴۹۸ شاخهٔ محلی (۴۹۰ محلی + ۸ ریموت) با `git merge-tree` و شمارش خطوطی که فقط در شاخه هست و در main نیست (مسیرهای محصول، بدون فایل‌های تولیدی و docs) سنجیده شد؛ برای شاخه‌های قبل از تغییر نام vrx→ngfw (۳ اکتبر، `bda4cc46f`) مقایسه با آخرین main پیش از تغییر نام هم انجام شد. چون تغییر نام محتوای تقریباً همهٔ فایل‌ها را عوض کرده، برای آن‌ها وضعیت برد (همهٔ ۱۳ ردیف `task/*` منطبق: merged) و بازبینی ادغام PR126 (۳ اکتبر) مرجع است.

| دسته | تعداد | نتیجه |
|---|---|---|
| هیچ خط محصولِ یکتا | ۱۰۳ | کاملاً در main یا فقط docs |
| ۱ تا ۳۹ خط یکتا | ۱۲۶ | تفاوت‌های جزئی تست/فیکسچر، نسخه‌های موازی بازبینی |
| ≥۴۰ خط، پیش از تغییر نام (تا ۳ اکتبر) | ۳۶ | شاخه‌های `task/*` و checkpointهای vrx؛ ردیف برد merged؛ طرح‌های بازنشسته (پلاگین strongSwan/kernel-vpp، `subsystems/ipsec.go` قدیمی) طبق DEC-ipsec-route-based و DEC-independent-ra-vpn عمداً وارد نشده‌اند |
| ≥۴۰ خط، RA VPN (۵–۷ اکتبر) | ۱۳۸ | تاریخچهٔ بازبینی/تکرار موتور RA پیش از PR193؛ همهٔ فایل‌های اصلی روی main هستند؛ head نهایی PR193 (`b0367efb`) مرج و در آزمایشگاه پاس شده |
| ≥۴۰ خط، PPPoE (۴–۵ اکتبر) | ۲۱ | طرح‌های «VPP session API patch 0006» و «native transport» که با D-243 (carrier هسته، ۹ اکتبر) جایگزین شدند؛ پذیرش PPPoE دو میزبان PASS |
| ≥۴۰ خط، ابزار آزمایشگاه | ۴ | اسکریپت‌های `test/topology` (private-fib، autoblock، gb-scale) که پذیرش‌شان بعداً با اسکریپت‌های دیگر PASS شد؛ کد محصول نیست |
| ≥۴۰ خط، سایر | ۶۵ | نسخه‌های قدیمی‌ترِ همان تغییرات (backup-restore با migration 0011 که در main به‌صورت 0010 است، BFD، traffic، wizard، setup-wan-no-lan) |

**پیاده‌سازی‌های موازیِ جایگزین‌شده (نه گم‌شده):**
`claude/ra-vpn-20261006` (موتور RA در زیرپکیج `raengine/`؛ PR193 با طرح دیگری مرج شد)، `codex/closeout-pppoe-client-transport-reviewed` (PPPoE native)، `codex/ra-process-identity-design-20261006` (`processidentity/` که در head PR193 نیست).

**یک مورد واقعیِ مرج‌نشده — شاخهٔ محلی `main` (dev-host):**
شاخهٔ محلی `main` این مخزن (`ba55d2744`، ۱۰ اکتبر) دنبالهٔ قدیمی dev-host است: ۳۳۵ کامیت vrx-bot (۲۸ سپتامبر تا ۱۰ اکتبر) که هیچ‌کدام جدّ main گیت‌هاب یا PR126 نیستند؛ merge-base با origin/main `7f9f503b3` (۳۰ سپتامبر). محتوای تا ۲ اکتبر از مسیر PR126 و ردیف‌های برد به گیت‌هاب رسیده (نمونه‌ها تأیید شد: S-alarms-restart-rebuild، login banner، sysident، licensing). دو کامیت بعدی:

| کامیت | وضعیت در main گیت‌هاب |
|---|---|
| `1a5306df5` feat(licensing): Bash license generator (۳ اکتبر) | نسخهٔ کامل‌تر همان اسکریپت در `tools/license/generate-license.sh` روی main است (PR129) — ادغام‌شده |
| `76484d79f` fix(interfaces): register missing data NICs automatically at boot (۱۰ اکتبر؛ ۱۵ فایل، +۴۵۳) | **در main نیست.** main فقط در first boot (بدون revision) NICها را seed می‌کند (`seed.service.ts` روی main: «never re-seeds once any revision exists»). این کامیت به دستور مالک (۱۰ اکتبر) NICهای غایب را در هر بوت و برای نصب‌های موجود با system commit ثبت می‌کند و `PENDING-automatic-network-bootstrap.md` را به «implemented» می‌برد؛ روی گیت‌هاب همان سند هنوز «Awaiting the product owner's answers» است. بخش agent آن (تشخیص virtio PCI در `hostfacts.go`) روی main هست؛ بخش API (`default-vpp-nics/seed.service.ts`، `ngfw-api.service`، تست‌ها و مستند) نیست. هیچ PR یا شاخهٔ ریموتی برای آن وجود ندارد. |

نتیجهٔ بخش ۱: روی گیت‌هاب هیچ کد محصول مرج‌نشده‌ای نیست؛ در workspace محلی تنها «ثبت خودکار NIC در هر بوت» (`76484d79f`) تغییر رفتاری‌ای است که به main نرسیده و نیازمند تصمیم مالک برای port روی main فعلی (با نام‌گذاری ngfw و بازبینی مستقل) یا کنار گذاشتن است.

## ۲. سلامت محصول

### گیت کامل quick به‌صورت محلی روی درخت main (`99a774734`)

`tools/ci.sh quick` بدون تغییر، با همان متغیرهای CI، در این workspace اجرا شد: **CI GATE PASSED** (۱۵ دقیقه و ۵۹ ثانیه).
شامل: generate/gen-check، gitleaks، forbidden patterns، turbo lint/typecheck/test/build (۳۵/۳۵)، agent و CLI (lint/test/build)، ماژول‌های Go تست (حالت unit)، shellcheck و ۱۴۹ سناریوی apply-startup روی دو shard.
یک هشدار استاندارد: خطوط `ALLOW:` فقط در فایل‌های تست (openssl برای گواهی تست).
تست‌های integration/آزمایشگاهی اینجا اجرا نمی‌شوند (`NGFW_INTEGRATION` خالی)؛ وضعیت آن‌ها در `DEFERRED-ACCEPTANCE.md` است.

| بررسی اضافی | نتیجه |
|---|---|
| `pnpm audit --prod` | صفر آسیب‌پذیری در وابستگی‌های محصول |
| `pnpm audit` (شامل dev) | ۶ مورد فقط در ابزار توسعه: vitest/@vitest/mocker (moderate)، esbuild (moderate)، source-map-js (high)، tinypool ×۲ (critical). هیچ‌کدام در بستهٔ نصبی نیست |
| `govulncheck` (agent، cli) | اجرا نشد: `vuln.go.dev` پاسخ 403 داد |
| تست‌های ابزار استقرار | `test_deploy_hosts.py` ۲۰/۲۰ و `test_prepare.py` ۴/۴ PASS؛ shellcheck بدون یافته |

### بازبینی مستقل مرج‌های اخیر (PR228–PR237)

سه بازبین مستقل فقط-خواندنی روی کد main؛ یافته‌های «تأییدشده» را خودم در کد دنبال کردم.

**الف) ابزار استقرار میزبان نصب‌شده (PR237، `deploy/deploy-hosts.sh`) — BLOCK از بازبین؛ دو نقص واقعی**

1. **بالا — نگهبان downgrade فقط برای کامیت‌های هم‌نسل درست است.** نسخهٔ dev برابر «تعداد کامیت‌های قابل‌دسترس + hash» است (`prepare.sh:26,91`، DEC-248) و مقایسه با `dpkg --compare-versions ge` (`deploy-hosts.sh:206`). شاخه‌های کاری با checkpointهای زیاد شمارهٔ بزرگ‌تری از main می‌گیرند؛ نصب شاخهٔ قدیمی‌تر روی appliance‌ای که main جدیدتر دارد پذیرفته می‌شود و برعکس، بعد از نصب یک build شاخه، buildهای main به‌عنوان downgrade رد می‌شوند و راه عبوری نیست. مستند `docs/install/deploy-hosts.md` می‌گوید downgrade رد می‌شود که دقیق نیست.
2. **متوسط — مقایسهٔ متن خام مسیر پیش‌فرض IPv6.** خروجی `ip -6 route show default` قبل/بعد مقایسه می‌شود (`deploy-hosts.sh:209,253`). مسیرهای RA دارای `expires NNNsec` هستند و همیشه تغییر می‌کنند؛ روی هر شبکهٔ مدیریتی با RA، نصبِ موفق «management default route changed» گزارش می‌شود و باقی fleet متوقف می‌شود. فیکسچر تست برای `ip -6` خروجی خالی دارد و این مسیر تست نشده.
3. متوسط — سلامت agent بعد از restart فقط با یک `systemctl is-active` سنجیده می‌شود؛ agentی که چند ثانیه بعد crash می‌کند «active» است و میزبان OK ثبت می‌شود.
4. موارد کم‌اهمیت: اسکریپت از stdin به `bash -s` داده می‌شود (هر فرزندی که stdin بخواند ادامهٔ اسکریپت را می‌بلعد؛ فعلاً فرزندی یافت نشد)، nginx بعد از ارتقا reload نمی‌شود، میزبان بدون default route رد می‌شود، نیاز به Python ≥۳.۱۱ مستند نشده، الگوی host زیرخط را رد می‌کند.

**ب) احراز هویت پیش‌فرض و ویزارد راه‌اندازی (PR233، PR234، PR236)**

1. **بالا (تصمیم مالک) — `admin/admin` هیچ‌جا مجبور به تغییر نمی‌شود.** D-247 صراحتاً admin/admin و رمز کوتاه را خواسته است؛ اما تنها فشار به سمت ویزارد، redirect داشبورد است (`DashboardPage.tsx:21`). مسیرهای دیگر وب، REST API و CLI به تکمیل setup یا تغییر رمز وابسته نیستند؛ اپراتوری که مستقیم به `/system/interfaces` برود یا از CLI استفاده کند می‌تواند admin/admin را برای همیشه روی HTTPS مدیریتی نگه دارد. این نقص کد نیست، پیامد تصمیم است؛ تصمیم «اجبار تغییر رمز در اولین ورود» با مالک است.
2. **متوسط — پاک‌سازی NAT/ACL هنگام تغییر حالت WAN از PPPoE فقط وقتی `input.lan` باشد** (`packages/schema/src/setup.ts:145-161`)، در حالی که ویزارد جدید `lan` نمی‌فرستد. دستگاهی که با ویزارد قدیمی (LAN+PPPoE) راه‌اندازی شده و حالا به DHCP/static می‌رود، با خطای گمراه‌کنندهٔ «Remove unrelated references to setup-pppoe» متوقف می‌شود.
3. **متوسط — برچسب `EthernetN` موقعیتی است** (`display-names.ts:11-28`، `interface_labels.go`): کارت‌های inventory-only مثل NIC مدیریت را هم می‌شمارد که فقط وقتی `hostNics` موفق باشد ظاهر می‌شوند؛ در قطعی inventory یا افزودن/حذف NIC همهٔ پورت‌های بعدی شماره‌گذاری مجدد می‌شوند و «Ethernet5» در picker ممکن است پورت دیگری را انتخاب کند. مستند `docs/user/interfaces/basics.md` برچسب‌ها را «پایدار» می‌نامد.
4. **متوسط (کارایی) — `InterfaceDisplayNamesProvider` در `App.tsx` سوار شده** و هر تب احراز‌هویت‌شده در هر صفحه هر ۳ ثانیه `/api/v1/state/interfaces` را می‌کشد (Retrieve از agent + dump زنده + hostNics + stats + دو خواندن DB). قبلاً فقط صفحات interfaces این poll را داشتند.
5. کم‌اهمیت: برچسب تکراری در وب «اولین» را می‌گیرد و در CLI «ambiguous» رد می‌شود؛ `show interfaces Ethernet1 --json` برچسب را resolve نمی‌کند؛ متن انگلیسی «At least 1 characters»؛ `default-admin-20261012-wip.md` با «final reviewer verdict pending» و رکورد TD2 (timeout ۶۰ ثانیه، ۱۴ تست skip) بدون به‌روزرسانی مرج شده.
6. درست یافته شد: raه‌اندازی بدون WAN کامل می‌شود؛ NIC مدیریتی در UI و API از انتخاب حذف می‌شود؛ i18n fa/en کامل است؛ رمز خالی رد می‌شود (حداقل ۱، حداکثر ۱۰۲۴ در همهٔ لایه‌ها).

**ج) PPPoE و RA VPN در agent (PR228–PR232) — همهٔ پنج کامیت APPROVE؛ بدون نشت secret**

1. **متوسط (تأییدشده) — بازخوانی RA گواهی را خالی‌شده برمی‌گرداند** (`apps/agent/internal/desired/ra_vpn.go:112,119,207-215`): فقط `CertificateRef/PrivateKeyRef/Ca` و برای CA فقط `CertificateRef`. هیچ زیرسیستم دیگری `Vpn.Pki.Certificates` را در readback پر نمی‌کند؛ گواهی‌ای با `description`، `expiry_alert_days`، `key_spec` یا CA با `crl/ocsp_url` در `GET /api/v1/state/drift` به‌عنوان drift کاذب `vpn` ظاهر می‌شود. تست `ra_vpn_test.go:67-74` فقط با گواهیِ دو-ref پاس می‌شود. نیاز به تست با گواهی کامل.
2. متوسط (مشکوک) — `promoteCarrierIPv4` (`pppoe_carrier_primary.go:55`) هر IPv4 اضافی را «foreign» می‌داند و `rt.mirrored` فقط در حافظه است؛ اگر PPP در زمان خاموشی agent با IP جدید وصل شود، بعد از restart آدرس /32 قدیمی ممکن است carrier را برای همیشه not-ready کند. باید در آزمایشگاه با سناریوی «reconnect در زمان restart agent» بررسی شود.
3. متوسط (کارایی) — `multiwan/cleanup.go:84-90` صفحهٔ دارای حذف را تا ۳۲ بار در همان فراخوانی تکرار می‌کند و هر `UserSessions` کل stream را می‌خواند؛ با ده‌ها هزار session روی WAN مرده، قفل تراکنش تا ۲ ثانیه در هر tick گرفته می‌شود. کران‌دار است، spin ندارد.
4. کم‌اهمیت: `Records/Sessions/Ready` با read-lock و گیت observation مشترک می‌توانند زیر polling همزمان UI «engine unavailable» کاذب بدهند؛ `Start` می‌تواند تا ~۱۰ ثانیه write-lock نگه دارد؛ سناریوهای canonical/TLS فقط با `NGFW_RA_ENGINE_ROOT` اجرا می‌شوند و در CI skip هستند.

### پذیرش‌های باز (بدون تغییر، از دفتر مرکزی)

ساخت/نصب/boot آفلاین ISO از سورس فعلی، reboot/upgrade/recovery کامل appliance، HA، کمپین‌های native فهرست‌شده در `DEFERRED-ACCEPTANCE.md`، اجرای مجدد ایزولهٔ TD2 (timeout ۶۰ ثانیه + ۱۴ skip در PR233) و متن مجوز محصول (`PENDING-P10-product-license.md`). این‌ها تغییر نکرده‌اند و هیچ‌کدام PASS فرض نشده.

## ۳. مسائل محیط این workspace (نه محصول)

- `~/.gitconfig` پراکسی `socks5h://127.0.0.1:1080` دارد که بالا نیست؛ `git fetch` شکست می‌خورد. در این بازبینی با `git -c http.proxy= fetch` دور زده شد؛ gh CLI مستقیم کار می‌کند.
- checkout اصلی `/root/NGFW` روی `d2d55984d` (PR216) است و ۲۰ کامیت از main عقب است. فایل untracked `license.vrxlic.api.env` در آن فقط `VRX_LICENSE_PUBLIC_KEYS` (کلید عمومی) دارد و در gitignore نیست.
- ۴۹۰ شاخهٔ محلیِ حذف‌شده از ریموت فقط در این workspace باقی‌اند (ریموت ۹ شاخه دارد).

## ۴. پیشنهاد اقدام (تصمیم مالک)

1. دو نقص ابزار استقرار (ترتیب نسخه بین شاخه‌های واگرا، مقایسهٔ مسیر IPv6 با `expires`) پیش از استفادهٔ عملی روی fleet اصلاح شوند.
2. تصمیم دربارهٔ اجبار تغییر رمز بعد از admin/admin (یا ثبت صریح پذیرش ریسک در D-247).
3. سه یافتهٔ متوسط دیگر (پاک‌سازی setup-pppoe، پایداری برچسب EthernetN و poll سراسری، drift کاذب RA) به‌عنوان تسک‌های کوچک جدا.
4. تصمیم دربارهٔ port کردن «ثبت خودکار NIC در هر بوت» (`76484d79f` از شاخهٔ محلی `main`) روی main فعلی، یا ثبت صریح کنار گذاشتن آن در `PENDING-automatic-network-bootstrap.md`.
5. دربارهٔ سه شاخهٔ مستندات/سوابق ۱۴ اکتبر تصمیم بگیرید: ادغامِ پاک‌سازی‌شده یا حفظ به‌صورت شاخه.
6. شاخهٔ محلی `main` را با main گیت‌هاب هم‌نام نگذارید (مثلاً به `devhost-main-archive` تغییر نام) تا ابزارها آن را با main واقعی اشتباه نگیرند؛ این گزارش هیچ شاخه‌ای را تغییر نداده است.
