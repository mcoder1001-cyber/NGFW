# P11-host current-main isolated live acceptance

Manager actual execution on0730017cad19c07d2f7fdc09fc427faf859c3ca4, tree50a98430bd2145dbf27cb633ed5741f6d326f6b0, parentb8c6c547. Current main native authentication/certificate/runtime fixes preserved; reviewed ten-file host acceptance delta applied cleanly. New own production agent/plugin built; reference /root/vpp read-only, shared VPP untouched.

Actual command in /dev/shm/ngfw-integrate-p11-host-20261005:

```sh
TMPDIR=/dev/shm/p-ys6xmg8x GOTMPDIR=/dev/shm/p-ys6xmg8x GOCACHE=/dev/shm/ngfw-manager-go-20261005 GOMAXPROCS=2 GOFLAGS=-p=2 NGFW_INTEGRATION=1 tools/heavy.sh test/topology/ipsec/run.sh --peer-loss
```

Exit0; safe summary current-main-production-summary.json records all build/responder/initiator/shared-VPP-and-slot-clean phases exit0. Responder actual TestIKEv2NativePackets PASS107.542s; initiator PASS109.523s. Actual encrypted ICMP both directions/TCP1048576bytes, foreignSPI refusal, production Apply and sealed-cache own agent restart retains SAs/traffic, rekey, route withdrawal/recovery, default DPD peer crash/expiry/restart/retry and authoritative empty rollback before private VPP stop passed. Private underlay captures demonstrate ESP/no plaintext; only capture hashes committed. Shared MainPID1014/NRestarts0 before/after; all owned fixture lifecycle cleanup performed by harness. Private diagnostic logs/pcaps remain0600 under ignored .scratch, not public evidence.

This manager run supplements independent fresh liveT3f59 report, not an independent manager self-review. PSK/native acceptance is not certificate negotiation or full installed appliance proof. Final exact current-main complete quick and hosted checks remain required after metadata integration; no final merge claimed.
