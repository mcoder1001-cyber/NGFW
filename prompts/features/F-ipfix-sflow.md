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
- D-071/D-082: exporter 0, flowprobe params and sflow global are VPP-globals — set only by the globals owner; tests hold the globals lock

## Contract changes
`services.ipfix.sflow.collectors` exists but VPP's sflow plugin does **not** export: export is hsflowd's job and hsflowd is not installed on
the host. If you add fields (e.g. exporter selection per producer) use `contract/F-ipfix-sflow`, additive, questions file, continue.

## Scope — build exactly this
Files you own: `apps/agent/internal/descriptors/{ipfix,flowprobe,sflow}/**`, `docs/agent/descriptors/{ipfix,flowprobe,sflow}.md`,
`apps/agent/internal/agent/project_ipfix_sflow*.go`, `apps/api/src/features/ipfix-sflow/**`, `apps/web/src/domains/services/ipfix-sflow/**`,
`apps/web/src/locales/*/ipfix-sflow.json`, `docs/user/services/ipfix-sflow.md`, `test/topology/ipfix-sflow/**`.
Shared files: one-line appends only (agent registry, `app.module.ts`, router/nav).
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
- [ ] `vppctl show flowprobe interface` / `show flowprobe params` and `show sflow` reflect the committed config; a UDP listener
      (`nc -u -l` / tiny Go collector on a slot port) receives IPFIX template + data sets after traffic on the rig (pasted, not a packet-path
      claim — just exporter evidence)
- [ ] Agent-restart simulation → objects back within 30 s; sflow shows the one expected re-create (V17) in the log excerpt
- [ ] Rollback removes flowprobe/sflow interfaces (Retrieve) and resets globals only in the globals owner
- [ ] flowprobe interfaces with no enabled exporter → 400 problem+json with a `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
hsflowd packaging/rendering (P10 follow-up; record it), NAT/CGNAT IPFIX logging (F-nat44-ed-sessions, F-det44-map-dslite-cnat),
classify-based IPFIX reports (write-only, V16 — no UI), dashboards/Prometheus (F-dashboard-prom-alarms), pcap (F-capture-trace).

## Open questions to surface, not to decide silently
sFlow without hsflowd samples but exports nothing — ship the config now with a warning, or hide sflow until P10 packages hsflowd?
Flowprobe params change is blocked by another owner's interface on the shared host — acceptable test skip?
