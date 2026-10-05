# TEST-traffic-B author validation

Source is ready for independent review. This is author evidence, not an independent T3 verdict. Main integration remains pending; do not mark the merge Done from this report.

All seven primary driver phases ran back-to-back on frozen local 727775ae7c41b3c2cf3ead1387df22549ad4394d, 2026-10-05T16:47:18Z–16:54:11Z. The only subsequent code change is d3c2ce451's three-line WireGuard JSON formatting fix: decimal instance 2760 is identical, the narrowing uint32 cast is removed, and actual WireGuard packets were rerun successfully on d3c. The unchanged complete quick gate passed d3c in 12m14s.

Commands:

```sh
NGFW_INTEGRATION=1 NGFW_TRAFFIC_STOCK_ROOT=/run/vrx-test/w10/swan-stock/root python3 test/topology/traffic-b/run.py --slot 27 --output .scratch/traffic-b-all-primary-complete
NGFW_INTEGRATION=1 python3 test/topology/traffic-b/run.py --slot 27 --phase wireguard --output .scratch/traffic-b-wireguard-lint-fix
NGFW_CI_TASK_CONCURRENCY=2 NGFW_CI_APPLY_SHARDS=2 GOMAXPROCS=2 GOFLAGS=-p=2 tools/ci.sh --base origin/main
```

Actual dispatcher output (selected result lines):

```text
END ipsec passed
END ipsec-cert passed
END wireguard passed
END tunnels passed
END bgp passed
END ospf passed
END dhcp-relay passed
```

Actual quick output:

```text
Tasks:    35 successful, 35 total Cached:    28 cached, 35 total
mode quick · wall time 12m14s
CI GATE PASSED
```

| Phase | Observed acceptance | Result |
|---|---|---|
| IPsec PSK | Production native initiator/responder, protected packets, rekey, agent restart, route withdrawal/recovery, rollback; shared MainPID 1014/NRestarts 0 unchanged | PASS |
| IPsec certificate | Stock RFC7427 peer, exact TCP 1048576 bytes, rekey, restart, route withdrawal/recovery and rollback | PASS; prior FLAKY retained |
| WireGuard | Actual kernel peer handshake/UDP capture, ICMP, state and rollback; exact instance 2760 rerun after formatting fix | PASS |
| GRE and L2VXLAN | REST candidate owner, candidate/running SHA, concrete revision; actual encapsulated kernel-peer ICMP, API/CLI state, rollback/no residue | PASS |
| BGP | 200 learned routes, exact learned-prefix FIB before/after, bidirectional ICMP and exact TCP echo, scoped capture, withdrawal/rollback | PASS 94.752s |
| OSPF | Learned-prefix FIB and bidirectional ICMP/TCP, scoped capture, existing convergence/restart/withdrawal/rollback | PASS 63.044s |
| DHCP relay | REST commit/state, captured DISCOVER giaddr10.27.1.1 and lease; Kea restart/rollback | PASS 29.023s |

The exact summaries and scoped text captures are in [evidence](TEST-traffic-B-evidence/composed-primary.json). Native captures and raw peer diagnostics remain private; only redacted summary/hashes and fixed positive markers are committed. All seven wrappers verified unchanged shared VPP PID/NRestarts. The native summary records the observed numeric values. The quick gate includes all 35 Turbo tasks, 138 agent packages with race detector, CLI and all 25 Go test modules, generated-output and safety guards. Startup harness was an unchanged-green cache hit under the unmodified gate; no fresh harness execution is claimed.

The first composed run failed once at native certificate immediate post-route-restore ping. The unchanged final composed run passed; this is explicitly FLAKY with preserved [failure excerpt](TEST-traffic-B-evidence/certificate-flaky-excerpt.txt), private-log SHA, owner and due date in docs/tech-debt.md. The G115 lint failure was a real source issue and was fixed before this successful full gate. The generated Valkey environment expression falsepositive was removed from active commit ancestry, with archived local refs preserved; no gitleaks rule changed.

Acceptance limits remain explicit. WG/BGP/OSPF use product gRPC integration fixtures; their REST candidate/commit acceptance is not established. DHCP and tunnels prove REST configuration. Unrelated disabled baseline agent warnings are recorded exactly; unsupported changed tunnel/interface/routing-l2 fields and new warnings refuse. Optional phase6 smokes are individually not-run with reasons in the summary; whole_wave_passed remains false, primary_passed is true. Native peer-loss is not requested and is not claimed.

Before merge: obtain independent applicable reviews/T1/T3, pin fresh main after the backup merge, validate the combined tree and actual REST baseline including any newly introduced disabled API-owned defaults, and run the required integration gate on that tree. This report does not claim those future checks passed.

## Independent R1 scope correction 2026-10-05

Original prompt Goal and Phases require slot REST API commit → applied for every
primary phase. Native IPsec, WireGuard, BGP and OSPF currently configure through
product gRPC fixtures; their real packet evidence is valid for that narrower
path but does not prove the required API path. No owner decision waived this.
This is missing orchestrator code, not deferrable laboratory execution. Task
remains running; independent R1 verdict is BLOCK pending REST configuration
wrappers and repeated real packets/rollback. The standalone tunnels.py entry
also lacks baseline_warnings argument and requires correction. Prior complete
quick and seven-phase results remain source-specific historical evidence.
