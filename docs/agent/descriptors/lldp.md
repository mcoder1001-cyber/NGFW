# lldp descriptors (DF-7, WBS D1.7)

Package `apps/agent/internal/descriptors/lldp` — VPP LLDP plugin. Messages only from `apps/agent/binapi/lldp`.
DF-7 conventions: see `policer.md`.

| Object type | Key | Create / Update / Delete | Retrieve | Notes |
|---|---|---|---|---|
| `lldp.global` (**global**) | `lldp.global/global` | `lldp_config` (Update in place) / Delete restores tx-hold 4, tx-interval 30 | **write-only** (no getter) | registered only by `lldp.RegisterGlobals` (D-071); an empty system name leaves VPP's unchanged (it cannot be unset) |
| `lldp.interface` | `lldp.interface/<if>` | `sw_interface_set_lldp` enable (port-desc, mgmt ip4/ip6/oid); Update = disable + enable (VPP ignores new parameters on an enabled interface); disable | **write-only** | Create verifies with `lldp_dump` that VPP enabled LLDP on the requested interface (`ErrIndexMismatch` otherwise) |

`lldp.Neighbours(ctx, client)` returns the neighbour table (`lldp_dump`, cursor-paginated): read-only state.
`lldp_dump` lists enabled interfaces with what was heard, not the configuration → no Retrieve (D-063 forbids
echoing desired state).

Dependencies: interface → `lldp.global/global` (optional) + `interface/<if>`.

## VPP 26.06 quirk

`sw_interface_set_lldp` passes the **sw_if_index** to `lldp_cfg_intf_set`, which treats it as a **hw_if_index**
(and the disable path looks the interface up by `hi->sw_if_index` as a hw index). Correct only where both indexes
coincide (NICs created at start-up); on the shared host a mismatch would enable LLDP on another interface. The
descriptor detects it by diffing `lldp_dump` and fails loudly with `ErrIndexMismatch` "… NOT undone"; nothing is
claimed. An automatic undo is not possible (review M6, investigated in `lldp_cli.c`): the enable keys the entry by
hw index X (the API's sw_if_index), `lldp_dump` reports it as hw(X).sw_if_index = Y, and the disable looks entries up
by hw(arg).sw_if_index — a disable with X removes the entry keyed Y (another interface's LLDP), and the argument that
reaches key X (the hw index of our interface) is not exposed by any API message. So no disable is sent; the stray
entry stays until a VPP restart (DF-7-questions Q8, V-item candidate). Delete is safe: a claim exists only after the
dump showed our own sw_if_index, i.e. hw(X).sw_if_index = X; the host test
finds an aligned loopback via `df7test.AlignedLoopback` (test-only `show hardware-interfaces` read through
`cli_inband`) or skips. FIB entries: none.
