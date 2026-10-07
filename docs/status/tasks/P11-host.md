# Final integration checkpoint

Manager-owned codex/integrate-p11-host-20261005, /dev/shm/ngfw-integrate-p11-host-20261005. Pinned final parent 31ad921385c5fe08227bc20f4150eda6a8e1d873 (actual PR174 merge). Full unchanged local and hosted quick gates are pending on this frozen integration tree. Reviewed provisional receipts are archived remotely at codex/archive-P11-host-integration-receipts-20261005 (9c2368c3). Reviewed sourcef59c3ea7 preserved locally and remote codex/archive-P11-host-reviewed-20261005 (081c9ac5). Only reviewed ten-file task delta applied with three-way merge; current main native auth/certificate/runtime fixes preserved. Mandatory R1/R2/R4/R7/R8 APPROVE; independent exact source fullquick9m55 and fresh real both-role T3 PASS107.48s/109.40s with encrypted ICMP/TCP, foreign-SPI refusal, rekey, agent restart, route withdrawal/recovery, default DPD peer crash/recovery and authoritative owned rollback. No certificate/full appliance proof inferred. Final complete integration quick/hosted and expected-head merge remain required.

Current-main live acceptance on0730017ca independently of old artifacts passed bothroles107.542s/109.523s; safe new summary and manager actual report adjacent. Fresh independent current-main T3 remote52fd2795 PASS107.94s/109.55s with both current native authpatches; report+safe receipts adjacent. Full final local/hosted gate still required. Historical evidence follows.

# P11-host native route-based host acceptance

Source implementation and production packet/lifecycle acceptance completed;
independent review and unchanged complete quick gate pending before merge.
No appliance deployment or certificate peer negotiation is claimed here.

`NGFW_INTEGRATION=1 test/topology/ipsec/run.sh --peer-loss` builds the current
product agent and disposable readback plugin outside reference `/root/vpp`, then
runs existing production-agent packet checks as responder and initiator. It
requires exclusive use of fixed fixture slot8, checks collisions, holds the
shared lab lock and refuses skipped packet tests. Shared system VPP is untouched.
Raw diagnostic logs remain private0600 under `.scratch`; the committed evidence
contains only phase outcomes, source SHA, capture SHA256 and shared PID/restarts.

Exact assertion source26ebf631, report-only child7efd712f. The production Go
implementation is unchanged from the freshly built initial875cc6dc checkpoint.
Latest run `.scratch/P11-host-20261005-061645-1244797/summary.json`:

```
responder: exit0; TestIKEv2NativePackets PASS108.76s
initiator: exit0; TestIKEv2NativePackets PASS
shared-VPP-unchanged-and-slot-clean: exit0
shared before/after: MainPID1014 NRestarts0
passed:true peer_loss_requested:true
```

Verified bidirectional ICMP, exact1MiB TCP payload, inbound/outbound SA counters,
ESP underlay and no plaintext IPIP across the lifecycle, route withdrawal and
recovery, foreign-SPI action refusal, rekey, default liveness after hard peer
loss/recovery, real product-agent restart retaining active SPIs/sealed secrets,
and explicit full owned rollback. Rollback removed profiles, protection, routes,
tunnels/interfaces and returned empty owned Retrieve before disposable shutdown.
The wrapper verifies final slot namespaces/veths are absent and unchanged shared
VPP identity. Packet hashes are recorded in `P11-host-evidence/native-production-summary.json`.

One earlier rollback assertion run failed because empty DesiredState without an
explicit subsystem list authorizes zero managed domains by service.go's existing
contract. The executable request now selects vpn/tunnels/routing/interfaces/vrfs;
it proves removal rather than treating disposable VPP shutdown as rollback.
No production code or substantive assertion was weakened.

Remaining separate acceptance: native certificate peer packet campaign, target
250 installation, real API/browser/CLI UX on installed appliance. Historical
kernel-vpp/policy tunnel requirements are superseded by DEC-ipsec-route-based.
