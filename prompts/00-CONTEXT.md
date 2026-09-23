# VRX — shared context for every agent task

> Paste this file at the top of every task prompt. It is the contract between agents.
> Precedence: your TASK ENVELOPE wins for branch, worktree, slot, time box and process; this file wins for
> architecture and security rules; the task prompt fills in the rest. **Never stop on a conflict** — write it in
> `docs/status/tasks/<id>-questions.md` (the manager: `docs/decisions/LOG.md`) and continue with the higher-precedence rule.

## What we are building

**VRX** is a high-performance software router / secure gateway (functional reference:
Netgate TNSR) built on **FD.io VPP 26.06** with a management plane of our own:

```
Browser: React 19 + Vite + TypeScript(strict) + MUI v7 + TanStack Query v5 + react-hook-form + Zod + i18next (en, fa/RTL)
   │  HTTPS REST (/api/v1) + WebSocket (/api/v1/stream)
vrx-api: Node.js 22 + NestJS (Fastify adapter) + TypeScript(strict) + PostgreSQL 16 + Valkey
   │  gRPC over unix socket /run/vrx/agent.sock  (protobuf in packages/proto)
vrx-agent: Go 1.23 + go.fd.io/govpp  (declarative reconciler; renders FRR/strongSwan/Kea/Unbound/chrony configs)
   │  VPP binary API + stats segment; files + systemd for the daemons
VPP 26.06 · FRR · strongSwan(kernel-vpp) · Kea · Unbound · chrony
```

## Non-negotiable architecture rules

1. **Node.js never talks to VPP.** No vppctl, no FFI, no shelling out. All data-plane
   access is via `vrx-agent` gRPC.
2. **vrx-agent is declarative.** RPCs carry *desired state*; the agent diffs against
   *retrieved* VPP state and converges. Every object type implements
   `Create/Update/Delete/Retrieve/Dependencies`. It must fully rebuild the data plane
   from the datastore after `kill -9 vpp`.
3. **PostgreSQL is the source of truth**, not VPP runtime state. The whole config is one
   JSON document validated by one root schema. `running` and `candidate` are two copies.
4. **Commit is atomic.** Validate (schema → semantic → renderer dry-run) → render →
   apply → persist revision. Any failure rolls everything back. Confirmed-commit
   (`?confirm=<sec>`) auto-reverts unless confirmed.
5. **One schema, three consumers.** `packages/schema` (Zod) generates TS types, OpenAPI
   components and JSON Schema for the UI form renderer. Never duplicate a type by hand.
6. **VPP API names come only from generated bindings** (`binapi-generator` from the
   pinned VPP 26.06 `.api.json`). Never write a VPP message name from memory.
7. **GPL daemons (FRR, strongSwan, chrony, keepalived) are separate processes**, driven by
   rendered config files + their own CLI/JSON. Never link their code.
8. **Config/State/Actions split** in the API: `/api/v1/config/**` (transactional),
   `/api/v1/state/**` (read-only live), `/api/v1/actions/**` (ping, capture, reboot).
9. **No user input ever reaches a shell.** Config files are rendered from templates with
   strict escaping and validated before reload.
10. **Secrets** (PSKs, private keys, passwords) are encrypted at rest, never returned by
    GET, never logged.

## Repository layout (pnpm workspace + Turborepo; Go module in apps/agent)

```
apps/web        React SPA                     apps/api      NestJS
apps/agent      Go dataplane agent            apps/cli      (later)
packages/schema Zod schemas → types/OpenAPI/JSON Schema
packages/proto  .proto + generated Go/TS stubs
packages/api-client  GENERATED TS client — never hand-edit
packages/ui-kit MUI theme, SchemaForm, DataGrid wrapper, charts
deploy/         dev helpers, debian/, systemd units, image build   tools/lab   VMware lab driver (SSH/govc)
test/           integration (host VPP + veth/netns rig), topology (VM inventories), e2e (playwright)
docs/           design docs
```

## Conventions

- TypeScript `strict`, ESLint flat config + Prettier; Go: `gofmt`, `go vet`, `golangci-lint`. Zero warnings.
- Conventional Commits. **Git is local-only on the host (no remote, no PRs, no CI service).** One task = one branch `task/<id>` in
  worktree `/root/ngfw-wt/<id>`, both created by the manager — never create your own. A "PR" in any prompt means: your branch +
  `docs/status/tasks/<id>.md` (what / how verified with pasted output / out of scope / questions). "Labelled `contract`" means:
  commit subject `contract(<pkg>): …` on branch `contract/<id>` plus `docs/status/tasks/<id>-contract.md` — the manager reviews it.
- Tests: Vitest (TS), `go test` (Go). `pnpm test` / `make test` are **unit-only**. Integration tests (VPP, daemons, DB) run only
  when `VRX_INTEGRATION=1` (Go: `t.Skip` otherwise; TS: `test:integration` script) and are executed by `tools/ci.sh full`
  under the shared lab lock. They run against the **real VPP on this host** (`/run/vpp/api.sock`) — mocks never count as integration.
- Errors: RFC 9457 `application/problem+json` with `pointer` to the offending JSON path.
- i18n: every UI string through `t()`, keys in `apps/web/src/locales/{en,fa}/*.json`.
  Use logical CSS properties (`margin-inline-start`), never `margin-left`.
- Do not add dependencies outside the stack above without stating why in the PR.

## Definition of done (all nine, every feature)

1. Config verified on VPP: `Retrieve()` equals desired **and** `vppctl show <x>` reflects it (packet-level test only where the task says so)
2. Survives `tools/lab restart-vpp vrx-a` (agent reconciles) and full stack restart
3. REST endpoint + OpenAPI + regenerated client
4. Works through candidate → diff → commit → rollback, including validation-failure paths
5. UI screen: schema-driven form + list + live status
6. en + fa strings
7. Unit + integration (+ E2E where a UI screen exists)
8. Docs page under `docs/user/` + CLI equivalent noted
9. Audit log entry on every mutation

## How to work

1. Read this file, then `docs/00-MASTER-PROMPT.md`, `docs/01-architecture.md`,
   `docs/04-api-datamodel.md` and the contracts in `packages/schema` and `packages/proto`.
2. VPP 26.06 is **already running on this host** (`/run/vpp/api.sock`; facts in `docs/lab/host-vrx-a.md`). `tools/lab` and the
   veth/netns packet rig exist once P04 is merged; PostgreSQL and Valkey are localhost systemd services (`deploy/dev/README.md`).
   Never testcontainers, containerlab or Docker.
3. Work only inside your task's scope. If you find you need a contract change, stop and
   open a separate PR labelled `contract` with the reasoning — do not silently change it.
4. Run the full check before declaring done: `pnpm lint && pnpm typecheck && pnpm test`
   and `cd apps/agent && go vet ./... && go test ./...` and the integration suite.
5. Finish with a PR description containing: what was built, how it was verified
   (paste the actual command output), what is explicitly out of scope, open questions.

## Never do these

- Invent VPP API message names or fields · write to `main` · hand-edit generated code ·
  ship a UI screen whose backend is stubbed · mark a test "passing" that skips VPP ·
  add features not in your task · put secrets in code, logs or fixtures ·
  paste environment variables, connection strings, PSKs, tokens or password hashes into any committed file
  (redact as `<redacted>`; test fixtures use the literal `VRX_TEST_PSK_<id>`) ·
  `pkill`/`killall`/kill-by-pattern (kill only PIDs you spawned) · edit `tools/binapi-gen.sh` or
  `apps/agent/binapi/` outside P04 (ask the manager via a questions file) ·
  use XLOOKUP-style "it probably exists" library APIs — check the installed version.

---

## FAST MODE — ACTIVE (overrides the sections above where they conflict)

We are running the compressed 21-day plan (`docs/11-compressed-plan-fa.md`). Rules:

- **Environment is VMware VMs (Ubuntu 26.04, VPP 26.06 built from source in /root/vpp), never Docker, never nested KVM.** The dev host **is** `vrx-a`: VPP v26.06 is running (`/run/vpp/api.sock`, group `vpp`; facts in `docs/lab/host-vrx-a.md`). Use VPP freely via API/vppctl; do NOT modify `/etc/vpp/startup.conf`, packages or `vpp.service` while that file says `handover: pending`.
- **Multi-tenancy (VDOM) is deferred** — follow the five guardrails in `docs/decisions/vdom.md`.
- **Decisions:** you decide and log (`docs/decisions/LOG.md`) unless the 2× rule or the always-ask list in `docs/decisions/decision-policy.md` applies — then write `PENDING-<slug>.md`, park only what depends on it, keep going.
- **Board and status:** `plan/tasks.yaml` is task state; every task ends with `docs/status/tasks/<id>.md` containing pasted real output. Workers do not edit the board; the manager does.
- **Lab:** `tools/lab` drives the VMware VMs over SSH (local mode for `vrx-a`, govc optional); the
  data-plane path is DPDK on vmxnet3 once data NICs exist, af_packet on veth/netns until then (D-010).
  Dev/CI host `172.30.126.195`, repo `/root/ngfw`, user root. Do not add Dockerfiles or compose files.
- **No C code in VPP. Ever, in this plan.** If your task seems to need a VPP plugin change or
  patch, STOP, write the reason into `docs/vpp-code-track.md` under the matching V1–V6 item
  (or a new one), implement the best configuration-only fallback, and say so in the PR.
- **Reduced definition of done** for a feature: (1) schema + descriptor/renderer + API + UI
  screen + en/fa strings; (2) ONE verification: after commit `Retrieve()` == desired and
  `vppctl show <x>` (or the daemon's own show command) reflects it; after rollback, nothing
  remains; (3) **restart-safety without restarting VPP**: stop your own agent process, delete your
  prefixed objects via binapi (simulated loss), start your agent → it recreates them (log + Retrieve).
  Nobody restarts or kills VPP while `docs/lab/host-vrx-a.md` says `handover: pending`; afterwards only
  the manager does, under `flock /run/lock/vrx-vpp.lock`. Packet-level tests only when the task explicitly
  lists one (path recorded: `af_packet` rig or DPDK). Unit tests are required in `packages/schema`, the
  scheduler, the commit engine, descriptors (fake VPP client) and renderers (golden files); optional elsewhere.
- **Shared host:** `docs/lab/shared-host-rules.md` is mandatory — your slot's `VRX_TEST_PREFIX`, ports, table
  range, database; daemon ownership; no `pkill`/`killall` by pattern, kill only PIDs you spawned; leave
  daemons stopped. Integration tests create only prefixed objects and clean up in `t.Cleanup`.
- **No performance work.** Do not tune, benchmark, or claim throughput. Week 4, humans, hardware.
- **Contracts v1 are tagged by the manager** when P02*/P03 pass acceptance (the human reviews afterwards,
  asynchronously). After that, *additive* schema/proto changes that follow `docs/04-api-datamodel.md` are
  ordinary `contract/<id>` branches the manager reviews and merges — no human wait, do not stall on them:
  commit the contract change first, tell the manager via `docs/status/tasks/<id>-questions.md`, and keep
  building against your branch. Only renaming/reshaping existing fields is always-PENDING (decision-policy #1).
- **Prefer breadth over polish**: a working screen with a plain SchemaForm beats a beautiful
  half-wired one. Polish is week 4+.
- Everything else in this file (architecture rules, provenance of VPP API names, no shell with
  user input, secrets handling, one PR per task in its own worktree) still applies in full.
