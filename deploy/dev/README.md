# Local dev services — PostgreSQL + Valkey on the dev host (P04)

The control plane (ngfw-api, P06) needs **PostgreSQL ≥ 16** and **Valkey**. On the dev/CI host `ngfw-a` they run as
plain **systemd services bound to localhost** — no containers (00-CONTEXT: never Docker/testcontainers).

| service | package (Ubuntu 26.04 archive) | version found 2026-09-23 | unit | listens | auth |
|---|---|---|---|---|---|
| PostgreSQL | `postgresql-18`, `postgresql-client-18` (meta `postgresql` = 18) | 18.6 (≥ 16 ✓) | `postgresql.service` → `postgresql@18-main` | `127.0.0.1:5432` | `local` peer; `host 127.0.0.1/::1` scram-sha-256 |
| Valkey | `valkey-server`, `valkey-tools` (`redis-server` is only a transitional meta-package in 26.04) | 9.0.4 | `valkey-server.service` | `127.0.0.1:6379`, `[::1]:6379`, protected-mode yes | none (localhost only) |

Both were already installed, enabled and active when P04 started, so P04 installed nothing; if a fresh host lacks them:
`apt install postgresql valkey-server valkey-tools` (allowed by P04 item 4 — never `vpp*`, `frr*`, `strongswan*`).
Keep `listen_addresses` / `bind` on localhost. `tools/lab status` reports both (state, version, listen address, PONG).

## Per-slot throwaway databases — `deploy/dev/pg-test.sh`

```
deploy/dev/pg-test.sh create w3     # role ngfw_w3 + database ngfw_w3, random password → /run/ngfw-test/w3/pg.env (0600)
deploy/dev/pg-test.sh dsn w3        # postgres://ngfw_w3:<pw>@127.0.0.1:5432/ngfw_w3?sslmode=disable
deploy/dev/pg-test.sh list
deploy/dev/pg-test.sh drop w3       # terminates sessions, drops database + role, removes pg.env
```

- `<name>` is your `NGFW_TEST_PREFIX` (`w1`…`w12`, see `tools/lab env <slot>`) — the database is `NGFW_PG_DATABASE=ngfw_w<N>`.
- The password is generated on `create` and stored only in `/run/ngfw-test/<name>/pg.env` (tmpfs, mode 0600). Nothing
  secret is committed; test harnesses `source` that file or read `NGFW_PG_DSN` (`NGFW_PG_PASSWORD` overrides for CI).
- `create` is idempotent (reuses the database, refreshes the password); `drop` verifies nothing named `ngfw_<name>` remains.
- Admin access is peer auth as the `postgres` OS user (`runuser` when root); non-root callers need a superuser role.

## Valkey per slot

Slot `N` uses Valkey **db index N** (`NGFW_VALKEY_DB`, printed by `tools/lab env N`): `valkey-cli -h 127.0.0.1 -n N ping`.
Clean up your own keys with `valkey-cli -n N flushdb` — never `flushall`.

## Locks

`/run/lock/ngfw-lab.lock` — integration harnesses take it **shared** (`tools/lab lock shared <cmd>`), the manager's
`tools/ci.sh full` and (after handover) VPP restarts take it **exclusive**. `/run/lock/ngfw-vpp.lock` — manager-only VPP restarts.
