# TD-11c — questions and notes for the manager

Worker slot 10, branch `task/TD-11c` (base task/P08@998e391, then `main` merged in at 1de0b3d after P08 merged). Nothing
here blocks the task. I went ahead on each item and say below what I chose.

## Q1 — file ownership: I added a 13-line agent hunk to make 3.2 flush once per transaction (please confirm)
Review 3.2 asks for "one flush per transaction". The only transaction boundary in the agent is `Service.applyLocked`
(apply, resync and revert all go through it). The only hook there today is P08's `BeforeTxn`, and it runs at the start, not the end.
If the agent does not call anything at the end, `KeyedClaims.Begin/Flush` and `Wiring.ClaimsTxn` are dead code, so I wired the smallest bracket:
- `service.go`: one unexported field `claimsTxn` next to `beforeTxn`. In `applyLocked`, the batch opens right after `beforeTxn()`, and
  `flushClaims()` runs after the outcome switch and before `st.save()`. A failed write logs, sets the agent DEGRADED and stays
  pending, and the next transaction end writes it.
- `agent.go`: one line after `a := &Agent{…}`: `svc.claimsTxn = wiring.ClaimsTxn`. I set it on the service after construction rather than
  through ServiceConfig, so the change does not touch the lines TD-8 changes (the NewService call, the ServiceConfig tail, the struct
  literal).
- `service_test.go` `newSvc`: the same line, so the agent tests exercise it.

Checked at d364013 (main with W-seed merged in). A 3-way `git merge-file` over main of my service.go, agent.go, service_test.go,
stores.go, reconciler.go, core.go and ifaddr.go with the task/TD-8 and task/TD-11b versions has no conflicts. subsystems.go has
none with TD-8. Against TD-11b it shows 3 conflicts, all W-seed anchor lines, because TD-11b predates W-seed. Over the pre-W-seed
base, TD-11b merges cleanly with my `core.Register` line. TD-9 will own `applyLocked`'s other hunks. If you would rather TD-8 or TD-9
carry this bracket, the hunks are in commits 1037660 and 1c7ded2 and can move unchanged.

## Q2 — host proof: it runs through the product wiring, not the gRPC/projection path (the projection cannot name an untagged af_packet NIC)
An af_packet interface is always called `host-<netdev>` in VPP. P08's projection (`desired.KindOf`) turns every `host-<netdev>`
name into an af_packet *creator*, so a document cannot refer to an existing *untagged* af_packet NIC. The product case is a DPDK NIC
(`lan`, KindExisting). The host proof `subsystems/TestUntaggedNICClaimsOnHost` therefore drives the agent's product registry
(`subsystems.Register` → core with the persisted `IfaceClaims`, the DF-1 alias, the scheduler) directly. The full
Service/projection path for a DPDK-style NIC is covered on the fake VPP by `agent/TestPhysicalNICAddressAndVRF`: address + VRF,
Retrieve == canonical document, restart writes nothing, removal releases the claims.
I did not use `sw_interface_set_interface_name` to rename the NIC into a KindExisting name. That API renames tx and output nodes on the
shared VPP, and after D-126/D-128 (node reuse around deleted interfaces) I did not want to add that risk for a test.

## Q3 — TD-11a doc hunk: core/README.md M5 is now done
`descriptors/core/README.md:64-68` (M5) says P08 owes "a claim path for untagged interfaces". That path exists now: `core.Env.Claims`,
wired in `subsystems.Register`. TD-11a owns that README hunk and `core.go:11-14`, so I did not edit either. I only rewrote core.go's
"Ownership" paragraph (lines 16-19 before), because it describes the behaviour I changed.

## Notes (decided, for the LOG if you agree — I did not take D-numbers)
- **N1, per-address claims.** Holder `interface-ip|<canonical prefix>` for each address, and `interface-ip.table` for the binding. The review plan said
  one holder, "interface-ip". Per address, an address that someone else puts on the NIC (linux-nl from the Linux side, a DHCP
  lease, vppctl) is never reported and never removed. With one per-interface claim, Retrieve would adopt every address on the NIC
  and the next transaction would delete the foreign ones. Cost: one claim record per address, which is small.
- **N2, VPP constraint.** `ip_table_bind` (interface_api.c) refuses any table change while the interface has an address of
  that family (ADDRESS_FOUND_FOR_INTERFACE). If a NIC carries someone else's address, our VRF binding or unbinding fails,
  and the transaction rolls back cleanly. The agent never removes an address it does not hold. Create restores an IPv4
  binding it already changed when IPv6 is then refused, and releases the claim.
- **N3, the trade-off of 3.2.** The keyed claims of a transaction reach disk at its end, before the new desired state is saved. If the agent
  *process* dies in the middle of a transaction, that transaction's keyed claims are lost. An untagged object it created then stays in
  VPP unclaimed, so it is invisible and never reverted, until VPP restarts, which makes the claims expire anyway (D-080). Before this
  change the same scale cost one whole-file rewrite with two fsyncs per claim: 2000 claims took 22.0 s, now 35 ms (TD-11c.md). Interface claims
  (`IfaceClaims`) still write at once, because there are few of them and the envelope scoped the batching to KeyedClaims.
- **N4, 3.1c.** Following the alias is generic, so F-vlan-qinq's `SubinterfaceDescriptor.ProvidedKeys` (e771ecb) is no longer needed
  for ordering. It is harmless and can stay, since the two agree. F-bonding and F-bridge-l2 need no KeyProvider obligation for delete order.
