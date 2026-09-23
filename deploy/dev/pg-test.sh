#!/usr/bin/env bash
# deploy/dev/pg-test.sh — throwaway per-slot databases on the localhost dev PostgreSQL (see deploy/dev/README.md).
#   pg-test.sh create <name>   role vrx_<name> (LOGIN, random password) + database vrx_<name> owned by it;
#                              writes /run/vrx-test/<name>/pg.env (0600: VRX_PG_HOST/PORT/USER/PASSWORD/DATABASE/DSN). Idempotent.
#   pg-test.sh drop <name>     terminates sessions, drops database + role, removes pg.env
#   pg-test.sh dsn <name>      prints the DSN from pg.env (contains the password — for `export`, never for logs)
#   pg-test.sh list            lists vrx_* databases with owners
# <name> is the caller's VRX_TEST_PREFIX (w1..w12) or another short lowercase token — everything created carries it.
# The password is generated at create time and lives only in /run (tmpfs), never in the repo (00-CONTEXT: no secrets in files).
set -euo pipefail
PG_HOST="${VRX_PG_HOST:-127.0.0.1}"; PG_PORT="${VRX_PG_PORT:-5432}"
RUN_BASE="${VRX_RUN_DIR:-/run/vrx-test}"
die() { echo "pg-test: $*" >&2; exit 1; }
valid() { [[ "${1:-}" =~ ^[a-z][a-z0-9_]{0,15}$ ]] || die "name '${1:-}' must match ^[a-z][a-z0-9_]{0,15}\$ (e.g. w3)"; }
# admin access = peer auth as the postgres OS user (root → runuser). Non-root callers need their own superuser role.
if [[ $EUID -eq 0 ]]; then adm() { (cd / && runuser -u postgres -- psql -X -v ON_ERROR_STOP=1 -qAt "$@"); }
else adm() { psql -X -v ON_ERROR_STOP=1 -qAt "$@"; }; fi

cmd="${1:-}"; name="${2:-}"
[[ "$cmd" =~ ^(create|drop|dsn|list)$ ]] || { sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'; exit 1; }
[[ "$cmd" == dsn ]] || adm -c 'select 1' >/dev/null 2>&1 || die "cannot reach PostgreSQL as admin — is postgresql active on localhost? (deploy/dev/README.md)"

case "$cmd" in
  create)
    valid "$name"; db="vrx_$name"; role="vrx_$name"; dir="$RUN_BASE/$name"; envf="$dir/pg.env"
    pw="${VRX_PG_PASSWORD:-}"
    [[ -n "$pw" || ! -r "$envf" ]] || pw="$(sed -n 's/^VRX_PG_PASSWORD=//p' "$envf")"
    [[ -n "$pw" ]] || pw="$(head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 24)"
    [[ "$pw" =~ ^[A-Za-z0-9_.-]+$ ]] || die "VRX_PG_PASSWORD may only contain [A-Za-z0-9_.-]"
    if [[ "$(adm -c "select 1 from pg_roles where rolname='$role'")" == 1 ]]; then
      adm -c "alter role $role with login password '$pw'" >/dev/null; echo "reuse  role $role (password refreshed)"
    else adm -c "create role $role with login password '$pw'" >/dev/null; echo "create role $role"; fi
    if [[ "$(adm -c "select 1 from pg_database where datname='$db'")" == 1 ]]; then echo "reuse  database $db"
    else adm -c "create database $db owner $role" >/dev/null; echo "create database $db (owner $role)"; fi
    install -d -m 0700 "$dir"
    ( umask 077; cat > "$envf" <<EOV
VRX_PG_HOST=$PG_HOST
VRX_PG_PORT=$PG_PORT
VRX_PG_USER=$role
VRX_PG_PASSWORD=$pw
VRX_PG_DATABASE=$db
VRX_PG_DSN=postgres://$role:$pw@$PG_HOST:$PG_PORT/$db?sslmode=disable
EOV
    )
    who="$(PGPASSWORD="$pw" psql -X -h "$PG_HOST" -p "$PG_PORT" -U "$role" -d "$db" -qAtc "select current_database() || ' as ' || current_user || ' · ' || split_part(version(), ',', 1)")" \
      || die "TCP login check to $PG_HOST:$PG_PORT failed"
    echo "check  $who"
    echo "ok     env $envf (0600) · DSN postgres://$role:<redacted>@$PG_HOST:$PG_PORT/$db" ;;
  drop)
    valid "$name"; db="vrx_$name"; role="vrx_$name"; dir="$RUN_BASE/$name"
    adm -c "select count(pg_terminate_backend(pid)) from pg_stat_activity where datname='$db' and pid<>pg_backend_pid()" >/dev/null
    adm -c "drop database if exists $db" >/dev/null && echo "drop   database $db"
    adm -c "drop role if exists $role" >/dev/null && echo "drop   role $role"
    rm -f "$dir/pg.env"; rmdir "$dir" 2>/dev/null || true
    left="$(adm -c "select datname from pg_database where datname='$db' union all select rolname from pg_roles where rolname='$role'")"
    [[ -z "$left" ]] || die "still present: $left"
    echo "ok     nothing named $db / $role remains" ;;
  dsn)
    valid "$name"; f="$RUN_BASE/$name/pg.env"; [[ -r "$f" ]] || die "no $f — run: pg-test.sh create $name"
    sed -n 's/^VRX_PG_DSN=//p' "$f" ;;
  list)
    adm -c "select d.datname || '  owner ' || r.rolname from pg_database d join pg_roles r on r.oid = d.datdba where d.datname like 'vrx\_%' order by 1" ;;
esac
