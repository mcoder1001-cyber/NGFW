# TD-2 — WIP (API follow-ups: admin password set, api-client types, /health schema, control chars, per-key candidates)

Slot 7 (`w7`, port 3700, DB `vrx_w7`, Valkey db 7). Base `main@b36b91c`.

Rounds: initial (items 1–6) done · fix round 1 (review cc70629, D-097) done, CI green @2d0bccb · **fix round 2 (verify ae52906, D-102) done**.

## Fix round 2 (07:54–08:20) — done, CI GATE PASSED @783a9e9; section in TD-2.md
| finding | plan | state |
|---|---|---|
| V1 keys minted during a reset survive | migration 0003 `app_user.credential_gen` (bumped in the reset tx, PG authoritative); `createApiKey` in a tx: `SELECT … FOR SHARE` + re-check (JWT gen / ApiKey row); refresh compares the chain gen with PG | done |
| V2 config-API hash staging (D-102) | `syncUsers` bumps gen + deletes keys for existing users whose hash changed; post-commit revoke + audit row `via: config` | done |
| V3 Valkey fails after the DB commit | in-memory revocation + bus first, Valkey in try/catch, audit `revocationPersisted` | done |
| V4 replaceInflightHash before commit | after the tx resolves, inside `exclusive` | done |
| V5 keepApiKeys audit | audit `keepApiKeys`, `apiKeysKept`, `discardedCandidate` | done |
