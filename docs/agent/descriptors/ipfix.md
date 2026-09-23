# ipfix export descriptors (DF-8, WBS D7.6)

Package `apps/agent/internal/descriptors/ipfix` — VPP's IPFIX exporters (vnet `ipfix-export`), the classify report
stream and the classify tables reported over IPFIX. Message names only from `apps/agent/binapi/ipfix_export`.
`ipfix.Register(registry, client, opts...)`; options `WithVRFKey`, `WithClassifyTableKey`, `WithCollectorScope`,
`WithClassifyTableScope`.

| Object type | Key | VPP messages | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| `ipfix.default-exporter` (exporter 0, singleton) | `ipfix.default-exporter/global` | `set_ipfix_exporter`; delete = collector 0.0.0.0 (VPP's "disabled") | `ipfix_exporter_dump` while the collector is set | in place | `vrf/<id>` optional |
| `ipfix.exporter` (additional) | `ipfix.exporter/<collector-ip>` | `ipfix_exporter_create_delete` is_create=1 / 0 | `ipfix_all_exporter_get` (cursor), without entry 0, collectors in scope | in place (create on an existing collector reconfigures it) | `vrf/<id>` optional |
| `ipfix.classify-stream` (singleton) | `ipfix.classify-stream/global` | `set_ipfix_classify_stream`; delete = domain 0 / port 0 ("unset") | **write-only** (VPP bug, below) | in place | `ipfix.default-exporter/global` optional |
| `ipfix.classify-table` | `ipfix.classify-table/<classify table index>` | `ipfix_classify_table_add_del` | **write-only** (VPP bug, below) | `ErrRecreate` | `ipfix.classify-stream/global` (mandatory: VPP refuses tables before the stream), `classify-table/<id>` optional (DF-2 key, default scheme until DF-2 merges) |

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
- Ownership: exporters by collector address (`WithCollectorScope`; tests `10.<N>.0.0/16`); classify tables by
  `WithClassifyTableScope`. Singletons are VPP-global: reported only when set (not at VPP's defaults); the host test
  skips when someone else set them and resets them in Cleanup.
- VPP 26.06 has no `show ipfix …` CLI; the evidence is the Retrieve log of the host run.
