# Dataplane: VPP workers, cores, NIC queues, hugepages, plugins

**Screen:** System › Dataplane (`/system/dataplane`). **API:** the generic configuration routes on `/dataplane`
(`GET/PUT/PATCH /api/v1/config/candidate/dataplane`, `/api/v1/config/dataplane`), then commit;
`GET /api/v1/state/dataplane` (installed start-up file + host facts) and `POST /api/v1/actions/dataplane/preview`
(rendered startup.conf + diff, read-only). **CLI:** `vrx configure set|merge dataplane …`, then `commit`.

## Why a restart is needed

These settings are not applied live. They are rendered into VPP's start-up configuration
(`/etc/vpp/startup.conf`), which VPP reads only when it starts. Committing them changes the configuration, not the
running data plane. Installing the new file restarts VPP, and **all forwarding stops during that restart**
(seconds to tens of seconds, plus link renegotiation).

In this release the **Apply and restart VPP** button is disabled. Installing the file is a manager step
(`deploy/vpp/apply-startup.sh`, with backup, dead-man timer and automatic rollback), and it waits for the appliance
approval gate (TD-17). The screen lets you stage, validate and preview.

## What each setting does

| group | field | effect (`startup.conf`) |
|---|---|---|
| CPU | Worker threads | `cpu { workers N }`: packet-processing threads; 0 = everything on the main thread |
| CPU | Worker cores | `cpu { corelist-workers … }`: pins workers to CPUs; its length must equal *Worker threads* when both are set |
| CPU | Main core | `cpu { main-core N }`: must not be one of the worker cores |
| DPDK and NICs | RX / TX queues per NIC | `dpdk { dev default { num-rx-queues / num-tx-queues } }` |
| DPDK and NICs | PCI whitelist, DPDK devices | NICs handed to DPDK, each with its logical interface name (`lan`, `wan`, …) and optional per-NIC queues and RX/TX descriptors (power of two, 64–16384) |
| DPDK and NICs | Management NICs | always blacklisted; the generator also protects the NIC the host is managed through |
| Memory | Hugepages (GB) | hugepage memory reserved for the data plane |
| Memory | Buffers per NUMA node | `buffers { buffers-per-numa N }`; must fit in the hugepages |
| Plugins | Plugin switches | `plugins { plugin <file> { enable\|disable } }`. When present, exactly these switches are rendered. When absent, the switches of the installed file are kept |

The schema's rules (worker count = corelist length, main core not a worker, unique PCI addresses, descriptor
sizes) are checked when you save. The error is shown at the field (for example `/dataplane/corelist`).

## Using the screen

1. Open **System › Dataplane**. The yellow banner is a reminder that nothing here takes effect before a VPP restart.
2. Edit the form and press **Save to candidate**. The *Candidate and running* table marks rows that are not committed.
   *Installed start-up file* shows what VPP booted with (workers, cores, plugin switches), the online CPUs and the
   free/total hugepages.
3. Press **Preview startup.conf**. The agent renders the file for the candidate on this host (read-only) and shows the
   diff against the installed file and its sha256.
4. Commit with the pending-change bar. The running VPP does not change.

## Example

```json
PATCH /api/v1/config/dataplane
{ "workers": 4, "corelist": [2, 3, 4, 5], "mainCore": 1, "hugepagesGb": 4 }
```

Preview diff (excerpt):

```diff
 cpu {
   main-core 1
-  workers 2
+  corelist-workers 2-5
 }
```

## Rolling back

- **Before installing:** revert the candidate (Discard in the pending-change bar) or roll back to an earlier revision
  (System › Revisions). Nothing reached VPP.
- **After a manager installed a file:** apply-startup.sh keeps a backup of the previous startup.conf and arms a
  dead-man timer. If VPP or management reachability does not come back within the window, it restores the backup
  and restarts VPP on its own. For a manual rollback, the manager restores the backup it printed and restarts VPP.
