# FRR LDP and label synchronization

The existing LDP renderer emits router ID, IPv4 transport address, mapped Linux
interfaces, secret references and the optional dynamic label block. `FRRDoc`
now preserves LDP configuration so the renderer participates in commit/rollback.

The registered `mpls-ldp` dynamic source polls once per second. FRR's
`show mpls ldp binding json`, `neighbor json`, `discovery detail json` and
`show ip route json` are read through the bounded FRR runner. The JSON adapter
follows FRRouting `ldpd/ldp_vty_exec.c` (`show_lib_msg_json`,
`show_nbr_msg_json`, `show_discovery_detail_adj_json`). These source-derived
fixtures are unit fixtures, not recordings from the project lab.

In-use IPv4 LIB rows join the peer LSR ID to link discovery adjacencies. Only
adjacency source addresses present in the selected IPv4 RIB's active next hops
are used. Interfaces resolve exclusively through the configured LCP mapping.
EOS routes use the local label; remote implicit-null pops, local implicit-null
creates no route. ECMP is normalized/deduplicated. Reserved labels other than
implicit-null fail the observation. A missing adjacency/mapping also fails the
observation rather than withdrawing healthy forwarding state.

`mpls-route.ldp` reuses DF-7's MPLS descriptor with its own persisted boot keys.
Its source belongs to no configuration domain. Static descriptors exclude LDP
records and reject attempted takeover. The S1 scheduler resolves dependencies
against the full retrieved state outside source scope; LDP routes depend on
MPLS table and interfaces. Filtering against the prospective configuration
removes routes before dependent interfaces or LDP configuration are deleted.
Only the scheduler writes VPP, under the service transaction lock.

Failed reads retain the last desired set for 60 seconds, then flush. Successful
empty reads withdraw immediately. Neighbor changes publish the existing LDP
event kind. Read failures publish a degraded event on transition. The existing
MplsLdpState RPC returns detached cached neighbors and LIB, while its installed
count comes from a bounded-time read of the named descriptor-owned VPP routes.
Failed retrieval returns unavailable instead of presenting a cached count.
Production uses table 0; a live topology probe only permits assigned slot tables.

## Deferred lab acceptance

No FRR/VPP is available in the cloud workspace. Canonical renderer comparison,
ldpd sessions, source suitability, kernel-MPLS dependency, LCP hello delivery,
restart/loss, scheduler collision and table-delete live evidence remain required.
`test/topology/mpls-ldp/verify.py` checks a pre-provisioned lab session and label;
`--withdrawn` checks withdrawal. It does not provision processes or claim the
unexecuted setup/hold-down/restart tests passed. Mutating table-0 globals acceptance remains under the lab globals lock.
The explicit read-only table-0 probe option described below acquires no ownership;
run it while holding the shared lab lock so VPP cannot restart underneath it.

Feature-local reader bounds are 4 MiB per command and 10,000 rows/hops/adjacencies
or translated paths. Joins index prefixes and neighbor identities and honor
cancellation. A nonempty JSON object without the command's expected top-level
field is an error; FRR's lazy empty LIB/neighbor object `{}` is accepted.
Failed scheduler applies leave the cache dirty and retry unchanged snapshots.
Prospective LCP remapping filters old adjacency paths immediately.

Only EOS IPv4 label switching is implemented. Non-EOS stacks and explicit-null
still require the original DF-7 host probe and are not claimed supported. A
`registerMplsLdpFor` internal seam selects a slot table for topology tests;
production registration always selects table 0. The probe's optional
`--read-table-zero` reads table 0 without changing globals or acquiring ownership.

Supported dynamic state is bounded to 256 distinct label routes. Larger snapshots
fail the read and retain the last good state during hold-down. This keeps the
inherited per-label authoritative collision dumps bounded; safety checks are kept.
The manager-owned bulk snapshot follow-up is due 2026-10-11; see the task scale-debt
file. No performance or throughput acceptance is claimed.

Run table-0 read probes under both shared locks (shared-host-rules §7):

```sh
flock -s /run/lock/ngfw-lab.lock flock -s /run/lock/ngfw-globals.lock python3 test/topology/mpls-ldp/verify.py --pathspace w7 --table 0 --read-table-zero --peer 192.0.2.9 --label 16000
```

Replace the example slot/peer/label with assigned lab values. This read-only command
never authorizes a table-0 mutation or an exclusive globals lock.
