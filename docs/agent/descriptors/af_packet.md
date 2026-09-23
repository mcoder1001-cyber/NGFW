# Descriptors — af_packet plugin (DF-1)

Package `apps/agent/internal/descriptors/af_packet` (Go package `afpacket`), model `afpacket_model.proto`
(agent-internal stand-in, D-055). `afpacket.Register(r, client, owner)`.

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Notes / limitations |
|---|---|---|---|---|---|
| `af-packet.host-interface` | `af-packet.host-interface/<name>` | nothing in VPP (the Linux netdev is a precondition) | `af_packet_create_v3`, `af_packet_delete` (by host_if_name), `sw_interface_tag_add_del`; Retrieve `af_packet_dump` (+ `sw_interface_dump` for tag and mode) | ErrRecreate | VPP names it `host-<host_if_name>`. mode ethernet / ip (decoded from the presence of an L2 address: ip-mode interfaces use the ip hw class). **Not modelled**: flags (qdisc-bypass, cksum-gso, version-2), frame sizes/counts, queue counts — `af_packet_details` reports only sw_if_index and host_if_name, so they could never round-trip; VPP defaults apply. af-packet queues start in **interrupt** mode (see `interface.rx-mode`). |

Tests create the veth pair `w<N>-afXX` / `w<N>-afXXp` with a fixed-argv `ip link` rig helper
(test-only, marked `ALLOW:`), delete it in Cleanup even on failure, and never assign addresses.
