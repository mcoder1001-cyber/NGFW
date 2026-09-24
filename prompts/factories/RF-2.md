# Task: RF-2 — Renderers for daemons: strongswan (swanctl/VICI path; vrx build lands in P11)   (prepend 00-CONTEXT.md)

## Goal
Write the agent-side **renderer** for strongSwan (WBS D6.2): desired IPsec/IKE state → validated `swanctl.conf` + secrets → applied through
charon's own control channel (VICI) → `Retrieve` from VICI `list-conns` / `list-sas` → real `ike-updown` / `child-updown` events. Same pattern
for every GPL daemon; P11 (strongSwan + VPP) is the reference and your consumer — you deliver the daemon side, P11 composes it with DF-5's VPP
descriptors and the kernel-vpp build. Develop and test against the **stock** strongSwan on this host (kernel-netlink inside namespaces); the
renderer must not care which kernel plugin charon loads.

## Inputs to read first
- `apps/agent/internal/renderers/renderer.go` + `README.md` + `ALLOWLIST.md` (P05a) — the interface and helpers you implement
- Daemon docs: strongSwan `swanctl.conf` / `strongswan.conf` man pages (on the host: `man swanctl.conf`, `man strongswan.conf`), VICI protocol doc
  (`src/libcharon/plugins/vici/README.md`), `swanctl --help`. Installed on this host (disabled): frr, strongswan (stock: `/usr/lib/ipsec/charon`,
  `/usr/sbin/swanctl`; vrx build comes from P11), kea-dhcp4/6 + kea-ctrl-agent, unbound, chrony, snmpd, keepalived, rsyslog
- `packages/proto` messages for the domain (P03) — the input type: `vpn.ipsec.proposals`, `vpn.ipsec.tunnels` as specified in `prompts/P11-strongswan-vpp.md`
  (P11 opens the contract PR; if it has not landed, build against the shape in that prompt on a `contract/<id>` branch and say so)
- `prompts/P11-strongswan-vpp.md` — the secret handling contract (`secretRef` → resolver, PSK never in logs/GET/world-readable files)
- `docs/lab/shared-host-rules.md` — you are `daemon-owner: strongswan` for this task; slot prefix `w<N>`, addresses `10.<N>.0.0/16`

## Scope — build exactly this, per daemon
Daemon: **charon** (+ `swanctl` as fallback CLI). All paths come from one injected `Paths` struct (product: `/etc/strongswan.conf`, `/etc/swanctl/conf.d/vrx.conf`,
`/etc/swanctl/conf.d/vrx-secrets.conf`, VICI `unix:///var/run/charon.vici`; tests: `/run/vrx-test/w<N>/swan/{a,b}/…`).
1. Templates in `internal/renderers/strongswan/templates/*.tmpl` rendered with `text/template` and **strict escaping helpers** — no user string reaches the file
   unescaped. Files: `swanctl/conf.d/vrx.conf` (`connections { <name> { version, local_addrs, remote_addrs, local/remote { auth psk|pubkey, id }, proposals,
   children { <name> { local_ts, remote_ts, esp_proposals, mode tunnel|transport, start_action, dpd_action, rekey_time, if_id_in/out } }, dpd_delay, mobike,
   encap, rekey_time, reauth_time } }`, `pools {}`), `swanctl/conf.d/vrx-secrets.conf` (`secrets { ike-<n> { id = …; secret = … } }`, mode 0600) and
   `strongswan.conf` (`charon { plugins { vici { socket = unix://<path> } } filelog … }` — the kernel plugin list is a parameter P11 fills). Escaping:
   strongSwan settings grammar — keys `[A-Za-z0-9_-]`, values without newline / `{` / `}` / `#` / leading whitespace; ids validated as ip/fqdn/email/dn
   shapes; the word `include` is never emitted from user data; connection names `w<N>-…` in tests, `[A-Za-z0-9_.-]{1,64}` always.
2. `Validate()` — strongSwan has **no offline checker** (document it): parse your own rendered output with a strict settings-grammar parser (round-trip
   must be identical), check proposals against the known keyword grammar (`aes128gcm16-prfsha256-ecp256`, …), TS prefixes, id shapes, secret presence.
   In integration, additionally `swanctl --load-conns --noprompt --file <tmp> --uri unix://<test vici>` against your own test charon — never the system one.
3. `Apply()` — write files atomically (temp + rename, correct owner/mode: conf 0640 root, secrets 0600 root), then apply via the control channel: **VICI**
   `load-conn` / `load-shared` / `unload-conn` (+ `load-pool`) from Go — use `github.com/strongswan/govici` (MIT; state in the PR why a dependency outside the
   stack: structured VICI beats parsing `swanctl` text) — or, fallback, `swanctl --load-all --noprompt --file <swanctl.conf> --uri unix://<vici>` as a
   **fixed argv, no shell**; list every binary you invoke in `internal/renderers/ALLOWLIST.md` (`/usr/sbin/swanctl`; test-only `/usr/lib/ipsec/charon`, `/usr/bin/ip`,
   `/usr/bin/unshare`). Apply is transactional: after loading, `list-conns` must contain every rendered connection, else unload what you loaded and return an
   error so the commit engine rolls back. Secrets reach the renderer through an injected `SecretResolver`; they exist in memory and in the 0600 file only.
4. `Retrieve()` — daemon state as structured data via VICI: `list-conns` (loaded config, for drift), `list-sas` (IKE SA: name, state, version, local/remote host + id,
   established/rekey/reauth times, child SAs: SPIs, proposals, bytes/packets in/out, TS), `stats`, `version`. Never emit secrets; `swanctl --list-sas --raw`
   is the fallback parser path only. VPP-side SA/SPD state is DF-5's descriptor — not here.
5. Events where the daemon exposes them (poll at 1 Hz otherwise): VICI `event-registration` for `ike-updown`, `child-updown`, `ike-rekey`, `child-rekey` →
   `StreamEvents`; reconnect with backoff when charon restarts; fall back to 1 Hz `list-sas` polling only while the event socket is down.
6. Unit tests with golden files (`testdata/*.golden`) — every template path covered (IKEv1/IKEv2, psk/pubkey, tunnel/transport, multiple children, pools, dpd,
   mobike, encap), including hostile strings: `"; rm -rf /`, `}\ninclude /etc/passwd\n{`, `#`, unicode, 5 KB id → rejected or escaped, asserted. Golden secrets are
   the literal `VRX_TEST_PSK_<id>`; a test asserts no PSK appears in any log line or in `vrx.conf` (only in the 0600 file).
7. Integration test on this host, **never through the system units or `/etc` paths**: two charons as child processes in the rig namespaces `ns-w<N>-a` /
   `ns-w<N>-b` (veth `w<N>-a`↔`w<N>-b`, `10.<N>.250.1/2`) with test-scoped config dirs: `ip netns exec ns-w<N>-a env STRONGSWAN_CONF=<dir>/a/strongswan.conf
   /usr/lib/ipsec/charon` (VICI socket per instance). charon's pid path is compile-time (`/var/run/charon.pid` on Ubuntu) and collides between instances: give each
   a private `/var/run` (`unshare -m` + tmpfs) unless `man strongswan.conf` on the host documents a pid-file setting — record what you did. Bound **only to
   `127.0.0.1:<slot port>` or to rig veths inside `ns-<prefix>-*`** — assert the rendered `local_addrs`/`remote_addrs` are in `10.<N>.0.0/16` before starting;
   never `ens192` (UDP 500/4500 are fine inside the namespaces, never in the host namespace). Flow: render a PSK IKEv2 tunnel on both sides → Validate →
   Apply (VICI load) → `initiate` from a → `list-sas` ESTABLISHED on both within 10 s, `ip -n ns-w<N>-a xfrm state` non-empty (path: kernel-netlink, no VPP) →
   `terminate` → unload → Retrieve empty, xfrm state empty. You own strongswan for this task; kill by the PID you spawned; the system units stay stopped and disabled.

## Acceptance (paste the evidence)
- [ ] `go test ./internal/renderers/strongswan/...` green, integration included (paste the `list-sas` excerpt with the PSK redacted)
- [ ] `grep -rn "sh -c\|bash -c" internal/renderers/strongswan` is empty; `ALLOWLIST.md` updated; `exec.Command` appears only in the fixed-argv runner
- [ ] A rendered config with `"; rm -rf /` in a description/id field is rejected or escaped (test present); the `include` injection test is rejected
- [ ] No child daemon left running after tests (`pgrep -f /run/vrx-test/w<N>/swan` empty); `systemctl is-active strongswan-starter strongswan` still `inactive`; `/etc/swanctl` and `/etc/strongswan.conf` untouched (`stat` before/after)
- [ ] `grep -rn "VRX_TEST_PSK" <test log>` finds nothing outside the 0600 secrets file

## Out of scope (do not build)
API/UI, schema changes (the `vpn.ipsec.*` contract is P11's — only an additive `contract/<id>` if a rendering field is missing), strongSwan build /
kernel-vpp / socket-vpp packaging (P11), VPP IPsec descriptors and tunnel-protect (DF-5/P11), route-based tunnel wiring with ipip (P11), certificates /
PKI / `authorities` loading (F-pki — render the section shape only, untested), EAP / remote-access / IKEv1 tests (IKEv1 allowed in templates,
not tested), QAT / crypto engines, FRR (RF-1), packet tests through VPP (P11). No `/etc/swanctl`, `/etc/strongswan.conf` or `strongswan.service` changes.
