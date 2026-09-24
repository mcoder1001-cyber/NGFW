# PENDING: vpp-host-hardening

- raised: 2026-09-24 14:40 by the manager (ngfw-46), from the read-only VPP audit (workflow wf_7f25cdf3, 4 investigators + 4 adversarial verifiers)
- decision: **<empty until the product owner fills it>**
- parked tasks: LAB-vpp-per-slot (option F, D-125). Everything else keeps running on the current VPP. Items A–C below change `/etc/vpp`, `vpp.service`, the host or the VM, so they need you (decision-policy always-ask #6/#7).

## Context (verified facts)
**Re-checked 2026-09-24 (D-125):** vpp.service up since 13:03:29, NRestarts=0, no coredumps, nr_hugepages=555, rmem_max=4194304, no hugepage cmdline args.

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
3. **Configuration.** `startup.conf` is the upstream reference file with no tuning (plus dpdk `no-pci` and the three plugins). It is valid, but tight for a VPP shared by 12 slots:
   - Main heap is the 1 GiB default. It ratchets up under churn: 281 → 437 MB with nothing left configured.
   - The API trace ring is 256K messages and lives in that heap.
   - There are no workers, and the main thread is not pinned to a chosen core; it runs wherever it started, shared with 12 slots of CI load.
   - linux_nl asks for a 128 MB netlink buffer, but `rmem_max` caps it at 4 MB.
   - `Restart=always` turns a clean EAL failure into a restart loop (the 12:52 burst of 5 starts).
4. **Host/VM.** The VMware balloon holds 1 GiB, and first-touch of fresh memory runs at a few MiB/s.
   - Because of that, `systemd-sysctl` timed out at boot and left **555** hugepages instead of 1024.
   - Each af_packet interface allocates a ~76 MiB zeroed kernel ring **on VPP's only thread**. Result: API/CLI stalls of 40 s to 5.5 min (13:43–13:49, 14:10:17–14:10:57). This is P08 review I6: it blocks every af_packet test and every slot. **Confirmed at 15:10 by a dedicated diagnosis:** the cause is ESXi reclaiming the VM's memory (balloon at its 1 GiB target, 81 GB free inside the guest). The same ring setup outside VPP took 11.6 s in a slow period and 10–19 ms minutes later, and a plain 64 MiB write ran at 4–6 MiB/s. Even idle, API ping spikes reach ~300 ms from hypervisor jitter. No startup.conf change fixes this; **option A is the real fix**.

## Options
| # | Option | Cost now | Reversal | Risk |
|---|---|---|---|---|
| A | **VM memory**: reserve all guest memory in vSphere (balloon → 0), or shrink the VM to what ESXi can back | your vSphere click | trivial | none; biggest single win for the stalls |
| B | **Hugepages on the kernel command line** (`default_hugepagesz=2M hugepagesz=2M hugepages=1024`), drop `vm.nr_hugepages` from 80-vpp.conf; add `/etc/sysctl.d/60-vrx-netlink.conf` with `net.core.rmem_max=268435456` / `wmem_max` | 0.5 h + 1 reboot | 0.5 h | low |
| C | **startup.conf tuning** (one VPP restart, applied by the manager through the generator under `flock /run/lock/vrx-vpp.lock`): `memory { main-heap-size 4G }`, `api-trace { on nitems 32768 }`, `cpu { main-core 1 }` (move the main thread off the CI cores), `statseg { size 128M }`, `buffers { buffers-per-numa 32768 }`; unit drop-in `Restart=on-failure`, `StartLimitIntervalSec=300`; `ExecStopPost` moves `/tmp/api_post_mortem.*` to `/var/lib/vpp-crash/`; install the already-built `vpp-dbg` deb | 1 h + 1 VPP restart | 0.5 h | low: none of these changes behaviour, only headroom and diagnostics |
| D | *Moved to `PENDING-vpp-c-track.md` (D-125)*: the VPP patch build for the upstream crash bugs (6 crashes from 3 bugs) is decided there, so A–C can be answered alone | — | — | — |
| E | Do nothing now, move to **26.10** when it is released (late Oct) | 0 now | — | the crash bugs are unchanged in 26.10 |
| F | **Per-slot VPP for tests** (row LAB-vpp-per-slot): each test slot gets its own small VPP (dpdk off, ~512M heap, own sockets); the shared VPP stays for tools/app. Needs A+B first: about 12×(heap+buffers), so hugepages above 555 | 10 h (the row) | 1 h | low once A+B are in; removes the cross-slot interference on the shared VPP |

## Recommendation
**A + B + C now; F (LAB-vpp-per-slot) once A + B are in; E as the next baseline. The patch question (old option D) is now `PENDING-vpp-c-track.md`.**
- A and C remove the stalls and give crash forensics.
- The upstream crash bugs stay contained by the agent-side guards already built: TD-3 for V19, TD-5 for V24, det44/gtpu never driven into the failing paths, IGMP/VRRP host tests opt-in (V22), CI slot 12 reserved (D-087).

## What continues meanwhile (no restart needed; the manager does these now, D-108)
- The test rig and the agent create af_packet interfaces with **small TX rings**: 2048-byte frames and 256 per block instead of 66 KiB × 1024. That is ~0.5 MiB instead of ~66 MiB per interface, which removes most of the stall.
- A read-only liveness probe (`timeout 5 vppctl show version`) in the manager's cycle.
- Blind classify-unbind sweeps are replaced by recorded-binding unbinds (tech-debt).
- `docs/lab/host-vrx-a.md` is corrected to today's facts: 32 vCPU / 94 GB / 2 vNUMA, 555 hugepages, coredump installed.

## خلاصهٔ فارسی
- **کرش:** VPP در این build **۱۰ بار** کرش کرده است (نه ۴ بار)، بین عصر ۲۳ سپتامبر و ۰۷:۲۷ امروز. از ری‌استارت ساعت ۱۳:۰۳ کرشی نبوده است.
  - ۶ کرش باگ خود VPP است: det44، gtpu و IGMP. در نسخهٔ 26.10 هم اصلاح نشده‌اند.
  - ۲ کرش از استفادهٔ نادرست تست‌های ما بود و مهار شده است.
  - ۱ کرش کمبود حافظهٔ heap بود و علت ۱ کرش معلوم نیست.
- **سورس:** درست است. `v26.06` همان آخرین commit شاخهٔ `stable/2606` است و بعد از انتشار هیچ اصلاحی به آن اضافه نشده.
- **پیکربندی:** معتبر است ولی تنظیم نشده (پیش‌فرض‌ها). برای VPP مشترک بین ۱۲ اسلات، حافظه و هستهٔ پردازشی آن کم است.
- **مشکل اصلی امروز از VM است:** balloon در VMware یک گیگابایت حافظه را گرفته، hugepageها ۵۵۵ به‌جای ۱۰۲۴ است، و ساخت هر اینترفیس af_packet چند ده ثانیه تا چند دقیقه VPP را قفل می‌کند.
- **بررسی دوباره (۲۰۲۶-۰۹-۲۴):** VPP از ساعت ۱۳:۰۳:۲۹ بدون ری‌استارت بالاست و کرش یا core dump تازه‌ای نبوده است. hugepageها هنوز ۵۵۵ است.
- **پیشنهاد:** گزینه‌های A، B و C همین حالا (رزرو حافظه در vSphere، hugepage روی خط فرمان کرنل، تنظیم `startup.conf` با یک ری‌استارت). گزینهٔ جدید F بعد از A و B: یک VPP جدا برای هر اسلات تست (ردیف LAB-vpp-per-slot، که تا آن موقع متوقف است). مهاجرت به 26.10 وقتی منتشر شد.
- **پچ VPP (گزینهٔ D قبلی):** به پروندهٔ جداگانهٔ `PENDING-vpp-c-track.md` منتقل شد.
- **تصمیم لازم است:** لطفاً در خط `decision:` بنویسید یا در چت بگویید.
