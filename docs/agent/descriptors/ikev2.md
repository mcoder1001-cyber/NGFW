# Descriptors: ikev2 (DF-5, WBS D6.3)

Package `apps/agent/internal/descriptors/ikev2` (VPP's native IKEv2 plugin), desired-state types in
`apps/agent/internal/descriptors/vpn/pb/vpn.proto`. Entry point: `ikev2.Register(registry, client,
owner, ikev2.WithSecrets(resolver), ikev2.WithBootStore(store), ikev2.WithGlobalsOwner(globalsOwner))`
(store = the owner's persisted `dfkit.BootStore`, shared by the DF-5 packages). Message names come from `apps/agent/binapi/ikev2`; the
transform / id-type / auth-method **values** are plain `u8` in ikev2.api (no binapi enum), so the
name ↔ number tables in `ikev2.go` are the IANA numbers exactly as VPP 26.06 defines them in
`src/plugins/ikev2/ikev2.h` (`foreach_ikev2_transform_*`, `foreach_ikev2_auth_method`,
`foreach_ikev2_id_type`), spelled as VPP's CLI spells them.

## Object ↔ message table

| Descriptor (`Name()`) | Key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| `ikev2.profile` | `ikev2.profile/<name>` | `ikev2_profile_add_del` + setters (below); Update re-issues only the changed setters; Delete `ikev2_profile_add_del` is_add=0 | `ikev2_profile_dump` | `interface/<responder if>`, `interface/<tunnel if>` (both Optional); `ikev2.local-key/global` (Optional) for rsa-sig | VPP name `<owner>-<name>` (≤ 63 bytes) |
| `ikev2.responder-hostname` | `ikev2.responder-hostname/<profile>` | `ikev2_set_responder_hostname` **applied once per VPP instance and value** (D-076, below); Delete = no VPP call (VPP cannot unset; goes with the profile), drops the record | **write-only**: `ErrRetrieveUnsupported` — VPP does not dump the hostname (D-063) | `ikev2.profile/<profile>`, `interface/<name>` (Optional) | alternative to `Ikev2Profile.responder` (address) |
| `ikev2.local-key` | `ikev2.local-key/global` | owner: `ikev2_set_local_key` (path of a PEM key on the VPP host; idempotent: VPP frees and reloads); Delete = no-op | **write-only**: `ErrRetrieveUnsupported` (no getter, D-063) | — | **VPP-global** (D-071); the file is the secret, the agent never reads it |
| `ikev2.sleep-interval` | `ikev2.sleep-interval/global` | owner: `ikev2_plugin_set_sleep_interval`; Delete = no-op | owner: `ikev2_get_sleep_interval` | — | **VPP-global** (D-071); non-owner requirement checked with the getter |
| `ikev2.liveness` | `ikev2.liveness/global` | owner: `ikev2_profile_set_liveness` (period, max_retries > 0; idempotent); Delete = no-op | **write-only**: `ErrRetrieveUnsupported` (no getter, D-063) | — | **VPP-global** (D-071): the message has no profile name (VPP stores it in `ikev2_main`); VPP defaults 30 s / 3 |

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

A failing setter during Create deletes the half-built profile. Interfaces (responder, tunnel) are
named by their **logical name** (D-069, DF-1's resolver): our interfaces by tag id, untagged ones by
VPP's name; another owner's interface is refused (`vpn.ErrForeignInterface`), and Retrieve reports
the logical name. Interface dependencies are the alias `interface/<name>` (D-065). Delete first
checks that the profile still exists (D-074: a vanished profile is done, no VPP call). Retrieve
never reports one key twice.

**Globals (D-071):** local key, sleep interval and liveness are registered as setters only with
`WithGlobalsOwner(true)` (the product agent on a real box, never a test slot). Everybody else
registers `vpn.Require`: the sleep interval requirement is met when `ikev2_get_sleep_interval`
already reports the desired value; liveness and local key have no getter and can never be required
(`ErrNotGlobalsOwner`). A global is never deleted because of absence.

**Responder hostname (D-076):** VPP's `ikev2_set_profile_responder_hostname` is not idempotent —
each call `vec_dup`s the hostname without freeing the previous copy (a leak per resync) and resets
`responder.is_resolved`. The descriptor keeps an applied-once record (`vpn.Records`, bound to the
D-080 boot identity) with `<hostname>|<sw_if_index>/<interface>` and skips the call while it matches;
after a VPP restart, a changed value, or a delete/re-add of the profile (`ikev2.profile` Create and
Delete drop the record; P05 re-creates dependents around a recreate) it is applied once more. Write-only descriptors (D-063) return (wrapped)
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
  (on the dev host) every worker. The host test reads the sleep interval under the shared globals
  lock (shared-host-rules §7) and requires it as a non-owner; the owner's setters of the getter-less
  liveness / local key run only in `TestIkev2GlobalsOwnerOnHost` behind `VRX_DF5_GLOBALS=1`
  (exclusive globals lock, a manager window — their previous values cannot be read back).

## Tests

* Unit: stateful fake (set-only flags, port unset-before-set, PSK in the dump, NUL-truncated ids,
  derived keys in SA dumps). Create/idempotent re-apply/changed-setters-only Update/ErrRecreate
  cases/rollback on setter failure/validation/ownership (`w4` vs `w3`, `w4` vs `w42`)/write-only
  responder hostname and singletons/zero-octet ids/SA state/actions/no PSK in `%v`, `%+v`, slog.
* Unit additions (D-069/D-071/D-076): foreign / untagged interfaces, second delete = no VPP call,
  responder hostname applied once per VPP instance (fake D-080 identity), re-applied after a
  profile re-add, non-owner requirements.
* Host: `TestIkev2OnHost` runs through **P05's reconciler** (`vpntest.Agent`, persisted record
  store): non-owner globals (sleep interval required, liveness / local key refused) → apply psk
  profile with every part + rsa-sig profile + responder hostname (throwaway cert under
  `/run/vrx-test/<prefix>/`) → Retrieve == desired → in-place change = 1 Update → same desired
  state = empty plan → **restart simulation** (fresh connection + descriptors, same store): empty
  plan except the write-only hostname re-apply (skipped in VPP by its record) → profile deleted via
  the API → plan = exactly its create → re-applied → empty → SA helper (0 SAs) → the empty desired
  state deletes our profiles. `VRX_DF5_PAUSE=<s>` holds the objects for `vppctl show ikev2
  profile` (which prints the PSK: evidence goes through a redaction filter).
