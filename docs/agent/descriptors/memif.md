# Descriptors — memif plugin (DF-1)

Package `apps/agent/internal/descriptors/memif`, models `memif_model.proto` (agent-internal stand-in, D-055).
`memif.Register(r, client, owner, memif.WithSocketDir(dir))`.

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Notes / limitations |
|---|---|---|---|---|---|
| `memif.socket` | `memif.socket/<socket id>` | — | `memif_socket_filename_add_del_v2`; Retrieve `memif_socket_filename_dump` | ErrRecreate | No tag in the API: ownership = the socket file lives **directly in the owner's socket dir** — `/run/vrx/memif` for the production owner `vrx`, `/run/vrx-test/<owner>/memif` otherwise (shared-host rules). Create makes the dir (0750). Socket id 0 is VPP's default socket and never ours. |
| `memif.memif` | `memif.memif/<name>` | `memif.socket/<socket id>` (an owned socket is mandatory: socket 0 is VPP's shared default — review L4) | `memif_create_v2`, `memif_delete`, `sw_interface_tag_add_del`; Retrieve `memif_dump` (+ owner tag from `sw_interface_dump`) | ErrRecreate | role master / slave, mode ethernet / ip / punt-inject, `id`, zero-copy. **Not modelled** (cannot round-trip): ring_size / buffer_size (`memif_details` reports the values negotiated with the peer, 0 before it connects), queue counts, secret, hw address (use `interface.mac-address`). VPP defaults apply (1024 / 2048). If tagging a new memif fails it is deleted again (review M3). Note (review L7): `memif_details.hw_addr` is reported and could round-trip in a later version. |
