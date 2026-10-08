# پیشرفت NGFW — ۸ اکتبر ۲۰۲۶
ابزار قفل وابستگی‌های نصب‌کننده در PR208 ادغام شد؛ کل TD19 هنوز تکمیل نشده است.
اصلاح FRR در PR209 و چرخهٔ PPPoE در PR210 منتشر و بازبینی شدند؛ ادغام آن‌ها باقی است.
سه آزمون FRR و چهار آزمون بازیابی PPPoE با race detector موفق شدند؛ هیچ skip نداشتند.
طبق دستور جدید مالک، CI تازه تا پایان همهٔ تسک‌ها به تعویق افتاد و در پایان یک‌بار تجمیعی اجرا می‌شود.
آزمون‌های وابسته به هویت پردازش PPPoE شکست خورده‌اند؛ ناسازگاری PID و proc در محیط اجرا مستقلاً تأیید شد، نه موفقیت محصول.

| بخش | وضعیت و شواهد |
|---|---|
| TD19 | PR208، merge417e8fcd؛ ۱۴ fixture موفق و CI قبلی37807521261 موفق19m40s؛ provenance/version/نصب واقعی باقی است. |
| FRR/P12 | PR209، source9e4c78fe؛ خطاهای G304 اولیه37807567473 با os.Root در fixtureها اصلاح شد؛ سه regression race موفق؛ علت mgmtd و اثبات۲۰۰ route باقی است. |
| PPPoE | PR210، sourcea9398271؛ بازیابی reload/restart/removal و ساخت اولیه اصلاح شد؛ چهار regression race موفق؛ golden و parsing موفق، lifecycle proc-dependent FAIL باقی است؛ CI نهایی هنوز اجرا نشده است. |

برد۲۱۲ ردیف:۲۰۵ merged،۲ running،۵ parked. این تعداد، درصد آمادگی محصول یا موجودی worker زنده نیست.
هفت ردیف اصلی همچنان بازند: TD19، RA VPN، P12، PPPoE، global blocking، NAT46 و OSPF.
پذیرش‌های واقعی شبکه/daemon/نصب/browser انجام نشده‌اند؛ نتیجهٔ NOT RUN به PASS تغییر نکرده است.
شاخه‌ها و آرشیوهای منتشرشده، تاریخچهٔ بازبینی و checkpointها را حفظ می‌کنند؛ main بازنویسی نشده است.
مرحلهٔ بعد: تکمیل پذیرش‌های نیازمند آزمایشگاه و قفل release؛ سپس اجرای واحد CI نهایی روی ترکیب نهایی و ادغام PRهای واجد شرایط.

شواهد: [PR208](https://github.com/mcoder1001-cyber/NGFW/pull/208)، [PR209](https://github.com/mcoder1001-cyber/NGFW/pull/209)، [PR210](https://github.com/mcoder1001-cyber/NGFW/pull/210)؛ [CI قبلی موفق](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37807521261).
