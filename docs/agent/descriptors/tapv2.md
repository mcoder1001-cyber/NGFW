# Descriptors — tapv2 plugin (DF-1)

Package `apps/agent/internal/descriptors/tapv2`, model `tapv2_model.proto` (agent-internal stand-in, D-055).
`tapv2.Register(r, client, owner)`.

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Notes / limitations |
|---|---|---|---|---|---|
| `tapv2.tap` | `tapv2.tap/<name>` | — | `tap_create_v3` (owner tag in `tag`), `tap_delete_v2`; Retrieve `sw_interface_tap_v2_dump` (+ `sw_interface_dump` for the tag) | ErrRecreate (a tap is immutable in VPP) | `id` mandatory (VPP names it `tap<id>`); **host_if_name mandatory** (≤ 15 bytes, tests `w<N>-*`; empty would let VPP pick one and plan a recreate on every resync — review M4), host namespace, host bridge, host IPv4/IPv6 prefix (canonical via `netip`), host MTU, rx/tx ring sizes (always explicit, VPP default 256), gso, csum-offload. One rx/tx queue. **Not modelled**: host-side MAC (VPP always generates and reports one, an unset value could never round-trip), VPP-side MAC (`interface.mac-address`), host gateways and queue counts (not in the dump). VPP device class (`interface_dev_type`) is `tap` (host-verified). |

Tests create Linux netdevs `w<N>-tapXX` and never assign management addresses; Cleanup deletes them
even when a test fails.
