# P06 WIP — API core

Worker slot 1 (w1, port 3100, DB vrx_w1, Valkey db 1 prefix vrx:w1:). Started 2026-09-24.

## Order / status
- [x] persistence (Drizzle + pg) + migrations + seed
- [x] datastore + lock
- [x] validation pipeline
- [x] commit engine vs fake agent
- [x] auth / RBAC guard + route-enumeration test
- [x] audit
- [x] revisions / rollback / confirmed commit
- [x] telemetry WS relay
- [ ] OpenAPI + api-client gen
- [ ] CI gate, P06.md

## Notes
- deps added: drizzle-orm, pg, @node-rs/argon2 (napi prebuilt, no install script), jose, @fastify/cookie,
  @fastify/websocket, iovalkey, @grpc/grpc-js; dev: drizzle-kit, @types/pg, @types/ws; api-client dev: @redocly/cli

## Log
- 01:30 unit (21 tests) + e2e on host PG (config 13, auth 8, stream 2) green; agent integration test written, skips (no P05 binary)
- next: OpenAPI + api-client gen (+ redocly), CI gate, P06.md
