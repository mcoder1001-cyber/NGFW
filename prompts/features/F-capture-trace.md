# Task: F-capture-trace — packet capture, BPF trace filter, trace, packet generator   (prepend 00-CONTEXT.md)

## Goal
Troubleshooting tools in the UI (WBS D8.2, D7.11 in `plan/wbs.csv`): start/stop a pcap capture (rx/tx/drop, interface, packet/byte limits,
BPF filter) and download the file; packet tracing and the packet generator **as far as VPP exposes them through the binary API**.
Reference: TNSR "Packet capture / trace"; VPP vnet `pcap trace`, plugins `bpf_trace_filter`, `pg`, `tracedump`/`tracenode` (not built here).

## Inputs to read first
- `apps/agent/internal/descriptors/{pcap,trace}/` (DF-8, merged) + `docs/agent/descriptors/{pcap,trace}.md`: `pcap.capture` (one capture per
  VPP, write-only, `ErrCaptureBusy`, file only `/tmp/<owner>-…`, VPP writes it 0664 → **you must move it to an agent dir with 0600 and a
  retention policy**), `pcap.filter-function`, `trace.bpf-filter` (expression validated to the pcap-filter alphabet, never a shell)
- `packages/proto/vrx/v1/dataplane.proto` — `Action` RPC with `CaptureAction{interface, bpf, max_packets, seconds, direction RX|TX|BOTH,
  snaplen}` and `ActionOutput.pcap_chunk` (the capture is an **action**, not config; the descriptors are driven by the action handler, not
  registered in `Domains`); `apps/api/src/actions/actions.controller.ts` (P06, merged: generic `POST /actions/:action`, `capture` is in the
  ACTIONS list and answers 501 — in wave A that file is F-vrf-static-ecmp's generic Action bridge: serve `POST /api/v1/actions/capture` as a
  **static route in your own controller** (it beats `:action`) and reuse F-vrf's Action stream method in `agent.client.ts` if it is on main)
- `apps/agent/internal/agent/server.go` `Action` — W-seed turns it into a type switch with case anchors; your capture case goes under your
  own anchor, the handler lives in your `internal/actions/capture-trace/` + `internal/agent/rpc_capture_trace*.go`
- `apps/agent/binapi/{interface,bpf_trace_filter,pg}/` — `pcap_trace_on/off`, `pcap_set_filter_function`, `bpf_trace_filter_set_v2`,
  `pg_create_interface_v3`, `pg_capture`, `pg_enable_disable`, `pg_delete_interface` (verified). **No binary API** exists for classic
  `trace add`/`show trace` or for defining PG streams — only `cli_inband`, which is CLI-by-API and is not allowed here without a manager decision
- **V18 / D-077 / D-089**: upstream ships tracedump/tracenode in `vpp-plugin-devtools`, which vrx-a does **not** install; F-vpp-debs (merged)
  can build a `deploy/vpp/build.sh --trace-plugins core` variant that moves them into vpp-plugin-core, but installing packages on vrx-a is
  handover-gated (D-012) and there is no `tracedump` binapi (`apps/agent/binapi/trace` is the unrelated `trace_profile_*` API). Trace Path has
  no API. So trace stays **skip-unless-loaded** with a typed error; binapi regeneration is a manager step once the plugin is on the host
- D-071/D-082: the BPF filter and filter-function are VPP-globals (globals owner only; host tests opt-in `VRX_DF8_GLOBALS=1`, globals lock)
- host facts (checked 2026-09-24): `tcpdump` 4.99.6 and `tshark` present (read the downloaded file with `tcpdump -r`); VPP writes the capture
  as `/tmp/<owner>-…` (0664) — never leave it there

## Contract changes
`CaptureAction` has interface/bpf/max_packets/seconds/direction/snaplen; missing: a drop capture and the node/error filter → additive fields
(numbers from your envelope / `docs/status/wave-BC-numbers.md`), plus capture-list/read/delete and PG RPCs only if the agent keeps the files,
as a separate `contract(proto): …` commit on **your task branch** (no own branches), questions file, continue. No config-document changes expected.

## Scope — build exactly this
Files you own and shared hotspots: your TASK ENVELOPE is authoritative (the board's old `agent/project_capture_trace*.go` became
`internal/subsystems/capture_trace*.go` + `internal/agent/rpc_capture_trace*.go`; `descriptors/bpf_trace_filter` does not exist — the BPF
filter is DF-8's `trace.bpf-filter` in `descriptors/trace`; `descriptors/pg` is new). Shared files: registration lines under your anchor only
(agent Action switch, `app.module.ts`, router/nav — the `tools` NotAvailable route and nav item are yours to replace).
1. **Agent action** `capture`: validate, set BPF filter (globals owner) → `pcap.capture` Create → stream progress → stop on limit/timeout/
   cancel → move file to `/var/lib/vrx/captures/` (tests: slot dir) 0600, record owner/size/sha256; list + delete captures; retention (count
   and bytes caps). Only one capture at a time per VPP — clear `busy` error. Never stop another owner's capture.
2. **Trace / PG**: implement `tracedump`-based trace and PG only if the binapi exists at start (skip-unless-loaded, typed error); else build
   PG interface create/delete + `pg_capture` only and leave stream definition documented as blocked (V18 + no stream API).
3. **API**: `POST /api/v1/actions/capture` (starts, returns id), `GET /api/v1/state/captures`, `GET /api/v1/state/captures/{id}/file`
   (download, admin RBAC, audit), `DELETE …/{id}`; problem+json for busy/invalid filter.
4. **UI**: Tools → Capture: form (interface picker, rx/tx/drop, limits, BPF expression), live progress, capture list with download; trace/PG
   tabs render "not available on this build" when the plugin is missing (never fake data); en + fa; screenshot.
5. **Docs**: `docs/user/tools/capture-trace.md` — capture on an interface with a BPF filter, open in Wireshark; limits and why trace needs V18.

## Acceptance (paste the evidence)
- [ ] Capture on a slot rig interface while pinging → downloaded file opens with `tcpdump -r` and shows the ICMP (pasted); file mode 0600
- [ ] Second concurrent capture → 409/problem+json `busy`; invalid BPF (`;` or quotes) → 400 with a `pointer`
- [ ] Agent-restart simulation during a capture → capture is stopped/recorded consistently (D-076 boot identity) — log excerpt
- [ ] Deleting a capture removes the file (ls before/after); `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
Enabling tracedump/tracenode in the VPP build (F-vpp-debs), binapi regeneration (manager), Trace Path (no API), `cli_inband`/vppctl use,
G2 event-log export, IPFIX/sFlow (F-ipfix-sflow), dashboards (F-dashboard-prom-alarms), ping/traceroute actions (F-vrf-static-ecmp),
support bundle (F-backup-restore).

## Open questions to surface, not to decide silently
Is a narrowly allow-listed `cli_inband` (fixed `trace add <node> <n>` / `show trace max <n>` templates, integers only) acceptable as an
interim trace path until V18 lands? Default: no. Retention defaults (e.g. 10 files / 500 MB)?
