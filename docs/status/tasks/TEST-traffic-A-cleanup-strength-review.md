# TEST-traffic-A inherited cleanup assertion closure

2026-10-02. **APPROVE R1/R2/R4/R5/R7/R8 as applicable** bounded test/document
phase exact ec7468c36b9fe905bdd2a7e780de95ef512eb783. Own isolated
NGFW-traffic-cleanup-strength-review/task/traffic-cleanup-strength-review.
Only this report authored; no product/test/CI or arbiter authorship.
No BLOCKER/MAJOR/MINOR found. Prior d038 diagnostic is preserved, not rewritten.

R1/R2: both affected cases now announce mounted /proc/self/stat PID and birth
AFTER SIG_IGN in closed atomic JSON. Patched actual Popen wrapper positively
asserts same mounted PID/birth and S/R state BEFORE handing the genuine process
to production execution. Postcleanup check permits gone/Z/reused only after
that real-child proof, closing mismatch-alone false PASS. No direct announced
numeric PID signals; private created Popen groups alone are cleaned. Wrong-birth
negative readiness controls fail before production pump as intended.

R4/R5: production commands/producer/transaction/evidence/run/scenario diff to
31e63d9 is empty. Only assertion/helpers change: exact timeout0.15s and noisy
4096-byte private0600 output/content, resource/child cleanup checks preserved.
Callback readiness remains bounded5s. No VPP API/YANG/caps/global/ownership or
live activation introduced. Atomic publication and robust comm/state parsing
work in actual mixed PID namespace environment. No performance claims.

R7/R8: independently AST-compared ordered test_* names/count against base31e:
identical, total source47 unchanged. Foundation named16 compatibility is not
silently reduced. Envelope explicitly distinguishes historical counts from
retroactive child proof, scopes new source phase and requires fresh hosted gate.
Only three source/doc paths changed; diagnostic retained. No CI weakening or
live authority/whole-task proof claim. This does not grade unrelated authored CI.

Actual independent results:
- Five repetitions each affected case, os.kill patched to forbid reported numeric
 PID signalling:10 PASS11.188s, no skips.
- Full strict source47: PASS4.160s, failures/errors/skips/xfail/xpass all zero.
- Two additional genuine affected-case executions with forged wrong birth:
 each yields expected readiness assertion failure before pump; rejection controls
 PASS. Private created groups cleaned in finally; no reported PID signalled.
- Ordered method identity and empty production-source diff PASS; whitespace clean.

No actual SSH/network/VPP/nft/ip/tcpdump/livelease/host lock or installation
executed. Test-strength diagnosis is closed with meaningful actual child evidence;
this is not a retrospective claim that old local identities were proven, nor a
product leak finding. All live authority, issuer/executor/config causality gaps
remain source work. Actual main composition, applicable panels, unchanged full
hosted quick and all phase gates on exact final head still required before merge.
