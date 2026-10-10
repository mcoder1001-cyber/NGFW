# Real Global Blocking host acceptance

All feasible real acceptance criteria passed on .195 and .250. Final source combines main ded860, independently reviewed delegation391 and empty-WAN observation fix1562c0b2a. Both hosts ran the same freshly compiled agent SHA2565683498457c6bb49a05c98951abefa806ee1219a2b2fd3ae4c0e3e9c2c1717fe. API/web source490 artifacts remain explicitly attributed; those product sources are unchanged by the combined Go fixes. Unchanged full local quick passes24m53s and hosted38048452085 passes on777. Final D112 integrates actual main4cda with an identical agent source tree; final exact hosted quick is pending before closure.

| Actual criterion | .195 | .250 |
| --- | --- | --- |
| Selected ingress/egress ICMP and exact TCP blocked | PASS | PASS |
| Unlisted and listed/unselected traffic passed | PASS | PASS |
| Selected VPP gateway local-in blocked; unselected passed | PASS | PASS |
| Same-list protectHost actual nft kernel input, counters, removal | PASS | PASS |
| Real preview line2 error, explicit import/commit, API counters | 0→12 packets | 0→10 packets |
| Normal authenticated English/Persian UI, no page errors | PASS | PASS |
| Real 10k commit | 2.781s | 2.999s |
| Real 200k commit | 32.911s | 29.943s |
| Real one-entry API commit | 69.581s | 53.462s |
| Exact incremental plan | 5 updates, 0 creates/deletes, no binding change | Same |
| Original revision rollback, private cleanup, shared identity | PASS | PASS |

One-entry times are complete HTTP transactions, not production phase deadlines. The production DryRun30s and Apply60s budgets are unchanged. The original repeating resync/504 and subsequent502 failures are retained. Empty learned-gateway observations previously expired during a slow full reconciliation and requested another full reconciliation. Reviewed156 only suppresses expiry-triggered reconciliation when both old/current valid lease maps are empty; configuration changes and real lease expiry/withdrawal retain their behavior. Focused regressions and the whole multiwan race suite passed; the new empty-observation regression fails against the original source.

Each 0/10k/200k lookup window resets only its independently verified private VPP runtime, uses exactly2000 paced packets at0.01s, and requires2000/2000 received and at least2000 actual ACL node vectors for nonempty lists. Zero-entry ACL feature cost is N/A, with an actual ip4-lookup reference. Final clocks/vector: .19510k11300/200k18100; .25010k15100/200k15400. These are bounded runtime windows, not throughput claims. Original .250 unlimited flood loss1992/2000 remains recorded.

Current real nftables renderer tests also passed on both hosts. .195200k set apply21.751s/retrieve6.480s/one-entry30.140s; .25020.838s/6.129s/26.939s. Same proof list with protectHosttrue blocked real local TCP/ping, incremented real counters, allowed the unlisted control and restored the listed flow on removal. Root nft tables remained unchanged.

[Final .195 receipts](lab-global-blocking-20261010-evidence/195-final/live.txt), [final .250 receipts](lab-global-blocking-20261010-evidence/250/final-live.txt), [remote actual UI](lab-global-blocking-20261010-evidence/250/browser.txt), [.250 host nft](lab-global-blocking-20261010-evidence/250/host-nft.txt). Screenshots and raw runtime/packet/plan receipts accompany each host's evidence directory.

An earlier .195 run completed the one-entry commit but its owned agent was killed by the kernel OOM killer before rollback. That run is incomplete and preserved. Only the final .195 rerun after reclaiming inactive owned scratch cache supplies complete rollback/cleanup proof; no memory or deadline waiver was used. The first local quick failed the pnpm12 safety check because the owned dependency directory was a symlink. The retry uses a real directory backed by an isolated dependency overlay with read-only lower/owned upper; no product checks changed.
