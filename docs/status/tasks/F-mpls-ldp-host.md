# F-mpls-ldp-host implementation report

Implemented the remaining host-independent product wiring: FRRDoc LDP preservation,
existing renderer registration, bounded source-derived FRR JSON adapter, active
RIB/link adjacency selection, S1 scoped label synchronization, failed-read hold-down,
scheduler retry, neighbor events, existing state RPC and isolated MPLS ownership.

Verification: three-package race tests passed; scoped TestLdp subsystem race tests
passed; go vet of frrsync/ldp, descriptors/mpls, subsystems, agent and desired passed;
CI source check passed. Source fixtures follow upstream FRRouting ldp_vty_exec.c.
No new contract, schema or generated binary API changes.

Lab acceptance NOT RUN: no FRR/VPP/lab services in cloud. Renderer canonical config,
ldpd operational sessions, PHP/ECMP forwarding, timed withdrawal, hold-down live,
restart simulated loss, table deletion and en/fa live screenshot remain deferred.
Topology verify.py is read-only and compiles; it has not been run against a lab.
EOS IPv4 switching is implemented. Non-EOS/explicit-null, ingress imposition,
IPv6 and targeted LDP remain unsupported/deferred as documented.

## Final developer evidence (actual output)

Executed in `apps/agent` with the shared Go binary on PATH after the retrieved
installed-count correction:

```text
$ go test -race ./internal/frrsync/ldp ./internal/descriptors/mpls ./internal/renderers/frr/ldp
ok  ngfw/agent/internal/frrsync/ldp  1.231s
ok  ngfw/agent/internal/descriptors/mpls  (cached)
ok  ngfw/agent/internal/renderers/frr/ldp  (cached)
$ go test -race ./internal/subsystems -run '^TestLdp' -count=1
ok  ngfw/agent/internal/subsystems  1.114s
$ go vet ./internal/frrsync/ldp ./internal/subsystems ./internal/agent
[no output; exit 0]
$ tools/ci.sh check --base main
check PASSED (0m06s)
```

The installed-count regression proves the state reports zero after a configuration
withdrawal even when a route remains in the observation cache, and reports a
surviving VPP-owned route after a subsequent failed scheduler apply. Installed
counts are read from named ownership-filtered Retrieve with a 10-second deadline;
failed/disconnected reads return unavailable.

## Aggregate gate status

Complete quick is **BLOCKED-ENV**, not green. The manager attempted this exact
command on baseline main `06e4368c` (not this branch):

```sh
PATH=/workspace/scratch/e4f791ef53f7/go/bin:/workspace/scratch/e4f791ef53f7/ci-tools:$PATH NGFW_CI_TOOLS_DIR=/workspace/scratch/e4f791ef53f7/ci-tools NGFW_CI_LOG_DIR=/workspace/scratch/e4f791ef53f7/baseline-ci GOMAXPROCS=2 GOFLAGS=-p=2 tools/ci.sh quick
```

The developer inspected the manager-produced `07-turbo.log` at
`baseline-ci/NGFW-20261004-044927-2`; representative real lines:

```text
@ngfw/api:test:  ❯ src/auth/tokens.keyring.test.ts (8 tests | 2 failed) 179ms
@ngfw/api:test:      → EINVAL: invalid argument, chown '/tmp/ngfw-td10b-ring-dPaRtr/foreign'
@ngfw/api:test:  ❯ src/commit/pg-lock.test.ts (3 tests | 1 failed) 2180ms
@ngfw/api:test:      → No address added out of total 1 resolved errors: [listen EPERM: operation not permitted /tmp/ngfw-td10a-h1-3qDN3M/agent.sock]
```

The manager stopped its owned hung process. This is evidence of baseline
execution restrictions, not proof of a complete branch quick pass. Hosted complete
quick and integration acceptance must be resolved by the manager; this report
makes no hosted green claim. Live lab acceptance remains NOT RUN.
