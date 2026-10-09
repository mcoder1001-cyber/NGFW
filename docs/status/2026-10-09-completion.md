# گزارش تکمیل NGFW — ۹ اکتبر ۲۰۲۶

کدهای غیرآزمایشگاهی پذیرفته‌شده در شاخهٔ تجمیعی آمادهٔ CI نهایی‌اند؛ هنوز مرج نهایی ثبت نشده است.
نسخهٔ محصول بررسی‌شده `33db7c63` است؛ R2 و R4 تأیید نهایی داده‌اند.
برد: ۲۱۲ تسک؛ ۱۹۷ merged، ۱۰ review و ۵ parked برای پذیرش محیط واقعی.
تست‌های هدفمند موفق‌اند؛ CI نهایی و نتیجهٔ مرج باید پس از اجرا در همین گزارش ثبت شوند.
مجوز معتبر محصول همچنان ورودی انتشار است و تست آزمایشگاهی محسوب نمی‌شود.

| بخش | شواهد فعلی |
|---|---|
| حامل PPP، VLAN، PD، Multi-WAN | اتصال کامل و بازبینی نهایی؛ بازیابی TAP، مالکیت مسیر، نسل NCP و خطای مشاهده اصلاح شده‌اند |
| ترتیب تراکنش WAN | تست scheduler/runtime/route واقعی با مرزهای بومی شبیه‌سازی‌شده؛ نسخهٔ قبلی شکست و نسخهٔ اصلاح‌شده موفق شد |
| بررسی forwarding و probe | تست مسیر واقعی runtime، خطاهای مالکیت/توپولوژی و تغییر نشست هنگام probe؛ race موفق |
| مدیریت کلید، IP unnumbered، ویزارد، FRR و بسته‌بندی | اصلاحات بازبینی‌شده در نسخهٔ تجمیعی؛ حفظ اتصال‌ها در بازبینی ترکیبی تأیید شد |
| تولید قراردادها | ۱۳ مرحلهٔ تولید موفق؛ خروجی‌ها ثبت شده‌اند |
| آزمایشگاه | نصب/boot، بسته‌ها، VPP/FRR/PPP واقعی، سرویس‌ها و مرورگر NOT RUN؛ نتایج تاریخی فقط به نسخهٔ ثبت‌شدهٔ خود تعلق دارند |

شواهد: [دفتر تکمیل](tasks/completion-20261008-wip.md)،
[کاندید و تست‌ها](tasks/carrier-finish-20261009-wip.md)،
[بازبینی R2](tasks/recovery-20261009-review-R2.md)،
[بازبینی R4](tasks/completion-carrier-20261009-review-R4.md)،
[پذیرش باقی‌مانده](DEFERRED-ACCEPTANCE.md).

## Final gate and merge receipt

Pending: unchanged hosted quick and applicable fixture workflows on the final single-commit candidate,
then expected-head merge and exact source-tree verification. No previous green CI is claimed for this candidate.
