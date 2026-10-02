# TEST-traffic-A source foundation — independent R1/R2/R5

Frozen `bd443810dd048ef767c4bba0e4b7153b2a0d907b`, isolated task/traffic-foundation-review. Prior descendant-leak BLOCK preserved in historical checkpoint/report. This review covers support foundation, not implemented composed traffic executors or full TEST-traffic-A completion.

**APPROVE the corrected source foundation. Prior process-descendant finding closed. No new MAJOR/BLOCKER for this explicitly inactive scope.**

- run_command creates a fresh0600 exclusive log and own process session, uses argv without shell, bounds execution timeout<=1200, and unconditionally sends TERM then KILL to its own created group even if leader already exited. Reaps its leader; no pkill/global process matching. The regression proves child SIGTERM-ignore readiness first and compares kernel birth identity before cleanup, preventing false pass due to startup timing and fixture PID reuse. Zombie/nonexecuting child is distinguished from a running descendant.
- Live CLI refuses require_implemented before lease read, lock or external commands. Reserved12/13/invalid slots, cross-slot environment, globals-owner/all-ID options and unsafe retained-state options are rejected. Manager lease requires fixed slot path, root regular0600, <=4096 bytes, matching boot/task/slot/run identity and bounded expiry. No live lease or global host mutation exercised.
- Capture validators require exact slot/run path for live evidence, namespace/device/fixed tcpdump argv, origin marker, finite bounded interval, zero reported loss, private regular bounded16MiB file and matching SHA256. Classic Ethernet pcap parser supports bounded IPv4 TCP/ICMP and<=2VLAN tags, rejects truncation, fragments, unsupported framing and nonmonotonic timestamps. This is structural parsing, not checksum/sequence/correlation proof; unsupported packets do not imply outcome success.
- Go JSON validator requires exactly selected test execution+pass and package pass, rejects fail/skip/foreign tests/packages/malformed input; bounded16MiB/100000lines. Strict fixture runner rejects zero tests, skipped/expected/unexpected success statuses. Neither suite exit nor matching textual log can become whole-chain proof.
- Evidence and stage contract responses always keep whole_chain_proven/packet_outcomes_proven false. Seven composed stage executors, candidate/commit lifecycle, capture generation, packet correlation and forwarding/drop assertions remain NOTIMPLEMENTED, not laboratory-only deferred functionality. Do not mark whole task complete based on this foundation.

MINOR for subsequent live implementation: subprocess stdout log currently has timeout but no explicit byte cap; before using it for lengthy actual support suites, stream/cap output and propagate a loud failure rather than loading an arbitrarily large file and only then applying16MiB JSON limits. Likewise fixed run directories/lease parent identity must be created/protected by the future manager lifecycle before enabling current lstat/read helpers. No active live path currently bypasses these unfinished boundaries.

Personally executed:

```
python3 test/topology/traffic-a/check.py
Ran16 tests in0.405s — OK
failures0 errors0 skipped0 expectedFailures0 unexpectedSuccesses0

Actual SIGTERM-ignoring descendant regression repeated5 times:
Ran5 tests in1.063s — OK
```

The tests launch only controlled temporary Python children, private files and executable refusal stubs. No rig/SSH/VPP/nft/capture command or global process kill occurred. Source fixtures PASS; real composed traffic/packet acceptance NOT RUN and code not yet implemented.
