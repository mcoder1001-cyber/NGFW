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
Outcome-to-packet correlation and transactional setup/cleanup remain unbuilt.

Commands use argv, bounded time and newly created private logs. Their own process
group is terminated on all completion paths, including descendants ignoring
TERM. No shared VPP trace/global capture/restart, SSH or host installation is
performed by source tests. No performance numbers are produced.
