# Host 172.30.110.211 — the 64-byte packet ceiling of the X710 ports (WIP)

Branch `claude/zen-knuth-454f68` (base origin/main 2a6d1611c), PR #257 (first checkpoint c4dc11982, pushed). Envelope: `machine-211-small-packet-limit-envelope.md`.
Board row `machine-211-small-packet-limit`. Predecessors: `machine-211-vpp-tuning-wip.md` (PR #251, the shared-TX-queue /
MDD diagnosis) and `apply-211-vpp-startup-20261020-wip.md` (D-256, the start-up file live since 2026-10-20 06:39 UTC).

Status 2026-10-20 ≈08:00 UTC: **measured and explained per card; no product change made** (verdict below); waiting for the
owner's experiments (§5) to tell a per-card hardware ceiling from a host-side (IOMMU) ceiling. Nothing on the appliance was
changed by this task: every command was `vppctl show …`, `lspci`, `/sys`, `dmesg`; no counter was cleared.

## 1. Finding in one paragraph

Under the owner's TRex test (64-byte UDP, three L2 xconnect pairs) the **NICs drop, VPP does not**. The 02:00.x card
(four ports in use, PCIe x8, NVM 9.56) accepts exactly ≈2.6 Mpps of receive traffic **per port on all four ports, whatever
the offered rate** (2.68 Mpps on f0/f1, 3.31 Mpps on f2/f3 → the same 2.59–2.60 Mpps delivered), and transmits exactly
≈2.10 Mpps per port; the rest is `rx_missed` (3 % / 21 %) and `dpdk tx failure` (19 %). The 03:00.x card (two ports in use,
PCIe x4, NVM 6.80) delivers 3.65 Mpps per port of 3.92 offered (6.8 % `rx_missed`) and transmits 3.61 (1.3 % TX failure).
So the same X710 silicon does 3.65 Mpps per port on one card and is held at 2.6 on the other: **the ceiling is not per
port, it is per card (per root port) — ≈10.4 Mpps in + 8.4 Mpps out for the 4-port card, ≈7.3 in + 7.2 out, already
lossy, for the 2-port x4 card**. The seven VPP workers are ≈⅓ busy and poll 1.45 packets per `dpdk-input` call; PCIe links
run at nominal width and speed with < 10 % of their bandwidth used. What is left as the limiting element is the DMA path
between the card and memory: either the Fortville's own per-chip descriptor/DMA engine, or the host's VT-d translation,
which is **on** for every NIC (`CONFIG_INTEL_IOMMU_DEFAULT_ON=y`, VFIO type-1 translated domains, all three cards behind the
single `dmar1` unit). None of the dpdk start-up options the renderer does not expose (`no-multi-seg`,
`no-tx-checksum-offload`, `enable-mbuf-fast-free`, larger `num-tx-desc`, `buffers { default data-size 9216 }`) changes
what the card sees on the bus; they only save CPU the box does not need, and two of them cost MTU 9000 or 9.4 GiB of
buffer memory. **No renderer change is justified by these measurements.**

## 2. Measurements (read-only, 2026-10-20 07:38:22–07:39:12 UTC, 50.1 s window)

Method: two full captures 50.1 s apart of `vppctl show interface` (VPP counters), `vppctl show hardware-interfaces <port>`
(i40e xstats: `rx_unicast_packets` = what the MAC received from the wire, `rx_good_packets` = what reached memory,
`rx_missed_errors` = i40e `rx_discards`, `tx_good_packets` = what left the wire) and `vppctl show runtime` (per-thread,
per-node calls and vectors), diffed by `machine-211-small-packet-limit-delta.py` (raw captures
`machine-211-small-packet-limit-sample-{a,b}.txt`, both committed next to this file). VPP MainPID 212946 since 06:39:57
UTC, start-up file sha256 `92985d0c…`, 7 workers on cpus 2,4,…,14, main 16, 7 RX / 8 TX queues per X710 port, 2048
descriptors each, no shared TX queue, no Malicious Driver Detection event since the restart. Only the six ports below carry
traffic; the 01:00.x card and 03:00.2/.3 have **no carrier** (not cabled).

| port (xconnect pair) | offered (`rx_unicast`) | delivered (`rx_good`) | `rx_missed` | miss % | VPP tried to send | `tx_good` | `tx-error` | TX fail % |
|---|---|---|---|---|---|---|---|---|
| enp2s0f0np0 ↔ | 2.676 | 2.588 | 0.088 | 3.3 | 2.596 | 2.094 | 0.501 | 19.3 |
| enp2s0f1np1 | 2.676 | 2.595 | 0.081 | 3.0 | 2.589 | 2.093 | 0.495 | 19.1 |
| enp2s0f2np2 ↔ | 3.307 | 2.600 | 0.707 | 21.4 | 2.600 | 2.101 | 0.499 | 19.2 |
| enp2s0f3np3 | 3.307 | 2.600 | 0.707 | 21.4 | 2.600 | 2.101 | 0.499 | 19.2 |
| enp3s0f0np0 ↔ | 3.918 | 3.654 | 0.265 | 6.8 | 3.654 | 3.606 | 0.048 | 1.3 |
| enp3s0f1np1 | 3.918 | 3.654 | 0.265 | 6.8 | 3.654 | 3.605 | 0.048 | 1.3 |
| **02:00.x card (4 ports)** | **11.966** | **10.383** | 1.583 | 13.2 | 10.385 | **8.390** | 1.994 | 19.2 |
| **03:00.x card (2 ports)** | **7.837** | **7.308** | 0.529 | 6.8 | 7.308 | **7.211** | 0.097 | 1.3 |

All rates in Mpps, 64-byte frames (`rx_size_64_packets` = `rx_unicast_packets` on every port). "VPP tried to send" is
VPP's `tx packets` counter (it equals `rx_good` of the paired port: the L2 xconnect forwards everything it gets);
`tx-error` is VPP's `Tx packet drops (dpdk tx failure)`; `rx_good + rx_missed = rx_unicast` within 0.1 %. `show errors`
lists no other drop reason; the xstats show no `rx_errors`, no `tx_errors`, no pause (`xon`/`xoff`) frames, no
`rx_length_errors`.

Reading the table:

- **The 4-port card is capped as a whole.** f0/f1 and f2/f3 are offered different rates (2.68 vs 3.31 Mpps) and both
  deliver 2.59–2.60 and transmit 2.09–2.10: an equal share of ≈10.4 Mpps in / ≈8.4 Mpps out, not a per-port number.
- **It is not the silicon per port**: the identical X710 on 03:00 passes 3.65 Mpps in / 3.61 out per port.
- **The 2-port x4 card is also at its ceiling**: 6.8 % `rx_missed` at 7.3 Mpps in + 7.2 out (14.5 M packet-DMAs/s) against
  the x8 card's 18.8 M packet-DMAs/s. A pure PCIe-width limit would give 2 : 1; a per-port limit would give the same
  per-port rate on both cards; neither fits. (Both cards, offered 64 B at these rates, use < 10 % of their link bandwidth:
  18.8 M × (64 B + descriptor) ≈ 1.5 GB/s of the x8 link's 7.9 GB/s.)
- **TX fails because the NIC does not drain the rings, not because VPP bursts badly**: VPP hands the 02:00 ports 6.5–7.6
  packets per `rte_eth_tx_burst` (0.28–0.32 M calls/s per port) and the 03:00 ports 5.3 packets per call at a *higher*
  call rate (0.68 M/s per port) without failures, so the doorbell rate is not the limiter either. With 2048 TX descriptors a
  "ring full" (`rte_eth_tx_burst` returns 0, VPP drops at once — `tx_burst_vector_internal` stops retrying when a burst
  sends nothing, `src/plugins/dpdk/device/device.c:147-173`) means the card is behind by ≈1 ms of traffic on every queue, continuously.

### VPP is not the limit (same window, per worker)

| per worker (7 identical) | value |
|---|---|
| vector rate in / out | 2.52 Mpps / 2.13 Mpps |
| `dpdk-input` | 1.74 M calls/s, **1.45 packets per call** (polls 17 ports × queue *q*) |
| `ethernet-input` / `l2-input` / `l2-output` | 6.9 / 15.6 / 15.6 packets per call |
| `<port>-tx` | 7.6 (02:00 ports) / 5.3 (03:00 ports) packets per call |
| clocks per packet (cumulative averages from `show runtime`) | dpdk-input 2.6e2, ethernet-input 7.9e1, l2-input 1.5e1, l2-output 1.3e1, output 9e0, tx 1.7e2 (02:00, includes the failed-burst retries) / 1.1e2 (03:00) |
| busy estimate | ≈ 520 clocks × 2.52 Mpps ≈ 1.3 G clocks/s = **31–35 % of a 3.7–4.2 GHz P-core** |

Raising the per-worker rate is therefore not the problem; the workers are mostly waiting for packets that the cards never
deliver. (`dpdk-input` 260 clocks per packet is the price of 17 polled queues per loop at 1.45 packets per loop, not a
sign of heavy per-packet work.)

### Hardware and driver facts behind the table

| item | value (read 2026-10-20) |
|---|---|
| CPU | i7-12700T, workers on P-cores 2,4,…,14 (`performance` governor), main on E-core 16 |
| PCIe | 01:00.x: CPU root port 00:01.0, Gen3 **x8**; 02:00.x: CPU root port 00:01.1 (the x16 slot bifurcated 8+8), Gen3 **x8**; 03:00.x: CPU root port 00:06.0, Gen3 **x4** (card max x8 → `LnkCap x8`, `LnkSta x4`) — all `8GT/s`, ASPM **disabled** on every link, `ExtTag+ RlxdOrd+`, MaxPayload 256, MaxReadReq 512, `NoSnoop-`, LTR- |
| cards | all `8086:1572` subsystem `8086:0000`; 01/02: fw 9.156 nvm 9.56 api 1.15; 03: fw 6.1 nvm 6.80 api 1.7 |
| DPDK / VPP | DPDK 26.03.0 in VPP 26.06-release+ngfw3; EAL `--in-memory --no-telemetry`, IOVA mode VA, **VFIO, "Using IOMMU type 1"** |
| i40e per port | `rx: queues 7 (max 192), desc 2048 (min 64 max 8160 align 32)`, `tx: queues 8, desc 2048`; `rx burst mode: Vector AVX2 Scattered`; **`tx burst mode: Scalar`**; rx offloads active `ipv4-cksum scatter`; tx offloads active `ipv4-cksum udp-cksum tcp-cksum outer-ipv4-cksum multi-segs`; `max-frame-size 9022` (MTU 9000 on all 17 ports); RSS active on ipv4/ipv6 l3/l4 |
| buffers | `buffers-per-numa 1048576`, data size 2048, element 2496 B (`vlib_buffer_alloc_size`: 128 B rte_mbuf + 256 B vlib_buffer_t + 2048, rounded to 64 and `|= 64`), arena `buffers-numa-0` 1249 × 2 MB pages; hugepages 7680 × 2 MB = 15 GiB reserved, 6354 free; no 1 GB pages |
| IOMMU | `CONFIG_INTEL_IOMMU_DEFAULT_ON=y`, `CONFIG_INTEL_IOMMU_SCALABLE_MODE_DEFAULT_ON=y`, `CONFIG_IOMMU_DEFAULT_DMA_LAZY=y`, no `intel_iommu=`/`iommu=` on the command line; `iommu: Default domain type: Translated`; groups of the X710 functions `DMA-FQ`; **one VT-d unit `dmar1` (cap `d2008c40660462`: SLLPS supports 2 MB and 1 GB second-level pages) serves every PCIe root port** (00:01.0, 00:01.1, 00:06.0, the PCH ports); `dmar0` serves only the iGPU |
| DMA-active buffer set | 17 ports × 7 RX queues × 2048 descriptors = **243 712 buffers (≈600 MB) pinned in RX rings** plus up to 17 × 8 × 2048 in TX rings, spread over the 2.5 GiB arena — every descriptor refill, packet write and TX data read is an IOMMU translation of a different 2 MB page |

## 3. What limits the cards — two candidates, and how to tell them apart

The measurements exclude: VPP CPU (⅓ busy), VPP burst sizes/doorbell rate (the clean card gets more doorbells),
PCIe bandwidth (< 10 %), ASPM/link state (off, nominal), MDD (no events), flow control (no pause frames), per-port
silicon (3.65 Mpps on one card, 2.6 on the other). What remains is the **DMA transaction path**: for every 64-byte packet the
card issues one 64 B write (RX data) + one 32 B write (RX descriptor write-back) on receive, and one descriptor read (shared
by the burst) + one 64 B data read on transmit, each a separate PCIe transaction that must be translated by VT-d and
completed by the root complex.

- **(A) Per-card: the Fortville's descriptor/DMA engine with small bursts.** The X710/XL710 is specified for ≈37 Mpps of
  64-byte forwarding per chip *with 32-packet bursts*; with 1.45-packet RX polls and 7-packet TX bursts the per-transaction
  overhead (partial cache lines, one outstanding fetch per queue, 28 active queues on the 4-port card) can hold the chip far
  below that. Prediction: spreading the three pairs over three cards raises the total; a card with one pair behaves like
  03:00 today (≈7.3 Mpps per direction per card, ≈3.6 per port).
- **(B) Host side: the shared VT-d unit.** All three cards translate through `dmar1`; the DMA-active set (≈600 MB in RX
  rings alone) is far beyond any client IOTLB, so a large share of the ≈33 M packet-DMAs/s both cards perform today ends
  in a page walk on the one unit. Prediction: spreading the pairs over three cards does **not** raise the total much
  (≈17–18 Mpps forwarded stays), the per-card shares just re-balance; shrinking the DMA footprint (1 GB pages for the buffer
  arena, fewer descriptors/queues) raises it; `intel_iommu=off` (noiommu VFIO) would remove it entirely but gives up DMA
  isolation — a security decision, not a tuning knob, and not recommended on the data so far.

Both predict that f0/f1 improve when f2/f3 stop; only the **total** across the box separates them (§5).

## 4. The dpdk start-up options the renderer does not expose — evaluated against the sources

VPP: `/root/vpp/src/plugins/dpdk/device/{init.c,common.c,device.c,node.c}` (26.06); DPDK 26.03:
`build-root/build-vpp-native/external/src-dpdk/drivers/net/intel/{i40e/i40e_rxtx.c,i40e/i40e_rxtx.h,common/tx.h,common/rx.h}`.

How the plugin configures these ports today (`dpdk_device_setup`, common.c:154-233): TX offload wish list =
`OUTER_IPV4_CKSUM | OUTER_UDP_CKSUM | IPV4_CKSUM | TCP_CKSUM | UDP_CKSUM` (unless `no-tx-checksum-offload`) `| MULTI_SEGS`
(unless `no-multi-seg`); RX = `IPV4_CKSUM | SCATTER` (SCATTER tied to multi-seg); `conf.rxmode.mtu` from the 9000-byte
default MTU; `rte_eth_tx_queue_setup(port, q, num-tx-desc, socket, NULL)` → i40e defaults `tx_rs_thresh 32`,
`tx_free_thresh 32` (i40e_ethdev.h:69-73), RX `rx_free_thresh 32`, vector RX rearm 64 (common/rx.h:22). i40e picks the TX
path in `i40e_set_tx_function_flag` (i40e_rxtx.c:3069-3081): `tx_simple_allowed` ⇔ `txq->offloads ⊆ {MBUF_FAST_FREE}`
and `tx_rs_thresh ≥ 32`; `tx_vec_allowed` ⇔ simple and `tx_rs_thresh ≤ 64`; only then `ci_tx_path_select` (common/tx.h:413)
may choose "Vector AVX2/AVX512" (`I40E_TX_VECTOR_OFFLOADS = MBUF_FAST_FREE`, i40e_rxtx.h:109). With VPP's default wish
list `txq->offloads` holds five checksum bits plus multi-segs, so the card logs `i40e_set_tx_function(): Using Scalar`
(VPP log 06:40:10-16 for all 12 ports), i.e. `i40e_xmit_pkts`. RX is already "Vector AVX2 Scattered" (descriptor count is
a power of two, i40e_rx_vec_dev_conf_condition_check_default).

| option (where) | what it changes in this VPP/DPDK | what the card sees on the bus | verdict for 211 |
|---|---|---|---|
| `dpdk { no-multi-seg }` (section level; `dev default { }` / `dev <pci> { }` do not accept it — init.c:1024-1072 vs 1210) | drops `MULTI_SEGS` from TX and `SCATTER` from RX (common.c:171-174) **and caps the frame size at the buffer data size**: `max_supported_frame_size = min(max_rx_pktlen, 2048)` (common.c:225-226) → MTU 9000 can no longer be set on any DPDK port; the agent's MTU apply would fail unless `buffers { default data-size ≥ 9022 }` | nothing for 64-byte frames: one segment per packet either way; identical descriptors | **no** — costs jumbo frames or 9.4 GiB of buffers (below), gains nothing here |
| `dpdk { no-tx-checksum-offload }` | drops the five checksum bits (common.c:166-169); together with `no-multi-seg`, `txq->offloads == 0` → `tx_simple_allowed` → **"Vector AVX2" TX** instead of "Scalar" | **identical descriptor stream**: xconnect traffic never carries `VNET_BUFFER_F_OFFLOAD`, so the scalar path writes one data descriptor per packet, no context descriptor, and VPP never calls `rte_eth_tx_prepare` (device.c:396-400, 441-446). The win is CPU only: the `-tx` node's ≈110–175 clocks per packet (≈10 % of a worker today) would shrink | **not for this problem** — the workers are ⅔ idle; the card, not the tx node, is behind. Legitimate later as a CPU saving for NAT/IPsec loads, where VPP would then compute TCP/UDP checksums of locally built packets in software |
| `dpdk { enable-mbuf-fast-free }` | `MBUF_FAST_FREE` only together with `no-multi-seg` (common.c:188); allowed by the vector path | none | no |
| larger `num-tx-desc` (per device; X710 max 8160, align 32; renderer allows powers of two ≤ 4096) | deeper TX ring | a deeper ring only delays the first drop: the deficit is sustained (card drains 2.1 Mpps of 2.6 offered per port), so after ≈2 ms the ring is full again and drops continue at the same rate; it also enlarges the DMA-active set (worse under hypothesis B) | **no**; if anything smaller rings are the experiment |
| `buffers { default data-size 9216 }` (vlib `buffers` section, buffer.c:960) | buffer element 9664 B (`vlib_buffer_alloc_size`) → 1 048 576 buffers = **9.4 GiB** of the 15 GiB hugepages (fits, but 4× the DMA footprint and TLB pressure) — only needed to keep MTU 9000 under `no-multi-seg` | no change per 64-byte packet, 4× more distinct pages under DMA | **no** |
| fewer `rxQueues` on the X710 ports (already exposed: `dataplane.devices.<pci>.rxQueues`) | e.g. 7 → 2: RX-pinned buffers 244 K → 70 K, larger per-queue bursts (each worker then polls 2 ports' queues instead of 17 × 1), 2 workers per port (≈2 × 7 Mpps of RX capacity per port at today's per-packet cost, still above the card ceiling) | fewer, denser descriptor fetches; smaller translated set | **candidate experiment** if §5 points at B or A; VPP restart (manager step, D-088) |
| `buffers { page-size 1g }` + boot-time `hugepagesz=1G` (Settings field exists for lab slots only; not in the dataplane domain) | the 2.5 GiB arena becomes 3 IOMMU/TLB entries (`dmar1` SLLPS supports 1 GB) | removes almost all translation misses | **the fix if B is confirmed**; a product change (kernel command line / hugepage reservation + a renderer field), to be designed after §5 |
| `interfaces.<if>.rxMode: adaptive` / `interrupt` (runtime, already exposed by the product, reversible) | workers sleep until the card interrupts (`int-supported` on these ports); bursts per poll grow by the wake-up latency | fewer, larger descriptor fetches per queue | optional runtime experiment; adds latency; does not touch the start-up file |

**Renderer verdict:** the options the task named do not reach the measured bottleneck; adding them to the `dataplane`
contract now (schema + proto + generated Go/TS + drift guard, `contract(...)` commit on a `contract/<id>` branch per
`docs/contributing.md`) would ship knobs whose only effect here is a CPU saving the box does not need, plus a footgun
(`no-multi-seg` silently caps the MTU at 2048 − overhead). If §5 confirms hypothesis B, the field that matters is a
buffer page size (`buffers { page-size 1g }`) with its hugepage prerequisite — a different, better-targeted contract
change; if it confirms A, the answer is cabling (one pair per card), not a field.

### Cross-check with the manager session's reading (2026-10-20, received after the first checkpoint)

The owner's session reported, from short (0.2 s) samples, a "near-constant 15–30 % TX failure on the 02:00.x ports at
every offered rate from 0.3 to 4 Mpps per port". The long counters do not support a rate-independent ratio: since the
restart (06:39:57) until 07:32 UTC enp2s0f0np0 had 333.4 M `tx-error` of 4 021 M `tx packets` = **8.3 % averaged over
all phases**, against **19.3 % in the 07:38 window at 2.6 Mpps delivered**; and the apply session measured **zero
`tx-error` on all six ports over 93 s at 0.86–1.0 Mpps per direction** right after the restart (same workers, same
queues: `apply-211-vpp-startup-20261020-wip.md`, check 8). The failure ratio therefore grows with the rate and vanishes
below the card's TX ceiling, which is what a per-card ceiling predicts (ratio = 1 − TX ceiling ÷ RX delivered: 1 − 2.1 ÷
2.6 = 19 %). The same session's earlier zero-loss window under the runtime remap (≈1 Mpps, 6–7 TX queues, cpus 3–9) is the
same low-rate regime, not evidence that the new worker/queue layout causes the loss. The other observations agree with
§2: loss evenly over the seven TX queues (`…-tx` ≈ 0.77 × `…-output` on every thread), no pause frames, no MDD,
continuous rather than bursty, the 03:00.x pair losing on RX instead.

## 5. Recommendation for the owner and the experiments that settle it

Today's practical ceiling for 64-byte frames: **≈2.6 Mpps per port on a card with four busy ports, ≈3.6 Mpps per port on a
card with two busy ports** (17 % / 24 % of the 14.88 Mpps line rate), limited by the card/host DMA path, not by VPP, not
by the start-up file. At IMIX (≈350 B average) the same packet ceilings correspond to ≈7–10 Gbps per port, at 512 B to line
rate (2.35 Mpps). Three experiments, cheapest first — I measure each read-only the same way as §2:

0. **Runtime, reversible, no cabling (suggested by the manager session; needs the owner's approval, manager step in
   their session):** make one worker poll every RX queue of the first pair, so one TX queue per port carries that pair's
   whole load in large bursts instead of seven queues in 7-packet bursts:
   `for q in 0 1 2 3 4 5 6; do vppctl set interface rx-placement enp2s0f0np0 queue $q worker 0; vppctl set interface rx-placement enp2s0f1np1 queue $q worker 0; done`
   (revert: `… queue $q worker $q`; a VPP restart reverts it too). Worker 0 then carries ≈5.4 Mpps, within its ≈7 Mpps
   capacity at today's per-packet cost (less with larger vectors). If f0/f1's TX failure and `rx_missed` fall while f2/f3
   (still seven queues) keep theirs at the same offered rate, the card is sensitive to the number of active queues and the
   burst size (a per-card DMA-engine effect, hypothesis A); if nothing changes, the limit is per packet regardless of
   queues (B, or a per-chip packet budget). Measure before/after with §6 at an unchanged TRex rate.
1. **No cabling, 5 minutes:** stop the enp2s0f2↔f3 stream and offer the other two pairs the highest 64-byte rate TRex can.
   Expected under both hypotheses: enp2s0f0/f1 climb from 2.6 to ≥ 3.6 Mpps delivered per port. The **total** forwarded
   (today ≈17.7 Mpps in, ≈15.6 out) is what matters: it should stay ≈17–18 if the box (B) is the limit, or fall towards
   ≈14.6 if each card (A) is.
2. **Cabling:** move the second pair's fibres to enp1s0f0np0/enp1s0f1np1 (01:00.0/.1, x8, NVM 9.56, no carrier today) and
   re-create that xconnect pair (manager step in the owner's session) — one pair per card. Total ≈ 22 Mpps → A (advice:
   one pair per card; no software fix exists); total still ≈ 17–18 Mpps → B (next: 1 GB buffer pages / fewer
   descriptors, product change).
3. **IMIX and 512 B at line rate** on all three pairs, to see how much the 64-byte ceiling costs at realistic sizes.

Not recommended without the data from 1–2: `no-multi-seg` / `no-tx-checksum-offload` (no effect on the ceiling; MTU
cost), bigger rings, `intel_iommu=off`.

## 6. Reproducing the measurement

```
ssh root@172.30.110.211 'date +%s.%N; echo "## IF"; vppctl show interface | grep -E "^enp[23]s0f[0-3]|rx packets|tx packets|rx-miss|tx-error|drops"; \
  for p in enp2s0f0np0 enp2s0f1np1 enp2s0f2np2 enp2s0f3np3 enp3s0f0np0 enp3s0f1np1; do echo "## XS $p"; vppctl show hardware-interfaces $p | sed -n "/extended stats/,\$p"; done; \
  echo "## RUNTIME"; vppctl show runtime; echo "## END"; date +%s.%N' > sample-a.txt
# … wait ≥ 30 s, repeat into sample-b.txt, then:
python3 -I docs/status/tasks/machine-211-small-packet-limit-delta.py sample-a.txt sample-b.txt
```

The script prints the per-port table of §2, the per-thread node rates and the TX calls per port. Adjust the port list when
the pairs move (experiment 2 adds `enp1s0f0np0 enp1s0f1np1`).

## 7. Exact next command

After the owner's answer: repeat §6 during the new test and append the results here (§2 table per experiment, totals, and
which hypothesis survived); then decide the follow-up (cabling advice, or the 1 GB-page / fewer-descriptor product change
as its own task with a contract change).
