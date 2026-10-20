# Host 172.30.110.211 — why VPP forwarded too little traffic, and the fix (WIP)

Branch `claude/machine-110-211-traffic-8c1edc` (base origin/main 921f82388). Envelope:
`machine-211-vpp-tuning-envelope.md`. Status 2026-10-20: diagnosis done, runtime fix applied and
measured, permanent configuration rendered and waiting for the manager apply step (VPP restart).

## Finding

The box was dropping **23 % of all forwarded packets** since 2026-10-19 (`enp2s0f0np0`: 15.58 G
`tx-error` of 68.95 G `tx packets`; per 10 s window: 1.06 Mpps offered, 266 kpps dropped). Every drop
is `Tx packet drops (dpdk tx failure)` on **TX queue 1** of the four `0000:02:00.x` X710 ports.

Cause chain:

1. `startup.conf` (from the dataplane document applied 2026-10-14) has 9 workers (`2-9,17`) + main
   core 16 = **10 VPP threads but `num-tx-queues 8`**. VPP maps thread *t* to TX queue *t* mod 8
   (`src/plugins/dpdk/device/init.c`), so threads 8/9 (`vpp_wk_7`, `vpp_wk_8`) share queues 0/1 with
   main and `vpp_wk_0`. Queue 1 was shared by two *transmitting* workers (`wk_0` on cpu2, `wk_8` on
   E-core cpu17).
2. On the `02:00.x` NIC (X710, NVM 9.56 / fw 9.156) the shared queue triggers an i40e **Malicious
   Driver Detection** event and the hardware disables the queue; the DPDK PMD only logs it
   (`vppctl show log`: `Malicious Driver Detection event 0x02 on TX queue 2 … device 0000:02:00.1`,
   2026-10-19 13:25:46/52 for `.1/.0`, 2026-10-20 04:40:56/04:47:26 for `.3/.2`; the logged queue
   number is VPP's queue + 1). DPDK bug 1343 ties event 0x02 to "too many segments". From then on
   `rte_eth_tx_burst` returns 0 on that queue and VPP drops every vector sent there. The `03:00.x`
   NIC (NVM 6.80) had no MDD event and only ≈4 % loss on its shared queue.
3. Reproduced live at 05:22: making queue 0 of `enp2s0f1np1` shared by `wk_7`+`wk_8` killed it within
   minutes (5th MDD event). No further events after the remap below.

Other observations (not the cause, but part of the permanent fix): the worker list `2-9,17` is four
P-cores with **both hyper-threads** plus one **E-core** (i7-12700T: P-cores cpu0-15 in HT pairs,
E-cores cpu16-19); P-cores 10-15 idle at 800 MHz; governor already `performance` on VPP cores;
package ≈83 W, all-core 4.19 GHz, no power throttling; buffers 1 048 576 / 15 GiB hugepages fine;
PCIe x8 (01/02) and x4 (03) fine; no RX misses during the test. Traffic: TRex UDP, 64-byte frames,
≈0.9–1.1 Mpps per direction on three xconnect pairs (`enp2s0f0np0↔f1`, `f2↔f3`, `enp3s0f0np0↔f1`).

## Runtime fix applied on the host (VPP CLI only, lost at the next VPP restart)

Workers `wk_0` and `wk_8` no longer poll the X710 test ports, so no live TX queue is shared by two
transmitting threads and the dead queues are unused:

```
vppctl set interface tx-queue enp2s0f0np0 queue 0 threads 0,8,9   # and queue 1 threads 1
vppctl set interface tx-queue enp2s0f1np1 queue 0 threads 0,8,9   # and queue 1 threads 1
vppctl set interface rx-placement enp2s0f0np0 queue 4 worker 4
vppctl set interface rx-placement enp2s0f0np0 queue 3 worker 1
vppctl set interface rx-placement enp2s0f0np0 queue 2 worker 2
vppctl set interface rx-placement enp2s0f1np1 queue 5 worker 3
vppctl set interface rx-placement enp2s0f1np1 queue 4 worker 4
vppctl set interface rx-placement enp2s0f2np2 queue 6 worker 2
vppctl set interface rx-placement enp2s0f2np2 queue 5 worker 2
vppctl set interface rx-placement enp2s0f3np3 queue 7 worker 1
vppctl set interface rx-placement enp2s0f3np3 queue 6 worker 1
vppctl set interface rx-placement enp3s0f0np0 queue 7 worker 1
vppctl set interface rx-placement enp3s0f1np1 queue 0 worker 1
```

Measured after the change (20 s, all six active ports): `tx-error` unchanged on every port = **0 loss**;
per thread `…-tx` vectors = `…-output` vectors; rates 0.97 / 1.13 / 0.97 Mpps per direction on the
three pairs, i.e. whatever the generator offers. Dead queues that remain (unused): `02:00.0` q1,
`02:00.1` q0+q1, `02:00.2` q1, `02:00.3` q1 — they come back at the next VPP restart or an admin
down/up of the port (`rte_eth_dev_stop/start`). Nothing on the host's disk was changed.

## Permanent configuration (manager apply step, VPP restart ≈ 9 s, D-088 procedure)

Document: `machine-211-vpp-tuning-dataplane.json` (the `dataplane` domain only; the running document's
other domains are unchanged). Rendering: `machine-211-vpp-tuning-startup.conf`,
**sha256 92985d0c7065ede7dfc417db011c891b06f634ff1c126a25f1493b0ab47191b6** (current file
`889f9a6fea6c2ec0e29088d4a816441bfe8c349f6913c64e06e3e05eff0ccc32`). Rendered with the generator of
this branch and the host facts (`--no-host --mgmt-pci 0000:04:00.0 --online-cpus 0-19 --numa-nodes 1
--hugepages-mb 15360`).

| setting | now | proposed | why |
|---|---|---|---|
| workers / corelist | 9 / `2-9,17` | 7 / `2,4,6,8,10,12,14` | one worker per P-core, no HT sibling pairs, no E-core worker; cpu0-1 + siblings + E-cores 17-19 stay for kernel, API, agent |
| mainCore | 16 | 16 | E-core is fine for the main thread |
| X710 ports rxQueues / txQueues | 8 / 8 | 7 / 8 | one RX queue per worker; **8 TX queues = 8 threads → no shared queue** |
| I226-V ports rxQueues / txQueues | 4 / 4 | 4 / 4 | hardware maximum; still shared 2:1 (igc has no MDD; residual risk noted) |
| buffersPerNuma, hugepagesGb | 1 048 576, 15 | same | adequate |

Also change `/etc/ngfw/dataplane-performance.json` to `{"cores": [16, 2, 4, 6, 8, 10, 12, 14]}` so the
performance governor follows the new cores.

Apply (manager, with the owner's approval recorded per D-088):

```
cd apps/agent && go build -o bin/ngfw-startupgen ./cmd/ngfw-startupgen && go build -o bin/ngfw-vppcheck ./cmd/ngfw-vppcheck
deploy/vpp/apply-startup.sh --doc docs/status/tasks/machine-211-vpp-tuning-dataplane.json            # dry run on the host
deploy/vpp/apply-startup.sh --doc … --apply --expect-sha256 889f9a6f… --expect-new-sha256 92985d0c…
```

Check afterwards: `vppctl show hardware-interfaces enp2s0f0np0` lists every TX queue `shared no`;
`vppctl show log | grep -i malicious` stays empty; `vppctl show interface` `tx-error` does not grow
under the TRex test.

## Product change on this branch

`apps/agent/internal/renderers/vppstartup/model.go`: `warnSharedTxQueues` — a warning for every
`txQueues` (default or per device) below workers + 1, naming the shared-queue mapping and the X710
MDD consequence. The current 211 document produces 17 such warnings; the proposed one produces 5
(the I226-V ports, hardware maximum 4). Test `TestSharedTxQueueWarning`; mapping rows in
`docs/agent/renderers/vppstartup.md`. `go test ./internal/renderers/vppstartup/`, `go vet`, `gofmt`: pass.

## Not done / limits

- The permanent file is **not applied**: it needs the manager apply procedure and a VPP restart.
- No throughput ceiling was measured: the generator offered ≈1 Mpps per port; the X710 line rate for
  64-byte frames is 14.88 Mpps per port, so a higher-rate test is needed before any capacity claim.
- Upstream: the shared-TX-queue → MDD behaviour deserves a VPP/DPDK report (not filed here).
- The renderer still picks worker CPUs by number when `corelist` is absent; it does not know HT
  siblings or hybrid cores. The 211 document uses an explicit corelist instead.
