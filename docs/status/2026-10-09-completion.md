# گزارش تکمیل NGFW — ۹ اکتبر ۲۰۲۶

کدهای غیرآزمایشگاهیِ پذیرفته‌شده در [PR214](https://github.com/mcoder1001-cyber/NGFW/pull/214) مرج شدند.
گیت کامل CI و هر سه گیت جانبی روی نسخهٔ `30de26ee` موفق شدند؛ درخت مرج با درخت تست‌شده یکسان است.
برد: **۲۱۲ تسک؛ ۲۰۵ merged و ۷ parked؛ صفر review و running**.
پذیرش محیط واقعی هنوز تأیید نشده است؛ خطاهای قبلی RA و mgmtd نیز باید در آزمایشگاه بررسی، اصلاح احتمالی و دوباره تست شوند.
مجوز معتبر محصول همچنان ورودیِ جداگانهٔ انتشار است؛ بنابراین ادعای «برای انتشار فقط تست باقی است» نمی‌کنیم.

| بخش تکمیل‌شده | نتیجه |
|---|---|
| PPP، VLAN، PD و Multi-WAN | اتصال حامل، بازیابی TAP، مالکیت مسیر، نسل NCP، برداشت forwarding هنگام خطا و تحویل gateway تکمیل و بازبینی شدند |
| مدیریت کلید و سرویس‌ها | مصرف‌کنندگان sealed credential، مالکیت تراکنش SNMP و اصلاحات امن مسیرها تأیید شدند |
| IP unnumbered، ویزارد و FRR | کد، قراردادها، رابط کاربری و اصلاحات مربوطه ادغام شدند |
| بسته‌بندی و provisioning | مالکیت فایل‌ها، هویت عمومی، وابستگی‌های pinned و نصب آفلاین در گیت‌های مربوطه موفق شدند |
| مستندات و برد | وضعیت جاری، شواهد CI و پذیرش‌های باقی‌مانده تطبیق داده شدند؛ سابقهٔ شکست‌ها حفظ شد |

هفت ردیف باقی‌مانده: `F-ra-vpn`، `P12-fib-proof`، `F-global-blocking-host`،
`F-pppoe-client-host`، `F-multiwan-host`، `F-nat46-host` و `F-ospf-host`.
جزئیات اجرای واقعی در [دفتر پذیرش آزمایشگاهی](DEFERRED-ACCEPTANCE.md) ثبت است؛
تست واحد و fixture جایگزین نصب، بسته‌های واقعی، سرویس‌ها و مرورگر نیستند.

## Final integrated source receipt — 2026-10-09

Source candidate `30de26ee6a5697a3713fe4375399b86c5588a472` passed the unchanged complete hosted quick gate
[37903333143](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333143) and all applicable fixture workflows.
[PR214](https://github.com/mcoder1001-cyber/NGFW/pull/214) merged as `d58db1a673d716ebcf595a9e49847a54991c58da`; merge tree equals tested tree
`d1de8b7c01220ce03b311d0a7dcafd5fe12ab1ad`. Reviewed history is preserved at
`codex/archive-completion-corrected-20261009` (`3f98838119ed8d67aab9c226308829ac49af9def`).
Final R2/R4 correction receipts approve the scoped source; initial failed CI and
local environment failures remain historical evidence, not retroactive PASS.

| Final check | Result |
|---|---|
| Mandatory quick | PASS, complete unchanged hosted gate |
| [Packaging37903333245](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333245) | 81 fixtures, zero failures/errors/skips; seven gate-policy controls PASS |
| [Provisioning37903333184](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333184) | 46 strict +23 offline Debian +11 trusted installer +18 portable export PASS |
| [Python37903333309](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333309) | 14 synthetic-wheel +5 release-contract tests PASS |

Board: **205 merged,7 parked,0 review,0 running**, total212. The eight reviewed
source rows are merged. PPP-host and MultiWAN-host source is integrated; their
native acceptance joins the five already parked rows. No live source worker is
claimed. All older pending-source/CI statements in this document are historical
and superseded by this receipt.

Native acceptance of this cumulative source remains **NOT RUN**. Known prior RA
supplier/post-ACK identity failures and P12 mgmtd startup failure at the unchanged
30-second deadline before the200-route proof still require diagnosis, any necessary
fix and rerun on the real target. No unit/fixture result closes these cases.
Product license text/name/copyright authority remains a separate release input
under `docs/decisions/PENDING-P10-product-license.md`; no license is invented.
Plan exclusions remain unchanged. This is source completion, not release certification.
