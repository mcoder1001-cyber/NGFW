# apply-211-vpp-startup-20261020 — manager apply of the permanent VPP start-up file on 172.30.110.211 (WIP)

Branch `claude/magical-cartwright-cda9e2` (base origin/main 5593e5ab2). Envelope: `apply-211-vpp-startup-20261020-envelope.md`.
Board row `apply-211-vpp-startup-20261020`. Approval record: `docs/decisions/PENDING-211-vpp-startup-tuning.md` +
LOG.md D-256 (PR #252). Diagnosis and the dataplane document: task machine-211-vpp-tuning, branch
`claude/machine-110-211-traffic-8c1edc`, PR #251.

## State 2026-10-20

| step | result |
|---|---|
| approval record | PR #252: PENDING file + D-256 + board row. Review round 1 (independent reviewer, head 4586600a1): BLOCK, 6 MAJOR / 6 minor; all addressed in round 2 (below). Quick gate: pending at the time of writing. Not merged yet |
| rendering reproduced | see "Generator command" below: sha256 `92985d0c7065ede7dfc417db011c891b06f634ff1c126a25f1493b0ab47191b6`; live file `889f9a6fea6c2ec0e29088d4a816441bfe8c349f6913c64e06e3e05eff0ccc32` |
| document on the host | `scp` → `/root/machine-211-vpp-tuning-dataplane.json`; `sha256sum` on the host and locally both `2699c5d1fa575e77e48d78ccf6dcd3fae85ccd57098127bcb6cee130fb00ffd3` (the file of PR #251 at 40c27c6ac) |
| dry run on the host | see "Dry run" below: both sums as expected, exit 3 only at the TD-17 gate (no `--approve-rendering` given), "nothing changed" |
| owner confirmation | 2026-10-20, relayed from the owner's session: «وقفهٔ ۹ ثانیه‌ای VPP را می‌پذیرم». The 9 s figure was this task's first estimate; the review showed ≈25–35 s (VPP + agent + API). The corrected figure was sent back; the apply waits for the owner's answer |
| apply | **not run** |
| performance cores | `/etc/ngfw/dataplane-performance.json` still `{"cores":[16,2,3,4,5,6,7,8,9,17]}`; `/usr/local/libexec/ngfw-dataplane-performance` applies the governor per CPU (`related_cpus` = the CPU itself on this host, checked for 2, 3, 16, 17). **Never `systemctl restart ngfw-dataplane-performance.service`** (see dependency chain) |
| running configuration | the API's `dataplane` domain on 211 still holds the 2026-10-14 document (9 workers, corelist 2-9,17, 8 RX queues; running revision 3 via `CommitService.systemCommit`, see `appliance-half-resource-defaults-wip.md` at 7fd2d4297 / 0291db553: "Running revision after apply | 3 | 2" for 211 / 37). Committing the new domain needs an administrator login (`ngfw` CLI on the host is not logged in: `ngfw show whoami` → "not logged in"). Follow-up |

### Why product mode
Host 211 is an appliance (packages `ngfw-* 0.1.0~dev+git695.cb148aafb288`, installed 2026-10-19 08:50), it has no
`/root/ngfw` checkout, and `/usr/lib/ngfw/apply-startup.sh` (sha256 `be49ec095677cdf6e81d863dab9c4f3276ecea6a9e3c002606792df21e995c8a`,
identical to `deploy/vpp/apply-startup.sh` on main) was already used in product mode for the 2026-10-14 apply
(`/var/lib/ngfw/startup-apply/20261014-151047-62477/settings`: `NGFW_APPLY_MODE=product`,
`APPROVE_RENDERING=889f9a6f…`, `EXPECT=367ead29…`; `committed` after a 60 s window with 7 identity reads). The lab-mode
gate `--i-have-product-owner-approval` cannot be evaluated there; the PENDING file and D-256 are the approval record
and name the same sum.

### Dependency chain on 211 (read 2026-10-20, `systemctl show`)
- `vpp.service`: `Requires=sysinit.target ngfw-dataplane-performance.service system.slice ngfw-hardware-211-bind.service ngfw-firstboot.service`; drop-ins `20-hardware-211-bind.conf`, `40-resource-defaults.conf` (`Requires=ngfw-dataplane-performance.service`, `After=`, `LimitMEMLOCK=infinity`), `vpp-firstboot.conf`; `RequiredBy=ngfw-agent.service`.
- `ngfw-agent.service`: `Requires=… vpp.service … nftables.service`; `ngfw-api.service`: `Requires=… ngfw-agent.service …`.
- So the apply's `systemctl restart vpp` restarts VPP, the agent and the API (the 2026-10-14 agent journal: agent stopped 15:10:53.02, started 15:10:53.10, one second after the install), and an explicit restart of `ngfw-dataplane-performance.service` or `ngfw-hardware-211-bind.service` would do the same outside the tool.

### Outage measured on 2026-10-14 (`/var/lib/ngfw/startup-apply/20261014-151047-62477/log`)
`15:10:52 installed /etc/vpp/startup.conf; restarting VPP` → `15:11:16 VPP restarted: identity …/63045/…` (24 s to the
first VPP API answer) → `15:12:24 COMMITTED: healthy for 60s (7 identity reads)`. The agent resync ("APPLIED 17/7
interfaces after restart", 7fd2d4297 wip) follows the API answer. Estimate for this apply: ≈25–35 s without forwarding
and without the web UI/API.

### Generator command (this machine, origin/main 5593e5ab2)
```
cd apps/agent && go build -o <scratch>/bin/ngfw-startupgen-main ./cmd/ngfw-startupgen && go build -o <scratch>/bin/ngfw-vppcheck ./cmd/ngfw-vppcheck
ssh root@172.30.110.211 'cat /etc/vpp/startup.conf' > <scratch>/live-startup.conf     # sha256 889f9a6f…
git show origin/claude/machine-110-211-traffic-8c1edc:docs/status/tasks/machine-211-vpp-tuning-dataplane.json > <scratch>/211-dataplane.json
<scratch>/bin/ngfw-startupgen-main --no-host --mgmt-pci 0000:04:00.0 --online-cpus 0-19 --numa-nodes 1 --hugepages-mb 15360 \
    --current <scratch>/live-startup.conf --plugin-dir /usr/lib/x86_64-linux-gnu/vpp_plugins <scratch>/211-dataplane.json | sha256sum
```
Output: `92985d0c7065ede7dfc417db011c891b06f634ff1c126a25f1493b0ab47191b6  -`; stderr: management NIC 0000:04:00.0,
`rendered sha256 92985d0c…`, 3 warnings (linux_cp / linux_nl / npt66 kept from the current file). With `--current none`
the sum is `601c2cca…` (no plugins block), which is why `--current` must be the live file.

### Dry run (host, 2026-10-20 ≈06:05 UTC)
`/usr/lib/ngfw/apply-startup.sh --mode product --doc /root/machine-211-vpp-tuning-dataplane.json`:
- generator: management `enp4s0 → 0000:04:00.0 (default route)`, `rendered sha256 92985d0c…`, the same 3 plugin warnings;
- unified diff: `corelist-workers 2-9,17 → 2,4,6,8,10,12,14`; `num-rx-queues 8 → 7` on the twelve X710 ports; semantic diff: the corelist line plus, per X710 device, a `dpdk > dev 0000:0{1,2,3}:00.x > num-rx-queues 8 → 7` pair (my capture filtered the `dpdk > dev` lines out, so only the corelist pair was visible in it; the gated dry run of step 1 records the full output);
- 17 PCI devices, all `vfio-pci`; management `enp4s0` pci 0000:04:00.0 driver igc manager ifupdown `172.30.110.211/24` gw `172.30.110.1`, restore plan printed; probe `neigh` passes now; manager session with 172.30.126.195 established;
- VPP preflight: `local0` present, boot identity `0219e19a-0e2e-4003-bfc9-1e587c34e368/167929/52292665`;
- gate: `REFUSED: product mode needs --approve-rendering` (expected without the flag; step 2 of the PENDING procedure repeats it with the flag, exit 0 required);
- sums: live `889f9a6fea6c2ec0e29088d4a816441bfe8c349f6913c64e06e3e05eff0ccc32`, rendering `92985d0c7065ede7dfc417db011c891b06f634ff1c126a25f1493b0ab47191b6`; `dry run: nothing changed`; exit 3.

### Live state before the apply (2026-10-20 05:43 UTC)
`vpp.service` MainPID 167929, NRestarts 0, active since 2026-10-19 08:50:35; `show threads` main lcore 16, workers lcores
2,3,4,5,6,7,8,9,17; `show mode`: xconnect pairs enp2s0f0np0↔enp2s0f1np1, enp2s0f2np2↔enp2s0f3np3, enp3s0f0np0↔enp3s0f1np1;
`show log | grep -i malicious`: 5 events (last 2026-10-20 05:22:00, `02:00.1` TX queue 1), none since the runtime remap.

## Review round 1 → round 2 (PR #252)
MAJOR 1 restart of the performance unit restarts VPP → procedure step 1 edits the file and runs the script directly;
MAJOR 2 outage understated → ≈25–35 s incl. UI/API, owner re-asked; MAJOR 3 rollback cost and the impossible
"reinstall backup.conf" → corrected (re-apply the 2026-10-14 document with a new approval); MAJOR 4 running
configuration keeps the old domain → stated, follow-up; MAJOR 5 board state → `parked` on the PENDING;
MAJOR 6 no wip/envelope → this file and the envelope, PENDING results point here; minors 7–12 (dead queues come back at
the next restart, five dead queues, I226-V still 2:1, dry run with `--approve-rendering`, checks on all X710 ports +
resync, D-088 wording, DEC rename, est/worktree fields) → fixed.

## 2026-10-20 ≈06:25 UTC — owner's answer, step 0 and step 1 on the host
- Owner, directly in this session, to the corrected figure (≈25–35 s, VPP + agent + web UI/API): «بله».
- Step 0 (read-only): `vpp.service` `Requires=sysinit.target ngfw-dataplane-performance.service system.slice ngfw-hardware-211-bind.service ngfw-firstboot.service`, drop-ins `20-hardware-211-bind.conf`, `40-resource-defaults.conf`, `vpp-firstboot.conf`; `show mode` 6 xconnect lines (3 pairs); live file `889f9a6f…`; `/var/lib/ngfw/startup-apply/` holds only `20261014-151047-62477` (has `committed`) and `current`. My ad-hoc shell check printed `UNFINISHED` for that directory, which is a bug of the check (`ls a b c` fails when any one of the result files is missing); the tool's own `unfinished_applies` test is part of the gated dry run below, which exited 0.
- Step 1, gated dry run `/usr/lib/ngfw/apply-startup.sh --mode product --doc /root/machine-211-vpp-tuning-dataplane.json --approve-rendering 92985d0c…`: **exit 0**, `gate: product mode — appliance approval of rendering 92985d0c… (dead-man mandatory)`, live `889f9a6fea6c2ec0e29088d4a816441bfe8c349f6913c64e06e3e05eff0ccc32`, rendering `92985d0c7065ede7dfc417db011c891b06f634ff1c126a25f1493b0ab47191b6`, `dry run: nothing changed`. Semantic diff, complete this time: `cpu > corelist-workers 2-9,17 → 2,4,6,8,10,12,14` plus `dpdk > dev 0000:0{1,2,3}:00.{0..3} > num-rx-queues 8 → 7` for the twelve X710 ports (13 pairs). Probe `neigh` passes, manager session 172.30.126.195 established, preflight `local0` present, boot identity `0219e19a-…/167929/52292665` (unchanged since 05:43).
- PR #252: review round 2 APPROVE (head 1f3dfd4d4), round-3 minors pushed as 2150f1046; CI gate run 39380053558 started 06:23:07 (recent gate runs take 16–23 min). The apply waits for the merge.

## Exact next command (after PR #252 on main)
Steps 0–3 of `docs/decisions/PENDING-211-vpp-startup-tuning.md` §Procedure, then the checks 4–8, then step 9.

## Results — 2026-10-20, apply COMMITTED

PR #252 merged as 7442c6a5f (squash, expected head 2150f1046, quick gate pass 21m17s). Then on the host, in order:

| time (UTC) | event |
|---|---|
| 06:37 | step 2: `/etc/ngfw/dataplane-performance.json` backed up to `.bak-20261020` and rewritten to `{"cores":[16,2,4,6,8,10,12,14]}`; `/usr/local/libexec/ngfw-dataplane-performance` run once → `Applied available performance governors …`; governors of 2,4,…,14,16 `performance`; VPP untouched (MainPID 167929, NRestarts 0) |
| 06:39:50 | step 3: `apply-startup.sh --mode product --doc … --apply --expect-sha256 889f9a6f… --expect-new-sha256 92985d0c… --approve-rendering 92985d0c…` → gate accepted, plan sealed `31ecc808…`, detached unit `ngfw-startup-apply-20261020-063950-212602`, planner exit 0 |
| 06:39:51 | run: locks held (holder pid 212645: `ngfw-vpp.lock`, `ngfw-lab.lock`), rendering `92985d0c…`, preflight `local0`, old identity `…/167929/52292665` |
| 06:39:55 | backup `/var/lib/ngfw/startup-apply/20261020-063950-212602/backup.conf` + `/etc/vpp/startup.conf.bak-20261020-063950-212602`; new and rollback files staged; drivers recorded (17 × vfio-pci, 04:00.0 igc); dead-man armed (2600 s); **installed, VPP restarted** |
| 06:39:57 | `vpp.service` active again (MainPID 212946); `ngfw-agent` (212948) and `ngfw-api` (212949) restarted with it (`Requires=` chain) |
| 06:40:16.8 | agent `reconcile done` mode `resync`, `APPLY_STATUS_APPLIED`, `created:23 unchanged:22`, `reapplied:17`, 7.56 s → forwarding back **≈22 s after the install** |
| 06:40:19 | tool: `VPP restarted: identity …/212946/60148891; MainPID=212946 NRestarts=0` (24 s install → first API answer, as on 2026-10-14) |
| 06:41:28 | **`COMMITTED: healthy for 60s (7 identity reads; manager session with 172.30.126.195: established); dead-man cancelled`**; locks released |

Live file after the apply: `sha256sum /etc/vpp/startup.conf` = `92985d0c7065ede7dfc417db011c891b06f634ff1c126a25f1493b0ab47191b6`.

Checks 4–8 (read-only, 06:41–06:43 UTC):
- **4** `show threads`: `vpp_main` lcore 16; `vpp_wk_0..6` lcores 2,4,6,8,10,12,14 (8 threads).
- **5** `show hardware-interfaces`, TX queue rows per port: the twelve X710 ports `enp1s0f0np0 … enp3s0f3np3` each 8 rows `shared no` (queue q → thread q, 0–7), 0 `shared yes`; the five I226-V ports `enp5s0 … enp9s0` 4 rows `shared yes` (0,4 / 1,5 / 2,6 / 3,7), as expected.
- **6** `show log | grep -ci malicious`: 0 (in-memory log since the restart).
- **7** `vpp`, `ngfw-agent`, `ngfw-api` active, `NRestarts=0`; `show interface`: 17 data ports up, none `enp*` not up; `show mode`: the three xconnect pairs (6 lines); management `enp4s0 172.30.110.211/24`, `default via 172.30.110.1` unchanged; `show interface rx-placement`: worker q polls queue q of every port.
- **8** under the owner's TRex traffic (running during the whole window), `show interface` on the six test ports:

| port | tx 06:41:57 | tx 06:42:17 | tx 06:43:30 | tx-error (all three reads) | rate over 93 s |
|---|---|---|---|---|---|
| enp2s0f0np0 / f1 | 25 191 853 / 25 191 849 | 41 976 788 / 41 976 788 | 105 483 708 / 105 483 732 | 0 / 0 | ≈0.86 Mpps per direction |
| enp2s0f2np2 / f3 | 29 326 319 / 29 326 335 | 49 008 050 / 49 008 056 | 123 077 058 / 123 077 058 | 0 / 0 | ≈1.0 Mpps per direction |
| enp3s0f0np0 / f1 | 99 827 210 / 99 827 405 | 119 208 190 / 119 208 385 | 190 071 803 / 190 071 997 | 0 / 0 | ≈0.97 Mpps per direction |

`show errors` lists no `dpdk tx failure` / drop counters. Governors of the former worker cpus 3,5,7,9,17 stay `performance`
until the next boot (expected, harmless).

Not done here: the API's running `dataplane` domain still holds the 2026-10-14 document (follow-up for an admin
session: `ngfw configure merge dataplane` with the PR #251 document, then `commit`); a pointer in PR #251's
`machine-211-vpp-tuning-wip.md` (that PR is still open, its session owns the file); no throughput ceiling was measured
(the generator offered ≈0.86–1.0 Mpps per direction). The PENDING file is renamed `DEC-256-211-vpp-startup-tuning.md`
with this commit.
