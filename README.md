# NGFW (codename VRX)

TNSR-class secure router on FD.io VPP 26.06 — Go dataplane agent, NestJS API, React/MUI UI.
Design docs: `docs/`. Agent prompts: `prompts/`. Master prompt: `docs/00-MASTER-PROMPT.md`.

## Layout
```
apps/web        React 19 + Vite + MUI          apps/api    NestJS (Fastify)      apps/agent  Go (govpp)
packages/schema Zod → JSON Schema / OpenAPI     packages/proto  gRPC contract     packages/api-client  GENERATED
packages/ui-kit MUI theme + shared widgets      deploy/     packaging, images     tools/      lab + codegen
```

## Run
```bash
pnpm install && pnpm gen && pnpm lint && pnpm typecheck && pnpm test && pnpm build
cd apps/agent && make lint test build
pnpm dev      # web :5173, api :3000
```
Dev host: `root@172.30.126.195:/root/ngfw` (also router vrx-a). VPP is built from `/root/vpp`;
VPP bring-up is owned by a separate agent — do not modify `/root/vpp`, `/etc/vpp` or the vpp service.

## Documentation and plan (start here)
| File | Purpose |
|---|---|
| [docs/13-handoff-fa.md](docs/13-handoff-fa.md) | **راهنمای تحویل به ایجنت مدیر (فارسی)** — چطور شروع کنید، کجا نگاه کنید، چه چیزی از شما لازم است |
| [prompts/MANAGER-PROMPT.md](prompts/MANAGER-PROMPT.md) | The manager agent that runs everything; prompts/README.md explains the flow |
| [plan/tasks.yaml](plan/tasks.yaml) | Task board — 74 tasks, deps, priorities, states |
| [docs/12-execution-stages.md](docs/12-execution-stages.md) | Stage DAG, gates, parallelism rules |
| [docs/11-compressed-plan-fa.md](docs/11-compressed-plan-fa.md) | Plan of record: 21 days, status of all 102 WBS items, VPP-code track |
| [docs/decisions/](docs/decisions/) | Decision policy (2x rule), LOG, PENDING files, OS and VDOM decisions |
| [docs/lab/host-vrx-a.md](docs/lab/host-vrx-a.md) | Verified facts about VPP 26.06 on this host + handover flag |
| [docs/00-MASTER-PROMPT.md](docs/00-MASTER-PROMPT.md) … [docs/09-os-packages.md](docs/09-os-packages.md) | Design package: architecture, OSS stack, roadmap, API/data model, UI spec, repo skeleton, risks, full schedule (fa/en), OS packages |
| [wbs/VRX-WBS.xlsx](wbs/VRX-WBS.xlsx) | Editable WBS workbook (+ Jira / MS Project CSV) |
| [scripts/](scripts/) | Installers: repos, runtime, build toolchain, strongSwan-VPP build, dataplane tuning, lab, dev-server prep |
