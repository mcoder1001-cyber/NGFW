# span descriptor (DF-7, WBS D1.10)

Package `apps/agent/internal/descriptors/span` — VPP port mirroring. Messages only from `apps/agent/binapi/span`.
DF-7 conventions: see `policer.md`.

| Object type | Key | Create / Update / Delete | Retrieve | Notes |
|---|---|---|---|---|
| `span.mirror` | `span.mirror/<src>/<dst>/<device\|l2>` | `sw_interface_span_enable_disable` state rx/tx/both (Update = new state in place) / state disabled | `sw_interface_span_dump` with `is_l2` false and true | one object per source→destination pair and level; the destination may be any non-foreign interface (e.g. a DF-6 GRE/ERSPAN tunnel: only its `interface/<name>` key is used) |

Dependencies: `interface/<src>`, `interface/<dst>`. Ownership: the source interface (claim rule for untagged).
Delete re-resolves both interfaces by logical name; a vanished interface is success. FIB entries: none.
