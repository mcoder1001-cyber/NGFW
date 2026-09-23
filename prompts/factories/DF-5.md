# Task: DF-5 — Descriptors for VPP plugins: ipsec, ikev2, wireguard   (prepend 00-CONTEXT.md)

## Goal
Write the reconciler **descriptors** (Create/Update/Delete/Retrieve/Dependencies) for VPP's native VPN data-plane objects — IPsec SA/SPD/
tunnel-protect/ipsec interfaces (WBS D6.1), the native IKEv2 responder profile path (D6.3) and WireGuard (D6.5) — against the scheduler
interface published by P05a and the generated bindings in `apps/agent/binapi/`. No API, no UI — pure agent-side building blocks that P11
(strongSwan + tunnel protect) and the VPN F-* tasks wire up later. All key material is a **secret**: never logged, never in fixtures except the
literal `VRX_TEST_PSK_<id>` / documented test vectors, never returned by Retrieve in clear.

## Inputs to read first
- `apps/agent/internal/scheduler/descriptor.go` (P05a — the interface; do not change it, request changes via a question file)
- `apps/agent/internal/descriptors/README.md` and one merged example (e.g. `interface/`) — key scheme `interface/<name>`, `vrf/<id>`
- `apps/agent/binapi/ipsec/`, `binapi/ipsec_types/`, `binapi/ikev2/`, `binapi/ikev2_types/`, `binapi/wireguard/`, `binapi/tunnel_types/`, `binapi/ipip/`
  (fixture only) — **the only source of message names and fields**; verify every name below in the package, never guess
- `prompts/P11-strongswan-vpp.md` — the consumer of `ipsec-sa`, `ipsec-tunnel-protect`, `ipsec-sa` Retrieve (counters) and the ipip fixture pattern
- VPP 26.06 docs: https://s3-docs.fd.io/vpp/26.06/ (ipsec, ikev2, wireguard)
- `docs/lab/host-vrx-a.md` — ipsec/ikev2/wireguard plugins are loaded; the host has **no workers** → async crypto mode and per-worker settings get
  integration tests marked `skip: no workers on host`; crypto engines available: native + openssl (check `ipsec_backend_dump`)
- `docs/lab/shared-host-rules.md` — prefix `w<N>`, addresses `10.<N>.0.0/16`, SPD/SA ids and UDP ports from your slot (document the port scheme, never 500/4500/51820 on the host)

## Scope — build exactly this
Enumerate the object types in `docs/status/tasks/DF-5.md` first, then build (estimate: 14 object types × Create/Update/Delete/Retrieve
+ unit test with the fake client + one integration check under the shared lock with your slot prefix):

### Object types per plugin (binapi package → messages to look for)
- **ipsec** (`binapi/ipsec`): `ipsec-spd` (ipsec_spd_add_del; spd id from your slot range), `ipsec-spd-interface` (ipsec_interface_add_del_spd),
  `ipsec-spd-entry` (ipsec_spd_entry_add_del_v2: priority, direction, protocol, local/remote ranges + ports, policy bypass/discard/protect/resolve,
  sa_id), `ipsec-sa` (ipsec_sad_entry_add_del_v3 or newest: sad_id, spi, protocol ESP/AH, crypto/integ algorithms + keys, flags tunnel/udp-encap/
  esn/anti-replay/inbound/async, tunnel src/dst + table + dscp/hop-limit, udp ports, salt), `ipsec-tunnel-protect` (ipsec_tunnel_protect_update:
  sw_if_index, nh, sa_out, sa_in[]; ipsec_tunnel_protect_del), `ipsec-itf` (ipsec_itf_create / ipsec_itf_delete: mode p2p/p2mp, user_instance),
  `ipsec-backend` (ipsec_select_backend — global singleton; Retrieve ipsec_backend_dump; tests read-only), `ipsec-async-mode` (ipsec_set_async_mode
  — skip on host). Retrieve: ipsec_spds_dump, ipsec_spd_interface_dump, ipsec_spd_dump (per spd), ipsec_sa_v5_dump or newest (also carries
  counters/replay window/last-seq for state), ipsec_tunnel_protect_dump, ipsec_itf_dump. Keys never appear in Retrieve output — compare by
  sad_id + spi + algorithm and treat a key change as Update=ErrRecreate; document it.
- **ikev2** (`binapi/ikev2`): `ikev2-profile` — one composite descriptor over ikev2_profile_add_del + ikev2_profile_set_auth (psk/rsa-sig) +
  ikev2_profile_set_id (local/remote: ip4/fqdn/rfc822/ip6/key-id) + ikev2_profile_set_ts (local/remote selectors) + ikev2_set_ike_transforms +
  ikev2_set_esp_transforms + ikev2_set_sa_lifetime + ikev2_profile_set_udp_encap + ikev2_profile_set_ipsec_udp_port + ikev2_profile_set_liveness
  + ikev2_set_tunnel_interface + ikev2_set_responder / ikev2_set_responder_hostname (Update re-issues only the changed setters; profile name
  `w<N>-*`), `ikev2-local-key` (ikev2_set_local_key — file path; global singleton, file under `/run/vrx-test/w<N>/`), `ikev2-sleep-interval`
  (ikev2_plugin_set_sleep_interval — global, read-only in tests). Retrieve: ikev2_profile_dump; state: ikev2_sa_dump / ikev2_sa_v3_dump (newest) +
  ikev2_child_sa_dump (Retrieve-only). ikev2_initiate_sa_init / ikev2_initiate_del_ike_sa / ikev2_initiate_del_child_sa / ikev2_initiate_rekey_child_sa
  = action helpers, not descriptors.
- **wireguard** (`binapi/wireguard`): `wireguard-interface` (wireguard_interface_create: private key (secret) or generate_key, listen port from your
  slot scheme, src ip in `10.<N>…`, user_instance; wireguard_interface_delete), `wireguard-peer` (wireguard_peer_add: public key, endpoint + port,
  allowed ips, persistent keepalive, table id; wireguard_peer_remove), `wireguard-async-mode` (wg_set_async_mode — skip on host). Retrieve:
  wireguard_interface_dump (show_private_key=false — never true), wireguard_peers_dump (flags carry state). Events: want_wireguard_peer_events →
  peer up/down for `StreamEvents` (register in the descriptor package, document the event shape).

### Dependencies (declare in `Dependencies()`, test ordering with the fake)
ipsec-spd → none · ipsec-spd-interface → ipsec-spd + interface · ipsec-spd-entry → ipsec-spd + ipsec-sa (when policy=protect) · ipsec-sa →
`vrf/<id>` (Optional; tunnel table) · ipsec-tunnel-protect → the tunnel interface key (`ipip` from DF-6 or `ipsec-itf` here) + sa_out + every sa_in ·
ipsec-itf → none · ikev2-profile → interface (tunnel interface + responder sw_if_index, Optional) + ikev2-local-key (Optional, rsa-sig only) ·
wireguard-interface → interface-ip of its src address (Optional) · wireguard-peer → wireguard-interface + `vrf/<id>` (Optional).
Until DF-6 merges, the tunnel-protect integration test creates its ipip tunnel as a fixture directly via `binapi/ipip` (prefixed instance) and
deletes it in `t.Cleanup`.

### Deliverables per object type
1. Descriptor in `apps/agent/internal/descriptors/<plugin>/<object>.go`: `KeyOf`, `Dependencies`, `Create`, `Update` (or `ErrRecreate`), `Delete`,
   `Retrieve` (full dump, decoded into the same proto type used for desired state, including metadata such as sw_if_index / sad index; secret
   fields left empty and marked as "present" via a flag or hash of the desired value — decide, document, test).
2. Registration in the plugin's `Register(scheduler)` function; add to the descriptor registry list.
3. Unit tests with the fake VPP client (table-driven: create, idempotent re-apply, update, delete, dependency ordering, Retrieve decoding; plus
   a test that a key never appears in `%v`/`slog` output of any descriptor type — implement `LogValue`/`String` redaction).
4. Integration test against the host VPP (`/run/vpp/api.sock`, `VRX_INTEGRATION=1`, `flock -s /run/lock/vrx-lab.lock`): create → Retrieve shows it →
   delete → Retrieve shows nothing. **Every object name/tag/table id carries your `VRX_TEST_PREFIX` / slot range**; Retrieve-based assertions
   filter by your prefix (other workers' objects exist on the same VPP). Use prefixed loopbacks/ipip fixtures; never touch `local0` or anything
   unprefixed; clean up in `t.Cleanup`. Test keys are documented test vectors, never real material. No peer, so SAs/IKE profiles stay idle —
   assert configuration, not traffic.
5. `docs/agent/descriptors/<plugin>.md`: table object type ↔ VPP messages ↔ notes/limitations (+ the secret-handling contract for P11).

## Rules
- Message names come from binapi. `apps/agent/binapi/` is **manager-owned** (generated for all plugins by P04, D-014): if a message is missing,
  write `docs/status/tasks/DF-5-questions.md` naming the plugin and continue with the rest; never edit `tools/binapi-gen.sh` or
  `apps/agent/binapi/` in your branch.
- Retrieve must decode *everything* the diff needs (all SA flags, algorithms, tunnel endpoints, selectors, transforms); a descriptor without
  Retrieve is not done. Secrets are the one documented exception — compare by identity, not by value.
- No shelling out to `vppctl`. No C. No changes to `startup.conf`. No VPP restarts (D-012).

## Acceptance (paste the evidence)
- [ ] `go test ./internal/descriptors/{ipsec,ikev2,wireguard}/...` green (unit + integration on the host)
- [ ] `grep -rn "vppctl\|exec.Command" internal/descriptors/{ipsec,ikev2,wireguard}` is empty
- [ ] Applying the same desired state twice yields an empty plan (log excerpt) — including with SAs whose keys are not retrievable
- [ ] Object ↔ message table committed; `grep -rn "VRX_TEST_PSK\|private_key" -i` over test logs shows no material (paste the grep)
- [ ] `vppctl show ipsec sa` / `show ipsec spd` / `show ikev2 profile` / `show wireguard interface` pasted for your prefixed objects, then empty after delete

## Out of scope (do not build)
API endpoints, UI screens, schema changes (`vpn.ipsec.*` is P11's contract), F-*/P11 wiring, performance, startup.conf changes. Not yours:
strongSwan renderer and build (RF-2/P11), ipip/gre tunnels (DF-6 — fixture use only), certificates/PKI, IKEv2 initiator flows and rekey
orchestration (F-*), crypto engine/QAT selection, async crypto and per-worker crypto queues (no workers), WireGuard key generation UX,
packet-level ESP tests (P11). No binapi regeneration, no VPP restart, no `local0`, no real key material anywhere.
