# trace descriptors (DF-8, WBS D8.2)

Package `apps/agent/internal/descriptors/trace` — the BPF trace filter (`bpf_trace_filter` plugin). Message names only
from `apps/agent/binapi/bpf_trace_filter`. `trace.Register(registry, client)`.

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
