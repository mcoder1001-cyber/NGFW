#!/usr/bin/env bash
# Appliance firstboot only. Never run this script on the shared development host.
set -euo pipefail
umask 077
[[ $EUID == 0 ]] || { echo 'firstboot requires root' >&2; exit 1; }
exec 9>/run/lock/vrx-firstboot.lock
flock -x 9
if [[ -e /var/lib/vrx/firstboot-complete ]]; then
  [[ ! -e /etc/vrx/bootstrap.env ]] || rm -- /etc/vrx/bootstrap.env
  exit 0
fi
[[ -f /etc/vrx/bootstrap.env && ! -L /etc/vrx/bootstrap.env ]] || { echo 'missing bootstrap.env' >&2; exit 1; }
[[ $(stat -c '%u:%a' /etc/vrx/bootstrap.env) == 0:600 ]] || { echo 'bootstrap.env must be root-owned 0600' >&2; exit 1; }
[[ -n ${VRX_BOOTSTRAP_ADMIN_USER:-} && -n ${VRX_BOOTSTRAP_ADMIN_PASSWORD:-} ]] || { echo 'missing bootstrap administrator' >&2; exit 1; }
# Fixed SQL identifiers; no bootstrap values enter a shell or SQL command.
runuser -u postgres -- psql -X -v ON_ERROR_STOP=1 -d postgres >/dev/null <<'SQL'
SELECT 'CREATE ROLE vrx LOGIN' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname='vrx')
\gexec
SQL
runuser -u postgres -- psql -X -v ON_ERROR_STOP=1 -d postgres >/dev/null <<'SQL'
SELECT 'CREATE DATABASE vrx OWNER vrx' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname='vrx')
\gexec
SQL
install -d -m 0750 -o root -g vrx /var/lib/vrx /etc/vrx
if [[ ! -e /var/lib/vrx/secret.key ]]; then
  TEMP=$(mktemp /var/lib/vrx/.secret.XXXXXXXX)
  openssl rand 32 > "$TEMP"
  chmod 0600 "$TEMP"
  chown vrx:vrx "$TEMP"
  mv "$TEMP" /var/lib/vrx/secret.key
fi
[[ -f /var/lib/vrx/secret.key && ! -L /var/lib/vrx/secret.key ]] || { echo 'invalid secret key file' >&2; exit 1; }
[[ $(stat -c '%s:%a:%U' /var/lib/vrx/secret.key) == 32:600:vrx ]] || { echo 'invalid secret key metadata' >&2; exit 1; }
if [[ ! -e /etc/vrx/api.env ]]; then
  TEMP=$(mktemp /etc/vrx/.api-env.XXXXXXXX)
  printf '%s\n' 'VRX_DATABASE_URL=postgresql:///vrx?host=/var/run/postgresql&user=vrx' 'VRX_SECRET_KEY_FILE=/var/lib/vrx/secret.key' > "$TEMP"
  JWT_SECRET=$(openssl rand -hex 32)
  [[ $JWT_SECRET =~ ^[a-f0-9]{64}$ ]] || { echo 'JWT key generation failed' >&2; exit 1; }
  printf 'VRX_JWT_SECRET=%s\n' "$JWT_SECRET" >> "$TEMP"
  chmod 0600 "$TEMP"
  mv "$TEMP" /etc/vrx/api.env
fi
[[ -f /etc/vrx/api.env && ! -L /etc/vrx/api.env && $(stat -c '%u:%a' /etc/vrx/api.env) == 0:600 ]] || { echo 'invalid API environment file' >&2; exit 1; }
# Initial API environment is canonical: optional runtime overrides are configured
# after successful provisioning. In particular JWT_KEY_FILE wins over JWT_SECRET.
/usr/bin/python3 - /etc/vrx/api.env <<'PYENV'
import pathlib, sys
allowed = {'VRX_DATABASE_URL', 'VRX_SECRET_KEY_FILE', 'VRX_JWT_SECRET'}
lines = pathlib.Path(sys.argv[1]).read_text(encoding='utf-8').splitlines()
if any(line.split('=', 1)[0] not in allowed for line in lines):
    sys.exit('initial API environment contains unsupported overrides')
PYENV
[[ -z ${VRX_JWT_KEY_FILE:-} ]] || { echo 'bootstrap JWT key-file override is unsupported' >&2; exit 1; }
for VARIABLE in VRX_DATABASE_URL VRX_SECRET_KEY_FILE; do
  COUNT=$(grep -Ec "^[[:space:]]*$VARIABLE[[:space:]]*=" /etc/vrx/api.env || true)
  [[ $COUNT == 1 ]] || { echo 'invalid persisted API environment assignment count' >&2; exit 1; }
done
grep -Fxq 'VRX_DATABASE_URL=postgresql:///vrx?host=/var/run/postgresql&user=vrx' /etc/vrx/api.env || { echo 'API database configuration requires manual review' >&2; exit 1; }
grep -Fxq 'VRX_SECRET_KEY_FILE=/var/lib/vrx/secret.key' /etc/vrx/api.env || { echo 'API secret configuration requires manual review' >&2; exit 1; }
JWT_COUNT=$(grep -Ec '^[[:space:]]*VRX_JWT_SECRET[[:space:]]*=' /etc/vrx/api.env || true)
[[ $JWT_COUNT == 1 ]] || { echo 'invalid persisted JWT assignment count' >&2; exit 1; }
JWT_SECRET=$(sed -n 's/^VRX_JWT_SECRET=//p' /etc/vrx/api.env)
[[ $JWT_SECRET =~ ^[a-f0-9]{64}$ ]] || { echo 'invalid or duplicate persisted JWT key' >&2; exit 1; }
export VRX_JWT_SECRET=$JWT_SECRET
# Parse generated environment as data instead of sourcing shell code.
VRX_DATABASE_URL=$(sed -n 's/^VRX_DATABASE_URL=//p' /etc/vrx/api.env)
VRX_SECRET_KEY_FILE=$(sed -n 's/^VRX_SECRET_KEY_FILE=//p' /etc/vrx/api.env)
export VRX_DATABASE_URL VRX_SECRET_KEY_FILE NODE_ENV=production
(cd /usr/lib/vrx/api && runuser --preserve-environment -u vrx -- /usr/bin/node bootstrap-db.mjs)
/usr/lib/vrx/tls-bootstrap.sh
printf '%s\n' '{"dataplane":{}}' > /var/lib/vrx/initial-dataplane.json
/usr/lib/vrx/bin/vrx-startupgen -o /etc/vpp/startup.conf /var/lib/vrx/initial-dataplane.json
# Later startup changes use the separately gated apply-startup.sh, never this tool.
if [[ -e /etc/nginx/sites-enabled/vrx || -L /etc/nginx/sites-enabled/vrx ]]; then
  [[ $(readlink /etc/nginx/sites-enabled/vrx) == /etc/vrx/nginx.conf ]] || { echo 'foreign nginx site; manual review required' >&2; exit 1; }
else
  ln -s /etc/vrx/nginx.conf /etc/nginx/sites-enabled/vrx
fi
if [[ -L /etc/nginx/sites-enabled/default && $(readlink /etc/nginx/sites-enabled/default) == /etc/nginx/sites-available/default ]]; then
  rm -- /etc/nginx/sites-enabled/default
fi
/usr/sbin/nginx -t
# Delete bootstrap credentials only after migrations, seed confirmation, TLS,
# startup rendering and nginx validation succeeded. Publish durable completion
# before credential deletion so a crash between the operations safely resumes cleanup.
TEMP=$(mktemp /var/lib/vrx/.firstboot-complete.XXXXXXXX)
printf 'completed\n' > "$TEMP"
sync -f /var/lib/vrx/secret.key
sync -f /etc/vrx/api.env
sync -f "$TEMP"
mv "$TEMP" /var/lib/vrx/firstboot-complete
sync -f /var/lib/vrx
sync -f /etc/vrx
rm -- /etc/vrx/bootstrap.env
systemctl disable vrx-firstboot.service
