# Task: F-bruteforce-detectors — trusted host observations → auto-block

## Goal
Close the ready board row by auditing existing implementation and fixing gaps in
SSH/journal, VPN, kernel scan → StreamEvents → AutoBlockService.observe → enforcement.
Reuse F-bruteforce-block and F-global-blocking; never duplicate their runtime sets.

## Inputs
prompts/00-CONTEXT.md; docs/contributing.md; decision-policy.md; F-bruteforce-block
prompt; FEATURE-TEMPLATE.md; exact plan/tasks.yaml row; agent detectors/watchers;
API auto-block engine; topology acceptance driver; native secret-safe VPN reader.

## Contract
Existing EVENT_KIND_AUTOBLOCK_OBSERVED = 25, attributes source_ip/detector is already
on main (4ead8bd2). Add only optional map destination_port for portScan; contract
note committed first. No hand-generated stubs, privileges, binaries or RPC changes.

## Scope
1. Audit trusted SSH, correlated charon parser and owned native VPN watcher. Charon
   remains test-peer compatibility, never authoritative product VPN input.
2. Preserve source and scan destination port through existing StreamEvents wiring.
3. API subscriber validates observations, applies configured thresholds/windows and
   allowlist, counts distinct scan ports with repeat refresh and bounded sources.
4. Meaningful unit tests for trusted/replayed events, scan refresh, invalid port,
   subscriber shutdown and management/IPv4-mapped allowlist protection.
5. Live topology driver: 10 rejected web/SSH logins → block both traffic paths →
   expiry or manual removal restores both → allowlisted source stays permitted.
6. Update user docs and recovery envelope/WIP/report. Manager owns board and merge.

## Acceptance
Actual focused unit/vet output; source-check gate; full quick gate where tools allow.
Lab-only acceptance remains NOTRUN until real local-in/forwarding output exists.
No invented live proof, no VPP restart or host journal mutation in cloud workspace.

## Out of scope
New auth protocols/IPS, native VPP patches, enforcement redesign, UI changes, board,
main, generated binapi, other worktrees. Existing host watcher edit explicitly granted
by manager; other owned files follow exact board row.
