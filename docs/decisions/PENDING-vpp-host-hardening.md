# PENDING: vpp-host-hardening

- raised: 2026-09-24 14:40 by the manager (ngfw-46), from the read-only VPP audit (workflow wf_7f25cdf3, 4 investigators + 4 adversarial verifiers)
- decision: **partially superseded by live host state verified 2026-10-03**: 4096 × 2 MiB hugepages are configured and all allocated; netlink rmem/wmem limits are 256 MiB. See `docs/lab/host-ngfw-a.md`. VMware balloon state is not guest-visible and remains unverified. Do not treat the 2026-09-24 measurements below as current.
- parked tasks: none for option F — **LAB-vpp-per-slot implemented 2026-10-06 without any host change** (branch claude/vpp-per-slot-20261006, `docs/status/tasks/claude-vpp-per-slot-wip.md`): per-slot VPPs run on 4k pages (no hugepages), no DPDK, MemoryMax 1G, capped at 2 developer instances (ceiling 4 until option A) + the CI slot, refused under an 8 GiB MemAvailable floor. The owner approval of 2026-10-06 to change /etc/vpp and vpp.service for this task was **not used**: /etc/vpp, vpp.service and the shared VPP process are unchanged (MainPID 1014, NRestarts 0 before/after). (Previously: LAB-vpp-per-slot parked here, option F, D-125.) Everything else keeps running on the current VPP. Items A–C below change `/etc/vpp`, `vpp.service`, the host or the VM, so they need you (decision-policy always-ask #6/#7).

## Context (verified facts)
**Historical check 2026-09-24 (D-125; superseded for memory and netlink settings):** vpp.service up since 13:03:29, NRestarts=0, no coredumps, nr_hugepages=555, rmem_max=4194304, no hugepage cmdline args.

**Fresh guest check 2026-10-03 17:02 +0330:** Ubuntu 26.04.1, kernel 7.0.0-31, 32 vCPU, 62.7 GiB RAM (43.8 GiB available), one NUMA node. `/etc/sysctl.d/80-vpp.conf` and live `vm.nr_hugepages` both request 4096; `/proc/meminfo` confirms `HugePages_Total=4096`, `HugePages_Free=4045`, page size 2 MiB (about 8 GiB reserved). `vm.max_map_count=1048576`. `net.core.rmem_max` and `wmem_max` are 268435456. `/proc/cmdline` has no hugepage parameters, so the 4096 pages came from the live sysctl configuration. `vpp.service` was active since 12:48:02 with NRestarts=0 and about 626 MiB RSS. These are guest-side observations; VMware balloon/reservation state was not available to verify, and no packet/af_packet test was run during this check.

1. **Crashes.** This build crashed **10 times**, not 4: 9 SIGSEGV and 1 SIGABRT, 2026-09-23 15:52 → 2026-09-24 07:27. There has been none since the 13:03 restart.
   - 6 are **upstream VPP bugs** reached through normal API use: det44 ×2 (V9), gtpu ×3 (V8), IGMP `ip4_options` trace ×1 (V22b). None is fixed on master or stable/2610.
   - 2 are **our test misuse hitting upstream weak spots**: `fib_table_flush` (two clients on one table, D-087) and classify V19 (D-095, fixed agent-side by TD-3).
   - 1 is an **out-of-memory abort** on the 1 GiB main heap, from an ND-proxy loop on a loopback (V12).
   - 1 is **unknown**: 07:27, PC 0x0. V24 is suspected but not proven.

   No crash left a core. `core_pattern=core` wrote to `/run/vpp` (tmpfs), and the 09:47 reboot erased it. systemd-coredump has been installed since 12:40, so the next crash will be captured.
2. **Source.** `/root/vpp` is a clean tree at `c3200b88` = tag `v26.06`, which **is** today's head of `stable/2606`.
   - stable/2606 has 0 commits after the release and no point releases. The installed debs are byte-identical to this build.
   - 26.10 is at rc1 (2026-09-23); the final release is expected around mid/late October.
   - Master has about 14 fixes in areas we use that are not in 26.06, e.g. `2e2179223 vnet: fix reuse of deleted interfaces` and IGMPv3 parsing. None touches our crash sites.
3. **Configuration (2026-09-24 audit; recheck before action).** `startup.conf` was the upstream reference file with no tuning (plus dpdk `no-pci` and the three plugins). The audit considered it tight for a VPP shared by 12 slots:
   - Main heap is the 1 GiB default. It ratchets up under churn: 281 → 437 MB with nothing left configured.
   - The API trace ring is 256K messages and lives in that heap.
   - There are no workers, and the main thread is not pinned to a chosen core; it runs wherever it started, shared with 12 slots of CI load.
   - At that time linux_nl asked for a 128 MB netlink buffer, but `rmem_max` capped it at 4 MB. Current live rmem/wmem limits are 256 MiB (verified 2026-10-03).
   - `Restart=always` turns a clean EAL failure into a restart loop (the 12:52 burst of 5 starts).
4. **Host/VM (historical audit; current hypervisor state unknown).** The 2026-09-24 audit observed a VMware balloon target of 1 GiB, and first-touch of fresh memory ran at a few MiB/s. The VM now reports 62.7 GiB total and 43.8 GiB available from inside the guest; balloon/reservation state still needs a hypervisor-side check.
   - Historical observation: `systemd-sysctl` timed out at boot and left **555** hugepages instead of the then-configured 1024. This is resolved in the current guest snapshot: 4096 pages are configured and allocated (2026-10-03); the cause or mechanism of the improvement is not established here.
   - Each af_packet interface allocates a ~76 MiB zeroed kernel ring **on VPP's only thread**. Result: API/CLI stalls of 40 s to 5.5 min (13:43–13:49, 14:10:17–14:10:57). This is P08 review I6: it blocks every af_packet test and every slot. **Confirmed at 15:10 by a dedicated diagnosis:** the cause is ESXi reclaiming the VM's memory (balloon at its 1 GiB target, 81 GB free inside the guest). The same ring setup outside VPP took 11.6 s in a slow period and 10–19 ms minutes later, and a plain 64 MiB write ran at 4–6 MiB/s. Even idle, API ping spikes reach ~300 ms from hypervisor jitter. No startup.conf change fixes this; **option A is the real fix**.

## Options
| # | Option | Cost now | Reversal | Risk |
|---|---|---|---|---|
| A | **VM memory**: reserve all guest memory in vSphere (balloon → 0), or shrink the VM to what ESXi can back | your vSphere click | trivial | none; biggest single win for the stalls |
| B | **Historical proposal; current state already meets/exceeds it**: live sysctl allocation is 4096 × 2 MiB and netlink rmem/wmem are 256 MiB. No kernel command-line hugepage arguments are present. | — | — | — |
| C | **startup.conf tuning** (one VPP restart, applied by the manager through the generator under `flock /run/lock/ngfw-vpp.lock`): `memory { main-heap-size 4G }`, `api-trace { on nitems 32768 }`, `cpu { main-core 1 }` (move the main thread off the CI cores), `statseg { size 128M }`, `buffers { buffers-per-numa 32768 }`; unit drop-in `Restart=on-failure`, `StartLimitIntervalSec=300`; `ExecStopPost` moves `/tmp/api_post_mortem.*` to `/var/lib/vpp-crash/`; install the already-built `vpp-dbg` deb | 1 h + 1 VPP restart | 0.5 h | low: none of these changes behaviour, only headroom and diagnostics |
| D | *Moved to `PENDING-vpp-c-track.md` (D-125)*: the VPP patch build for the upstream crash bugs (6 crashes from 3 bugs) is decided there, so A–C can be answered alone | — | — | — |
| E | Do nothing now, move to **26.10** when it is released (late Oct) | 0 now | — | the crash bugs are unchanged in 26.10 |
| F | **Per-slot VPP for tests** (row LAB-vpp-per-slot): each test slot gets its own small VPP (dpdk off, ~512M heap, own sockets); the shared VPP stays for tools/app. The old A+B memory prerequisites have changed: B is verified complete; A's VMware balloon/reservation state remains unverified. Reassess capacity and shared-host safety before starting 12 instances. | 10 h (the row) | 1 h | low once prerequisites are verified; removes cross-slot interference on the shared VPP |

## Recommendation
**Historical recommendation (2026-09-24): A + B + C, then F.** As of 2026-10-03, B is satisfied by live guest configuration (4096 hugepages and 256 MiB netlink buffers). Re-check A with the hypervisor owner; C remains an unimplemented proposal and should be evaluated against current test performance before changing VPP. The patch question (old option D) is now `PENDING-vpp-c-track.md`.
- The 2026-09-24 audit concluded that A and C would remove the stalls and improve crash forensics. Current impact of A remains unverified at the hypervisor, and C has not been applied; retest before relying on that conclusion.
- The upstream crash bugs stay contained by the agent-side guards already built: TD-3 for V19, TD-5 for V24, det44/gtpu never driven into the failing paths, IGMP/VRRP host tests opt-in (V22), CI slot 12 reserved (D-087).

## What continues meanwhile (no restart needed; the manager does these now, D-108)
- The test rig and the agent create af_packet interfaces with **small TX rings**: 2048-byte frames and 256 per block instead of 66 KiB × 1024. That is ~0.5 MiB instead of ~66 MiB per interface, which removes most of the ring allocation and should avoid most of the old stall; current latency still needs a host test.
- A read-only liveness probe (`timeout 5 vppctl show version`) in the manager's cycle.
- Blind classify-unbind sweeps are replaced by recorded-binding unbinds (tech-debt).
- `docs/lab/host-ngfw-a.md` records the 2026-10-03 guest facts: 32 vCPU / 62.7 GiB / 1 NUMA node, 4096 hugepages, coredump installed.

## خلاصهٔ فارسی
- **گزارش تاریخی کرش (۲۰۲۶-۰۹-۲۴):** VPP در آن build **۱۰ بار** کرش کرده است (نه ۴ بار)، بین عصر ۲۳ سپتامبر و ۰۷:۲۷ روز ۲۴ سپتامبر. در بازبینی همان روز، از ری‌استارت ساعت ۱۳:۰۳ کرشی ثبت نشده بود؛ این گزاره وضعیت فعلی را توصیف نمی‌کند.
  - ۶ کرش باگ خود VPP است: det44، gtpu و IGMP. در نسخهٔ 26.10 هم اصلاح نشده‌اند.
  - ۲ کرش از استفادهٔ نادرست تست‌های ما بود و مهار شده است.
  - ۱ کرش کمبود حافظهٔ heap بود و علت ۱ کرش معلوم نیست.
- **سورس:** درست است. `v26.06` همان آخرین commit شاخهٔ `stable/2606` است و بعد از انتشار هیچ اصلاحی به آن اضافه نشده.
- **پیکربندی:** معتبر است ولی تنظیم نشده (پیش‌فرض‌ها). برای VPP مشترک بین ۱۲ اسلات، حافظه و هستهٔ پردازشی آن کم است.
- **وضعیت تازهٔ مهمان (۲۰۲۶-۱۰-۰۳):** ۴۰۹۶ hugepage دو مگابایتی تنظیم و تخصیص یافته (حدود ۸ گیگابایت) و بافرهای netlink برابر ۲۵۶ MiB است؛ وضعیت VMware balloon از داخل سیستم‌عامل قابل تأیید نبود. VPP فعال و بدون ری‌استارت از ساعت ۱۲:۴۸ است. در این بررسی تست بسته اجرا نشد.
- گزارش قدیمی ۵۵۵ hugepage و محدودیت ۴ MiB مربوط به ۲۴ سپتامبر است و با تنظیم فعلی هم‌خوانی ندارد. کندی قدیمی ساخت af_packet هنوز باید با اجرای تست روی وضعیت فعلی دوباره سنجیده شود.
- پیشنهاد A+B+C مربوط به ۲۴ سپتامبر بود: B اکنون انجام‌شده است؛ A باید در سمت VMware بررسی شود و C را پس از سنجش دوبارهٔ کارایی ارزیابی کنید. ساخت VPP جدا برای هر اسلات (F) هنوز نیازمند بررسی ظرفیت میزبان و تداخل تست‌هاست.
- **پچ VPP (گزینهٔ D قبلی):** به پروندهٔ جداگانهٔ `PENDING-vpp-c-track.md` منتقل شد.
- **تصمیم لازم است:** لطفاً در خط `decision:` بنویسید یا در چت بگویید.
