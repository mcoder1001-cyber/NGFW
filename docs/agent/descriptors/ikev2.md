# Descriptors: ikev2 (DF-5, WBS D6.3)

Package `apps/agent/internal/descriptors/ikev2` (VPP's native IKEv2 plugin), desired-state types in
`apps/agent/internal/descriptors/vpn/pb/vpn.proto`. Entry point: `ikev2.Register(registry, client,
owner, ikev2.WithSecrets(resolver))`. Message names come from `apps/agent/binapi/ikev2`; the
transform / id-type / auth-method **values** are plain `u8` in ikev2.api (no binapi enum), so the
name ↔ number tables in `ikev2.go` are the IANA numbers exactly as VPP 26.06 defines them in
`src/plugins/ikev2/ikev2.h` (`foreach_ikev2_transform_*`, `foreach_ikev2_auth_method`,
`foreach_ikev2_id_type`), spelled as VPP's CLI spells them.

## Object ↔ message table

| Descriptor (`Name()`) | Key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| `ikev2.profile` | `ikev2.profile/<name>` | `ikev2_profile_add_del` + setters (below); Update re-issues only the changed setters; Delete `ikev2_profile_add_del` is_add=0 | `ikev2_profile_dump` | `interface/<responder if>`, `interface/<tunnel if>` (both Optional); `ikev2.local-key/global` (Optional) for rsa-sig | VPP name `<owner>-<name>` (≤ 63 bytes) |
| `ikev2.responder-hostname` | `ikev2.responder-hostname/<profile>` | `ikev2_set_responder_hostname` (idempotent); Delete = no-op (VPP cannot unset; goes with the profile) | **write-only**: `ErrRetrieveUnsupported` — VPP does not dump the hostname (D-063) | `ikev2.profile/<profile>`, `interface/<name>` (Optional) | alternative to `Ikev2Profile.responder` (address) |
| `ikev2.local-key` | `ikev2.local-key/global` | `ikev2_set_local_key` (path of a PEM key on the VPP host, idempotent); Delete = no-op | **write-only**: `ErrRetrieveUnsupported` (no getter, D-063) | — | plugin-wide; the file is the secret, the agent never reads it |
| `ikev2.sleep-interval` | `ikev2.sleep-interval/global` | `ikev2_plugin_set_sleep_interval`; Delete = no-op | `ikev2_get_sleep_interval` | — | plugin-wide; read-only in tests |
| `ikev2.liveness` | `ikev2.liveness/global` | `ikev2_profile_set_liveness` (period, max_retries > 0, idempotent); Delete = no-op | **write-only**: `ErrRetrieveUnsupported` (no getter, D-063) | — | plugin-wide: the message has no profile name (VPP stores it in `ikev2_main`); VPP defaults 30 s / 3 |

Profile setters, one per desired-state part (Create issues those that are set, Update those that changed):

| Desired field | Message | In place? |
|---|---|---|
| `auth` (psk → `shared-key-mic`=2 with the resolved PSK; rsa-sig=1 with `cert_file`) | `ikev2_profile_set_auth` (is_hex=0) | change yes; removal → ErrRecreate |
| `local_id` / `remote_id` (`ip4`=1 4 bytes, `ip6`=5 16 bytes, `fqdn`=2, `rfc822`=3 text; ambiguous ip ids refused, below) | `ikev2_profile_set_id` | change yes; removal → ErrRecreate |
| `local_ts` / `remote_ts` | `ikev2_profile_set_ts` (is_local) | change yes; removal → ErrRecreate |
| `responder` {interface, address} (a hostname is `ikev2.responder-hostname`) | `ikev2_set_responder` | change yes; removal → ErrRecreate |
| `ike` {crypto_alg, crypto_key_size, integ_alg, prf_alg, dh_group} | `ikev2_set_ike_transforms` | change yes; removal → ErrRecreate |
| `esp` {crypto_alg, crypto_key_size, integ_alg} | `ikev2_set_esp_transforms` | change yes; removal → ErrRecreate |
| `lifetime` {seconds, jitter, handover, max_data} | `ikev2_set_sa_lifetime` | change yes; removal → ErrRecreate |
| `udp_encap` | `ikev2_profile_set_udp_encap` | on yes; off → ErrRecreate (set-only in VPP) |
| `ipsec_over_udp_port` | `ikev2_profile_set_ipsec_udp_port` (is_set=0 old, then is_set=1 new — VPP refuses to set while set) | yes, including removal |
| `tunnel_interface` | `ikev2_set_tunnel_interface` | change yes; removal → ErrRecreate |
| `natt_disabled` | `ikev2_profile_disable_natt` | on yes; off → ErrRecreate (set-only) |

A failing setter during Create deletes the half-built profile. Interface dependencies are the
alias `interface/<name>` (D-065). Write-only descriptors (D-063) return (wrapped)
`vpn.ErrRetrieveUnsupported`, whose message equals P05's `scheduler.ErrRetrieveUnsupported`; they
never echo cached desired state and their Create is idempotent (re-applied on resync).

State and actions (not descriptors — nothing retrieves them into desired state):

* `ikev2.SAs(ctx, client, owner)` — IKE SAs of owned profiles (`ikev2_sa_v3_dump`) with child SAs
  (`ikev2_child_sa_v2_dump`): state name, SPIs, addresses, ids, negotiated transforms
  (`aes-cbc/256`, …), stats, uptime. VPP returns the derived keys SK_d/SK_ai/… in both dumps; they
  are zeroed on receipt and `SAState` has no field for them.
* `InitiateSAInit` (`ikev2_initiate_sa_init`, profile name owner-prefixed), `DeleteIKESA`
  (`ikev2_initiate_del_ike_sa`), `DeleteChildSA` (`ikev2_initiate_del_child_sa`), `RekeyChildSA`
  (`ikev2_initiate_rekey_child_sa`) — for the VPN F-* tasks.

## Secrets

* `auth.psk` is a `sha256:<hex>` reference. Create resolves it (`vpn.Resolve` verifies material ↔
  reference), sends `ikev2_profile_set_auth`, zeroes the request buffer.
* `ikev2_profile_dump` returns the PSK in clear. Retrieve hashes it into the reference and zeroes
  the buffer (also for other owners' profiles, which are then dropped). Retrieve == desired, the
  second apply plans nothing, and this survives an agent restart. A PSK change is a different
  reference → Update → one `ikev2_profile_set_auth` in place.
* `rsa-sig`: `cert_file` and the local key path are paths on the VPP host; the files are
  provisioned outside the agent (certificates/PKI are out of scope for DF-5).

## VPP limitations and how they are handled

* **Responder hostname is not dumped** (`ikev2_responder` has only sw_if_index + addr), so it is the
  separate write-only descriptor `ikev2.responder-hostname` (D-063 forbids echoing it from a
  cache). The profile reports a responder only when VPP has an address for it; the sw_if_index a
  hostname setter leaves behind does not change the profile's value. Caveat: when an initiator flow
  (F-*, out of scope) resolves the hostname, VPP fills the address and the profile would show a
  responder its desired value lacks — the F-* task must model that.
* **id data is cut at the first NUL** — `ikev2_id.data` is `string[64]` and govpp's decoder stops at
  the first zero byte, so an ip4/ip6 id like 10.4.0.1 arrives as "10.4" (`data_len` still 4) and
  could never be retrieved exactly; after-apply verification would fail forever. Create therefore
  **refuses** ip ids with a zero byte followed by a non-zero byte (10.4.0.1, 10.0.4.4, fd00::1) with
  that reason; ids whose tail after the first zero is all zero (10.4.0.0, fd00::) and all ids
  without a zero byte round-trip exactly. Use an fqdn id otherwise. Proper fix = VPP `u8 data[64]`
  or a length-aware govpp decode (question filed).
* **key-id ids** are rejected by VPP 26.06 (`ikev2_is_id_supported`: ip4, ip6, fqdn, rfc822 only);
  Create refuses them with that reason.
* **Unset profile parts** — VPP zero-initialises a profile; Retrieve decodes all-zero selectors,
  transforms and lifetime, `ipsec_over_udp_port` 0xffff (`IPSEC_UDP_PORT_NONE`) and `tun_itf` ~0 as
  "unset", so desired state must omit a part rather than send it empty.
* **UDP 500/4500** — adding the first profile makes VPP register its IKE ports 500/4500 in VPP's own
  UDP stack (refcounted, `ikev2_bind`); this is inside VPP, not a host socket. Tests use only the
  slot's ports for ipsec-over-udp (20000+100·slot+1/+2).
* **Plugin-wide state** — liveness, local key and sleep interval are shared by every profile and
  (on the dev host) every worker. The integration test reads the sleep interval only, re-applies
  VPP's liveness defaults (30/3) and sets a throwaway local key under `/run/vrx-test/<prefix>/`.

## Tests

* Unit: stateful fake (set-only flags, port unset-before-set, PSK in the dump, NUL-truncated ids,
  derived keys in SA dumps). Create/idempotent re-apply/changed-setters-only Update/ErrRecreate
  cases/rollback on setter failure/validation/ownership (`w4` vs `w3`, `w4` vs `w42`)/write-only
  responder hostname and singletons/zero-octet ids/SA state/actions/no PSK in `%v`, `%+v`, slog.
* Integration: `TestIkev2OnHost` — psk profile with every part set, in-place Update, ErrRecreate
  on removal, rsa-sig profile + write-only responder hostname (throwaway cert + key generated at
  test time under `/run/vrx-test/<prefix>/`, removed in Cleanup), singletons, SA helper (0 SAs, no
  peer), second plan empty, delete → gone. `VRX_DF5_PAUSE=<s>` holds the objects for
  `vppctl show ikev2 profile` (which prints the PSK: evidence goes through a redaction filter).
