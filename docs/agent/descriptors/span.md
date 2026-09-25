# span descriptor (DF-7, WBS D1.10)

Package `apps/agent/internal/descriptors/span` — VPP port mirroring. Messages only from `apps/agent/binapi/span`.
DF-7 conventions: see `policer.md`.

| Object type | Key | Create / Update / Delete | Retrieve | Notes |
|---|---|---|---|---|
| `span.mirror` | `span.mirror/<src>/<dst>/<device\|l2>` | `sw_interface_span_enable_disable` state rx/tx/both (Update = new state in place) / state disabled | `sw_interface_span_dump` with `is_l2` false and true | one object per source→destination pair and level; the destination may be any non-foreign interface (e.g. a DF-6 GRE/ERSPAN tunnel: only its `interface/<name>` key is used) |

Dependencies: `interface/<src>`, `interface/<dst>`. Ownership: the source interface (claim rule for untagged).
Delete re-resolves both interfaces by logical name; a vanished interface is success. FIB entries: none.

F-loopback-bvi-gso-lldp-span: projected from `interfaces.<src>.mirror[]` (`internal/desired/mirror.go`; `direction` →
state, `level: l2` → `is_l2`); Retrieve's sessions are assembled back in the stored document's order. A destination
deleted behind the agent's back leaves the session in VPP's span bookkeeping of our source (span.c has no
interface-delete hook — V-new (F-loopback-bvi-gso-lldp-span)); Retrieve reports it as `span.mirror/<src>/#<sw_if_index>/…`
and Delete clears exactly that bit of our source (`TestStaleDestinationCleared`). ERSPAN: the destination is a GRE tunnel
of type erspan by its logical name (host check `TestERSPANOnHost` with DF-6's `gre.tunnel` descriptor). Ownership
declaration (TD-11b): `CheckPersistent` over the owner's claim store.
