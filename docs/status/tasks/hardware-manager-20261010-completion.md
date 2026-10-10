# Hardware installation and repair — final acceptance

Status: **DONE** at 2026-10-10T16:40:13+00:00. All mandatory execution checks passed on both appliances and R7 independently approved final scoped acceptance. This is the existing single hardware campaign, not another task. Carrierless physical peer packet tests are the sole authorized laboratory deferral.

## Result

Both scoped offline ext4 root repairs and clean checks passed; original raw undo and repair evidence remain private/offhost. This repairs the filesystem, not SSD wear. Both appliances have all11 native packages installed/configured with empty dpkg audit. Management stays on .37 enp12s0/PCI0000:0c:00.0 and .211 enp4s0/PCI0000:04:00.0 with their original static addresses/default routes. Protected management addresses, global routing/rules, DNS, sysctls and firewall semantics pass the documented protected/current comparisons. Full current L3 is captured and compared with the accepted protected preboot state. Authorized VFIO binding removes unused kernel DATA interface entries; earlier .37 normal boot also removed four unused DATA IPv6 link-local addresses and twelve related routes, documented in the WIP. This does not claim every original kernel DATA address/map entry survived binding.

Seven data NICs on172.30.126.37 and seventeen on172.30.110.211 are persistently bound through finite appliance-specific boot binders and imported by canonical native configuration. All24 desired enabled flags match actual API adminUp and independent VPP up state after reboot. Running/candidate equality, no pending commit, in-sync agent, original seeded native revisions(.37 revision1; .211 revision2 with originalseed1 and scoped buffer resource revision2), RPC and authenticated admin HTTPS pass. Actual RX requirements7168/17408 fit VPP pools16784/66297. No management PCI function/global VFIO ID/unsafe-no-IOMMU setting was changed.

Both appliances completed two authorized normal hardware-test reboots: first exposed old RTC clocks causing HTTPS certificate-not-yet-valid; controller-authoritative UTC plus standard RTC write fixed the actual failure, and second reboots prove UTC/RTC persistence and final authenticated native acceptance. Original failed receipts and both one-use request records are retained. No certificate regeneration, verification bypass, NTP-source change, desired-timezone change, network reload or manual postboot service restart was used. Chrony remains active but unsynchronised; manual UTC/RTC correction is verified and no NTP-synchronisation claim is made.

The desired native timezone is UTC. Canonical protected managed symlinks resolve to correct UTC bytes/offset. systemd259.5 timedated does not label the required two-hop managed link; this precise compatibility limitation is recorded instead of falsely claiming a fresh timedated label.

## Final execution evidence

Private receipts are root-owned0600/fsynced beneath /root/Documents/Codex/2026-10-10/hardware. Passwords, tokens, private keys and raw undo are not published.

| Check | .37 | .211 |
|---|---|---|
| Final new boot ID |66e81101-eb23-4183-a845-098d0f69a11d|a0915f60-2978-4c1d-af5f-4aa3c96e2703|
| Protected boot runtime receipt |manager-boot37-observe-boot-20261010T163007Z.json|manager-boot211-observe-boot-20261010T163630Z.json|
| Boot receipt SHA256 |66f3f543fb3b5df8e3d2382cdb71e2a26e08f85c03983653cfa94019d9c840c2|d424d176728a2b1e78332142363ac768bd81e0e097daccdca412f1d749d7f2be|
| Final authenticated native receipt |manager-physical-native37-postboot-20261010T163040Z.json|manager-physical-native211-postboot-20261010T163709Z.json|
| Native receipt SHA256 |89b2eab7dd26d820bab52d6e058f886506bd08a3d9e74a245faec28bd7a27b01|a7d36be76ee27e1c05287c569a7f6a485e0a995cf00e94afeb1601316505627b|
| Absolute UTC/RTC persistence receipt |manager-clock-persistence37-20261010T163005Z.json|manager-clock-persistence211-20261010T163501Z.json|
| RTC receipt SHA256 |18eabed836c966b6721fa16534a0a301a3b3bf306d20218737e27035a924b9bf|29aeab7ef2af148acb5216a625a14cda8d773c883403c11523a65f489a3b4744|

Both final boot/native checks require fresh same-epoch stable storage counters and full fresh-boot kernel storage-error scan; no new storage errors. Both root filesystems are clean, runtime core units automatically active with zero restarts, and persistent boot binders/dependencies correct. Postboot acceptance is read-only.

Final management-destination HTTPS receipt manager-external-HTTPS-20261010T163628Z.json SHA30a032a2f88f798fe9952931ba6b397eb428e270c6b2b4154bdde6dd60e31bf4: both web200/protected unauthenticated API401/actual referenced JS200. TLS verifies each SSH-authenticated appliance certificate and its localhost DNS identity while connecting to its management IP. This is not an IP-SAN/browser-trust claim. Identical response content produces the same SHA as the earlier receipt; the distinct immutable file records the actual final rerun.

Exact readonly RTC commands and full source are preserved in hardware-manager-20261010-clock-persistence-37-source.py and hardware-manager-20261010-clock-persistence-211-source.py. The37 source archives the original inline Python body after execution; the211 source was published before execution and its sourceSHA/command are in its receipt. Neither source writes any clock/device/configuration.

## Source integration and validation

PR217 native API dependencies, PR225 firstboot plugins/snapshot and PR226 bounded DPDK EAL fix are integrated. Latest verified main at16:36 UTC is0c21e65eea8b2d2cfb61f57ec9f6b57a0e6c55f3, unchanged complete hosted quick38062974117SUCCESS; PR226 final branch48afba4adfcfe7f0ef0d8585d7c1afc9ed5edde9 complete quick38061529505SUCCESS. Native four-package payload version0.1.0~dev+97ae88ee5b6a is compiled from97ae88ee5b6aaf304f78547abed39e80bbeac5e1, not falsely relabeled48af/merge0c; product paths match the final docs-only integration delta. Native archive manifests, exact package hashes, meaningful fixtures and independent source/artifact reviews remain in the campaign WIP.

R7 independently approved physical commit, strong preboot native, source/clock ABI/frame predicates, actual repairs, both RTC persistence receipts, final37 boot/native and scoped packet deferral. Final211 boot/native/external receipts are independently APPROVED in published R7 verdict6cb4a6430c4450d1e628de0d1d2f83a4634a9b5d; no real code, clock, TLS or postboot native blocker remains. ROOT operational check65132PASS14s/gitleaks7.27KB/board212valid. R7 final source/actual verdict6cb4 documentation checkPASS13s/394.14KB/board212valid. Final ROOT committed documentation check and exact publication readback follow closure.

## Remaining laboratory execution

Physical peer forwarding/loss/throughput: **NOT RUN**, all24 data ports have linkUp=false and speed0 with no carrier/connected peer. Exact scoped deferral and next peer test are in docs/status/DEFERRED-ACCEPTANCE.md. No real code, clock, TLS or authenticated native failure is deferred. This hardware campaign has no existing WBS row: no unrelated board item was marked Done or started; board remains212 tasks/205merged/7parked.

Next action: none for mandatory installation/repair acceptance. Execute only the documented peer packet laboratory tests when connected peers/carrier are available. This single campaign is DONE; no unrelated task is started. Publication branch: codex/hardware-manager-20261010; last published/read-back parent2344b2b5a00bbad0bb2c3be95db66893718efb04. Final status commit and exact remote readback are reported in the task publication receipt and GitHub completion comment.
