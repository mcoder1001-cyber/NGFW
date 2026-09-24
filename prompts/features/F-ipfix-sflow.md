# Task: F-ipfix-sflow — IPFIX/NetFlow (flowprobe) + sFlow sampling   (prepend 00-CONTEXT.md)

## Goal
Flow export end to end (WBS D7.6 in `plan/wbs.csv`): IPFIX exporters, flowprobe per interface (L2/IPv4/IPv6, rx/tx), and sFlow packet
sampling. Reference: TNSR "IPFIX / flow export"; VPP plugins `flowprobe`, `sflow`, vnet `ipfix-export`.

## Inputs to read first
- `packages/schema/src/domains/services.ts` — `services.ipfix{exporters{<n>}, flowprobe{activeTimerSec, passiveTimerSec, recordL2/L3/L4,
  interfaces[]}, sflow{enabled, samplingN, pollingIntervalSec, headerBytes, collectors[], agentAddress, vrf, interfaces[]}}` exists
- `apps/agent/internal/descriptors/{ipfix,flowprobe,sflow}/` (DF-8, merged) + their `docs/agent/descriptors/*.md`: `ipfix.default-exporter`
  (exporter 0 — the only one flowprobe uses, IPv4 collector only), `ipfix.exporter`, `flowprobe.params` (singleton, ErrRecreate; VPP refuses
  changes while **any** owner has an interface enabled), `flowprobe.interface`, `sflow.global`, `sflow.interface` (hw→sw learning)
- `apps/agent/binapi/{ipfix_export,flowprobe,sflow}/` — only source of message names
- `docs/vpp-code-track.md` **V16** (ipfix classify dumps broken → `ipfix.classify-*` write-only, D-063) and **V17** (`sflow_interface_details`
  has only hw_if_index → descriptor learns hw→sw in Create; after an agent restart the first resync re-creates once)
- D-071/D-082: exporter 0, flowprobe params and sflow global are VPP-globals — set only by the globals owner (`ipfix.RegisterGlobals`,
  `flowprobe.RegisterGlobals`, `sflow.RegisterGlobals` only when `Env.GlobalsOwner`); slot agents (`VRX_GLOBALS_OWNER=0`) only *require* them.
  The collector evidence below needs exporter 0 pointed at your slot collector = a VPP-global change: only behind your own opt-in env var
  (`VRX_IPFIX_GLOBALS=1`) holding `flock -x /run/lock/vrx-globals.lock`, saving and restoring exactly the previous exporter-0/flowprobe/sflow
  values (never VPP defaults), in a manager window; without it the test skips with that reason
- `ipfix.Register` needs DF-2's classify store (`Wiring.ClassifyStore()`, P08) for the write-only `ipfix.classify-*` types
- host facts (checked 2026-09-24): `hsflowd`, `nc` and `socat` are **not installed** → collectors are tiny Go UDP listeners in the test
  (never a package install); `tcpdump`/`tshark` exist for optional pasted evidence

## Contract changes
`services.ipfix.sflow.collectors` exists but VPP's sflow plugin does **not** export: export is hsflowd's job and hsflowd is not installed on
the host. If you add fields (e.g. exporter selection per producer): additive, as separate `contract(schema|proto): …` commits on **your task
branch** (no own branches; numbers from your envelope / `docs/status/wave-BC-numbers.md`), questions file, continue.

## Scope — build exactly this
Files you own and shared hotspots: your TASK ENVELOPE is authoritative (the board's old `agent/project_ipfix_sflow*.go` became
`internal/desired/ipfix_sflow*.go` + `internal/subsystems/ipfix_sflow*.go` + `internal/agent/rpc_ipfix_sflow*.go`, wave-A hotspots A2).
The DF-8 descriptors `apps/agent/internal/descriptors/{ipfix,flowprobe,sflow}/**` are gap-only (use, do not rebuild, D-104).
Shared files: registration lines under your anchor only (agent registry, `app.module.ts`, router/nav, services tab registry).
1. **Schema** (contract branch if missing): flowprobe needs an enabled exporter with an IPv4 collector (exporter 0 rule); one flowprobe
   variant per interface (exists); sflow `headerBytes` in steps of 32 (VPP rounds silently — descriptor already rejects).
2. **Agent**: project the first enabled exporter → `ipfix.default-exporter`, further ones → `ipfix.exporter`; flowprobe params + interfaces;
   sflow global + interfaces; interface refs `interface/<name>` (D-065/D-069). Non-globals-owner agents only *require* the globals.
3. **API**: config via pointer routes; `GET /api/v1/state/ipfix` (exporters from Retrieve incl. stat index when known, flowprobe interfaces,
   sflow interfaces + sampling counters from the stats segment).
4. **UI**: Services → Flow export: Exporters / Flowprobe / sFlow tabs with interface picker; a banner that sFlow collector export needs
   hsflowd (not shipped yet); en + fa; screenshot.
5. **Docs**: `docs/user/services/ipfix-sflow.md` — IPFIX to a collector for LAN flows, sFlow sampling 1:1000; CLI equivalent.

## Acceptance (paste the evidence)
- [ ] `vppctl show flowprobe interface` / `show flowprobe params` and `show sflow` reflect the committed config; a tiny Go UDP collector on a
      slot port receives IPFIX template + data sets after traffic on the rig (pasted, not a packet-path claim — just exporter evidence; opt-in
      globals window, see Inputs)
- [ ] Agent-restart simulation → objects back within 30 s; sflow shows the one expected re-create (V17) in the log excerpt
- [ ] Rollback removes flowprobe/sflow interfaces (Retrieve) and resets globals only in the globals owner
- [ ] flowprobe interfaces with no enabled exporter → 400 problem+json with a `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
hsflowd packaging/rendering (P10 follow-up; record it), NAT/CGNAT IPFIX logging enables (`nat.ipfix` — EI's `nat44-ei.ipfix` is
projected by F-nat44-ei-64-66-nptv6; the ED enable `nat_ipfix_enable_disable` has **no descriptor yet** — see the open question, do not build
it unless the manager assigns it to you), classify-based IPFIX reports (write-only, V16 — no UI), dashboards/Prometheus
(F-dashboard-prom-alarms), pcap (F-capture-trace). The exporter those NAT loggers send through is exporter 0 — yours.

## Open questions to surface, not to decide silently
sFlow without hsflowd samples but exports nothing — ship the config now with a warning, or hide sflow until P10 packages hsflowd?
Flowprobe params change is blocked by another owner's interface on the shared host — acceptable test skip?
NAT44-ED IPFIX logging ownership (found in wave-B/C prep): DF-3 left `nat_ipfix_enable_disable` to "DF-8", DF-8 did not build it, the
F-nat44-ed-sessions envelope fences it to "DF-8 / F-ipfix-sflow" and F-det44 names you for CGNAT logging export — who builds the ED enable?
