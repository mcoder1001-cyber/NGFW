# TEST-traffic-A bounded capture-producer continuation

Owner: isolated correlation branch/worktree. Prior approved capture integrity
source d37 is frozen and preserved. Scope: source adapter and offline stubs only.

Fixed per-side argv uses slot namespace/device and tcpdump ICMP-or-TCP filter,
`-w -` so packet bytes pass through a byte-limited pipe before any disk write.
A separate bounded stderr stream supplies strict tcpdump received/drop accounting.
Created process-group cleanup must happen on success, timeout and byte overflow.
Files use exclusive0600 directory-relative creation in a manager-owned0700 run
root; no directory, namespace, lease or slot is created/claimed by the adapter.

The existing lease attests task/slot/boot, not actual namespace/device ownership.
Thus live production remains NOTIMPLEMENTED until a reviewed manager namespace
ownership/transaction contract exists. The fixture-only entry models private
ownership with the invoking uid and explicit stub executor; it does not mint a
live lease or claim genuine capture. Both streams/composed probe synchronization
and config/counter causality remain actual source gaps. No run.py activation.

Tests: real temporary executable stdout pcap/stderr counters, explicit argv,
zero-loss/unknown accounting refusal, byte limits, timeout/own-group cleanup,
symlink/mode/owner/existing-output refusal, digest/parse and capture metadata.
No actual ip/netns/tcpdump/SSH/VPP/nft/host actions.

Initial implementation checkpoint is UNTESTED WIP, not reviewed completion.
The root descriptor is ownership/mode-checked, output exclusive0600, stdout and
stderr byte-bounded, and only the created process group is cleaned. Parsing,
zero-loss accounting and timestamps feed source_fixture metadata. Live fails
before any host/file/process operation because the ownership contract is absent.
No formal test/review executed pending manager's fresh arbitration/split ruling.
Compile-only syntax check completed; substantive fixture tests still required.

Fresh A3 ruling f7e7f04d executed: new isolated NGFW-traffic-a-producer worktree,
branch task/TEST-traffic-A-producer-20261002, ancestry f5 WIP preserved. Approved
correlation branch remains untouched. Formal round1 now covers only this phase.

Implemented fixture producer returns FIXTURE_CAPTURED with all proof flags false.
Its fixed piped argv now validates in evidence acquisition. Source stubs create
correctly checksummed packet bytes at execution time and received/drop counters.
Real success, loss/unknown/duplicate counts, stdout/stderr bounds, timeout with
ready ignored-TERM descendant, directory symlink/uid/mode and existing output
refusal are exercised without real ip/tcpdump. Earlier adversarial FIFO fixture
had the wrong argv path index and refused before opening FIFO; corrected it to
exercise NONBLOCK/fstat rejection as intended. Initial suite exposed the same
piped-argv mismatch, now fixed. No assertion/gate was weakened.
Live manager lease/namespace/device ownership and composed transaction/capture
synchronization/config linkage still genuinely NOTIMPLEMENTED; live calls refuse
before creating output or invoking an executor. No whole-task completion.

Final actual strict suite:39PASS3.550s; zero failures/errors/skips/expected failures
or unexpected success. Unchanged sourced tools/ci.sh check --base origin/main
EXIT0 in2s; gitleaks122345bytes/no leaks; git diff --check clean. New producer
source phase needs independent review/full unchanged hosted gate before merge.

Test-only parent-exit coverage checkpoint: renamed the prior sleeping-parent case
accurately. New actual subprocess case confirms parent exit0 via Linux waitid
WNOWAIT/WNOHANG without poll/wait/reaping before producer starts; its ready child
ignores TERM and inherits both pipes. Producer times out, reaps the leader and
kills the created group. Child PID plus kernel start-time identity is checked as
gone/Z (never signal/check a recycled process as the same child); final waitid
confirms leader is already reaped. Unsupported waitid fails rather than skips or
claims PASS. Product code unchanged. Independent first producer approval report
4dceefd7 preserved unchanged. Actual strict40PASS5.030s, zero failures/errors/
skips/expected failures/unexpected success. Sourced unchanged check EXIT0 in2s,
gitleaks135656bytes/no leaks; diff check clean. Test delta needs independent R2.
Live ownership/lease/config linkage still NOTIMPLEMENTED; all proof flags false.
