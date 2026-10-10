# Acceptance campaign 195 / 250 — 2026-10-10

Owner authorized all feasible tests on 172.30.126.195 and 172.30.126.250 and closure only after acceptance. Source base main4908716b4501312102382e6979b8fc1ded6f9311. Manager branch codex/lab-acceptance-195-250-20261010.

Workers: routing codex/lab-routing-20261010 slots14/6 private FRR; NAT46 codex/lab-nat46-20261010 slot17; WAN codex/lab-wan-20261010 slot20. Two simultaneous private VPPs maximum. Shared VPP restart/startup/management path unchanged. Installed250 source4c8d1b247c6b remains older than test source.

Own builds and logs /tmp/ngfw-lab-manager-20261010, own private RA helper and sensitive evidence /dev/shm/ngfw-ra-lab-20261010. Actual /tmp inode exhaustion observed; reversible remount raised nr_inodes2097152; no old files deleted. Additional owned tmpfs /run/ngfw-lab-build-20261010 size8G created for compilation only, remove after campaign.

Current own agent built, race agent test binary compiling. RA first run fails sandbox phase2 because test workspace TMPDIR outside /dev/shm; failures retained. Next retry with protected /dev/shm/ngfw-ra-lab-20261010/tmp. Broker capability/SCM_RIGHTS boundary test PASS. No RA acceptance/task closure claimed.

Next: tail docs/status/tasks/lab-manager-20261010-evidence/ra-private-run1.txt; await race build session42855; run six current-source host groups with existing test-closeout runner after private VPP budget available, then run same exact test artifacts on250 privately where feasible. Reconcile worker full criteria, publish independent review and receipts, update board only for complete acceptance.

Temporary filesystem prerequisite: ext4 /dev/sda2 original reserved2099368blocks,4096bytes. Root had1768795freeblocks but0unreserved; FRR/PG uid writes failed ENOSPC. Temporarily set reserve2percent (1046072blocks), freeing unreserved ~2.7GiB without deleting any files. Restore exact original count with tune2fs -r2099368 /dev/sda2 after all test databases and artifacts clean up.

2026-10-10 supplemental: RA actual EAP-MSCHAPv2/readback/disconnect, badpassword rejection and EAP-TLSvalid/revoked/foreign PASS using authenticated historical engine artifact and freshly built current helper in protected shm. Production artifact readonlypreflight still fails; noRAclosure. Generic remote250 original run passed agentbaseline/dataplane/MPLS/SRv6/LB+GC; stale fixture ownercleanup/referencehostname assumptions corrected independentlyreviewedbyWANtester7ae30113a. Fresh helper tunnels-delete-owned and wg executable transferred into private temporary tree; no packagesinstalled250. Retrying currentsourceoriginalassertions.

Temporary finite PPP inventory broker prerequisite: created exact scripts/pppoe-kernel-carrier.py at/usr/lib/ngfw/pppoe-carrier.py and originalbroker@template at/run/systemd/system/ngfw-pppoe-broker@.service, both previouslyabsent, no carrierunit. systemctldaemon-reload only. SHA receipts temporary-ppp-broker-assets.json. WANtester permitted only emptyrealinventoryforMultiWAN baseline; cleanupexactfilesaftertest, no foreignledger deletion.
