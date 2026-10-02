# TEST-traffic-A fixture producer — independent R1 / R2 / R4 / R5

2026-10-02. **APPROVE** bounded inactive source phase at exact
`4c94484bc644e28291558ae2fe2c506df96cd9f9`. Own branch
 task/traffic-producer-source-review; isolated worktree
NGFW-traffic-producer-source-review. Only this report is authored by reviewer.
No BLOCKER or MAJOR; one optional MINOR coverage-description finding below.

R1: produce() validates stage/run/slot/time, refuses non-fixture and default real
executor before directory/output/process operations, then creates its private
output relative to an opened directory descriptor. Separate binary/accounting
streams are drained with bounded select/read/write. Exactly one captured and
received accounting line, exactly one zero-drop line, parsed record equality,
received>=captured and parsed timestamps in producer interval are required before
metadata returns. Hash and read_pcap consume the same bytes read from its created
FD. Errors cannot return FIXTURE_CAPTURED. Refused partial files remain private
and no existing output is overwritten. Two-side synchronization and transaction/
config/counter linkage remain absent, without a PASS claim.

R2/R4: fixed tcpdump argv encodes allocated namespace/device, explicit ICMP-or-TCP
filter and -w -, with no shell or external input executable selection. Injected
fixture executor is mandatory; default ip/tcpdump executor cannot be selected by
fixture entry. Checked owned0700 opened directory, NOFOLLOW directory/exclusive
NOFOLLOW0600 file, close-on-exec and existing-file preservation. The contract
explicitly does not attest parent namespace/device ownership or manager lease.
Live remains refused; run.py unchanged/inactive. New evidence acceptance permits
only original exact file argv or exact piped argv; path-index12 is correctly -w
argument. Owner/mode/FIFO fixture now reaches NONBLOCK/fstat refusal. No Go/YANG,
VPP API, agent capabilities or shared/global ownership change. Results always
retain false whole-chain, packet-outcome and live-provenance proof flags.

R5: stdout <=16MiB, stderr <=65536 bytes, chunk65536 and monotonic <=1200s deadline;
private file never exceeds pcap cap. Parser retains2048-record/two-VLAN limits.
Cleanup targets verified child-own process group, TERM then KILL even if leader
has exited, and wait/reap; never global PID selection. No performance or live
capture results claimed. Metadata-labelled provenance remains consistency only.

## Optional MINOR

`test_producer.py:test_timeout_kills_own_ignored_term_descendant_after_leader_exit`
keeps the leader sleeping30s in its actual source; readiness correctly confirms
SIG_IGN child, but the title overstates prior-leader-exit coverage. Rename to
reflect timeout cleanup or later add a distinct already-exited-leader regression.
This does not invalidate the real timeout descendant assertion. Source cleanup
is structurally unconditional on leader exit; existing foundation regressions and
historical closure remain preserved. No source fix required for this source-only
approval.

## Actual execution and limits

Personally ran `python3 test/topology/traffic-a/check.py` in exact frozen worktree:
39 tests in3.569s, PASS; tests=39 failures=0 errors=0 skipped=0
expectedFailures=0 unexpectedSuccesses=0. Meaningful new cases use actual temporary
Python subprocesses producing correctly checksummed PCAP and stderr accounting;
fixed argv/environment, private-mode metadata replay, live/default fixture refusal,
directory symlink/owner/mode/existing-file preservation, loss/missing/duplicate/
mismatched counts, separate stdout/stderr overflow and ready ignored-TERM child
cleanup are exercised. Previous integrity/checksum/correlation tests remain run.
Supplemental reviewer-only prior-leader-exit probes were not valid: one explicitly
reaped leader before ownership check (ProcessLookupError), others failed their own
readiness precondition. They are not reported PASS or treated as product failures;
cleanup of their self-created processes was attempted explicitly. No new product
or test source was written from those exploratory probes.

No actual network, ip/tcpdump, SSH, VPP/nft, real lease/namespace, capture or host
mutation occurred. Approval is not whole producer DONE, live provenance approval,
whole TEST-traffic-A completion or hosted CI. Final current-main composition and
unchanged full hosted quick/strict suite remain required. Genuine live ownership,
composed executor/synchronization and causal linkage gaps require source work;
only eventual actual laboratory execution is NOT RUN/deferred acceptance.
