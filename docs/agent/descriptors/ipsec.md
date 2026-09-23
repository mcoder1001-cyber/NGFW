# Descriptors: ipsec (DF-5, WBS D6.1)

Package `apps/agent/internal/descriptors/ipsec`, desired-state types in
`apps/agent/internal/descriptors/vpn/pb/vpn.proto` (agent-internal until P03b moves VPN leaf
messages into `packages/proto`, D-055). Entry point: `ipsec.Register(registry, client, owner,
ipsec.WithSecrets(resolver), ipsec.WithIDRange(lo, hi))`. All message names come from
`apps/agent/binapi/ipsec` (VPP 26.06).

## Object ↔ message table

| Descriptor (`Name()`) | Key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| `ipsec.spd` | `ipsec.spd/<spd_id>` | `ipsec_spd_add_del` (is_add 1/0); Update = ErrRecreate | `ipsec_spds_dump` | — | owned by id range |
| `ipsec.spd-interface` | `ipsec.spd-interface/<interface>` | `ipsec_interface_add_del_spd`; Update = ErrRecreate | `ipsec_spd_interface_dump` + `sw_interface_dump` | `ipsec.spd/<id>`, interface (see below) | pool-index limitation below |
| `ipsec.spd-entry` | `ipsec.spd-entry/<spd>/<dir>/<prio>/<action>/<sa>/<proto>/<l-range>/<l-ports>/<r-range>/<r-ports>` | `ipsec_spd_entry_add_del_v2`; Update = ErrRecreate (every field is identity in VPP) | `ipsec_spd_dump` per owned SPD | `ipsec.spd/<id>`; `ipsec.sa/<sa_id>` for action `protect` | desired protocol 0 = any, **sent as 255** (see below) |
| `ipsec.sa` | `ipsec.sa/<sad_id>` | `ipsec_sad_entry_add_v2` / `ipsec_sad_entry_del`; Update = ErrRecreate | `ipsec_sa_v5_dump` | `vrf/<table_id>` (Optional) for a tunnel SA in a non-zero table | keys are secret references; salt byte-swap below |
| `ipsec.tunnel-protect` | `ipsec.tunnel-protect/<interface>[/<nh>]` | `ipsec_tunnel_protect_update` (Create and in-place Update: swap `sa_out` / `sa_in`) / `ipsec_tunnel_protect_del`; other interface or nh = ErrRecreate | `ipsec_tunnel_protect_dump` | interface, `ipsec.sa/<sa_out>`, every `ipsec.sa/<sa_in>` | nh only for p2mp |
| `ipsec.itf` | `ipsec.itf/ipsec<instance>` | `ipsec_itf_create` (+ `sw_interface_tag_add_del`) / `ipsec_itf_delete`; Update = ErrRecreate | `ipsec_itf_dump` + `sw_interface_dump` (owner tag) | — | mode `p2p` / `p2mp`; provides the alias `interface/ipsec<instance>` (`ProvidedKeys`) |
| `ipsec.backend` | `ipsec.backend/<esp\|ah>` | `ipsec_select_backend` (by name → index from the dump); Delete = no-op | `ipsec_backend_dump` (active backend per protocol) | — | global singleton; VPP 26.06 on the host reports **no** backends |
| `ipsec.async-mode` | `ipsec.async-mode/global` | `ipsec_set_async_mode` (idempotent); Delete = no-op | **write-only**: `ErrRetrieveUnsupported` (no getter, D-063) | — | global; not exercised on the host (no workers) |

Interface dependency (D-065): always the alias `interface/<name>` — DF-1's alias descriptor for
loopbacks / DF-6 tunnels / NICs; `ipsec.itf` provides `interface/ipsec<N>` itself through P05's
optional `KeyProvider.ProvidedKeys`, so tunnel-protect on an ipsec interface orders after it.

Write-only (D-063): a descriptor whose VPP object has no dump returns (wrapped)
`vpn.ErrRetrieveUnsupported` — the same message as P05's `scheduler.ErrRetrieveUnsupported`, so
`scheduler.IsRetrieveUnsupported` recognises it. It never echoes cached desired state; the
reconciler re-applies it on resync, so Create is idempotent.

## Ownership on a shared VPP

* Interfaces (`ipsec.itf`) carry the owner tag `<owner>:ipsec<N>`; `spd-interface` and
  `tunnel-protect` are retrieved only on interfaces tagged by the owner.
* SPDs and SAs have neither tag nor name: ownership is the numeric range from `WithIDRange`
  (tests: the slot range `VRX_VPP_TABLE_BASE..+999`, e.g. 4000–4999 for w4). Create refuses an id
  outside the range, so a test can never create something it would not retrieve. Production: the
  zero range = every id.
* `ipsec.backend` / `ipsec.async-mode` are plugin-wide and unowned; Delete never touches VPP.

## Secrets (contract for P11)

* `IpsecSa.crypto_key` / `integ_key` are **references**, never material: `sha256:<hex sha256 of
  the key bytes>`. Create resolves them through `vpn.Resolver` (the agent's secret store; tests use
  `vpn.MapResolver`), verifies material ↔ reference, sends `ipsec_sad_entry_add_v2`, then zeroes
  the request buffer.
* `ipsec_sa_v5_dump` returns the key material in clear. Retrieve hashes it into the same reference
  and zeroes the buffer before the value leaves the function. So Retrieve == desired without any
  secret in a Value, key, Meta, error or log line, applying the same desired state twice yields an
  empty plan, and this survives an agent restart (no cache involved).
* A key change is a different reference → proto diff → `Update` → `ErrRecreate` (the scheduler
  deletes and re-creates the SA and re-creates its dependents: SPD entries, tunnel protection).
* Resolvers must be opaque to `fmt` (`%+v` walks unexported fields without calling `String`):
  `MapResolver` keeps material behind a pointer; any production resolver must do the same.
  `TestNoMaterialInOutput` checks raw, decimal and hex encodings of every test key in `%v`, `%+v`
  and slog JSON of values, keys, metas, errors and the descriptor itself.

## VPP limitations and how they are handled

* **SPD pool index** — `ipsec_spd_interface_details` carries the SPD's pool index, not its
  `spd_id`, and no dump maps one to the other. The descriptor learns index → id when it binds (dump
  right after `ipsec_interface_add_del_spd`). After an agent restart an unknown index is retrieved
  with `spd_id: 0`: the diff plans an Update → ErrRecreate → re-bind, which teaches the map again.
  Delete of such a binding unbinds with any owned SPD id (VPP only needs an existing id).
* **Salt byte order** — `ipsec_sa_v5_details` converts `salt` by hand and the generated endian pass
  swaps it again (verified on 26.06: `0x1234` comes back as `0x34120000`); Retrieve swaps it back.
* **Anti-replay window** — required (power of two ≥ 64) exactly when `use_anti_replay`; decoded
  only when the flag is set so defaults compare equal.
* **UDP encap ports** — must be explicit when `udp_encap` (VPP would otherwise pick 4500; the host
  rule forbids 500/4500/51820), must be 0 otherwise.
* **`ipsec_sad_entry_update`** (tunnel / UDP ports in place) is not used: all SA changes recreate,
  which keeps the SA ↔ key reference strictly immutable.
* **SPD protocol "any"** — `ipsec_spd_entry_add_del_v2` stores `protocol` exactly as sent; only the
  old v1 handler maps 0 to `IPSEC_POLICY_PROTOCOL_ANY` (255). Sending 0 installs a policy for IP
  protocol 0 (HOPOPT) — found on the host (`show ipsec spd` printed `protocol
  IP6_HOP_BY_HOP_OPTIONS`) and fixed: desired 0 = any is sent as 255, Retrieve maps 255 back to 0,
  a desired 255 is refused, literal protocol 0 cannot be expressed.
* **Backends** — VPP 26.06 on the dev host returns an empty `ipsec_backend_dump`; the integration
  test is read-only and re-selects the active backend only if there is one.
* **Async mode** — no getter → write-only; skipped on the host (no worker threads).

## Port scheme (tests)

`20000 + 100*slot + k`: k=0 SA UDP encap (ipsec), k=1–2 IKEv2 ipsec-over-udp, k=10–11 WireGuard
listen/peer ports. Never 500 / 4500 / 51820.

## Tests

* Unit (`go test ./internal/descriptors/ipsec/`): stateful fake modelling the pool-index and
  salt quirks; create, idempotent re-apply, update/ErrRecreate, delete, dependency keys, owner
  filtering, VPP errors, disconnected client, no material in output.
* Integration (`VRX_INTEGRATION=1`, shared lab lock, slot prefix): `TestIpsecOnHost` — loopback and
  ipip fixtures (ipip via `binapi/ipip` until DF-6 merges), every object type created → Retrieve ==
  desired → second plan empty (`vpntest.MustEmptyPlan`) → deleted → gone. `VRX_DF5_PAUSE=<s>`
  holds the objects for `vppctl show` evidence (VPP prints SA keys in `show ipsec sa <id>` detail —
  evidence is captured through a redaction filter).
