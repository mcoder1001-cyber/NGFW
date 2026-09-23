# pppoe descriptors (DF-6, WBS D6.6)

Package `apps/agent/internal/descriptors/pppoe`. Messages only from `apps/agent/binapi/pppoe`. Shared rules: [df6.md](df6.md).

| Object type | Descriptor / key | Create / Delete | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| PPPoE session | `pppoe.session` · `pppoe.session/<client_mac>/<session_id>` | `pppoe_add_del_session` + owner tag | `pppoe_session_dump` + tag | `ErrRecreate` | `vrf/<decap_vrf_id>` |
| PPPoE control-plane interface | `pppoe.cp` · `pppoe.cp/<interface>` | `pppoe_add_del_cp` | **write-only** | — | `interface/<interface>` |

Model `pppoe.Session`: `session_id` (1–65535), `client_ip`, `client_mac`, `decap_vrf_id`.

Limitation: VPP creates a session only for a client MAC that `pppoe-input` has learned from PPPoE discovery packets;
otherwise `pppoe_add_del_session` returns INVALID_SW_IF_INDEX, mapped to `pppoe.ErrClientNotLearned`. The host has no
PPPoE clients and DF-6 sends no packets, so the host test verifies that typed error and skips the create/retrieve part
(Q3). PPPoE client/server daemons are out of scope.
