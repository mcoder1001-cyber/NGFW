# Hardware task status — 2026-10-10 09:33 UTC

بسته‌های اصلاح‌شده آماده‌اند؛ CI کامل قبل و بعد از ادغام PR217 موفق شد.
مالک تعمیر را با پذیرش فرسودگی SSD ماشین172.30.126.37 تأیید کرد.
محیط تعمیر و SSH اضطراری در RAM هر دو دستگاه آماده و آزمایش شده است.
مدیریت و روتینگ برقرار است؛ مرحله انتقال و تشخیص آفلاین .211 آزاد شده است.
هنوز تعمیر فایل‌سیستم یا نصب بسته‌ها اجرا نشده؛ ابتدا .211 و سپس .37 انجام می‌شود.

Fresh main4908716b unchanged; no openPR; main quick38035583209 and both fixtures SUCCESS.
Board212:205merged,7parked,0running/ready/review; no invented hardware WBS row.
No new product merge during recovery; PR217 was the earlier packaging correction.
Verified live chat roles: root manager, two exclusive host operators/testers, one
independent recovery reviewer; external live inventory unverifiable, no persistent runner.
Root published/readback86c0ba96; .211e65e4f86; .37direct-read/held-state687b1d9d;
R7final network/transition-only reviewcb7ed8d7. Current SHAs: each branch readback.
Both bounded256MiB aligned direct reads PASS with stable sample counters/no new storage errors.
.211 historicalCRC1003 and query-associated+3 SCSI counter are preserved, not a health certification.
.37 endurance126%/two remaps persist physically; owner explicitly accepts wear for logical repair.
Protected .37enp12s0/.211enp4s0 addresses/routes unchanged; original22 and RAM2222 authenticated.
.211 heldRAMPTY71683; .37heldRAMPTY30565; no nextroot transition execution claimed yet.
Conditional .211 release: final RAM-only audit helper validation/publication, then ordinary
soft-reboot; require actual RAM PID1/auth/network and all-reference/exclusive offline proof
before e2fsck-n/metadata image. No corrective fsck or normal return released yet.
Scoped private auth/network/boot backups verified; full userdata image absent; metadata/undo pending.
Controller privileged free7.3GB/write-fsync PASS; private backup RAM1.9GB, actual image size pending.
Package hashes/dependencies independently prepared; hugepages0 and explicit NIC-seed opt-in need controlled setup.
Remaining: offline diagnosis/preservation, reviewed repair/normal return on both, guarded install
and exact7/17 physical-row activation plus API/TLS/forwarding/restart/reboot tests. Acceptance NOT RUN.
Read hardware-manager-20261010-wip.md and recovery.md for exact current phase and recovery commands.
