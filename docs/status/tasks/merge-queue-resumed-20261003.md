# Resumed merge queue — 2026-10-03

Root owns finite validation jobs and sequential merges; three developers use separate feature/review worktrees. No waiting developer must hold a product worktree for root tests. Frozen heads are not edited during validation. Short fixture lane and bounded heavy lanes run independently; only owned test processes are cleaned up.

| PR | Result | Reviewed head | Full hosted quick gate | Main merge |
| --- | --- | --- | --- | --- |
| 120 | merged, process cleanup sealed before final validation | 6041b1ab | 37121830337 success | f469d56e |
| 118 | merged, inherited CPU affinity respected | 301998cf | 37121698391 success | b2c214b5 |
| 123 | merged, truthful unavailable PKI file RPC | 56dadacf | 37125083465 success | 78d187a4 |
| 122 | merged, console credential retention/recovery | a784561a | 37125285146 success | f3d10efb |
| 124 | merged, precise PKI expiry severity | 13d87fd1 | 37125606956 success | f9bc4a4e |
| 125 | merged, strict installer disk size inventory | ec7a7cc2 | 37125833896 success | dcaab032 |
| 127 | pending final CI and combined gate; bounded OSPF observations | 3251c614 | 37126886202 running | pending |

Each completed merge was checked against root's current-main composition tree. The six merges are conflict-free, and the active workspace is synced to dcaab032. Reviewed remote heads are preserved under archive/*-reviewed-root-20261003 branches.

Actual focused checks: pipeline 11 cases; VPP verification and Debian packaging fixtures; ISO aggregate 70 cases, then 71 with the disk-size guard; PKI RPC race/lint/build with clean temporary output; PKI expiry/DTO 11 cases plus typecheck/lint; OSPF 21 cases, API lint/typecheck, client compile tests and generated CLI API tests. Hosted ISO fixture gates succeeded on final reviewed console and size source heads. Live acceptance is not inferred from these fixtures.

Root combined source checkpoint499deec3b contains current main, final OSPF implementation and strict size guard. Finite heavy job83c77500fa134acf8b60b0c6ed057d4b runs the unchanged repository quick gate with a 1500-second bound; its final result is pending. OSPF hosted failure37126512312 was solely the missing contract commit marker after publication consolidation. Forward commit3251c614 restores the reviewed contract marker without changing the source tree; source review2176e066a and local focused validation f9deacdb remain applicable by exact tree equality.

Earlier OSPF review569e83d9 requested real FRR mapping corrections; subsequent55214641 and final2176e066a approve them. Review0299bbfb was source-only for an intermediate error test whose actual execution still failed; d684e186 replaces that fixture with a plain asynchronous failing adapter and a real HTTP503 problem-filter regression, approved in a2535d74. Final root21-case validation passed. No failed or intermediate result is claimed as a final pass.

## Remaining acceptance and external ownership

PKI file materializer/P11 secret delivery and charon ordering remain unimplemented in this RPC path. Live SMTP/webhook delivery, browser behavior, OSPF/FRR adjacency, hardware/VPP forwarding, signed offline ISO closure/build and VM install/reinstall acceptance remain NOT RUN. Separate managers own actual VPP compilation, P10/P11 transport, management HTTPS lifecycle and SDK/CLI development; their worktrees and live processes were not modified. Broad plan/tasks.yaml remains outside this batch's ownership.

The connector exposes PR-triggered workflow runs only; no main-push CI verdict is claimed.
