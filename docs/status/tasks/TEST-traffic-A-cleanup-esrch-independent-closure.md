# TEST-traffic-A final ESRCH closure — independent review

2026-10-02. **APPROVE R1/R2/R4/R5/R7/R8 as applicable** exact
1016e962c8df8c8ffafa256358952aa92ca651fc. Own isolated
NGFW-traffic-cleanup-esrch-review/task/traffic-cleanup-esrch-review.
Only report authored; no product/test/CI or arbiter authorship.

Actual independent execution: separate controls2 PASS0.003s; both affected
cleanup cases five repetitions each10 PASS11.168s with os.kill patched to forbid
reported numeric PID signalling; full strict47 PASS4.042s, failure/error/skip/
xfail/xpass counts all zero. No new failure requiring A3 reassignment occurred.
Historical failed9PASS1ERROR remains failure, not re-labelled by this closure;
separate later47 did not previously close it. Binding A3 is preserved.

Correction only broadens POSTcleanup verified-child stat disappearance from
FileNotFoundError to explicit ProcessLookupError (ESRCH). Positive mounted PID/
birth/liveS/R BEFOREcleanup is unchanged. Readiness ESRCH still fails. No generic
OSError or permissions suppression: actual controlled EACCES/EPERM/EIO/EINVAL
postcleanup errors propagate, ESRCH/permissions/I/O readiness errors propagate,
wrongPID/birth/Z readiness fail, valid comm-with-spaces identity succeeds.
After positive identity, explicit process gone is legitimate nonrunning evidence.
Direct reported numeric PID signals remain prohibited; only owned groups cleaned.

Independent source/gate identity diff from failedf717: production commands,
producer/transaction/evidence/run/scenario and all.github gates unchanged.
Original test methods/counts/timings/caps unchanged; separate2 control tests are
outside47 discovery and not inflated source acceptance counts. Prior reports/
failed evidence/ruling remain preserved; new envelope accurately attributes author
results and requires independent closure/exact hosted gates. Whitespace clean.
No capability/VPPAPI/YANG/globalownership/live activation or new sourcefunction.

This closes only bounded intermittent postcleanup ESRCH failure under the eligible
A3 condition; not a new ruling, live authority proof or whole-task DONE. Fresh
actual-main composition and immutable integration preserving predecessors, all
mandatory panels and unchanged hosted quick/all phase gates on exact final head
remain required. No real SSH/network/VPP/nft/ip/tcpdump/livelease/host locks or
installation operations occurred. No arbitrary child-reported PID was signalled.
