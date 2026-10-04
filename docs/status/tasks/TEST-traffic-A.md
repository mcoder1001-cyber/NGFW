# TEST-traffic-A — composed source completion

Branch `codex/ready-traffic-a-composed-20261004`; worktree
`/root/.codex/worktrees/f796/work-traffic-a`. Product source base d5557440c.
Published source checkpoint42b8147674b38df9870dd4ffc986d7a8489ce0fa via GitHub connector.
CLI push cannot publish (403); connector checkpoints are remote and durable.

Implemented complete product candidate baseline/chain, tagged peers/probes, private bounded
capture production, checksum-checked packet correlations, own NAT fixture ED/EI transitions,
CLI/Retrieve/counter attribution, candidate hash/revision linkage and normal rollback/residue checks.
Foreign/dirty/stale candidate, lost lease, changed NAT fingerprint, nonempty NAT config/users,
ignored composed domains, invalid/missing capture and partial applied cleanup all refuse.
Owned processes are stopped; failed applied transactions retain a down rig/evidence for recovery.
No test restarts VPP, installs services, uses packet trace or grants agent global privileges.

Actual source validation: RootConfig schema success, semantic[];55Python source fixtures pass
without skips; Go globals helper unit3tests pass and go vet passes. Additional authority fixtures
run in final source gate. These tests neither exercise VPP nor prove live forwarding.

Live full-chain/rollback/packet acceptance: **NOTRUN**. Manager explicitly has not allocated
an otherwise-idle traffic window while other chats are active. Full quick remains mandatory;
known prerequisite schema/tunnel-locale baseline failures are repaired separately by root PR162.
Independent source review is requested; whole-chain host PASS cannot be inferred from merge.

See `test/topology/traffic-a/COMPOSED.md` for exact next host build/run commands and provisioning;
`TEST-traffic-A-composed-wip.md` records recovery state and remaining acceptance.
