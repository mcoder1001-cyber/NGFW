# Merge disposition audit — 2026-10-03

Read-only product/plan audit by reviewer 3. Seven reviewed rows are eligible to become `merged` **after their actual integration commit is merged**. Native IPsec is eligible for integration of its verified PSK milestone, but the feature row must remain `running`. No board state was changed by this review.

## Exact dispositions

| Row | Disposition after successful merge | Evidence and scope boundary |
| --- | --- | --- |
| F-system-identity-host | merged | [14.47s production API/agent acceptance](F-system-identity-host-2026-10-03.md): control-character/timezone refusal, exact goldens, restart/mtime invariants and unchanged host identity. T4 integrated screenshots remain deferred, as the prompt's final D-175 instructions explicitly require screenshots after merge. |
| F-isis-rip-host | merged | [154.20s strict acceptance](F-isis-rip-host-2026-10-03.md): live ISIS/RIP, real API invalid-circuit refusal, RIP FIB, agent restart, withdrawal/reannouncement and rollback. V27 private-plugin lifecycle and author-independent [FRR review](review-frr-2026-10-03.md) accompany it. OSI-punt-gated ISIS adjacency is explicitly gated by the row title, not claimed as tested. |
| S-cli-ipsec | merged | [Full CLI race/build acceptance](S-cli-ipsec-2026-10-03.md) and [native/CLI review](review-native-ipsec-2026-10-03.md): generated REST-only state, paging, exact SPI handling and authorization/unavailable failures. This completes CLI scope, not certificate authentication. |
| F-tunnels-host | merged | [Final API and direct-agent acceptance](F-tunnels-host-2026-10-03.md) covers GRE/IPIP/VXLAN exact Retrieve, duplicate400, unchanged no-op, exact owned loss, agent restart recovery, historical rollback and empty metadata. [Independent driver review](review-tunnels-2026-10-03.md) closed all three P2 findings. T4 screenshots remain deferred; no full VPP-process restart or throughput claim. |
| S-capture-retention-stop | merged | [Stable-source race/API/web/build evidence](S-capture-retention-stop-2026-10-03.md), [independent review](review-capture-2026-10-03.md): recoverable stop/filter ownership, newest retention, bounded plans, startup/read/delete recovery and UI stop behavior. Separate capture host acceptance/screenshots and capability follow-ups remain separate rows. |
| S-vrrp-product-fixes | merged | [Product/HTTP evidence](S-vrrp-product-fixes.md) and [independent lifecycle review](review-network-lifecycle-2026-10-03.md): actual Master VIP filtering/empty HTTP drift, order fidelity, disable and vanished-interface claim cleanup. Missing keepalived produces the requested nonprivileged finding; daemon-start privileges remain an explicit separate owner decision. Do not preserve stale notes saying HTTP drift is pending. |
| TD-lcp-leftover-local-path | merged | [Disposable live cleanup and race evidence](TD-lcp-leftover-local-path.md), [independent review](review-network-lifecycle-2026-10-03.md): lone LOCAL cleanup, ownership fence, stale-index/refusal fixes and read-failure restoration. Ambiguous foreign Accept paths are intentionally preserved. |
| F-ikev2-native | running; record merged PSK milestone in notes | [Native acceptance](F-ikev2-native.md), [API secret lifecycle](native-ipsec-api-2026-10-03.md), [review](review-native-ipsec-2026-10-03.md): packet/counter/actions, default-DPD recovery, active-SA agent restart and version-pinned PSK rollback. Certificate trust/key provisioning remains open; full VPP-process restart, IPv6 overlay and NAT traversal are unverified. No whole-feature completion claim. |

The unrelated eighth review row, `F-det44-cnat-fix`, retains its existing `review` state. It is outside this integration's audited completion set.

## Latest-main preservation audit

The shared local `origin/main` was stale at `29f9cae41bb1f1120918a85e92126c1aed49edff`. A read-only remote query and an isolated temporary bare-repository fetch independently obtained main **`6db19a51311e9fca238e2595207361264aea3393`**. No shared Git refs or worktrees were mutated. That remote plan has 156 rows; this integration's plan has 211. Every remote ID is present locally; none of the seven completion IDs is present in that remote plan. Therefore replacing the local board wholesale with the remote file would discard 55 rows and regress recorded work.

Use an ID-wise reconciliation, preserving unrelated notes/ownership and completed rows. In particular remote main still lists these locally completed rows as running, ready or parked: `F-det44-map-dslite-cnat`, `F-nat46`, `F-dataplane-ui`, `F-management-ui`, `P11`, `F-tunnels`, `F-ospf`, `F-isis-rip`, `F-vrrp-config-sync`, `P10`, `P12-fib-proof`, `F-lb-host`, `F-srv6-host`, `F-mpls-srmpls-host`, `F-rule-expiry-host`, `F-global-blocking-host`, `F-pppoe-client-host`, `F-multiwan-host`, `F-bruteforce-block-host` and `F-igmp-mfib-host`. Do not downgrade them solely to resolve YAML conflict.

`F-pki`, `F-notifications` and `P14` remain owned by the other active coordination work. Preserve that work's actual completion commits and latest notes if they arrive during rebase; this reviewer does not authorize changing their task states from the older local snapshot. Preserve other deliberate local dispositions, including `P11-pkg: todo` with its recorded D-223 hold, and the untouched DET44 review row. Do not reset native IPsec to remote `todo`.

## Merge gate boundary

[Integrated validation](review-and-progress-2026-10-03.md) records fresh compile CI, evidence secret scan and scoped runtime/race checks. Its uncommitted contract guard compared zero new commits; that limitation is explicitly disclosed. The isolated integration commit/rebase must receive actual commit-based contract checks and current-tree validation before merge. A rebased conflict resolution that changes executable code requires relevant checks again. Plugin package registration is mergeable source work; installation on the shared appliance is a separate deployment action and is not implied by any `merged` task state.

No slot or shared VPP resource was used by this disposition audit. No merge has been performed by this reviewer.
