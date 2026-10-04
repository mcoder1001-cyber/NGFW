# Fresh security host acceptance — 2026-10-04

Both scoped live tests passed against the real API, PostgreSQL18.6, Valkey, production agent and a disposable VPP. No fake agent was used and no test skipped. This closes the forwarding/expiry/replay/rollback host cases below; it does not certify browser screenshots, feed refresh failure cases or historic200k-entry scale/performance requirements.

Source audit branch codex/closeout-traffic-audit at eca0fde51; parent product tree0f8f40429f67ac6309bae1f0d65902172c1abba1. apps/agent and apps/api source match; generated YANG is the only difference among inspected apps/packages/test harness paths. The parent supplied the already-built product artifacts:

- ngfw-agent SHA25639c1db9f0d9f80089e8df860642ae931ce7cd7e6902d197f945cda529d4723f4.
- apps/api/dist/main.js SHA25627bfd88933101b58826a9f9d0ee3960c00276cddb4f350edc2f05d5e516b9a7f.

## Exact run and outcome

`NGFW_SECURITY_PRODUCT_ROOT=/root/.codex/worktrees/6189/NGFW python3 test/topology/security-host-acceptance/run.py`

The runner loads canonical tools/lab env8, stages the existing ACL module and custom focused assertions in a temporary directory, invokes tools/heavy.sh + isolated-vpp.py + go test -v -count=1 -timeout5m with exact selected test names. It supplies the existing product NGFW_HOST_ACL_NETNS=ns-w8-host hook to the real agent, keeping nft changes outside the shared host network namespace. API/DB traffic and real AF_PACKET rig devices remain in the original network namespace. Source syntax/dry-run and tools/ci.sh check passed; no complete quick/full gate is claimed here.

Final raw evidence: [netns-product-final.txt](closeout-security-host-evidence/netns-product-final.txt).

| Test | Actual outcome |
|---|---|
| TestRuleExpiryRealAPI | PASS35.68s. Real API commit applied; before expiry3/3 packets. Without another commit the VPP permit disappears, one deny remains and packets0/3. API running config unchanged, Retrieve retains expiresAt/owner/ticket. Stop own agent, delete only owned VPP ACLs, restart actual agent: ACL rebuilt with expired permit absent; packets0/3. Rollback removes all owned ACLs and restores3/3. |
| TestGlobalBlockingRealAPI | PASS32.54s. Real API commit applied creates global-block ACLs and host-acl.nftables. Listed forwarding source and reverse destination each0/3. Unlisted source3/3. Host local-input listed peer0/3; actual nft JSON has inet ngfw_w8/in__gb and drop counter3packets252bytes. Stop own agent, delete owned VPP ACLs, restart: recreated ACLs and forwarding0/3. Selecting only WAN makes listed source pass through unselected LAN to its gateway3/3. Rollback removes owned ACLs and restores routing and host local-input each3/3. |

Combined EXIT0: two live tests, no skips,68.242s. Test source asserts transmitted count as well as received count, avoiding a missing/broken ping being interpreted as a drop. ARP warmup is bounded and excluded from acceptance counts. No throughput claim.

## Cleanup and retained failures

Disposable VPP PID3594547 stopped by its runner. Own API/agent processes stopped; DB ngfw_w8 and role dropped. All ns-w8-lan/wan/host/hostpeer absent after exit; rig veths removed. Shared VPP NRestarts remained unchanged. Slot8 released to manager.

All setup failures are preserved, never relabelled PASS:

- first-run.txt: initial rig ARP cost one baseline reply; added bounded warmup then strict3/3 assertion.
- rerun.txt: newly added unlisted source needed its own ARP warmup; expiry already passed.
- isolated-host-final.txt: moving entire agent to host netns hides root rig veths from legitimate AF_PACKET validation. This discarded setup was not a product defect.
- host-mirror-rerun.txt: test owner defaults to nft check mode; applied syntax does not establish enforcement, so local-input3/3 correctly failed. Replaced discarded agent/mirror workaround with the existing official NGFW_HOST_ACL_NETNS renderer hook.
- global-replay.txt: earlier forwarding-only actual-replay pass. Final product-netns run supersedes it for local-input evidence.

No product source was edited. Root manager owns board reconciliation, independent review and publication. Remaining wider acceptance must stay explicit: browser/screenshots, feed scheduled refresh and invalid/empty-list recovery, IPv6/global anti-lockout interactions,200k scale, and complete aggregate gate on final integration tree.
