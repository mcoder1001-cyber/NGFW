# bfd descriptors (DF-7, WBS D3.8)

Package `apps/agent/internal/descriptors/bfd` — VPP BFD (UDP, single hop). Messages only from
`apps/agent/binapi/bfd`. DF-7 conventions: see `policer.md`.

| Object type | Key | Create / Update / Delete | Retrieve | Notes |
|---|---|---|---|---|
| `bfd.auth-key` | `bfd.auth-key/<conf key id>` | `bfd_auth_set_key` (secret from the `bfd.Secrets` resolver) ; Update = ErrRecreate / `bfd_auth_del_key` | `bfd_auth_keys_dump` (id, type; Meta = use count) | types keyed-sha1 / meticulous-keyed-sha1 (the only ones VPP supports); secret ≤ 20 bytes, never in a Value, error or log; request buffer wiped after sending; rotate = new conf-key id (VPP refuses to change a key in use); ids by `df7.WithIDRange` |
| `bfd.udp-session` | `bfd.udp-session/<if>/<local>/<peer>` | `bfd_udp_add` (+ auth) then `bfd_udp_session_set_flags` for admin-down; Update in place: `bfd_udp_mod` (timers), `bfd_udp_auth_activate` / `_deactivate`, set_flags; `bfd_udp_del` (BFD_ENOENT = gone) | `bfd_udp_session_dump` | timers in µs; `admin_down` decoded from state AdminDown; live state (down/init/up) is not in the Value |
| `bfd.echo-source` (**global**) | `bfd.echo-source/global` | `bfd_udp_set_echo_source` (Update in place) / `bfd_udp_del_echo_source` only while still ours | `bfd_udp_get_echo_source` | one per VPP → `bfd.RegisterGlobals` only (D-071) |

Events (`StreamEvents`): `bfd.WatchEvents(ctx, client, owner)` sends `want_bfd_events` (this process' PID), watches
`bfd_udp_session_event` and delivers `SessionEvent{Key, Interface, Local, Peer, State}` (`State` ∈ admin-down,
down, init, up) for this owner's sessions; the registration is withdrawn when ctx ends. `bfd.Sessions` gives the
live state per session key. Host run: `{Key:bfd.udp-session/loop1060/10.10.60.1/10.10.60.3 … State:admin-down}`.

Dependencies: session → `interface/<if>` + `bfd.auth-key/<id>` when authenticated. VPP 26.06 does not require the
local address to be configured on the interface, so no `interface-ip` dependency is declared (the prefix length is
not part of the session; DF-7-questions.md). FIB entries: none.
