# strongSwan renderer — desired state ↔ rendered directives (RF-2)

Code: `apps/agent/internal/renderers/strongswan` (README there: files, escaping, VICI apply,
govici limits, test harness). Files: `/etc/strongswan.conf` and `/etc/swanctl/conf.d/vrx.conf`
(0640 root:root), `/etc/swanctl/conf.d/vrx-secrets.conf` (**0600** root:root), written atomically;
applied over VICI (`load-*` / `unload-*`, then a convergence check against `list-conns`/`get-shared`/
`get-pools`); validated structurally (strongSwan has no offline checker; strict round-trip parser +
semantic checks); state from `list-conns`/`list-sas`/`stats`/`version`; events from
`ike-updown`/`child-updown`/`ike-rekey`/`child-rekey`.

## Which tunnels are rendered

`vpn.ipsec.tunnels.<name>` with `enabled` ≠ false and `engine` unset or `strongswan`
(`vpp-ikev2` tunnels belong to a VPP descriptor). One connection + one CHILD_SA per tunnel,
both named `ConnName(<name>)` (`.` → `+`; `site.a` → `site+a`; state and events report both names).
Tunnel names are limited to **60** characters (the derived secret `ike-<name>` must fit strongSwan-side
names of 64; the schema's `objectName` allows 63 — questions file). With an owner prefix every name
must start with it.

## Mapping (connection `connections.<conn>`)

| desired state (JSON path) | rendered | validation / note |
|---|---|---|
| `ikeVersion` | `version = 1\|2` | default 2 |
| `localAddr` | `local_addrs = <ip>` | IP, canonical |
| `remoteAddr` | `remote_addrs = <ip\|hostname\|%any>` | same family as localAddr, ≠ localAddr; `%any` cannot `start` |
| `localId` | `local { id = "<id>" }` | IP / FQDN / e-mail / `@#keyid` / DN; default: `localAddr` (rendered explicitly) |
| `remoteId` | `remote { id = "<id>" }` | as above; default: `remoteAddr` when it is an IP (explicit, so charon never accepts an arbitrary peer identity), else not rendered (`%any`) |
| `auth.method = psk`, `auth.secretRef` | `local/remote { auth = psk }` + `secrets.ike-<conn> { id-local; id-remote; secret = 0s<base64> }` | ref `psk/<name>` (D-051), resolved at render time, 8–1024 bytes; two PSK tunnels with the same identity pair are rejected |
| `auth.method = cert`, `certificate`, `remoteCa` | `local { auth = pubkey; certs = <certificate>.pem }`, `remote { auth = pubkey; cacerts = <remoteCa>.pem }` | shape only (F-pki installs the files in `x509/`, `x509ca/`); untested end-to-end |
| `proposal` → `proposals.<p>.ike` | `proposals = <encr>[-<integ>][-<prf>]-<dh>` | keyword grammar; AEAD ⇔ no integ; AEAD without `prf` → `prfsha256`; IKEv1 + AEAD rejected |
| `proposal` → `proposals.<p>.esp` + `protocol`, `esn` | child `esp_proposals = <encr>[-<integ>][-<dh>][-esn]` or `ah_proposals = <integ>[-<dh>][-esn]` | AH needs `integ` |
| `dpd.enabled/delaySec/timeoutSec/action` | `dpd_delay = <s>s`, IKEv1: `dpd_timeout = <s>s`; child `dpd_action = clear\|trap\|restart` | absent block → no DPD |
| `mobike` | `mobike = yes\|no` (IKEv2 only) | unset → charon default |
| `fragmentation` | `fragmentation = yes\|no\|force\|accept` | unset → default |
| `rekey.ikeSec` (+`reauth`) | `rekey_time = <s>s`, or with `reauth`: `rekey_time = 0s` + `reauth_time = <s>s` | 60–604800 |
| `rekey.espSec/espBytes/espPackets` | child `rekey_time`, `rekey_bytes`, `rekey_packets` | |
| `antiReplay = false` | child `replay_window = 0` | |
| `mode` | child `mode = tunnel\|transport` | transport without selectors = dynamic |
| `localTs[]`, `remoteTs[]` | child `local_ts`, `remote_ts` (comma lists, masked) | ≤ 64 each; policy-based tunnel mode needs both |
| `routeBased.ipipInterface` | child `local_ts/remote_ts = 0.0.0.0/0,::/0` (unless given) + `if_id_in/if_id_out` | if_ids from the `IfIDMapper` P11 provides; none → rejected |
| `startAction`, `closeAction` | child `start_action`, `close_action` (`none` not rendered) | `start` is in the file (boot path) but withheld from VICI: the renderer initiates the child only when it has no CHILD_SA (no teardown on edits, no duplicates) |
| `description` | `# <text>` comment line | ≤ 255, no control characters |
| `natT` | not rendered: NAT-T detection is always on in charon; `encap = yes` (forced UDP encapsulation) exists in the model but has no document field yet | |
| `vrf`, `underlayVrf`, `enabled`, `engine` | not rendered (VRF placement is P11/kernel-vpp; see "Which tunnels") | |
| `vpn.ipsec.settings` | not rendered (VPP crypto engine: DF-5/P11) | |

`pools { <name> { addrs; dns } }` and `authorities { <name> { cacert } }` are rendered from the
model (remote access F-ra-vpn, PKI F-pki map them later); no document field feeds
them yet.

## strongswan.conf

`charon { load_modular = no; load = "<DaemonConfig.Plugins>"; install_routes; [port; port_nat_t];
plugins { vici { socket = unix://<Paths.ViciSocket> } }; [filelog { vrx { path; default ≤ 1 } }];
[journal/syslog default = -1] }` and `swanctl { load = "…"; socket = unix://<Paths.ViciSocket> }`.
P11 sets the plugin list (kernel-vpp); charon-systemd reads the `charon` section by fallback.

## CLI equivalent

`swanctl --load-all` (boot path, same files), `swanctl --list-conns`, `swanctl --list-sas`,
`swanctl --initiate --child <conn> --ike <conn>`, `swanctl --terminate --ike <conn>`.

## What a change does to a running tunnel (`Renderer.Impact`)

| change | effect on established SAs |
|---|---|
| local/remote address, IKE version, IKE proposal, auth method, identities, PSK | IKE_SA terminated (DELETE to the peer) and re-negotiated (`start`: by the renderer; else by the peer/trap) |
| selectors, ESP/AH proposal (incl. PFS group, ESN), mode, route-based if_id, anti-replay | that CHILD_SA terminated and re-negotiated |
| DPD, MOBIKE, fragmentation, lifetimes, start/close/DPD action, pools | none: applies at the next negotiation |
