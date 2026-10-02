# TEST-traffic-A inherited cleanup identity diagnosis

2026-10-02. Source snapshot bf59eca6, inherited test_foundation.py unchanged.
Independent reviewer, no source authorship. This is a concrete [other: R1/R2]
test-strength finding, not proof of broken source process-group cleanup or a
waiver of tests. Historical fda source remains immutable; correction belongs a
new source phase. Do not call these assertions sufficient local descendant proof.

Actual bounded diagnostic executed only the existing descendant-timeout and
noisy-producer cases with Path.read_text recording and os.kill patched to raise
if any child-reported numeric PID would be signalled. Both cases PASS2.225s.
Recorded child identity: child=7 birth4163657. Actual inspected outer /proc/7/stat:
7 (sites-preview), state S, birth148. Noisy child reports8 birth4163676; outer
/proc8 unavailable. Thus inner os.getpid and mounted /proc address space differ.

The descendant assertion checks same birth only AFTER cleanup and succeeds on
mismatch; noisy assertion similarly allows absent/reused identity. Neither first
positively proves announced PID/birth is the actual child in mounted /proc.
They can pass on unrelated/missing outer PID irrespective of real child's state.
This is a real local proof weakness, not an observed surviving descendant.
Group cleanup source remains unconditional own-group TERM/KILL; no product defect
inferred, no arbitrary reportedPID signal sent during diagnosis.

Required new-phase correction: announce mounted /proc/self/stat PID and birth
only after SIG_IGN readiness; publish closed/atomic identity; reviewer/parent must
positively match actual mounted PID/birth and live state BEFORE cleanup, then
verify same identity gone/Z or legitimately reused AFTER cleanup. No direct
reportedPID signalling. For noisy case preserve exact byte cap and bounded own
process-group execution while establishing readiness/actual identity before
starting overflow. Execute meaningful regressions in this namespace environment
and preserve historical reports. Existing producer correction demonstrates this
pattern without rewriting old foundation refs. Manager owns assignment/integration.
