# PENDING: 211-vpp-startup-tuning

- raised: 2026-10-20 by the manager session of task machine-211-vpp-tuning (branch `claude/machine-110-211-traffic-8c1edc`, PR #251)
- decision: **empty until the product owner confirms, in chat, the ≈9 s data-plane outage of one VPP restart on 172.30.110.211; the manager then appends the confirmation and the apply result to LOG.md D-256 and runs the apply**
- parked tasks: none — only the apply step waits. The diagnosis, the runtime remap and the renderer warning (PR #251) do not depend on it.

## Context
- Host 172.30.110.211 (i7-12700T hybrid, 3× Intel X710 quad 10G, 5× I226-V) runs VPP 26.06 with the start-up file applied on 2026-10-14 (sha256 `889f9a6fea6c2ec0e29088d4a816441bfe8c349f6913c64e06e3e05eff0ccc32`): 9 workers on cpus 2-9,17 plus main core 16 = 10 VPP threads, but `num-tx-queues 8` on every port. Threads 8 and 9 therefore share TX queues 0 and 1 with main and `vpp_wk_0`. On the `0000:02:00.x` X710 (NVM 9.56) a TX queue used by two threads trips the i40e Malicious Driver Detection, the hardware disables the queue for good, and VPP dropped **23 % of all forwarded packets** (5 MDD events logged 2026-10-19/20, one reproduced on purpose). Full evidence: `docs/status/tasks/machine-211-vpp-tuning-wip.md` (PR #251).
- A runtime-only remap (`set interface rx-placement` / `set interface tx-queue`) has been live since 2026-10-20 05:30 and removed the loss (0 `tx-error` growth on the six test ports). It does not survive a VPP restart, and the dead queues stay dead until one.
- The permanent fix is a new `dataplane` document, `docs/status/tasks/machine-211-vpp-tuning-dataplane.json` (PR #251): 7 workers on the P-cores 2,4,6,8,10,12,14 (one per physical core, no hyper-thread pairs, no E-core worker), main core 16, `rxQueues 7` / `txQueues 8` on the twelve X710 ports (8 TX queues = 8 threads, nothing shared), the five I226-V ports unchanged at 4/4 (hardware maximum), buffers and hugepages unchanged. Rendering sha256 **`92985d0c7065ede7dfc417db011c891b06f634ff1c126a25f1493b0ab47191b6`**, reproduced on 2026-10-20 with the generator of origin/main (5593e5ab2) from the live file as `--current`; the installed product generator renders the same input the same way (the generator change on PR #251 only adds a warning).
- Installing it restarts VPP: **≈9 s with no forwarding on every data port**, plus the tool's 10 s settle and 60 s health window during which the box must not be reconfigured. The 2026-10-14 apply on this host took 24 s from install to "VPP restarted" including the API wait, and committed after its 60 s window. Decision policy always-ask #6 (the owner's hands and test rig) and #7 (restarting the VPP process).
- Host 211 is a product appliance (TD-17): there is no `/root/ngfw` checkout on it. The apply runs as `/usr/lib/ngfw/apply-startup.sh --mode product` with the installed binaries in `/usr/lib/ngfw/bin`, gated by `--approve-rendering <sha256>`; the lab-mode gate `--i-have-product-owner-approval PENDING-…` cannot be evaluated there. This file and the LOG.md D-row are the owner-approval record that D-088 requires, and the sha256 named in the D-row is exactly the sum that `--approve-rendering` must repeat.

## Options
| # | Option | Cost now | Reversal cost | Risk |
|---|---|---|---|---|
| 1 | **Apply the rendering** (manager, `apply-startup.sh --mode product`, backup + dead-man + automatic rollback), then set `/etc/ngfw/dataplane-performance.json` to `{"cores":[16,2,4,6,8,10,12,14]}` and restart `ngfw-dataplane-performance.service` | one ≈9 s data-plane outage + 70 s health window; ~0.5 h manager | the tool's `backup.conf` reinstalled through the same tool: one more ≈9 s outage | low: same tool, generator and host as the 2026-10-14 apply; a failed health check rolls back on its own |
| 2 | Keep only the runtime remap | 0 | — | the next VPP restart (package upgrade, crash, reboot) brings the 23 % loss back; the four dead X710 queues stay unusable until then |
| 3 | Option 1 at a maintenance moment the owner names | as 1, later | as 1 | as 1 |

## Recommendation
Option 1, at a moment the owner names: the owner's TRex generator is the only traffic on the box, and the test can resume right after the 60 s window.

## What continues meanwhile
Everything: PR #251 (renderer warning + diagnosis), the runtime remap stays live, no board row is parked.

## Procedure and checks (the manager records the results in `docs/status/tasks/machine-211-vpp-tuning-wip.md` and LOG.md D-256)
1. Dry run on the host: `/usr/lib/ngfw/apply-startup.sh --mode product --doc /root/machine-211-vpp-tuning-dataplane.json` must print live sum `889f9a6f…` and rendering sum `92985d0c…` (the document was copied to the host on 2026-10-20, sha256 `2699c5d1fa575e77e48d78ccf6dcd3fae85ccd57098127bcb6cee130fb00ffd3`).
2. Apply: the same command with `--apply --expect-sha256 889f9a6fea6c2ec0e29088d4a816441bfe8c349f6913c64e06e3e05eff0ccc32 --expect-new-sha256 92985d0c7065ede7dfc417db011c891b06f634ff1c126a25f1493b0ab47191b6 --approve-rendering 92985d0c7065ede7dfc417db011c891b06f634ff1c126a25f1493b0ab47191b6`; follow `journalctl -fu ngfw-startup-apply-<stamp>` to `COMMITTED`.
3. `vppctl show threads`: main on 16, workers on 2,4,6,8,10,12,14.
4. `vppctl show hardware-interfaces enp2s0f0np0`: every TX queue `shared no`.
5. `vppctl show log | grep -i malicious`: no event after the restart.
6. Under the owner's TRex test, `vppctl show interface` `tx-error` does not grow on `enp2s0f0np0/f1`, `f2/f3`, `enp3s0f0np0/f1`.

## خلاصهٔ فارسی
- **مشکل:** دستگاه ۱۱۰.۲۱۱ با فایل راه‌اندازی فعلی VPP (۹ worker روی هسته‌های ۲ تا ۹ و ۱۷ + هستهٔ اصلی ۱۶ = ۱۰ نخ) فقط ۸ صف ارسال (TX) روی هر پورت دارد؛ دو نخ یک صف را مشترک می‌کنند و فرم‌ور NVM 9.56 کارت X710 آن صف را به‌عنوان «درایور مخرب» (MDD) برای همیشه خاموش می‌کند. نتیجه: ۲۳٪ از همهٔ بسته‌ها دور ریخته می‌شد.
- **راه‌حل موقت:** از صبح ۲۰ اکتبر با دستورهای زمان اجرا (rx-placement / tx-queue) اتلاف به صفر رسیده است، ولی با اولین ری‌استارت VPP از بین می‌رود.
- **راه‌حل دائمی:** فایل راه‌اندازی جدید با ۷ worker روی هسته‌های فیزیکی P-core (۲، ۴، ۶، ۸، ۱۰، ۱۲، ۱۴)، هستهٔ اصلی ۱۶ و ۷ صف دریافت / ۸ صف ارسال روی پورت‌های X710؛ هیچ صفی مشترک نمی‌ماند. sha256 رندر: `92985d0c…`.
- **هزینه:** نصب این فایل VPP را یک بار ری‌استارت می‌کند: **حدود ۹ ثانیه هیچ ترافیکی عبور نمی‌کند** و تا حدود ۷۰ ثانیه بعد از آن ابزار سلامت دستگاه را می‌پاید. پشتیبان گرفته می‌شود و در صورت خرابی، بازگشت خودکار است.
- **پیشنهاد:** گزینهٔ ۱ (اعمال دائمی) در زمانی که شما تعیین می‌کنید. این سند و ردیف D-256 در LOG.md سابقهٔ تأیید شما طبق D-088 است؛ خود اعمال روی دستگاه با `--approve-rendering 92985d0c…` انجام می‌شود، چون ۲۱۱ یک appliance محصولی است و مخزن کد روی آن نیست.
- **تأیید شما:** یک جملهٔ «تأیید می‌کنم، وقفهٔ ۹ ثانیه‌ای قابل قبول است» در چت کافی است (و اگر زمان خاصی می‌خواهید، آن را بگویید).
