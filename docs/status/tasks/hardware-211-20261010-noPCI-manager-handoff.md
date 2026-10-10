# Required-plugin no-PCI manager handoff

Actual first API seed stayed at revision0 because agent validation failed while the required linux-cp plugin was not loaded. The canonical startup preview enables only `linux_cp_plugin.so`, `linux_nl_plugin.so` and `npt66_plugin.so`; it retains noPCI, managementPCI04 blacklist and zero physical device rows. This operational rendering does not edit the stored API document or create a revision. The worker has not restarted/applied/bound anything; root manager alone owns real startup application.

Actual reviewed preview: private `host-211/noPCI-plugin-preflight-20261010T123700Z.json`, 274716B/0600/fsync, SHA256 `35131df9f0853f2865b197251f067db7762ec664a3dd5cdcd3ca509e234678c6`; exact source4a63. SSH0/stderr0, canonicalrender0 and installed productdryrun0. Actual management TCP controller22/enp4s0/PCI04igc28, VPP7359 bootidentity/local0 and product approval gate pass. FullL3 unchanged, ioerr6stable/newstorageerrors[]. Only the three plugin switches differ; no new CPU, physical NIC or route setting is introduced.

Controller-private artifact directory: `/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211`, 0700. The following new files are0600, fsynced and never committed:

| Artifact | Bytes | SHA256 |
|---|---:|---|
| `noPCI-required-plugins.doc.json` | 3170 | `3c1ba66789d713ac8c2a8b8fb55b021ba8279c153d5b68a57794f04289419d4b` |
| `noPCI-required-plugins.conf` | 735 | `367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184` |
| `noPCI-current-startup.conf.before` | 608 | `c892394e36bfc45950407128a81c849fdaa1c66a1fbe64ce9154fa271f215b5e` |

The previous actual startup metadata is root:root0644,608B (firstboot9134 receipt); current fresha7af confirms root0644/608B/identical hash. Current fresh original22 proof is `resumed-fresh-state-20261010T123410Z.json`162847B SHAa7af1ab5, all13commands0, sameboot3a609803/VPP7359/agent7477/API7481/nginx9281 active/NRestarts0, filesystemCLEAN/protectedmanagement/all17originalkernel/fullL3/storagePASS. Fresh native API proof `resumed-api-protection-20261010T123506Z.json`11808B SHA215e3d9e confirms configrevision0/exactemptydocument/noPending/agentreachable, all16VM+routing sysctls/DNS/netplan unchanged and full nativeNFT snapshot. These are observations, not service-state restoration commands.

Manager staging must refuse collisions or changed live hash. Copy the exact3170B document to a new root-private0700 original-root directory `/var/lib/ngfw-install-recovery/hardware-211-20261010-noPCI-plugin-fix/doc.json`0600, verify the exact SHA and fsync; this path has not been created by the worker. Before any service pause, preserve actual current startup bytes/metadata, unit states and complete management/NFT/sysctl/PCI facts there and offhost. No credential material is needed in this runtime-only document. Preserve any existing directory/file rather than overwriting an unknown record.

Root-manager-only real command, after actual staging and applicable approval:

```sh
/usr/lib/ngfw/apply-startup.sh \
  --mode product \
  --doc /var/lib/ngfw-install-recovery/hardware-211-20261010-noPCI-plugin-fix/doc.json \
  --apply \
  --approve-rendering 367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184 \
  --expect-sha256 c892394e36bfc45950407128a81c849fdaa1c66a1fbe64ce9154fa271f215b5e \
  --expect-new-sha256 367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184 \
  --mgmt-if enp4s0 \
  --mgmt-peer 172.30.126.195 \
  --mgmt-probe tcp:172.30.126.195:22
```

Use the installed asynchronous product path and its mandatory lock holder/dead-man; no foreground, force, fake host facts or worker invocation. Record the actual detached unit/work path and committed/rolled-back/console-needed marker; do not call a launched job successful. Preserve preexisting API/agent active states and pause API then agent before the manager's VPP restart if that is the reviewed chosen sequence. VPP dependencies can stop them; explicitly inspect actual states and resume agent then API only under manager's ordered phase after committed noPCI runtime and actual plugin/binaryAPI proof. Nginx may remain serving while SSH22 and the original network stay intact.

Finite rollback: the packaged guardedapply backs up exactly the live608B startup before replacement, seals rendering/live hashes, records VPP/management identities and arms one dead-man. On failure it restores that backup and verifies the same management probe, unit/boot identity and interface checks; do not manually race its rollback or repeat startup changes. If it reports console-needed, preserve all state and escalate that actual failure. All17data PCI drivers/overrides remain untouched by this noPCI phase; no driverctl/new_id/per-device binding rollback is necessary or authorized here. nr1024/sysctl/DNS/netplan and storedrevision0 must remain unchanged. Restore original service intent only after the actual rollback outcome is known; a failed rollback is not acceptance.

After actual committed noPCI application, observe real native FIRST/API seed to revision1/system.seed-defaults/17original physicalrows, running=candidate/noPending and protected management. Never bind early, bypass validation, write the DB or fabricate revision1. Persistent data binding and original-driver/link/master rollback remain separately unfinished draft work. Root owns any product firstboot-default correction, package rebuild and mandatory hosted quick/review; this runtime preview does not claim that code fix is integrated.
