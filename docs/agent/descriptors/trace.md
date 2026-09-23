# trace descriptors (DF-8, WBS D8.2)

Package `apps/agent/internal/descriptors/trace` — the BPF trace filter (`bpf_trace_filter` plugin). Message names only
from `apps/agent/binapi/bpf_trace_filter`. `trace.RegisterGlobals(registry, client)`.

| Object type | Key | VPP messages | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| `trace.bpf-filter` (singleton) | `trace.bpf-filter/global` | `bpf_trace_filter_set_v2` is_add=1 (expression, optimize) / is_add=0 | **write-only** (no getter; `show bpf trace filter` is CLI only) | in place (recompile) | — |
| `trace-filter` (tracedump) | — | **not available:** `trace_set_filters`, `trace_set_filter_function`, `trace_filter_function_dump`, `trace_v2_dump`, `trace_capture_packets`, `trace_clear_capture` are in the `tracedump` plugin, which is **not built/installed** on vrx-a (no `tracedump_plugin.so`, no `tracedump.api.json`, no binapi package) | — | — | — |
| `tracenode-interface` | — | **not available:** `tracenode_enable_disable` is in the `tracenode` plugin, not built/installed (no `.so`, no `.api.json`, no binapi) | — | — | — |
| `trace-path` | — | **no API:** the 26.06 Trace Path plugin (`src/plugins/tracepath`) has no `.api` file (CLI only) | — | — | — |

The missing plugins are in `DF-8-questions.md` (the manager builds/installs them and regenerates binapi, or drops the
object types). `binapi/trace` is the iOAM trace-profile API (`trace_profile_add/del`) — unrelated to packet tracing.

## Notes
- The expression is user input compiled by libpcap **inside VPP**: Validate bounds it (1..1024 bytes) and restricts it
  to the pcap-filter(7) alphabet (letters, digits, blanks, `. : / [ ] ( ) & | ! = < > - + * % ^ ~ _ ,` — no quotes,
  backslashes, `;`, `$`, backticks). It never reaches a shell.
- VPP answers retval -1 when libpcap cannot compile the expression, and it **frees the previous program before
  compiling**: a failed Create leaves no filter (host-verified). The reconciler's rollback re-applies the old value.
- VPP-global without getter: nobody else on the shared host sets it; the host test sets and removes it.
- The filter is used by the packet tracer and by pcap when `pcap.filter-function` is `bpf_trace_filter`.

## Registration, ownership and restarts (D-069, D-071, D-074, D-076)
- There is no per-owner `Register` (everything here is VPP-global); `trace.RegisterGlobals(...)` registers the
  VPP-global singletons (`trace.bpf-filter`) constructed as **globals owner** — P08 calls it only in the designated globals
  owner's agent (D-071). A descriptor constructed without the role (`dfkit.GlobalsOwner(false)`)
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
- Restart simulation (fresh connection + fresh descriptors → empty plan; objects deleted via binapi → exactly their
  re-creation planned → empty plan again): `internal/descriptors/dfkit/restarttest`, output in `DF-8.md`.
