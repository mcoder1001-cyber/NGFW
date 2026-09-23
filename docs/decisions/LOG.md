# Decision log

One line per decision taken by an agent (or recorded from chat). The product owner reviews; to overturn, append `OVERTURNED → …`.

| date | id | decision | why | reversal cost | tasks |
|---|---|---|---|---|---|
| 2026-09-23 | D-001 | Appliance OS = Ubuntu 26.04; VPP 26.06 built from source in /root/vpp, shipped as our .debs | product owner's choice; FD.io has no 26.04 build | high (recorded in os.md) | all |
| 2026-09-23 | D-002 | Lab = VMware VMs, VPP with DPDK on vmxnet3; no Docker, no libvirt/nested KVM | host has no nested virt; hardware code path | medium | P04, P08+ |
| 2026-09-23 | D-003 | VDOM/multi-tenancy deferred; guardrails in vdom.md | product owner; time | high later (~1.5× with guardrails) | P02, P06, P07 |
| 2026-09-23 | D-004 | pnpm 12 `allowBuilds` in pnpm-workspace.yaml (legacy keys ignored) | only mechanism that works in pnpm 12 | trivial | P01 |
| 2026-09-23 | D-005 | Node↔agent gRPC = @grpc/grpc-js + ts-proto v2 (@bufbuild/protobuf runtime); Go = protoc-gen-go/-grpc; buf drives both | unix-socket support, mature | medium (contract layer) | P01, P03, P05, P06 |
| 2026-09-23 | D-006 | Zod 4 native `toJSONSchema` for JSON Schema 2020-12 and OpenAPI 3.1 components; no extra library | one schema, three consumers, zero deps | low | P01, P02 |
| 2026-09-23 | D-007 | NestJS tests on vitest + unplugin-swc | esbuild lacks emitDecoratorMetadata | low | P01, P06 |
| 2026-09-23 | D-008 | buf lint STANDARD with SERVICE_SUFFIX excepted (service stays `Dataplane`) | matches docs/04 contract naming | trivial | P03 |
| 2026-09-23 | D-009 | golangci-lint deferred to P09 (go vet only for now) | binary not installed; not blocking | trivial | P09 |
| 2026-09-23 | D-010 | Until data NICs exist on vrx-a, packet tests use af_packet host-interfaces on veth+netns; DPDK path exercised when NICs arrive | host has a single mgmt NIC | low | P04, P08, F-* |
| 2026-09-23 | D-011 | Decision policy = 2× rule (decision-policy.md); one manager agent orchestrates all workers | product owner's instruction | — | all |
