# PR128 current documentation review and packaging task audit

Review branch `codex/pr128-review-20261003`, isolated `developers/PR128-review`.
Current base `360018d3b`; exact reviewed PR head
`799566f1331921f46a1dcb74aefb09d05dfe5f4a`.
Verdict: APPROVE WITH LIMITS for the two documentation files.

The latest head preserves current-main product content; delta is exactly
docs/decisions/PENDING-vpp-host-hardening.md and docs/lab/host-vrx-a.md. It marks
September memory/netlink/stall readings historical and consistently records the
October guest snapshot: 4096 two-MiB pages (~8 GiB), free4045, 256MiB socket limits,
32vCPU/62.7GiB/oneNUMA and point-in-time service health. Both documents retain
hypervisor balloon/reservation uncertainty and pending latency/packet acceptance.
The fresh head also updates the recommendation/Farsi summary rather than presenting
the old 555-page state as today's. No product privileges/configuration are changed.

Read-only GitHub workflow query at review time: exact-head CI gate run37128418527
was in_progress, conclusion null. Earlier da9 run37127768310 was cancelled and is
not a passing result for this head. Author local quick success is reported, not
independently rerun. Require actual latest hosted green before merge. No live host
readings were taken by reviewer; approval concerns recorded evidence consistency,
not independent verification of current machine measurements.

## Board recommendations (root-owned changes only)

| ID | Factual current source and remaining scope | Recommendation |
| --- | --- | --- |
| P10 | Packaging/firstboot/systemd/signed repository source and bounded fixtures exist; original VPP producer/affinity/source verification exists. CAP_CHOWN/daemon ownership decision remains explicitly pending; release licensing metadata unresolved; actual install/remove/upgrade/reboot/daemon permissions NOT RUN. | Keep RUNNING for unresolved security/release source decisions; record completed reviewed subsets separately, do not mark whole DONE. |
| P11 | Existing strongSwan renderer/VICI/IPsec descriptor source exists; current tree has no deploy/strongswan producer, deploy/debian/vrx-strongswan package or internal/charon lifecycle directory, and VPN IPsec UI is absent. PKI materializer/secret transport remains unwired. | Keep RUNNING; these are source gaps, not merely lab acceptance. External worker output is not merged-source completion. |
| P14 | Main now has builder, strict label/size/manifest/count guards, offline aggregate/workflow and canonical guide. Source subset is implemented; signed production closure/schema/build/reproducibility and disposable BIOS/UEFI VM acceptance remain NOT RUN, dependent on external signed pool. | Replace stale TODO with source-complete/acceptance-deferred tracking according to board convention; do not claim complete appliance acceptance. |
| TD-19 | Pinned Go/module/installer/provisioning/FRR-selection source and strict fixture gates exist; scripts require independently authorized FRR/NodeSource fingerprints. Status records unresolved production fourth FRR signer authorization/Node authority, distinct from live artifact transfer/install/version/boot NOT RUN. | Replace stale TODO with RUNNING/partial source status; preserve source-authority gaps and lab campaign, no blanket DONE. |

Evidence inspected: plan/tasks.yaml current rows, centralized deferred acceptance,
P10 ownership decision/code report, TD19 current-main/FRR selection reviews,
installer scripts and actual main file inventory. No board/shared docs/product edits,
builds, installation, host inventory or full test launches occurred.
