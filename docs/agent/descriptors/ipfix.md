# ipfix export descriptors (DF-8, WBS D7.6)

Package `apps/agent/internal/descriptors/ipfix` — VPP's IPFIX exporters (vnet `ipfix-export`), the classify report
stream and the classify tables reported over IPFIX. Message names only from `apps/agent/binapi/ipfix_export`.
`ipfix.Register(registry, client, classifyStore, opts...)` (+ `ipfix.RegisterGlobals` for exporter 0 and the classify stream); options `WithVRFKey`, `WithCollectorScope`,
`WithGlobals`.

| Object type | Key | VPP messages | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| `ipfix.default-exporter` (exporter 0, singleton) | `ipfix.default-exporter/global` | `set_ipfix_exporter`; delete = collector 0.0.0.0 (VPP's "disabled") | `ipfix_exporter_dump` while the collector is set | in place | `vrf/<id>` optional |
| `ipfix.exporter` (additional) | `ipfix.exporter/<collector-ip>` | `ipfix_exporter_create_delete` is_create=1 / 0 | `ipfix_all_exporter_get` (cursor), without entry 0, collectors in scope | in place (create on an existing collector reconfigures it) | `vrf/<id>` optional |
| `ipfix.classify-stream` (singleton) | `ipfix.classify-stream/global` | `set_ipfix_classify_stream`; delete = domain 0 / port 0 ("unset") | **write-only** (VPP bug, below) | in place | `ipfix.default-exporter/global` optional |
| `ipfix.classify-table` | `ipfix.classify-table/<DF-2 table name>` | `ipfix_classify_table_add_del`; the VPP index is resolved from DF-2's classify store (`classify.LiveTables`: VPP instance + geometry verified) right before every call, never kept (review H2) | **write-only** (VPP bug, below) | `ErrRecreate` | `classify.table/<name>` (mandatory, DF-2), `ipfix.classify-stream/global` optional (registered by the globals owner only, L2; VPP refuses tables before it is set) |

Value fields (`ipfix.Exporter`): collector, collector_port (≠0), src (same family), vrf (`ipfix.NoVRF` = ~0 = none),
path_mtu 68..1450, template_interval > 0, udp_checksum. Meta of `ipfix.exporter`: `ExporterMeta{StatIndex}` (0 after
Retrieve — the dump does not carry it).

## Notes and limitations
- **flowprobe and the classify reports use exporter 0 only** (`flowprobe/node.c`, `flow_api.c`): the default exporter is
  what makes flow records leave the box; additional exporters are for other report producers (NAT logging, DF-3).
  Exporter 0 takes an IPv4 collector only.
- **VPP bug → write-only classify objects:** `ipfix_classify_stream_details` / `ipfix_classify_table_details` are sent
  with the bare message id (`flow_api.c`: `ntohs (VL_API_IPFIX_CLASSIFY_*_DETAILS)` without `REPLY_MSG_ID_BASE`). On the
  host govpp received them as an unrelated message ("No subscription found for the notification message … msgId=12")
  and the dump came back empty. Retrieve returns `ErrClassifyDumpBroken` (wraps `ErrRetrieveUnsupported`); the dumps are
  not sent. `ClassifyTableDescriptor.DecodeClassifyTables` is the (unit-tested) decoder to switch to once VPP is fixed
  — listed for `docs/vpp-code-track.md` in `DF-8-questions.md`.
- `ipfix_all_exporter_get` returns exporter 0 first; `ipfix.exporter` skips it. VPP identifies additional exporters by
  collector address alone (one exporter per collector IP).
- Ownership: exporters by collector address (`WithCollectorScope`; tests `10.<N>.0.0/16`); classify tables through this owner's DF-2 classify store (a name that is not a live table of ours is `ErrNoClassifyTable`; Delete of a gone table is a no-op, a stale index is never used). Singletons are VPP-global: reported only when set (not at VPP's defaults); exporter 0 is tested
  read-first under the lab-wide lock `/run/lock/vrx-globals.lock`; the classify stream has no getter, so the classify host
  test is opt-in (`VRX_DF8_GLOBALS=1`, review M3).
- VPP 26.06 has no `show ipfix …` CLI; the evidence is the Retrieve log of the host run.

## Registration, ownership and restarts (D-069, D-071, D-074, D-076)
- `ipfix.Register(...)` registers the per-owner object types (`ipfix.exporter`, `ipfix.classify-table`); `ipfix.RegisterGlobals(...)` registers the
  VPP-global singletons (`ipfix.default-exporter`, `ipfix.classify-stream`) constructed as **globals owner** — P08 calls it only in the designated globals
  owner's agent (D-071), before `Register`. A descriptor constructed without the role (`dfkit.GlobalsOwner(false)`)
  only *requires* the value: Create succeeds when VPP already has it (checked through the getter where one exists,
  otherwise `dfkit.ErrNotGlobalsOwner`), Delete is a no-op, Retrieve is write-only.
- Interfaces are named by their **logical name** and resolved with DF-1's `iface.ResolveName` (D-069): this owner's
  tag id first, then an untagged interface's VPP name; another owner's interface fails with
  `iface.ErrForeignInterface`, local0 never resolves. Objects on an **untagged** interface (a DPDK NIC) are recorded
  in the owner's ClaimStore (`iface.Claims`, shared with DF-1; P05/P08 install a persisted one) **only after VPP
  accepted the add**, released on Delete, and reported by Retrieve only while claimed (D-071 claim rule). An object that
  already exists on an untagged interface without our claim is **never adopted** (Create fails with
  `dfkit.ErrNotOurs`, nothing is claimed) and Delete never touches it (review H1). Claims are bound to the D-080 VPP
  boot identity (kernel boot_id, VPP main PID, VPP start time — `internal/vpp/bootid` via `dfkit.BootIdentity`) and the sw_if_index, so they
  expire when VPP restarts or the name moves to another interface.
- Deletes re-resolve the logical name right before acting by sw_if_index (never a Meta index — indexes are reused
  after a VPP restart) and first check that the object still exists (D-074); "already gone" is success.
- Retrieve never reports a key twice (`dfkit.Dedupe`).
- Restart simulation (fresh connection + fresh descriptors → empty plan; objects deleted via binapi → exactly their
  re-creation planned → empty plan again): `internal/descriptors/dfkit/restarttest`, output in `DF-8.md`.
