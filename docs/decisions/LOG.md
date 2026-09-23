# Decision log

One line per decision taken by an agent (or recorded from chat), **with the options that were considered**.
The product owner reviews; to overturn, append `OVERTURNED → …` to the line or say so in chat.

| date | id | decision | options considered | why | reversal cost | tasks |
|---|---|---|---|---|---|---|
| 2026-09-23 | D-001 | Appliance OS = Ubuntu 26.04; VPP 26.06 built from source in /root/vpp, shipped as our .debs | (a) 24.04 + FD.io packages (b) 26.04 + source build | product owner's choice; FD.io has no 26.04 build; dev host already 26.04 | high (os.md) | all |
| 2026-09-23 | D-002 | Lab = VMware VMs, VPP with DPDK on vmxnet3; no Docker, no libvirt/nested KVM | (a) Docker VPP (b) nested KVM (c) VMware VMs directly | host has no nested virt; (c) is the hardware code path | medium | P04, P08+ |
| 2026-09-23 | D-003 | VDOM/multi-tenancy deferred; guardrails in vdom.md | (a) tenants in schema day 1 (+10%) (b) defer with guardrails (~1.5× later) (c) defer without (~3×) | product owner: time | high later | P02*, P06, P07* |
| 2026-09-23 | D-004 | pnpm 12 `allowBuilds` in pnpm-workspace.yaml | (a) legacy onlyBuiltDependencies (ignored) (b) allowBuilds (c) approve-builds interactive | only (b) works unattended | trivial | P01 |
| 2026-09-23 | D-005 | Node↔agent gRPC = @grpc/grpc-js + ts-proto v2 (@bufbuild/protobuf runtime); Go = protoc-gen-go/-grpc; buf drives both | (a) grpc-js + ts-proto (b) Connect (connect-node/connect-go) (c) protobufjs | (a) proven over unix sockets, mature | medium (contract layer) | P01, P03, P05, P06 |
| 2026-09-23 | D-006 | Zod 4 native `toJSONSchema` for JSON Schema 2020-12 + OpenAPI 3.1 | (a) zod-to-json-schema lib (b) Zod 4 native | zero deps, one source of truth | low | P01, P02* |
| 2026-09-23 | D-007 | NestJS tests on vitest + unplugin-swc | (a) jest (b) vitest+swc | esbuild lacks emitDecoratorMetadata; vitest shared with web | low | P01, P06 |
| 2026-09-23 | D-008 | buf lint STANDARD, SERVICE_SUFFIX excepted (service stays `Dataplane`) | (a) rename DataplaneService (b) except rule | matches docs/04 naming | trivial | P03 |
| 2026-09-23 | D-009 | golangci-lint deferred to P09 (go vet only now) | (a) install now (b) defer | binary not present; not blocking | trivial | P09 |
| 2026-09-23 | D-010 | Until data NICs exist on vrx-a, packet tests use af_packet host-interfaces on veth+netns; DPDK path when NICs arrive | (a) wait for NICs (b) af_packet rig now | (b) keeps work moving; path recorded per test | low | P04, P08, F-* |
| 2026-09-23 | D-011 | Decision policy = 2× rule; one manager orchestrates all workers; options logged | — | product owner's instruction | — | all |
| 2026-09-23 | D-012 | No VPP restarts by anyone before handover; restart-safety proven by agent-restart simulation; after handover manager-only under flock | (a) allow restarts under lock now (b) forbid until handover | VPP bring-up agent may be mid-work; (b) is safe and still proves reconcile | low | P05, P08, F-*, docs |
| 2026-09-23 | D-013 | Board rows split so one task = one worker: P02s/P02a/P02b/P02c, P07a/P07b; P05a own task (deps P01) | (a) multi-worker rows (b) split rows | worktree/branch/merge are per task | trivial | S1, S2 |
| 2026-09-23 | D-014 | `apps/agent/binapi/` generated for ALL plugins on the host, owned by P04 then the manager; factories never regenerate | (a) per-factory regen (b) single owner, generate all | (a) guarantees merge conflicts | low | P04, DF-* |
| 2026-09-23 | D-015 | Operating model until Claude Code is authenticated on the host: manager = Claude session on the operator desktop; workers edit via rsync and run via SSH; all git on the host | (a) wait for server login (b) desktop-driven | (a) stops work | trivial | all |
| 2026-09-23 | D-016 | This Claude session acts as manager (product owner's instruction); S1 started with 4 parallel workers; P02s skeleton first, then P02a/b/c + P03 | (a) wait for a server-side manager agent (needs Claude login on host) (b) start now from the desktop | (a) idles the programme | trivial | S1 |
