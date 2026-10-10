# Independent proposed DPDK inventory review

Reviewed input files on October 10, 2026:

- `/root/Documents/Codex/2026-10-10/hardware/proposed-dataplane-37.json`, SHA256 `fb96a46f52153f211c2b656841ccd398d4a5755d2d4babfb9ad75da8f6c6e7e7`.
- `/root/Documents/Codex/2026-10-10/hardware/proposed-dataplane-211.json`, SHA256 `38730e6e7b2e93ce7a941455989b51ed2fac7167764a97ce0fc498f87de97bec`.
- Product source remains the original reviewed tree `d2d55984d74fa1d06c32e8271886f11f16375407`; this reviewer changes documentation only.

These files are **PROPOSED, NOT APPLIED**. Structural root filesystem corruption still blocks target installation, activation, PCI rebinding and reboot. No target mutation occurred in this follow-up.

## Independent comparison actually executed

A local Python command read each input document and independently invoked read-only SSH `python3 -` on each host to enumerate `/sys/class/net/*/device`, bound drivers, PCI function IOMMU-group members and bridge masters. It compared the returned physical NIC inventory against all `dataplane.devices` PCI/name pairs and the `pciWhitelist`. It asserted exact management PCI, exact data PCI/name equality, no duplicate whitelist entries or names, no management PCI in either data field, and singleton IOMMU-group membership for every physical NIC. The command exited zero and returned:

```text
172.30.126.37: data_count=7, exact_match=true, all_iommu_groups_singleton=true
  management=enp12s0 pci=0000:0c:00.0 driver=igc group=58 master=null
172.30.110.211: data_count=17, exact_match=true, all_iommu_groups_singleton=true
  management=enp4s0 pci=0000:04:00.0 driver=igc group=28 master=null
```

This is an exact inventory/config comparison and IOMMU isolation observation, not evidence that VPP/DPDK can initialize or forward packets on these NICs. The manager separately reported current-schema validation; this follow-up did not repeat broad tests.

## Exact approved proposed mapping

For 172.30.126.37, `managementPci` must remain `["0000:0c:00.0"]`; it is excluded from all seven data devices and whitelist entries. All physical NICs currently use `igc`. Each listed IOMMU group contains only the PCI function in that row.

| PCI | Logical/kernel name | IOMMU group | Current bridge master |
| --- | --- | --- | --- |
| 0000:0a:00.0 | enp10s0 | 56 | none |
| 0000:0b:00.0 | enp11s0 | 57 | none |
| 0000:0d:00.0 | enp13s0 | 59 | none |
| 0000:0e:00.0 | enp14s0 | 60 | none |
| 0000:0f:00.0 | enp15s0 | 61 | none |
| 0000:10:00.0 | enp16s0 | 62 | none |
| 0000:11:00.0 | enp17s0 | 63 | none |

For 172.30.110.211, `managementPci` must remain `["0000:04:00.0"]`; it is excluded from all seventeen data devices and whitelist entries. The twelve data functions on PCI buses 01–03 use `i40e`; the five functions on buses 05–09 use `igc`. Each listed IOMMU group contains only the PCI function in that row.

| PCI | Logical/kernel name | IOMMU group | Current bridge master |
| --- | --- | --- | --- |
| 0000:01:00.0 | enp1s0f0np0 | 16 | none |
| 0000:01:00.1 | enp1s0f1np1 | 17 | none |
| 0000:01:00.2 | enp1s0f2np2 | 18 | br9 |
| 0000:01:00.3 | enp1s0f3np3 | 19 | br9 |
| 0000:02:00.0 | enp2s0f0np0 | 20 | br2 |
| 0000:02:00.1 | enp2s0f1np1 | 21 | br2 |
| 0000:02:00.2 | enp2s0f2np2 | 22 | br3 |
| 0000:02:00.3 | enp2s0f3np3 | 23 | br3 |
| 0000:03:00.0 | enp3s0f0np0 | 24 | br4 |
| 0000:03:00.1 | enp3s0f1np1 | 25 | br5 |
| 0000:03:00.2 | enp3s0f2np2 | 26 | br4 |
| 0000:03:00.3 | enp3s0f3np3 | 27 | br5 |
| 0000:05:00.0 | enp5s0 | 29 | none |
| 0000:06:00.0 | enp6s0 | 30 | none |
| 0000:07:00.0 | enp7s0 | 31 | none |
| 0000:08:00.0 | enp8s0 | 32 | none |
| 0000:09:00.0 | enp9s0 | 33 | none |

## Blacklist and application limits

The schema's management list is `dataplane.managementPci`, not a separate `blacklist` field. `vppstartup/model.go:598` rejects any device matching a detected management PCI; the startup template renders the protected management PCI into a DPDK `blacklist` entry. Therefore the proposed required blacklist is exactly `0000:0c:00.0` for 126.37 and `0000:04:00.0` for 110.211 under the observed singleton-management route/control inventory. The future guarded renderer must still re-read host facts and preserve any newly detected management NIC before apply; this receipt does not authorize bypassing that guard.

The documents contain dataplane settings only. They do not by themselves create application `interfaces.<name>` physical ownership rows, commit them to PostgreSQL, detach bridge ports, bind PCI devices, restart VPP or establish live interface state. Complete app import and guarded startup remain required after filesystem recovery. Any bridge changes must affect only the listed authorized data ports; management is not enslaved to a bridge. All observed data links lacked carrier in preflight, so packet acceptance will need connected peers or an explicitly recorded no-carrier limitation.

**Verdict: APPROVE these exact proposed PCI/name/management-exclusion mappings as recovery-stage preparation. BLOCK applying them until the offline-root recovery prerequisites in `hardware-review-20261010-review-R8.md` pass.**
