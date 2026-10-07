# کارهای باقی‌مانده پس از ادغام همه PRها

Snapshot: 2026-10-07، حدود19:05 تهران؛ remote mainf6ae6e555ecb1edf5397ff1a8115076ebfc11d38. گزارش بر اساس برد همین SHA، رسید ادغام‌ها، WIPهای منبع، تصمیم‌های فعلی و وضعیت GitHub است. چهارPR197/196/193/180 و گزارشPR200 ادغام شده‌اند؛ PR باز صفر. CI خودکارmain37643967099 در زمان بررسیin_progress؛ دستور قبلی مالک برای عدم اجرای گیت تازه حفظ شد، گیت جدیدی توسط عامل اجرا نشد.

## اندازه قطعی باقی‌مانده

۲۱۲ردیف برد:۲۰۵merged،۱running،۶parked.۷ردیف باز =۳٫۳٪ از تعداد. مجموع ساعات تخمینی اولیه برنامه۱۵۷۸٫۵، سهم۷ردیف باز۲۷٫۵ساعت؛ این اندازه اولیه تسک‌هاست، نه اندازه‌گیری کار فعلی یا وعده زمان اتمام. درصد۹۸٫۳ساعت/۹۶٫۷تعداد فقط درصد ادغام برد است، نه درصد آمادگی production. وقت واقعی برای تشخیص عیب‌ها، اصلاحات، پذیرش‌های معوق تسک‌هایmerged و آماده‌سازی اهداف در این۲۷٫۵ منظور نشده است. تاریخ دقیق اتمام از این داده‌ها قابل دفاع نیست.

| ردیف | ساعت تخمینی اولیه | مانع/کار فعلی |
|---|---|---|
|F-ra-vpn|10|سرPR193 ادغام شد؛ شاخه بعدیcodex/integrate-ra-final-20261005 هنوز در711e18e12ac2776ad4fc15066f811b60599e63f2 است و union بعدی خودکار ادغام نشده. تثبیت اصلاحات بعدی، آمادگی supplier و real EAP/TLS/session/disconnect/restart/API/browser باقی است. Boot38 در متن قدیمیPR ادعای تاریخی running دارد؛ نتیجه فعلی از آن استنباط نمی‌شود.|
|TD-19|3.5|حذف ابزارهای container/KVM درPR197 ادغام شده. NGFW_INSTALL_ROOT/--dry-run، pinهای Python/pnpm و followupهای inventory/exact-version provisioning و نصب/boot واقعی باقی است. اعتماد مخازن دیگر منتظر تصمیم نیست.|
|P12-fib-proof|3|driver/proof recovery و conflict resolution ادغام شد؛ اجرای واقعی mgmtd در deadline30s شکست خورده، قبل از proof200routes. علت نامعلوم، تشخیص و تکرار واقعی لازم است.|
|F-global-blocking-host|2|topology/forwarding/local-in/lookup و screenshot واقعی باقی است.|
|F-pppoe-client-host|2|IPv6 ادغام شده؛ product dial بدونPADO شکست می‌خورد وVPP encapsulation برایLANtransit ناقص است. LANdelegated-prefix فقط گزارش می‌شود و تخصیصش ساخته نشده. real reconnect/NAT/MSS/browser پذیرش لازم دارد.|
|F-nat46-host|3|driver آماده، packet/FIB/rollback/restart/API واقعی باقی است.|
|F-ospf-host|4|driver/بخش‌هایی ازشواهد آماده، fullrig/FIB/withdrawal/rollback/restart/API واقعی باقی است.|

## اصلاح شواهد قدیمی

گزارش قبلی و برد هنوزTD19 را رویPENDING-TD19-repository-trust متوقف نشان می‌دادند. خود این فایل «answered» است؛ DEC-238 تصمیم مالک۶اکتبر option2 را ثبت کرده و pins درscript00 پیاده شده‌اند. این مانع قدیمی باید با کار منبع/پذیرش واقعی جایگزین شود؛ انتظار تصمیم اعتماد مالک لازم نیست. این گزارش stateبرد را خودسرانه تغییر نمی‌دهد.

WIPPPPoE قدیمی نبودنdhcpcd-base درDepends را می‌گوید؛ main فعلی dhcpcd-base دارد، اما ppp/pppoe درDepends همان package نیامده‌اند. متن قدیمی اثبات نبودن هر سه dependency نیست. پیش از انتشار appliance، پوشش واقعی runtime dependencyها باید با قرارداد بسته‌ها تطبیق یابد.

چهار ردیفRA/TD19/P12/PPPoE هنوز ادغام/اصلاح منبع یا تشخیص شکست واقعی لازم دارند؛ سه ردیفglobal-blocking/NAT46/OSPF عمدتاً پذیرش میزبان هستند. صفرPRباز به معنی صفرکار کدنویسی یا صفرپذیرش باز نیست.

## کارهای باز بیرون از این۷ردیف

پروندهDEFERRED-ACCEPTANCE شامل پذیرش‌های تسک‌هایmerged هم هست؛ این‌ها در درصد۷/۲۱۲ محاسبه نشده‌اند:
- نصب/boot/upgrade واقعیappliance، signing وmanifest و migration/firstboot و daemon renderers زیر sandboxنصب‌شده؛ imageهایVM/cloud وfirmware/A-B rollback.
- زنجیرهpacket واقعیtrafficA/B/C و routing/NAT/ACL/multicast/MPLS/SRv6 وcleanup/withdrawal/recovery.
- HA/VRRP واقعی دوگره‌ای وTCP/session continuity؛ BFD دوpeer و IPsec certificate/peer/rekey؛ شواهد محدود قبلی به کل محصول تعمیم نمی‌یابد.
- realAPI/browser en/fa وRTL/light/dark، anti-lockout، confirmedcommit/rollback، notifications وtelemetry وinventory/ویزارد رویapplianceواقعی.
- MultiWAN DHCP/PPPoE gatewayhandoff در پرونده ثبت‌شده «notbuilt» است؛ probesمحدود بهdefaultnamespace/defaultVRF هستند. این gapمنبع مستقل از موفقیت failoverfixture است.
- P10 fileownership/global/etc sandbox و release-license followup: pendingfileمالکیت هنوز بی‌پاسخ است. unitفعلیCAP_CHOWN ندارد وProtectSystemstrict/writableparentقراردادrendererها نیازمند حل معماری/تصمیم امنیتی است. نصبP10 قبلاً اثبات شده؛ ازاین mismatchنباید نتیجه نصب‌نشدن یا شکست runtimeمشاهده‌شده گرفت. مجوزrelease ازمتن تاریخی قطعیresolved اعلام نمی‌شود.

این فهرست گروه‌های شواهد باز است، نه شمارش تازهWBS یا ادعای ممیزی کامل تمام۲۰۵تسک. تعداد ریزموارد پذیرش/زمان آن‌ها بدون ماتریس تازه تک‌به‌تک قطعی نیست. دامنه‌های عمداًunsupported مثل HAED/IPsec-SA/ACLsync را بدون تغییر مصوبscope جزوکار تعهدشده جدید حساب نمی‌کنیم.

## اجرای فعلی و ترتیب بعدی

هیچ توسعه‌دهنده فعال برای باقی‌مانده‌ها در همین گفتگو تأیید نشده؛ عامل ریشه فقط گزارش می‌دهد. فهرست جهانیعامل‌های گفتگوهای دیگرunverifiable است؛ boardrunning یافرایندcodexبه‌تنهایی evidenceworkerفعال نیست. سرویس دائمی supervisor تأیید نشده. نصب/service/configuration درادغام قبلی به‌روزرسانی نشد.

ترتیب پیشنهادی:1.اصلاحاتRAunion وsourceclosure؛2.تشخیص productPPPoE وmgmtdP12؛3.safeinstaller/pins/provisionTD19؛4.حل تصمیمP10مالکیت/parentwrite وreleasefollowup؛5.پذیرش واقعی هدف‌های اختصاصی ودوگره‌ای وbrowser؛6.استقرار نسخه جدید وثبتشواهد. handoversharedVPPهنوزpending؛ این گزارش هیچhostchange یاrestart انجام نمی‌دهد.

گزارش روی شاخهcodex/remaining-work-report-20261007 منتشر می‌شود؛ این شاخه گزارش است و بهmainادغام‌شده ادعا نمی‌شود. Snapshot۷ردیف وکارهای بیرون آن باید پیش از ادعای«پایان محصول» بهreceiptهایactualPASS/sourceclosure تبدیل شوند.
