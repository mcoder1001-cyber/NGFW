# Wave-A traffic foundation

This source foundation is **not a complete traffic scenario**. `run.py plan`
prints seven unsupported composed stages and their reusable feature test names.
`run.py run` refuses until reviewed executors and capture lifecycle exist. No
whole-chain PASS or live forwarding is inferred from a feature-suite exit code,
contract-shaped evidence or synthetic packets.

Read-only plan:

```sh
python3 test/topology/traffic-a/run.py plan --slot 14
python3 test/topology/traffic-a/check.py
```

Existing feature modules support separate configuration/readback and sometimes
packets. Bridge/BVI has no packet test; uRPF/PBR has agent subsystem integration
rather than a topology module. Composed VLAN→bridge/BVI→VRF/ECMP→uRPF/PBR→ACL→
NAT44-ED/EI must still be implemented, including expected forward/drop, selected
PBR path, both ECMP paths and translation/endpoint-independence assertions.
This is a real source gap, independently of lab availability.

Future host boundary: a manager allocates slot1–11/14–32 (never12/13), supplies
canonical `tools/lab env` values and a root-owned regular0600 lease at
`/run/vrx-test/w<N>/traffic-a-lease.json`. Its exact fields are task
`TEST-traffic-A`, integer slot, matching prefix, current boot ID, bounded
`expires_unix`, and32-hex lease ID. The runner never grants its own lease.
Live opt-ins are `VRX_INTEGRATION=1`, `VRX_TRAFFIC_A_HOST=1`; global VPP ownership
and retained NAT64/global flags are refused. Shared lab lock, per-slot exclusivity
and lease renewal/revalidation must be implemented before live enabling.

Capture contract: both `ns-w<N>-lan/wan` peer devices, fixed argv, fixed `/run/vrx-test/w<N>/traffic-a/<run-id>/<side>.pcap`, private
regular0600 bounded classic Ethernet pcap, run identity, content SHA256,
bounded time window and zero dropped packets. Parser supports IPv4 ICMP/TCP
and at most two VLAN headers, rejects truncated/fractured framing. Config
readback hashes must match a committed revision, and interface counters must
belong to the allocated slot. Synthetic fixtures are explicitly `source_fixture`
and refused as `tcpdump_live`. External metadata is not cryptographic provenance:
only future in-tree executors may produce live attestations, under the lease.

`CONTRACT_VALIDATED` and `SUPPORT_TEST_VALIDATED` mean structural checks only;
both return `whole_chain_proven=false` and `packet_outcomes_proven=false`.
Pure packet-to-expectation correlation is implemented in `correlation.py`.
Transactional live setup/cleanup, capture production and provenance attestation
remain unbuilt.

Commands use argv, bounded time and newly created private logs. Their own process
group is terminated on all completion paths, including descendants ignoring
TERM. No shared VPP trace/global capture/restart, SSH or host installation is
performed by source tests. No performance numbers are produced.

## Pure packet correlation

Typed `Flow`, `Probe` and `ExpectedCase` specify one exact stage outcome and mode;
the validator reads both hashed private captures, never a claimed outcome string.
Input/output captures must be distinct, same slot/run/stage, zero-loss and span
the probe window plus a bounded quiet interval. Each probe has unique IPv4 ID/
transport sequence, explicit flow, payload digest, VLAN stack and TTL behavior.
The parser bounds2048 records and64 probes. Forwarding preserves the typed flow;
translation must explicitly change IP/port/ICMP identifier. Missing/duplicate or
unexpected records, TTL/payload/header changes and wrong next-hop MACs refuse.
Drops require a genuine matching input and no correlated output through the
quiet window. ECMP requires both declared next-hop identities; endpoint-independent
NAT requires a stable translated source mapping across distinct destinations.
PBR requires one exact selected next hop. Reverse TCP NAT flow is supported.

The return status is `FIXTURE_CORRELATED` for synthetic source bytes or
`PACKET_EXPECTATIONS_MATCHED` for structurally labelled live bytes. Neither is
traffic PASS: `live_provenance_verified=false`, `whole_chain_proven=false`.
Matching observed frames to an expectation does not attribute a drop to uRPF
rather than ACL or prove configured ECMP/NAT semantics without live config/
counter/executor linkage. That linkage remains a real implementation gap.

Capture acquisition opens each path once with `O_NOFOLLOW`, `O_CLOEXEC` and
`O_NONBLOCK`, validates the descriptor as an owned0600 regular file and reads a
bounded snapshot. Hashing and parsing use those identical bytes; alias detection
uses opened inode identities. Size/time changes during acquisition are refused.
This does not attest the producer or make a mutable file cryptographically
trusted; the protected live directory/producer contract remains unimplemented.
IPv4 header, TCP pseudoheader/segment and ICMP checksums are validated. Invalid
or unknown/offloaded checksums raise typed `INDETERMINATE`, never a matched/drop
outcome. Arbitrary metadata cannot waive this check. Non-IPv4/unsupported
transports, fragments and truncated records cannot establish correlation.
Synthetic fixtures now encode correct checksums; they remain offline fixtures.
