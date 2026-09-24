# Descriptors: wireguard (DF-5, WBS D6.5)

Package `apps/agent/internal/descriptors/wireguard`, desired-state types in
`apps/agent/internal/descriptors/vpn/pb/vpn.proto`. Entry point: `peer := wireguard.Register(registry,
client, owner, wireguard.WithSecrets(resolver), wireguard.WithGlobalsOwner(globalsOwner))` — it returns the peer descriptor, which is also
the plugin's event source. Message names come from `apps/agent/binapi/wireguard` (VPP 26.06).

## Object ↔ message table

| Descriptor (`Name()`) | Key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| `wireguard.interface` | `wireguard.interface/wg<instance>` | `wireguard_interface_create` (explicit private key, `generate_key`=false) + `sw_interface_tag_add_del`; Update = ErrRecreate; `wireguard_interface_delete` | `wireguard_interface_dump` with `show_private_key=false` (never true) + `sw_interface_dump` (owner tag) | — (see src_ip below) | VPP names it `wg<user_instance>`; provides the alias `interface/wg<instance>` (`ProvidedKeys`) |
| `wireguard.peer` | `wireguard.peer/<interface>/<public key, std base64>` | `wireguard_peer_add_v2`; Update = ErrRecreate (VPP has no peer update); `wireguard_peer_remove` (peer_index from Meta) | `wireguard_peers_v2_dump` | `interface/wg<N>` (D-065 alias), `vrf/<table_id>` (Optional) when ≠ 0 | public keys are unique **VPP-wide** |
| `wireguard.async-mode` | `wireguard.async-mode/global` | owner: `wg_set_async_mode` (idempotent, D-076: sets/clears a flag); Delete = no-op | **write-only**: `ErrRetrieveUnsupported` (no getter, D-063) | — | **VPP-global** (D-071): setter only with `WithGlobalsOwner(true)`, otherwise a requirement that always fails (`ErrNotGlobalsOwner`) |

Meta: `InterfaceMeta{SwIfIndex}`, `PeerMeta{PeerIndex, SwIfIndex}` — Retrieve fills both exactly as
Create does.

Ownership (D-069, D-071, D-074): the interface carries `<owner>:wg<N>` — its logical name, VPP's
name and the tag id coincide; VPP refuses an existing instance, so nothing is adopted. Peers are
added only to **our** (tagged) WireGuard interfaces, named by logical name (another owner's →
`vpn.ErrForeignInterface`, an untagged one → `vpn.ErrNotOurs`). Deletes re-verify right before the
call: the interface index must still carry our tag for that instance (else `ErrNotOurs`, or nothing
to do when it is gone); the peer at the Meta's index must still have the same public key on the same
interface (VPP reuses peer indexes) — otherwise the peer is gone and the index is not touched. The key's id contains `/` when the base64 public key does; `Key.ID()` is everything
after the descriptor name.

## Events (StreamEvents)

`peer.Events(ctx) (<-chan wireguard.PeerEvent, error)` subscribes with
`want_wireguard_peer_events` (sw_if_index ~0, peer_index ~0, enable, pid) and watches
`wireguard_peer_event`. Event shape (JSON as StreamEvents publishes it; no key material):

```json
{ "key": "wireguard.peer/wg4001/D2Mq…Y=", "interface": "wg4001", "public_key": "D2Mq…Y=",
  "peer_index": 3, "established": true, "dead": false }
```

`established` / `dead` are the `wireguard_peer_flags` bits `WIREGUARD_PEER_ESTABLISHED` (2) and
`WIREGUARD_PEER_STATUS_DEAD` (1). Only peers on interfaces owned by this agent are delivered.

* VPP registers the client **only on peers that exist when `want_wireguard_peer_events` is sent**.
  While a subscription is active, `Peer.Create` therefore registers each new peer with
  `want_wireguard_peer_events{peer_index}`.
* Each event's peer index is resolved when it arrives with `wireguard_peers_dump{peer_index}` (the
  v1 dump — it carries no preshared key) plus the interface owner tag. Nothing is cached: peer
  indexes are reused after a peer delete or a VPP restart.
* Cancelling ctx closes the channel and sends `want_wireguard_peer_events` enable=0 (best effort).
  One subscription per descriptor at a time (`ErrEventsActive`).
* Flags are state, never part of the peer's Value.

## Secrets

* `WireguardInterface.private_key` is an `x25519:<base64 public key>` reference (Create refuses any
  other form). Create resolves the 32-byte private key, `vpn.Resolve` verifies that it derives the
  referenced public key, the request buffer is zeroed after the call. Retrieve rebuilds the
  reference from the public key in `wireguard_interface_dump` — the private key is never read back
  (`show_private_key` is never set; the field is zeroed regardless). Retrieve == desired and survives
  an agent restart; a different key is a different reference → ErrRecreate.
* `WireguardPeer.preshared_key` is a `sha256:<hex>` reference or "" (none). `wireguard_peers_v2_dump`
  returns the preshared key in clear; Retrieve hashes it into the reference and zeroes the buffer,
  also for other owners' peers (then dropped). `preshared_key_set=false` → "".
* `generate_key` is not supported: a key VPP generates could not be referenced by desired state
  (WireGuard key-generation UX is out of scope).

## Canonical form and limitations

* `allowed_ips`: masked, sorted by string, unique — Create refuses anything else; Retrieve sorts
  what VPP returns (VPP keeps insertion order). 1–255 entries (VPP refuses 0).
* `endpoint`: "" means none (VPP stores 0.0.0.0, decoded back to "").
* `src_ip` dependency: the task asked for an Optional dependency on the interface address that
  carries `src_ip`; DF-1's address key needs interface + prefix length, which the WireGuard object
  does not have, so no dependency is declared (VPP accepts an unconfigured src_ip). Question filed.
* Listen ports are registered in VPP's UDP stack by `wg_if_create`; tests use
  `20000+100·slot+10/+11`, never 51820.

## Tests

* Unit: stateful fake (public key derived from the private key, VPP-wide public-key uniqueness,
  allowed-ips reordering, PSK in clear in the v2 dump, per-peer event registration). Interface and
  peer create/re-apply/restart/ErrRecreate/delete, ownership, validation, events (existing peer,
  peer created during the subscription, foreign peer filtered, index resolved after restart without
  a v2 dump), async mode, no private/preshared key in `%v`, `%+v`, slog (raw, decimal and hex).
* Unit additions: second delete = no VPP call, stale interface index refused, stale peer index
  (now another peer) untouched, peers refused on foreign / untagged wg interfaces, globals roles.
* Host: `TestWireguardOnHost` runs through **P05's reconciler** (`vpntest.Agent`): events
  subscription, `wg<base+1>` with slot keys (test vectors = SHA-256 of
  `VRX_TEST_PSK_DF5_wg_<label>_<slot>`), two peers (with/without PSK) applied → Retrieve ==
  desired → same desired state = empty plan → **restart simulation** (fresh connection +
  descriptors) = empty plan → peer removed via the API → plan = exactly its create → re-applied →
  empty → non-owner async mode refused → the empty desired state deletes all of ours.
  `VRX_DF5_PAUSE=<s>` holds the objects for `vppctl show wireguard interface` / `show wireguard
  peer` — the former prints the private key in base64 **and** hex plus the mac-key: evidence goes
  through a redaction filter.
