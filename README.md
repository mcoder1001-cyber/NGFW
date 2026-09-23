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
