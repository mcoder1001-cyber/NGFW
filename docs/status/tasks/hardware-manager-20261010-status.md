# Hardware task status — 2026-10-10 11:27 UTC

فرسودگی SSD روی172.30.126.37 است؛ مالک تعمیر را تأیید کرده است.
تعمیر فایل‌سیستم .211 و بوت عادی، SSH22، تمام مسیرها و DNS موفق بوده‌اند.
.37 در RAM است؛ نسخهٔ متادیتا و هفت بلوک آسیب‌دیده خارج از دستگاه محفوظ‌اند.
تعمیر هدفمند تأیید شده؛ خطای اجرای اول پیش از هر تغییری متوقف شد و ابزار اصلاح شده است.
نصب دقیق .211 تأیید شده؛ فراخوانی گستردهٔ sysctl قبل از نصب کشف و با گزینهٔ رسمی بسته مهار می‌شود.
هنوز نصب یا اتصال NIC داده اجرا نشده؛ پورت و مسیر مدیریت محفوظ‌اند.

Fresh main7c28b192 after unrelated218; main quick38048002335 running, previous
38044587907 and original hardware38035583209 SUCCESS. Board212=205merged+7parked,
fca789c5 unchanged. Four chat roles running; external inventory unverifiable.
Root prior20c35fc1; latest .211 skip-source065cdcd7 push reported/readback pending,
.37 guarded launcher0f4353bc published/readback reported, R7 source approvals.
Current actual phase/source hashes/next commands in latest WIP; historical detail below.

# Historical hardware task status — 2026-10-10 10:56 UTC

بسته‌های اصلاح‌شده آماده‌اند؛ CI کامل پیش و پس از ادغام PR217 موفق شد.
فرسودگی SSD مربوط به172.30.126.37 است؛ مالک تعمیر را با پذیرش آن تأیید کرد.
خرابی اولیه فایل‌سیستم روی هر دو دستگاه تأیید شد؛ تعمیر .211 تمام شده است.
بررسی کامل آفلاین بعد از تعمیر .211 بدون خطا گذشت؛ undo و مدارک خارج از دستگاه محفوظ‌اند.
بررسی فایل‌های بوت و ورود52/52 موفق بود؛ .211 به بوت عادی و SSH22 برگشت و فایل‌سیستم clean است.
مقایسهٔ نهایی DNS، تمام آدرس‌ها و مسیرها و لاگ بوت .211 موفق بود؛ آغاز مرحلهٔ آفلاین .37 تأیید شده است.

Reviewed hardware merge4908716b main quick38035583209 and both fixtures SUCCESS.
Later unrelated laboratory PR219 merged;218/220/221/222 open at latest readback.
10:05 remote board snapshot212:205merged,7parked; hourly6096451111 successfully posted.
Verified live roles: manager, two exclusive host operators/testers, one recovery reviewer;
external live inventory unverifiable, no persistent runner claim.
Fresh10:43 observed main ded860762c81e72fb9f760caec4bd98898c8b6a4 after unrelated
test-onlyPR219; its mainCI38044587907SUCCESS. Immutablehardwarepayload remains
reviewed2045ab8; no silent installation of unmerged218/221product changes.
Root remote5e29d552; .2113d1e1a8f; .3724a5859d; R77ca7c4ed before next updates.
.211 ordinary RAM transition/heldPTY71683/fresh2222 auth PASS; full offline audit
267processes/93FDs/nsfs0/races0/failures0/finalguard0; six network sections identical.
Native e2image PASS986808320B apparent/10096640B sparse; offhost gzip1727953B
SHA55ede2b7, full decompressed SHA b324a9c0 independently verified; seven4096B raw
supplements saved0600, all-zero/ad7facb2. No complete user-data backup claimed.
Affected journal3/cache/config known; scoped private auth/network/boot backups verified.
R7 actual targeted interactive fixes_only,nodiscard APPROVE; verified1GiB undo cap.
Correction exit1 then full-f-n0 all5passes; undo749568B0600 source/offhost SHA92df3461
verified/fsynced, complete private transcript3270B/ledger20099B preserved.
Emptyregular259595/259600 and directory259603 preserved in lost+found;
postaudit260processes/93FDs/nsfs0/races0/failures0/finalguard0; kernel equal/ioerr18stable.
Selected originalroot/EFI readonly boot/auth integrity52/52True and ordinaryunmount0;
matchingRAMshutdown/manager/finalguard/sync PASS9569441f, R7 normal-returnAPPROVE.
ActualsingleforceSSH0/reconnectfourthprobeSSH0/newboot3a609803/rootclean;
managerindependentfresh22/protectedenp4s0PCI04igcgroup28/fsckrootsuccess0 PASS.
WorkerallL3/DNS/newkernelproofPASSbaf2/e2d563; absentoptionalresolvectl retained,
directDNSfallbackPASS. .37 actualnetdstop applicability3dd7 R7APPROVE;
ownreviewed bdb2sourcepublishedf1f9/imports0 andfresh211checkPASS. Ordinary37RAM
transition/readonlyphase fullyreleased; actualtransition/preservation notyetobserved.
Both bounded256MiB aligned direct reads PASS, stable counters/no new read-storage errors;
.211 historicalCRC1003/query-associated counter increments retained; .37wear126%/2remaps.
Protected .37enp12s0/.211enp4s0 exact network preserved; no dataPCI binding/install yet.
Package hash/direct-dependency prep done; hugepages0 and NIC-seed opt-in require setup.
Remaining: .37 actualoffline repair/clean check/normal return, install
and exact7/17 physical-row activation, API/TLS/forwarding/restart/reboot actual tests.
Read hardware-manager-20261010-wip.md and recovery.md for current gates/next commands.
