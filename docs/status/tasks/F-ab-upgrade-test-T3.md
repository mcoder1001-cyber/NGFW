# Independent T3 F-ab-upgrade

Exact SHA be1448118afd246f39b0d6a7db01d2f986448482; 2026-10-05; branch codex/review-r4-seven-20261005. Own reports only; reviewer-owned git archive snapshot; no product edits or shared host/VPP mutation. Task envelope: independent R4/T3, filesystem fixtures only; no real kills or shared services/config changes.

Actual reviewer command: `tools/heavy.sh .review-fixtures/ab/deploy/upgrade/tests/run.sh`
```
Ran 17 tests in 4.174s
OK
```

Actual additional command: `tools/heavy.sh unshare --mount --propagation private env NGFW_INTEGRATION=1 python3 .review-fixtures/ab/deploy/upgrade/tests/loop_image.py` (exit 0):
```
PASS: unsigned and one-byte-tampered bundles refused; inactive device unchanged
PASS: stage/fstab/identity/manifest -> one-shot B -> confirm B; failed health -> default A
PASS: host lsblk/grubenv unchanged; no owned loop devices remain
```
Host grubenv before/after SHA256 f64122858064885ef0733e42c6a3d2d3fd642671f714db0d974b880c0f087430; full log is reviewer-owned .review-fixtures/ab-loop.log. No firmware boot.

Slot none: filesystem-only/private loop fixture; no shared dataplane objects.

| Scenario | Expected | Observed | Result |
|---|---|---|---|
| 17 signed/lifecycle/health tests plus private-mount disposable loop image | successful safe fixture run | output above, exit 0 | PASS |
| Actual appliance runtime/firmware | live acceptance | unavailable, not executed | DEFERRED |

Verdict: PASS for executed host-independent/disposable-disk scope; deferred appliance execution is not a runtime PASS.
