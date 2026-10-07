# Frozen PPPoE R8 operability review

Exact source: `4bcf9f4977e2a9c248548de23cf552e4bcef94da`, tree `7d914836662eb129ada7ac33459b97d4ce449b28`.

**BLOCKER R8-F1:** R4-F1's stop/remove/restart admission gap prevents a verified no-writer transition; hook action locks end before Go file editing. Fix and deterministic transition/reconnect/rollback regressions are specified in R4 report. This also affects safe recovery after failed application.

**MAJOR R8-F2:** R4-F2's unpinned parent PID can report a dead pppd session as up after reuse. Parent absence alone is insufficient lifecycle evidence; preserve and monitor original identity.

Resolved: `deploy/debian/ngfw/debian/control:12` now declares `dhcpcd-base` and `python3`; OS package docs list both. No host package installation performed. Setup/client exit/observation failures use fixed sanitized error codes and ReadSessionState exposes LastError; focused tests exercise missing/invalid executable, sysctl/state-write failures and child exit. Full-disk state-writing cannot persist its own failure, but helper returns nonzero rather than success. Actual late DHCP event is rejected after revocation, TERM-resistant fake child is killed and exit verified. Tests use private sysctl/ip/dhcpcd substitutes and a recording systemctl; they do not establish real service/package installation or end-to-end rollback acceptance.

Verdict: BLOCK.
