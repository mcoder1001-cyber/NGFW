# Task P01 — Monorepo scaffold   (prepend 00-CONTEXT.md)

## Goal
Create the empty-but-working monorepo exactly as laid out in 00-CONTEXT.md. Every
package builds, lints and has one trivial passing test. No product code yet.

## Build exactly this
1. Root: `pnpm-workspace.yaml`, `turbo.json` (pipelines: lint, typecheck, test, build, gen),
   `package.json` with scripts `lint typecheck test build gen dev`, `.editorconfig`,
   `.gitignore`, `.nvmrc` (22), `README.md` (one screen: what/how to run).
2. `apps/web` — Vite + React 19 + TS strict + MUI v7 + TanStack Query + react-router +
   i18next with `en` and `fa` empty namespaces. Renders "VRX" and a theme toggle. Vitest + one test.
3. `apps/api` — NestJS with Fastify adapter, TS strict, `GET /api/v1/health` → `{status:"ok"}`,
   Vitest + one e2e test using the Nest testing module. Config via env (`VRX_*`), zod-validated.
4. `apps/agent` — Go 1.23 module `github.com/<org>/vrx/agent`, `cmd/vrx-agent/main.go` that
   starts, logs "vrx-agent starting" with structured logging (`slog`), handles SIGTERM.
   `Makefile` with `build lint test`. `golangci-lint` config. One test.
5. `packages/schema` — TS package exporting an empty `RootConfig = z.object({})` and a
   script `gen` that writes `dist/json-schema/root.json` and `dist/openapi-components.json`.
6. `packages/proto` — `vrx/dataplane.proto` with a single `Health` RPC; `buf` config; script
   `gen` producing Go stubs into `apps/agent/gen/` and TS stubs (`@bufbuild/protobuf` +
   `@connectrpc`, or `ts-proto` — pick one, state why) into `packages/proto/gen/ts`.
7. `packages/api-client` — generated from the API's OpenAPI with `openapi-typescript` +
   a small fetch wrapper; `gen` script; header comment "GENERATED — do not edit".
8. `packages/ui-kit` — exports `createVrxTheme(mode, direction)` and nothing else yet.
9. `deploy/dev/` placeholder: a script that starts postgres:16 and valkey locally (systemd user units or plain binaries) for the API's own tests; P04 owns the VM lab.
10. `pnpm gen` runs all generators; CI must fail if `git status` is dirty afterwards.

## Acceptance
- [ ] Fresh clone: `pnpm install && pnpm gen && pnpm lint && pnpm typecheck && pnpm test && pnpm build` all green
- [ ] `cd apps/agent && make lint test build` green
- [ ] `pnpm dev` starts web on :5173 and api on :3000; web calls `/api/v1/health` and shows the result
- [ ] Node 22 / pnpm 9 / Go 1.23 pinned and documented

## Out of scope
Any product feature, any VPP code, auth, database schema, container images of any kind.
