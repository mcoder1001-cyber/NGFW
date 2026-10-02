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
