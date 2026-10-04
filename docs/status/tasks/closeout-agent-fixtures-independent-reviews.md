# Independent review hand-back, 2026-10-04

Reviewer: management_acceptance, separate branch/worktree from reviewed implementation owners. Read-only inspection; no reviewed product/test edits. Remote publication blocked HTTP403 remains explicit.

## Root generated contracts / gate

Reviewed root commit `0f8f40429f67ac6309bae1f0d65902172c1abba1`: APPROVE.
Generated setup and OSPF additions reflect existing schema; `@ngfw/yang` already participates in generation. Including its committed output in `GEN_PATHS` makes the same dirty/untracked guard enforce freshness. Inspected generated files and CI source are clean in the root tree.

Reviewed root `test/topology/test-closeout/run.py`: original blocker was sourceHEAD recorded independently of the existing compiled test binary. Manager added frozen sourceSHA, required build-provenance sidecar, binarySHA verification, and rejection of dirty tracked agent source. Readback confirms the issue addressed; APPROVE current runner scope. Private-VPP runner protects shared service identity and terminates only its own processgroup. Skips and missing actual test outcomes fail closeout rather than silently pass.

## Traffic/security scoped acceptance

Reviewed `/root/ngfw-wt/codex-closeout-traffic-audit` commits `eca0fde511265d123d40b50b48d913e66601c9f9` and `a1e0b31b2dfe7b1d87300f1740ae9ef395120ecd`: APPROVE scoped real expiry/global-blocking host acceptance. Did not rerun released slot8; read actual final `netns-product-final.txt` and source/harness/production hook.

- Strict transmitted/received counts reject broken ping as apparent block. Successful baselines and rollback bracket packet-drop assertions.
- Expiry crosses real API/agent/VPP: 2rules→1 without another commit,3/3→0/3; running config unchanged. Recorded Retrieve retains expiresAt/owner/ticket. Deleting owned ACLs while the own agent is stopped, then restarting, proves persisted replay rather than simply retaining live VPP state.
- Global blocking tests listed source forwarding and reverse listed destination, unlisted forwarding, unselectedLAN, actual production netnsNFT enforcement. Final nft JSON shows inet ngfw_w8/in__gb, source set, drop rule and3packets252bytes. Rollback restores forwarding and host local-in3/3.
- Existing official `NGFW_HOST_ACL_NETNS` hook loads only private namespace firewall rules; slot8 globals owner0 and disposable VPP guard remain enforced. Owned processes, namespaces, database/role and disposable VPP are cleaned. No product source changed.
- Source audit correctly keeps unavailable composed traffic executors and broader feed/IPv6/anti-lockout/browser/200k obligations explicit. Scoped success does not establish complete historic task scope.

Non-blocking hardening: add explicit regression assertions for retrieved expiry metadata and nft counter JSON rather than only logging them. The current recorded readbacks do support the report's claims, so this does not invalidate the observed acceptance.
