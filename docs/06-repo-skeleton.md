# Monorepo skeleton and tooling

```
vrx/
├─ apps/
│  ├─ web/              React 19 + Vite + MUI v7           (TypeScript)
│  ├─ api/              NestJS + Fastify                    (TypeScript)
│  ├─ agent/            dataplane agent                     (Go)
│  └─ cli/              optional on-box CLI, talks to api    (Go or TS/oclif)
├─ packages/
│  ├─ schema/           Zod schemas → TS types, JSON Schema, OpenAPI components
│  ├─ api-client/       GENERATED TypeScript client (do not edit by hand)
│  ├─ proto/            .proto files + generated Go/TS stubs
│  └─ ui-kit/           MUI theme, SchemaForm, DataGrid wrappers, charts
├─ deploy/
│  ├─ packaging/        debian/ rules, systemd units, postinst reconcile hook
│  ├─ image/            installer ISO / cloud-init / autoinstall
│  ├─ ansible/          lab provisioning
│  └─ apt-repo/         signed repo tooling
├─ test/
│  ├─ integration/      API ↔ real VPP in containers (testcontainers)
│  ├─ topology/         containerlab / QEMU labs + Robot Framework suites
│  ├─ perf/             TRex profiles, pktgen scripts, baselines
│  └─ e2e/              Playwright
├─ docs/                user docs (MkDocs or Docusaurus) + these design docs
└─ tools/               codegen, binapi regeneration, lint config
```

## Tooling

- **pnpm workspaces** + **Turborepo** (or Nx) for the TS side; Go modules for `agent`.
- **Codegen pipeline** (a single `pnpm gen`):
  `packages/schema` → JSON Schema + OpenAPI → `packages/api-client` → UI types.
  `packages/proto` → Go + TS stubs. VPP `.api.json` → GoVPP bindings (`binapi-generator`).
  CI fails if generated output is dirty.
- **Dev environment:** `docker compose up` gives Postgres + Valkey + a VPP container with
  two `memif`/`veth` links + the agent + the API, and `pnpm dev` runs Vite against it.
  A developer must be productive without a 100G NIC.
- **Lint/format:** ESLint flat config + Prettier + `typescript-eslint` strict;
  `golangci-lint` with `errcheck`, `gosec`, `revive`.
- **Commits:** Conventional Commits → changelog → package versions.
- **CI (GitHub Actions or GitLab CI):**
  `lint → unit → build → integration(vpp container) → package(.deb) → e2e(playwright) → nightly topology+perf`.
- **Release:** signed `.deb`s to an APT repo + a signed offline bundle (`.vrxupd`) for
  air-gapped sites, containing packages, checksums, signature and a manifest.

## Upgrade design (decide early, it constrains packaging)

Two viable models:

1. **Package-based** (`apt upgrade`) — simple, but partial-failure states are messy.
2. **A/B image** (two root partitions, boot flag, auto-rollback on failed health check) —
   what serious appliances do. Costs more up front, saves you from bricked customer boxes.

Recommendation: package-based for beta, A/B images before GA.
