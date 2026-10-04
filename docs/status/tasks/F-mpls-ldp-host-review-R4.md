# F-mpls-ldp-host — R4 dataplane/shared-host review

Reviewed product source `99218b6d3dbf34afbf487cfaeeef43128e87d536` and final documentation head `9d871721305f2277d123a858e221c30eae2aafea` against `06e4368c`. Independent reviewer; no product edits.

Initial BLOCKER findings resolved in this source: failed apply now leaves the cache dirty for retry of an unchanged snapshot; static IP binding refuses an active LDP label; acceptance driver can explicitly read the actual production table zero, with documented shared lab/globals lock invocation. No remaining BLOCKER or MAJOR within R4's source safety aspect.

Verified generated MPLS APIs are reused and binapi is untouched. `mpls-route.ldp` uses distinct persistent boot ownership keys; static route Retrieve excludes dynamic records, static Create/Update rejects takeover, static IPBind bind rejects dynamic ownership, named Create refuses unowned existing entries, and Delete/Retrieve are boot-scoped. Reconstructed named descriptors recover the same current-boot owned state. The poller only changes cached desired state and enters the S1 scheduler; prospective LCP identity/configuration filters remove stale paths on remapping, disable or interface removal. Successful empty state withdraws, failed reads retain forwarding for the documented hold-down and then withdraw, and transient scheduler failures retry. Production uses table zero; the internal test seam can use assigned slot tables. No daemon start/unit, kernel module, sysctl, VPP restart, packet trace, generated API or startup configuration mutation introduced. The acceptance probe is read-only and slot/label validated; table-zero access requires explicit opt-in and the documented shared locks.

Independent exact-product source commands in `ldp/apps/agent`:

```text
/workspace/scratch/e4f791ef53f7/go/bin/go test -race -count=1 ./internal/frrsync/ldp ./internal/descriptors/mpls
ok ngfw/agent/internal/frrsync/ldp 1.360s
ok ngfw/agent/internal/descriptors/mpls 1.065s
/workspace/scratch/e4f791ef53f7/go/bin/go test -race -count=1 ./internal/subsystems -run Ldp
ok ngfw/agent/internal/subsystems 1.140s
```

`python3 -m py_compile test/topology/mpls-ldp/verify.py` exited 0. No real FRR/VPP packet, session, daemon restart, withdrawal or simulated dataplane-loss acceptance ran here. Those lab acceptance items remain explicitly deferred under the owner policy. Implemented LFIB is EOS IPv4 only; NEOS/explicit-null are not claimed supported. Bounded full-RIB polling and per-label dump scale are manager/other-reviewer concerns, with the documented 256-route supported bound and follow-up debt; no throughput claim is made.

Verdict: **APPROVE** (source review; lab acceptance deferred).

Final delta review: `cb2560ed81d56c2d74f0a35075e4a5e40c0ad5a1` safely filters cached labels outside the prospective configured range; the 256-route cap preserves prior ownership/error handling. State count now uses the named descriptor's bounded ownership-guarded Retrieve and returns unavailable on failure, without dataplane writes. Independent final scoped LDP subsystem race verification PASS (recorded in R8 report). Verdict remains **APPROVE**.
