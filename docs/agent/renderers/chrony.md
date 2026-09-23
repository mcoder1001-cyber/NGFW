# chrony renderer — desired state ↔ rendered config

Package `apps/agent/internal/renderers/chrony` (RF-3; NTP lives only in `services.ntp`, D-050). chrony 4.8.
Files: `/etc/chrony/chrony.conf`, `/etc/chrony/sources.d/vrx.sources`, `/etc/chrony/chrony.keys` (secret).

| `services.ntp` | chrony |
|---|---|
| `enabled: false` / absent | comment, `port 0`, no sources (idle) |
| `vrf` | validated only (chronyd runs in the agent's namespace) |
| `listen[]` | `bindaddress <ip>` (one per family) |
| `port` | `port <n>` (0 = client only) |
| `allow[]` / `deny[]` | `allow <network>` / `deny <network>` |
| `rateLimit {interval, burst, leak}` | `ratelimit interval <i> burst <b> leak <l>` |
| `localStratum`, `orphan` | `local stratum <n> [orphan]` |
| `rtcSync` | `rtcsync` (product only; never on test instances) |
| `makestep {thresholdSec, limit}` | `makestep <threshold> <limit>` |
| `servers[] {address, iburst, prefer, minPoll, maxPoll, nts, keyRef}` | `vrx.sources`: `server <host> [iburst] [prefer] [minpoll n] [maxpoll n] [key <id>] [nts]` |
| `servers[].keyRef` (`key/<name>`) | `chrony.keys`: `<id> SHA256 HEX:<hex of secret>` (id = FNV-32a of the reference, stable) |
| `pools[]` | `vrx.sources`: `pool <host> iburst` |
| `ntsServer` | refused (NTS-KE server certificates are F-ntp) |
| — (agent) | `bindcmdaddress <run>/chronyd.sock`, `cmdport 0`, `driftfile`, `pidfile`, `keyfile`, `ntsdumpdir` (with NTS sources), `sourcedir`, `logdir`, `log tracking` |

Apply: `chronyc reload sources` / `chronyc rekey`; any chrony.conf change → typed restart request
(`ActionRequired`). Retrieve: `chronyc -c tracking | sources | sourcestats | serverstats`. CLI equivalent: none yet.
