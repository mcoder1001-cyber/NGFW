# Network lifecycle independent review — 2026-10-03

Reviewer: codex-restart-socket (reviewer 3). Scoped approval: no actionable correctness finding in the reviewed LCP, V27 multicast, VRRP or GRE/IPIP default-MTU changes. This is a source/evidence review, not a merge or production deployment approval. The review deliberately excludes this reviewer's FRR implementation and routing fixture changes.

## Scope and conclusions

| Scope | Review conclusion |
| --- | --- |
| `descriptors/lcp/lcp.go`, `mfibguard.go`, leftover and netlink regressions | Cleanup removes only the API-source leftover LOCAL path when the observed entry is otherwise empty. Active outside-default pairs remain fenced; unrelated Accept paths are preserved. Read-after-write failures restore LOCAL and preserve both errors when restoration fails. Tracking Accept independently of the current default namespace preserves later deletion/restart metadata. Netlink ACK decoding and request validation are explicit. |
| `deploy/vpp/patches/0003-lcp-multicast-reconcile.patch` (V27) | Pair creation reconciles existing IPv4 addresses; completed address changes reconcile actual remaining pool membership. Pair deletion removes only the physical interface's plugin-owned inputs. IPv6 delegates follow first enable/final disable, preserving link-local acceptance after last global-address removal. Formatter arguments and delegate removal match pinned VPP interfaces. There is no higher-priority API-source replacement of plugin-low multicast state. |
| VRRP descriptor, core address classification, desired metadata and subsystem/keepalived wiring | Only owned accept-mode Master VIPs are filtered. Backup/foreign VRs and classification errors remain visible. Presentation order is restored only for an equal live address multiset. A vanished interface releases its qualified previous claim without deleting an unidentified VPP index; unresolved pool residue remains explicitly documented. Keepalived PID checks reject missing daemons before writing/reloading configuration. |
| `descriptors/interface/attributes.go` | The 9000 creation default is limited to exact IPIP/GRE device classes (or the existing no-link case). Hardware LinkMtu values 65516/65512 do not turn custom MTU 1500 or nondefault family vectors into defaults. Other device classes retain their existing behavior. |

Pinned `/root/vpp/src/vnet/ip/ip4_forward.c` confirms IPv4 address callbacks run after the successful address-pool mutation. `/root/vpp/src/vnet/ip/ip6_link.c` confirms delegate register/update/remove and formatter conventions used by V27. The VRRP classifier uses the normal global persisted interface-claim store through dfkit; its default Base options do not bypass ownership.

## Evidence provenance and restart behavior

Previously recorded hardware evidence was inspected, not independently rerun against the shared system:

- [LCP acceptance](TD-lcp-leftover-local-path.md): disposable slot 5, table 5224, lone API LOCAL cleanup, mixed-table ownership fence and rollback race regressions. Cleanup depends on observable MFIB best-source state; it intentionally preserves ambiguous unrelated Accept paths. The preexisting cross-agent check/create race is not represented as atomic.
- [V27 lifecycle](V27-lcp-multicast-reconcile-2026-10-03.md) and [patched lifecycle output](lcp-multicast-2026-10-03-evidence/patched-lifecycle.log): stock failure followed by private-plugin success for pair recreation, IPv4 multiple/last-address changes, IPv6 LL-only recreation, real netlink last-global deletion, final disable and preservation of the other pair. Registered source hashes independently checked successfully during this review.
- [Strict routing acceptance](F-isis-rip-host-2026-10-03.md): 154.20-second final private-plugin run proves post-agent-restart learning, withdrawal/reannouncement and rollback. This hardware run was executed earlier by this reviewer; it is supporting restart evidence, not independent approval of the reviewer's FRR code.
- [VRRP product evidence](S-vrrp-product-fixes.md): private slot 5 demonstrates real Master VIP filtering, unchanged reconciliation, disable, vanished-interface cleanup and actual HTTP `/api/v1/state/drift` returning an empty change list. The raw orphan VR remains visible to administrators; success does not claim removal of that unsafe-to-identify pool residue.
- [Tunnel baseline](F-tunnels-host-2026-10-03.md): actual GRE omitted-MTU API commit/retrieve yields empty drift. This earlier reviewer-run baseline does not certify all tunnel kinds or restart combinations; those are a separate acceptance task.

Shared VPP was not restarted/reconfigured by this review; slot 6 was not used. V27 packaging/install rollout and an actual merge remain separate work.

Independent focused race checks passed for LCP, VRRP, core VIP classification, desired VRRP ordering, subsystem wiring and keepalived checks. A separate explicit interface run passed MTU/IPIP/GRE creation-default and VPP-restart memory tests (the first selection did not match interface tests). Both commands used `tools/heavy.sh`, race detection and `-count=1`; [full output](review-network-lifecycle-2026-10-03-evidence/focused-race.txt) records the actual selection and results. Privileged hardware tests were not rerun during this review.
