# Task P13 — Basic CLI   (prepend 00-CONTEXT.md)

## Goal
An on-box CLI (`vrx`) that network engineers can live in: `show` commands and a config mode
with candidate/commit/rollback semantics — a thin client over the REST API, never touching
the agent or VPP directly.

## Build exactly this
1. Go binary `apps/cli` using the OpenAPI spec to generate its client; auth via local unix
   socket to the API with peer-credential check (root/`vrx-admin` group) or API key.
2. Interactive REPL (`github.com/chzyer/readline` or similar): tab completion driven by the
   JSON Schema (config paths, enums), history, `?` help.
3. Operational: `show interfaces [name]`, `show ip route [vrf]`, `show bgp summary`,
   `show ipsec sa`, `show system`, `show configuration [path] [json|text]`,
   `show configuration diff`, `show revisions`, `ping`, `traceroute`.
4. Config mode: `configure` → `set <path> <value>`, `delete <path>`, `edit <path>`, `show`,
   `commit [confirm <sec>] [comment "..."]`, `confirm`, `rollback <rev>`, `discard`, `exit`.
   Paths map 1:1 to JSON pointers of the schema; values validated client-side with the schema
   before sending. Text rendering of the config in a stable, diff-friendly format.
5. `vrx --json` machine mode for every command; exit codes documented.
6. Tests: unit for path↔pointer mapping and text rendering; e2e against the dev stack
   scripted with `expect`: set MTU → commit confirm 5 → observe auto-revert.
7. `docs/user/cli.md` with the full command reference generated from the code.

## Acceptance
- [ ] Every CLI command maps to a documented REST call; `grep -rn vppctl apps/cli` is empty
- [ ] Completion offers valid config paths and enum values from the live schema

## Out of scope
SSH server integration/login shell wiring (documented as a follow-up), NETCONF, scripting language.
