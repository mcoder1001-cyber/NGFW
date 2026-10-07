# Isolated two-node HA acceptance

`ngfw-b.yml` remains a planned appliance, not a running second node. Provision
two dedicated globals-owner VPP appliances with matching NAT outside addresses,
EI listener/failover endpoints, VRRP and explicit packet policy. Use trusted TLS
and dedicated admin API keys. The shared host VPP is never a mutable test node.
The actual two-node run, native UDP capture and convergence numbers are deferred.

`driver.py` retains read-only observations and the historical external-hook
interface. External hook exit codes are not continuity evidence. Use the concrete
`acceptance.py` for acceptance. It creates exactly one TCP connection, challenges
every echo (bounded10-second read, including normal master-down delay), compares its server-observed translated tuple with the exact EI
session on A and B, observes MASTER/BACKUP transition and increasing B packet
counters. It refuses truncated/ambiguous dumps, tuple changes and reconnects.
No shell hooks or API credentials enter the traffic worker.

On the dedicated outside echo namespace run:

```
ip netns exec w18-ha-server python3 flow.py --server --address 10.101.2.2 --port 18750
```

Configure the allocated `w18-ha-client` namespace with its inside address and
route through the VRRP VIP. Resolve the planned inventory management addresses
to trusted appliance origins before running:

```
NGFW_HA_TOKEN_A=... NGFW_HA_TOKEN_B=... python3 acceptance.py \
  --isolated-lab --node-a https://fw-a --node-b https://fw-b \
  --namespace w18-ha-client --inside 10.101.1.2 --echo-address 10.101.2.2 \
  --router lan-v4 --vrf default --output continuity.json
```

Both nodes must begin with A MASTER and B BACKUP. For priority mode disable
configuration sync on the isolated fixture, enable VRRP preemption and give B
priority greater than one. The probe acquires a dedicated API-key candidate
lease and checks revision and complete candidate identity before lowering only
A's selected priority. It uses a 60-second commit-confirmed transaction, never
confirms it, and verifies automatic restoration. Changed leases, candidates or
foreign/ambiguous pending transactions prevent cleanup; other work is preserved.
Evidence is created exclusively with mode0600 before any mutation. An unsuccessful
run does not emit a successful continuity report.

## Post-handover VPP fault mode

Only in the manager's D-012 post-handover window install `kill_vpp_lab.py` as
`/usr/local/libexec/ngfw-ha-lab-kill-vpp` inside disposable appliance A. Provision
root-owned, non-writable `/etc/ngfw/ha-acceptance-lab.json` containing exactly
`bootId` (the current appliance boot UUID) and `handoverNonce` (32–64 hexadecimal
characters). This opt-in marker must never exist on the shared host. Pin the
appliance SSH host key beforehand; host verification remains strict.

Add `--kill-vpp --after-handover --ssh-node-a fw-a --boot-id-a <UUID>
--vpp-pid-a <PID> --handover-nonce <nonce>` to the same concrete command. The SSH
host must equal appliance A's HTTPS hostname. The fixed helper refuses non-VM
hosts, wrong boot/PID/nonce, writable markers, foreign executables or units. It
uses pidfd and rechecks process starttime before signaling only appliance A's
VPP. Appliance supervision owns recovery. The same established TCP flow must
survive with B becoming MASTER and forwarding the same session. This mode has
not been executed; it does not authorize shared-host fault injection.

Capture native HA UDP8750 on the dedicated link during resync, retain pcap
privately, repeat with controlled loss and require missed/timeout to fail.
ED session loss, native IPsec fresh rekey, appliance restart and globals rollback
remain separate runtime acceptance items; no evidence is fabricated.

Offline regression tests run automatically in the unchanged quick gate through
`driver_test.go`: persistent real socket/challenge echo, replay rejection, exact
session/truncation checks, full mocked transition, candidate lease/pending guards,
TLS/namespace guards and fault identity refusal. Offline passes do not prove a
real two-node failover.
