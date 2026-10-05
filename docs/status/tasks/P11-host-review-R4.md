# Independent R4 P11-host

SHA f59c3ea7b9cf0d6ad52a0f1c4db82bb0f6bddd1b. 2026-10-05; reviewer branch codex/review-r4-seven-20261005; owned reports only. Exact-SHA git archive snapshots under this reviewer worktree. Envelope: independent R4/T3, no product changes, no real process/service kills, no shared VPP/host config mutation.

Inspected host-acceptance.py: explicit integration/root gates, exclusive fixed-slot8 fixture lock, shared lab lock, collision refusal, slot exports parsed as data, private logs, before/after shared-VPP PID/restart and final slot-clean checks. Inspected isolated-vpp.py at exact SHA: unshare private mount, child-only /run/vpp mapping, unique API segment, no-pci, own process termination. /root/vpp stays readonly. Generated bindings used by production implementation; this wrapper does not alter them.

Inspected exact ikev2_integration_test.go: production binary receives owner w8, table base8000, globalsOwner0, private state/socket; restart targets its own spawned agent; 1MiB TCP, SA counters, ESP/no plaintext, route recovery and changed CHILD SPI checks exist. Authoritative empty rollback selects vpn/tunnels/routing/interfaces/vrfs and asserts empty Retrieve plus profiles/protection/interfaces before fixture shutdown.

Inspected evidence, NOT reviewer execution: canonical P11-host.md and native-production-summary.json record responder/initiator exit0, peer-loss requested, shared MainPID1014 NRestarts0 unchanged and two pcap hashes. Evidence SHA7efd712f differs only in documentation from requested f59c3ea (git diff confirmed). No raw secrets/logs copied. No native certificate acceptance claimed.

Actual reviewer commands:
`tools/heavy.sh python3 -m unittest discover -s .review-fixtures/p11/test/topology/ipsec -p test_host_acceptance.py -v`
```
Ran 2 tests in 0.000s
OK
```
`tools/heavy.sh env NGFW_INTEGRATION=0 python3 .review-fixtures/p11/test/topology/ipsec/host-acceptance.py`
```
requires NGFW_INTEGRATION=1; this is not a unit or shared-VPP run
```
exit2 as intended.

Findings: none within inspected R4 safety scope. Verdict: APPROVE. Independent live T3 not claimed: envelope prohibits killing any real process; this harness terminates real fixture VPP/agent/peer processes. Manager may retain supplied acceptance or allocate a separately authorized live rerun.
