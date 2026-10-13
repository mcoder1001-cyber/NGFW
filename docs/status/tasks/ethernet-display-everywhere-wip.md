# Ethernet display mapping

- Branch: `codex/ethernet-display-everywhere`; base `74dc791cd` (deployed interface alias implementation). Initial checkout `ba55d2744` predates that implementation; this task fixes the deployed tree.
- Compiled source checkpoint: `3c4e834ce`. Subsequent status commits only record verification.
- Root owns naming map, authenticated inventory provider, docs and integration. Independent branches supplied interface pages, other network screens, and UI-kit/config rendering; recovery worktrees under `/root/ngfw-wt/ethernet-display-*-20261013`.
- Cause: old labels append `state.linkSpeedKbps`; inventory-only, offline or zero-speed ports become `EthernetN-Unknown` despite known PCI identity.
- Completed: stable `EthernetN` labels; native hardware detection; canonical/VPP/Linux alias lookup; VLAN labels; shared authenticated provider; tables, controls, dialogs, summaries and schema-aware config displays. Logical identities remain unchanged; host pickers resolve in the Linux namespace, capture uses the engine namespace, unrelated object names are preserved.
- Independent review: root mapping/provider approved; host namespace/VLAN, QoS object-label collision and double-formatting bond-member findings all fixed and regression-tested. Final UI-kit/config review has no remaining actionable findings.
- Verification: 68 interface tests, 104 UI-kit tests, 44 network consumer tests, 32 config tests, 2 QoS collision tests and 2 bond collision tests passed in relevant worker suites. Integrated web typecheck and focused root tests passed before final review fixes; final integrated checks are running.
- Required full quick gate: running `NGFW_CI_TASK_CONCURRENCY=2 tools/ci.sh quick --base 74dc791cd` against source `3c4e834ce`, output `/tmp/ethernet-quick-final.log`. An earlier superseded run exposed a test label assertion (Key versus Name), corrected before the final run.
- Publication: direct push initially failed because the configured local proxy was unavailable. GitHub connector published checkpoint `8ff55d741d1ea8563d8855c4439640a219ee8f01`; direct Git with proxy variables unset subsequently worked. That checkpoint was merged without changing the working tree, preserving both histories.
- No host deployment performed. Next: finish the complete quick gate, record actual result, publish final branch and review PR.
