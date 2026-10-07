# گزارش پیشرفت NGFW — ۷ اکتبر ۲۰۲۶

زمان بررسی: ۱۶:۲۷ UTC، ۱۹:۵۷ تهران. main محصول پس از ادغام‌های این مرحله: `aede625c1b78702c133d21df4555dbd347a22f43`. کارهای قابل انجام بدون تغییر میزبان در چهار بخش جلو رفتند و مرج شدند؛ یک اصلاح سازگاری fixture هم پس از مشاهدهٔ شکست خودکار اضافه شد.

## ادغام‌های جدید و شواهد

| بخش | PR و merge SHA | نتیجهٔ واقعی |
|---|---|---|
| وابستگی‌های PPPoE | PR201، `1c3092efbe411566d1ab5815790a47cfd8d62352` | وابستگی مستقیم agent به ppp و pppoe اضافه شد؛ دو آزمون بسته‌بندی و بازبینی مستقل موفق. |
| کنترل‌های P12 | PR203، `188a2ea9835298466e9fc2601add2bfa587204b1` | inventory واقعی nsfs، خطاهای cleanup و کنترل هویت immutable پردازش بازیابی/اصلاح شد. ۱۱ آزمون Python روی ترکیب main موفق در0.041s؛ race آزمون‌های agent1.857s و BGP1.128s موفق. یافتهٔ PID reuse در بازبینی اولیه رفع شد؛ APPROVE دور دوم1409c4739. |
| نسخهٔ نهایی VPN | PR202، `19052bb130ab46977dc5b7aa7be25e976fb5aaf9` | delta دقیق۴۵ مسیر union نهایی بازیابی شد: transport احرازشده و محدود PID1، pagination، renderer lazy، counter/locale. بازبینی مستقل APPROVE97f83b97b؛ race ترکیب نهایی runtime3.341s، renderer7.145s، agent74.701s و سه آزمون packaging2.632s موفق. نویسندهUI11/11، typecheck و lint محدود را گذراند؛ شکست‌های اولیه در گزارش اصلی حفظ شده‌اند. تصمیم canonical PID1 نیز درPR203 حفظ شد. |
| installer TD19 | PR204، `4fcdc4557e857efb8d079bf2c52c0e2f0fedd9db` | dry-run، ریشهٔ آزمایشی با recording harness دقیق، pnpm ثابت و الزام hash closure Python؛ اجرای unchecked command، تزریق Python، shell metadata، مسیر استاندارد native و staging اصلاح شدند. ۴۶ آزمون نهایی موفق149.121s؛ بازبین۱۱ safe-root و۷ preflight و کنترل‌های مستقل را گذراند، APPROVE4c50ca949. روی ترکیب actual main،۱۱ آزمون محدود10.586s و کنترل metadata نهایی0.119s موفق؛ tree واقعی main5865c794… دقیقاً برابر tree ترکیب آزمایش‌شده بود. |
| سازگاری fixtureهای TD19 | PR207، `aede625c1b78702c133d21df4555dbd347a22f43` | دو fixture قدیمی با helper و مسیر reader فعلی هماهنگ شدند. هیچ محصول، runner، workflow، تعداد آزمون یا شرطی ضعیف نشد. آزمون‌های مستقل module6/6 موفق0.965s وFRR13/13 موفق25.082s؛ policy کنترل‌ها موفق، APPROVE5cd6c7cf. |

تاریخچهٔ بازبینی‌شده قبل از squash در شاخه‌های remote `codex/archive-pppoe-readiness-20261007`، `codex/archive-p12-recovery-20261007`، `codex/archive-ra-union-20261007`، `codex/archive-td19-safe-root-20261007` و `codex/archive-td19-source-fixture-compat-20261007` حفظ شد. main بازنویسی نشد. گزارش‌های BLOCK اولیه، اصلاحات، آزمون‌های ناموفق اولیه و تأییدهای نهایی حفظ شده‌اند.

## CI و محدودهٔ پذیرش

طبق دستور صریح مالک، گیت تجمیعی تازه به‌صورت دستی اجرا نشد. آزمون‌های محدود واقعی و بازبینی مستقل انجام شدند. GitHub به‌صورت خودکار workflow اجرا کرد: روی4fcdc، دو source-fixture قدیمی37651344590/37651344539 شکست خوردند؛ این شکست‌ها بازتولید و درPR207 اصلاح شدند. روی main جدیدaede625c1، همان workflowها37652990531 و37652990565 SUCCESS هستند. provisioning37652990588 درحال اجرا و CI gate37652990461 pending بود؛ گیت کامل نهایی PASS اعلام نمی‌شود.

هیچ نصب واقعی، deploy، تغییر بسته/سرویس/NIC، تغییر shared VPP یا startup انجام نشد. پذیرش آزمایشگاه NOTRUN باقی است؛ handover میزبان pending است. موفقیت آزمون‌های بدون میزبان واقعی، آمادگی نسخهٔ عملیاتی نیست.

## کارهای باز

برد معتبر۲۱۲ ردیف دارد:۲۰۵ merged،۲ running،۵ parked؛ ready/review/todo/failed صفر. همان هفت ردیف اصلی هنوز بازند. پیشرفت برنامه۱۵۵۱ از۱۵۷۸٫۵ ساعت تخمینی است؛۲۷٫۵ ساعت باقی‌مانده تخمین اولیهٔ کل ردیف‌های باز است، زمان واقعی باقی‌مانده یا موعد تحویل نیست.

| تسک | کار باقی‌مانده |
|---|---|
| TD-19 | قفل معتبر release برای closure وابستگی‌های Python و نصب/boot واقعی. تصمیم اعتماد repository قبلاً درDEC-238 پاسخ داده شده و مانع قدیمی برد اصلاح شد. |
| F-ra-vpn | supplier/runtime READY و پذیرش واقعی EAP/TLS، packet، session/disconnect/restart، API و browser. وضعیت running اثبات نویسندهٔ زنده نیست. |
| P12-fib-proof | علت شکست mgmtd در deadline اصلی۳۰ثانیه هنوز معلوم نیست؛ اثبات واقعی۲۰۰ route باقی است. اصلاح cleanup این شکست را PASS نکرده است. |
| F-pppoe-client-host | ایراد source transition-admission/resurrection و هویت parent PID، discovery/PADO، encapsulation، LAN delegated-prefix و پذیرش packet/FIB/MSS/reconnect/browser. این ایرادهای منبع به‌عنوان lab-only deferred ثبت نشده‌اند. |
| F-global-blocking-host | پذیرش واقعی forwarding/local-in، ظرفیت lookup و screenshot. |
| F-nat46-host | packet/FIB/rollback/restart/API واقعی. |
| F-ospf-host | rig/FIB/packet/restart/rollback/API واقعی. |

P10 هنوز محدودیت privilege/file ownership دارد؛ multi-WAN gateway handoff نیز تکمیل نشده است. درصد merged برد این پذیرش‌های باقی‌مانده را حذف نمی‌کند.

فهرست زندهٔ همین گفتگو: فقط root مدیر برای ثبت گزارش فعال است؛ توسعه‌دهندگان و بازبین‌های این مرحله با checkpoint منتشرشده پایان داده‌اند. موجودی workerهای گفتگوهای دیگر unverifiable است؛ سرویس supervisor دائمی تأیید نشده است.

PR206 جدیدِ ساختار ناوبری یک پیش‌نمایش جداگانه است؛ در متنش صریحاً آمده «Owner review only: do not merge until the owner explicitly approves.» این PR در این مرحله ادغام نشد؛ بررسی/CI ذکرشده در متن آن اثبات worker زنده در این گفتگو نیست.

گام بعدی: release Python closure و اصلاح source lifecycle PPPoE قابل ازسرگیری‌اند؛ پذیرش‌های native فقط با preflight/slot/اختیار میزبان موجود اجرا می‌شوند. این گزارش پایان کارهای محدود این مرحله است و تکمیل همهٔ قابلیت‌ها را ادعا نمی‌کند.
