# Ethernet display mapping

- Branch: `codex/ethernet-display-everywhere`.
- Base: `74dc791cd` (`codex/deploy-interface-names-ngfw`, deployed interface alias implementation). Initial checkout `ba55d2744` predates that implementation; the task branch uses the deployed tree to fix the observed bug.
- Root owns the naming map, authenticated app provider, docs, integration and validation. Delegated worktrees own interface pages, other domain pages, and shared UI-kit/config rendering.
- Cause: the old label suffix was derived from `state.linkSpeedKbps`; inventory-only, offline or zero-speed ports became `EthernetN-Unknown` despite known PCI identity.
- Implementation in progress: speed-independent `EthernetN` labels shared throughout the UI; VLANs and host/VPP spellings resolve consistently while edits retain original identities.
- Validation: pending integrated build and tests. No host deployment performed.
- Next command after integration: `tools/ci.sh quick --base 74dc791cd`.
