# TEST-traffic-A correlation continuation — independent security/source round2

Frozen `d37df610cfabc7cd475cb5da89304d78f84aaa7f`, remote checkpoint reported `dac9fc20e0ecf28d77f54ae9b6f0bbc762536165`, isolated task/traffic-correlation-security-review. Reviewer did not author production/source code. This review covers new capture acquisition/checksum continuation, not previous foundation arbitration or full traffic task.

**APPROVE this bounded source continuation (R1/R2/R5). No new BLOCKER/MAJOR identified.**

- Capture acquisition opens exactly one descriptor with NOFOLLOW/CLOEXEC/NONBLOCK, fstats regular0600/current-fixture-owner or root-live owner, bounds initial size24..16MiB, then bounded os.read chunks. Growth beyond limit rejects; final size/mtime/ctime changes reject. Identical acquired byte buffer is hashed and parsed, eliminating independent path/hash/parser reopen. FD closes in finally. Root private parent/run directory authority still belongs to future executor, not proven by final-component NOFOLLOW alone.
- Correlation compares dev/inode identities from actual acquired FDs, not pre-open path stat, and rejects LAN/WAN aliases. Exact run/slot namespace/device/argv/origin and stage metadata remain required. File replacement cannot change already opened bytes; concurrent content mutation is refused. No arbitrary network path or downloaded input is executed.
- IPv4 header, TCP pseudoheader+transport and ICMP one's-complement checksums now validate before Packet creation. Invalid/offload-ambiguous checksums produce Indeterminate(INDETERMINATE), not match/proof. Unsupported Ethernet/transport records do likewise. Fragments/truncation/framing remain fail-closed Refused; do not inaccurately say every malformed framing case has the Indeterminate subclass. No skipped malformed packet can manufacture a matched/drop outcome.
- Packet extraction bounds classic pcap size/snaplen, timestamps/record lengths, TCP header length and VLAN depth; allocation/checksum workload is bounded by the16MiB stream and<=65535-byte frames. Correlation<=64 probes remains a pure typed expectation matcher, checking ingress existence, exact identities/payload/timing/flags/TTL/VLAN/remote NAT semantics and distinct next-hop outcomes. SHA metadata equality alone is never asserted as cryptographic executor provenance.
- Successful results still set whole_chain_proven, packet_outcomes_proven and live_provenance_verified false. Composed candidate/commit/retrieve/rollback executor and actual capture producer remain NOTIMPLEMENTED; this is a real code gap, not laboratory-only deferred acceptance. Current live entry still refuses before host work.

Personally ran exact frozen strict fixture gate:

```
python3 test/topology/traffic-a/check.py
Ran33 tests in3.465s — OK
failures0 errors0 skipped0 expectedFailures0 unexpectedSuccesses0
```

Meaningful cases independently executed: actual same-FD path replacement, content mutation during read, FIFO/mode/owner/oversize before read, corrupted IPv4/TCP/ICMP checksums, zero/offload ambiguity, fragments/short transport, LAN/WAN alias, loss/stage/window mismatch, duplicate input/output, incorrect payload/header identity and late drop output. Fixtures use temporary files and controlled children/stubs only; no live rig/network/SSH/VPP/nft/capture operation performed. Source fixtures PASS; live composed traffic/provenance NOT RUN and not implemented.
