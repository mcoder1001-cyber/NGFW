# Wave-C traffic acceptance

This is an unfinished acceptance driver. The current CLI only validates the slot
and prints its dry-run plan. It cannot run live acceptance and cannot report
`TEST-traffic-C` as complete.

```sh
python3 test/topology/traffic-c/driver.py --slot 14 --dry-run
python3 -m unittest discover -s test/topology/traffic-c -v
```

The importable API transaction primitive uses the slot's local product API,
requires a clean candidate and a committed rollback revision, rejects partial
apply/unsupported fields, and restores the baseline even when evidence fails.
It assumes an exclusively owned slot API/agent/database and candidate; it does
not establish that ownership. Do not call it against an existing shared stack.

Packet parsers require tcpdump text with the owned source/destination in the same
packet line as the actual MPLS label or SRH. Unknown formats fail closed. Parser
fixtures are unit checks, not captured packets. VRRP outage includes the first
and last unanswered interval, and QoS requires all three counters to increase
plus a positive WAN packet count smaller than the actual sent count.

Still required: owned stack/rig fixture, manager quiet window and daemon lease,
global snapshot/restoration, live packet collection, VRRP commit choreography,
IPFIX/product-capture/IGMP/metrics riders, real rollback/residue readback,
NRestarts and TD-H18 host proof. The scenario uses one VPP and a keepalived peer;
HA state sync with two VPP belongs to INTEGRATE-E2E.
