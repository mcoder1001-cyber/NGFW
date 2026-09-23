# pcap capture descriptors (DF-8, WBS D8.2)

Package `apps/agent/internal/descriptors/pcap` — VPP's built-in pcap dispatch capture (vnet `interface.api`) and the
filter function it uses. Message names only from `apps/agent/binapi/interface`. `pcap.Register(registry, client, owner, opts...)` (+ `RegisterGlobals` for the filter function).

| Object type | Key | VPP messages | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| `pcap.capture` (singleton, one per VPP) | `pcap.capture/global` | `pcap_trace_on` / `pcap_trace_off` | **write-only** (no status message) | `ErrRecreate` | `interface/<ifname>` when not `any`; `pcap.filter-function/global` optional |
| `pcap.filter-function` (singleton) | `pcap.filter-function/global` | `pcap_set_filter_function`; delete = `vnet_is_packet_traced` (default) | **write-only** | in place | `trace.bpf-filter/global` optional when the name is `bpf_trace_filter` |

Value fields: `Capture{rx, tx, drop (≥1), interface ("any" or an owned interface), max_packets > 0,
max_bytes_per_packet 32..9000, filter, error ("node/error", optional), file}`; `FilterFunction{name}`.

## Notes and limitations
- **One capture per VPP.** `pcap_trace_on` while a capture runs is refused (`INVALID_VALUE`): Create returns
  `ErrCaptureBusy` unless this owner recorded the identical capture for the running VPP process (write-only re-apply,
  D-076 below). Delete sends `pcap_trace_off` only when this owner started the capture on the running VPP process —
  never stops another slot's capture.
  `pcap_trace_off` answers `NO_SUCH_ENTRY` when no packet was captured (the capture is stopped, no file written) and
  `VALUE_EXIST` when nothing ran; both count as deleted.
- **Capture file under /tmp:** VPP takes a bare file name and writes `/tmp/<file>` (`unformat_vlib_tmpfile` rejects
  "/" and ".."), so files cannot live under `/run/vrx-test/<prefix>/` as the prompt asked; Validate rejects paths,
  tests use `/tmp/<prefix>-df8.pcap` and remove it. Recorded as a decision in `DF-8.md`.
- `interface: "any"` is sw_if_index 0 in `pcap_trace_on` — VPP's "any" wildcard, not `local0`.
- No pcap status/dump message exists (`pcap trace status` is CLI only) → write-only (D-063).
- The filter function names are VPP-registered trace filter functions (`vnet_is_packet_traced`, `bpf_trace_filter`);
  an unknown name fails with retval -1.

## Registration, ownership and restarts (D-069, D-071, D-074, D-076)
- `pcap.Register(...)` registers the per-owner object types (`pcap.capture`); `pcap.RegisterGlobals(...)` registers the
  VPP-global singletons (`pcap.filter-function`) constructed as **globals owner** — P08 calls it only in the designated globals
  owner's agent (D-071), before `Register`. A descriptor constructed without the role (`dfkit.GlobalsOwner(false)`)
  only *requires* the value: Create succeeds when VPP already has it (checked through the getter where one exists,
  otherwise `dfkit.ErrNotGlobalsOwner`), Delete is a no-op, Retrieve is write-only.
- Interfaces are named by their **logical name** and resolved with DF-1's `iface.ResolveName` (D-069): this owner's
  tag id first, then an untagged interface's VPP name; another owner's interface fails with
  `iface.ErrForeignInterface`, local0 never resolves. Objects on an **untagged** interface (a DPDK NIC) are recorded
  in the owner's ClaimStore (`iface.Claims`, shared with DF-1; P05/P08 install a persisted one) on Create, released on
  Delete, and reported by Retrieve only while claimed (D-071 claim rule).
- Deletes re-resolve the logical name right before acting by sw_if_index (never a Meta index — indexes are reused
  after a VPP restart) and first check that the object still exists (D-074); "already gone" is success.
- Retrieve never reports a key twice (`dfkit.Dedupe`).
- **D-076:** `pcap_trace_on` is not idempotent (`INVALID_VALUE` while a capture runs); the applied capture is recorded
  in the owner's BootStore (`dfkit.Boot`, keyed by the VPP main-thread PID) so a resync skips the re-add and Delete
  stops only a capture this owner started on the running VPP process; after a VPP restart it is started once more.
- Restart simulation (fresh connection + fresh descriptors → empty plan; objects deleted via binapi → exactly their
  re-creation planned → empty plan again): `internal/descriptors/dfkit/restarttest`, output in `DF-8.md`.
