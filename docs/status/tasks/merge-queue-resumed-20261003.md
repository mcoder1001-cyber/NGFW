# Resumed merge queue completed — 2026-10-03

Seven PRs were merged sequentially after independent source review, successful full hosted quick gates and verification against the current main composition. The active source workspace is clean at ef4fe22492e035eb60b5ffbcfb508ff170d24e68.

| PR | Change | Reviewed head | Successful full hosted quick gate | Main merge |
| --- | --- | --- | --- | --- |
| [120](https://github.com/mcoder1001-cyber/NGFW/pull/120) | Seal test validation after owned descendant cleanup | 6041b1ab | 37121830337 | f469d56e |
| [118](https://github.com/mcoder1001-cyber/NGFW/pull/118) | Respect inherited CPU affinity during VPP builds | 301998cf | 37121698391 | b2c214b5 |
| [123](https://github.com/mcoder1001-cyber/NGFW/pull/123) | Return truthful unavailable PKI file state | 56dadacf | 37125083465 | 78d187a4 |
| [122](https://github.com/mcoder1001-cyber/NGFW/pull/122) | Preserve console credentials for uncertain or positive counts | a784561a | 37125285146 | f3d10efb |
| [124](https://github.com/mcoder1001-cyber/NGFW/pull/124) | Classify PKI expiry using the precise validity timestamp | 13d87fd1 | 37125606956 | f9bc4a4e |
| [125](https://github.com/mcoder1001-cyber/NGFW/pull/125) | Reject malformed or failed installer size inventory | ec7a7cc2 | 37125833896 | dcaab032 |
| [127](https://github.com/mcoder1001-cyber/NGFW/pull/127) | Expose bounded authenticated read-only OSPF neighbors | 3251c614 | 37126886202 | ef4fe224 |

Reviewed heads remain available under archive/*-reviewed-root-20261003. The OSPF reviewed source and final contract marker are separately preserved under archive/ospf-state-reviewed-root-20261003 and archive/ospf-state-contract-reviewed-root-20261003. No branch was force-pushed.

## Final composition validation

Frozen root source checkpoint 499deec3b9e64276a662d08a84a80c10f7f63d31 and final main ef4fe22492e035eb60b5ffbcfb508ff170d24e68 have the exact same Git tree: e67e3c63d529b2663b6a1a69c61d7be255fc99a1. GitHub PR127 tested merge bb983a4eede6de2aebcee3e91102eeb4e131e904 has that same tree as well.

Finite heavy job 83c77500fa134acf8b60b0c6ed057d4b passed the unchanged repository quick gate with a 1500-second bound. Gate duration: 18 minutes 16 seconds. Actual checks included generation stability, contract and secret guards, all 35 Turbo tasks, agent race tests/lint/build, CLI checks, every test Go module and the VPP apply-startup fake-host harness. The final clean-head seal passed after owned process cleanup; the frozen tree remained clean.

Additional focused checks passed: pipeline 11 cases; VPP verification and Debian packaging fixtures; ISO aggregate 70 cases, then 71 with the size guard; PKI RPC race/lint/build with temporary output cleanup; PKI expiry/DTO 11 cases plus typecheck/lint; OSPF 21 cases, API/app.module lint/typecheck, client compile checks and generated CLI API tests. Hosted offline ISO fixtures 37125285293 and 37125834019 succeeded on the final console and size source heads. Packaging and provisioning fixture gates for PR118 also succeeded.

The connector exposes PR-triggered workflow runs only; no main-push CI verdict is inferred. The PR127 hosted full gate and the independently validated root composition establish the exact final source-tree result.

## Independent review and coordination

Four work streams consisted of root integration/test ownership and three developers using isolated feature/review worktrees. Root took over finite tests and sequential merges while developers moved to successors and independent reviews. The reserved short fixture lane and bounded heavy lanes allowed independent checks to proceed without holding developer worktrees. Only owned test process groups were cleaned up.

Fourteen new independent review documents accompany the ten earlier reviews and prior queue completion record in this evidence branch. OSPF review 569e83d9 requested actual FRR mapping corrections; compatibility and final reviews 55214641 and 2176e066a approve the fixes. The intermediate source-only error-test approval 0299bbfb did not claim a test pass; final author d684e186 replaces the failing mocked rejection fixture with an asynchronous adapter and real HTTP503 problem-filter regression. Final 21-case validation passed, and a2535d74/2176e066a approve that final implementation and generated contracts.

Publication consolidation omitted the OSPF contract commit marker, so hosted run 37126512312 correctly rejected the history guard. Forward commit 3251c614 restores the already reviewed marker without changing the source tree; its full hosted run 37126886202 succeeded. Failed or intermediate results were never used as final merge approvals.

Other managers retain ownership of actual VPP compilation, P10/P11 transport, management HTTPS lifecycle, namespace migration and separate SDK/CLI development. Their source worktrees and live processes were not modified. The broad plan/tasks.yaml board remains outside this batch's ownership.

## Remaining acceptance

This completes the source batch, not appliance acceptance. PKI file materialization/P11 secret delivery and charon ordering remain unimplemented in the new RPC path. Live SMTP/webhook delivery, browser behavior, OSPF/FRR adjacency, hardware/VPP forwarding, signed offline ISO closure/build and VM install/reinstall acceptance remain NOT RUN. Integration tests skip in the quick gate because VRX_INTEGRATION is unset; no live forwarding or release-ready claim is made.
