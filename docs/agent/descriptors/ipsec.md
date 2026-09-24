# Descriptors: ipsec (DF-5, WBS D6.1)

Package `apps/agent/internal/descriptors/ipsec`, desired-state types in
`apps/agent/internal/descriptors/vpn/pb/vpn.proto` (agent-internal until P03b moves VPN leaf
messages into `packages/proto`, D-055). Entry point:

```go
ipsec.Register(registry, client, owner,
    ipsec.WithSecrets(resolver),          // the agent's secret store (vpn.Resolver)
    ipsec.WithBootStore(store),           // persisted ownership records (dfkit.NewFileBootStore in the state dir)
    ipsec.WithGlobalsOwner(globalsOwner), // agent config globalsOwner (D-071)
    ipsec.WithIDRange(lo, hi))            // tests only: the slot's id range
```

All message names come from `apps/agent/binapi/ipsec` (VPP 26.06).

## Object ↔ message table

| Descriptor (`Name()`) | Key | Create / Update / Delete | Retrieve | Dependencies | Ownership / notes |
|---|---|---|---|---|---|
| `ipsec.spd` | `ipsec.spd/<spd_id>` | `ipsec_spd_add_del` (is_add 1/0); Update = ErrRecreate | `ipsec_spds_dump` | — | ownership record (below) |
| `ipsec.spd-interface` | `ipsec.spd-interface/<logical interface>` | `ipsec_interface_add_del_spd`; Update = ErrRecreate | `ipsec_spd_interface_dump` + `sw_interface_dump` | `ipsec.spd/<id>`, `interface/<name>` | ownership record holds sw_if_index + spd_id + SPD pool index |
| `ipsec.spd-entry` | `ipsec.spd-entry/<spd>/<dir>/<prio>/<action>/<sa>/<proto>/<l-range>/<l-ports>/<r-range>/<r-ports>` | `ipsec_spd_entry_add_del_v2`; Update = ErrRecreate (every field is identity in VPP) | `ipsec_spd_dump` per owned SPD | `ipsec.spd/<id>`; `ipsec.sa/<sa_id>` for `protect` | belongs to its SPD; Create refuses an SPD that is not ours; protocol any sent as 255 |
| `ipsec.sa` | `ipsec.sa/<sad_id>` | `ipsec_sad_entry_add_v2` / `ipsec_sad_entry_del`; Update = ErrRecreate | `ipsec_sa_v5_dump` | `vrf/<table_id>` (Optional) for a tunnel SA in a non-zero table | ownership record `<spi>/<protocol>`; keys are secret references |
| `ipsec.tunnel-protect` | `ipsec.tunnel-protect/<logical interface>[/<nh>]` | `ipsec_tunnel_protect_update` (Create and in-place Update: swap `sa_out` / `sa_in`) / `ipsec_tunnel_protect_del`; other interface or nh = ErrRecreate | `ipsec_tunnel_protect_dump` | `interface/<name>`, `ipsec.sa/<sa_out>`, every `ipsec.sa/<sa_in>` | only on our (tagged) tunnel interfaces |
| `ipsec.itf` | `ipsec.itf/ipsec<instance>` | `ipsec_itf_create` (+ `sw_interface_tag_add_del`) / `ipsec_itf_delete`; Update = ErrRecreate | `ipsec_itf_dump` + `sw_interface_dump` | — | tag `<owner>:ipsec<N>`; provides `interface/ipsec<N>` (`ProvidedKeys`) |
| `ipsec.backend` | `ipsec.backend/<esp\|ah>` | owner: `ipsec_select_backend` (by name → index); Delete = no-op | owner: `ipsec_backend_dump` (active backend per protocol) | — | **VPP-global** (D-071); VPP 26.06 on the host reports no backends |
| `ipsec.async-mode` | `ipsec.async-mode/global` | owner: `ipsec_set_async_mode` (idempotent, D-076); Delete = no-op | **write-only**: `ErrRetrieveUnsupported` (no getter, D-063) | — | **VPP-global** (D-071) |

Interfaces are named by their **logical name** (D-069) and resolved with DF-1's resolver
(`iface.Table.IndexByName` through `vpn.DumpInterfaces`): our interfaces by their owner-tag id,
untagged (physical) interfaces by VPP's name; another owner's interface is refused with
`vpn.ErrForeignInterface`, VPP's name of one of our own interfaces with `vpn.ErrNoInterface`.
Dependencies always use the alias `interface/<name>` (D-065); `ipsec.itf` provides
`interface/ipsec<N>` itself.

## Ownership on a shared VPP (D-071, D-080)

* **Tagged:** `ipsec.itf` interfaces carry `<owner>:ipsec<N>`; a protection belongs to the owner
  of its tunnel interface, and `tunnel-protect` refuses untagged tunnel interfaces (`ErrNotOurs`:
  tunnel interfaces are always created by some owner).
* **Ownership records:** SPDs, SAs and SPD bindings carry neither tag nor name, and VPP reuses SPD /
  SA ids after a restart. Each is ours only while an ownership record says so (`vpn.Records` over
  the owner's `dfkit.BootStore`):
  * written only **after** VPP accepted our add — never before it, never for an object that
    already existed. VPP itself refuses an existing `spd_id` / `sad_id` / a second SPD on an
    interface, so Create fails instead of adopting;
  * bound to the D-080 VPP boot identity (kernel boot_id, VPP main PID, VPP start time,
    `apps/agent/internal/vpp/bootid`): after a VPP restart or reboot every record has expired and
    the object is ours again only when our own Create re-adds it;
  * SA records hold `<spi>/<protocol>`, so an SA somebody else created under our id never matches;
    binding records hold `<sw_if_index>/<spd_id>/<SPD pool index>`, so a binding is reported only
    for the interface index and SPD it was made for — on tagged interfaces too, since charon's
    kernel-vpp plugin may bind its own SPD to any interface.
  * P05/P08 must pass a **persisted** store (`WithBootStore(dfkit.NewFileBootStore(...))`, one per
    owner, shared by the DF-5 packages): with the in-memory default a restarted agent recognises
    none of its SPDs/SAs/bindings and fails to re-create them (it never deletes or adopts them).
* `WithIDRange` additionally confines SPD / SA ids (tests: the slot's `VRX_VPP_TABLE_BASE..+499`;
  production: the zero range = every id). Create refuses an id outside it.
* **Deletes re-verify** (D-071, D-074): right before a delete by id or index each descriptor
  re-reads the object — SA by id (same SPI, our record), SPD (exists, our record), policy (still in
  our SPD), binding (logical name still resolves to the recorded index, our record), protection /
  itf (index still carries our tag and id). An object that is gone is done (nil, no VPP call); one
  that is not ours is refused with `vpn.ErrNotOurs`.
* **Globals** (D-071): `ipsec.backend` and `ipsec.async-mode` are registered as setters only with
  `WithGlobalsOwner(true)` (the product agent on a real box, never a test slot). Everybody else
  registers `vpn.Require`: the backend requirement is met when VPP's active backend already equals
  the desired one (read with `ipsec_backend_dump`); the getter-less async mode can never be
  required (`ErrNotGlobalsOwner`). Delete never touches VPP, and a global is never deleted because
  of absence (`DeleteOnAbsence() == false`).
* Retrieve never reports one key twice (`dfkit.Dedupe`).

## Orphaned SAs after a charon restart (D-089)

With the kernel-vpp plugin (P11) charon installs its CHILD_SAs into VPP itself. When charon
restarts, the SAs of the previous charon stay in VPP. RF-2 reports the restart
(`strongswan.State.Restarted`); P11 then calls

```go
res, err := ipsec.SweepAndAck(ctx, cfg, ipsec.CharonSweep{IDs: charonRange, Live: liveSPI}, renderer)
```

* orphan = an SA inside the charon id range (`IDs`, mandatory — the zero range is refused), without
  an ownership record of `ipsec.sa`, whose SPI is not live (`Live`: the SPIs of the CHILD_SAs the
  running charon has installed, from RF-2's state);
* the sweep deletes the `protect` policies that use an orphan (only in charon-range SPDs without our
  record), then the SA — each re-read right before its delete; SAs still referenced by a tunnel
  protection are reported in `InUse` and left alone (P11 decides about the tunnel);
* `SweepAndAck` calls `renderer.AckRestart` only when the sweep finished without error.
* Not swept: charon's SPDs and bypass policies — a fresh charon's SPD holds only bypass policies
  until its first CHILD_SA and cannot be told apart from a stale one (DF-5-questions Q10).

## Secrets (contract for P11)

* `IpsecSa.crypto_key` / `integ_key` are **references**, never material: `sha256:<hex sha256 of
  the key bytes>`. Create resolves them through `vpn.Resolver` (the agent's secret store; tests use
  `vpn.MapResolver`), verifies material ↔ reference, sends `ipsec_sad_entry_add_v2`, then zeroes
  the request buffer.
* `ipsec_sa_v5_dump` returns the key material in clear. Retrieve hashes it into the same reference
  and zeroes the buffer before the value leaves the function. So Retrieve == desired without any
  secret in a Value, key, Meta, error or log line, applying the same desired state twice yields an
  empty plan, and this survives an agent restart (no cache involved).
* A key change is a different reference → proto diff → `Update` → `ErrRecreate` (P05 deletes and
  re-creates the SA and re-creates its dependents: SPD entries, tunnel protection).
* Resolvers must be opaque to `fmt` (`%+v` walks unexported fields without calling `String`):
  `MapResolver` keeps material behind a pointer; any production resolver must do the same.
  `TestNoMaterialInOutput` checks raw, decimal and hex encodings of every test key in `%v`, `%+v`
  and slog JSON of values, keys, metas, errors and the descriptor itself.
* The reference format vs D-051 (`psk/<name>`) is open (DF-5-questions Q11).

## VPP limitations and how they are handled

* **SPD pool index** — `ipsec_spd_interface_details` carries the SPD's pool index, not its
  `spd_id`, and no dump maps one to the other. The binding's ownership record stores both (learnt
  by dumping right after the bind), so Retrieve reports the right `spd_id` also after an agent
  restart with the persisted store (closes Q7 of the first pass).
* **Salt byte order** — `ipsec_sa_v5_details` converts `salt` by hand and the generated endian pass
  swaps it again (verified on 26.06: `0x1234` comes back as `0x34120000`); Retrieve swaps it back.
* **Anti-replay window** — required (power of two ≥ 64) exactly when `use_anti_replay`; decoded
  only when the flag is set so defaults compare equal.
* **UDP encap ports** — must be explicit when `udp_encap` (VPP would otherwise pick 4500; the host
  rule forbids 500/4500/51820), must be 0 otherwise.
* **`ipsec_sad_entry_update`** (tunnel / UDP ports in place) is not used: all SA changes recreate,
  which keeps the SA ↔ key reference strictly immutable.
* **SPD protocol "any"** — `ipsec_spd_entry_add_del_v2` stores `protocol` exactly as sent; only the
  old v1 handler maps 0 to `IPSEC_POLICY_PROTOCOL_ANY` (255). Desired 0 = any is sent as 255,
  Retrieve maps 255 back to 0, a desired 255 is refused, literal protocol 0 cannot be expressed.
* **SPD delete** unbinds every interface bound to the SPD (VPP) — only ours, since the SPD is ours.
* **Backends** — VPP 26.06 on the dev host returns an empty `ipsec_backend_dump`.
* **Async mode** — no getter → write-only; `ipsec_set_async_mode` stores the flag and re-sets every
  SA's crypto op data (idempotent, D-076). Globals owner only; not exercised on the host.

## Port and id scheme (tests)

`20000 + 100*slot + k`: k=0 SA UDP encap (ipsec), k=1–2 IKEv2 ipsec-over-udp, k=10–11 WireGuard
listen/peer ports. Never 500 / 4500 / 51820. Ids: `VRX_VPP_TABLE_BASE + 1…499` descriptors,
`+500…599` the simulated charon of the sweep test.

## Tests

* Unit (`go test ./internal/descriptors/ipsec/`): stateful fake modelling the pool-index and salt
  quirks and duplicate adds (VPP errors); a fake D-080 identity (`vpntest.NewFakeBoot`, RestartVPP).
  Create, idempotent re-apply, update/ErrRecreate, delete, second delete = no VPP call, stale
  index / reused id refused, records expire on VPP restart, agent restart with the same store,
  stranger store adopts nothing, D-069 name resolution (foreign / VPP name / untagged), globals
  roles, charon sweep (orphan, live, in-use, own SA, failing delete → no ack), no material in output.
* Host (`VRX_INTEGRATION=1`, shared lab lock, slot prefix, one package at a time): `TestIpsecOnHost`
  runs through **P05's reconciler** (`vpntest.Agent` = scheduler + DF-1 alias + these descriptors,
  persisted record store): apply → Retrieve == desired per object type (tagged and untagged
  loopback bindings, SAs, protect on an ipip fixture, itf) → in-place SA swap = 1 Update → same
  desired state = empty plan → **restart simulation**: fresh connection + fresh descriptors + same
  record store = empty plan; a store without our records retrieves and adopts nothing; SA + NIC
  binding deleted via the API → plan = exactly their two creates → re-applied → empty → non-owner
  globals (backend requirement, async mode refused) → charon sweep → the empty desired state
  deletes all of ours and nothing else. `VRX_DF5_PAUSE=<s>` holds the objects for `vppctl show`
  evidence (VPP prints SA keys in `show ipsec sa <id>` detail — evidence is captured through a
  redaction filter).
