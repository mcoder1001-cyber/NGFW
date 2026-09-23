# pcap capture descriptors (DF-8, WBS D8.2)

Package `apps/agent/internal/descriptors/pcap` — VPP's built-in pcap dispatch capture (vnet `interface.api`) and the
filter function it uses. Message names only from `apps/agent/binapi/interface`. `pcap.Register(registry, client, owner, opts...)`.

| Object type | Key | VPP messages | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| `pcap.capture` (singleton, one per VPP) | `pcap.capture/global` | `pcap_trace_on` / `pcap_trace_off` | **write-only** (no status message) | `ErrRecreate` | `interface/<ifname>` when not `any`; `pcap.filter-function/global` optional |
| `pcap.filter-function` (singleton) | `pcap.filter-function/global` | `pcap_set_filter_function`; delete = `vnet_is_packet_traced` (default) | **write-only** | in place | `trace.bpf-filter/global` optional when the name is `bpf_trace_filter` |

Value fields: `Capture{rx, tx, drop (≥1), interface ("any" or an owned interface), max_packets > 0,
max_bytes_per_packet 32..9000, filter, error ("node/error", optional), file}`; `FilterFunction{name}`.

## Notes and limitations
- **One capture per VPP.** `pcap_trace_on` while a capture runs is refused (`INVALID_VALUE`): Create returns
  `ErrCaptureBusy` unless the running capture is this process's identical one (write-only re-apply). Delete sends
  `pcap_trace_off` only when this process started the capture — never stops another slot's capture.
  `pcap_trace_off` answers `NO_SUCH_ENTRY` when no packet was captured (the capture is stopped, no file written) and
  `VALUE_EXIST` when nothing ran; both count as deleted.
- **Capture file under /tmp:** VPP takes a bare file name and writes `/tmp/<file>` (`unformat_vlib_tmpfile` rejects
  "/" and ".."), so files cannot live under `/run/vrx-test/<prefix>/` as the prompt asked; Validate rejects paths,
  tests use `/tmp/<prefix>-df8.pcap` and remove it. Recorded as a decision in `DF-8.md`.
- `interface: "any"` is sw_if_index 0 in `pcap_trace_on` — VPP's "any" wildcard, not `local0`.
- No pcap status/dump message exists (`pcap trace status` is CLI only) → write-only (D-063).
- The filter function names are VPP-registered trace filter functions (`vnet_is_packet_traced`, `bpf_trace_filter`);
  an unknown name fails with retval -1.
