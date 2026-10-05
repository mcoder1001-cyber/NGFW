# Independent P11-host live T3

Source f59c3ea7b9cf0d6ad52a0f1c4db82bb0f6bddd1b, git archive in owned executable RAM /dev/shm/r4p; slot8; 2026-10-05 09:04–09:12 UTC. No inherited actual acceptance. Snapshot metadata HEAD pinned to the same source using read-only Git object alternates; no product files changed. Fresh own patched plugin built against read-only /root/vpp c3200b88dc46bd380f00a49ca3392a102cc1980b; fresh own production agent built from the snapshot. No root .scratch artifacts reused.

Exact command, cwd /dev/shm/r4p:

```sh
TMPDIR=/dev/shm/r4t GOTMPDIR=/dev/shm/r4t GOCACHE=/dev/shm/r4g GOMAXPROCS=2 GOFLAGS=-p=2 NGFW_INTEGRATION=1 tools/heavy.sh test/topology/ipsec/run.sh --peer-loss
```

Actual outer result: exit0.

```text
{"passed": true, "summary": "/dev/shm/r4p/.scratch/P11-host-20261005-090415-2355934/summary.json"}
```

The driver held private-fixture exclusive /run/lock/ngfw-p11-host.lock and shared lab lock, under the heavy semaphore. Both roles used production sealed-secret delivery and actual gRPC Apply, private VPP native IKEv2 and stock strongSwan peer only. Exact snapshot swantest selects the extracted slot10 root and starts its `usr/sbin/charon-systemd` binary inside private mount/network namespaces; the plain `usr/lib/ipsec/charon` file also exists, but this exact source does not select it. No arbitrary charon-systemd version command ran on the host, no system service was started. This is PSK peer acceptance, not strongSwan RA incident or new RA-engine acceptance.

| Scenario | Expected | Actual | Result |
|---|---|---|---|
| PSK negotiation + ICMP both directions | Native protected forwarding | Both roles executed packet assertions | PASS |
| TCP | Exact 1MiB payload | TCP exact1048576 bytes marker both roles | PASS |
| Foreign SPI | Refuse unrelated actions | Runtime refusal assertions both roles | PASS |
| Reconcile + own agent restart | Retain active SAs and traffic, reopen sealed secrets | Same IKE/CHILD SPI assertions and traffic both roles | PASS |
| Route withdrawal/recovery | Stop prefix forwarding and restore | Assertion and marker both roles | PASS |
| Rekey | Replace CHILD SPI | Marker both roles | PASS |
| Own peer crash/restart | Default DPD removes SAs, lowers IPIP, fails closed, recovers | Actual crash/expiry/restart and encrypted traffic both roles | PASS |
| Underlay capture | ESP present, plaintext IPIP absent | Actual private capture parsed before/during/after lifecycle | PASS |
| Owned rollback | Empty Retrieve, no owned profiles/protection/routes/tunnels/interfaces | Assertions before private VPP shutdown both roles | PASS |
| Host safety + cleanup | Shared VPP unchanged; owned fixture absent | MainPID1014 NRestarts0 before/after; no slot8 ns/veths; private PIDs exited | PASS |

Actual selected output:

```text
responder:
DISPOSABLE_VPP pid=2363951 runtime=/dev/shm/r4p/.scratch/isolated-vpp-2363945
    ikev2_integration_test.go:381: TCP exact 1048576 bytes passed
    ikev2_integration_test.go:436: production native state measured inbound/outbound counters without keys; foreign SPI actions refused
    ikev2_integration_test.go:447: production Apply reconciliation retained active SA and traffic
    ikev2_integration_test.go:457: production agent restart reopened sealed secrets, initial Service.Resync retained identical active IKE/CHILD SPIs and protected traffic
    ikev2_integration_test.go:495: rekey changed CHILD SPI
    ikev2_integration_test.go:524: peer crashed; awaiting native VPP default liveness expiry (30 seconds, 3 retries)
    ikev2_integration_test.go:538: default VPP DPD removed crashed peer SA
    ikev2_integration_test.go:575: default VPP DPD cleared crashed peer and lowered IPIP; peer restart and automatic native retry restored bidirectional encrypted traffic
    ikev2_integration_test.go:644: production owned rollback removed profiles, protection, routes, tunnels and interfaces; Retrieve empty before disposable VPP shutdown
    ikev2_integration_test.go:646: underlay capture: ESP present; plaintext IPIP absent before negotiation, during ICMP/TCP/rekey, and after SA deletion
    ikev2_integration_test.go:647: route withdrawal and recovery passed; SA deletion lowered IPIP and stopped traffic while route remained
--- PASS: TestIKEv2NativePackets (107.48s)
DISPOSABLE_VPP_STOPPED pid=2363951
initiator:
DISPOSABLE_VPP pid=2372133 runtime=/dev/shm/r4p/.scratch/isolated-vpp-2372123
    ikev2_integration_test.go:381: TCP exact 1048576 bytes passed
    ikev2_integration_test.go:436: production native state measured inbound/outbound counters without keys; foreign SPI actions refused
    ikev2_integration_test.go:447: production Apply reconciliation retained active SA and traffic
    ikev2_integration_test.go:457: production agent restart reopened sealed secrets, initial Service.Resync retained identical active IKE/CHILD SPIs and protected traffic
    ikev2_integration_test.go:495: rekey changed CHILD SPI
    ikev2_integration_test.go:524: peer crashed; awaiting native VPP default liveness expiry (30 seconds, 3 retries)
    ikev2_integration_test.go:538: default VPP DPD removed crashed peer SA
    ikev2_integration_test.go:575: default VPP DPD cleared crashed peer and lowered IPIP; peer restart and automatic native retry restored bidirectional encrypted traffic
    ikev2_integration_test.go:644: production owned rollback removed profiles, protection, routes, tunnels and interfaces; Retrieve empty before disposable VPP shutdown
    ikev2_integration_test.go:646: underlay capture: ESP present; plaintext IPIP absent before negotiation, during ICMP/TCP/rekey, and after SA deletion
    ikev2_integration_test.go:647: route withdrawal and recovery passed; SA deletion lowered IPIP and stopped traffic while route remained
--- PASS: TestIKEv2NativePackets (109.40s)
DISPOSABLE_VPP_STOPPED pid=2372133
```

Safe summary/capture hashes: R4-closures-evidence/p11-summary.json. Source, production agent, plugin, peer binary and original private log hashes: R4-closures-evidence/p11-hashes.json. No raw peer/SA/secrets logs or pcaps committed; no shared host trace. Outer driver2355934, responder private VPP2363951 and initiator private VPP2372133 all absent after completion (`ps -p ... -o pid,comm` printed header only); `ip netns list` and slot8-link filter empty. Harness owns and stops peer and agent PIDs and removes private slot8 base; private RAM scratch removed after preserving safe summaries/hashes. No other worker paths/global caches touched.

Verdict: PASS (fresh actual private-native-production PSK T3). Source R4 approval remains the separately reported prior approval; it is not substituted for this run. Complete quick not run here; assigned to R1.
